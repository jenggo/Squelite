package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

func (t *Tab) dataView(c *ui.Context) {
	if t.counting {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "Counting rows…").TextColor(c.Theme().TextMuted)
		})
		return
	}
	ui.Column(c).Fill().Children(func() {
		t.toolbar(c)
		if t.count == 0 && len(t.db.insertsFor(t.table)) == 0 {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "This table has no rows").TextColor(c.Theme().TextMuted)
			})
			return
		}
		t.gridView(c)
	})
}

func (t *Tab) toolbar(c *ui.Context) {
	ui.Row(c).Gap(6).Padding(6, 8).Children(func() {
		if ui.Button(c, "✓ Apply").Disabled(!t.hasStaged()).Clicked() {
			t.apply()
		}
		if ui.Button(c, "✕ Discard").Disabled(!t.hasStaged()).Clicked() {
			t.discard()
		}
		if ui.Button(c, "+ Row").Disabled(!t.editable()).Clicked() {
			t.insertRow()
		}
		if ui.Button(c, "− Delete").Disabled(t.chosen.Len() == 0).Clicked() {
			t.db.app.deleteAsk = t
			t.db.app.deleteOpen = true
		}
		if ui.Button(c, "Set NULL").Disabled(t.chosen.Len() == 0 || t.list.Cells.Cursor == "").Clicked() {
			t.setNullChosen()
		}
		ui.Spacer(c)
		if t.working {
			ui.Spinner(c)
		}
	})
}

func (t *Tab) gridView(c *ui.Context) {
	cols := make([]ui.TableColumn, len(t.columns))
	for i, col := range t.columns {
		cols[i] = ui.TableColumn{Title: col.Name, Sortable: true, Align: alignFor(col)}
	}
	n := int(t.count) + len(t.db.insertsFor(t.table))
	t.list.Selection = &t.chosen
	t.list.CellSelect = true
	t.list.Sort = &t.sort
	t.list.Key = func(i int) any { return t.keyAt(i) }
	t.list.CellMenu = func(m *ui.Menu, row, col int) {
		if row >= int(t.count) {
			return
		}
		if m.Item("Set NULL").Chosen() {
			t.stageNull(row, col)
		}
	}
	theme := c.Theme()
	t.list.HeaderCell = func(cell ui.Element, col int) {
		cell.DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, theme.Border, 0)
		})
	}
	t.grid.beginBuild()
	el := ui.Table(c, &t.list, cols, n, func(row, col int) { t.cell(c, row, col) }).Grow(1)
	if el.Changed() && t.sort != t.lastSort {
		t.lastSort = t.sort
		t.resort()
	}
	t.afterBuild()
}

func (t *Tab) cell(c *ui.Context, row, col int) {
	theme := c.Theme()
	if row >= int(t.count) {
		t.insertCell(c, row-int(t.count), col)
		return
	}
	rd, ok := t.grid.lookup(row)
	if !ok {
		gridCell(c, func() { ui.Text(c, "…").TextColor(theme.TextMuted) })
		return
	}
	ci := t.columns[col]
	if sv, ok := t.db.edits[editKey{table: t.table, rowid: rd.key, column: ci.Name}]; ok {
		gridCell(c, func() { ui.Text(c, sv.display()).TextColor(theme.Warning).SingleLine() })
		return
	}
	if t.db.deletes[deleteKey{t.table, rd.key}] {
		gridCell(c, func() {
			ui.Text(c, formatValue(rd.vals[col])).TextColor(theme.TextMuted).Strikethrough().SingleLine()
		})
		return
	}
	if !t.canEdit(ci, rd.vals[col]) {
		gridCell(c, func() {
			txt := ui.Text(c, formatValue(rd.vals[col])).SingleLine()
			if isNumericValue(rd.vals[col]) {
				txt.Font("monospace")
			}
		})
		return
	}
	s := formatValue(rd.vals[col])
	gridCell(c, func() {
		c.Key([3]int64{rd.key, int64(col), int64(t.discardGen)})
		el := ui.EditableText(c, &s)
		if isNumericValue(rd.vals[col]) {
			el.Font("monospace")
		}
		if el.Changed() {
			t.stage(rd.key, ci.Name, s)
		}
		if el.FocusWithin() {
			t.editingRow = rd.key
		}
	})
}

// gridCell builds a cell's content with a bottom and a right border at the
// cell's edges, as a spreadsheet's grid.
func gridCell(c *ui.Context, content func()) {
	t := c.Theme()
	padY, padX := t.Space(1.5), t.Space(2.5)
	cell := ui.Row(c).Fill()
	cell.DrawOver(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X - padX, Y: r.Y + r.H + padY - 1, W: r.W + 2*padX, H: 1}, t.Border, 0)
		p.Fill(ui.Rect{X: r.X + r.W + padX - 1, Y: r.Y - padY, W: 1, H: r.H + 2*padY}, t.Border, 0)
	})
	cell.Children(content)
}

func isNumericValue(v any) bool {
	switch v.(type) {
	case int64, float64:
		return true
	}
	return false
}

func (t *Tab) insertCell(c *ui.Context, i, col int) {
	ins := t.db.insertsFor(t.table)
	if i >= len(ins) {
		return
	}
	ci := t.columns[col]
	s := ins[i].values[ci.Name]
	gridCell(c, func() {
		c.Key([3]int64{-1 - int64(i), int64(col), int64(t.discardGen)})
		if ui.EditableText(c, &s).Changed() {
			ins[i].values[ci.Name] = s
		}
	})
}

func alignFor(col ColumnInfo) ui.Align {
	switch affinity(col.Type) {
	case affinityInteger, affinityReal, affinityNumeric:
		return ui.End
	}
	return ui.Start
}

func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return fmt.Sprintf("(BLOB, %d)", len(x))
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return "0"
	case time.Time:
		return x.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprint(x)
	}
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case []byte:
		n, _ := strconv.ParseInt(string(x), 10, 64)
		return n
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}
