# fiscal

Scanner de código aberto que confere o que o código gerado por IA costuma errar:

- **Pacotes inexistentes** (slopsquatting): dependências que não existem no registro, ou nomes recém-criados e suspeitos.
- **Segredos**: chaves, tokens e senhas nos arquivos e no histórico do Git.
- **Vulnerabilidades**: dependências do lockfile com CVE conhecida na base [OSV](https://osv.dev).

Não compete com revisores de PR por IA. É a camada que *prova* o que um revisor de diff não confere.

> **Status:** semana 1 de 10. O motor e o CLI existem, mas os verificadores ainda não estão ligados — `fiscal scan` hoje sempre responde "Nenhum problema encontrado". Veja o [roteiro](#roteiro).

## Instalação

Requer Go 1.27+.

```sh
go install github.com/Alexandre-arocha/fiscal/cmd/fiscal@latest
```

## Uso

```sh
fiscal scan                     # analisa o diretório atual
fiscal scan caminho/do/projeto
fiscal scan --format json .     # saída para máquinas
fiscal version
```

As flags vêm antes do diretório.

| Código de saída | Significado |
|---|---|
| 0 | nenhum achado |
| 1 | há achados |
| 2 | erro de uso ou de execução |

O `fiscal` **nunca executa** o código analisado: não roda `npm install`, `pip install` nem scripts do repositório.

## Arquitetura

Verificadores independentes rodam em paralelo e o motor junta os achados, do mais grave ao menos grave. Detalhes e plano da versão web em [docs/architecture.md](docs/architecture.md).

```
cmd/fiscal/   CLI
scanner/      motor: tipos Finding/Severity, interface Checker, execução paralela
report/       saída em texto (português) e JSON
```

## Roteiro

| Semana | Entrega |
|---|---|
| 1 | fundação: repositório, CI, motor, CLI |
| 2 | verificador de pacotes (npm, PyPI, Go) |
| 3 | verificador de segredos (arquivos + histórico) |
| 4 | verificador de vulnerabilidades (OSV), SARIF, primeira versão publicada |
| 5 | nota geral, mensagens, testes de integração |
| 6–8 | versão web, worker, login com GitHub e limites de uso |
| 9 | GitHub Action e hook de commit |
| 10 | lançamento |

**Fora do MVP:** revisão de lógica por IA, análise do site publicado, repositórios privados.

## Desenvolvimento

```sh
go vet ./...
go test ./...
```

## Segurança

Para reportar uma falha, veja [SECURITY.md](SECURITY.md). Não abra issue pública.

## Licença

[Apache 2.0](LICENSE).
