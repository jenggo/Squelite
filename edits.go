package main

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
)

// editKey names one staged cell.
type editKey struct {
	table  string
	column string
	rowid  int64
}

// deleteKey names one row staged for deletion.
type deleteKey struct {
	table string
	rowid int64
}

// stagedInsert is a row added in the grid and not yet written.
type stagedInsert struct {
	values map[string]string
	table  string
}

// stagedValue is a cell's staged value, which may be NULL.
type stagedValue struct {
	text string
	null bool
}

func (v stagedValue) display() string {
	if v.null {
		return "NULL"
	}
	return v.text
}

func (v stagedValue) arg(declType string) any {
	if v.null {
		return nil
	}
	return parseValue(v.text, declType)
}

func (t *Tab) stage(rowid int64, column, value string) {
	t.db.edits[editKey{table: t.table, rowid: rowid, column: column}] = stagedValue{text: value}
	t.db.app.wake()
}

func (t *Tab) stageNull(row, col int) {
	rd, ok := t.grid.lookup(row)
	if !ok {
		return
	}
	t.db.edits[editKey{table: t.table, rowid: rd.key, column: t.columns[col].Name}] = stagedValue{null: true}
	t.db.app.wake()
}

// setNullChosen stages NULL in the chosen column of every chosen row.
func (t *Tab) setNullChosen() {
	col := t.list.Cells.Cursor
	if col == "" {
		return
	}
	for k := range t.chosen.All() {
		if k.kind == keyRow {
			t.db.edits[editKey{table: t.table, rowid: k.v, column: col}] = stagedValue{null: true}
		}
	}
	t.db.app.wake()
}

func (t *Tab) hasStaged() bool {
	for k := range t.db.edits {
		if k.table == t.table {
			return true
		}
	}
	for k := range t.db.deletes {
		if k.table == t.table {
			return true
		}
	}
	return len(t.db.insertsFor(t.table)) > 0
}

func (t *Tab) discard() {
	for k := range t.db.edits {
		if k.table == t.table {
			delete(t.db.edits, k)
		}
	}
	for k := range t.db.deletes {
		if k.table == t.table {
			delete(t.db.deletes, k)
		}
	}
	t.db.removeInserts(t.table)
	t.editingRow = 0
	t.discardGen++
	t.grid.reset(t.pinnedSet())
	t.db.app.wake()
}

func (t *Tab) insertRow() {
	t.db.inserts = append(t.db.inserts, &stagedInsert{table: t.table, values: map[string]string{}})
	t.db.app.wake()
}

func (t *Tab) stageDeletes() {
	for k := range t.chosen.All() {
		if k.kind == keyRow {
			t.db.deletes[deleteKey{t.table, k.v}] = true
		}
	}
	t.chosen.Clear()
	t.db.app.wake()
}

// applyInsert is a staged insert copied for the write, with the live one it
// came from.
type applyInsert struct {
	live   *stagedInsert
	values map[string]string
}

func (t *Tab) apply() {
	if !t.hasStaged() {
		return
	}
	byRow := map[int64]map[string]stagedValue{}
	for k, v := range t.db.edits {
		if k.table != t.table {
			continue
		}
		if byRow[k.rowid] == nil {
			byRow[k.rowid] = map[string]stagedValue{}
		}
		byRow[k.rowid][k.column] = v
	}
	var dels []int64
	for k := range t.db.deletes {
		if k.table == t.table {
			dels = append(dels, k.rowid)
		}
	}
	var ins []applyInsert
	for _, in := range t.db.insertsFor(t.table) {
		cp := map[string]string{}
		maps.Copy(cp, in.values)
		ins = append(ins, applyInsert{live: in, values: cp})
	}
	t.db.do(func() { t.applyNow(byRow, dels, ins) })
}

func (t *Tab) applyNow(byRow map[int64]map[string]stagedValue, dels []int64, ins []applyInsert) {
	accepted, acceptedDels, acceptedIns, errs := t.writeAll(byRow, dels, ins)
	t.db.app.update(func() {
		for k := range accepted {
			delete(t.db.edits, k)
		}
		for k := range acceptedDels {
			delete(t.db.deletes, k)
		}
		for in := range acceptedIns {
			t.db.removeInsert(in)
		}
		for _, e := range errs {
			t.db.app.appendLog(logEntry{db: t.db.name, text: e})
		}
		t.grid.reset(t.pinnedSet())
		t.startCount(false)
	})
}

// writeAll runs the staged writes, one transaction per row, and reports
// what the database accepted.
func (t *Tab) writeAll(byRow map[int64]map[string]stagedValue, dels []int64, ins []applyInsert) (map[editKey]bool, map[deleteKey]bool, map[*stagedInsert]bool, []string) {
	accepted := map[editKey]bool{}
	acceptedDels := map[deleteKey]bool{}
	acceptedIns := map[*stagedInsert]bool{}

	ctx := context.Background()
	conn, err := t.db.db.Conn(ctx)
	if err != nil {
		return accepted, acceptedDels, acceptedIns, []string{fmt.Sprintf("apply: %v", err)}
	}
	defer func() { _ = conn.Close() }()

	errs := t.writeRowEdits(ctx, conn, byRow, accepted)
	errs = append(errs, t.writeDeletes(ctx, conn, dels, acceptedDels)...)
	errs = append(errs, t.writeInserts(ctx, conn, ins, acceptedIns)...)
	return accepted, acceptedDels, acceptedIns, errs
}

func (t *Tab) writeRowEdits(ctx context.Context, conn *sql.Conn, byRow map[int64]map[string]stagedValue, accepted map[editKey]bool) []string {
	var errs []string
	for rowid, cols := range byRow {
		names := make([]string, 0, len(cols))
		for n := range cols {
			names = append(names, n)
		}
		sort.Strings(names)
		sets := make([]string, len(names))
		args := make([]any, 0, len(names)+1)
		for i, n := range names {
			sets[i] = quoteIdent(n) + " = ?"
			args = append(args, cols[n].arg(t.columnType(n)))
		}
		args = append(args, rowid)
		q := fmt.Sprintf("UPDATE %s SET %s WHERE rowid = ? RETURNING %s",
			quoteIdent(t.table), strings.Join(sets, ", "), strings.Join(quoteAll(names), ", "))
		if err := t.writeRow(ctx, conn, q, args, rowid); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", t.table, err))
			continue
		}
		for n := range cols {
			accepted[editKey{table: t.table, rowid: rowid, column: n}] = true
		}
	}
	return errs
}

func (t *Tab) writeDeletes(ctx context.Context, conn *sql.Conn, dels []int64, accepted map[deleteKey]bool) []string {
	var errs []string
	q := fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", quoteIdent(t.table))
	for _, rowid := range dels {
		if err := t.execTx(ctx, conn, q, rowid); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", t.table, err))
			continue
		}
		accepted[deleteKey{t.table, rowid}] = true
	}
	return errs
}

func (t *Tab) writeInserts(ctx context.Context, conn *sql.Conn, ins []applyInsert, accepted map[*stagedInsert]bool) []string {
	var errs []string
	for _, in := range ins {
		q, args := t.insertStatement(in.values)
		if err := t.execTx(ctx, conn, q, args...); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", t.table, err))
			continue
		}
		accepted[in.live] = true
	}
	return errs
}

func (t *Tab) insertStatement(values map[string]string) (string, []any) {
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", quoteIdent(t.table)), nil
	}
	ph := make([]string, len(names))
	args := make([]any, len(names))
	for i, n := range names {
		ph[i] = "?"
		args[i] = parseValue(values[n], t.columnType(n))
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteIdent(t.table), strings.Join(quoteAll(names), ", "), strings.Join(ph, ", "))
	return q, args
}

// writeRow writes one row's staged cells in one transaction and reports
// whether the update matched the row.
func (t *Tab) writeRow(ctx context.Context, conn *sql.Conn, q string, args []any, rowid int64) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	rs, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	matched, err := rowMatched(rs)
	if err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	if !matched {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return fmt.Errorf("row %d changed by another connection", rowid)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	return nil
}

// rowMatched reports whether the statement returned a row, closing the
// result set.
func rowMatched(rs *sql.Rows) (bool, error) {
	defer func() { _ = rs.Close() }()
	matched := rs.Next()
	return matched, rs.Err()
}

func (t *Tab) execTx(ctx context.Context, conn *sql.Conn, q string, args ...any) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, q, args...); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	return nil
}

// parseValue sends text as the storage class the column's affinity takes.
func parseValue(text, declType string) any {
	s := strings.TrimSpace(text)
	switch affinity(declType) {
	case affinityInteger:
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
	case affinityReal:
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	case affinityNumeric:
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return text
}
