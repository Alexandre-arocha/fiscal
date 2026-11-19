package packages

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Alexandre-arocha/fiscal/scanner"
)

func names(deps []Dep) []string {
	var out []string
	for _, d := range deps {
		out = append(out, d.Name)
	}
	slices.Sort(out)
	return out
}

func TestParseNPM(t *testing.T) {
	p, err := parseNPM([]byte(`{
  "name": "demo",
  "scripts": { "jest": "jest --ci" },
  "dependencies": {
    "express": "^4.19.0",
    "my-lodash": "npm:lodash@^4.17.0",
    "from-git": "github:owner/repo",
    "from-disk": "file:../lib",
    "in-workspace": "workspace:*",
    "shorthand": "owner/repo"
  },
  "devDependencies": { "@types/node": "^22.0.0", "jest": "^29.0.0" }
}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(p.deps), []string{"@types/node", "express", "jest", "lodash"}; !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	lines := map[string]int{}
	for _, d := range p.deps {
		lines[d.Name] = d.Line
	}
	// jest must point at devDependencies, not at the scripts entry.
	if want := map[string]int{"express": 5, "lodash": 6, "jest": 12, "@types/node": 12}; !maps.Equal(lines, want) {
		t.Errorf("lines = %v, want %v", lines, want)
	}
	if want := []depKey{{NPM, "demo"}}; !slices.Equal(p.local, want) {
		t.Errorf("local = %v, want %v", p.local, want)
	}
}

func TestParseRequirements(t *testing.T) {
	p, err := parseRequirements([]byte(`# comentário
requests>=2.31  # http
Flask_SQLAlchemy[async]==3.1 ; python_version >= "3.9"
-r dev.txt
-e git+https://github.com/a/b.git#egg=b
--index-url https://example.com/simple
mypkg @ https://example.com/mypkg.tar.gz
./local/pkg-1.0-py3-none-any.whl

uvicorn
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Dep{
		{Ecosystem: PyPI, Name: "requests", Line: 2},
		{Ecosystem: PyPI, Name: "flask-sqlalchemy", Line: 3},
		{Ecosystem: PyPI, Name: "uvicorn", Line: 10},
	}
	if !slices.Equal(p.deps, want) {
		t.Fatalf("deps = %+v, want %+v", p.deps, want)
	}
	// Git and URL requirements are declared (code may import them) but
	// never looked up.
	var declared []string
	for _, k := range p.declared {
		declared = append(declared, k.name)
	}
	slices.Sort(declared)
	if want := []string{"b", "flask-sqlalchemy", "mypkg", "requests", "uvicorn"}; !slices.Equal(declared, want) {
		t.Errorf("declared = %v, want %v", declared, want)
	}
}

func TestPEP508Name(t *testing.T) {
	type result struct {
		name     string
		registry bool
	}
	for spec, want := range map[string]result{
		"requests":                       {"requests", true},
		"Django>=4.2":                    {"django", true},
		"zope.interface[test] ; os_name": {"zope-interface", true},
		"pkg@ https://example.com/p.whl": {"pkg", false},
		"pkg [extra] @ git+https://x/y":  {"pkg", false},
		"  ":                             {"", false},
		"./local/dir":                    {"", false},
	} {
		name, registry := pep508Name(spec)
		if got := (result{name, registry}); got != want {
			t.Errorf("pep508Name(%q) = %+v, want %+v", spec, got, want)
		}
	}
}

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

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollect(t *testing.T) {
	root := t.TempDir()
	write(t, root, "package.json", `{"dependencies": {"express": "^4"}}`)
	write(t, root, "node_modules/express/package.json", `{"dependencies": {"should-be-skipped": "1"}}`)
	write(t, root, "api/requirements-dev.txt", "pytest\n")
	write(t, root, "broken/package.json", `{not json`)

	deps, bad, err := Collect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(deps), []string{"express", "pytest"}; !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for _, d := range deps {
		if d.Name == "pytest" && d.File != "api/requirements-dev.txt" {
			t.Errorf("pytest file = %q", d.File)
		}
	}
	if len(bad) != 1 || bad[0].File != "broken/package.json" || bad[0].Severity != scanner.Low {
		t.Fatalf("bad = %+v", bad)
	}
}
