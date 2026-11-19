package packages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/mod/module"
)

// Info is what the registries say about one package.
type Info struct {
	Exists  bool      `json:"exists"`
	Created time.Time `json:"created"` // zero when unknown
}

// Client asks deps.dev about packages and confirms misses against the
// ecosystem's own registry, because deps.dev indexes new packages with delay.
type Client struct {
	HTTP     *http.Client
	DepsDev  string // base URLs; tests point them at a fake server
	NPM      string
	PyPI     string
	GoProxy  string
	CacheDir string // "" disables the on-disk cache
	Now      func() time.Time
}

// NewClient returns a client for the public registries with a cache in the
// user's cache directory.
func NewClient() *Client {
	dir, err := os.UserCacheDir() // "" on error, which disables the cache
	if err == nil {
		dir = filepath.Join(dir, "fiscal", "packages")
	}
	return &Client{
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		DepsDev:  "https://api.deps.dev",
		NPM:      "https://registry.npmjs.org",
		PyPI:     "https://pypi.org",
		GoProxy:  "https://proxy.golang.org",
		CacheDir: dir,
		Now:      time.Now,
	}
}

// Lookup reports whether name exists in eco and when it was first published.
func (c *Client) Lookup(ctx context.Context, eco Ecosystem, name string) (Info, error) {
	key := string(eco) + "/" + name
	if info, ok := c.cached(key); ok {
		return info, nil
	}
	info, err := c.lookup(ctx, eco, name)
	if err != nil {
		return Info{}, err
	}
	c.store(key, info)
	return info, nil
}

func (c *Client) lookup(ctx context.Context, eco Ecosystem, name string) (Info, error) {
	var body struct {
		Versions []struct {
			PublishedAt time.Time `json:"publishedAt"`
		} `json:"versions"`
	}
	found, err := c.getJSON(ctx, c.DepsDev+"/v3/systems/"+string(eco)+"/packages/"+url.PathEscape(name), &body)
	if err != nil {
		return Info{}, err
	}
	if found {
		info := Info{Exists: true}
		for _, v := range body.Versions {
			if !v.PublishedAt.IsZero() && (info.Created.IsZero() || v.PublishedAt.Before(info.Created)) {
				info.Created = v.PublishedAt
			}
		}
		return info, nil
	}

	target := c.registryURL(eco, name)
	if target == "" {
		return Info{}, nil
	}
	resp, err := c.do(ctx, http.MethodHead, target)
	if err != nil {
		return Info{}, err
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		// Exists but deps.dev has not indexed it yet: published moments ago.
		return Info{Exists: true, Created: c.Now()}, nil
	case http.StatusNotFound, http.StatusGone:
		return Info{}, nil
	default:
		return Info{}, fmt.Errorf("%s: HTTP %d", target, resp.StatusCode)
	}
}

func (c *Client) registryURL(eco Ecosystem, name string) string {
	switch eco {
	case NPM:
		return c.NPM + "/" + url.PathEscape(name)
	case PyPI:
		return c.PyPI + "/simple/" + url.PathEscape(name) + "/"
	case Go:
		escaped, err := module.EscapePath(name)
		if err != nil {
			return "" // not a valid module path, so it cannot exist
		}
		return c.GoProxy + "/" + escaped + "/@latest"
	}
	return ""
}

const maxBody = 64 << 20

// getJSON decodes a 200 response into v. A 404 means "not found", not an error.
func (c *Client) getJSON(ctx context.Context, target string, v any) (bool, error) {
	resp, err := c.do(ctx, http.MethodGet, target)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(v); err != nil {
			return false, fmt.Errorf("%s: %w", target, err)
		}
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("%s: HTTP %d", target, resp.StatusCode)
	}
}

// errUnavailable marks a registry that stayed unreachable, rate limited or
// failing through every retry: other lookups would fail the same way.
var errUnavailable = errors.New("registro indisponível")

// retryBase is the first backoff step; tests shrink it.
var retryBase = 500 * time.Millisecond

const (
	maxAttempts   = 4
	maxRetryAfter = 30 * time.Second
)

// do sends the request, retrying network errors, 429 and 5xx with
// exponential backoff or the server's Retry-After.
func (c *Client) do(ctx context.Context, method, target string) (*http.Response, error) {
	var (
		lastErr error
		wait    time.Duration
	)
	for attempt := range maxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		wait = retryBase << attempt

		req, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "fiscal-scanner")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return resp, nil
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("%s: HTTP %d", target, resp.StatusCode)
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			wait = min(time.Duration(secs)*time.Second, maxRetryAfter)
		}
	}
	return nil, fmt.Errorf("%w: %w", errUnavailable, lastErr)
}
