// Command fiscal checks what AI-generated code usually gets wrong:
// packages that do not exist, leaked secrets and vulnerable dependencies.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/Alexandre-arocha/fiscal/packages"
	"github.com/Alexandre-arocha/fiscal/report"
	"github.com/Alexandre-arocha/fiscal/scanner"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `fiscal — confere o que o código gerado por IA costuma errar.

Uso:
  fiscal scan [--format text|json] [--offline] [diretório]
  fiscal version

Códigos de saída: 0 sem achados, 1 com achados, 2 erro.
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "scan":
		return scan(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, "fiscal", version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "comando desconhecido: %q\n\n%s", args[0], usage)
		return 2
	}
}

func scan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "formato de saída: text ou json")
	offline := fs.Bool("offline", false, "não consulta a rede (pula o verificador de pacotes)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	var write func(io.Writer, []scanner.Finding) error
	switch *format {
	case "text":
		write = report.Text
	case "json":
		write = report.JSON
	default:
		fmt.Fprintf(stderr, "formato inválido: %q (use text ou json)\n", *format)
		return 2
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "informe no máximo um diretório")
		return 2
	}
	root := "."
	if fs.NArg() == 1 {
		root = fs.Arg(0)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "não é um diretório: %s\n", root)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *offline {
		fmt.Fprintln(stderr, "modo offline: verificador de pacotes desativado")
	}
	findings, scanErr := scanner.Run(ctx, root, checkers(*offline))
	if err := write(stdout, findings); err != nil {
		fmt.Fprintln(stderr, "erro ao escrever relatório:", err)
		return 2
	}
	if scanErr != nil {
		fmt.Fprintln(stderr, "erro:", scanErr)
		return 2
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

// checkers lists the enabled checks; offline mode drops those that query
// the network. Secrets and vulnerabilities arrive in weeks 3–4 of the plan.
func checkers(offline bool) []scanner.Checker {
	if offline {
		return nil
	}
	return []scanner.Checker{packages.New()}
}
