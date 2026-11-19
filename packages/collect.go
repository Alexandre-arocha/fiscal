// Package packages checks that every dependency a project declares or
// imports exists in its public registry and is not suspiciously new,
// catching names an AI made up (slopsquatting).
package packages

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/module"

	"github.com/Alexandre-arocha/fiscal/scanner"
)

// Ecosystem identifies a package registry. Values match deps.dev system names.
type Ecosystem string

const (
	NPM  Ecosystem = "npm"
	PyPI Ecosystem = "pypi"
	Go   Ecosystem = "go"
)

// Dep is one dependency: declared in a manifest or imported by source code.
type Dep struct {
	Ecosystem Ecosystem
	Name      string // registry package name
	Import    string // module as written in an import; "" for manifest entries
	File      string // relative to the scanned root, slash-separated
	Line      int
}

type depKey struct {
	eco  Ecosystem
	name string
}

// parsed is what one file contributes to a scan.
type parsed struct {
	deps     []Dep    // registry dependencies to look up
	declared []depKey // every dependency the file names, wherever it installs from
	local    []depKey // packages the repository defines itself
	private  []string // npm scopes served by a private registry
	aliases  []string // import specifiers resolved by tsconfig paths or import maps
	imports  []Dep    // modules imported by source code, resolved after the walk
}

const (
	maxManifestSize = 5 << 20
	// Larger source files are generated or minified, not written by hand.
	maxSourceSize = 1 << 20
)

// skipDirs hold installed dependencies, build output, caches and test
// fixtures: code the project did not write, or fake on purpose.
var skipDirs = setOf(`
.git node_modules vendor .venv venv bower_components site-packages
dist build out coverage .next .nuxt .output .svelte-kit .yarn __pycache__ .tox
testdata fixtures __fixtures__
`)

// repo is what the walk learns about the repository as a whole.
type repo struct {
	declared map[depKey]bool
	local    map[depKey]bool
	private  map[string]bool // npm scopes, lower case
	aliases  []string
	names    map[string]bool // lower-case folder names and source file stems
	pairs    map[string]bool // the same as "parent/name", for "dir/file" imports
	noProxy  string          // Go module patterns fetched without the public proxy
}

// Collect walks root and gathers the dependencies to look up: those declared
// in manifests and, for JavaScript/TypeScript and Python, those imported by
// source code without being declared. Symlinks and oversized files are
// skipped so a hostile repository cannot point the scan outside root or
// exhaust memory. Unreadable manifests become low-severity findings instead
// of aborting the scan.
//
// Packages the repository defines itself (workspaces, nested modules) and
// packages served by a private registry (.npmrc and .yarnrc.yml scopes,
// GOPRIVATE) are dropped: looking them up publicly would raise false alarms
// and leak private names.
func Collect(root string) ([]Dep, []scanner.Finding, error) {
	r := repo{
		declared: map[depKey]bool{},
		local:    map[depKey]bool{},
		private:  map[string]bool{},
		names:    map[string]bool{},
		pairs:    map[string]bool{},
	}
	var (
		deps, imports []Dep
		bad           []scanner.Finding
	)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() && skipDirs[name] {
			return filepath.SkipDir
		}
		parent := ""
		if dir := filepath.Dir(path); dir != root {
			parent = filepath.Base(dir)
		}
		r.addName(name, d.IsDir(), parent)
		if d.IsDir() {
			return nil
		}
		parse, limit := parserFor(name, parent)
		if parse == nil || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > limit {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		var p parsed
		data, err := os.ReadFile(path)
		if err == nil {
			p, err = parse(data)
		}
		if err != nil {
			bad = append(bad, scanner.Finding{
				Severity: scanner.Low,
				Title:    "não foi possível ler o manifesto: " + err.Error(),
				File:     rel,
			})
			return nil
		}
		r.add(p)
		for _, dep := range p.deps {
			dep.File = rel
			deps = append(deps, dep)
		}
		for _, imp := range p.imports {
			imp.File = rel
			imports = append(imports, imp)
		}
		return nil
	})
	r.noProxy = goNoProxy()
	deps = slices.DeleteFunc(deps, func(d Dep) bool {
		return r.local[depKey{d.Ecosystem, d.Name}] || r.isPrivate(d)
	})
	return append(deps, r.resolveImports(imports)...), bad, err
}

// parserFor picks the parser for a file, and the largest size worth reading.
// parent is the name of the folder holding the file ("" at the root).
func parserFor(name, parent string) (func([]byte) (parsed, error), int64) {
	ext := filepath.Ext(name)
	switch {
	case name == "package.json":
		return parseNPM, maxManifestSize
	case name == ".npmrc":
		return parseNpmrc, maxManifestSize
	case name == ".yarnrc.yml":
		return parseYarnrc, maxManifestSize
	case (strings.HasPrefix(name, "tsconfig") || strings.HasPrefix(name, "jsconfig")) && ext == ".json":
		return parseTSConfig, maxManifestSize
	case name == "deno.json" || name == "deno.jsonc" || name == "import_map.json":
		return parseImportMap, maxManifestSize
	case strings.HasSuffix(name, ".min.js"):
		return nil, 0
	case jsSource[ext]:
		return parseJSImports, maxSourceSize
	case name == "go.mod":
		return parseGoMod, maxManifestSize
	case name == "pyproject.toml":
		return parsePyproject, maxManifestSize
	case name == "Pipfile":
		return parsePipfile, maxManifestSize
	case name == "setup.cfg":
		return parseSetupCfg, maxManifestSize
	case name == "setup.py":
		return parseSetupPy, maxManifestSize
	case name == "environment.yml" || name == "environment.yaml":
		return parseCondaEnv, maxManifestSize
	case isRequirementsFile(name, parent):
		return parseRequirements, maxManifestSize
	case ext == ".py":
		return parsePyImports, maxSourceSize
	case ext == ".ipynb":
		return parseNotebook, maxManifestSize
	}
	return nil, 0
}

// addName records a folder or source file as a local module: Python imports
// and bare or "dir/file" JavaScript imports may point at it.
func (r *repo) addName(name string, isDir bool, parent string) {
	if !isDir {
		ext := filepath.Ext(name)
		if !jsSource[ext] && ext != ".py" {
			return
		}
		name = strings.TrimSuffix(name, ext)
	}
	name = strings.ToLower(name)
	r.names[name] = true
	if parent != "" {
		r.pairs[strings.ToLower(parent)+"/"+name] = true
	}
}

// add merges what one file says about the repository.
func (r *repo) add(p parsed) {
	for _, k := range p.declared {
		r.declared[k] = true
	}
	for _, k := range p.local {
		r.local[k] = true
	}
	for _, scope := range p.private {
		r.private[strings.ToLower(scope)] = true
	}
	r.aliases = append(r.aliases, p.aliases...)
}

// isPrivate reports whether d is served by a private registry: an npm scope
// mapped in the repository's config, or a Go module in GONOPROXY/GOPRIVATE.
func (r *repo) isPrivate(d Dep) bool {
	switch d.Ecosystem {
	case NPM:
		scope, _, ok := strings.Cut(d.Name, "/")
		return ok && r.private[strings.ToLower(scope)]
	case Go:
		return r.noProxy != "" && module.MatchPrefixPatterns(r.noProxy, d.Name)
	}
	return false
}

// resolveImports turns raw imports into lookups for the packages nothing in
// the repository accounts for, keeping the first import of each package per
// file.
func (r *repo) resolveImports(imports []Dep) []Dep {
	provided := r.pyProvided()
	var out []Dep
	index := map[[3]string]int{}
	for _, imp := range imports {
		switch imp.Ecosystem {
		case NPM:
			imp.Name = r.npmImport(imp.Import)
		case PyPI:
			imp.Name = r.pyImport(imp.Import, provided)
		}
		if imp.Name == "" {
			continue
		}
		key := [3]string{string(imp.Ecosystem), imp.Name, imp.File}
		if i, ok := index[key]; ok {
			if imp.Line < out[i].Line {
				out[i] = imp
			}
			continue
		}
		index[key] = len(out)
		out = append(out, imp)
	}
	return out
}

func setOf(words string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

// lineWhere returns the 1-based number of the first line at index from or
// later that satisfies match, or 0 when none does.
func lineWhere(lines []string, from int, match func(string) bool) int {
	for i := from; i < len(lines); i++ {
		if match(lines[i]) {
			return i + 1
		}
	}
	return 0
}

func lineOf(lines []string, from int, needle string) int {
	return lineWhere(lines, from, func(l string) bool { return strings.Contains(l, needle) })
}

// lineIndex maps byte offsets of a text to 1-based line numbers.
type lineIndex []int // offsets of each '\n'

func newLineIndex(text string) lineIndex {
	var idx lineIndex
	for i := range len(text) {
		if text[i] == '\n' {
			idx = append(idx, i)
		}
	}
	return idx
}

func (idx lineIndex) line(offset int) int {
	n, _ := slices.BinarySearch(idx, offset)
	return n + 1
}

func hasAnySuffix(s string, suffixes []string) bool {
	return slices.ContainsFunc(suffixes, func(suf string) bool { return strings.HasSuffix(s, suf) })
}

func hasAnyPrefix(s string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(pre string) bool { return strings.HasPrefix(s, pre) })
}
