package securityaudit

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

const (
	// DefaultPassCacheTTL is how long a final allow verdict is reused for the
	// exact same scan text (Claude Code resends the same user turn on every
	// tool continuation).
	DefaultPassCacheTTL = 30 * time.Minute
	// DefaultPassCacheMaxEntries bounds the in-process pass cache.
	DefaultPassCacheMaxEntries = 50000
)

// promptPassCache remembers scan texts that were finally allowed so repeated
// identical requests skip the guard endpoints. Only DecisionAllow is cached;
// flags, blocks and errors always go back to the endpoints.
type promptPassCache struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	order      *list.List // front = oldest insertion
	entries    map[string]*list.Element
}

type passCacheEntry struct {
	key       string
	expiresAt time.Time
}

func newPromptPassCache(ttl time.Duration, maxEntries int) *promptPassCache {
	if ttl <= 0 {
		ttl = DefaultPassCacheTTL
	}
	if maxEntries < 1 {
		maxEntries = DefaultPassCacheMaxEntries
	}
	return &promptPassCache{ttl: ttl, maxEntries: maxEntries, order: list.New(), entries: map[string]*list.Element{}}
}

// passCacheKey binds a verdict to the config version, the enabled endpoint set
// and models, and the exact scan text.
func passCacheKey(cfg ActiveConfig, endpoints []ActiveEndpoint, scanText string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(strconv.FormatInt(cfg.ConfigVersion, 10)))
	_, _ = hasher.Write([]byte{0})
	for _, endpoint := range endpoints {
		_, _ = hasher.Write([]byte(endpoint.ID))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(endpoint.Model))
		_, _ = hasher.Write([]byte{0})
	}
	_, _ = hasher.Write([]byte{1})
	_, _ = hasher.Write([]byte(scanText))
	return hex.EncodeToString(hasher.Sum(nil))
}

// Hit reports whether key holds an unexpired allow verdict. Expired entries
// are removed on access.
func (c *promptPassCache) Hit(key string, now time.Time) bool {
	if c == nil || key == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return false
	}
	entry := element.Value.(*passCacheEntry)
	if !now.Before(entry.expiresAt) {
		c.order.Remove(element)
		delete(c.entries, key)
		return false
	}
	return true
}

// Delete drops key, e.g. when the upstream refused a request the cached
// verdict had allowed.
func (c *promptPassCache) Delete(key string) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		c.order.Remove(element)
		delete(c.entries, key)
	}
}

// Store records an allow verdict for key, evicting the oldest entries when the
// cache is full.
func (c *promptPassCache) Store(key string, now time.Time) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	expiresAt := now.Add(c.ttl)
	if element, ok := c.entries[key]; ok {
		element.Value.(*passCacheEntry).expiresAt = expiresAt
		c.order.MoveToBack(element)
		return
	}
	for len(c.entries) >= c.maxEntries {
		oldest := c.order.Front()
		if oldest == nil {
			break
		}
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*passCacheEntry).key)
	}
	c.entries[key] = c.order.PushBack(&passCacheEntry{key: key, expiresAt: expiresAt})
}
