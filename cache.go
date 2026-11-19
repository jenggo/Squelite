package main

import (
	"math"
	"sync"
)

// windowSize is how many rows a fetch holds around the viewport.
const windowSize = 200

// rowData is one row as the driver returned it.
type rowData struct {
	vals []any
	key  int64
}

// rowCache holds the rows near the viewport and releases those that leave
// it, so memory does not grow with the table.
type rowCache struct {
	pos  map[int]int64
	rows map[int64]*rowData

	lo int
	hi int

	wantLo int
	wantHi int
	gen    int

	count      int64
	mu         sync.Mutex
	fetching   bool
	countKnown bool
}

func newRowCache() *rowCache {
	return &rowCache{
		pos:    map[int]int64{},
		rows:   map[int64]*rowData{},
		wantLo: math.MaxInt,
	}
}

// beginBuild forgets the range the last build asked for.
func (c *rowCache) beginBuild() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wantLo, c.wantHi = math.MaxInt, 0
}

// lookup returns the row at index i, recording that the build asked for it.
func (c *rowCache) lookup(i int) (*rowData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i < c.wantLo {
		c.wantLo = i
	}
	if i+1 > c.wantHi {
		c.wantHi = i + 1
	}
	key, ok := c.pos[i]
	if !ok {
		return nil, false
	}
	r, ok := c.rows[key]
	return r, ok
}

// keyAt returns the rowid at index i when it is loaded.
func (c *rowCache) keyAt(i int) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key, ok := c.pos[i]
	return key, ok
}

// needsFetch reports the range to fetch when the build asked for rows the
// cache does not cover.
func (c *rowCache) needsFetch() (int, int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fetching || !c.countKnown || c.wantHi <= c.wantLo {
		return 0, 0, false
	}
	if c.wantLo >= c.lo && c.wantHi <= c.hi {
		return 0, 0, false
	}
	lo := max(c.wantLo-windowSize/4, 0)
	hi := min(max(lo+windowSize, c.wantHi), int(c.count))
	if lo > c.wantLo {
		lo = c.wantLo
	}
	return lo, hi, true
}

func (c *rowCache) beginFetch() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetching = true
	return c.gen
}

func (c *rowCache) failFetch(gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen == c.gen {
		c.fetching = false
	}
}

// install replaces the window with the fetched rows, keeping pinned rows
// that left it.
func (c *rowCache) install(gen, lo, hi int, keys []int64, rows []*rowData, pinned map[int64]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return
	}
	c.fetching = false
	pos := make(map[int]int64, len(keys))
	kept := make(map[int64]*rowData, len(rows))
	for i, k := range keys {
		pos[lo+i] = k
		kept[k] = rows[i]
	}
	for k, r := range c.rows {
		if _, ok := kept[k]; ok {
			continue
		}
		if pinned[k] {
			kept[k] = r
		}
	}
	c.pos, c.rows, c.lo, c.hi = pos, kept, lo, hi
}

// reset drops the window and invalidates in-flight fetches, keeping pinned
// rows.
func (c *rowCache) reset(pinned map[int64]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.lo, c.hi = 0, 0
	c.pos = map[int]int64{}
	kept := map[int64]*rowData{}
	for k, r := range c.rows {
		if pinned[k] {
			kept[k] = r
		}
	}
	c.rows = kept
	c.fetching = false
	c.wantLo, c.wantHi = math.MaxInt, 0
}

func (c *rowCache) setCount(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count, c.countKnown = n, true
}
