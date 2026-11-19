package packages

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

func parseGoMod(data []byte) (parsed, error) {
	// Parse, not ParseLax: Lax drops replace directives, which we need.
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return parsed{}, err
	}
	var p parsed
	if f.Module != nil {
		p.local = append(p.local, depKey{Go, f.Module.Mod.Path})
	}
	// A replaced module is never fetched under its own path.
	replaced := map[string]bool{}
	for _, r := range f.Replace {
		replaced[r.Old.Path] = true
	}
	for _, r := range f.Require {
		if !replaced[r.Mod.Path] {
			p.deps = append(p.deps, Dep{Ecosystem: Go, Name: r.Mod.Path, Line: r.Syntax.Start.Line})
		}
	}
	return p, nil
}

// goNoProxy returns the module patterns the go command fetches without the
// public proxy: GONOPROXY, which defaults to GOPRIVATE.
func goNoProxy() string {
	if v := goEnv("GONOPROXY"); v != "" {
		return v
	}
	return goEnv("GOPRIVATE")
}

// goEnv reads a Go setting the way the go command does: the environment
// first, then the file written by `go env -w` (GOENV, or its default path).
func goEnv(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	path := os.Getenv("GOENV")
	if path == "off" {
		return ""
	}
	if path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(dir, "go", "env")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+"="); ok {
			return v
		}
	}
	return ""
}
