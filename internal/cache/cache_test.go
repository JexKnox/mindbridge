package cache

import (
	"testing"
	"time"
)

func TestLRUBasic(t *testing.T) {
	c := NewLRU[int](2, 0)
	c.Set("a", 1)
	c.Set("b", 2)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("get a = %v %v", v, ok)
	}
	// a 刚被访问过,写入 c 时应淘汰 b。
	c.Set("c", 3)
	if _, ok := c.Get("b"); ok {
		t.Error("b should be evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("a should survive")
	}
}

func TestLRUTTL(t *testing.T) {
	c := NewLRU[int](8, 10*time.Millisecond)
	c.Set("k", 1)
	if _, ok := c.Get("k"); !ok {
		t.Fatal("should hit before expiry")
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Error("should miss after expiry")
	}
}

func TestLRUUpdate(t *testing.T) {
	c := NewLRU[int](2, 0)
	c.Set("a", 1)
	c.Set("a", 2)
	if v, _ := c.Get("a"); v != 2 {
		t.Errorf("a = %d, want 2", v)
	}
}

func TestVersions(t *testing.T) {
	v := NewVersions()
	if v.Get("kb1") != 0 {
		t.Error("initial version should be 0")
	}
	v.Bump("kb1")
	v.Bump("kb1")
	if v.Get("kb1") != 2 {
		t.Errorf("version = %d, want 2", v.Get("kb1"))
	}
	if v.Get("kb2") != 0 {
		t.Error("kb2 should be independent")
	}
}
