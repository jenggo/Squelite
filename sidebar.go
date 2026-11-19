package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(8).Gap(6).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ui.TextInput(c, &a.filter).Placeholder("Filter tables").Grow(1)
			if ui.Button(c, "Open…").Clicked() {
				a.openDialog()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			if len(a.dbs) == 0 {
				ui.Column(c).Fill().Padding(12).Gap(4).Children(func() {
					ui.Text(c, "No databases open").TextColor(t.TextMuted)
					ui.Text(c, "Open a SQLite file to browse its tables.").FontSize(12).TextColor(t.TextMuted)
				})
				return
			}
			ui.Tree(c, func() { a.tree(c) })
		})
	})
}

func (a *App) tree(c *ui.Context) {
	for _, db := range a.dbs {
		a.treeDatabase(c, db)
	}
}

func (a *App) treeDatabase(c *ui.Context, db *Database) {
	root := ui.TreeItem(c, db.name, &db.open, func() {
		a.treeTables(c, db)
		a.treeViews(c, db)
	})
	root.ContextMenu(func(m *ui.Menu) {
		if m.Item("Close database").Chosen() {
			a.askClose(db)
		}
	})
}

func (a *App) treeTables(c *ui.Context, db *Database) {
	ui.TreeItem(c, fmt.Sprintf("Tables (%d)", len(db.schema.Tables)), &db.tablesOpen, func() {
		for _, tb := range db.schema.Tables {
			if !matchFilter(a.filter, tb.Name) {
				continue
			}
			sel := a.active != nil && a.active.db == db && a.active.table == tb.Name
			if ui.TreeItem(c, tb.Name, nil, nil).Selected(sel).Clicked() {
				a.openTab(db, tb.Name, false)
			}
		}
	})
}

func (a *App) treeViews(c *ui.Context, db *Database) {
	ui.TreeItem(c, fmt.Sprintf("Views (%d)", len(db.schema.Views)), &db.viewsOpen, func() {
		for _, v := range db.schema.Views {
			if !matchFilter(a.filter, v.Name) {
				continue
			}
			sel := a.active != nil && a.active.db == db && a.active.table == v.Name
			item := ui.TreeItem(c, v.Name, nil, nil).Selected(sel)
			item.ContextMenu(func(m *ui.Menu) {
				if m.Item("Show definition").Chosen() {
					a.showViewDefinition(db, v.Name)
				}
			})
			if item.Clicked() {
				a.openTab(db, v.Name, true)
			}
		}
	})
}

func (a *App) showViewDefinition(db *Database, name string) {
	a.openTab(db, name, true)
	for _, t := range db.tabs {
		if t.table == name {
			a.viewDef = t
		}
	}
	a.viewOpen = true
}

func matchFilter(filter, name string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(filter))
}
