package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

type fake struct {
	name     string
	findings []Finding
	err      error
}

func (f fake) Name() string { return f.name }

func (f fake) Check(context.Context, string) ([]Finding, error) { return f.findings, f.err }

func TestRunMergesAndSortsBySeverity(t *testing.T) {
	got, err := Run(context.Background(), ".", []Checker{
		fake{name: "a", findings: []Finding{{Severity: Low, Title: "low", File: "b.go", Line: 2}}},
		fake{name: "b", findings: []Finding{
			{Severity: Critical, Title: "crit"},
			{Severity: Low, Title: "low-first", File: "a.go", Line: 9},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, f := range got {
		titles = append(titles, f.Title)
	}
	if want := []string{"crit", "low-first", "low"}; !slices.Equal(titles, want) {
		t.Fatalf("order = %v, want %v", titles, want)
	}
	if got[0].Check != "b" {
		t.Errorf("Check = %q, want %q", got[0].Check, "b")
	}
}

func TestRunKeepsFindingsWhenACheckerFails(t *testing.T) {
	boom := errors.New("boom")
	got, err := Run(context.Background(), ".", []Checker{
		fake{name: "ok", findings: []Finding{{Severity: High, Title: "x"}}},
		fake{name: "bad", err: boom},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
}

func TestRunWithNoCheckersReturnsEmptySlice(t *testing.T) {
	got, err := Run(context.Background(), ".", nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Run(nil) = %v, %v; want empty non-nil slice, nil", got, err)
	}
}

func TestSeverityMarshalsAsName(t *testing.T) {
	b, err := json.Marshal(Finding{Severity: High})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"severity":"high"`) {
		t.Fatalf("json = %s", b)
	}
}
