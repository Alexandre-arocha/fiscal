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

const maxManifestSize = 5 << 20

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true, "venv": true,
}

// Collect walks root and parses the manifests it knows. Symlinks and
// oversized files are skipped so a hostile repository cannot point the scan
// outside root or exhaust memory. Unreadable manifests become low-severity
// findings instead of aborting the scan.
func Collect(root string) ([]Dep, []scanner.Finding, error) {
	var (
		deps []Dep
		bad  []scanner.Finding
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
		data, err := os.ReadFile(path)
		if err == nil {
			var found []Dep
			if found, err = parse(data); err == nil {
				for i := range found {
					found[i].File = rel
				}
				deps = append(deps, found...)
				return nil
			}
		}
		bad = append(bad, scanner.Finding{
			Severity: scanner.Low,
			Title:    "não foi possível ler o manifesto: " + err.Error(),
			File:     rel,
		})
		return nil
	})
	return deps, bad, err
}

func parserFor(name string) func([]byte) ([]Dep, error) {
	switch {
	case name == "package.json":
		return parseNPM
	case name == "go.mod":
		return parseGoMod
	case strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt"):
		return parseRequirements
	}
	return nil
}

func parseNPM(data []byte) ([]Dep, error) {
	var pkg struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var deps []Dep
	for _, group := range []map[string]string{
		pkg.Dependencies, pkg.DevDependencies, pkg.OptionalDependencies, pkg.PeerDependencies,
	} {
		for key, spec := range group {
			name := key
			if alias, ok := strings.CutPrefix(spec, "npm:"); ok {
				// "npm:real-name@^1" installs real-name under the key's name.
				name = alias
				if i := strings.LastIndex(alias, "@"); i > 0 {
					name = alias[:i]
				}
			} else if strings.ContainsAny(spec, ":/") {
				// git, file, link, workspace, URL or owner/repo: not the registry.
				continue
			}
			deps = append(deps, Dep{Ecosystem: NPM, Name: name, Line: lineOf(lines, `"`+key+`"`)})
		}
	}
	return deps, nil
}

var (
	reqName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)
	pypiSeps  = regexp.MustCompile(`[-_.]+`)
	localFile = []string{".whl", ".tar.gz", ".zip"}
)

func parseRequirements(data []byte) ([]Dep, error) {
	var deps []Dep
	for i, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "#")
		line = strings.TrimSpace(line)
		// Options (-r, -e, --index-url), URLs, direct references and local
		// archives are not registry names.
		if line == "" || strings.HasPrefix(line, "-") || strings.Contains(line, "://") ||
			strings.Contains(line, " @ ") || hasAnySuffix(line, localFile) {
			continue
		}
		if name := reqName.FindString(line); name != "" {
			deps = append(deps, Dep{Ecosystem: PyPI, Name: normalizePyPI(name), Line: i + 1})
		}
	}
	return deps, nil
}

// normalizePyPI applies PEP 503: case and runs of -_. do not matter.
func normalizePyPI(name string) string {
	return pypiSeps.ReplaceAllString(strings.ToLower(name), "-")
}

func parseGoMod(data []byte) ([]Dep, error) {
	// Parse, not ParseLax: Lax drops replace directives, which we need.
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}
	// A replaced module is never fetched under its own path.
	replaced := map[string]bool{}
	for _, r := range f.Replace {
		replaced[r.Old.Path] = true
	}
	var deps []Dep
	for _, r := range f.Require {
		if !replaced[r.Mod.Path] {
			deps = append(deps, Dep{Ecosystem: Go, Name: r.Mod.Path, Line: r.Syntax.Start.Line})
		}
	}
	return deps, nil
}

func lineOf(lines []string, needle string) int {
	for i, l := range lines {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	return 0
}

func hasAnySuffix(s string, suffixes []string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}
