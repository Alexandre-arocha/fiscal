package packages

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseGoMod(t *testing.T) {
	p, err := parseGoMod([]byte(`module example.com/demo

go 1.22

require (
	github.com/google/uuid v1.6.0
	golang.org/x/mod v0.20.0 // indirect
	example.com/local v0.0.0
)

replace example.com/local => ../local
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Dep{
		{Ecosystem: Go, Name: "github.com/google/uuid", Line: 6},
		{Ecosystem: Go, Name: "golang.org/x/mod", Line: 7},
	}
	if !slices.Equal(p.deps, want) {
		t.Fatalf("deps = %+v, want %+v", p.deps, want)
	}
	if want := []depKey{{Go, "example.com/demo"}}; !slices.Equal(p.local, want) {
		t.Errorf("local = %v, want %v", p.local, want)
	}
}

func TestGoNoProxy(t *testing.T) {
	file := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(file, []byte("GOPROXY=https://proxy.golang.org,direct\nGOPRIVATE=*.corp.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", file)
	t.Setenv("GONOPROXY", "")
	t.Setenv("GOPRIVATE", "")
	if got := goNoProxy(); got != "*.corp.example" {
		t.Errorf("from go env file: got %q", got)
	}
	t.Setenv("GOPRIVATE", "git.env.example")
	if got := goNoProxy(); got != "git.env.example" {
		t.Errorf("environment should win over the file: got %q", got)
	}
	t.Setenv("GONOPROXY", "git.noproxy.example")
	if got := goNoProxy(); got != "git.noproxy.example" {
		t.Errorf("GONOPROXY should win over GOPRIVATE: got %q", got)
	}
}
