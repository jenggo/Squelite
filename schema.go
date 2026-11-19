package main

import (
	"context"
	"database/sql"
	"strings"
)

// Type affinities SQLite gives a declared type.
const (
	affinityInteger = "INTEGER"
	affinityReal    = "REAL"
	affinityNumeric = "NUMERIC"
	affinityText    = "TEXT"
	affinityBlob    = "BLOB"
)

// Schema is a database's tables and views.
type Schema struct {
	Tables []*TableInfo
	Views  []*ViewInfo
}

// TableInfo is one table's columns and introspection.
type TableInfo struct {
	Name         string
	SQL          string
	Columns      []ColumnInfo
	ForeignKeys  []ForeignKey
	Indexes      []IndexInfo
	Triggers     []TriggerInfo
	HasRowid     bool
	WithoutRowid bool
}

// ViewInfo is one view's columns and definition.
type ViewInfo struct {
	Name    string
	SQL     string
	Columns []ColumnInfo
}

// ColumnInfo is one column, as PRAGMA table_xinfo reports it.
type ColumnInfo struct {
	Name       string
	Type       string
	Default    string
	CID        int
	PK         int
	Hidden     int
	NotNull    bool
	HasDefault bool
}

// ForeignKey is one row of PRAGMA foreign_key_list.
type ForeignKey struct {
	Table    string
	From     string
	To       string
	OnUpdate string
	OnDelete string
}

// IndexInfo is one index and its columns.
type IndexInfo struct {
	Name    string
	Columns []string
	Unique  bool
}

// TriggerInfo is one trigger.
type TriggerInfo struct {
	Name string
	SQL  string
}

func (d *Database) loadSchema() error {
	ctx := context.Background()
	rows, err := d.db.QueryContext(ctx, "SELECT name, type, sql FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type entry struct{ name, typ, sql string }
	var entries []entry
	for rows.Next() {
		var e entry
		var text sql.NullString
		if err := rows.Scan(&e.name, &e.typ, &text); err != nil {
			return err
		}
		e.sql = text.String
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	s := &Schema{}
	for _, e := range entries {
		switch e.typ {
		case "table":
			ti, err := d.loadTable(e.name, e.sql)
			if err != nil {
				return err
			}
			s.Tables = append(s.Tables, ti)
		case "view":
			vi, err := d.loadView(e.name, e.sql)
			if err != nil {
				return err
			}
			s.Views = append(s.Views, vi)
		}
	}
	d.schema = s
	return nil
}

func (d *Database) loadTable(name, text string) (*TableInfo, error) {
	ti := &TableInfo{Name: name, SQL: text}
	cols, err := d.columns(name)
	if err != nil {
		return nil, err
	}
	ti.Columns = cols
	ti.HasRowid = d.hasRowid(name)
	ti.WithoutRowid = !ti.HasRowid
	if ti.ForeignKeys, err = d.foreignKeys(name); err != nil {
		return nil, err
	}
	if ti.Indexes, err = d.indexes(name); err != nil {
		return nil, err
	}
	if ti.Triggers, err = d.triggers(name); err != nil {
		return nil, err
	}
	return ti, nil
}

func (d *Database) loadView(name, text string) (*ViewInfo, error) {
	cols, err := d.columns(name)
	if err != nil {
		return nil, err
	}
	return &ViewInfo{Name: name, SQL: text, Columns: cols}, nil
}

func (d *Database) columns(table string) ([]ColumnInfo, error) {
	rows, err := d.db.QueryContext(context.Background(), "PRAGMA table_xinfo("+quoteIdent(table)+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ColumnInfo
	for rows.Next() {
		var ci ColumnInfo
		var typ, dflt sql.NullString
		if err := rows.Scan(&ci.CID, &ci.Name, &typ, &ci.NotNull, &dflt, &ci.PK, &ci.Hidden); err != nil {
			return nil, err
		}
		ci.Type = typ.String
		ci.Default = dflt.String
		ci.HasDefault = dflt.Valid
		out = append(out, ci)
	}
	return out, rows.Err()
}

func (d *Database) hasRowid(table string) bool {
	var x any
	err := d.db.QueryRowContext(context.Background(), "SELECT rowid FROM "+quoteIdent(table)+" LIMIT 1").Scan(&x)
	return err == nil || err == sql.ErrNoRows
}

func (d *Database) foreignKeys(table string) ([]ForeignKey, error) {
	rows, err := d.db.QueryContext(context.Background(), "PRAGMA foreign_key_list("+quoteIdent(table)+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ForeignKey
	for rows.Next() {
		var id, seq int
		var fk ForeignKey
		var match string
		if err := rows.Scan(&id, &seq, &fk.Table, &fk.From, &fk.To, &fk.OnUpdate, &fk.OnDelete, &match); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}

// listedIndex is one row of PRAGMA index_list.
type listedIndex struct {
	name   string
	unique bool
}

func (d *Database) indexes(table string) ([]IndexInfo, error) {
	listed, err := d.indexList(table)
	if err != nil {
		return nil, err
	}
	var out []IndexInfo
	for _, li := range listed {
		cols, err := d.indexColumns(li.name)
		if err != nil {
			return nil, err
		}
		out = append(out, IndexInfo{Name: li.name, Unique: li.unique, Columns: cols})
	}
	return out, nil
}

func (d *Database) indexList(table string) ([]listedIndex, error) {
	rows, err := d.db.QueryContext(context.Background(), "PRAGMA index_list("+quoteIdent(table)+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []listedIndex
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, err
		}
		out = append(out, listedIndex{name: name, unique: unique != 0})
	}
	return out, rows.Err()
}

func (d *Database) indexColumns(index string) ([]string, error) {
	rows, err := d.db.QueryContext(context.Background(), "PRAGMA index_info("+quoteIdent(index)+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, err
		}
		out = append(out, name.String)
	}
	return out, rows.Err()
}

func (d *Database) triggers(table string) ([]TriggerInfo, error) {
	rows, err := d.db.QueryContext(context.Background(), "SELECT name, sql FROM sqlite_master WHERE type = 'trigger' AND tbl_name = ? ORDER BY name", table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []TriggerInfo
	for rows.Next() {
		var tr TriggerInfo
		var text sql.NullString
		if err := rows.Scan(&tr.Name, &text); err != nil {
			return nil, err
		}
		tr.SQL = text.String
		out = append(out, tr)
	}
	return out, rows.Err()
}

// quoteIdent quotes a name for SQL.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = quoteIdent(n)
	}
	return out
}

// affinity is the type affinity SQLite gives a declared type.
func affinity(declType string) string {
	u := strings.ToUpper(declType)
	switch {
	case strings.Contains(u, "INT"):
		return affinityInteger
	case strings.Contains(u, "CHAR"), strings.Contains(u, "CLOB"), strings.Contains(u, "TEXT"):
		return affinityText
	case strings.Contains(u, "BLOB"), u == "":
		return affinityBlob
	case strings.Contains(u, "REAL"), strings.Contains(u, "FLOA"), strings.Contains(u, "DOUB"):
		return affinityReal
	}
	return affinityNumeric
}
