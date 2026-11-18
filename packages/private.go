package packages

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/module"
)

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

// isPrivate reports whether d is served by a private registry: an npm scope
// mapped in the repository's config, or a Go module matching goPatterns.
func isPrivate(d Dep, npmScopes map[string]bool, goPatterns string) bool {
	switch d.Ecosystem {
	case NPM:
		scope, _, ok := strings.Cut(d.Name, "/")
		return ok && npmScopes[strings.ToLower(scope)]
	case Go:
		return goPatterns != "" && module.MatchPrefixPatterns(goPatterns, d.Name)
	}
	return false
}

// goNoProxy returns the module patterns the go command fetches without the
// public proxy: GONOPROXY, which defaults to GOPRIVATE.
func goNoProxy() string {
	if v := goEnv("GONOPROXY"); v != "" {
		return v
	}
	return goEnv("GOPRIVATE")
}

// goEnv reads a Go setting the way the go command does: the environment
// first, then the file written by `go env -w` (GOENV, or its default path).
func goEnv(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	path := os.Getenv("GOENV")
	if path == "off" {
		return ""
	}
	if path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(dir, "go", "env")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+"="); ok {
			return v
		}
	}
	return ""
}
