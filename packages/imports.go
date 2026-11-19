package packages

import (
	"regexp"
	"slices"
	"strings"
)

var (
	// import/export ... from "x" and import "x". The character before the
	// keyword rules out CSS @import and method calls such as a.import().
	jsStatic = regexp.MustCompile(`(?:^|[^\w$.@])(?:import|export)\s*(?:[\w$*{}\s,]+?\s*from\s*)?["']([^"'\s]+)["']`)
	// require("x") and import("x").
	jsDynamic = regexp.MustCompile(`(?:^|[^\w$.@])(?:require|import)\s*\(\s*["']([^"'\s]+)["']\s*\)`)
	npmName   = regexp.MustCompile(`^(?:@[A-Za-z0-9][\w.~-]*/)?[A-Za-z0-9][\w.~-]*$`)

	pyImport = regexp.MustCompile(`^\s*import\s+(.+)`)
	pyFrom   = regexp.MustCompile(`^\s*from\s+([A-Za-z_]\w*)[\w.]*\s+import\b`)
	pyIdent  = regexp.MustCompile(`^[A-Za-z_]\w*$`)
)

// parseJSImports lists the module specifiers of static imports, re-exports,
// require() and import() calls. It reads text, so an import quoted in a
// comment counts too; that only adds a lookup.
func parseJSImports(data []byte) (parsed, error) {
	var p parsed
	text := string(data)
	lines := newLineIndex(text)
	for _, re := range []*regexp.Regexp{jsStatic, jsDynamic} {
		for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
			p.imports = append(p.imports, Dep{Ecosystem: NPM, Import: text[m[2]:m[3]], Line: lines.line(m[2])})
		}
	}
	return p, nil
}

// parsePyImports lists the top-level modules of import and from-import
// statements, skipping relative imports and triple-quoted strings.
func parsePyImports(data []byte) (parsed, error) {
	var p parsed
	inString := false
	for i, line := range strings.Split(string(data), "\n") {
		if quotes := strings.Count(line, `"""`) + strings.Count(line, `'''`); inString || quotes > 0 {
			if quotes%2 == 1 {
				inString = !inString
			}
			continue
		}
		line, _, _ = strings.Cut(line, "#")
		for _, stmt := range strings.Split(line, ";") {
			for _, mod := range pyModules(stmt) {
				p.imports = append(p.imports, Dep{Ecosystem: PyPI, Import: mod, Line: i + 1})
			}
		}
	}
	return p, nil
}

// pyModules returns the top-level modules one Python statement imports.
func pyModules(stmt string) []string {
	if m := pyFrom.FindStringSubmatch(stmt); m != nil {
		return []string{m[1]}
	}
	m := pyImport.FindStringSubmatch(stmt)
	if m == nil {
		return nil
	}
	var mods []string
	for _, part := range strings.Split(m[1], ",") { // import a.b as c, d
		if fields := strings.Fields(part); len(fields) > 0 {
			if top, _, _ := strings.Cut(fields[0], "."); pyIdent.MatchString(top) {
				mods = append(mods, top)
			}
		}
	}
	return mods
}

// npmPackageOf returns the package a bare import specifier loads
// ("@scope/pkg/sub" gives "@scope/pkg", "pkg/sub" gives "pkg"), or false for
// relative paths, URLs, protocol imports such as "node:fs" and alias
// prefixes such as "@/", "~/", "#" and "$".
func npmPackageOf(spec string) (string, bool) {
	parts := strings.SplitN(spec, "/", 3)
	name := parts[0]
	if strings.HasPrefix(name, "@") && len(parts) > 1 {
		name += "/" + parts[1]
	}
	return name, !strings.Contains(spec, ":") && npmName.MatchString(name)
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

// npmImport returns the npm package to look up for a JS/TS import, or ""
// when it is relative, built in, aliased, declared, private or local.
func (r *repo) npmImport(spec string) string {
	name, ok := npmPackageOf(spec)
	if !ok || aliased(spec, r.aliases) {
		return ""
	}
	// "components/Button" or "@components/Card" usually point at a project
	// folder through a bundler alias or baseUrl.
	first, _, _ := strings.Cut(strings.TrimPrefix(name, "@"), "/")
	if nodeBuiltins[name] || r.declared[depKey{NPM, name}] || r.local[depKey{NPM, name}] ||
		r.names[strings.ToLower(first)] || isPrivate(Dep{Ecosystem: NPM, Name: name}, r.private, "") {
		return ""
	}
	return name
}

// pyImport returns the PyPI name to look up for a Python import, or "" when
// the module is standard library, provided by the host program, local,
// known to come from a differently named distribution, or provided by a
// declared one.
func (r *repo) pyImport(mod string, provided map[string]bool) string {
	lower := strings.ToLower(mod)
	if strings.HasPrefix(mod, "_") || pyStdlib[mod] || pyHost[lower] || hasAnyPrefix(lower, pyHostPrefixes) ||
		r.names[lower] || pyImportDist[lower] != "" || provided[lower] {
		return ""
	}
	name := normalizePyPI(mod)
	if r.declared[depKey{PyPI, name}] || r.local[depKey{PyPI, name}] {
		return ""
	}
	return name
}

// pyProvided lists the import names the declared PyPI distributions likely
// provide, from common naming patterns: python-dateutil gives dateutil,
// PyYAML gives yaml, google-cloud-storage gives google.
func (r *repo) pyProvided() map[string]bool {
	provided := map[string]bool{}
	for k := range r.declared {
		if k.eco != PyPI {
			continue
		}
		d := k.name // normalized: lower case, "-" separated
		variants := []string{
			d,
			strings.TrimPrefix(d, "python-"),
			strings.TrimPrefix(d, "py"),
			strings.TrimSuffix(d, "-python"),
			strings.TrimSuffix(d, "-py"),
			strings.TrimSuffix(d, "-binary"),
		}
		if first, _, ok := strings.Cut(d, "-"); ok {
			variants = append(variants, first, d[strings.LastIndex(d, "-")+1:])
		}
		for _, v := range variants {
			provided[strings.ReplaceAll(v, "-", "_")] = true
		}
	}
	return provided
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
