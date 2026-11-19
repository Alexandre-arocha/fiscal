package packages

import (
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// pyproject holds the parts of pyproject.toml that name PyPI packages.
type pyproject struct {
	BuildSystem struct {
		Requires []string `toml:"requires"`
	} `toml:"build-system"`
	Project struct {
		Name                 string              `toml:"name"`
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
	} `toml:"project"`
	// PEP 735: entries are requirement strings or {include-group = "..."}.
	DependencyGroups map[string][]any `toml:"dependency-groups"`
	Tool             struct {
		Poetry struct {
			Name            string         `toml:"name"`
			Dependencies    map[string]any `toml:"dependencies"`
			DevDependencies map[string]any `toml:"dev-dependencies"`
			Group           map[string]struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"group"`
		} `toml:"poetry"`
		UV struct {
			DevDependencies []string       `toml:"dev-dependencies"`
			Sources         map[string]any `toml:"sources"`
		} `toml:"uv"`
		PDM struct {
			DevDependencies map[string][]string `toml:"dev-dependencies"`
		} `toml:"pdm"`
	} `toml:"tool"`
}

func parsePyproject(data []byte) (parsed, error) {
	var pp pyproject
	if _, err := toml.Decode(string(data), &pp); err != nil {
		return parsed{}, err
	}
	lines := strings.Split(string(data), "\n")
	var p parsed
	for _, name := range []string{pp.Project.Name, pp.Tool.Poetry.Name} {
		if name != "" {
			p.local = append(p.local, depKey{PyPI, normalizePyPI(name)})
		}
	}

	// [tool.uv.sources] pins a package to git, a path, a URL, a workspace
	// member or a chosen index: uv never takes it from PyPI.
	pinned := map[string]bool{}
	for name := range pp.Tool.UV.Sources {
		pinned[normalizePyPI(name)] = true
	}
	specs := slices.Concat(pp.BuildSystem.Requires, pp.Project.Dependencies)
	for _, extra := range slices.Sorted(maps.Keys(pp.Project.OptionalDependencies)) {
		specs = append(specs, pp.Project.OptionalDependencies[extra]...)
	}
	for _, group := range slices.Sorted(maps.Keys(pp.DependencyGroups)) {
		for _, entry := range pp.DependencyGroups[group] {
			if s, ok := entry.(string); ok {
				specs = append(specs, s)
			}
		}
	}
	specs = append(specs, pp.Tool.UV.DevDependencies...)
	for _, group := range slices.Sorted(maps.Keys(pp.Tool.PDM.DevDependencies)) {
		specs = append(specs, pp.Tool.PDM.DevDependencies[group]...)
	}
	for _, s := range specs {
		p.addRequirement(s, literalLine(lines, s), pinned)
	}

	addPoetry := func(header string, table map[string]any) {
		for _, key := range slices.Sorted(maps.Keys(table)) {
			if strings.EqualFold(key, "python") { // the interpreter, not a package
				continue
			}
			name := normalizePyPI(key)
			p.declared = append(p.declared, depKey{PyPI, name})
			if poetryFromRegistry(table[key]) {
				p.deps = append(p.deps, Dep{Ecosystem: PyPI, Name: name, Line: keyLine(lines, header, key)})
			}
		}
	}
	addPoetry("tool.poetry.dependencies", pp.Tool.Poetry.Dependencies)
	addPoetry("tool.poetry.dev-dependencies", pp.Tool.Poetry.DevDependencies)
	for _, group := range slices.Sorted(maps.Keys(pp.Tool.Poetry.Group)) {
		addPoetry("tool.poetry.group."+group+".dependencies", pp.Tool.Poetry.Group[group].Dependencies)
	}
	return p, nil
}

// poetryFromRegistry reports whether a Poetry dependency installs from PyPI:
// a version string, or a table without git, path or url and not pinned to
// another source. A list of constraints counts if any of them does.
func poetryFromRegistry(v any) bool {
	switch v := v.(type) {
	case string:
		return true
	case map[string]any:
		for _, k := range []string{"git", "path", "url"} {
			if _, ok := v[k]; ok {
				return false
			}
		}
		source, _ := v["source"].(string)
		return source == "" || strings.EqualFold(source, "pypi")
	case []map[string]any:
		return slices.ContainsFunc(v, func(m map[string]any) bool { return poetryFromRegistry(m) })
	case []any:
		return slices.ContainsFunc(v, poetryFromRegistry)
	}
	return false
}

type pipfile struct {
	Source []struct {
		Name string `toml:"name"`
		URL  string `toml:"url"`
	} `toml:"source"`
	Packages    map[string]any `toml:"packages"`
	DevPackages map[string]any `toml:"dev-packages"`
}

func parsePipfile(data []byte) (parsed, error) {
	var pf pipfile
	if _, err := toml.Decode(string(data), &pf); err != nil {
		return parsed{}, err
	}
	// Sources that are PyPI itself; an entry bound to any other index is
	// private.
	pypi := map[string]bool{}
	for _, s := range pf.Source {
		if u, err := url.Parse(s.URL); err == nil && (u.Hostname() == "pypi.org" || strings.HasSuffix(u.Hostname(), ".pypi.org")) {
			pypi[s.Name] = true
		}
	}
	lines := strings.Split(string(data), "\n")
	var p parsed
	for _, table := range []struct {
		header  string
		entries map[string]any
	}{{"packages", pf.Packages}, {"dev-packages", pf.DevPackages}} {
		for _, key := range slices.Sorted(maps.Keys(table.entries)) {
			name := normalizePyPI(key)
			p.declared = append(p.declared, depKey{PyPI, name})
			if pipfileFromRegistry(table.entries[key], pypi) {
				p.deps = append(p.deps, Dep{Ecosystem: PyPI, Name: name, Line: keyLine(lines, table.header, key)})
			}
		}
	}
	return p, nil
}

// pipfileFromRegistry reports whether a Pipfile entry installs from PyPI: a
// version string, or a table without git, path or file and with no index
// other than PyPI.
func pipfileFromRegistry(v any, pypi map[string]bool) bool {
	switch v := v.(type) {
	case string:
		return true
	case map[string]any:
		for _, k := range []string{"git", "path", "file"} {
			if _, ok := v[k]; ok {
				return false
			}
		}
		index, _ := v["index"].(string)
		return index == "" || pypi[index]
	}
	return false
}

// literalLine finds the TOML string literal that holds spec.
func literalLine(lines []string, spec string) int {
	if n := lineOf(lines, 0, `"`+spec+`"`); n > 0 {
		return n
	}
	return lineOf(lines, 0, `'`+spec+`'`)
}

// keyLine finds `key = ...` inside the [header] table, or anywhere in the
// file when the header is not written out (inline or dotted tables).
func keyLine(lines []string, header, key string) int {
	from := lineWhere(lines, 0, func(l string) bool {
		l, _, _ = strings.Cut(l, "#")
		return strings.ReplaceAll(strings.TrimSpace(l), " ", "") == "["+header+"]"
	})
	return lineWhere(lines, from, func(l string) bool {
		rest, ok := strings.CutPrefix(strings.TrimLeft(l, " \t\"'"), key)
		return ok && strings.HasPrefix(strings.TrimLeft(rest, " \t\"'"), "=")
	})
}
