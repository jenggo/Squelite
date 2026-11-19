package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// Database is one open SQLite file: its connection, its schema, its staged
// edits and its open tabs.
type Database struct {
	app *App
	db  *sql.DB

	schema *Schema

	edits   map[editKey]stagedValue
	deletes map[deleteKey]bool

	reqs    chan func()
	done    chan struct{}
	name    string
	tabs    []*Tab
	inserts []*stagedInsert
	mu      sync.Mutex

	readOnly bool
	inline   bool
	closed   bool

	open       bool
	tablesOpen bool
	viewsOpen  bool
}

// openDatabase opens path read-write with WAL and a busy timeout, or
// read-only when the process cannot write it.
func openDatabase(a *App, path string) (*Database, error) {
	readOnly := false
	if f, err := os.OpenFile(path, os.O_RDWR, 0); err != nil {
		readOnly = true
	} else {
		_ = f.Close()
	}

	dsn := "file:" + path + "?_pragma=busy_timeout(5000)"
	if readOnly {
		dsn += "&mode=ro"
	} else {
		dsn += "&_pragma=journal_mode(WAL)"
	}
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqldb.SetMaxOpenConns(1)
	ctx := context.Background()
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	var n int
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master").Scan(&n); err != nil {
		_ = sqldb.Close()
		return nil, err
	}

	d := &Database{
		app:        a,
		name:       filepath.Base(path),
		db:         sqldb,
		readOnly:   readOnly,
		edits:      map[editKey]stagedValue{},
		deletes:    map[deleteKey]bool{},
		done:       make(chan struct{}),
		open:       true,
		tablesOpen: true,
		viewsOpen:  true,
	}
	if err := d.loadSchema(); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	d.inline = a.inline
	d.start()
	return d, nil
}

// start runs the goroutine that serialises this database's queries and
// writes.
func (d *Database) start() {
	if d.inline {
		return
	}
	d.reqs = make(chan func(), 64)
	go func() {
		for {
			select {
			case fn := <-d.reqs:
				fn()
			case <-d.done:
				return
			}
		}
	}()
}

// do runs fn on the database's goroutine, or inline in a test.
func (d *Database) do(fn func()) {
	if d.inline {
		fn()
		return
	}
	select {
	case d.reqs <- fn:
	case <-d.done:
	}
}

// wait blocks until the work queued before it has run.
func (d *Database) wait() {
	if d.inline {
		return
	}
	done := make(chan struct{})
	d.do(func() { close(done) })
	<-done
}

func (d *Database) close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	close(d.done)
	d.mu.Unlock()
	return d.db.Close()
}

// hasStaged reports whether the database holds edits awaiting apply.
func (d *Database) hasStaged() bool {
	return len(d.edits) > 0 || len(d.inserts) > 0 || len(d.deletes) > 0
}

func (d *Database) insertsFor(table string) []*stagedInsert {
	var out []*stagedInsert
	for _, ins := range d.inserts {
		if ins.table == table {
			out = append(out, ins)
		}
	}
	return out
}

func (d *Database) removeInsert(target *stagedInsert) {
	for i, ins := range d.inserts {
		if ins == target {
			d.inserts = append(d.inserts[:i], d.inserts[i+1:]...)
			return
		}
	}
}

func (d *Database) removeInserts(table string) {
	kept := d.inserts[:0]
	for _, ins := range d.inserts {
		if ins.table != table {
			kept = append(kept, ins)
		}
	}
	d.inserts = kept
}

func baseName(path string) string {
	return filepath.Base(path)
}
