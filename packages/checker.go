package packages

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Alexandre-arocha/fiscal/scanner"
)

const (
	defaultMinAge  = 30 * 24 * time.Hour
	defaultWorkers = 8
)

var registryName = map[Ecosystem]string{NPM: "npm", PyPI: "PyPI", Go: "proxy do Go"}

// Checker reports dependencies, declared or imported, that are missing from
// their registry or were created less than MinAge ago.
type Checker struct {
	Client  *Client
	MinAge  time.Duration
	Workers int // concurrent registry lookups; keeps us under rate limits
}

// New returns a checker for the public registries.
func New() *Checker {
	return &Checker{Client: NewClient(), MinAge: defaultMinAge, Workers: defaultWorkers}
}

func (*Checker) Name() string { return "packages" }

func (c *Checker) Check(ctx context.Context, root string) ([]scanner.Finding, error) {
	deps, findings, err := Collect(root)
	if err != nil {
		return findings, err
	}

	// Each package is looked up once, however many manifests declare it.
	var keys []depKey
	seen := map[depKey]bool{}
	for _, d := range deps {
		k := depKey{d.Ecosystem, d.Name}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	infos := make([]Info, len(keys))
	errs := make([]error, len(keys))
	sem := make(chan struct{}, max(c.Workers, 1))
	var wg sync.WaitGroup
	for i, k := range keys {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			infos[i], errs[i] = c.Client.Lookup(ctx, k.eco, k.name)
		})
	}
	wg.Wait()

	known := map[depKey]Info{}
	for i, k := range keys {
		if errs[i] == nil {
			known[k] = infos[i]
		}
	}
	now := c.Client.Now()
	for _, d := range deps {
		info, ok := known[depKey{d.Ecosystem, d.Name}]
		if !ok {
			continue // lookup failed; reported through the error
		}
		if f, suspicious := judge(d, info, now, c.MinAge); suspicious {
			findings = append(findings, f)
		}
	}
	return findings, errors.Join(errs...)
}

func judge(d Dep, info Info, now time.Time, minAge time.Duration) (scanner.Finding, bool) {
	reg := registryName[d.Ecosystem]
	f := scanner.Finding{File: d.File, Line: d.Line}
	subject := fmt.Sprintf("pacote %q", d.Name)
	if d.Import != "" {
		subject = fmt.Sprintf("o código importa %q", d.Import)
		if d.Import != d.Name {
			subject += fmt.Sprintf(" (pacote %q)", d.Name)
		}
	}
	switch age := now.Sub(info.Created); {
	case !info.Exists && d.Import != "":
		f.Severity = scanner.High
		f.Title = fmt.Sprintf("%s, que não existe no %s", subject, reg)
		f.Fix = "Não instale esse nome sem confirmar de onde ele vem: a IA pode ter inventado o pacote, e qualquer pessoa pode registrá-lo com código malicioso (slopsquatting)."
		if d.Ecosystem == PyPI {
			// The PyPI name behind a Python module is a guess.
			f.Severity = scanner.Medium
			f.Fix += " Se o módulo vem de um pacote com outro nome, declare esse pacote nas dependências."
		}
	case !info.Exists:
		f.Severity = scanner.High
		f.Title = fmt.Sprintf("%s não existe no %s", subject, reg)
		f.Fix = "Confira o nome. A IA pode ter inventado este pacote, e qualquer pessoa pode registrá-lo com código malicioso (slopsquatting). " + privateHint(d)
	case !info.Created.IsZero() && age < minAge:
		days := int(age.Hours() / 24)
		f.Severity = scanner.Medium
		f.Title = fmt.Sprintf("%s foi criado há %d dia(s) no %s", subject, days, reg)
		if d.Import != "" {
			f.Title = fmt.Sprintf("%s, criado há %d dia(s) no %s", subject, days, reg)
		}
		f.Fix = "Pacotes muito novos são comuns em ataques. Confira autor, repositório e downloads antes de usar."
	default:
		return f, false
	}
	return f, true
}

// privateHint says how to keep a genuinely private package from being
// replaced by a public one with the same name (dependency confusion).
func privateHint(d Dep) string {
	switch {
	case d.Ecosystem == Go:
		return "Se for um módulo privado, inclua-o em GOPRIVATE."
	case d.Ecosystem == NPM && strings.HasPrefix(d.Name, "@"):
		return "Se for um pacote privado, mapeie o escopo no .npmrc do projeto e garanta que o escopo é seu no npm público."
	case d.Ecosystem == NPM:
		return "Se for um pacote privado, publique-o num escopo seu: nomes sem escopo podem ser registrados por qualquer pessoa."
	default:
		return "Se for um pacote privado, um pacote público com o mesmo nome pode ser instalado no lugar dele: reserve o nome no PyPI ou use um índice exclusivo."
	}
}
