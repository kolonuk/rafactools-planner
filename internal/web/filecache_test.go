package web

import (
	"fmt"
	"testing"
)

func TestFileCacheDropsTheOldestBeyondItsLimit(t *testing.T) {
	c := newFileCache(3)
	for i := 0; i < 5; i++ {
		c.put(fmt.Sprint("k", i), []byte{byte(i)})
	}
	for i, want := range []bool{false, false, true, true, true} {
		if _, ok := c.get(fmt.Sprint("k", i)); ok != want {
			t.Errorf("k%d cached = %v, want %v", i, ok, want)
		}
	}
	c.put("k4", []byte{9}) // the same key again does not grow the list
	if len(c.order) != 3 {
		t.Errorf("%d keys, want 3", len(c.order))
	}
}
