package packages

import (
	"slices"
	"testing"
)

func TestParsePyproject(t *testing.T) {
	p, err := parsePyproject([]byte(`[build-system]
requires = ["hatchling"]

[project]
name = "My_App"
dependencies = [
    "requests>=2.31",
    "Flask_SQLAlchemy[async]; python_version >= '3.9'",
    "mylib @ git+https://github.com/acme/mylib",
    "torch",
]

[project.optional-dependencies]
docs = ["mkdocs"]

[dependency-groups]
test = ["pytest", {include-group = "lint"}]
lint = ["ruff"]

[tool.uv.sources]
torch = { index = "pytorch" }

[tool.poetry.dependencies]
python = "^3.11"
httpx = "^0.27"
pydantic = { version = "^2", extras = ["email"] }
internal = { version = "1.0", source = "acme" }
vendored = { path = "../vendored" }
numpy = [{ version = "<2", python = "<3.9" }, { version = ">=2", python = ">=3.9" }]

[tool.poetry.group.dev.dependencies]
black = "^24"
`))
	if err != nil {
		t.Fatal(err)
	}
	// Skipped: the direct reference (mylib), the uv-pinned index (torch), the
	// interpreter (python), the private source (internal) and the path
	// dependency (vendored).
	want := []Dep{
		{Ecosystem: PyPI, Name: "hatchling", Line: 2},
		{Ecosystem: PyPI, Name: "requests", Line: 7},
		{Ecosystem: PyPI, Name: "flask-sqlalchemy", Line: 8},
		{Ecosystem: PyPI, Name: "mkdocs", Line: 14},
		{Ecosystem: PyPI, Name: "ruff", Line: 18},
		{Ecosystem: PyPI, Name: "pytest", Line: 17},
		{Ecosystem: PyPI, Name: "httpx", Line: 25},
		{Ecosystem: PyPI, Name: "numpy", Line: 29},
		{Ecosystem: PyPI, Name: "pydantic", Line: 26},
		{Ecosystem: PyPI, Name: "black", Line: 32},
	}
	if !slices.Equal(p.deps, want) {
		t.Fatalf("deps =\n%+v\nwant\n%+v", p.deps, want)
	}
	if want := []depKey{{PyPI, "my-app"}}; !slices.Equal(p.local, want) {
		t.Errorf("local = %v, want %v", p.local, want)
	}
}

func TestParsePipfile(t *testing.T) {
	p, err := parsePipfile([]byte(`[[source]]
name = "pypi"
url = "https://pypi.org/simple"

[[source]]
name = "private"
url = "https://pypi.corp.example/simple"

[packages]
requests = "*"
django = {version = ">=4.2", extras = ["argon2"]}
corp-lib = {version = "*", index = "private"}
local-pkg = {path = ".", editable = true}
from-git = {git = "https://github.com/a/b.git"}

[dev-packages]
pytest = "*"
`))
	if err != nil {
		t.Fatal(err)
	}
	// corp-lib comes from the private index; local-pkg and from-git are
	// not on PyPI at all. All six are still declared.
	want := []Dep{
		{Ecosystem: PyPI, Name: "django", Line: 11},
		{Ecosystem: PyPI, Name: "requests", Line: 10},
		{Ecosystem: PyPI, Name: "pytest", Line: 17},
	}
	if !slices.Equal(p.deps, want) {
		t.Fatalf("deps =\n%+v\nwant\n%+v", p.deps, want)
	}
	if len(p.declared) != 6 {
		t.Errorf("declared = %v, want 6 entries", p.declared)
	}
}

func TestParsePyprojectInvalid(t *testing.T) {
	if _, err := parsePyproject([]byte("[project\nname = 1")); err == nil {
		t.Fatal("want error for invalid TOML")
	}
}
