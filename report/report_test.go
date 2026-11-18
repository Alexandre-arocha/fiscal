package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Alexandre-arocha/fiscal/scanner"
)

func TestTextShowsLocationAndFix(t *testing.T) {
	var buf bytes.Buffer
	err := Text(&buf, []scanner.Finding{{
		Check: "secrets", Severity: scanner.High, Title: "chave AWS",
		File: "config.go", Line: 12, Fix: "revogue a chave",
	}})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"[ALTO] secrets: chave AWS", "em config.go:12", "correção: revogue a chave", "1 problema(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestTextStripsControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, []scanner.Finding{{Title: "x", File: "evil\x1b[2J.go"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("escape sequence leaked: %q", buf.String())
	}
}

func TestJSONEmptyIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"findings": []`) {
		t.Fatalf("json = %s", buf.String())
	}
}
