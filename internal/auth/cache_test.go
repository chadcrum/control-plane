package auth

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestActorCache_SetAndGet(t *testing.T) {
	cache := NewActorCache(time.Minute)
	info := ActorInfo{ActorID: "user-1", ActorType: "user"}

	cache.Set("alice", info)
	got, ok := cache.Get("alice")

	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if got != info {
		t.Fatalf("Get() = %+v, want %+v", got, info)
	}
}

func TestActorCache_MissOnUnknownKey(t *testing.T) {
	cache := NewActorCache(time.Minute)

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss for unknown key")
	}
}

func TestActorCache_ExpiredEntryReturnsMiss(t *testing.T) {
	cache := NewActorCache(time.Millisecond)
	cache.Set("bob", ActorInfo{ActorID: "user-2", ActorType: "user"})

	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Get("bob")
	if ok {
		t.Fatal("expected cache miss for expired entry")
	}
}

func TestActorCache_EntryWithinTTLReturnsHit(t *testing.T) {
	cache := NewActorCache(time.Hour)
	info := ActorInfo{ActorID: "user-3", ActorType: "service-account"}

	cache.Set("svc", info)
	got, ok := cache.Get("svc")

	if !ok {
		t.Fatal("expected cache hit within TTL")
	}
	if got != info {
		t.Fatalf("Get() = %+v, want %+v", got, info)
	}
}

func TestActorCache_MaxEntriesEviction(t *testing.T) {
	cache := NewActorCache(time.Hour)
	cache.maxEntries = 3

	cache.Set("a", ActorInfo{ActorID: "1"})
	cache.Set("b", ActorInfo{ActorID: "2"})
	cache.Set("c", ActorInfo{ActorID: "3"})

	// At capacity — next Set evicts one existing entry.
	cache.Set("d", ActorInfo{ActorID: "4"})

	if len(cache.entries) > 3 {
		t.Fatalf("cache size = %d, want <= 3", len(cache.entries))
	}
	if _, ok := cache.Get("d"); !ok {
		t.Fatal("newest entry should be present")
	}
}

func TestActorCache_Invalidate(t *testing.T) {
	cache := NewActorCache(time.Hour)
	cache.Set("alice", ActorInfo{ActorID: "user-1"})

	cache.Invalidate("alice")
	if _, ok := cache.Get("alice"); ok {
		t.Fatal("expected cache miss after invalidation")
	}
}

func TestActorCache_SetOverwritesExistingEntry(t *testing.T) {
	cache := NewActorCache(time.Minute)
	cache.Set("alice", ActorInfo{ActorID: "old", ActorType: "user"})
	cache.Set("alice", ActorInfo{ActorID: "new", ActorType: "service_account"})

	got, ok := cache.Get("alice")
	if !ok {
		t.Fatal("expected cache hit after overwrite")
	}
	if got.ActorID != "new" {
		t.Fatalf("ActorID = %q, want %q", got.ActorID, "new")
	}
	if got.ActorType != "service_account" {
		t.Fatalf("ActorType = %q, want %q", got.ActorType, "service_account")
	}
}

func TestActorCache_ExpiredEntriesCleanedLazilyOnGet(t *testing.T) {
	cache := NewActorCache(time.Millisecond)
	cache.maxEntries = 3

	cache.Set("a", ActorInfo{ActorID: "1"})
	cache.Set("b", ActorInfo{ActorID: "2"})
	cache.Set("c", ActorInfo{ActorID: "3"})

	time.Sleep(5 * time.Millisecond)

	// Set evicts one entry to make room; Get lazily cleans expired entries.
	cache.Set("fresh", ActorInfo{ActorID: "4"})

	if _, ok := cache.Get("a"); ok {
		t.Fatal("expected expired entry 'a' to miss")
	}
	if _, ok := cache.Get("b"); ok {
		t.Fatal("expected expired entry 'b' to miss")
	}
	if _, ok := cache.Get("c"); ok {
		t.Fatal("expected expired entry 'c' to miss")
	}
	if _, ok := cache.Get("fresh"); !ok {
		t.Fatal("expected fresh entry to be present")
	}
	if len(cache.entries) != 1 {
		t.Fatalf("cache size = %d, want 1 (expired entries cleaned by Get)", len(cache.entries))
	}
}

func TestActorCache_SetEvictsExactlyOneWhenFull(t *testing.T) {
	cache := NewActorCache(time.Hour)
	cache.maxEntries = 3

	cache.Set("a", ActorInfo{ActorID: "1"})
	cache.Set("b", ActorInfo{ActorID: "2"})
	cache.Set("c", ActorInfo{ActorID: "3"})

	cache.Set("d", ActorInfo{ActorID: "4"})

	if len(cache.entries) != 3 {
		t.Fatalf("cache size = %d, want exactly 3 after eviction", len(cache.entries))
	}
	if _, ok := cache.Get("d"); !ok {
		t.Fatal("newest entry 'd' should be present")
	}
}

func TestActorCache_ConcurrentAccess(_ *testing.T) {
	// This test is meaningful under -race to detect data races.
	cache := NewActorCache(time.Second)
	var wg sync.WaitGroup
	const goroutines = 50

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("user-%d", id)
			cache.Set(key, ActorInfo{ActorID: key, ActorType: "user"})
			cache.Get(key)
			cache.Invalidate(key)
			cache.Set(key, ActorInfo{ActorID: key, ActorType: "user"})
			// Also read/invalidate a key another goroutine may be writing
			cache.Get("user-0")
			cache.Invalidate("user-0")
		}(i)
	}
	wg.Wait()
}
