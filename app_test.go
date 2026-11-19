package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestThreeDatabasesStaySeparate(t *testing.T) {
	a := newTestApp()
	dbs := make([]*Database, 0, 3)
	for range 3 {
		path := createDB(t,
			`CREATE TABLE server (id INTEGER PRIMARY KEY, name TEXT)`,
			`INSERT INTO server (name) VALUES ('s1'),('s2')`,
		)
		dbs = append(dbs, openTestDB(t, a, path))
	}
	for _, db := range dbs {
		a.openTab(db, "server", false)
	}
	if n := len(a.allTabs()); n != 3 {
		t.Fatalf("%d tabs, want 3", n)
	}
	for i, db := range dbs {
		db.tabs[0].stage(1, "name", fmt.Sprintf("db%d", i))
	}
	for i, db := range dbs {
		sv, ok := db.edits[editKey{table: "server", rowid: 1, column: "name"}]
		if !ok || sv.text != fmt.Sprintf("db%d", i) {
			t.Fatalf("database %d holds %+v, %v", i, sv, ok)
		}
	}
}

func TestViewShowsRows(t *testing.T) {
	a := newTestApp()
	path := createDB(t,
		`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`,
		`INSERT INTO t (v) VALUES ('hello')`,
	)
	db := openTestDB(t, a, path)
	a.openTab(db, "t", false)

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("hello") {
		t.Fatalf("texts: %q", tt.Texts())
	}
}

func TestOpenFailureLeavesSidebarUnchanged(t *testing.T) {
	a := newTestApp()
	path := filepath.Join(t.TempDir(), "not-a-db.txt")
	if err := os.WriteFile(path, []byte("this is not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.openDatabase(path)
	if len(a.dbs) != 0 {
		t.Fatal("a file that is not a database should not be added")
	}
	if len(a.log) == 0 {
		t.Fatal("the failure should be logged")
	}
}

func TestAsyncFetchKeepsPainting(t *testing.T) {
	a := NewApp()
	a.queue = make(chan func(), 100)
	path := createDB(t,
		`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`,
		`INSERT INTO t (v) VALUES ('hello')`,
	)
	db, err := openDatabase(a, path)
	if err != nil {
		t.Fatal(err)
	}
	a.dbs = append(a.dbs, db)
	a.openTab(db, "t", false)
	db.wait()
	a.drain()

	tt := ui.NewTester(a.view, 1200, 800)
	db.wait()
	a.drain()
	tt.Frame()
	if !tt.HasText("hello") {
		t.Fatalf("texts: %q", tt.Texts())
	}
}

func TestOpenDatabaseFromPath(t *testing.T) {
	a := NewApp()
	a.queue = make(chan func(), 10)
	path := createDB(t, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)
	a.openDatabase(path)
	a.drain()
	if len(a.dbs) != 1 {
		t.Fatalf("%d databases, want 1", len(a.dbs))
	}
	if a.dbs[0].name != "test.db" {
		t.Fatalf("name %q, want test.db", a.dbs[0].name)
	}
	if len(a.dbs[0].schema.Tables) != 1 {
		t.Fatalf("%d tables, want 1", len(a.dbs[0].schema.Tables))
	}
}

func TestTabStripClosesTab(t *testing.T) {
	a := newTestApp()
	path := createDB(t,
		`CREATE TABLE t (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE u (id INTEGER PRIMARY KEY)`,
	)
	db := openTestDB(t, a, path)
	a.openTab(db, "t", false)
	a.openTab(db, "u", false)

	tt := ui.NewTester(a.view, 1200, 800)
	if err := tt.Click("Close u"); err != nil {
		t.Fatal(err)
	}
	if len(db.tabs) != 1 || db.tabs[0].table != "t" {
		t.Fatalf("tabs after close: %d", len(db.tabs))
	}
}

func TestSelectingTableFocusesItsTab(t *testing.T) {
	a := newTestApp()
	path := createDB(t,
		`CREATE TABLE t (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE u (id INTEGER PRIMARY KEY)`,
	)
	db := openTestDB(t, a, path)
	a.openTab(db, "t", false)

	tt := ui.NewTester(a.view, 1200, 800)
	if err := tt.Click("u"); err != nil {
		t.Fatal(err)
	}
	if a.active == nil || a.active.table != "u" {
		t.Fatalf("active tab %v, want u", a.active)
	}
}
