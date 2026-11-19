package packages

import (
	"slices"
	"testing"
)

func TestParseSetupCfg(t *testing.T) {
	p, err := parseSetupCfg([]byte(`[metadata]
name = Demo_App

[options]
packages = find:
install_requires =
    requests>=2.31
    click; python_version >= "3.8"
    mylib @ git+https://github.com/acme/mylib
python_requires = >=3.9

[options.extras_require]
dev =
    pytest
docs = mkdocs
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Dep{
		{Ecosystem: PyPI, Name: "requests", Line: 7},
		{Ecosystem: PyPI, Name: "click", Line: 8},
		{Ecosystem: PyPI, Name: "pytest", Line: 14},
		{Ecosystem: PyPI, Name: "mkdocs", Line: 15},
	}
	if !slices.Equal(p.deps, want) {
		t.Fatalf("deps =\n%+v\nwant\n%+v", p.deps, want)
	}
	if want := []depKey{{PyPI, "demo-app"}}; !slices.Equal(p.local, want) {
		t.Errorf("local = %v, want %v", p.local, want)
	}
	if !slices.Contains(p.declared, depKey{PyPI, "mylib"}) {
		t.Errorf("git requirement not declared: %v", p.declared)
	}
}

func TestParseSetupPy(t *testing.T) {
	p, err := parseSetupPy([]byte(`from setuptools import setup

setup(
    name="demo",
    install_requires=[
        "requests>=2.31",
        # pinned for py3.8
        'uvicorn[standard]>=0.30',
    ],
    extras_require={"dev": ["pytest"]},
    tests_require=["hypothesis"],
    setup_requires=build_deps(),
)
`))
	if err != nil {
		t.Fatal(err)
	}
	// extras_require and lists built by code are not read.
	want := []Dep{
		{Ecosystem: PyPI, Name: "requests", Line: 6},
		{Ecosystem: PyPI, Name: "uvicorn", Line: 8},
		{Ecosystem: PyPI, Name: "hypothesis", Line: 11},
	}
	if !slices.Equal(p.deps, want) {
		t.Fatalf("deps =\n%+v\nwant\n%+v", p.deps, want)
	}
}
