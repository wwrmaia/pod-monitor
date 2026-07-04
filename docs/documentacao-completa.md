# Pod Monitor — Documentação Completa

## Índice

1. [Visão Geral](#1-visão-geral)
2. [Arquitetura](#2-arquitetura)
3. [Implantação Kubernetes](#3-implantação-kubernetes)
4. [Autenticação e Controle de Acesso](#4-autenticação-e-controle-de-acesso)
5. [API — Referência Completa](#5-api--referência-completa)
6. [Banco de Dados PostgreSQL](#6-banco-de-dados-postgresql)
7. [Frontend — Módulos e Abas](#7-frontend--módulos-e-abas)
8. [Módulo de Análise](#8-módulo-de-análise)
9. [Dashboard](#9-dashboard)
10. [Temas Visuais](#10-temas-visuais)
11. [Docker / Podman — Monitoramento Externo](#11-docker--podman--monitoramento-externo)
12. [Monitoramento Multi-Cluster](#12-monitoramento-multi-cluster)
13. [Documentação de Ajuda In-App](#13-documentação-de-ajuda-in-app)
14. [Desenvolvimento Local](#14-desenvolvimento-local)
15. [Topologia](#15-topologia)
16. [Cotas de Recursos (Quotas)](#16-cotas-de-recursos-quotas)
17. [Webhooks de Alertas](#17-webhooks-de-alertas)
18. [Thresholds de Alerta Configuráveis](#18-thresholds-de-alerta-configuráveis)
19. [Log de Auditoria](#19-log-de-auditoria)
20. [Atualizações em Tempo Real (SSE)](#20-atualizações-em-tempo-real-sse)
21. [Aviso de Expiração de Sessão](#21-aviso-de-expiração-de-sessão)
22. [Modo NOC — Rotação Automática](#22-modo-noc--rotação-automática)
23. [Segurança](#23-segurança)
24. [Paginação de APIs](#24-paginação-de-apis)
25. [Documentação OpenAPI / Swagger UI](#25-documentação-openapi--swagger-ui)
26. [Changelog](#26-changelog)

---

## 1. Visão Geral

O **Pod Monitor** é um dashboard de monitoramento de infraestrutura que consolida em uma única interface:

- Recursos de CPU e memória de pods Kubernetes (requests, limits, uso real)
- Status de nodes do cluster
- PVCs (Persistent Volume Claims) e recursos órfãos
- Containers Docker e Podman — tanto os nós do cluster quanto daemons externos
- Releases gerenciadas pelo Helm
- Deployments e sua situação de disponibilidade
- Histórico de snapshots de recursos
- Dashboards personalizáveis com widgets gráficos
- **Análise de boas práticas** — varredura sob demanda baseada no CIS Kubernetes Benchmark

### Tecnologias

| Camada | Tecnologia |
|--------|-----------|
| Backend | Go 1.23+, `k8s.io/client-go`, `github.com/lib/pq`, `golang.org/x/crypto/bcrypt`, `github.com/pquerna/otp` |
| Frontend | React 19, Vite 8, Axios, react-grid-layout, recharts |
| Banco de dados | PostgreSQL 16 (container dedicado) |
| Autenticação | JWT com HMAC-SHA256 + blacklist, MFA via TOTP criptografado (AES-256-GCM), rate limiting por IP |
| Infraestrutura | Kubernetes (testado no Minikube), Nginx Ingress |

---

## 2. Arquitetura

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Navegador                                                               │
│  React + Vite (porta 5173 em dev / Nginx em produção)                   │
│  Axios → /api/*                                                          │
└───────────────────────────┬──────────────────────────────────────────────┘
                            │ HTTP (Nginx Ingress → pod-monitor.local)
┌───────────────────────────▼──────────────────────────────────────────────┐
│  Backend Go (porta 8080)                                                 │
│  ┌──────────────┐  ┌───────────────┐  ┌──────────────────────────────┐  │
│  │  Auth / JWT  │  │  Kubernetes   │  │  Docker/Podman HTTP clients  │  │
│  │  MFA (TOTP)  │  │  client-go    │  │  (Unix socket ou TCP proxy)  │  │
│  └──────────────┘  └───────────────┘  └──────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────────────┐    │
│  │  PostgreSQL  (users, groups, pod_snapshots, dashboards, etc.)    │    │
│  └──────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────┘
         │                   │                      │
    k8s API server     metrics-server         Docker/Podman
    (ClusterRole)      (metrics.k8s.io)        daemons
```

### Fluxo de dados principal

1. O frontend solicita `/api/clusters` e `/api/namespaces` ao montar.
2. O usuário seleciona cluster e namespace, clica em Consultar.
3. O frontend chama `/api/resources?cluster=X&namespace=Y`.
4. O backend consulta a Kubernetes API (pods specs) e a Metrics API (uso real).
5. Os dados são mesclados e retornados como JSON; o backend salva um snapshot no PostgreSQL.
6. O frontend converte unidades (`m`, `Gi`, `Mi`, `Ki`), compara com os thresholds e renderiza a tabela.

---

## 3. Implantação Kubernetes

Todos os recursos residem no namespace `pod-monitor`.

### Manifestos

```
k8s/
├── namespace.yaml       # Cria o namespace pod-monitor
├── rbac.yaml            # ServiceAccount, ClusterRole, ClusterRoleBinding, Role, RoleBinding
├── postgres.yaml        # Secret, PVC, Deployment e Service do PostgreSQL
├── backend.yaml         # Deployment + Service do backend (+ sidecar docker-socket-proxy)
├── frontend.yaml        # Deployment + Service do frontend
├── ingress.yaml         # Ingress nginx (host: pod-monitor.local)
└── network-policy.yaml  # NetworkPolicy restringindo tráfego entre pods (requer CNI compatível)
```

### Ordem de aplicação

```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/rbac.yaml
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/backend.yaml
kubectl apply -f k8s/frontend.yaml
kubectl apply -f k8s/ingress.yaml
kubectl apply -f k8s/network-policy.yaml   # opcional, requer Calico/Cilium/Weave
```

### Helm Chart

O chart está em `helm/pod-monitor/` e suporta todos os ambientes (kind, k3s, AKS, EKS, OpenShift).

```bash
# Instalar com valores padrão (puxa imagens do Docker Hub: docker.io/wwrmaia)
helm install pod-monitor helm/pod-monitor -n pod-monitor --create-namespace

# Instalar com values específicos de ambiente
helm install pod-monitor helm/pod-monitor -n pod-monitor --create-namespace \
  -f helm/pod-monitor/values-kind.yaml

# Atualizar
helm upgrade pod-monitor helm/pod-monitor -n pod-monitor -f helm/pod-monitor/values-kind.yaml
```

#### Principais valores configuráveis (`values.yaml`)

| Valor | Descrição | Padrão |
|-------|-----------|--------|
| `image.registry` | Registry prefix | `docker.io/wwrmaia` |
| `image.pullPolicy` | Pull policy das imagens | `IfNotPresent` |
| `postgres.enabled` | Implanta PostgreSQL junto com a aplicação | `true` |
| `postgres.user` | Usuário do banco | `podmonitor` |
| `postgres.password` | Senha do banco (trocar em produção!) | `changeme` |
| `postgres.existingSecret` | Secret k8s existente com as credenciais do Postgres | `""` |
| `backend.jwtSecret.existingSecret` | Nome do Secret k8s com o JWT | `""` |
| `backend.frontendOrigin` | Origin permitida pelo CORS (ex: `https://monitor.empresa.com`) | `""` (qualquer) |
| `backend.docker.enabled` | Habilita monitoramento de daemons Docker externos | `false` |
| `frontend.backendHost` | Hostname do backend usado pelo Nginx (`/api/*`) | `backend-svc` |
| `storage.className` | StorageClass do PVC do PostgreSQL | default do cluster |
| `storage.size` | Tamanho do PVC do PostgreSQL | `5Gi` |
| `ingress.enabled` | Cria Ingress | `true` |
| `ingress.host` | Hostname do Ingress | `pod-monitor.local` |
| `openshift.enabled` | Cria Route em vez de Ingress | `false` |

> **`frontend.backendHost`** é injetado como variável de ambiente no container Nginx. Em Kubernetes o padrão `backend-svc` é sempre correto. Em docker-compose, o `docker-compose.hub.yml` define `BACKEND_HOST=backend` automaticamente.

#### Values por ambiente

| Arquivo | Ambiente |
|---------|----------|
| `values-kind.yaml` | kind (local) |
| `values-k3s.yaml` | k3s |
| `values-aks.yaml` | Azure AKS |
| `values-eks.yaml` | AWS EKS |
| `values-openshift.yaml` | OpenShift / OKD |

### Instalação via Docker Compose (Docker Hub)

Para instalar em um host com Docker sem Kubernetes:

```bash
# Baixar o compose e subir
curl -O https://raw.githubusercontent.com/wwrmaia/pod-monitor/main/docker-compose.hub.yml
docker compose -f docker-compose.hub.yml up -d
```

Acesso em `http://localhost:3000`. As imagens são puxadas diretamente do Docker Hub (`wwrmaia/pod-monitor-backend:latest` e `wwrmaia/pod-monitor-frontend:latest`).

Variáveis opcionais via arquivo `.env` na mesma pasta:

```env
JWT_SECRET=sua_chave_secreta
ADMIN_PASSWORD_HASH=hash_bcrypt_da_senha_admin
POSTGRES_PASSWORD=senha_segura          # padrão: changeme
FRONTEND_ORIGIN=https://seu-dominio.com # restringe CORS (recomendado em produção)
```

### Build das imagens (Minikube)

```bash
# IMPORTANTE: sempre buildar dentro do daemon Docker do Minikube
eval $(minikube docker-env)

# Backend
docker build -t pod-monitor-backend:latest ./backend

# Frontend
docker build -t pod-monitor-frontend:latest ./frontend

# Reiniciar deployments após o build
kubectl rollout restart deployment/backend deployment/frontend -n pod-monitor
```

> Nunca use `minikube image load` para tags `:latest` — o `imagePullPolicy: Never` exige que a imagem já esteja no daemon do Minikube com o hash correto.

### RBAC — Permissões

O `ClusterRole pod-monitor-role` concede acesso de leitura a:

| API Group | Recursos |
|-----------|----------|
| `""` (core) | pods, namespaces, nodes, persistentvolumeclaims, services, endpoints, configmaps, secrets, serviceaccounts, resourcequotas, limitranges |
| `""` (core) | pods/log (get) |
| `apps` | deployments, replicasets, statefulsets, daemonsets |
| `networking.k8s.io` | ingresses |
| `metrics.k8s.io` | pods, nodes |

> `resourcequotas` e `limitranges` são necessários para o módulo de Análise.

O `Role pod-monitor-admin-role` (namespaced, apenas no namespace `pod-monitor`) permite ao backend gerenciar seus próprios Secrets e Deployments, necessário para o módulo de administração de clusters.

### Variáveis de ambiente do backend

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| `DATABASE_URL` | DSN PostgreSQL (obrigatório) | — |
| `JWT_SECRET` | Chave HMAC para assinatura dos tokens | Chave aleatória (tokens invalidados ao reiniciar) |
| `FRONTEND_ORIGIN` | Origin permitida pelo CORS (ex: `https://monitor.empresa.com`) | `""` (qualquer) |
| `DOCKER_HOSTS` | Lista de daemons Docker/Podman a monitorar | Auto-detecta `/var/run/docker.sock` e `/run/podman/podman.sock` |
| `KUBECONFIG` | Caminho para kubeconfig de clusters adicionais | `/etc/kubeconfig/config` |
| `BACKEND_HOST` | Hostname do backend usado pelo Nginx do frontend | `backend-svc` (k8s) / `backend` (docker-compose) |

Exemplo de `DATABASE_URL`:
```
postgres://podmonitor:changeme@postgres-svc:5432/podmonitor?sslmode=disable
```

> **Inicialização sem clusters:** o backend sobe normalmente mesmo sem kubeconfig configurado, exibindo um aviso no log. Os clusters são adicionados em runtime pela interface de admin (Admin → Clusters).

#### Formato de DOCKER_HOSTS

```
DOCKER_HOSTS=nome1=/caminho/unix.sock,nome2=tcp://IP:PORTA,...
```

Exemplo real:

```yaml
value: "minikube-node=/var/run/docker.sock,podman=/run/podman/podman.sock,host-docker=tcp://192.168.49.1:2375"
```

### JWT Secret (produção)

O JWT_SECRET é armazenado em um Secret Kubernetes, nunca hardcoded:

```bash
# Criar ou rotacionar o secret
kubectl create secret generic pod-monitor-secrets \
  --from-literal=jwt-secret=$(openssl rand -hex 32) \
  --namespace pod-monitor \
  --dry-run=client -o yaml | kubectl apply -f -

# Após rotação, reiniciar o backend para invalidar tokens antigos
kubectl rollout restart deployment/backend -n pod-monitor
```

### Ingress

- Host: `pod-monitor.local`
- `/api/*` → `backend-svc:8080`
- `/` → `frontend-svc:80`

Adicione ao `/etc/hosts` da máquina host:

```
$(minikube ip)  pod-monitor.local
```

---

## 4. Autenticação e Controle de Acesso

### Perfis de usuário (roles)

| Role | Label na UI | Acesso |
|------|-------------|--------|
| `administration` | Administration | Acesso total — todos os módulos, gerenciamento de usuários, grupos, clusters |
| `reader` | Reader | Monitoramento somente leitura — pods, nodes, storage, docker, helm, deployments, histórico, análise |
| `dev` | Dev | Acesso restrito aos clusters/namespaces liberados pelo administrador + logs de pods |

### Fluxo de login

```
POST /api/auth/login
  → senha correta sem MFA    → retorna JWT
  → senha correta com MFA    → retorna mfa_token (JWT temporário 10 min)
    → POST /api/auth/mfa/validate  → retorna JWT final
  → primeiro login com MFA   → retorna setup_token
    → GET  /api/auth/mfa/setup          → retorna QR code + secret
    → POST /api/auth/mfa/setup/confirm  → confirma código, retorna JWT final
```

### JWT

- Algoritmo: HMAC-SHA256
- Payload: `sub` (username), `role`, `exp` (24h), `allowed_clusters`, `allowed_namespaces`
- Enviado como `Authorization: Bearer <token>` em todas as requisições autenticadas

### MFA — Autenticação Multi-Fator

- Padrão TOTP (RFC 6238), compatível com Google Authenticator, Authy, etc.
- Configurável por usuário individual ou por grupo
- O administrador pode:
  - Habilitar/desabilitar MFA por usuário (`POST /api/auth/mfa/toggle`)
  - Resetar o MFA de um usuário (remove secret, reseta setup) via `POST /api/auth/mfa/reset-user`
  - Gerar QR codes em massa para um grupo inteiro via `POST /api/auth/groups/mfa`
- O botão "Resetar MFA" aparece na UI sempre que `totp_enabled = true`, mesmo antes do setup estar concluído

### Grupos

Grupos centralizam a configuração de acesso:

```
Grupo
 ├── role (administration / reader / dev)
 ├── totp_enabled
 ├── allowed_clusters[]
 ├── allowed_namespaces[]
 └── members[]  → usuários herdam todas as configurações do grupo
```

Usuários sem grupo mantêm suas configurações individuais.

### Usuários padrão (seed)

Criados automaticamente se não existirem no banco:

| Username | Senha inicial | Role |
|----------|--------------|------|
| `admin` | `admin` | administration |
| `reader` | `reader` | reader |
| `dev` | `dev` | dev |
| `dev2` | `dev2` | dev |

> Troque as senhas imediatamente após o primeiro acesso.

---

## 5. API — Referência Completa

### Middlewares de autorização

| Middleware | Quem pode acessar |
|------------|------------------|
| `authMiddleware` | Qualquer usuário autenticado |
| `readerOrAdminOnly` | reader + administration |
| `devOrAdminOnly` | dev + administration |
| `adminOnly` | Apenas administration |

### Autenticação

| Método | Endpoint | Auth | Descrição |
|--------|----------|------|-----------|
| POST | `/api/auth/login` | — | Login com username/password |
| POST | `/api/auth/mfa/validate` | — | Validação do código TOTP pós-login |
| GET | `/api/auth/mfa/setup` | — | Retorna QR code e secret para configuração |
| POST | `/api/auth/mfa/setup/confirm` | — | Confirma código e ativa MFA |
| POST | `/api/auth/mfa/reset-user` | adminOnly | Reseta MFA de um usuário |

### Gerenciamento de usuários (adminOnly)

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| GET | `/api/auth/users` | Lista todos os usuários |
| POST | `/api/auth/users/create` | Cria novo usuário |
| POST | `/api/auth/users/delete` | Remove usuário |
| POST | `/api/auth/users/password` | Altera senha de qualquer usuário |
| POST | `/api/auth/users/role` | Altera a role de um usuário |
| POST | `/api/auth/users/dev-access` | Define clusters/namespaces permitidos para um usuário |
| POST | `/api/auth/mfa/toggle` | Habilita/desabilita MFA de um usuário |

### Gerenciamento de grupos (adminOnly)

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| GET | `/api/auth/groups` | Lista todos os grupos |
| POST | `/api/auth/groups/create` | Cria novo grupo |
| POST | `/api/auth/groups/delete` | Remove grupo |
| POST | `/api/auth/groups/access` | Define clusters/namespaces permitidos para um grupo |
| POST | `/api/auth/groups/mfa` | Habilita/desabilita MFA do grupo e gera QR codes |
| POST | `/api/auth/groups/members` | Adiciona/remove membros do grupo |

### Administração de clusters (adminOnly)

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| POST | `/api/admin/validate` | Valida um kubeconfig e retorna os clusters disponíveis |
| POST | `/api/admin/create-sa` | Cria ServiceAccount + RBAC no cluster alvo e retorna kubeconfig |
| POST | `/api/admin/apply` | Aplica kubeconfig de um cluster adicional e o registra no backend |
| POST | `/api/admin/docker-host` | Adiciona um novo host Docker/Podman em runtime |
| DELETE | `/api/admin/cluster/delete` | Remove um cluster do monitoramento (requer senha bcrypt) |

### Monitoramento de Kubernetes (autenticado)

| Método | Endpoint | Auth | Descrição |
|--------|----------|------|-----------|
| GET | `/api/clusters` | auth | Lista clusters disponíveis |
| GET | `/api/namespaces?cluster=X` | auth | Lista namespaces do cluster |
| GET | `/api/resources?cluster=X&namespace=Y` | auth | Pods com resources + usage; salva snapshot |
| GET | `/api/nodes?cluster=X` | readerOrAdmin | Nodes com CPU/mem alocável vs. uso |
| GET | `/api/storage?cluster=X&namespace=Y` | readerOrAdmin | Lista PVCs |
| GET | `/api/orphans?cluster=X` | readerOrAdmin | Detecta recursos órfãos (PVCs, Services, ConfigMaps, Secrets, Ingresses, ServiceAccounts) |
| GET | `/api/deployments?cluster=X&namespace=Y` | readerOrAdmin | Lista Deployments com status de réplicas |
| GET | `/api/logs?cluster=X&namespace=Y&pod=P&container=C` | devOrAdmin | Logs de container |
| GET | `/api/history?cluster=X&namespace=Y` | readerOrAdmin | Histórico de snapshots (até 2000, últimos 7 dias) |
| GET | `/api/history/csv?cluster=X&namespace=Y` | readerOrAdmin | Histórico em formato CSV |
| GET | `/api/analysis?cluster=X&namespace=Y` | readerOrAdmin | Análise de boas práticas (namespace opcional) |
| GET | `/api/topology?cluster=X&namespace=Y` | readerOrAdmin | Grafo de topologia do cluster (nodes, edges, HPAs) |
| GET | `/api/quotas?cluster=X&namespace=Y` | readerOrAdmin | ResourceQuotas e LimitRanges por namespace |

### Observabilidade em tempo real

| Método | Endpoint | Auth | Descrição |
|--------|----------|------|-----------|
| GET | `/api/sse/events` | auth | Stream SSE — eventos `summary` (alertas) e `topology_refresh` (topologia) |

### Administração avançada (adminOnly)

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| GET | `/api/thresholds` | Lista thresholds de alerta configurados |
| POST | `/api/thresholds` | Salva ou atualiza um threshold |
| DELETE | `/api/thresholds?id=N` | Remove um threshold |
| GET | `/api/webhooks` | Lista webhooks configurados |
| POST | `/api/webhooks` | Salva ou atualiza um webhook |
| DELETE | `/api/webhooks?id=N` | Remove um webhook |
| POST | `/api/webhooks/test` | Envia payload de teste para uma URL webhook |
| GET | `/api/audit?limit=N` | Lista entradas do log de auditoria |

### Monitoramento Docker/Podman (readerOrAdmin)

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| GET | `/api/docker/hosts` | Lista hosts Docker/Podman configurados com flag `cluster` |
| GET | `/api/docker/containers?host=X` | Containers do host externo (excluindo containers k8s) |
| GET | `/api/containers?host=X` | Containers dos nós do cluster (containers k8s) |

### Helm (readerOrAdmin)

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| GET | `/api/helm/releases?cluster=X&namespace=Y` | Lista releases Helm com status, versão, data de deploy |

### Dashboard

| Método | Endpoint | Auth | Descrição |
|--------|----------|------|-----------|
| GET | `/api/dashboard/summary?cluster=X` | auth | Agregado: pods por fase, nodes, top-5 CPU/mem, alertas, Helm, Docker |
| GET | `/api/dashboard/timeseries?cluster=X&hours=N` | auth | Série temporal de CPU/mem do cluster |
| GET | `/api/dashboards` | auth | Lista dashboards salvos do usuário logado |
| POST | `/api/dashboards/save` | auth | Salva ou atualiza um dashboard |
| POST | `/api/dashboards/delete` | auth | Remove um dashboard |

#### Estrutura de resposta do `/api/analysis`

```json
{
  "findings": [
    {
      "severity": "critical",
      "category": "resources",
      "resource_kind": "Pod",
      "resource_name": "my-pod",
      "namespace": "default",
      "container": "app",
      "message": "Container sem memory limit definido",
      "recommendation": "Defina resources.limits.memory para evitar OOMKill de outros processos no nó."
    }
  ],
  "summary": {
    "critical": 3,
    "warning": 12,
    "info": 8,
    "scanned_namespaces": 5,
    "scanned_pods": 42,
    "scanned_deployments": 10,
    "scanned_nodes": 2,
    "duration_ms": 1234
  },
  "nodes": ["minikube", "worker-1"]
}
```

---

## 6. Banco de Dados PostgreSQL

O Pod Monitor usa **PostgreSQL 16** como banco de dados. A conexão é configurada via variável de ambiente `DATABASE_URL`. O schema é criado automaticamente no primeiro startup (idempotente: usa `CREATE TABLE IF NOT EXISTS` e `ADD COLUMN IF NOT EXISTS`).

Em Kubernetes, um container `postgres:16-alpine` dedicado é implantado no mesmo namespace. Em Docker Compose, o serviço `postgres` é definido no `docker-compose.hub.yml`. Para apontar para um PostgreSQL externo (RDS, Cloud SQL, etc.), basta fornecer o DSN correto em `DATABASE_URL` e desabilitar `postgres.enabled: false` no Helm.

### Migração de SQLite para PostgreSQL

Usuários que tinham uma instalação anterior com SQLite podem migrar os dados com o script incluído:

```bash
./scripts/migrate-sqlite-to-postgres.sh \
  --sqlite /path/para/pod-monitor.db \
  --pg-url "postgres://podmonitor:changeme@localhost:5432/podmonitor?sslmode=disable"
```

Requisitos: `sqlite3` e `psql` instalados localmente. O script é idempotente para a maioria das tabelas.

### Tabelas

#### `users`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `username` | TEXT PK | Nome do usuário |
| `password` | TEXT | Hash bcrypt |
| `role` | TEXT | `administration`, `reader` ou `dev` |
| `allowed_clusters` | TEXT | JSON array de clusters permitidos |
| `allowed_namespaces` | TEXT | JSON array de namespaces permitidos |
| `totp_secret` | TEXT | Secret TOTP criptografado com AES-256-GCM (prefixo `enc:`) |
| `totp_enabled` | INTEGER | 0 ou 1 |
| `group_name` | TEXT | Nome do grupo ao qual o usuário pertence |

#### `groups`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `name` | TEXT PK | Nome do grupo |
| `role` | TEXT | Role aplicada a todos os membros |
| `totp_enabled` | INTEGER | 0 ou 1 |
| `allowed_clusters` | TEXT | JSON array |
| `allowed_namespaces` | TEXT | JSON array |

#### `pod_snapshots`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGSERIAL PK | Autoincrement |
| `session_id` | TEXT | ID aleatório que agrupa snapshots de uma consulta |
| `captured_at` | TEXT | ISO 8601 UTC |
| `cluster` | TEXT | Nome do cluster |
| `namespace` | TEXT | Namespace |
| `pod` | TEXT | Nome do pod |
| `node` | TEXT | Nó onde o pod está |
| `container` | TEXT | Nome do container |
| `cpu_request/limit/usage` | TEXT | Valores em formato k8s (ex: `250m`, `1`) |
| `mem_request/limit/usage` | TEXT | Valores em formato k8s (ex: `128Mi`, `1Gi`) |

Índices: `(namespace, captured_at)` e `(cluster, captured_at)`.

Limpeza automática: registros com mais de 7 dias são removidos a cada 6 horas.

#### `dashboards`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGSERIAL PK | Autoincrement |
| `username` | TEXT | Dono do dashboard |
| `name` | TEXT | Nome do dashboard |
| `widgets` | TEXT | JSON array de widgets com layout e tipo |

#### `alert_thresholds`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGSERIAL PK | Autoincrement |
| `cluster` | TEXT | Nome do cluster (vazio = global) |
| `namespace` | TEXT | Namespace (vazio = aplica ao cluster inteiro) |
| `warn_pct` | INTEGER | Percentual de uso para alerta Warning (padrão: 85) |
| `crit_pct` | INTEGER | Percentual de uso para alerta Critical (padrão: 90) |

Constraint UNIQUE em `(cluster, namespace)`. Hierarquia de lookup: `cluster+namespace` → `cluster` → global (`""`, `""`).

#### `webhooks`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGSERIAL PK | Autoincrement |
| `name` | TEXT | Nome descritivo |
| `url` | TEXT | URL destino do HTTP POST |
| `events` | TEXT | Eventos: `critical`, `warning`, `critical,warning` ou `*` |
| `enabled` | INTEGER | 0 ou 1 |

#### `audit_log`

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGSERIAL PK | Autoincrement |
| `timestamp` | TEXT | ISO 8601 UTC |
| `username` | TEXT | Usuário que executou a ação |
| `role` | TEXT | Role do usuário no momento |
| `action` | TEXT | Código da ação (ver seção 19) |
| `detail` | TEXT | Detalhes da ação (alvo, parâmetros) |
| `ip` | TEXT | IP de origem da requisição |

---

## 7. Frontend — Módulos e Abas

### Thresholds de alerta (frontend)

| Recurso | Limiar |
|---------|--------|
| CPU | > 500 millicores (0.5 vCPU) |
| Memória | > 20 GiB |

### Abas disponíveis por perfil

| Aba | Perfil mínimo | Descrição |
|-----|--------------|-----------|
| Monitor | reader | Recursos de pods por namespace |
| Top 10 | reader | Ranking de CPU e memória |
| Histórico | reader | Snapshots históricos com tendências ▲▼ |
| Namespaces | reader | Lista de namespaces com contagem de pods |
| Storage | reader | PVCs e status |
| Orphans / Auditoria | reader | Recursos sem uso |
| Containers | reader | Containers Docker nos nós do cluster |
| Docker / Podman | reader | Daemons externos ao cluster |
| Helm | reader | Releases Helm |
| Deployments | reader | Deployments com status de réplicas |
| Dashboards | reader | Dashboards personalizáveis |
| **Análise** | **reader** | **Varredura de boas práticas (CIS Benchmark)** |
| **Topologia** | **reader** | **Grafo interativo de recursos do cluster** |
| **Cotas** | **reader** | **ResourceQuotas e LimitRanges por namespace** |
| Logs | dev | Logs de containers (acesso restrito ao dev) |
| Nodes | administration | Nodes com CPU/mem alocável vs. uso |
| Admin | administration | Gerenciar clusters, usuários, grupos, webhooks, thresholds, audit log e **Modo NOC** |

### Monitor de Pods

- Seleciona cluster e um ou mais namespaces
- Exibe tabela com: pod, namespace, node, fase, container, imagem, CPU request/limit/usage, Mem request/limit/usage, % Limit
- Células com uso > threshold ficam destacadas em vermelho
- Botão para exportar CSV

### Top 10

- Ranking dos 10 containers com maior consumo de CPU ou de memória
- Alterna entre CPU e Memória
- Dados carregados automaticamente ao abrir a aba

### Histórico

- Snapshots agrupados por `session_id`
- Filtro por cluster e namespace
- Setas ▲▼ indicam tendência em relação à sessão anterior
- Exportação em CSV

### Nodes

- CPU e memória alocável vs. uso real por nó
- Nós com status diferente de `Ready` são destacados

### Storage (PVCs)

- Lista PVCs com capacidade, status, storage class e volume
- PVCs com status diferente de `Bound` indicam problemas

### Orphans (Recursos Órfãos)

Detecta automaticamente:

| Tipo | Critério de orfandade |
|------|-----------------------|
| PVC | Não montado por nenhum pod em execução |
| Service | Sem endpoints ativos |
| ConfigMap | Não referenciado em pods ou deployments |
| Secret | Não referenciado em pods ou deployments |
| Ingress | Aponta para service inexistente |
| ServiceAccount | Não utilizado por nenhum pod |

### Logs

- Exibe logs dos containers em tempo real diretamente pelo painel
- Filtro por namespace e seleção do pod desejado
- Acesso disponível para perfis `dev` e `administration`

### Dashboards

- Área personalizável com widgets arrastáveis e redimensionáveis
- Múltiplos dashboards por usuário, salvos no PostgreSQL
- Botão "＋ Widget" para adicionar painéis do catálogo
- Layout salvo automaticamente por dashboard

### Admin — Clusters

Formulário para adicionar um novo cluster Kubernetes ao monitoramento:

1. Cole o kubeconfig do cluster alvo
2. Clique em Validar → o backend verifica a conectividade
3. Opcionalmente gere um ServiceAccount dedicado (cria o SA, ClusterRole e kubeconfig via API)
4. Clique em Aplicar → o cluster fica disponível nos seletores

**Excluir um cluster:** clique no ícone de exclusão ao lado do cluster. Uma confirmação por senha é exigida antes da remoção para evitar exclusões acidentais. A operação remove o cluster do monitoramento mas **não altera nada no cluster Kubernetes em si**.

### Admin — Usuários e Grupos

- Criar/remover usuários
- Alterar senha, perfil e acesso a clusters/namespaces
- Criar/remover grupos e gerenciar membros
- Habilitar/desabilitar MFA por usuário ou grupo
- Gerar QR codes em massa para grupos com MFA
- Resetar MFA de um usuário específico
- Perfil (role) editável inline na tabela de usuários clicando no badge

### Admin — Thresholds de Alerta

Permite configurar percentuais de Warning e Critical por escopo:

- **Global** (`cluster=""`, `namespace=""`): aplica a todos os clusters que não têm regra própria
- **Por cluster** (`cluster="X"`, `namespace=""`): aplica a todos os namespaces do cluster X
- **Por cluster + namespace**: regra mais específica, tem precedência sobre as anteriores

Hierarquia de resolução (mais específico vence):
```
cluster + namespace  →  cluster  →  global (padrão: warn=85%, crit=90%)
```

### Admin — Webhooks

Notificações HTTP POST disparadas automaticamente quando o dashboard summary detecta alertas. Cada webhook pode ser configurado para receber eventos `critical`, `warning` ou ambos.

**Payload enviado:**
```json
{
  "event": "critical",
  "timestamp": "2026-04-06T10:00:00Z",
  "data": {
    "namespace": "default",
    "pod": "meu-pod-abc",
    "container": "app",
    "resource": "cpu",
    "usagePct": 95.2
  }
}
```

**Botão "Testar"**: disponível em cada webhook cadastrado. Envia imediatamente um payload de teste (`event: "test"`) para a URL configurada e exibe o status HTTP retornado.

### Admin — Log de Auditoria

Tabela com todas as ações administrativas registradas. Exibe: timestamp, usuário, role, ação e IP de origem. Limite de exibição configurável (50 a 1000 entradas).

### Admin — Modo NOC

Seção dedicada ao uso do Pod Monitor em NOCs (Network Operations Centers) ou painéis de TV. Permite habilitar uma rotação automática de clusters e módulos sem intervenção manual.

**Configurações:**

| Opção | Valores | Descrição |
|-------|---------|-----------|
| Habilitar rotação (NOC) | checkbox on/off | Ativa ou desativa o modo NOC |
| Intervalo de troca de cluster | 5 min / 10 min | A cada quanto tempo o cluster ativo avança para o próximo |
| Módulos que alternam | checkboxes | Quais abas ciclam automaticamente: Monitor, Namespaces, Containers |

**Comportamento:**

- A cada **1 minuto** a aba ativa avança para a próxima da lista selecionada
- A cada **N minutos** (5 ou 10) o cluster ativo avança para o próximo e a rotação de módulos recomeça da primeira aba
- Exemplo com 2 clusters, intervalo 5 min e módulos [Monitor, Namespaces, Containers]:

```
00:00 → cluster-1 / Monitor
01:00 → cluster-1 / Namespaces
02:00 → cluster-1 / Containers
03:00 → cluster-1 / Monitor
04:00 → cluster-1 / Namespaces
05:00 → cluster-2 / Monitor   ← troca de cluster
06:00 → cluster-2 / Namespaces
...
```

**Indicador visual:** quando o Modo NOC está ativo, um badge vermelho pulsante **NOC** aparece no cabeçalho da aplicação. O tooltip exibe o intervalo configurado e os módulos selecionados.

**Persistência:** as configurações são salvas no `localStorage` do navegador (`pm_noc`, `pm_noc_interval`, `pm_noc_modules`) e sobrevivem a reloads de página.

> **Dica:** para uso em TV/NOC, acesse a URL em modo tela cheia (F11) e habilite o modo NOC no painel Admin antes de fixar o painel na tela.

---

## 8. Módulo de Análise

### Visão geral

O módulo de Análise varre o cluster sob demanda e retorna achados categorizados por severidade, baseados nas melhores práticas oficiais do Kubernetes e no **CIS Kubernetes Benchmark**.

> **Importante:** A análise é **exclusivamente sob demanda** (botão manual). Ela nunca roda em background para não gerar carga extra no API Server do cluster, especialmente em ambientes grandes.

### Como usar

1. Acesse a aba **Análise**
2. Selecione o cluster (e opcionalmente um namespace para análise focada)
3. Clique em **Analisar Cluster**
4. Aguarde o resultado — o tempo varia de acordo com o tamanho do cluster

### Performance esperada

| Tamanho do ambiente | Tempo estimado |
|---------------------|----------------|
| Pequeno (< 50 pods) | 2–5s |
| Médio (50–200 pods) | 5–15s |
| Grande (200–500 pods) | 15–40s |
| Muito grande (500+ pods) | 40s–2min+ |

### Filtros disponíveis

- **Severidade**: Crítico / Aviso / Dica (ou todos)
- **Categoria**: Segurança / Confiabilidade / Recursos (ou todas)
- **Namespace**: seletor com os namespaces que possuem achados
- **Node**: seletor com todos os nodes do cluster (independente de achados)

Filtros de namespace e node são mutuamente exclusivos — achados de node não pertencem a namespaces.

### Paginação

A lista de achados suporta paginação com 50, 100 ou 200 itens por página. Qualquer alteração de filtro reseta para a página 1.

### Severidades

| Severidade | Significado |
|-----------|-------------|
| **Crítico** | Risco imediato de instabilidade, falha ou comprometimento de segurança. Requer atenção prioritária. |
| **Aviso** | Configuração abaixo do recomendado. Não causa falha imediata mas aumenta o risco operacional. |
| **Dica** | Melhorias opcionais de robustez e observabilidade. |

### Categorias

| Categoria | Foco |
|-----------|------|
| **Segurança** | Permissões excessivas, imagens inseguras, execução como root |
| **Confiabilidade** | Alta disponibilidade, detecção de falhas, probes |
| **Recursos** | Requests, limits, quotas de namespace |

---

### Checks implementados

Os checks são baseados no **CIS Kubernetes Benchmark v1.9** e na documentação oficial do Kubernetes. Cada item abaixo indica a referência de origem.

---

#### Segurança

---

**Imagem sem tag fixa (`:latest` ou sem tag)**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Pod / Container |
| Referência | CIS Benchmark 5.5.1 — *"Ensure that image tags are immutable"* |
| Doc oficial | https://kubernetes.io/docs/concepts/containers/images/#image-names |

Imagens sem tag explícita ou com `:latest` não garantem que o mesmo código será executado em diferentes deploys. Imagens pinadas por digest (`@sha256:...`) são consideradas seguras e não são sinalizadas.

**Correção:** use tags de versão específicas (ex: `nginx:1.25.3`).

---

**Container em modo privilegiado**

| Campo | Valor |
|-------|-------|
| Severidade | Crítico |
| Recurso | Pod / Container |
| Referência | CIS Benchmark 5.2.1 — *"Minimize the admission of privileged containers"* |
| Doc oficial | https://kubernetes.io/docs/concepts/security/pod-security-standards/ |

Containers privilegiados têm acesso quase irrestrito ao host. O Pod Security Standards — perfil `Baseline` e `Restricted` — proíbem esse modo.

**Correção:** remova `securityContext.privileged: true`. Use `capabilities.add` para permissões específicas quando necessário.

---

**Container pode estar rodando como root**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Pod / Container |
| Referência | CIS Benchmark 5.2.6 — *"Minimize the admission of root containers"* |
| Doc oficial | https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted |

Quando `runAsNonRoot` não está definido como `true` e `runAsUser` não está definido ou é `0`, o container pode rodar como root. O Pod Security Standards — perfil `Restricted` — exige `runAsNonRoot: true`.

**Correção:** defina `securityContext.runAsNonRoot: true` e um `runAsUser` não-zero.

---

#### Confiabilidade

---

**Alto número de restarts**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso (≥ 10) / Crítico (≥ 50) |
| Recurso | Pod / Container |
| Referência | https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-restart-policy |

Restarts frequentes indicam crash, OOM ou misconfiguration. Os thresholds (10 e 50) são baseados em práticas amplamente adotadas pela comunidade — não há valor oficial no Kubernetes.

**Correção:** verifique logs com `kubectl logs <pod> --previous`. Ajuste resources e adicione liveness probe.

---

**Pod em fase Failed ou Unknown**

| Campo | Valor |
|-------|-------|
| Severidade | Crítico |
| Recurso | Pod |
| Referência | https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase |

Pods nessas fases não estão servindo tráfego e podem indicar falha persistente ou perda de comunicação com o node.

**Correção:** `kubectl describe pod <nome>` e `kubectl logs <nome> --previous`.

---

**Container em estado CrashLoopBackOff, OOMKilled, ImagePullBackOff ou ErrImagePull**

| Campo | Valor |
|-------|-------|
| Severidade | Crítico |
| Recurso | Pod / Container |
| Referência | https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-states |

Estados de espera críticos indicam falha ativa no container. `CrashLoopBackOff` = container falha repetidamente. `OOMKilled` = limite de memória atingido. `ImagePullBackOff` = imagem inacessível ou inexistente.

---

**Container sem liveness probe**

| Campo | Valor |
|-------|-------|
| Severidade | Dica |
| Recurso | Pod / Container |
| Referência | https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/ |

Sem liveness probe, o Kubernetes não consegue detectar containers travados (deadlock, loop infinito) e não os reinicia automaticamente.

**Correção:** adicione `livenessProbe` com httpGet, tcpSocket ou exec adequado à aplicação.

---

**Container sem readiness probe**

| Campo | Valor |
|-------|-------|
| Severidade | Dica |
| Recurso | Pod / Container |
| Referência | https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/ |

Sem readiness probe, o Kubernetes envia tráfego ao container assim que ele inicia, antes de estar realmente pronto. Isso causa erros durante inicializações lentas ou após rollouts.

**Correção:** adicione `readinessProbe` que verifique se a aplicação está pronta para receber requisições.

---

**Deployment com réplica única**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Deployment |
| Referência | https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#replicas |

Com apenas uma réplica, qualquer falha no node ou no pod causa indisponibilidade total do serviço. Não há tolerância a falhas.

**Correção:** aumente `spec.replicas` para pelo menos 2. Considere também um `PodDisruptionBudget`.

---

**StatefulSet com réplica única**

| Campo | Valor |
|-------|-------|
| Severidade | Dica |
| Recurso | StatefulSet |
| Referência | https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/ |

Pode ser intencional (banco de dados single-node), por isso é apenas uma dica. Se alta disponibilidade for necessária, o StatefulSet precisa de múltiplas réplicas e storage devidamente configurado.

---

#### Recursos

---

**Container sem CPU request**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Pod / Container |
| Referência | CIS Benchmark 5.2.10 — *"Minimize the admission of containers without resource limits"* |
| Doc oficial | https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |

Sem `requests.cpu`, o scheduler não sabe quanto CPU o container precisa e pode alocar o pod em nodes sobrecarregados.

---

**Container sem CPU limit**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Pod / Container |
| Referência | CIS Benchmark 5.2.11 |
| Doc oficial | https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |

Sem `limits.cpu`, o container pode consumir CPU ilimitada e degradar outros workloads no mesmo node (CPU throttling não protege completamente).

---

**Container sem memory request**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Pod / Container |
| Referência | CIS Benchmark 5.2.10 |
| Doc oficial | https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |

Sem `requests.memory`, o scheduler pode alocar o pod em um node sem memória suficiente, resultando em OOMKill.

---

**Container sem memory limit**

| Campo | Valor |
|-------|-------|
| Severidade | **Crítico** |
| Recurso | Pod / Container |
| Referência | CIS Benchmark 5.2.11 |
| Doc oficial | https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |

Sem `limits.memory`, um vazamento de memória pode consumir toda a RAM do node e causar OOMKill em outros pods ou no próprio kubelet. É o único check de recursos com severidade crítica.

---

**Namespace sem ResourceQuota**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Namespace |
| Referência | CIS Benchmark 5.7.1 — *"Create administrative boundaries between resources using namespaces"* |
| Doc oficial | https://kubernetes.io/docs/concepts/policy/resource-quotas/ |

Sem ResourceQuota, um namespace pode consumir recursos ilimitados do cluster, impactando outros times ou aplicações. Namespaces de sistema (`kube-system`, `kube-public`, `kube-node-lease`) são ignorados.

**Correção:** defina `ResourceQuota` com limites de CPU, memória e contagem de objetos.

---

**Namespace sem LimitRange**

| Campo | Valor |
|-------|-------|
| Severidade | Dica |
| Recurso | Namespace |
| Referência | https://kubernetes.io/docs/concepts/policy/limit-range/ |

Sem LimitRange, containers sem requests/limits explícitos ficam sem nenhum valor padrão — exatamente o que gera os checks anteriores. O LimitRange é a rede de segurança que garante que novos deployments sem configuração explícita ainda tenham limites razoáveis.

---

#### Nodes

---

**Node não está Ready**

| Campo | Valor |
|-------|-------|
| Severidade | Crítico |
| Recurso | Node |
| Referência | https://kubernetes.io/docs/concepts/architecture/nodes/#condition |

O node não está respondendo ao control plane. Pode indicar problema no kubelet, na rede, no disco ou nos recursos. Pods no node podem estar inacessíveis.

**Correção:** `kubectl describe node <nome>` e verifique os eventos.

---

**Node com MemoryPressure**

| Campo | Valor |
|-------|-------|
| Severidade | Crítico |
| Recurso | Node |
| Referência | https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/ |

O kubelet detectou que a memória disponível está abaixo do threshold configurado. Pode estar evictando pods para liberar memória.

**Correção:** verifique os workloads no node e considere adicionar nodes ao cluster.

---

**Node com DiskPressure**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Node |
| Referência | https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/ |

O espaço em disco do node está baixo. Pode impedir que novos pods sejam iniciados.

**Correção:** limpe imagens não utilizadas com `docker system prune` ou aumente o disco do node.

---

**Node com PIDPressure**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Node |
| Referência | https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/ |

O número de processos (PIDs) do node está próximo do limite do sistema operacional. Quando o limite é atingido, nenhum novo processo pode ser criado — containers não sobem e o próprio kubelet pode falhar.

**Correção:** verifique processos zumbis ou containers com muitos threads. O limite do sistema pode ser verificado com `cat /proc/sys/kernel/pid_max`.

---

**Node cordonado (unschedulable)**

| Campo | Valor |
|-------|-------|
| Severidade | Aviso |
| Recurso | Node |
| Referência | https://kubernetes.io/docs/concepts/architecture/nodes/#manual-node-administration |

O node foi marcado manualmente como `unschedulable` via `kubectl cordon`. O Kubernetes não agenda novos pods nele, mas os pods existentes continuam rodando. É uma operação intencional durante manutenções, mas um node esquecido nesse estado reduz a capacidade do cluster silenciosamente.

**Correção (se não intencional):** `kubectl uncordon <nome-do-node>`.

---

### Fontes que não são monitoradas

O módulo usa exclusivamente a Kubernetes API e a Metrics API, sem dependências externas. As seguintes verificações **não estão implementadas** por requererem ferramentas externas:

| Verificação | Ferramenta necessária |
|-------------|----------------------|
| CVEs em imagens | Trivy, Grype |
| NetworkPolicy ausente | Análise de tráfego de rede |
| CIS Benchmark de nodes (OS-level) | kube-bench |
| Conformidade com PSA (Pod Security Admission) | Audit logs do API Server |

---

### Manutenção das regras — Acompanhamento do Benchmark

> **Atenção:** as regras implementadas são baseadas em fontes estáveis do Kubernetes e do CIS Benchmark. Elas não se atualizam automaticamente. Quando novas versões do benchmark ou da documentação oficial forem publicadas, as regras devem ser revisadas e o código atualizado manualmente.

#### Referências oficiais a acompanhar

| Fonte | URL | Frequência sugerida de revisão |
|-------|-----|-------------------------------|
| **CIS Kubernetes Benchmark** | https://www.cisecurity.org/benchmark/kubernetes | A cada nova versão (geralmente anual) |
| **Kubernetes Pod Security Standards** | https://kubernetes.io/docs/concepts/security/pod-security-standards/ | A cada release minor do K8s |
| **Kubernetes Node Pressure Eviction** | https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/ | A cada release minor do K8s |
| **Kubernetes Resource Management** | https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ | A cada release minor do K8s |
| **Kubernetes Probes** | https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/ | A cada release minor do K8s |
| **NIST SP 800-190** (Container Security) | https://csrc.nist.gov/publications/detail/sp/800-190/final | A cada revisão do documento |

#### Como identificar o que atualizar no código

Cada bloco de check no arquivo `backend/main.go` (função `handleAnalysis`) contém comentários com a referência de origem no formato:

```go
// [CIS-K8S] 5.2.1: "Minimize the admission of privileged containers"
// Referência: https://kubernetes.io/docs/concepts/security/pod-security-standards/
```

Ao revisar uma nova versão do benchmark, basta buscar o número do controle (ex: `5.2.1`) no código para localizar o check correspondente.

---

## 9. Dashboard

### Visão geral

O Dashboard é uma área personalizável onde o usuário monta visões agregadas com widgets arrastáveis e redimensionáveis (powered by `react-grid-layout`).

### Gerenciamento de dashboards

- O usuário pode criar múltiplos dashboards nomeados
- Os dashboards são salvos por usuário no PostgreSQL (`/api/dashboards/save`)
- É possível renomear e excluir dashboards

### Catálogo de widgets

| Widget | Ícone | Tamanho padrão | Dados exibidos |
|--------|-------|----------------|----------------|
| Resumo de Pods | ⬡ | 4×3 | Running / Pending / Failed / Outros |
| Nodes do Cluster | ◈ | 4×3 | Ready / Not Ready / Total |
| Alertas Ativos | ⚠ | 3×3 | Contagem de containers acima do threshold |
| Releases Helm | ⎈ | 3×3 | Deployed / Failed / Total |
| Docker / Podman | ⬛ | 3×3 | Hosts configurados / Containers running |
| Top CPU | ⚡ | 6×4 | Tabela top-5 containers por % do limite de CPU |
| Top Memória | ◉ | 6×4 | Tabela top-5 containers por % do limite de memória |
| Pods — Pizza | ◔ | 4×4 | Gráfico donut: distribuição de pods por fase |
| Helm — Pizza | ◕ | 4×4 | Gráfico donut: distribuição de releases por status |
| Pods Fora de Execução | ⊘ | 6×4 | Lista de pods não-Running com motivo |
| Top CPU — Barras | ▬ | 6×4 | Gráfico de barras horizontal: top consumidores de CPU |
| Top Mem — Barras | ▭ | 6×4 | Gráfico de barras horizontal: top consumidores de memória |
| CPU — Linha | 〜 | 8×4 | Série temporal de CPU do cluster |
| Memória — Linha | 〰 | 8×4 | Série temporal de memória do cluster |

### Endpoint de dados

`GET /api/dashboard/summary?cluster=X`

Agrega em uma única chamada: contagem de pods por fase, status de nodes, top-5 CPU/mem com percentual relativo ao limite, contagem de alertas, status do Helm e contagem de containers Docker em execução.

---

## 10. Temas Visuais

O tema é selecionado no cabeçalho e persistido em `localStorage`. Cada tema é aplicado via atributo `data-theme` no `<html>` e define variáveis CSS.

| ID | Nome | Cor de destaque |
|----|------|----------------|
| `dark` | Dark | Roxo `#8b5cf6` |
| `light` | Light | Roxo escuro `#7c3aed` |
| `dracula` | Dracula | Rosa `#bd93f9` |
| `nord` | Nord | Azul ártico `#88c0d0` |
| `tokyo` | Tokyo Night | Azul `#7aa2f7` |
| `sop` | Shades of Purple | Amarelo `#FAD000` |
| `cyberpunk` | Cyberpunk 2077 | Amarelo neon `#fff000` |
| `tomorrow` | Tomorrow Night Blue | Azul claro `#64a8ff` |
| `solarized` | Solarized Dark | Azul `#268bd2` |

---

## 11. Docker / Podman — Monitoramento Externo

### Arquitetura de conexão

O backend se conecta aos daemons Docker/Podman via:

- **Unix socket** — para daemons no mesmo nó do pod (`/var/run/docker.sock`, `/run/podman/podman.sock`)
- **TCP** — para daemons remotos ou na máquina host via proxy socat

### Detecção automática de tipo

Na inicialização, o backend inspeciona cada daemon e verifica se algum container possui o label `io.kubernetes.pod.name`. Hosts com esse label são marcados como `cluster=true` e aparecem na aba **Containers**. Os demais aparecem na aba **Docker / Podman**.

### Proxy socat para a máquina host

Quando o backend roda dentro do Minikube, o `hostPath` do socket aponta para o nó da VM, não para a máquina host. Use socat para expor daemons externos:

```bash
# Descobrir o IP da bridge do Minikube (geralmente 192.168.49.1)
ip addr show $(ip route | grep $(minikube ip) | awk '{print $3}') | grep 'inet ' | awk '{print $2}' | cut -d/ -f1
```

#### Serviço systemd para expor o Docker da máquina host

```ini
# /etc/systemd/system/docker-tcp-proxy.service
[Unit]
Description=Socat TCP proxy para Docker da máquina host
After=network.target

[Service]
ExecStart=/usr/bin/socat TCP-LISTEN:2375,bind=192.168.49.1,reuseaddr,fork UNIX-CONNECT:/var/run/docker.sock
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now docker-tcp-proxy.service
```

#### Registrar o novo daemon

Edite `DOCKER_HOSTS` em `k8s/backend.yaml`:

```yaml
env:
  - name: DOCKER_HOSTS
    value: "minikube-node=/var/run/docker.sock,podman=/run/podman/podman.sock,host-docker=tcp://192.168.49.1:2375"
```

Após editar:

```bash
kubectl apply -f k8s/backend.yaml
```

#### Verificar

```bash
curl http://192.168.49.1:2375/v1.41/info
```

---

## 12. Monitoramento Multi-Cluster

### Como adiciona-se um cluster

1. Obtenha o kubeconfig do cluster alvo (EKS, AKS, GKE, on-premises, etc.)
2. Acesse Admin → Clusters no Pod Monitor
3. Cole o kubeconfig e clique em Validar
4. Opcionalmente clique em **Criar SA** — o backend cria automaticamente um ServiceAccount com todas as permissões necessárias (incluindo `resourcequotas` e `limitranges`) e retorna um kubeconfig dedicado
5. Clique em Aplicar — o cluster fica disponível imediatamente

> **Atualizar permissões de um SA existente:** basta clicar em "Criar SA" novamente. O backend detecta o ClusterRole existente e o atualiza com as regras mais recentes (operação idempotente).

### RBAC criado pelo "Criar SA"

O ClusterRole `pod-monitor-role` gerado automaticamente inclui:

| API Group | Recursos |
|-----------|----------|
| `""` (core) | pods, namespaces, nodes, persistentvolumeclaims, services, endpoints, configmaps, secrets, serviceaccounts, **resourcequotas**, **limitranges** |
| `""` (core) | pods/log (get) |
| `apps` | deployments, replicasets, statefulsets, daemonsets |
| `networking.k8s.io` | ingresses |
| `metrics.k8s.io` | pods, nodes |

### Clusters locais

O backend detecta automaticamente o `KUBECONFIG` montado no pod. O cluster local (Minikube, kind, etc.) sempre aparece como primeira opção.

### kind remoto

Por padrão, o kind cria o cluster com o API server escutando apenas em `127.0.0.1`. Para acessar de outra máquina (ou de dentro de um container Docker), crie o cluster com suporte a acesso externo:

```yaml
# kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "0.0.0.0"
  apiServerPort: 34567
kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      certSANs:
        - "192.168.68.126"   # IP da máquina onde o kind roda
        - "localhost"
        - "127.0.0.1"
```

```bash
kind create cluster --config kind-config.yaml
```

O kubeconfig gerado já conterá o IP externo no campo `server:` e o certificado TLS terá o IP como SAN, funcionando de qualquer host na rede.

**Pod Monitor rodando em Docker no Windows:** se o kind estiver na mesma máquina, use `host.docker.internal` no campo `server:` do kubeconfig:

```yaml
server: https://host.docker.internal:34567
```

`host.docker.internal` é resolvido pelo Docker Desktop para o IP real da máquina Windows, permitindo que containers alcancem serviços do host sem depender do IP da interface de rede.

### Clusters EKS / AKS / GKE

São plenamente suportados desde que o kubeconfig tenha conectividade à API server do cluster a partir do pod do backend. Tipicamente:

- **EKS**: exige que o pod tenha acesso à internet ou ao endpoint privado da API. O kubeconfig usa autenticação por token AWS — recomenda-se usar um SA do próprio cluster com kubeconfig estático gerado pelo módulo de criação de SA.
- **AKS**: similar ao EKS, use um SA dedicado com kubeconfig estático.

---

## 13. Documentação de Ajuda In-App

O ícone `?` no cabeçalho abre um painel lateral com documentação filtrada pelo perfil do usuário logado.

### Tópicos disponíveis por perfil

| Tópico | Perfis |
|--------|--------|
| Monitor de Pods | administration, reader |
| Top 10 | administration, reader |
| Histórico | administration, reader |
| Namespaces | administration, reader |
| Storage (PVCs) | administration, reader |
| Auditoria (Órfãos) | administration, reader |
| Containers (Cluster) | administration, reader |
| Docker / Podman | administration, reader |
| Helm | administration, reader |
| Deployments | administration, reader |
| Dashboards | administration, reader |
| **Análise** | **administration, reader** |
| **Topologia** | **administration, reader** |
| **Cotas** | **administration, reader** |
| Logs | administration, reader, dev |
| Nodes | administration |
| Admin — Clusters | administration |
| Admin — Usuários e Grupos | administration |
| Acesso restrito (Dev) | dev |

---

## 14. Desenvolvimento Local

### Backend

```bash
cd backend
go run main.go
# Requer kubeconfig em ~/.kube/config ou variável KUBECONFIG
# Requer PostgreSQL local:
#   docker run -d --name pg -e POSTGRES_USER=podmonitor -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=podmonitor -p 5432:5432 postgres:16-alpine
# Opcional: export JWT_SECRET=qualquercoisa
export DATABASE_URL=postgres://podmonitor:dev@localhost:5432/podmonitor?sslmode=disable
```

### Frontend

```bash
cd frontend
npm install
npm run dev
# Dev server em http://localhost:5173
# Proxy: /api/* → http://localhost:8080
```

A configuração do proxy está em `vite.config.js`:

```js
server: {
  proxy: {
    '/api': 'http://localhost:8080'
  }
}
```

### Lint

```bash
cd frontend
npm run lint
```

### Build de produção

```bash
cd frontend
npm run build        # gera dist/
npm run preview      # serve o build localmente
```

O Nginx do container de produção usa `frontend/nginx.conf` com rewrite SPA (`try_files $uri /index.html`).

---

---

## 15. Topologia

### Visão geral

A aba **Topologia** exibe um grafo interativo SVG com todos os recursos do cluster (ou de um namespace específico) e as relações entre eles. O grafo suporta dois modos de layout e atualização automática via SSE.

### Tipos de nó

| Tipo | Cor | Descrição |
|------|-----|-----------|
| Pod | Azul `#4a9eff` | Pods em execução |
| Service | Laranja `#f59e0b` | Services Kubernetes |
| Deployment | Verde `#10b981` | Workloads Deployment |
| StatefulSet | Ciano `#06b6d4` | Workloads StatefulSet |
| DaemonSet | Verde-limão `#84cc16` | Workloads DaemonSet |
| ReplicaSet | Cinza `#64748b` | ReplicaSets (ocultos por padrão se vazios) |
| Job | Rosa `#f472b6` | Jobs batch |
| CronJob | Lilás `#c084fc` | CronJobs (geradores de Jobs) |
| Ingress | Ciano-claro `#22d3ee` | Ingresses de rede |
| ConfigMap | Roxo `#a78bfa` | ConfigMaps referenciados |
| Secret | Vermelho `#f87171` | Secrets referenciados |
| HPA | Laranja `#fb923c` | HorizontalPodAutoscaler |

O status visual de cada nó é indicado pela cor da borda:
- **Verde** (`ok`) — recurso em estado normal
- **Amarelo** (`warn`) — Pod pending, réplicas indisponíveis, Job em execução, CronJob suspenso, HPA com réplicas abaixo do desejado
- **Vermelho** (`error`) — Pod/Job com falha

### Tipos de aresta (relação)

| Tipo | Linha | Cor | Descrição |
|------|-------|-----|-----------|
| `owns` | Sólida | Verde | Deployment / StatefulSet / DaemonSet / ReplicaSet / Job possui Pod(s) |
| `selects` | Sólida | Laranja | Service seleciona Pods (via label selector) |
| `mounts` | Sólida | Roxo | Pod monta ConfigMap ou Secret como volume |
| `env` | Tracejada | Cinza | Pod referencia ConfigMap ou Secret via `envFrom` / `env.valueFrom` |
| `scales` | Sólida | Laranja | HPA escala o Deployment / StatefulSet alvo |
| `routes` | Sólida | Ciano | Ingress roteia tráfego para Service |
| `spawns` | Tracejada | Lilás | CronJob cria um Job |

### Layout do grafo

O grafo suporta dois modos de layout, alternados pelo botão `⬡`/`◎` no canto superior direito:

| Modo | Ícone | Comportamento |
|------|-------|---------------|
| **Force-directed** (padrão) | `⬡` | Simulação física D3.js — repulsão entre nós, atração por arestas, colisão. Melhor para grafos complexos. |
| **Circular** | `◎` | Nós agrupados por tipo em anéis concêntricos. Mais legível para grafos simples. |

Ao trocar de modo, o layout é recalculado e o grafo é re-centralizado automaticamente.

### Interação

| Ação | Efeito |
|------|--------|
| Arrastar fundo | Move o canvas inteiro (pan) |
| Arrastar nó | Move apenas aquele nó |
| Scroll | Zoom centrado no cursor |
| Clicar nó | Abre painel de detalhes (nome, namespace, conexões, metadados HPA/RS) |
| Botão `+` / `−` | Zoom in / out |
| Botão `⊙` | Re-centraliza o grafo no centro da tela |
| Botão `⬡` / `◎` | Alterna entre layout force-directed e circular |

### Busca de nós

Campo de texto centralizado na parte superior do canvas, com suporte a **regex** (case-insensitive):

- Ao digitar, nós que não batem ficam com opacidade 0.12 (dimmed), destacando os resultados
- Mostra contador `X/Y` com o número de nós que batem vs. total visível
- Suporta regex completa (ex: `nginx`, `^my-app`, `api|worker`)
- Regex inválida usa matching simples por substring como fallback
- Limpar o campo restaura todos os nós à opacidade normal

### Filtro de tipos (legenda interativa)

A legenda na parte inferior-esquerda é clicável:

- Clicar em qualquer tipo de nó (Pod, Service, Deployment, etc.) **oculta/exibe** todos os nós daquele tipo
- Nós ocultos ficam com opacidade reduzida na legenda
- Arestas conectadas a nós ocultos são removidas automaticamente
- **Ocultar ReplicaSets vazios** — checkbox que filtra ReplicaSets com `availableReplicas = 0` (ativo por padrão, mantém o grafo limpo)
- Ao ocultar/exibir tipos, o layout force-directed é recalculado automaticamente

### Filtro de namespace por regex

O seletor de namespace conta com um campo de texto para filtrar os namespaces disponíveis via expressão regular:

- Filtro aplicado em tempo real, case-insensitive
- Suporta regex completa (ex: `prod|stg`, `^kube-`, `monitoring.*`)
- Ao digitar no filtro, o seletor reseta para "Todos" automaticamente
- Namespace inválido (regex com erro de sintaxe) mostra todos os namespaces como fallback

O seletor de namespace é independente da aba Monitor — alterar a seleção aqui não afeta outros tabs.

### Auto-refresh via SSE

O botão **● Live** na barra de controles ativa/desativa a atualização automática do grafo:

- Quando ativo, o frontend conecta ao endpoint `/api/sse/events` e escuta o evento `topology_refresh`
- O backend publica esse evento a cada **30 segundos** para todos os clusters registrados
- Ao receber o evento para o cluster ativo, o grafo é re-buscado silenciosamente (sem spinner)
- O indicador verde pulsante (`●`) confirma que a conexão SSE está ativa
- Auto-refresh só funciona após o carregamento inicial da topologia
- Ao trocar de aba ou desativar o botão, a conexão SSE é encerrada automaticamente

### Painel de detalhes do nó

Ao clicar em qualquer nó, um painel lateral exibe:
- **Kind**, **Name** e **Namespace**
- Número de conexões (arestas incidentes)
- Para nós **HPA**: alvo (`targetKind/targetName`), réplicas correntes, desejadas, mínimas e máximas

### Suporte a HPA

HorizontalPodAutoscalers são automaticamente incluídos no grafo quando presentes no namespace consultado:

- **Nó HPA** exibe réplicas (`currentReplicas / desiredReplicas`) no painel de detalhes
- **Aresta `scales`** conecta o HPA ao seu alvo (Deployment, StatefulSet, etc.)
- Status `warn` quando `currentReplicas < desiredReplicas`

Pré-requisito RBAC — o ClusterRole `pod-monitor-role` deve incluir:
```yaml
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list"]
```

### Pré-requisito RBAC (topologia completa)

O `k8s/rbac.yaml` deve incluir permissões para todos os grupos usados pelo grafo:

```yaml
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list"]
- apiGroups: ["networking.k8s.io"]
  resources: ["ingresses"]
  verbs: ["get", "list"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list"]
```

Se o cluster usar um RBAC desatualizado, os recursos ausentes são ignorados silenciosamente (o grafo exibe o que tiver permissão).

### Endpoint

```
GET /api/topology?cluster=X&namespace=Y
```

Retorna:
```json
{
  "nodes": [
    { "id": "Pod/default/meu-pod",        "kind": "Pod",        "name": "meu-pod",        "namespace": "default", "status": "ok" },
    { "id": "Deployment/default/meu-dep", "kind": "Deployment", "name": "meu-dep",        "namespace": "default", "status": "ok" },
    { "id": "ReplicaSet/default/meu-rs",  "kind": "ReplicaSet", "name": "meu-rs",         "namespace": "default", "status": "ok",
      "meta": { "replicas": "3", "availableReplicas": "3" } },
    { "id": "StatefulSet/default/meu-sts","kind": "StatefulSet","name": "meu-sts",        "namespace": "default", "status": "ok" },
    { "id": "DaemonSet/default/meu-ds",   "kind": "DaemonSet",  "name": "meu-ds",         "namespace": "default", "status": "ok" },
    { "id": "Job/default/meu-job",        "kind": "Job",        "name": "meu-job",        "namespace": "default", "status": "warn" },
    { "id": "CronJob/default/meu-cj",     "kind": "CronJob",    "name": "meu-cj",         "namespace": "default", "status": "ok" },
    { "id": "Ingress/default/meu-ing",    "kind": "Ingress",    "name": "meu-ing",        "namespace": "default", "status": "ok" },
    { "id": "HPA/default/meu-hpa",        "kind": "HPA",        "name": "meu-hpa",        "namespace": "default", "status": "ok",
      "meta": { "minReplicas": "1", "maxReplicas": "10", "currentReplicas": "3", "desiredReplicas": "3",
                "targetKind": "Deployment", "targetName": "meu-dep" } }
  ],
  "edges": [
    { "from": "Deployment/default/meu-dep", "to": "ReplicaSet/default/meu-rs",  "type": "owns" },
    { "from": "ReplicaSet/default/meu-rs",  "to": "Pod/default/meu-pod",        "type": "owns" },
    { "from": "CronJob/default/meu-cj",     "to": "Job/default/meu-job",        "type": "spawns" },
    { "from": "Job/default/meu-job",        "to": "Pod/default/meu-pod",        "type": "owns" },
    { "from": "Ingress/default/meu-ing",    "to": "Service/default/meu-svc",    "type": "routes" },
    { "from": "HPA/default/meu-hpa",        "to": "Deployment/default/meu-dep", "type": "scales" }
  ]
}
```

---

## 16. Cotas de Recursos (Quotas)

### Visão geral

A aba **Cotas** exibe os `ResourceQuota` e `LimitRange` configurados em cada namespace do cluster, com o uso atual vs. o limite definido (hard).

### Pré-requisito RBAC

O ClusterRole `pod-monitor-role` deve incluir permissão de leitura em `resourcequotas` e `limitranges` (já incluído em `k8s/rbac.yaml` desde esta versão):

```yaml
- apiGroups: [""]
  resources: ["resourcequotas", "limitranges"]
  verbs: ["get", "list"]
```

Se o cluster estiver com RBAC desatualizado, a aba exibirá uma mensagem orientando a reaplicar o manifesto:

```bash
kubectl apply -f k8s/rbac.yaml
```

### Estrutura de resposta do `/api/quotas`

```json
[
  {
    "namespace": "default",
    "quotas": [
      {
        "name": "exemplo-quota",
        "resources": {
          "requests.cpu":    { "hard": "2", "used": "250m" },
          "limits.memory":   { "hard": "8Gi", "used": "512Mi" },
          "pods":            { "hard": "20", "used": "3" }
        }
      }
    ],
    "limit_ranges": [
      {
        "name": "exemplo-limitrange",
        "type": "Container",
        "default":  { "cpu": "500m", "memory": "256Mi" },
        "defaultRequest": { "cpu": "100m", "memory": "128Mi" },
        "min":      { "cpu": "50m",  "memory": "64Mi" },
        "max":      { "cpu": "2",    "memory": "2Gi" }
      }
    ]
  }
]
```

### Arquivo de exemplo

O arquivo `k8s/example-quota.yaml` contém um `ResourceQuota` e um `LimitRange` de exemplo para o namespace `default`:

```bash
kubectl apply -f k8s/example-quota.yaml    # criar para testes
kubectl delete -f k8s/example-quota.yaml   # remover após testes
```

---

## 17. Webhooks de Alertas

### Como funciona

O backend dispara um HTTP POST para cada webhook habilitado sempre que o endpoint `/api/dashboard/summary` detecta pods acima do threshold configurado, e também para cada operação registrada no audit log (`events="audit"`). Os disparos são assíncronos (goroutine por URL) e não bloqueiam a resposta ao frontend.

### Retry com backoff exponencial

A partir da v0.4.0, cada disparo de webhook tem até **3 tentativas** com backoff exponencial:

| Tentativa | Aguarda antes da próxima |
|-----------|--------------------------|
| 1ª falha  | 5 segundos |
| 2ª falha  | 30 segundos |
| 3ª falha  | abandona, loga erro |

**Retentar em:** erro de rede, timeout, HTTP 5xx (servidor indisponível).  
**Não retentar em:** HTTP 4xx — URL incorreta, autenticação inválida, recurso não encontrado. Repetir não resolveria o problema.

Cada URL configurada roda em goroutine independente — uma URL lenta não bloqueia as demais.

### Configuração (Admin → Webhooks)

| Campo | Descrição |
|-------|-----------|
| Nome | Identificador legível |
| URL | Endpoint HTTP destino |
| Eventos | `critical`, `warning` ou ambos |
| Ativo | Liga/desliga sem excluir |

### Payload

```json
{
  "event": "critical",
  "timestamp": "2026-04-06T10:00:00Z",
  "data": {
    "namespace": "production",
    "pod": "api-server-abc123",
    "container": "app",
    "resource": "cpu",
    "usagePct": 95.2
  }
}
```

### Payload de teste

```json
{
  "event": "test",
  "timestamp": "2026-04-06T10:00:00Z",
  "data": {
    "message": "Webhook de teste do Pod Monitor",
    "namespace": "default",
    "pod": "exemplo-pod-abc123",
    "container": "app",
    "resource": "cpu",
    "usagePct": 95.2
  }
}
```

### Validação

1. Cadastre a URL (ex: `webhook.site`) em Admin → Webhooks
2. Clique em **Testar** — o backend faz o POST imediatamente e retorna o status HTTP
3. Verifique o payload recebido no destino

### Endpoint de teste

```
POST /api/webhooks/test
{ "url": "https://..." }

→ 200 OK: { "ok": true, "status": 200 }
→ 502 Bad Gateway: erro de conexão
```

---

## 18. Thresholds de Alerta Configuráveis

### Padrão global

Sem configuração, o backend usa:
- Warning: **≥ 85%** do limite
- Critical: **≥ 90%** do limite

### Hierarquia de resolução

```
1. cluster="X" + namespace="Y"   (mais específico)
2. cluster="X" + namespace=""    (todos os namespaces do cluster)
3. cluster=""  + namespace=""    (global — fallback final)
```

### Configuração (Admin → Thresholds)

| Campo | Descrição |
|-------|-----------|
| Cluster | Nome do cluster (vazio = global) |
| Namespace | Namespace (vazio = cluster inteiro) |
| Warn % | Percentual de uso para nível Warning |
| Crit % | Percentual de uso para nível Critical |

Restrição: `warn_pct < crit_pct` e ambos > 0.

### Efeito

Os thresholds configuram tanto os alertas do Dashboard (`DashSummary.Alerts`) quanto o disparo de webhooks. A avaliação ocorre a cada chamada de `/api/dashboard/summary`.

---

## 19. Log de Auditoria

### Visão geral

Todas as ações administrativas executadas no Pod Monitor são registradas automaticamente na tabela `audit_log` do PostgreSQL. O log é exibido em Admin → Auditoria (somente para `administration`).

A partir da v0.4.0, cada entrada do audit log também dispara webhooks configurados com `events="audit"` ou `events="*"` em tempo real, com retry automático (ver [seção 17](#17-webhooks-de-alertas)).

### Ações registradas

| Código | Descrição |
|--------|-----------|
| `user_create` | Novo usuário criado |
| `user_delete` | Usuário removido |
| `password_change` | Senha de usuário alterada |
| `role_change` | Role de usuário alterada (`usuario -> nova_role`) |
| `mfa_setup` | Usuário configurou seu próprio MFA (TOTP) |
| `mfa_enable` | Admin habilitou MFA para um usuário |
| `mfa_disable` | Admin desabilitou MFA para um usuário |
| `mfa_reset` | Admin resetou o MFA de um usuário |
| `group_create` | Novo grupo criado |
| `group_delete` | Grupo removido |
| `webhook_create` | Novo webhook cadastrado |
| `webhook_update` | Webhook atualizado |
| `webhook_delete` | Webhook removido |
| `threshold_save` | Threshold salvo/atualizado (`cluster/ns warn/crit`) |
| `threshold_delete` | Threshold removido |
| `pod_logs` | Logs de pod acessados (`cluster/ns/pod/container`) |
| `cluster_delete` | Cluster removido do monitoramento |

### Estrutura de cada entrada

```json
{
  "timestamp": "2026-04-06T10:00:00Z",
  "username": "admin",
  "role": "administration",
  "action": "role_change",
  "detail": "joao -> reader",
  "ip": "192.168.1.10:54321"
}
```

### Endpoint

```
GET /api/audit?limit=100
```

Retorna as `N` entradas mais recentes (máximo configurável na UI de 50 a 1000).

---

## 20. Atualizações em Tempo Real (SSE)

### Visão geral

O Pod Monitor usa **Server-Sent Events (SSE)** para receber atualizações em tempo real sem necessidade de polling constante. O mesmo endpoint `/api/sse/events` serve tanto o Dashboard quanto a aba de Topologia.

### Como funciona — Dashboard

1. O `DashboardPage` abre uma conexão `EventSource` para `/api/sse/events`
2. A cada chamada ao `/api/dashboard/summary` que detectar alertas, o backend publica o evento `summary`
3. O frontend atualiza os widgets imediatamente
4. Fallback: se o navegador não suportar SSE ou a conexão cair, o frontend usa polling de 5 minutos como backup

### Como funciona — Topologia (auto-refresh)

1. Ao ativar o botão **● Live** na aba Topologia, o frontend abre uma `EventSource` para `/api/sse/events`
2. Um goroutine no backend publica o evento `topology_refresh` a cada **30 segundos** com a lista de clusters registrados
3. Ao receber o evento para o cluster ativo, o frontend re-busca o grafo silenciosamente (sem spinner)
4. A conexão é encerrada ao trocar de aba ou desativar o botão Live

### Eventos publicados

| Evento SSE | Quando | Payload |
|------------|--------|---------|
| `summary` | A cada chamada ao `/api/dashboard/summary` com alertas | Objeto `DashSummary` completo |
| `topology_refresh` | A cada 30 segundos (goroutine periódico) | `{"clusters":["cluster1","cluster2",...]}` |

### Heartbeat

O backend envia um comentário SSE (`: heartbeat`) a cada 30 segundos para manter a conexão ativa através de proxies e firewalls que fecham conexões ociosas.

### Endpoint

```
GET /api/sse/events
Authorization: Bearer <token>

Content-Type: text/event-stream
Cache-Control: no-cache
```

---

## 21. Aviso de Expiração de Sessão

### Comportamento

Quando o JWT do usuário tem **30 minutos ou menos** de validade restante, um banner de aviso aparece entre o cabeçalho e a barra de navegação:

> "Sua sessão expira em breve. Faça login novamente para continuar."

O banner inclui um botão **Renovar** que redireciona para a tela de login.

### Implementação

O frontend decodifica o campo `exp` do JWT (sem biblioteca externa — apenas `atob` + `JSON.parse` no payload base64) e configura um `setInterval` de 1 minuto para verificar a validade.

### Tokens JWT

- Duração padrão: **24 horas**
- Algoritmo: HMAC-SHA256
- Renovação: novo login — não há refresh token

---

## 22. Modo NOC — Rotação Automática

O **Modo NOC** permite que o Pod Monitor funcione como painel autônomo em NOCs, salas de operação ou TVs de monitoramento, alternando automaticamente entre clusters e módulos sem interação manual.

### Ativação

Acesse **Admin → Modo NOC** e habilite a checkbox **"Habilitar rotação automática (NOC)"**.

### Configurações

| Opção | Valores | Comportamento |
|-------|---------|---------------|
| **Habilitar NOC** | on / off | Liga ou desliga toda a automação |
| **Intervalo de cluster** | 5 min ou 10 min | Tempo entre trocas de cluster |
| **Módulos** | Monitor, Namespaces, Containers | Abas que ciclam a cada 1 minuto |

### Lógica de rotação

```
Ciclo de módulos : a cada 1 minuto → próxima aba da lista
Ciclo de cluster : a cada N minutos → próximo cluster + volta ao 1º módulo
```

Exemplo prático com 3 clusters, intervalo = 5 min, módulos = [Monitor, Namespaces, Containers]:

| Tempo | Cluster | Aba |
|-------|---------|-----|
| 00:00 | prod | Monitor |
| 01:00 | prod | Namespaces |
| 02:00 | prod | Containers |
| 03:00 | prod | Monitor |
| 04:00 | prod | Namespaces |
| 05:00 | staging | Monitor |
| 06:00 | staging | Namespaces |
| ... | ... | ... |
| 10:00 | dev | Monitor |

### Badge no header

Quando o modo NOC está ativo, um badge **NOC** vermelho pulsante aparece no cabeçalho. O tooltip exibe: `NOC • Nmin/cluster • módulo1, módulo2, ...`.

### Persistência

As configurações são salvas no `localStorage` do navegador e sobrevivem a reloads de página:

| Chave | Tipo | Descrição |
|-------|------|-----------|
| `pm_noc` | boolean | NOC habilitado |
| `pm_noc_interval` | number | Intervalo de cluster em minutos |
| `pm_noc_modules` | JSON array | Lista de IDs de módulos selecionados |

> Para uso em TV: abra em modo tela cheia (F11), faça login, configure o Modo NOC no Admin e deixe o painel correr.

---

## 23. Segurança

### Medidas implementadas (v0.4.0)

#### Bcrypt cost = 12

O custo do bcrypt foi elevado explicitamente de 10 (padrão Go) para **12** em todos os pontos de geração de hash (criação de usuário, troca de senha, usuário admin padrão e seed de usuários). A constante `bcryptCost = 12` está definida em `backend/main.go`.

#### Container não-root (backend)

O Dockerfile do backend cria um usuário dedicado (`appuser`) e executa o processo como não-root. O securityContext do Kubernetes reforça isso com `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities: drop [ALL]` e `seccompProfile: RuntimeDefault`.

#### Pod Security Standards

O namespace `pod-monitor` tem labels PSS configuradas:

| Label | Valor | Efeito |
|-------|-------|--------|
| `enforce` | `baseline` | Bloqueia containers privilegiados, hostPID, hostNetwork |
| `warn` | `restricted` | Emite avisos para violações do nível mais restritivo |
| `audit` | `restricted` | Registra violações do nível mais restritivo nos logs de auditoria do cluster |

O frontend tem `allowPrivilegeEscalation: false`, `drop [ALL]` e `add [NET_BIND_SERVICE]` (necessário para o nginx bindar a porta 80).

### Medidas implementadas (v0.3.0)

#### Rate Limiting (autenticação)

Os endpoints `/api/auth/login` e `/api/auth/mfa/validate` têm proteção contra força bruta por IP:

| Parâmetro | Valor |
|-----------|-------|
| Tentativas permitidas | 10 em 5 minutos |
| Bloqueio após exceder | 15 minutos |
| Resposta | `429 Too Many Requests` + `Retry-After: 900` |

O IP é extraído do header `X-Real-IP` (enviado pelo Nginx).

#### JWT Blacklist

Tokens são revogados no logout via `POST /api/auth/logout`. A blacklist é mantida em memória e limpa a cada hora. Tokens expirados são removidos automaticamente da lista.

#### TOTP criptografado

Os seeds TOTP são armazenados no banco criptografados com AES-256-GCM. A chave de criptografia é derivada do `JWT_SECRET` via SHA-256. Valores legados sem criptografia são migrados automaticamente no startup (`enc:` prefix distingue cifrados de legados).

#### CORS restritivo

Configure a variável `FRONTEND_ORIGIN` para restringir `Access-Control-Allow-Origin` ao domínio da aplicação. Sem ela, qualquer origin é aceita (adequado apenas para ambientes locais).

#### Headers HTTP de segurança (Nginx)

| Header | Valor |
|--------|-------|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; frame-ancestors 'none'; object-src 'none'` |
| `X-Frame-Options` | `DENY` |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | desabilita câmera, microfone, geolocalização |

#### Docker Socket Proxy (Kubernetes)

O docker.sock **nunca é montado diretamente** no container do backend. Um sidecar `docker-socket-proxy` expõe apenas os endpoints de leitura (`/version`, `/containers/*`) em `tcp://localhost:2375`. Isso elimina o risco de escalação de privilégios via socket do Docker.

#### NetworkPolicy

O arquivo `k8s/network-policy.yaml` restringe o tráfego de rede:
- Backend: aceita apenas de pods `app: frontend` e do namespace `ingress-nginx`
- Frontend: aceita apenas do namespace `ingress-nginx`

Requer CNI compatível (Calico, Cilium ou Weave).

#### Aviso de senha padrão

No startup, o backend verifica se o usuário `admin` ainda usa a senha padrão `admin` e emite um aviso proeminente nos logs.

### Análise completa

A análise detalhada de segurança, comparação com ferramentas de mercado e roadmap estão em `docs/security-analysis.md`.

---

## 24. Paginação de APIs

### Visão geral

Os endpoints `/api/resources` e `/api/nodes` suportam paginação opcional a partir da v0.4.0. O comportamento padrão (sem paginação) é preservado para compatibilidade com o frontend atual.

### Como usar

Adicione o parâmetro `page` à requisição para ativar a paginação:

```
GET /api/resources?namespace=default&page=1&page_size=50
GET /api/nodes?page=1
```

| Parâmetro | Descrição | Padrão | Máximo |
|-----------|-----------|--------|--------|
| `page` | Número da página (base 1) — ativa paginação | — | — |
| `page_size` | Itens por página | 100 | 500 |

### Formato da resposta paginada

```json
{
  "items": [ ...array de pods ou nós... ],
  "total": 1234,
  "page": 1,
  "page_size": 100,
  "total_pages": 13
}
```

Sem o parâmetro `page`, a resposta continua sendo um array JSON simples (comportamento original).

---

## 25. Documentação OpenAPI / Swagger UI

### Spec OpenAPI 3.0

A partir da v0.4.0, o backend serve a especificação OpenAPI 3.0 completa de todos os endpoints:

```
GET /openapi.yaml
```

O arquivo é embutido no binário via `//go:embed openapi.yaml` — sem dependência de arquivos externos em runtime. Pode ser importado diretamente no Postman, Insomnia, ou qualquer ferramenta compatível.

### Swagger UI

Interface visual interativa disponível em:

```
GET /docs
```

Carrega o Swagger UI via CDN e aponta para `/openapi.yaml`. Permite explorar, testar e entender todos os endpoints sem sair do navegador.

### Cobertura

A spec cobre todos os 40+ endpoints organizados em 11 tags:

| Tag | Endpoints |
|-----|-----------|
| Health | `/healthz`, `/openapi.yaml`, `/docs` |
| Auth | Login, logout, MFA (setup, validar, reset) |
| Admin | Usuários, grupos, clusters, docker hosts |
| Clusters | Listar clusters e namespaces |
| Resources | Pods, nós, storage, logs, análise, quotas, topologia |
| Docker | Hosts Docker/Podman externos e containers |
| Helm | Releases e Deployments |
| History | Snapshots históricos e exportação CSV |
| Dashboard | Resumo, timeseries e layouts salvos |
| Audit | Audit log, webhooks e thresholds |
| SSE | Server-Sent Events |

---

## 26. Changelog

### v0.4.0 — 2026-07-03

#### Novas funcionalidades
- **Webhook retry** — disparo de webhooks com 3 tentativas e backoff exponencial (5s → 30s → 2min). HTTP 4xx não retenta; HTTP 5xx e erros de rede retentam automaticamente.
- **Audit log → webhook** — cada operação registrada no audit log agora dispara webhooks configurados com `events="audit"` em tempo real.
- **Paginação de APIs** — `/api/resources` e `/api/nodes` aceitam `?page=N&page_size=N`; sem o parâmetro, comportamento original é preservado.
- **OpenAPI 3.0** — spec completa em `GET /openapi.yaml` (embutida no binário via `go:embed`); Swagger UI interativo em `GET /docs`.
- **Health check real** — `/healthz` verifica conectividade com o PostgreSQL (`db.PingContext`) e reporta número de clusters registrados. Retorna HTTP 503 quando o DB está inacessível.

#### Segurança
- **Bcrypt cost = 12** — elevado de 10 (padrão Go) para 12 em todos os pontos de geração de hash.
- **Container não-root** — Dockerfile do backend cria `appuser` e executa como não-root; `securityContext` adicionado nos manifests Kubernetes.
- **Pod Security Standards** — namespace `pod-monitor` com `enforce: baseline`, `warn: restricted`, `audit: restricted`.

#### Infraestrutura
- Repositório publicado no GitHub: `github.com/wwrmaia/pod-monitor`
- Imagens Docker Hub: `wwrmaia/pod-monitor-backend:0.4.0`, `wwrmaia/pod-monitor-frontend:0.3.1`
- Helm chart OCI: `oci://registry-1.docker.io/wwrmaia/pod-monitor:0.4.0`

### v0.3.1 — 2026-05-21

#### Segurança
- Rate limiting nos endpoints de autenticação (10 tentativas / 5 min por IP, bloqueio de 15 min)
- JWT blacklist — logout revoga token imediatamente
- TOTP criptografado com AES-256-GCM
- CORS restritivo via `FRONTEND_ORIGIN`
- Headers HTTP de segurança no Nginx (CSP, X-Frame-Options, etc.)
- NetworkPolicy Kubernetes
- Docker socket proxy sidecar (nunca monta docker.sock no container principal)
- Aviso de senha padrão no startup

---

## Apêndice — Estrutura de arquivos

```
pod-monitor/
├── backend/
│   ├── main.go              # Servidor Go único — toda a lógica do backend
│   ├── openapi.yaml         # Spec OpenAPI 3.0 (embutida no binário via go:embed)
│   ├── go.mod / go.sum
│   └── Dockerfile           # Multi-stage build (não-root: appuser)
├── frontend/
│   ├── src/
│   │   ├── App.jsx          # Componente React único — toda a UI
│   │   ├── App.css          # Estilos globais + temas
│   │   └── i18n.js          # Strings PT/EN — useT(lang) hook
│   ├── nginx.conf           # Config Nginx para produção (SPA routing)
│   ├── vite.config.js       # Proxy /api/* → backend
│   ├── package.json
│   └── Dockerfile           # Multi-stage: build Vite + Nginx
├── k8s/
│   ├── namespace.yaml
│   ├── rbac.yaml            # ClusterRole com resourcequotas + limitranges
│   ├── postgres.yaml        # Secret, PVC, Deployment e Service do PostgreSQL
│   ├── backend.yaml         # Deployment + Service (+ sidecar docker-socket-proxy)
│   ├── frontend.yaml
│   ├── ingress.yaml         # TLS e security headers via nginx ingress
│   ├── network-policy.yaml  # NetworkPolicy (requer CNI compatível)
│   └── example-quota.yaml   # ResourceQuota + LimitRange de exemplo (testes)
├── helm/
│   └── pod-monitor/         # Helm chart multi-ambiente (v0.4.0)
├── scripts/
│   └── migrate-sqlite-to-postgres.sh  # Migração de dados SQLite → PostgreSQL
├── docs/
│   ├── docker-host-access.md          # Guia socat proxy
│   ├── security-analysis.md           # Análise de segurança e roadmap
│   └── documentacao-completa.md       # Este arquivo
├── docker-compose.yml               # Stack local com k3s embutido (uso com deploy.sh)
├── docker-compose.hub.yml           # Instalação via Docker Hub (PostgreSQL + backend + frontend)
├── deploy.sh                        # Script de deploy automatizado (k3s + registry local)
└── CLAUDE.md                        # Instruções para o assistente IA (local only, não versionado)
```
