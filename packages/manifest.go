// Package packages checks that every dependency a project declares or
// imports exists in its public registry and is not suspiciously new,
// catching names an AI made up (slopsquatting).
package packages

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"

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

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true, "venv": true,
	// Build output and caches hold generated code, not the project's imports.
	"dist": true, "build": true, "out": true, "coverage": true, ".next": true, ".nuxt": true,
	".output": true, ".svelte-kit": true, ".yarn": true, "bower_components": true,
	"__pycache__": true, "site-packages": true, ".tox": true,
}

var jsSource = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true,
	".mts": true, ".cts": true, ".vue": true, ".svelte": true, ".astro": true,
}

// repo is what the walk learns about the repository as a whole.
type repo struct {
	declared map[depKey]bool
	local    map[depKey]bool
	private  map[string]bool // npm scopes, lower case
	aliases  []string
	names    map[string]bool // lower-case directory names and source file stems
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
	}
	var (
		deps, imports []Dep
		bad           []scanner.Finding
	)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			if skipDirs[name] {
				return filepath.SkipDir
			}
			r.names[strings.ToLower(name)] = true
			return nil
		}
		if ext := filepath.Ext(name); jsSource[ext] || ext == ".py" {
			r.names[strings.ToLower(strings.TrimSuffix(name, ext))] = true
		}
		parse, limit := parserFor(name)
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
		for _, dep := range p.deps {
			dep.File = rel
			deps = append(deps, dep)
		}
		for _, imp := range p.imports {
			imp.File = rel
			imports = append(imports, imp)
		}
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
		return nil
	})
	r.noProxy = goNoProxy()
	deps = slices.DeleteFunc(deps, func(d Dep) bool {
		return r.local[depKey{d.Ecosystem, d.Name}] || isPrivate(d, r.private, r.noProxy)
	})
	return append(deps, r.resolveImports(imports)...), bad, err
}

func parserFor(name string) (func([]byte) (parsed, error), int64) {
	ext := filepath.Ext(name)
	switch {
	case name == "package.json":
		return parseNPM, maxManifestSize
	case name == ".npmrc":
		return parseNpmrc, maxManifestSize
	case name == ".yarnrc.yml":
		return parseYarnrc, maxManifestSize
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
	case strings.HasPrefix(name, "requirements") && ext == ".txt":
		return parseRequirements, maxManifestSize
	case (strings.HasPrefix(name, "tsconfig") || strings.HasPrefix(name, "jsconfig")) && ext == ".json":
		return parseTSConfig, maxManifestSize
	case name == "deno.json" || name == "deno.jsonc" || name == "import_map.json":
		return parseImportMap, maxManifestSize
	case strings.HasSuffix(name, ".min.js"):
		return nil, 0
	case jsSource[ext]:
		return parseJSImports, maxSourceSize
	case ext == ".py":
		return parsePyImports, maxSourceSize
	}
	return nil, 0
}

func parseNPM(data []byte) (parsed, error) {
	var pkg struct {
		Name                 string            `json:"name"`
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return parsed{}, err
	}
	var p parsed
	if pkg.Name != "" {
		p.local = append(p.local, depKey{NPM, pkg.Name})
	}
	lines := strings.Split(string(data), "\n")
	for _, group := range []struct {
		field string
		deps  map[string]string
	}{
		{"dependencies", pkg.Dependencies},
		{"devDependencies", pkg.DevDependencies},
		{"optionalDependencies", pkg.OptionalDependencies},
		{"peerDependencies", pkg.PeerDependencies},
	} {
		// Search keys from the group's own line, so "jest" in scripts does
		// not take the line of the "jest" dependency.
		from := max(lineOf(lines, 0, `"`+group.field+`"`)-1, 0)
		for key, spec := range group.deps {
			// Code imports the key, whatever the spec installs.
			p.declared = append(p.declared, depKey{NPM, key})
			if name, ok := npmRegistryName(key, spec); ok {
				p.deps = append(p.deps, Dep{Ecosystem: NPM, Name: name, Line: lineOf(lines, from, `"`+key+`"`)})
			}
		}
	}
	return p, nil
}

// npmRegistryName returns the registry package that key/spec installs, or
// false when spec points at git, a path, a workspace or a URL.
func npmRegistryName(key, spec string) (string, bool) {
	if alias, ok := strings.CutPrefix(spec, "npm:"); ok {
		// "npm:real-name@^1" installs real-name under the key's name.
		if i := strings.LastIndex(alias, "@"); i > 0 {
			return alias[:i], true
		}
		return alias, true
	}
	return key, !strings.ContainsAny(spec, ":/")
}

var (
	// PEP 508 names start and end with a letter or digit.
	reqName   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?`)
	eggName   = regexp.MustCompile(`#egg=([A-Za-z0-9][\w.-]*)`)
	pypiSeps  = regexp.MustCompile(`[-_.]+`)
	localFile = []string{".whl", ".tar.gz", ".zip"}
)

func parseRequirements(data []byte) (parsed, error) {
	var p parsed
	for i, line := range strings.Split(string(data), "\n") {
		// "-e git+https://...#egg=name" installs name from git: declared,
		// never looked up.
		if m := eggName.FindStringSubmatch(line); m != nil {
			p.declared = append(p.declared, depKey{PyPI, normalizePyPI(m[1])})
		}
		line, _, _ = strings.Cut(line, "#")
		line = strings.TrimSpace(line)
		// Options (-r, -e, --index-url) and local archives are not
		// requirements; "name @ url" direct references are.
		if strings.HasPrefix(line, "-") || (hasAnySuffix(line, localFile) && !strings.Contains(line, "@")) {
			continue
		}
		p.addRequirement(line, i+1, nil)
	}
	return p, nil
}

// addRequirement records a PEP 508 requirement as declared and, when it
// installs from the registry and is not pinned elsewhere, as a lookup.
func (p *parsed) addRequirement(spec string, line int, pinned map[string]bool) {
	name, registry := pep508Name(spec)
	if name == "" || (registry && strings.Contains(spec, "://")) { // a bare URL
		return
	}
	p.declared = append(p.declared, depKey{PyPI, name})
	if registry && !pinned[name] {
		p.deps = append(p.deps, Dep{Ecosystem: PyPI, Name: name, Line: line})
	}
}

// pep508Name returns the normalized name in a PEP 508 requirement and
// whether it installs from the registry; a direct reference
// ("name @ url") does not.
func pep508Name(spec string) (name string, registry bool) {
	spec = strings.TrimSpace(spec)
	raw := reqName.FindString(spec)
	if raw == "" {
		return "", false
	}
	rest := strings.TrimSpace(spec[len(raw):])
	if strings.HasPrefix(rest, "[") { // extras
		_, rest, _ = strings.Cut(rest, "]")
		rest = strings.TrimSpace(rest)
	}
	return normalizePyPI(raw), !strings.HasPrefix(rest, "@")
}

// normalizePyPI applies PEP 503: case and runs of -_. do not matter.
func normalizePyPI(name string) string {
	return pypiSeps.ReplaceAllString(strings.ToLower(name), "-")
}

func parseGoMod(data []byte) (parsed, error) {
	// Parse, not ParseLax: Lax drops replace directives, which we need.
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return parsed{}, err
	}
	var p parsed
	if f.Module != nil {
		p.local = append(p.local, depKey{Go, f.Module.Mod.Path})
	}
	// A replaced module is never fetched under its own path.
	replaced := map[string]bool{}
	for _, r := range f.Replace {
		replaced[r.Old.Path] = true
	}
	for _, r := range f.Require {
		if !replaced[r.Mod.Path] {
			p.deps = append(p.deps, Dep{Ecosystem: Go, Name: r.Mod.Path, Line: r.Syntax.Start.Line})
		}
	}
	return p, nil
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

func hasAnySuffix(s string, suffixes []string) bool {
	return slices.ContainsFunc(suffixes, func(suf string) bool { return strings.HasSuffix(s, suf) })
}

func hasAnyPrefix(s string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(pre string) bool { return strings.HasPrefix(s, pre) })
}
