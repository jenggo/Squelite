package main

import (
	"context"
	"testing"
)

func TestStagedEditFollowsRow(t *testing.T) {
	a := newTestApp()
	path := createDB(t,
		`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`,
		`INSERT INTO t (v) VALUES ('a'),('b'),('c'),('d'),('e')`,
	)
	db := openTestDB(t, a, path)
	tab := db.newTab("t", false)

	tab.grid.beginBuild()
	tab.grid.lookup(0)
	lo, hi, ok := tab.grid.needsFetch()
	if !ok {
		t.Fatal("expected a fetch")
	}
	rows, keys, err := tab.query(context.Background(), lo, hi, tab.orderBy())
	if err != nil {
		t.Fatal(err)
	}
	tab.grid.install(tab.grid.beginFetch(), lo, hi, keys, rows, nil)

	tab.stage(3, "v", "changed")
	tab.grid.reset(nil)

	tab.grid.beginBuild()
	tab.grid.lookup(0)
	lo, hi, ok = tab.grid.needsFetch()
	if !ok {
		t.Fatal("expected a refetch")
	}
	rows, keys, err = tab.query(context.Background(), lo, hi, tab.orderBy())
	if err != nil {
		t.Fatal(err)
	}
	tab.grid.install(tab.grid.beginFetch(), lo, hi, keys, rows, nil)

	if k, ok := tab.grid.keyAt(2); !ok || k != 3 {
		t.Fatalf("row at index 2 has key %d, %v; want 3", k, ok)
	}
	if sv, ok := db.edits[editKey{table: "t", rowid: 3, column: "v"}]; !ok || sv.text != "changed" {
		t.Fatalf("staged edit lost: %+v %v", sv, ok)
	}
}

func TestLargeTableBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a million rows")
	}
	a := newTestApp()
	path := createDB(t,
		`CREATE TABLE big (id INTEGER PRIMARY KEY, v TEXT)`,
		`WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM c WHERE i < 1000000)
		 INSERT INTO big (v) SELECT 'x' FROM c`,
	)
	db := openTestDB(t, a, path)
	tab := db.newTab("big", false)
	if tab.count != 1000000 {
		t.Fatalf("count %d, want 1000000", tab.count)
	}

	for _, start := range []int{0, 100000, 500000, 999000} {
		tab.grid.beginBuild()
		for i := start; i < start+20; i++ {
			tab.grid.lookup(i)
		}
		lo, hi, ok := tab.grid.needsFetch()
		if !ok {
			continue
		}
		rows, keys, err := tab.query(context.Background(), lo, hi, tab.orderBy())
		if err != nil {
			t.Fatal(err)
		}
		tab.grid.install(tab.grid.beginFetch(), lo, hi, keys, rows, nil)
		if n := len(tab.grid.rows); n > windowSize+1 {
			t.Fatalf("cache holds %d rows at %d", n, start)
		}
	}
}

func TestReadOnlyPaths(t *testing.T) {
	a := newTestApp()
	path := createDB(t,
		`CREATE TABLE wr (id INTEGER PRIMARY KEY, v TEXT) WITHOUT ROWID`,
		`CREATE TABLE blobs (id INTEGER PRIMARY KEY, b BLOB)`,
		`CREATE VIEW vw AS SELECT id, v FROM wr`,
		`INSERT INTO wr VALUES (1, 'x')`,
		`INSERT INTO blobs VALUES (1, x'00ff')`,
	)
	db := openTestDB(t, a, path)

	if vt := db.newTab("vw", true); vt.editable() {
		t.Fatal("a view should be read-only")
	}
	if wt := db.newTab("wr", false); wt.editable() {
		t.Fatal("a WITHOUT ROWID table should be read-only")
	}
	bt := db.newTab("blobs", false)
	if bt.canEdit(bt.columns[1], []byte{0, 1}) {
		t.Fatal("a blob cell should be read-only")
	}
}
