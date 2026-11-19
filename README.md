# fiscal

Scanner de código aberto que confere o que o código gerado por IA costuma errar:

- **Pacotes inexistentes** (slopsquatting): dependências que não existem no registro, ou nomes recém-criados e suspeitos.
- **Segredos**: chaves, tokens e senhas nos arquivos e no histórico do Git.
- **Vulnerabilidades**: dependências do lockfile com CVE conhecida na base [OSV](https://osv.dev).

Não compete com revisores de PR por IA. É a camada que *prova* o que um revisor de diff não confere.

> **Status:** semana 2 de 10. O verificador de pacotes funciona; segredos e vulnerabilidades chegam nas semanas 3 e 4. Veja o [roteiro](#roteiro).

## O que já é verificado

**Pacotes** (`packages`) — lê os manifestos e os imports do código (ignora `node_modules`, `vendor`, `.venv`, `dist`, `build` e afins) e consulta cada dependência:

| Ecossistema | Manifestos | Imports |
|---|---|---|
| npm | `package.json` (dependencies, dev, optional, peer) | `.js` `.jsx` `.mjs` `.cjs` `.ts` `.tsx` `.vue` `.svelte` `.astro` |
| PyPI | `requirements*.txt`, `pyproject.toml` (PEP 621, `[dependency-groups]`, Poetry, uv, PDM, `[build-system]`), `Pipfile`, `setup.cfg`, `setup.py` (só listas literais) | `.py` |
| Go | `go.mod` | — (o compilador já exige tudo no `go.mod`) |

| Situação | Gravidade |
|---|---|
| dependência declarada que não existe no registro (npm, PyPI, proxy do Go) | alta |
| import JS/TS de pacote não declarado que não existe no npm | alta |
| import Python de módulo sem pacote com esse nome no PyPI | média (o nome do pacote é deduzido) |
| pacote criado há menos de 30 dias | média |

Um import só é consultado quando nada no repositório explica o nome. Ficam de fora: módulos nativos (Node, biblioteca padrão do Python), caminhos relativos, aliases (`paths` do `tsconfig`/`jsconfig`, import maps do Deno, prefixos `@/`, `~/`, `#`, `$`), nomes de pastas e arquivos do próprio projeto, módulos de ambientes como Blender e MicroPython, e módulos Python de pacotes com nome conhecido diferente (`yaml` → PyYAML, `cv2` → opencv-python…).

Não são consultados:

- dependências de git, caminho local, URL ou workspace;
- pacotes definidos no próprio repositório (`name` de outro `package.json` ou `pyproject.toml`, `module` de outro `go.mod`);
- escopos npm com registro próprio no `.npmrc` (`@acme:registry=…`) ou em `npmScopes` do `.yarnrc.yml`;
- módulos Go em `GONOPROXY`/`GOPRIVATE` (do ambiente ou de `go env -w`) ou com `replace`;
- dependências Poetry com `source` próprio e pacotes listados em `[tool.uv.sources]`.

Ainda não são lidos: `extras_require` do `setup.py`, notebooks (`.ipynb`) e `environment.yml` do conda.

**Privacidade:** os *nomes* das dependências e dos imports sem explicação local (não o código) são enviados a [api.deps.dev](https://deps.dev) e, se ele não conhecer o pacote, ao registro oficial. Pacotes privados reconhecidos pelas regras acima nunca são enviados; outros nomes internos, sim. As respostas ficam em cache local por 24 h (1 h para pacotes não encontrados), na pasta de cache do usuário (`fiscal/packages`).

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
packages/     verificador de pacotes inexistentes ou recém-criados
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
FISCAL_LIVE=1 go test -run TestLive ./packages/   # consulta os registros reais
```

## Segurança

Para reportar uma falha, veja [SECURITY.md](SECURITY.md). Não abra issue pública.

## Licença

[Apache 2.0](LICENSE).
