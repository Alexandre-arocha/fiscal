package packages

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// fakeRegistries serves deps.dev and the npm registry for a few names and
// counts requests per path.
type fakeRegistries struct {
	mu   sync.Mutex
	hits map[string]int
}

func (f *fakeRegistries) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func versions(times ...time.Time) string {
	var parts []string
	for _, t := range times {
		parts = append(parts, fmt.Sprintf(`{"publishedAt":%q}`, t.Format(time.RFC3339)))
	}
	return `{"versions":[` + strings.Join(parts, ",") + `]}`
}

func newFake(t *testing.T) (*Client, *fakeRegistries) {
	t.Helper()
	retryBase = time.Millisecond
	f := &fakeRegistries{hits: map[string]int{}}
	old := time.Date(2014, 3, 14, 0, 0, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits[r.URL.Path]++
		n := f.hits[r.URL.Path]
		f.mu.Unlock()

		switch r.URL.Path {
		case "/v3/systems/npm/packages/old", "/v3/systems/npm/packages/@scope/pkg":
			fmt.Fprint(w, versions(old.AddDate(1, 0, 0), old))
		case "/v3/systems/npm/packages/new":
			fmt.Fprint(w, versions(testNow.AddDate(0, 0, -3)))
		case "/v3/systems/npm/packages/flaky":
			if n == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			fmt.Fprint(w, versions(old))
		case "/v3/systems/npm/packages/down":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/npm/fresh":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return &Client{
		HTTP:    srv.Client(),
		DepsDev: srv.URL,
		NPM:     srv.URL + "/npm",
		PyPI:    srv.URL + "/pypi",
		GoProxy: srv.URL + "/goproxy",
		Now:     func() time.Time { return testNow },
	}, f
}

func TestLookup(t *testing.T) {
	c, _ := newFake(t)
	for _, tc := range []struct {
		name    string
		exists  bool
		created time.Time
	}{
		{"old", true, time.Date(2014, 3, 14, 0, 0, 0, 0, time.UTC)},
		{"@scope/pkg", true, time.Date(2014, 3, 14, 0, 0, 0, 0, time.UTC)},
		{"new", true, testNow.AddDate(0, 0, -3)},
		{"fresh", true, testNow}, // on npm, not yet on deps.dev
		{"ghost", false, time.Time{}},
	} {
		info, err := c.Lookup(context.Background(), NPM, tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if info.Exists != tc.exists || !info.Created.Equal(tc.created) {
			t.Errorf("%s: got %+v, want exists=%v created=%v", tc.name, info, tc.exists, tc.created)
		}
	}
}

func TestLookupRetriesRateLimit(t *testing.T) {
	c, f := newFake(t)
	info, err := c.Lookup(context.Background(), NPM, "flaky")
	if err != nil || !info.Exists {
		t.Fatalf("got %+v, %v", info, err)
	}
	if n := f.count("/v3/systems/npm/packages/flaky"); n != 2 {
		t.Errorf("requests = %d, want 2", n)
	}
}

func TestLookupGivesUpOnServerErrors(t *testing.T) {
	c, f := newFake(t)
	if _, err := c.Lookup(context.Background(), NPM, "down"); !errors.Is(err, errUnavailable) {
		t.Fatalf("err = %v, want errUnavailable", err)
	}
	if n := f.count("/v3/systems/npm/packages/down"); n != maxAttempts {
		t.Errorf("requests = %d, want %d", n, maxAttempts)
	}
}

func TestLookupCache(t *testing.T) {
	c, f := newFake(t)
	c.CacheDir = t.TempDir()
	ctx := context.Background()

	for range 2 {
		if _, err := c.Lookup(ctx, NPM, "old"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Lookup(ctx, NPM, "ghost"); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("/v3/systems/npm/packages/old"); n != 1 {
		t.Errorf("old fetched %d times, want 1 (cached)", n)
	}

	// Negative answers expire after missTTL; positive ones live longer.
	later := testNow.Add(missTTL + time.Minute)
	c.Now = func() time.Time { return later }
	c.Lookup(ctx, NPM, "old")
	c.Lookup(ctx, NPM, "ghost")
	if n := f.count("/v3/systems/npm/packages/old"); n != 1 {
		t.Errorf("old refetched before hitTTL: %d", n)
	}
	if n := f.count("/v3/systems/npm/packages/ghost"); n != 2 {
		t.Errorf("ghost fetched %d times, want 2 (miss expired)", n)
	}
}
