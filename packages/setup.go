package packages

import (
	"regexp"
	"strings"
)

var setupKeys = map[string]bool{"install_requires": true, "setup_requires": true, "tests_require": true}

// parseSetupCfg reads [metadata] name as the project's own package, and as
// dependencies the install_requires, setup_requires and tests_require of
// [options] plus every list in [options.extras_require]. Values are
// dangling lists: one requirement per indented line.
func parseSetupCfg(data []byte) (parsed, error) {
	var (
		p          parsed
		section    string
		collecting bool
	)
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || trimmed[0] == '#' || trimmed[0] == ';':
			continue
		case trimmed[0] == '[':
			section, collecting = strings.Trim(trimmed, "[] "), false
			continue
		case line[0] != ' ' && line[0] != '\t': // a new key
			key, value, ok := strings.Cut(trimmed, "=")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if ok && section == "metadata" && key == "name" && value != "" {
				p.local = append(p.local, depKey{PyPI, normalizePyPI(value)})
			}
			collecting = ok && (section == "options.extras_require" || section == "options" && setupKeys[key])
			if !collecting {
				continue
			}
			trimmed = value
		case !collecting:
			continue
		}
		p.addRequirement(trimmed, i+1, nil)
	}
	return p, nil
}

var (
	setupList = regexp.MustCompile(`\b(?:install_requires|setup_requires|tests_require)\s*=\s*\[`)
	// One string literal of the list, after any blanks and comments.
	setupItem = regexp.MustCompile(`^(?:\s|#[^\n]*)*["']([^"'\n]*)["']\s*,?`)
)

// parseSetupPy reads the literal lists passed as install_requires,
// setup_requires and tests_require. setup.py is code and is never run, so
// lists built at runtime are not seen.
func parseSetupPy(data []byte) (parsed, error) {
	var p parsed
	text := string(data)
	lines := newLineIndex(text)
	for _, m := range setupList.FindAllStringIndex(text, -1) {
		for pos := m[1]; ; {
			item := setupItem.FindStringSubmatchIndex(text[pos:])
			if item == nil {
				break
			}
			p.addRequirement(text[pos+item[2]:pos+item[3]], lines.line(pos+item[2]), nil)
			pos += item[1]
		}
	}
	return p, nil
}
