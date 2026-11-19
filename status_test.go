package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStatusDockShowsLatest(t *testing.T) {
	a := newTestApp()
	a.logf("app.db", "opened")
	tt := ui.NewTester(a.view, 1200, 800)
	if !tt.HasText("app.db: opened") {
		t.Fatalf("texts: %q", tt.Texts())
	}
}

func TestStatusDockDragResizes(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	box, ok := tt.Find("Resize status")
	if !ok {
		t.Fatal("resize handle not found")
	}
	before := a.statusHeight
	x, y := box.X+box.W/2, box.Y+box.H/2
	tt.Press(x, y)
	tt.Move(x, y-60)
	tt.Release(x, y-60)
	if a.statusHeight <= before {
		t.Fatalf("statusHeight %v, want > %v", a.statusHeight, before)
	}
}

func TestStatusDockShowsCounts(t *testing.T) {
	a := newTestApp()
	path := createDB(t,
		`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`,
		`INSERT INTO t (v) VALUES ('alpha'),('beta')`,
	)
	db := openTestDB(t, a, path)
	a.openTab(db, "t", false)

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("2 rows · 2 columns") {
		t.Fatalf("counts missing: %q", tt.Texts())
	}
}
