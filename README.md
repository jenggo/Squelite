# squelite

A native SQLite browser written in Go. One binary, no webview, no JavaScript.

Built with [MyGo](https://github.com/egoist/mygo)'s native UI and
[modernc.org/sqlite](https://modernc.org/sqlite), so it compiles with
`CGO_ENABLED=0` and needs no C toolchain.

## Features

- **Several databases at once.** Open files from the command line or the
  file dialog; each is a root in the sidebar.
- **Sidebar tree** of every database's tables and views, with a
  case-insensitive name filter.
- **A tab per table or view**, deduplicated by database and table, each with
  its own close button.
- **Five sub-tabs**: Structure, Data, Constraints, Indexes, Triggers.
- **Virtualized data grid.** Only the rows near the viewport are held in
  memory, so a table of millions of rows opens without reading the whole
  table and scrolls at constant memory.
- **Sorting** by any column, ascending or descending, off the main thread.
- **Inline editing.** Click a cell, type, Enter stages it (amber); Escape
  reverts. Apply writes each row in one transaction and shows what the
  database actually stored. Discard reverts everything.
- **Set NULL** explicitly, insert rows, and delete chosen rows with a
  confirmation.
- **Status dock** at the bottom: the latest message, the active table's row
  and column counts, and a drag handle to expand the full log.

## What it does not do

- No DDL or SQL editor. The schema tabs display definitions; they do not run
  them.
- No BLOB editing. BLOB cells show `(BLOB, size)` and are read-only.
- Views, `WITHOUT ROWID` tables, and files the process cannot write open
  read-only.
- One window. No detached tabs, no session restore.
- No LRU of evicted rows: scrolling back re-queries.

## Build

Requires Go 1.27.4 or newer.

```sh
CGO_ENABLED=0 go build -o squelite .
```

## Run

```sh
./squelite                 # open the window, then use Open…
./squelite ~/.list.db      # open one or more databases immediately
./squelite a.db b.db
```

On Linux the native UI needs GTK 3 and OpenGL at runtime. These are runtime
dependencies only; the build still needs no C toolchain.

## Development

```sh
golangci-lint run
betteralign ./...
go test ./...
go test -race -short ./...
```
