package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func call(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	if code, out, _ := call("version"); code != 0 || !strings.HasPrefix(out, "fiscal ") {
		t.Fatalf("version = %d %q", code, out)
	}
}

func TestScanCleanDirectory(t *testing.T) {
	code, out, _ := call("scan", t.TempDir())
	if code != 0 || !strings.Contains(out, "Nenhum problema") {
		t.Fatalf("scan = %d %q", code, out)
	}
}

func TestScanOffline(t *testing.T) {
	code, out, errOut := call("scan", "--offline", t.TempDir())
	if code != 0 || !strings.Contains(out, "Nenhum problema") || !strings.Contains(errOut, "offline") {
		t.Fatalf("scan --offline = %d %q %q", code, out, errOut)
	}
}

func TestScanJSON(t *testing.T) {
	code, out, _ := call("scan", "--format", "json", t.TempDir())
	var got struct{ Findings []any }
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got.Findings == nil {
		t.Fatalf("scan json = %d %q", code, out)
	}
}

func TestUsageErrors(t *testing.T) {
	dir := t.TempDir()
	for name, args := range map[string][]string{
		"no args":        nil,
		"unknown cmd":    {"nope"},
		"bad format":     {"scan", "--format", "xml", dir},
		"missing dir":    {"scan", filepath.Join(dir, "missing")},
		"too many paths": {"scan", dir, dir},
	} {
		if code, _, _ := call(args...); code != 2 {
			t.Errorf("%s: exit = %d, want 2", name, code)
		}
	}
}
