# Ideias para depois

Fora do MVP. Entram no plano só por decisão explícita, para o escopo não crescer sozinho.

## Pacotes

- **Comandos de instalação em texto**: `pip install x`, `npm install x` e `npx x` em README, Dockerfile, scripts e workflows. É por onde o nome inventado chega ao terminal; `npx` executa na hora. Hoje só o `%pip install` de notebooks é lido.
- **Typosquatting**: nome parecido com pacote popular (`reqeusts`, `lodahs`), por distância de edição contra os mais baixados.
- **Pacotes descontinuados**: o deps.dev já informa `isDeprecated`; a IA costuma sugerir pacotes abandonados (`request` no npm).
- **Registro padrão privado** (`registry=` no `.npmrc`): pacotes internos sem escopo hoje aparecem como inexistentes.
- **Lista de exceções por projeto** (arquivo de configuração) para silenciar um falso positivo. Necessária antes da GitHub Action (semana 9).
- **Amostra de código gerado por IA** para medir falso positivo: a primeira medição ([pacotes.md](pacotes.md#medição-de-falsos-positivos)) usou projetos escritos por pessoas.
- **Imports Go** fora do `go.mod`: hoje o compilador já pega.
- `setup.py` com listas montadas em tempo de execução.
- **Progresso no terminal** durante consultas longas.

## Plataforma

- **Cache compartilhado** no worker web (Postgres) em vez de disco local.
- Revisão de lógica por IA, análise do site publicado, repositórios privados (fora do MVP desde o plano original).
