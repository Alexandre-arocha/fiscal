package packages

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	hitTTL = 24 * time.Hour
	// A missing name can be registered by an attacker at any moment, so
	// negative answers expire sooner.
	missTTL = time.Hour
)

type cacheEntry struct {
	Info
	Fetched time.Time `json:"fetched"`
}

func (c *Client) cachePath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.CacheDir, hex.EncodeToString(sum[:])+".json")
}

func (c *Client) cached(key string) (Info, bool) {
	if c.CacheDir == "" {
		return Info{}, false
	}
	data, err := os.ReadFile(c.cachePath(key))
	if err != nil {
		return Info{}, false
	}
	var e cacheEntry
	if json.Unmarshal(data, &e) != nil {
		return Info{}, false
	}
	ttl := hitTTL
	if !e.Exists {
		ttl = missTTL
	}
	if c.Now().Sub(e.Fetched) > ttl {
		return Info{}, false
	}
	return e.Info, true
}

// store is best effort: a cache that cannot be written only costs speed.
// It writes to a temp file and renames so concurrent scans never read a
// half-written entry.
func (c *Client) store(key string, info Info) {
	if c.CacheDir == "" || os.MkdirAll(c.CacheDir, 0o700) != nil {
		return
	}
	data, err := json.Marshal(cacheEntry{Info: info, Fetched: c.Now()})
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.CacheDir, "*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), c.cachePath(key)) != nil {
		os.Remove(tmp.Name())
	}
}
