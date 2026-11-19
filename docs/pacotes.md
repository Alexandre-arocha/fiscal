# Verificador de pacotes

Confere se cada dependência do projeto existe no registro público e não foi criada há poucos dias. Pega pacotes que a IA inventou (*slopsquatting*): um nome que não existe hoje pode ser registrado amanhã por um atacante, com código malicioso.

## O que é lido

| Ecossistema | Manifestos | Imports no código |
|---|---|---|
| npm | `package.json` (`dependencies`, `devDependencies`, `optionalDependencies`, `peerDependencies`) | `.js` `.jsx` `.mjs` `.cjs` `.ts` `.tsx` `.mts` `.cts` `.vue` `.svelte` `.astro` |
| PyPI | `requirements*.txt`, `*-requirements.txt`, `requirements*.in`, `requirements/*.txt`; `pyproject.toml` (PEP 621, `[dependency-groups]`, Poetry, uv, PDM, Hatch, Rye, `[build-system]`); `Pipfile`; `setup.cfg`; `setup.py` (só listas literais); seção `pip:` do `environment.yml` (conda); `%pip install` em notebooks | `.py` e células de código de `.ipynb` |
| Go | `go.mod` | — (o compilador já exige tudo no `go.mod`) |

Pastas ignoradas: dependências instaladas (`node_modules`, `vendor`, `.venv`…), build e cache (`dist`, `build`, `.next`, `__pycache__`…) e fixtures de teste (`testdata`, `fixtures`, `__fixtures__`). Também ficam de fora arquivos-fonte acima de 1 MB e `.min.js`.

## Achados

| Situação | Gravidade |
|---|---|
| dependência declarada que não existe no registro | alta |
| import JS/TS de pacote não declarado que não existe no npm | alta |
| import Python de módulo sem pacote com esse nome no PyPI | média (o nome do pacote é deduzido) |
| pacote criado há menos de 30 dias | média |
| manifesto ilegível | baixa |

## O que não é consultado

Para não gerar alarme falso nem vazar nomes internos:

- dependências de git, caminho local, URL ou workspace (contam como declaradas);
- pacotes definidos no próprio repositório: `name` de `package.json`, `pyproject.toml` e `setup.cfg`, `module` de `go.mod`;
- escopos npm com registro próprio no `.npmrc` (`@acme:registry=…`) ou em `npmScopes` do `.yarnrc.yml`;
- módulos Go em `GONOPROXY`/`GOPRIVATE` (do ambiente ou de `go env -w`) ou com `replace`;
- dependências Poetry com `source` próprio, entradas do `Pipfile` presas a índice privado e pacotes em `[tool.uv.sources]`;
- pacotes conda: vêm dos canais do conda, não do PyPI (contam como declarados).

Um import só vira consulta quando nada no repositório explica o nome. Ficam de fora:

- módulos nativos (Node, biblioteca padrão do Python) e imports relativos;
- aliases: `paths` do `tsconfig`/`jsconfig`, import maps do Deno, prefixos `@/`, `~/`, `#`, `$`;
- arquivos e pastas do projeto: `utils`, `components/Button` e `@components/Card` quando existem `utils.*`, `components/Button.*` e `components/Card.*`;
- módulos de ambientes como Blender, MicroPython e Kodi;
- módulos Python de pacotes com nome conhecido diferente (`yaml` → PyYAML, `cv2` → opencv-python, `PIL` → Pillow…) ou que uma dependência declarada fornece (`python-dateutil` → `dateutil`).

## Como consulta

1. Fonte principal: [deps.dev](https://deps.dev) (`/v3/systems/{npm|pypi|go}/packages/{nome}`): existência e data da primeira versão.
2. Se o deps.dev não conhece o pacote, confirma no registro oficial (npm, `/simple/` do PyPI, `@latest` do proxy do Go). Existe lá e não no deps.dev = publicado há pouco.
3. Até 8 consultas simultâneas. Erros de rede, 429 e 5xx têm até 4 tentativas, com espera crescente ou `Retry-After` (máx. 30 s). Se um registro continua indisponível depois disso, as consultas restantes param e o erro aparece uma vez só (código de saída 2).
4. Cache em disco, na pasta de cache do usuário (`fiscal/packages`): 24 h para pacotes encontrados, 1 h para não encontrados (um nome livre pode ser registrado a qualquer momento).

## Privacidade

Saem da máquina só os *nomes* das dependências e dos imports sem explicação local, nunca o código. Pacotes privados reconhecidos pelas regras acima não são enviados; outros nomes internos, sim. Para não consultar nada: `fiscal scan --offline`.
