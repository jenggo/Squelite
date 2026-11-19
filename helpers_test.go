package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func newTestApp() *App {
	a := NewApp()
	a.inline = true
	return a
}

func createDB(t *testing.T, stmts ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	for _, s := range stmts {
		if _, err := raw.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return path
}

func openTestDB(t *testing.T, a *App, path string) *Database {
	t.Helper()
	db, err := openDatabase(a, path)
	if err != nil {
		t.Fatal(err)
	}
	a.dbs = append(a.dbs, db)
	return db
}

func installRange(c *rowCache, gen, lo, hi int, pinned map[int64]bool) {
	keys := make([]int64, hi-lo)
	rows := make([]*rowData, hi-lo)
	for i := range keys {
		keys[i] = int64(lo + i)
		rows[i] = &rowData{key: keys[i], vals: []any{keys[i]}}
	}
	c.install(gen, lo, hi, keys, rows, pinned)
}
