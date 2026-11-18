# Arquitetura

## Hoje (CLI)

```mermaid
flowchart LR
    U[usuário / CI] --> CLI[cmd/fiscal]
    CLI --> Motor[scanner.Run]
    Motor -->|goroutine| P[pacotes]
    Motor -->|goroutine| S[segredos]
    Motor -->|goroutine| V[vulnerabilidades]
    P --> R[(npm / PyPI / Go proxy)]
    V --> O[(OSV)]
    Motor --> Rel[report: texto / JSON / SARIF]
```

- Cada verificador implementa `scanner.Checker` e roda na própria goroutine.
- Um verificador que falha não esconde os achados dos outros: o motor devolve resultado parcial mais o erro.
- Achados saem ordenados por gravidade, depois arquivo e linha.
- Regra fixa: nenhum verificador executa código do repositório analisado.
- O relatório em texto remove caracteres de controle vindos do repositório (nomes de arquivo podem carregar sequências de escape do terminal).

Verificadores (semanas 2–4) ficarão em pacotes próprios e entram na lista `checkers()` de `cmd/fiscal/main.go`.

## Planejado (web, semanas 6–8)

```mermaid
flowchart LR
    B[navegador] --> W[Next.js na Vercel]
    W -->|cria trabalho| DB[(Postgres)]
    K[worker Go em contêiner] -->|SKIP LOCKED| DB
    K -->|clone limitado| GH[(GitHub público)]
    K -->|grava relatório| DB
    W -->|lê progresso| DB
```

Propostas ainda abertas para decisão:

- **Fila no próprio Postgres** (`SELECT … FOR UPDATE SKIP LOCKED`) em vez de serviço de fila separado.
- **Segredos sempre mascarados** no relatório web; detalhe completo só para quem prova ser dono do repositório (login GitHub com permissão de push). Sem isso, a página pública vira coletor de chaves de terceiros.
- **Clone limitado** no worker: histórico parcial, `--filter=blob:limit`, sem submódulos nem LFS, com limite de tamanho, CPU, memória e tempo.
