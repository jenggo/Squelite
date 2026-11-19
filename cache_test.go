package main

import "testing"

func TestRowCacheFetchAndEvict(t *testing.T) {
	pinned := map[int64]bool{5: true}
	c := newRowCache()
	c.setCount(1000)

	c.beginBuild()
	if _, ok := c.lookup(0); ok {
		t.Fatal("row 0 should not be loaded yet")
	}
	lo, hi, ok := c.needsFetch()
	if !ok {
		t.Fatal("expected a fetch")
	}
	if lo != 0 || hi != windowSize {
		t.Fatalf("range %d..%d, want 0..%d", lo, hi, windowSize)
	}
	installRange(c, c.beginFetch(), lo, hi, pinned)
	if _, ok := c.lookup(0); !ok {
		t.Fatal("row 0 should be loaded")
	}

	c.beginBuild()
	c.lookup(900)
	lo, hi, ok = c.needsFetch()
	if !ok {
		t.Fatal("expected a fetch after scrolling")
	}
	installRange(c, c.beginFetch(), lo, hi, pinned)

	if _, ok := c.lookup(0); ok {
		t.Fatal("row 0 should be released")
	}
	if _, ok := c.rows[5]; !ok {
		t.Fatal("pinned row 5 should be kept")
	}
	if n := len(c.rows); n > windowSize+1 {
		t.Fatalf("cache holds %d rows", n)
	}
}
