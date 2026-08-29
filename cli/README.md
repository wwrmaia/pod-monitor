# podmon — CLI do Pod Monitor

Cliente de terminal pra API REST do Pod Monitor. Fala com o mesmo backend
usado pelo dashboard web — mesma autenticação, mesmo RBAC (`reader`/`admin`/`dev`),
mesmos dados.

Este é o **nível 1** do roadmap de CLI do projeto: um wrapper fino sobre a API
já existente, focado em leitura. Uma TUI interativa (navegação ao vivo, estilo
`k9s`) é a evolução planejada pra depois — ver `internal/client` e
`internal/config`, que já estão isolados da camada de apresentação (`cmd/`)
justamente pra serem reaproveitados sem mudança quando essa TUI vier.

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

Fora do escopo desta v1 (operações de admin/escrita — ver plano de implementação):
webhooks, thresholds, usuários/grupos, cadastro/remoção de cluster, hosts
Docker/Podman, Helm releases, deployments, topologia.

## Limitação conhecida: `podmon logs` não segue (`-f`)

O endpoint `GET /api/logs` do backend busca uma quantidade fixa de linhas uma
única vez (`--tail`, padrão 200) — não existe streaming de logs no servidor
hoje. `podmon logs` reflete isso honestamente: não tem flag `--follow`. Um
`tail -f` de verdade exigiria trabalho novo no backend, não só na CLI.

## Configuração local

A sessão (servidor, token, usuário) fica em `~/.config/pod-monitor-cli/config.yaml`,
com permissão `0600` — é onde o token Bearer fica guardado entre uma chamada e
outra, então não deve ser legível por outros usuários da máquina.
