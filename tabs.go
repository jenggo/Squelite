package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

const colName = "Name"

func (t *Tab) structureView(c *ui.Context) {
	cols := []ui.TableColumn{
		{Title: colName},
		{Title: "Type", Width: 140},
		{Title: "PK", Width: 50, Align: ui.End},
		{Title: "Nullable", Width: 80},
		{Title: "Default", Width: 140},
		{Title: "Generated", Width: 100},
	}
	ui.Table(c, nil, cols, len(t.columns), func(row, col int) {
		ci := t.columns[row]
		switch col {
		case 0:
			ui.Text(c, ci.Name).SingleLine()
		case 1:
			ui.Text(c, ci.Type).SingleLine()
		case 2:
			if ci.PK > 0 {
				ui.Text(c, "yes")
			}
		case 3:
			if !ci.NotNull {
				ui.Text(c, "yes")
			}
		case 4:
			ui.Text(c, ci.Default).SingleLine()
		case 5:
			if ci.Hidden == 2 || ci.Hidden == 3 {
				ui.Text(c, "yes")
			}
		}
	}).Grow(1)
}

func (t *Tab) constraintsView(c *ui.Context) {
	var fks []ForeignKey
	if ti := t.tableInfo(); ti != nil {
		fks = ti.ForeignKeys
	}
	cols := []ui.TableColumn{
		{Title: "Column"},
		{Title: "References"},
		{Title: "On update", Width: 120},
		{Title: "On delete", Width: 120},
	}
	ui.Table(c, nil, cols, len(fks), func(row, col int) {
		fk := fks[row]
		switch col {
		case 0:
			ui.Text(c, fk.From).SingleLine()
		case 1:
			ui.Text(c, fk.Table+"."+fk.To).SingleLine()
		case 2:
			ui.Text(c, fk.OnUpdate).SingleLine()
		case 3:
			ui.Text(c, fk.OnDelete).SingleLine()
		}
	}).Grow(1)
}

func (t *Tab) indexesView(c *ui.Context) {
	var idx []IndexInfo
	if ti := t.tableInfo(); ti != nil {
		idx = ti.Indexes
	}
	cols := []ui.TableColumn{
		{Title: colName},
		{Title: "Unique", Width: 80},
		{Title: "Columns"},
	}
	ui.Table(c, nil, cols, len(idx), func(row, col int) {
		ix := idx[row]
		switch col {
		case 0:
			ui.Text(c, ix.Name).SingleLine()
		case 1:
			if ix.Unique {
				ui.Text(c, "yes")
			}
		case 2:
			ui.Text(c, strings.Join(ix.Columns, ", ")).SingleLine()
		}
	}).Grow(1)
}

func (t *Tab) triggersView(c *ui.Context) {
	var trs []TriggerInfo
	if ti := t.tableInfo(); ti != nil {
		trs = ti.Triggers
	}
	cols := []ui.TableColumn{{Title: colName, Width: 200}, {Title: "SQL"}}
	ui.Table(c, nil, cols, len(trs), func(row, col int) {
		tr := trs[row]
		switch col {
		case 0:
			ui.Text(c, tr.Name).SingleLine()
		case 1:
			ui.Text(c, tr.SQL).SingleLine()
		}
	}).Grow(1)
}
