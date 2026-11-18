// Package report renders findings for people (text) and machines (JSON).
package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/Alexandre-arocha/fiscal/scanner"
)

var labels = map[scanner.Severity]string{
	scanner.Critical: "CRÍTICO",
	scanner.High:     "ALTO",
	scanner.Medium:   "MÉDIO",
	scanner.Low:      "BAIXO",
	scanner.Info:     "INFO",
}

// clean drops control characters: file names and titles come from the
// scanned repository and could carry terminal escape sequences.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// Text writes one block per finding, in the order given, in Portuguese.
func Text(w io.Writer, findings []scanner.Finding) error {
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "Nenhum problema encontrado.")
		return err
	}
	bw := bufio.NewWriter(w)
	for _, f := range findings {
		fmt.Fprintf(bw, "[%s] %s: %s\n", labels[f.Severity], clean(f.Check), clean(f.Title))
		if f.File != "" {
			loc := clean(f.File)
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", loc, f.Line)
			}
			fmt.Fprintf(bw, "    em %s\n", loc)
		}
		if f.Fix != "" {
			fmt.Fprintf(bw, "    correção: %s\n", clean(f.Fix))
		}
	}
	fmt.Fprintf(bw, "\n%d problema(s) encontrado(s).\n", len(findings))
	return bw.Flush()
}

// JSON writes {"findings": [...]}. This shape is the stable machine contract.
func JSON(w io.Writer, findings []scanner.Finding) error {
	if findings == nil {
		findings = []scanner.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Findings []scanner.Finding `json:"findings"`
	}{findings})
}
