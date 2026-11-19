package packages

import (
	"maps"
	"slices"
	"testing"
)

func TestParseNPM(t *testing.T) {
	p, err := parseNPM([]byte(`{
  "name": "demo",
  "scripts": { "jest": "jest --ci" },
  "dependencies": {
    "express": "^4.19.0",
    "my-lodash": "npm:lodash@^4.17.0",
    "react": "catalog:",
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
	if got, want := names(p.deps), []string{"@types/node", "express", "jest", "lodash", "react"}; !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	lines := map[string]int{}
	for _, d := range p.deps {
		lines[d.Name] = d.Line
	}
	// jest must point at devDependencies, not at the scripts entry.
	if want := map[string]int{"express": 5, "lodash": 6, "react": 7, "jest": 13, "@types/node": 13}; !maps.Equal(lines, want) {
		t.Errorf("lines = %v, want %v", lines, want)
	}
	if want := []depKey{{NPM, "demo"}}; !slices.Equal(p.local, want) {
		t.Errorf("local = %v, want %v", p.local, want)
	}
}

func TestParseNpmrc(t *testing.T) {
	p, _ := parseNpmrc([]byte(`registry=https://registry.npmjs.org/
@corp:registry=https://npm.corp.example/
  @Other:registry = https://npm.other.example/
; @commented:registry=https://npm.old.example/
//npm.corp.example/:_authToken=${NPM_TOKEN}
`))
	if want := []string{"@corp", "@Other"}; !slices.Equal(p.private, want) {
		t.Fatalf("scopes = %v, want %v", p.private, want)
	}
}

func TestParseYarnrc(t *testing.T) {
	p, _ := parseYarnrc([]byte(`nodeLinker: node-modules
npmScopes:
  acme:
    npmRegistryServer: "https://npm.acme.example"
    npmAuthToken: "${NPM_TOKEN}"
  "@beta":
    npmRegistryServer: "https://npm.beta.example"
yarnPath: .yarn/releases/yarn.cjs
`))
	if want := []string{"@acme", "@beta"}; !slices.Equal(p.private, want) {
		t.Fatalf("scopes = %v, want %v", p.private, want)
	}
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
