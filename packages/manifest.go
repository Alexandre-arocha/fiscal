// Package packages checks that every declared dependency exists in its public
// registry and is not suspiciously new, catching names an AI made up
// (slopsquatting).
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

// Dep is one dependency declaration found in a manifest.
type Dep struct {
	Ecosystem Ecosystem
	Name      string
	File      string // relative to the scanned root, slash-separated
	Line      int
}

type depKey struct {
	eco  Ecosystem
	name string
}

// parsed is what one file contributes to a scan.
type parsed struct {
	deps    []Dep
	local   []depKey // packages the repository defines itself
	private []string // npm scopes served by a private registry
}

const maxManifestSize = 5 << 20

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true, "venv": true,
}

// Collect walks root and parses the manifests it knows. Symlinks and
// oversized files are skipped so a hostile repository cannot point the scan
// outside root or exhaust memory. Unreadable manifests become low-severity
// findings instead of aborting the scan.
//
// Packages the repository defines itself (workspaces, nested modules) and
// packages served by a private registry (.npmrc and .yarnrc.yml scopes,
// GOPRIVATE) are dropped: looking them up publicly would raise false alarms
// and leak private names.
func Collect(root string) ([]Dep, []scanner.Finding, error) {
	var (
		deps    []Dep
		bad     []scanner.Finding
		local   = map[depKey]bool{}
		private = map[string]bool{}
	)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		parse := parserFor(d.Name())
		if parse == nil || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxManifestSize {
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
		for _, k := range p.local {
			local[k] = true
		}
		for _, scope := range p.private {
			private[strings.ToLower(scope)] = true
		}
		return nil
	})
	goPatterns := goNoProxy()
	deps = slices.DeleteFunc(deps, func(d Dep) bool {
		return local[depKey{d.Ecosystem, d.Name}] || isPrivate(d, private, goPatterns)
	})
	return deps, bad, err
}

func parserFor(name string) func([]byte) (parsed, error) {
	switch {
	case name == "package.json":
		return parseNPM
	case name == ".npmrc":
		return parseNpmrc
	case name == ".yarnrc.yml":
		return parseYarnrc
	case name == "go.mod":
		return parseGoMod
	case name == "pyproject.toml":
		return parsePyproject
	case strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt"):
		return parseRequirements
	}
	return nil
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
	pypiSeps  = regexp.MustCompile(`[-_.]+`)
	localFile = []string{".whl", ".tar.gz", ".zip"}
)

func parseRequirements(data []byte) (parsed, error) {
	var p parsed
	for i, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "#")
		line = strings.TrimSpace(line)
		// Options (-r, -e, --index-url), URLs and local archives are not
		// registry names.
		if strings.HasPrefix(line, "-") || strings.Contains(line, "://") || hasAnySuffix(line, localFile) {
			continue
		}
		if name, ok := pep508Name(line); ok {
			p.deps = append(p.deps, Dep{Ecosystem: PyPI, Name: name, Line: i + 1})
		}
	}
	return p, nil
}

// pep508Name returns the normalized name in a PEP 508 requirement, or false
// when there is none or it is a direct reference ("name @ url"), which does
// not come from the registry.
func pep508Name(spec string) (string, bool) {
	spec = strings.TrimSpace(spec)
	name := reqName.FindString(spec)
	if name == "" {
		return "", false
	}
	rest := strings.TrimSpace(spec[len(name):])
	if strings.HasPrefix(rest, "[") { // extras
		_, rest, _ = strings.Cut(rest, "]")
		rest = strings.TrimSpace(rest)
	}
	if strings.HasPrefix(rest, "@") {
		return "", false
	}
	return normalizePyPI(name), true
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
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}
