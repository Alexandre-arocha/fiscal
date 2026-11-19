package packages

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Alexandre-arocha/fiscal/scanner"
)

func TestCheckerStopsWhenRegistryIsDown(t *testing.T) {
	t.Setenv("GOENV", "off")
	retryBase = time.Millisecond
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), DepsDev: srv.URL, NPM: srv.URL, PyPI: srv.URL, GoProxy: srv.URL, Now: time.Now}
	root := t.TempDir()
	write(t, root, "package.json", `{"dependencies": {"a": "1", "b": "1", "c": "1", "d": "1"}}`)

	ch := &Checker{Client: c, MinAge: time.Hour, Workers: 1}
	findings, err := ch.Check(context.Background(), root)
	if len(findings) != 0 || !errors.Is(err, errUnavailable) || !strings.Contains(err.Error(), "4 pacote(s)") {
		t.Fatalf("findings = %v, err = %v", findings, err)
	}
	// The first package used up its retries; the other three never asked.
	if n := requests.Load(); n != maxAttempts {
		t.Errorf("requests = %d, want %d", n, maxAttempts)
	}
}

func TestCheckerFlagsMissingAndNewPackages(t *testing.T) {
	c, _ := newFake(t)
	root := t.TempDir()
	write(t, root, "package.json", `{
  "dependencies": {
    "old": "^1.0.0",
    "new": "^0.1.0",
    "ghost": "^1.0.0"
  }
}`)
	write(t, root, "web/package.json", `{"dependencies": {"ghost": "^1.0.0"}}`)

	ch := &Checker{Client: c, MinAge: 30 * 24 * time.Hour, Workers: 2}
	findings, err := ch.Check(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}

	bySeverity := map[scanner.Severity]int{}
	for _, f := range findings {
		bySeverity[f.Severity]++
		if f.Line == 0 || f.File == "" {
			t.Errorf("finding without location: %+v", f)
		}
		if f.Severity == scanner.High && !strings.Contains(f.Fix, "nomes sem escopo") {
			t.Errorf("missing npm private-package hint: %q", f.Fix)
		}
	}
	// ghost appears in two manifests; new once; old is fine.
	if bySeverity[scanner.High] != 2 || bySeverity[scanner.Medium] != 1 || len(findings) != 3 {
		t.Fatalf("findings = %+v", findings)
	}
}

func TestCheckerReportsUnknownImports(t *testing.T) {
	t.Setenv("GOENV", "off")
	c, _ := newFake(t)
	root := t.TempDir()
	write(t, root, "app.js", "import g from \"ghost\"\nimport o from \"old\"\n")
	write(t, root, "tool.py", "import ghost\n")

	ch := &Checker{Client: c, MinAge: 30 * 24 * time.Hour, Workers: 2}
	findings, err := ch.Check(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]scanner.Severity{}
	for _, f := range findings {
		got[f.File] = f.Severity
		if !strings.Contains(f.Title, `importa "ghost"`) {
			t.Errorf("title = %q", f.Title)
		}
	}
	// JS imports name the npm package exactly; a Python module's PyPI
	// name is a guess, so it is reported with less certainty.
	if len(findings) != 2 || got["app.js"] != scanner.High || got["tool.py"] != scanner.Medium {
		t.Fatalf("findings = %+v", findings)
	}
}

func TestLiveRegistries(t *testing.T) {
	if os.Getenv("FISCAL_LIVE") == "" {
		t.Skip("set FISCAL_LIVE=1 to query the real registries")
	}
	c := NewClient()
	c.CacheDir = ""
	for _, tc := range []struct {
		eco    Ecosystem
		name   string
		exists bool
	}{
		{NPM, "express", true},
		{NPM, "@types/node", true},
		{NPM, "react-super-auth-helper-zz9", false},
		{PyPI, "requests", true},
		{PyPI, "flask-sqlalchemy", true},
		{PyPI, "fastapi-magic-orm-zz9", false},
		{Go, "github.com/google/uuid", true},
		{Go, "github.com/nobody-zz9/nothing", false},
	} {
		info, err := c.Lookup(context.Background(), tc.eco, tc.name)
		if err != nil {
			t.Errorf("%s/%s: %v", tc.eco, tc.name, err)
			continue
		}
		if info.Exists != tc.exists {
			t.Errorf("%s/%s: exists = %v, want %v", tc.eco, tc.name, info.Exists, tc.exists)
		}
		if info.Exists && info.Created.IsZero() {
			t.Errorf("%s/%s: no creation date", tc.eco, tc.name)
		}
	}
}
