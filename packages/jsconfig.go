package packages

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// parseTSConfig collects compilerOptions.paths keys from tsconfig.json and
// jsconfig.json: imports matching them resolve to project files, not npm.
// A config that does not parse is ignored; it only prevents false alarms.
func parseTSConfig(data []byte) (parsed, error) {
	var cfg struct {
		CompilerOptions struct {
			Paths map[string]json.RawMessage `json:"paths"`
		} `json:"compilerOptions"`
	}
	if json.Unmarshal(stripJSONC(data), &cfg) != nil {
		return parsed{}, nil
	}
	return parsed{aliases: slices.Collect(maps.Keys(cfg.CompilerOptions.Paths))}, nil
}

// parseImportMap collects the keys of a Deno config or import map: they map
// bare specifiers to URLs or JSR, not to npm.
func parseImportMap(data []byte) (parsed, error) {
	var cfg struct {
		Imports map[string]json.RawMessage `json:"imports"`
	}
	if json.Unmarshal(stripJSONC(data), &cfg) != nil {
		return parsed{}, nil
	}
	return parsed{aliases: slices.Collect(maps.Keys(cfg.Imports))}, nil
}

// aliased reports whether spec is resolved by a tsconfig path or import map
// key: "@app/*" and "lib/" match by prefix, other keys exactly. The
// catch-all "*" is ignored because it usually falls back to node_modules.
func aliased(spec string, aliases []string) bool {
	for _, a := range aliases {
		prefix, wildcard := strings.CutSuffix(a, "*")
		switch {
		case wildcard && prefix != "" && strings.HasPrefix(spec, prefix):
			return true
		case strings.HasSuffix(a, "/") && strings.HasPrefix(spec, a):
			return true
		case spec == a:
			return true
		}
	}
	return false
}

// stripJSONC turns JSON with comments and trailing commas (tsconfig, Deno
// config) into plain JSON. String contents are copied untouched.
func stripJSONC(data []byte) []byte {
	var out []byte
	for i := 0; i < len(data); i++ {
		switch c := data[i]; {
		case c == '"':
			start := i
			for i++; i < len(data) && data[i] != '"'; i++ {
				if data[i] == '\\' {
					i++
				}
			}
			out = append(out, data[start:min(i+1, len(data))]...)
		case bytes.HasPrefix(data[i:], []byte("//")):
			for i < len(data) && data[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case bytes.HasPrefix(data[i:], []byte("/*")):
			end := bytes.Index(data[i+2:], []byte("*/"))
			if end < 0 {
				return out
			}
			i += end + 3
			out = append(out, ' ')
		case c == ',':
			if j := nextSignificant(data, i+1); j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue // trailing comma
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// nextSignificant returns the index of the next byte at or after i that is
// neither whitespace nor inside a comment.
func nextSignificant(data []byte, i int) int {
	for i < len(data) {
		switch {
		case data[i] == ' ' || data[i] == '\t' || data[i] == '\n' || data[i] == '\r':
			i++
		case bytes.HasPrefix(data[i:], []byte("//")):
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case bytes.HasPrefix(data[i:], []byte("/*")):
			end := bytes.Index(data[i+2:], []byte("*/"))
			if end < 0 {
				return len(data)
			}
			i += end + 4
		default:
			return i
		}
	}
	return i
}
