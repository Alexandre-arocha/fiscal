package packages

import (
	"fmt"
	"slices"
	"testing"
)

func importsOf(p parsed) []string {
	var out []string
	for _, d := range p.imports {
		out = append(out, fmt.Sprintf("%s:%d", d.Import, d.Line))
	}
	return out
}

func TestParseJSImports(t *testing.T) {
	p, _ := parseJSImports([]byte(`import React, { useState } from "react";
import type { Config } from 'cfg-types'
import {
  a,
  b,
} from "multi-line";
import "side-effect";
export * from "re-export";
const fs = require("fs");
const lazy = await import("lazy-mod");
// a.import("not-this") and @import "not-css";
const css = "@import 'also-not'";
export const x = 1;
`))
	want := []string{"react:1", "cfg-types:2", "multi-line:6", "side-effect:7", "re-export:8", "fs:9", "lazy-mod:10"}
	if got := importsOf(p); !slices.Equal(got, want) {
		t.Fatalf("imports = %v, want %v", got, want)
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

func TestNpmPackageOf(t *testing.T) {
	for spec, want := range map[string]string{
		"react":            "react",
		"lodash/fp":        "lodash",
		"@scope/pkg/sub":   "@scope/pkg",
		"./local":          "",
		"/abs/path":        "",
		"node:fs":          "",
		"https://esm.sh/x": "",
		"@/utils":          "",
		"~/lib":            "",
		"#internal":        "",
		"$lib/stores":      "",
	} {
		name, ok := npmPackageOf(spec)
		if !ok {
			name = ""
		}
		if name != want {
			t.Errorf("npmPackageOf(%q) = %q, want %q", spec, name, want)
		}
	}
}

func TestCollectResolvesImports(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GOPRIVATE", "")
	root := t.TempDir()
	write(t, root, "package.json", `{"name": "web", "dependencies": {"react": "^18"}}`)
	write(t, root, "tsconfig.json", `{
  // path aliases
  "compilerOptions": { "paths": { "@app/*": ["src/app/*"] } },
}`)
	write(t, root, "src/components/Button.tsx", "export default function Button() {}\n")
	write(t, root, "src/main.ts", `import React from "react"
import fs from "node:fs"
import path from "path"
import { core } from "@app/core"
import { u } from "@/utils"
import Button from "components/Button"
import Card from "@components/Card"
import fp from "lodash/fp"
import magic from "made-up-pkg"
import again from "made-up-pkg/sub"
`)
	write(t, root, "requirements.txt", "requests\npython-dateutil\n")
	write(t, root, "app/mymodule.py", "VALUE = 1\n")
	write(t, root, "app/main.py", `import os
import yaml
import requests
import dateutil.parser
import mymodule
import bpy
from fancy_ai_helper import magic
`)

	deps, bad, err := Collect(root)
	if err != nil || len(bad) != 0 {
		t.Fatalf("Collect: %v, bad = %+v", err, bad)
	}
	var imported []string
	for _, d := range deps {
		if d.Import != "" {
			imported = append(imported, fmt.Sprintf("%s %s %s:%d", d.Name, d.Import, d.File, d.Line))
		}
	}
	slices.Sort(imported)
	want := []string{
		"fancy-ai-helper fancy_ai_helper app/main.py:7",
		"lodash lodash/fp src/main.ts:8",
		"made-up-pkg made-up-pkg src/main.ts:9",
	}
	if !slices.Equal(imported, want) {
		t.Fatalf("imports =\n%v\nwant\n%v", imported, want)
	}
}
