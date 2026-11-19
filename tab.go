package main

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"
)

// rowKey identifies a row in the grid: a table row by its rowid, a staged
// insert by its place, or a row not yet loaded by its index.
type rowKey struct {
	kind int
	v    int64
}

const (
	keyRow = iota
	keyInsert
	keyUnloaded
)

// Tab is one table or view open in the window.
type Tab struct {
	ctx context.Context
	db  *Database

	grid   *rowCache
	cancel context.CancelFunc

	table    string
	viewSQL  string
	chosen   ui.Selection[rowKey]
	list     ui.ListState
	columns  []ColumnInfo
	sort     ui.SortOrder
	lastSort ui.SortOrder

	sub int

	count int64

	editingRow int64
	discardGen int
	isView     bool
	counting   bool
	working    bool
}

func (d *Database) newTab(name string, isView bool) *Tab {
	t := &Tab{db: d, table: name, isView: isView, sub: 1}
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.grid = newRowCache()
	if isView {
		for _, v := range d.schema.Views {
			if v.Name == name {
				t.columns, t.viewSQL = v.Columns, v.SQL
			}
		}
	} else {
		for _, tb := range d.schema.Tables {
			if tb.Name == name {
				t.columns = tb.Columns
			}
		}
	}
	t.startCount(true)
	if isView {
		d.app.logf(d.name, "%s is a view; opened read-only", name)
	} else if !t.hasRowid() {
		d.app.logf(d.name, "%s has no rowid; opened read-only", name)
	}
	return t
}

func (t *Tab) startCount(initial bool) {
	if initial {
		t.counting = true
	}
	t.db.do(func() {
		var n int64
		err := t.db.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+quoteIdent(t.table)).Scan(&n)
		t.db.app.update(func() {
			if err != nil {
				t.db.app.appendLog(logEntry{db: t.db.name, text: fmt.Sprintf("count %s: %v", t.table, err)})
			} else {
				t.count = n
				t.grid.setCount(n)
			}
			t.counting = false
		})
	})
}

func (t *Tab) tableInfo() *TableInfo {
	for _, tb := range t.db.schema.Tables {
		if tb.Name == t.table {
			return tb
		}
	}
	return nil
}

func (t *Tab) hasRowid() bool {
	if t.isView {
		return false
	}
	ti := t.tableInfo()
	return ti != nil && ti.HasRowid
}

func (t *Tab) columnType(name string) string {
	for _, c := range t.columns {
		if c.Name == name {
			return c.Type
		}
	}
	return ""
}

// pinnedSet is the rows that hold a staged edit or an active editor.
func (t *Tab) pinnedSet() map[int64]bool {
	out := map[int64]bool{}
	for k := range t.db.edits {
		if k.table == t.table {
			out[k.rowid] = true
		}
	}
	if t.editingRow != 0 {
		out[t.editingRow] = true
	}
	return out
}

func (t *Tab) keyAt(i int) any {
	if i >= int(t.count) {
		return rowKey{kind: keyInsert, v: int64(i - int(t.count))}
	}
	if k, ok := t.grid.keyAt(i); ok {
		return rowKey{kind: keyRow, v: k}
	}
	return rowKey{kind: keyUnloaded, v: int64(i)}
}

func (t *Tab) canEdit(ci ColumnInfo, val any) bool {
	if t.isView || t.db.readOnly || !t.hasRowid() {
		return false
	}
	if ci.Hidden != 0 {
		return false
	}
	if _, ok := val.([]byte); ok {
		return false
	}
	return true
}

func (t *Tab) editable() bool {
	return !t.isView && !t.db.readOnly && t.hasRowid()
}

func (t *Tab) afterBuild() {
	lo, hi, ok := t.grid.needsFetch()
	if !ok {
		return
	}
	gen := t.grid.beginFetch()
	t.working = true
	ctx := t.ctx
	order := t.orderBy()
	pinned := t.pinnedSet()
	t.db.do(func() { t.fetch(ctx, lo, hi, gen, order, pinned) })
}

func (t *Tab) fetch(ctx context.Context, lo, hi, gen int, order string, pinned map[int64]bool) {
	rows, keys, err := t.query(ctx, lo, hi, order)
	if err != nil {
		t.grid.failFetch(gen)
		t.db.app.update(func() {
			t.working = false
			if ctx.Err() == nil {
				t.db.app.appendLog(logEntry{db: t.db.name, text: fmt.Sprintf("read %s: %v", t.table, err)})
			}
		})
		return
	}
	t.grid.install(gen, lo, hi, keys, rows, pinned)
	t.db.app.update(func() { t.working = false })
}

func (t *Tab) query(ctx context.Context, lo, hi int, order string) ([]*rowData, []int64, error) {
	var q string
	if t.isView {
		q = "SELECT * FROM " + quoteIdent(t.table)
	} else {
		q = "SELECT rowid, * FROM " + quoteIdent(t.table)
	}
	if order != "" {
		q += " ORDER BY " + order
	}
	q += " LIMIT ? OFFSET ?"

	rs, err := t.db.db.QueryContext(ctx, q, hi-lo, lo)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rs.Close() }()
	cols, err := rs.Columns()
	if err != nil {
		return nil, nil, err
	}
	var rows []*rowData
	var keys []int64
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		var key int64
		if t.isView {
			key = int64(lo + len(rows))
		} else {
			key = toInt64(vals[0])
			vals = vals[1:]
		}
		rows = append(rows, &rowData{key: key, vals: vals})
		keys = append(keys, key)
	}
	return rows, keys, rs.Err()
}

func (t *Tab) orderBy() string {
	if t.sort.Column == "" {
		if t.isView {
			return ""
		}
		return "rowid"
	}
	dir := "ASC"
	if t.sort.Descending {
		dir = "DESC"
	}
	o := quoteIdent(t.sort.Column) + " " + dir
	if !t.isView {
		o += ", rowid"
	}
	return o
}

func (t *Tab) resort() {
	t.cancel()
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.grid.reset(t.pinnedSet())
	t.working = true
	t.db.app.wake()
}
