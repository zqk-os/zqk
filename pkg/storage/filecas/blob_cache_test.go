package filecas

import (
	"bytes"
	"strings"
	"testing"
)

func TestBlobCache_PutPeekGetCopy(t *testing.T) {
	c := NewBlobCache(8)
	if c.Len() != 0 {
		t.Fatalf("Len=%d want 0", c.Len())
	}
	payload := []byte("kind: backlog_item\n")
	c.Put("k1", payload)
	got, ok := c.Peek("k1")
	if !ok || !bytes.Equal(got, payload) {
		t.Fatalf("Peek got %q ok=%v", got, ok)
	}
	got[0] = 'X'
	again, ok := c.Peek("k1")
	if !ok || again[0] == 'X' {
		t.Fatal("Peek must copy so callers cannot mutate the store")
	}
	fromGet, ok := c.Get("k1")
	if !ok || !bytes.Equal(fromGet, payload) {
		t.Fatalf("Get got %q ok=%v", fromGet, ok)
	}
}

func TestBlobCache_CapEvictsOldestPut(t *testing.T) {
	c := NewBlobCache(2)
	c.Put("a", []byte("A"))
	c.Put("b", []byte("B"))
	c.Put("c", []byte("C"))
	if c.Len() != 2 {
		t.Fatalf("Len=%d want 2", c.Len())
	}
	if _, ok := c.Peek("a"); ok {
		t.Fatal("oldest put (a) must be evicted")
	}
	if _, ok := c.Peek("b"); !ok {
		t.Fatal("b must remain")
	}
	if _, ok := c.Peek("c"); !ok {
		t.Fatal("c must remain")
	}
}

func TestBlobCache_GetSavesFromEviction(t *testing.T) {
	c := NewBlobCache(2)
	c.Put("a", []byte("A"))
	c.Put("b", []byte("B"))
	if _, ok := c.Get("a"); !ok {
		t.Fatal("Get a")
	}
	c.Put("c", []byte("C"))
	if _, ok := c.Peek("a"); !ok {
		t.Fatal("Get must refresh a so b is the LRU victim")
	}
	if _, ok := c.Peek("b"); ok {
		t.Fatal("b must be evicted after Get(a)")
	}
}

func TestBlobCache_PeekDoesNotRefresh(t *testing.T) {
	c := NewBlobCache(2)
	c.Put("a", []byte("A"))
	c.Put("b", []byte("B"))
	if _, ok := c.Peek("a"); !ok {
		t.Fatal("Peek a")
	}
	c.Put("c", []byte("C"))
	if _, ok := c.Peek("a"); ok {
		t.Fatal("Peek must not save a from FIFO eviction")
	}
}

func TestBlobCache_EvictAndClear(t *testing.T) {
	c := NewBlobCache(4)
	c.Put("a", []byte("A"))
	c.Put("b", []byte("B"))
	if !c.Evict("a") {
		t.Fatal("Evict a should report present")
	}
	if c.Evict("a") {
		t.Fatal("second Evict a should miss")
	}
	if _, ok := c.Peek("a"); ok {
		t.Fatal("a must be gone")
	}
	if c.Len() != 1 {
		t.Fatalf("Len=%d want 1", c.Len())
	}
	c.Clear()
	if c.Len() != 0 {
		t.Fatalf("Len=%d want 0 after Clear", c.Len())
	}
	if _, ok := c.Peek("b"); ok {
		t.Fatal("Clear must drop b")
	}
}

func TestBlobCache_EvictMiddleThenCapDropsTrueLRU(t *testing.T) {
	c := NewBlobCache(3)
	c.Put("a", []byte("A"))
	c.Put("b", []byte("B"))
	c.Put("c", []byte("C"))
	if !c.Evict("b") {
		t.Fatal("Evict b")
	}
	c.Put("d", []byte("D"))
	if c.Len() != 3 {
		t.Fatalf("Len=%d want 3", c.Len())
	}
	if _, ok := c.Peek("a"); !ok {
		t.Fatal("a is still LRU among remaining; cap not hit until next new key")
	}
	c.Put("e", []byte("E"))
	if _, ok := c.Peek("a"); ok {
		t.Fatal("after filling the hole, next new key must evict a (true LRU), not a stale pointer")
	}
	if _, ok := c.Peek("c"); !ok {
		t.Fatal("c must remain")
	}
	if _, ok := c.Peek("d"); !ok {
		t.Fatal("d must remain")
	}
	if _, ok := c.Peek("e"); !ok {
		t.Fatal("e must remain")
	}
}

func TestBlobCache_PutOverwriteDoesNotGrow(t *testing.T) {
	c := NewBlobCache(1)
	c.Put("a", []byte("A1"))
	c.Put("a", []byte("A2"))
	if c.Len() != 1 {
		t.Fatalf("Len=%d want 1", c.Len())
	}
	got, ok := c.Peek("a")
	if !ok || string(got) != "A2" {
		t.Fatalf("got %q ok=%v want A2", got, ok)
	}
}

func TestCASBlobCache_RejectsNonHash(t *testing.T) {
	StoreCASBlob("not-a-hash", []byte("x"))
	if _, ok := LookupCASBlob("not-a-hash"); ok {
		t.Fatal("non-hash key must not store")
	}
}

func TestCASBlobCache_RoundTrip(t *testing.T) {
	hash := strings.Repeat("ab", 32)
	payload := []byte("kind: backlog_item\n")
	StoreCASBlob(hash, payload)
	got, ok := LookupCASBlob(hash)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q want %q", got, payload)
	}
	got[0] = 'X'
	again, ok := LookupCASBlob(hash)
	if !ok || again[0] == 'X' {
		t.Fatal("cache must copy on lookup so callers cannot mutate the store")
	}
}
