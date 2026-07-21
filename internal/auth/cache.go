// Package auth provides HTTP authentication middleware and actor resolution.
package auth

import (
	"sync"
	"time"
)

type cacheEntry struct {
	info      ActorInfo
	expiresAt time.Time
}

const defaultMaxCacheEntries = 10000

type ActorCache struct {
	mu         sync.RWMutex
	entries    map[string]cacheEntry
	ttl        time.Duration
	maxEntries int
}

func NewActorCache(ttl time.Duration) *ActorCache {
	return &ActorCache{
		entries:    make(map[string]cacheEntry),
		ttl:        ttl,
		maxEntries: defaultMaxCacheEntries,
	}
}

func (c *ActorCache) Get(externalID string) (ActorInfo, bool) {
	c.mu.RLock()
	entry, ok := c.entries[externalID]
	c.mu.RUnlock()
	if !ok {
		return ActorInfo{}, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		if current, exists := c.entries[externalID]; exists && time.Now().After(current.expiresAt) {
			delete(c.entries, externalID)
		}
		c.mu.Unlock()
		return ActorInfo{}, false
	}
	return entry.info, true
}

func (c *ActorCache) Set(externalID string, info ActorInfo) {
	c.mu.Lock()
	if len(c.entries) >= c.maxEntries {
		now := time.Now()
		evicted := false
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
				evicted = true
				break
			}
		}
		if !evicted {
			var oldest string
			var oldestExp time.Time
			for k, v := range c.entries {
				if oldest == "" || v.expiresAt.Before(oldestExp) {
					oldest = k
					oldestExp = v.expiresAt
				}
			}
			if oldest != "" {
				delete(c.entries, oldest)
			}
		}
	}
	c.entries[externalID] = cacheEntry{
		info:      info,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

func (c *ActorCache) Invalidate(externalID string) {
	c.mu.Lock()
	delete(c.entries, externalID)
	c.mu.Unlock()
}
