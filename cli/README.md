# podmon — CLI do Pod Monitor

Cliente de terminal pra API REST do Pod Monitor. Fala com o mesmo backend
usado pelo dashboard web — mesma autenticação, mesmo RBAC (`reader`/`admin`/`dev`),
mesmos dados.

O **nível 1** do roadmap de CLI é um wrapper fino sobre a API já existente,
focado em leitura (subcomandos abaixo). O **nível 2** é `podmon tui`, um
painel interativo estilo `k9s` (navegação ao vivo, atualização automática) —
ver seção [TUI](#tui-podmon-tui) abaixo. Os dois reaproveitam o mesmo
`internal/client`/`internal/config`, isolados da camada de apresentação
(`cmd/`/`internal/tui`) desde o início.

Pra instalar binários pré-compilados (Linux/macOS/Windows) sem precisar do Go,
ver [`INSTALL.md`](./INSTALL.md).

## Build

```bash
cd cli
go build -o podmon .
```

Sem automação de release/binários multi-plataforma por enquanto — `go build`
é suficiente pra uso local, mesma simplicidade do resto do projeto.

## Uso

```bash
podmon login --server http://192.168.49.2:30080   # ou o endereço do seu deploy
podmon clusters
podmon resources --cluster minikube --namespace pod-monitor
podmon costs --cluster minikube
podmon dashboard --cluster minikube
podmon events --cluster minikube                  # tail ao vivo dos eventos (SSE), Ctrl+C pra sair
podmon logout
```

Toda saída aceita `--output json` pra scripting:

```bash
podmon --output json costs --cluster prod-eks | jq '.items[] | select(.cost_month > 100)'
```

## Comandos

| Comando | Equivalente na web | Observação |
|---|---|---|
| `login` | — | Prompt de usuário/senha; trata MFA (código já configurado, ou secret pra configurar) |
| `logout` | — | Revoga o token no servidor e limpa a sessão local |
| `clusters` | seletor de cluster | |
| `namespaces` | seletor de namespace | |
| `resources` | Monitor | requests/limits/uso de CPU e memória, por container |
| `nodes` | Nodes | |
| `top` | Top 10 | |
| `costs` | Custos | mostra aviso se o cluster estiver com a estimativa desativada (padrão) ou sem preço configurado |
| `certs` | Certificados | |
| `storage` | Storage | |
| `quotas` | Quotas | |
| `orphans` | Auditoria | recursos órfãos (PVC, Service, ConfigMap, Secret, Ingress, ServiceAccount) |
| `analysis` | Análise | roda a varredura de segurança/confiabilidade/boas práticas sob demanda |
| `logs <pod>` | Logs | **snapshot único, não é streaming** — ver limitação abaixo |
| `events` | (SSE interno) | tail ao vivo de todos os eventos do backend |
| `tui` | — | painel interativo, ver seção [TUI](#tui-podmon-tui) |

Fora do escopo desta v1 (operações de admin/escrita — ver plano de implementação):
webhooks, thresholds, usuários/grupos, cadastro/remoção de cluster, hosts
Docker/Podman, Helm releases, deployments, topologia.

## TUI (`podmon tui`)

Painel interativo (Bubble Tea) que fica rodando e se atualiza sozinho, em vez
de um comando por chamada:

```bash
podmon tui                                    # abre no seletor de clusters
podmon tui --cluster local --namespace pod-monitor   # pula direto pra tabela de pods
```

Fluxo principal: clusters → namespaces → tabela de pods/containers (com uso
de CPU/mem e severidade aproximada). De qualquer uma dessas três telas, uma
tecla global abre uma tela cheia com outro recurso — `esc` volta pra onde
você estava:

| Tecla | Abre |
|---|---|
| `d` | Dashboard/alertas (dados autoritativos, iguais ao `podmon dashboard`) |
| `n` | Nodes |
| `s` | Storage (PVCs) |
| `u` | Quotas |
| `o` | Auditoria (recursos órfãos) |
| `p` | Custos |
| `t` | Certificados TLS |

Na tabela de pods: `/` filtra por nome, `c` cicla ordenação (nome/CPU%/MEM%).
Em qualquer tela: `r` força atualização, `?` abre a ajuda completa, `:` abre
uma barra de comando (digite `nodes`, `costs`, `dashboard` etc. e `enter` —
alternativa às teclas de atalho, útil se você não decorou os atalhos),
`q`/`Ctrl+C` sai.

Uma barra de contexto fixa no topo mostra cluster/namespace/usuário atuais e
um resumo de alertas (do último dashboard já visitado); a logo do podmon
aparece à direita em terminais largos o bastante. Tabelas/listas ganham
borda quando cabem no terminal — numa tela muito estreita pra tabela de pods
(13-14 colunas), a borda some sozinha em vez de aparecer quebrada.

Atualização automática: a tabela de pods e o dashboard pollam sozinhos
(8s/10s) enquanto abertos, mais um empurrão extra via SSE quando algo muda no
backend. As seis telas de recurso (nodes/storage/quotas/orphans/custos/certs)
buscam ao abrir e com `r` manual — mudam devagar o bastante pra não precisar
de polling automático.

## Limitação conhecida: `podmon logs` não segue (`-f`)

O endpoint `GET /api/logs` do backend busca uma quantidade fixa de linhas uma
única vez (`--tail`, padrão 200) — não existe streaming de logs no servidor
hoje. `podmon logs` reflete isso honestamente: não tem flag `--follow`. Um
`tail -f` de verdade exigiria trabalho novo no backend, não só na CLI.

## Configuração local

A sessão (servidor, token, usuário) fica em `~/.config/pod-monitor-cli/config.yaml`,
com permissão `0600` — é onde o token Bearer fica guardado entre uma chamada e
outra, então não deve ser legível por outros usuários da máquina.
