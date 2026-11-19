package packages

import (
	"slices"
	"testing"
)

func TestIsRequirementsFile(t *testing.T) {
	for _, tc := range []struct {
		name, parent string
		want         bool
	}{
		{"requirements.txt", "", true},
		{"requirements-dev.txt", "", true},
		{"dev-requirements.txt", "", true},
		{"requirements.in", "", true},
		{"base.txt", "requirements", true},
		{"notes.txt", "docs", false},
		{"requirements.md", "", false},
	} {
		if got := isRequirementsFile(tc.name, tc.parent); got != tc.want {
			t.Errorf("isRequirementsFile(%q, %q) = %v, want %v", tc.name, tc.parent, got, tc.want)
		}
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

func TestParsePyImports(t *testing.T) {
	p, _ := parsePyImports([]byte(`"""Module docstring.

import not_this
"""
import os, sys as system
import yaml.composer
from requests.adapters import HTTPAdapter
from . import sibling
from .pkg import thing
x = 1; import json
import numpy as np  # comment
def f():
    import lazy_mod
s = "import fake"
`))
	want := []string{"os:5", "sys:5", "yaml:6", "requests:7", "json:10", "numpy:11", "lazy_mod:13"}
	if got := importsOf(p); !slices.Equal(got, want) {
		t.Fatalf("imports = %v, want %v", got, want)
	}
}

func TestPyImportSkipsStandardLibrary(t *testing.T) {
	var r repo
	// Python 2 names come from compatibility code such as psf/requests'
	// "try: import StringIO / except ImportError: import io".
	for _, mod := range []string{"os", "tomllib", "StringIO", "cStringIO", "BaseHTTPServer", "urllib2", "_thread", "__future__"} {
		if got := r.pyImport(mod, nil); got != "" {
			t.Errorf("pyImport(%q) = %q, want it skipped", mod, got)
		}
	}
	if got := r.pyImport("fancy_helper", nil); got != "fancy-helper" {
		t.Errorf("pyImport(fancy_helper) = %q, want fancy-helper", got)
	}
}

func TestParseNotebook(t *testing.T) {
	p, err := parseNotebook([]byte(`{
 "cells": [
  {
   "cell_type": "markdown",
   "source": ["# import nothing_here\n"]
  },
  {
   "cell_type": "code",
   "source": [
    "%pip install -q fancy-lib==1.0 \"other-lib>=2\"\n",
    "import pandas as pd\n",
    "from fancy_lib import magic"
   ]
  },
  {
   "cell_type": "code",
   "source": "!pip install -r requirements.txt\nimport os"
  }
 ],
 "nbformat": 4
}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Dep{
		{Ecosystem: PyPI, Name: "fancy-lib", Line: 10},
		{Ecosystem: PyPI, Name: "other-lib", Line: 10},
	}
	if !slices.Equal(p.deps, want) {
		t.Fatalf("deps = %+v, want %+v", p.deps, want)
	}
	if got, want := importsOf(p), []string{"pandas:11", "fancy_lib:12", "os:17"}; !slices.Equal(got, want) {
		t.Fatalf("imports = %v, want %v", got, want)
	}
}

func TestParseCondaEnv(t *testing.T) {
	p, err := parseCondaEnv([]byte(`name: ds
channels:
  - conda-forge
dependencies:
  - python=3.11
  - conda-forge::numpy=1.26
  - cudf  # rapids
  - pip
  - pip:
      - requests>=2.31  # see https://example.com
      - "fancy-ai-lib"
  - pandas
prefix: /opt/conda
`))
	if err != nil {
		t.Fatal(err)
	}
	// Only the pip section comes from PyPI; conda packages are declared.
	want := []Dep{
		{Ecosystem: PyPI, Name: "requests", Line: 10},
		{Ecosystem: PyPI, Name: "fancy-ai-lib", Line: 11},
	}
	if !slices.Equal(p.deps, want) {
		t.Fatalf("deps = %+v, want %+v", p.deps, want)
	}
	for _, name := range []string{"numpy", "cudf", "pandas"} {
		if !slices.Contains(p.declared, depKey{PyPI, name}) {
			t.Errorf("%s not declared: %v", name, p.declared)
		}
	}
}
