# Política de segurança

## Como reportar uma falha

Não abra issue pública. Use o reporte privado do GitHub:
aba **Security** do repositório → **Report a vulnerability**.

Inclua, se puder:

- versão (`fiscal version`) e sistema operacional;
- passos para reproduzir;
- impacto esperado.

Tentamos responder em até 7 dias e combinar com você a data de divulgação depois da correção.

## Versões com suporte

Só a versão mais recente recebe correções de segurança.

## Escopo

Interessam especialmente:

- qualquer caminho em que o `fiscal` execute código do repositório analisado;
- leitura ou escrita fora do diretório analisado (symlinks, path traversal);
- segredos encontrados sendo exibidos sem máscara ou enviados para fora da máquina.
