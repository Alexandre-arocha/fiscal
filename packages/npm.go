package packages

import (
	"encoding/json"
	"regexp"
	"strings"
)

var jsSource = setOf(".js .jsx .mjs .cjs .ts .tsx .mts .cts .vue .svelte .astro")

// nodeBuiltins are the Node.js core modules (module.builtinModules, top
// level) plus names provided by the runtime that runs the script (k6).
var nodeBuiltins = setOf(`
assert async_hooks buffer child_process cluster console constants crypto dgram diagnostics_channel dns
domain events fs http http2 https inspector module net os path perf_hooks process punycode querystring
readline repl stream string_decoder sys timers tls trace_events tty url util v8 vm wasi worker_threads zlib
k6
`)

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
	// pnpm catalogs keep the version range in pnpm-workspace.yaml; the
	// package still comes from the registry.
	if strings.HasPrefix(spec, "catalog:") {
		return key, true
	}
	return key, !strings.ContainsAny(spec, ":/")
}

var npmrcScope = regexp.MustCompile(`^\s*(@[^:\s]+):registry\s*=`)

// parseNpmrc collects scopes mapped to their own registry
// ("@acme:registry=https://npm.acme.example"). npm, pnpm and Yarn 1 never
// look those scopes up on the public registry. Other lines, including auth
// tokens, are ignored.
func parseNpmrc(data []byte) (parsed, error) {
	var p parsed
	for _, line := range strings.Split(string(data), "\n") {
		if m := npmrcScope.FindStringSubmatch(line); m != nil {
			p.private = append(p.private, m[1])
		}
	}
	return p, nil
}

// parseYarnrc collects the scopes under npmScopes in Yarn 2+ config. It reads
// only the block-style map Yarn itself writes, not general YAML.
func parseYarnrc(data []byte) (parsed, error) {
	var p parsed
	inScopes, child := false, -1
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			inScopes, child = strings.HasPrefix(trimmed, "npmScopes:"), -1
			continue
		}
		if !inScopes {
			continue
		}
		if child < 0 {
			child = indent
		}
		if name, _, ok := strings.Cut(trimmed, ":"); ok && indent == child {
			p.private = append(p.private, "@"+strings.TrimLeft(strings.Trim(name, `"'`), "@"))
		}
	}
	return p, nil
}

var (
	// import/export ... from "x" and import "x". The character before the
	// keyword rules out CSS @import and method calls such as a.import().
	jsStatic = regexp.MustCompile(`(?:^|[^\w$.@])(?:import|export)\s*(?:[\w$*{}\s,]+?\s*from\s*)?["']([^"'\s]+)["']`)
	// require("x") and import("x").
	jsDynamic = regexp.MustCompile(`(?:^|[^\w$.@])(?:require|import)\s*\(\s*["']([^"'\s]+)["']\s*\)`)
	npmName   = regexp.MustCompile(`^(?:@[A-Za-z0-9][\w.~-]*/)?[A-Za-z0-9][\w.~-]*$`)
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

// npmImport returns the npm package to look up for a JS/TS import, or ""
// when it is relative, built in, aliased, declared, private or local.
func (r *repo) npmImport(spec string) string {
	name, ok := npmPackageOf(spec)
	if !ok || aliased(spec, r.aliases) || nodeBuiltins[name] ||
		r.declared[depKey{NPM, name}] || r.local[depKey{NPM, name}] || r.isPrivate(Dep{Ecosystem: NPM, Name: name}) {
		return ""
	}
	// Bundler aliases and baseUrl let "utils", "components/Button" and
	// "@components/Card" point at project files; a scope that merely shares
	// a folder name (@supabase/x next to supabase/) still gets looked up.
	parts := strings.Split(strings.ToLower(strings.TrimPrefix(spec, "@")), "/")
	if len(parts) == 1 && r.names[parts[0]] || len(parts) > 1 && r.pairs[parts[0]+"/"+parts[1]] {
		return ""
	}
	return name
}
