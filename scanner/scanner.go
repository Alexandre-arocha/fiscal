// Package scanner runs independent checkers over a directory in parallel
// and merges their findings, most severe first.
package scanner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
)

// Severity ranks a finding; higher is worse.
type Severity int

const (
	Info Severity = iota
	Low
	Medium
	High
	Critical
)

var severityNames = [...]string{"info", "low", "medium", "high", "critical"}

func (s Severity) String() string {
	if s < Info || s > Critical {
		return fmt.Sprintf("severity(%d)", int(s))
	}
	return severityNames[s]
}

// MarshalText makes severities readable in JSON output.
func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Finding is one problem reported by a checker.
type Finding struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Title    string   `json:"title"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Fix      string   `json:"fix,omitempty"`
}

// Checker inspects the tree at root. It must never execute code found there.
type Checker interface {
	Name() string
	Check(ctx context.Context, root string) ([]Finding, error)
}

// Run executes every checker concurrently. A failing checker does not hide
// the findings of the others: partial results come back with the joined error.
func Run(ctx context.Context, root string, checkers []Checker) ([]Finding, error) {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		all  = []Finding{}
		errs []error
	)
	for _, c := range checkers {
		wg.Go(func() {
			found, err := c.Check(ctx, root)
			mu.Lock()
			defer mu.Unlock()
			for i := range found {
				found[i].Check = c.Name()
			}
			all = append(all, found...)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", c.Name(), err))
			}
		})
	}
	wg.Wait()
	slices.SortStableFunc(all, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(b.Severity, a.Severity),
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
		)
	})
	return all, errors.Join(errs...)
}
