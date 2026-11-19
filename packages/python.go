package packages

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// PEP 508 names start and end with a letter or digit.
	reqName   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?`)
	eggName   = regexp.MustCompile(`#egg=([A-Za-z0-9][\w.-]*)`)
	pypiSeps  = regexp.MustCompile(`[-_.]+`)
	localFile = []string{".whl", ".tar.gz", ".zip"}

	pyImport = regexp.MustCompile(`^\s*import\s+(.+)`)
	pyFrom   = regexp.MustCompile(`^\s*from\s+([A-Za-z_]\w*)[\w.]*\s+import\b`)
	pyIdent  = regexp.MustCompile(`^[A-Za-z_]\w*$`)
	// "%pip install x" and "!pip install x" in a notebook cell.
	pipMagic = regexp.MustCompile(`^\s*[!%]\s*pip3?\s+install\s+(.+)`)
)

// pipValueFlags are pip install options whose value is not a package.
var pipValueFlags = setOf(`-r --requirement -c --constraint -e --editable -i --index-url --extra-index-url
-f --find-links -t --target --prefix --root --src`)

// isRequirementsFile matches pip requirement files: requirements.txt,
// requirements-dev.txt, dev-requirements.txt, requirements.in, and any .txt
// or .in inside a requirements/ folder.
func isRequirementsFile(name, parent string) bool {
	ext := filepath.Ext(name)
	if ext != ".txt" && ext != ".in" {
		return false
	}
	stem := strings.ToLower(strings.TrimSuffix(name, ext))
	return strings.HasPrefix(stem, "requirements") || strings.HasSuffix(stem, "requirements") ||
		strings.EqualFold(parent, "requirements")
}

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

// parseNotebook reads the code cells of a Jupyter notebook: their imports,
// and the packages they install with %pip or !pip, which count as declared
// dependencies. Lines point at the notebook file where the code appears.
func parseNotebook(data []byte) (parsed, error) {
	var nb struct {
		Cells []struct {
			CellType string          `json:"cell_type"`
			Source   json.RawMessage `json:"source"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(data, &nb); err != nil {
		return parsed{}, err
	}
	var code []string
	for _, cell := range nb.Cells {
		if cell.CellType != "code" {
			continue
		}
		var src []string // usually a list of lines, sometimes one string
		if json.Unmarshal(cell.Source, &src) != nil {
			var s string
			if json.Unmarshal(cell.Source, &s) != nil {
				continue
			}
			src = []string{s}
		}
		code = append(code, strings.Split(strings.Join(src, ""), "\n")...)
	}

	raw := strings.Split(string(data), "\n")
	fileLine := func(codeLine string) int {
		return lineOf(raw, 0, jsonEscape(strings.TrimSpace(codeLine)))
	}
	p, _ := parsePyImports([]byte(strings.Join(code, "\n")))
	for i := range p.imports {
		p.imports[i].Line = fileLine(code[p.imports[i].Line-1])
	}
	for _, line := range code {
		if m := pipMagic.FindStringSubmatch(line); m != nil {
			for _, spec := range pipInstallArgs(m[1]) {
				p.addRequirement(spec, fileLine(line), nil)
			}
		}
	}
	return p, nil
}

// jsonEscape returns s as it appears inside a JSON string, without quotes.
func jsonEscape(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.Trim(strings.TrimSpace(b.String()), `"`)
}

// pipInstallArgs returns the requirements of a "pip install" argument list,
// up to the first shell operator or comment.
func pipInstallArgs(args string) []string {
	var specs []string
	fields := strings.Fields(args)
	for i := 0; i < len(fields); i++ {
		f := strings.Trim(fields[i], `"'`)
		switch {
		case f == "&&" || f == "||" || f == ";" || f == "|" || strings.HasPrefix(f, ">") || strings.HasPrefix(f, "#"):
			return specs
		case pipValueFlags[f]:
			i++ // skip the option's value
		case strings.HasPrefix(f, "-"):
		default:
			specs = append(specs, f)
		}
	}
	return specs
}

// parseCondaEnv reads a conda environment.yml. Conda packages come from
// conda channels, not PyPI: they count as declared (code may import them)
// but are not looked up. Items under "- pip:" are PyPI requirements.
func parseCondaEnv(data []byte) (parsed, error) {
	var p parsed
	inDeps, pipIndent := false, -1
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			inDeps, pipIndent = strings.HasPrefix(trimmed, "dependencies:"), -1
			continue
		}
		item, isItem := strings.CutPrefix(trimmed, "- ")
		if !inDeps || !isItem {
			continue
		}
		if pipIndent >= 0 && indent <= pipIndent {
			pipIndent = -1 // left the pip block
		}
		item, _, _ = strings.Cut(item, " #")
		item = strings.Trim(strings.TrimSpace(item), `"'`)
		switch {
		case pipIndent >= 0:
			p.addRequirement(item, i+1, nil)
		case item == "pip:":
			pipIndent = indent
		default:
			// "conda-forge::numpy=1.26" declares numpy.
			if _, after, found := strings.Cut(item, "::"); found {
				item = after
			}
			if name := reqName.FindString(item); name != "" {
				p.declared = append(p.declared, depKey{PyPI, normalizePyPI(name)})
			}
		}
	}
	return p, nil
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
