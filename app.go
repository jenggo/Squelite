package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// App is the state the window shows: the open databases, the tab strip, the
// sidebar filter and the status log.
type App struct {
	win *mygo.Window

	active *Tab
	// queue, when set, receives the updates a test runs itself.
	queue chan func()

	closeAsk  *Database
	deleteAsk *Tab
	viewDef   *Tab

	filter string

	dbs      []*Database
	log      []logEntry
	tabIndex int

	split        float32
	statusHeight float32

	// inline runs database work on the calling goroutine, for tests.
	inline     bool
	closeOpen  bool
	deleteOpen bool
	viewOpen   bool
}

// NewApp makes the app's state.
func NewApp() *App {
	return &App{split: 240, statusHeight: 26}
}

// wake asks the window for a frame. It is safe from any goroutine.
func (a *App) wake() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// update runs fn on the main thread, where the view reads the state, and
// redraws. It runs fn inline when there is no window, as in a test.
func (a *App) update(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
		return
	}
	if a.queue != nil {
		a.queue <- fn
		return
	}
	fn()
}

// drain runs the updates a test queued.
func (a *App) drain() {
	for {
		select {
		case fn := <-a.queue:
			fn()
		default:
			return
		}
	}
}

// allTabs lists the open tabs of every database, in the order they opened.
func (a *App) allTabs() []*Tab {
	var out []*Tab
	for _, db := range a.dbs {
		out = append(out, db.tabs...)
	}
	return out
}

// indexOf is the place of t in allTabs.
func (a *App) indexOf(t *Tab) int {
	i := 0
	for _, db := range a.dbs {
		for _, x := range db.tabs {
			if x == t {
				return i
			}
			i++
		}
	}
	return 0
}

// view builds the window from the state, for every frame.
func (a *App) view(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		ui.Split(c, &a.split, func() { a.sidebar(c) }, func() { a.main(c, a.allTabs()) }).Grow(1)
		a.statusDock(c)
	})
	a.dialogs(c)
}

func (a *App) main(c *ui.Context, tabs []*Tab) {
	if len(tabs) == 0 {
		a.active = nil
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "Open a database to begin").TextColor(c.Theme().TextMuted)
		})
		return
	}
	if a.tabIndex >= len(tabs) {
		a.tabIndex = len(tabs) - 1
	}
	if a.tabIndex < 0 {
		a.tabIndex = 0
	}
	a.active = tabs[a.tabIndex]
	ui.Column(c).Fill().Children(func() {
		a.tabStrip(c, tabs)
		t := tabs[a.tabIndex]
		ui.Tabs(c, &t.sub, "Structure", "Data", "Constraints", "Indexes", "Triggers")
		switch t.sub {
		case 0:
			t.structureView(c)
		case 1:
			t.dataView(c)
		case 2:
			t.constraintsView(c)
		case 3:
			t.indexesView(c)
		case 4:
			t.triggersView(c)
		}
	})
}

func (a *App) dialogs(c *ui.Context) {
	if a.closeAsk != nil {
		switch ui.AlertDialog(c, &a.closeOpen, "Close "+a.closeAsk.name+"?",
			"It has edits waiting to be applied. Closing discards them.", "Cancel", "Close") {
		case 1:
			a.closeDatabase(a.closeAsk)
			a.closeAsk = nil
		case ui.AlertDismissed:
			a.closeAsk = nil
		}
	}
	if a.deleteAsk != nil {
		switch ui.AlertDialog(c, &a.deleteOpen, "Delete rows?",
			"The chosen rows are marked for deletion and written when you apply.", "Cancel", "Delete") {
		case 1:
			a.deleteAsk.stageDeletes()
			a.deleteAsk = nil
		case ui.AlertDismissed:
			a.deleteAsk = nil
		}
	}
	if a.viewDef != nil {
		ui.DialogBase(c, &a.viewOpen, func(backdrop, panel ui.Element) {
			panel.Width(600).Padding(16).Gap(10).Background(c.Theme().Background).Radius(10).Children(func() {
				ui.Text(c, a.viewDef.table).FontSize(15).Bold()
				ui.Scroll(c).Height(300).Children(func() {
					ui.Text(c, a.viewDef.viewSQL).Font("monospace").FontSize(12)
				})
				ui.Row(c).Gap(8).Children(func() {
					ui.Spacer(c)
					if ui.Button(c, "Close").Clicked() {
						a.viewOpen = false
					}
				})
			})
		})
		if !a.viewOpen {
			a.viewDef = nil
		}
	}
}

func (a *App) openDialog() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: "Open SQLite database",
			Filters: []mygo.FileFilter{{
				Name:       "SQLite",
				Extensions: []string{"db", "sqlite", "sqlite3", "*"},
			}},
		})
		if err != nil {
			a.logf("", "open dialog: %v", err)
			return
		}
		for _, p := range paths {
			a.openDatabase(p)
		}
	}()
}

func (a *App) openDatabase(path string) {
	db, err := openDatabase(a, path)
	if err != nil {
		a.logf(baseName(path), "cannot open: %v", err)
		return
	}
	a.update(func() { a.dbs = append(a.dbs, db) })
	a.logf(db.name, "opened %s", path)
	if db.readOnly {
		a.logf(db.name, "opened read-only: the file is not writable")
	}
}

func (a *App) openTab(db *Database, name string, isView bool) {
	for _, t := range db.tabs {
		if t.table == name {
			a.tabIndex = a.indexOf(t)
			return
		}
	}
	t := db.newTab(name, isView)
	db.tabs = append(db.tabs, t)
	a.tabIndex = a.indexOf(t)
	a.wake()
}

func (a *App) closeTab(t *Tab) {
	db := t.db
	for i, x := range db.tabs {
		if x == t {
			db.tabs = append(db.tabs[:i], db.tabs[i+1:]...)
			break
		}
	}
	t.cancel()
	a.wake()
}

func (a *App) askClose(db *Database) {
	if db.hasStaged() {
		a.closeAsk = db
		a.closeOpen = true
		return
	}
	a.closeDatabase(db)
}

func (a *App) closeDatabase(db *Database) {
	for i, d := range a.dbs {
		if d == db {
			a.dbs = append(a.dbs[:i], a.dbs[i+1:]...)
			break
		}
	}
	for _, t := range db.tabs {
		t.cancel()
	}
	if err := db.close(); err != nil {
		a.logf(db.name, "close: %v", err)
	}
	a.logf(db.name, "closed")
	a.wake()
}
