package packages

import (
	"fmt"
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

func importsOf(p parsed) []string {
	var out []string
	for _, d := range p.imports {
		out = append(out, fmt.Sprintf("%s:%d", d.Import, d.Line))
	}
	return out
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
	write(t, root, "test/fixtures/app/package.json", `{"dependencies": {"fake-on-purpose": "1"}}`)
	write(t, root, "api/requirements-dev.txt", "pytest\n")
	write(t, root, "requirements/base.txt", "django\n")
	write(t, root, "test-requirements.txt", "hypothesis\n")
	write(t, root, "broken/package.json", `{not json`)

	deps, bad, err := Collect(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(deps), []string{"django", "express", "hypothesis", "pytest"}; !slices.Equal(got, want) {
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

func TestCollectDropsLocalAndPrivatePackages(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GOPRIVATE", "git.corp.example")
	root := t.TempDir()
	write(t, root, "package.json", `{"name": "monorepo", "workspaces": ["packages/*"]}`)
	write(t, root, ".npmrc", "@corp:registry=https://npm.corp.example/\n")
	write(t, root, ".yarnrc.yml", "npmScopes:\n  other:\n    npmRegistryServer: \"https://npm.other.example\"\n")
	write(t, root, "packages/a/package.json", `{"name": "@acme/a", "dependencies": {
		"@acme/b": "^1.0.0", "@corp/secret": "^2.0.0", "@other/thing": "^1.0.0", "left-pad": "^1.3.0"}}`)
	write(t, root, "packages/b/package.json", `{"name": "@acme/b"}`)
	write(t, root, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire (\n"+
		"\texample.com/app/lib v0.0.0\n\tgit.corp.example/team/tool v1.0.0\n\tgithub.com/google/uuid v1.6.0\n)\n")
	write(t, root, "lib/go.mod", "module example.com/app/lib\n\ngo 1.22\n")
	write(t, root, "py/pyproject.toml", "[project]\nname = \"Corp_Utils\"\n")
	write(t, root, "requirements.txt", "corp-utils\nrequests\n")

	deps, bad, err := Collect(root)
	if err != nil || len(bad) != 0 {
		t.Fatalf("Collect: %v, bad = %+v", err, bad)
	}
	if got, want := names(deps), []string{"github.com/google/uuid", "left-pad", "requests"}; !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
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
	write(t, root, "src/components/Card.tsx", "export default function Card() {}\n")
	write(t, root, "supabase/migrations/001_init.sql", "create table t ();\n")
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
import { auth } from "@supabase/auth-magic"
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
	// "@components/Card" is a project file through an alias, but a supabase/
	// folder does not explain "@supabase/auth-magic".
	want := []string{
		"@supabase/auth-magic @supabase/auth-magic src/main.ts:11",
		"fancy-ai-helper fancy_ai_helper app/main.py:7",
		"lodash lodash/fp src/main.ts:8",
		"made-up-pkg made-up-pkg src/main.ts:9",
	}
	if !slices.Equal(imported, want) {
		t.Fatalf("imports =\n%v\nwant\n%v", imported, want)
	}
}
