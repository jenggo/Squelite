package main

import (
	"context"
	"testing"
)

func TestApplyPartialFailure(t *testing.T) {
	a := newTestApp()
	path := createDB(t,
		`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT, u TEXT UNIQUE)`,
		`INSERT INTO t (v, u) VALUES ('a','x'),('b','y'),('c','z')`,
	)
	db := openTestDB(t, a, path)
	tab := db.newTab("t", false)

	tab.stage(1, "v", "A")
	tab.stage(2, "v", "B")
	tab.stage(3, "u", "x")
	tab.apply()

	if _, ok := db.edits[editKey{table: "t", rowid: 1, column: "v"}]; ok {
		t.Fatal("accepted edit should be cleared")
	}
	if _, ok := db.edits[editKey{table: "t", rowid: 2, column: "v"}]; ok {
		t.Fatal("accepted edit should be cleared")
	}
	if _, ok := db.edits[editKey{table: "t", rowid: 3, column: "u"}]; !ok {
		t.Fatal("rejected edit should stay staged")
	}

	var v string
	if err := db.db.QueryRowContext(context.Background(), "SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "A" {
		t.Fatalf("row 1 holds %q, want A", v)
	}
	if err := db.db.QueryRowContext(context.Background(), "SELECT u FROM t WHERE id = 3").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "z" {
		t.Fatalf("row 3 holds %q, want z", v)
	}
}
