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
	setupList   = regexp.MustCompile(`\b(?:install_requires|setup_requires|tests_require)\s*=\s*\[`)
	setupExtras = regexp.MustCompile(`\bextras_require\s*=\s*\{`)
	// One string literal of a list, after any blanks and comments.
	setupItem = regexp.MustCompile(`^(?:\s|#[^\n]*)*["']([^"'\n]*)["']\s*,?`)
)

// parseSetupPy reads the literal lists passed as install_requires,
// setup_requires and tests_require, and the lists inside a literal
// extras_require dict. setup.py is code and is never run, so requirements
// built at runtime are not seen.
func parseSetupPy(data []byte) (parsed, error) {
	var p parsed
	text := string(data)
	lines := newLineIndex(text)
	for _, m := range setupList.FindAllStringIndex(text, -1) {
		p.addSetupList(text, m[1], lines)
	}
	for _, m := range setupExtras.FindAllStringIndex(text, -1) {
		dict := text[:closingBrace(text, m[1])]
		for pos := m[1]; ; {
			open := strings.IndexByte(dict[pos:], '[')
			if open < 0 {
				break
			}
			pos += open + 1
			p.addSetupList(dict, pos, lines)
		}
	}
	return p, nil
}

// addSetupList records the string literals of a Python list that starts at
// pos (just after "["), up to the first item that is not a literal.
func (p *parsed) addSetupList(text string, pos int, lines lineIndex) {
	for {
		item := setupItem.FindStringSubmatchIndex(text[pos:])
		if item == nil {
			return
		}
		p.addRequirement(text[pos+item[2]:pos+item[3]], lines.line(pos+item[2]), nil)
		pos += item[1]
	}
}

// closingBrace returns the index of the "}" closing a dict whose body starts
// at start, skipping string literals; len(text) if it is never closed.
func closingBrace(text string, start int) int {
	depth := 1
	for i := start; i < len(text); i++ {
		switch c := text[i]; c {
		case '"', '\'':
			if j := strings.IndexByte(text[i+1:], c); j >= 0 {
				i += j + 1
			}
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(text)
}
