package packages

import (
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
	deps, err := parseNPM([]byte(`{
  "dependencies": {
    "express": "^4.19.0",
    "my-lodash": "npm:lodash@^4.17.0",
    "from-git": "github:owner/repo",
    "from-disk": "file:../lib",
    "in-workspace": "workspace:*",
    "shorthand": "owner/repo"
  },
  "devDependencies": { "@types/node": "^22.0.0" }
}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(deps), []string{"@types/node", "express", "lodash"}; !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for _, d := range deps {
		if d.Name == "express" && d.Line != 3 {
			t.Errorf("express line = %d, want 3", d.Line)
		}
	}
}

func TestParseRequirements(t *testing.T) {
	deps, err := parseRequirements([]byte(`# comentário
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
	if !slices.Equal(deps, want) {
		t.Fatalf("deps = %+v, want %+v", deps, want)
	}
}

func TestParseGoMod(t *testing.T) {
	deps, err := parseGoMod([]byte(`module example.com/demo

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
	if !slices.Equal(deps, want) {
		t.Fatalf("deps = %+v, want %+v", deps, want)
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
