# Pod Monitor

Dashboard de monitoramento de infraestrutura Kubernetes (estilo Grafana) que consolida em uma única interface: uso de CPU/memória de pods, saúde de nodes, PVCs e recursos órfãos, containers Docker/Podman (do cluster e de hosts externos), releases Helm, topologia de recursos, análise de boas práticas (CIS Kubernetes Benchmark) e dashboards personalizáveis — com suporte a múltiplos clusters, autenticação com MFA e controle de acesso por perfil.

📖 **Documentação completa:** [`docs/documentacao-completa.md`](docs/documentacao-completa.md)

## Principais funcionalidades

- Monitoramento de recursos por namespace: requests, limits e uso real de CPU/memória
- **Topologia** — grafo interativo dos recursos do cluster (Pods, Services, Deployments, HPAs, Ingress, etc.), com busca por regex, filtros por tipo e **exportação para draw.io**
- **Análise** — varredura sob demanda de boas práticas baseada no CIS Kubernetes Benchmark
- Multi-cluster, com controle de acesso por cluster/namespace por usuário ou grupo
- Autenticação JWT com MFA (TOTP), grupos e perfis (`administration`, `reader`, `dev`)
- Histórico de snapshots, exportação CSV, dashboards com widgets arrastáveis
- Webhooks de alerta com retry e backoff exponencial
- Modo NOC — rotação automática de cluster/módulo para painéis de TV
- Monitoramento de containers Docker/Podman, inclusive daemons externos ao cluster

## Tecnologias

| Camada | Tecnologia |
|--------|-----------|
| Backend | Go, `k8s.io/client-go`, PostgreSQL |
| Frontend | React 19, Vite, Recharts, react-grid-layout |
| Autenticação | JWT (HMAC-SHA256) + MFA TOTP |
| Infraestrutura | Kubernetes (Helm chart incluso), Docker Compose |

## Instalação rápida

### Kubernetes (Helm)

```bash
helm install pod-monitor helm/pod-monitor -n pod-monitor --create-namespace
```

Values por ambiente disponíveis em `helm/pod-monitor/values-{kind,k3s,aks,eks,openshift}.yaml`.

### Docker Compose (sem Kubernetes para hospedar a própria aplicação)

```bash
curl -O https://raw.githubusercontent.com/wwrmaia/pod-monitor/main/docker-compose.hub.yml
docker compose -f docker-compose.hub.yml up -d
```

Acesso em `http://localhost:3000`. Clusters Kubernetes a monitorar são adicionados depois, pela interface de administração.

### Desenvolvimento local

```bash
# Backend (porta 8080, requer kubeconfig + PostgreSQL)
cd backend && go run main.go

# Frontend (porta 5173, proxy /api/* → localhost:8080)
cd frontend && npm install && npm run dev
```

Usuário padrão: `admin` / `admin` (troque a senha no primeiro acesso).

## Imagens

Publicadas no Docker Hub: [`wwrmaia/pod-monitor-backend`](https://hub.docker.com/r/wwrmaia/pod-monitor-backend) e [`wwrmaia/pod-monitor-frontend`](https://hub.docker.com/r/wwrmaia/pod-monitor-frontend).

## Documentação

Para arquitetura detalhada, referência completa da API, schema do banco, RBAC, checks de análise implementados e changelog, veja [`docs/documentacao-completa.md`](docs/documentacao-completa.md).
