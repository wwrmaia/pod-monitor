# Pod Monitor — Análise de Segurança e Plano de Melhorias

**Data:** 2026-05-21  
**Versão analisada:** Helm 0.2.0 / backend main.go ~4.800 LOC / frontend App.jsx ~4.900 LOC

---

## 1. Resumo Executivo

O Pod Monitor é uma plataforma de observabilidade Kubernetes auto-hospedada com RBAC próprio, MFA/TOTP e suporte multi-cluster. A análise identificou **3 vulnerabilidades críticas**, **4 altas**, **6 médias** e **4 baixas**.

O projeto está em estado adequado para uso em redes internas controladas. Para exposição em redes corporativas ou produção com acesso externo, as melhorias críticas e altas devem ser aplicadas primeiro.

---

## 2. Vulnerabilidades Identificadas

### 2.1 Críticas

#### C-01 — TLS não enforçado por padrão
- **Localização:** `k8s/ingress.yaml`, `frontend/nginx.conf`
- **Problema:** Todo o tráfego trafega em HTTP por padrão. Tokens JWT e credenciais são enviados em plaintext.
- **Impacto:** Interceptação de credenciais em qualquer rede não criptografada.
- **Correção:** Adicionar anotações `force-ssl-redirect` no ingress e configurar cert-manager para TLS automático.
- **Status:** ✅ Parcialmente corrigido (anotações e instruções adicionadas em `k8s/ingress.yaml`)

#### C-02 — Docker socket montado sem restrições
- **Localização:** `k8s/backend.yaml` (volumes `docker-sock`, `podman-sock`)
- **Problema:** O arquivo `/var/run/docker.sock` era montado diretamente no pod do backend. Qualquer comprometimento do processo backend permite spawn de containers privilegiados no host.
- **Impacto:** Escalação de privilégios para root no nó Kubernetes.
- **Correção:** Sidecar `docker-socket-proxy` adicionado a `k8s/backend.yaml` e ao Helm chart (`backend.docker.socketProxy.enabled=true`). O docker.sock é montado apenas no sidecar; o backend acessa via `tcp://localhost:2375`. Apenas endpoints `VERSION` e `CONTAINERS` são permitidos. No Helm, ativar com `backend.docker.socketProxy.enabled: true`.
- **Status:** ✅ Corrigido em `k8s/backend.yaml` e `helm/pod-monitor/`

#### C-03 — Kubeconfigs armazenados em etcd sem criptografia garantida
- **Localização:** `k8s/rbac.yaml` (Role com `create/update/patch` em Secrets)
- **Problema:** Kubeconfigs de clusters remotos são gravados em Kubernetes Secrets. Se o etcd não tiver encryption-at-rest habilitado, todos os tokens de acesso ficam expostos.
- **Impacto:** Acesso não autorizado a todos os clusters monitorados.
- **Correção:** Habilitar [encryption at rest no etcd](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/) e usar [External Secrets Operator](https://external-secrets.io/) para gerenciamento de segredos.
- **Status:** ⚠️ Pendente (depende de configuração do cluster de destino)

---

### 2.2 Altas

#### A-01 — Sem rate limiting nos endpoints de autenticação
- **Localização:** `backend/main.go` — `handleLogin`, `handleMFAValidate`
- **Problema:** Não havia limitação de tentativas por IP. Um atacante podia tentar senha de forma ilimitada.
- **Impacto:** Brute-force de credenciais.
- **Correção:** Rate limiter implementado: 10 tentativas por IP em janela de 5 min, bloqueio de 15 min. Retorna HTTP 429 com header `Retry-After`.
- **Status:** ✅ Corrigido em `backend/main.go`

#### A-02 — Senha padrão "admin" sem aviso de startup visível
- **Localização:** `backend/main.go` — `initAuth()`
- **Problema:** O usuário admin é criado com senha "admin" se não houver nenhum admin no banco. O aviso no log era sutil.
- **Impacto:** Instalações novas ficam abertas com credencial conhecida.
- **Correção:** Aviso proeminente com borda no log de startup; verificação também feita após carga do DB.
- **Status:** ✅ Corrigido em `backend/main.go`

#### A-03 — Sem NetworkPolicy (acesso irrestrito ao backend)
- **Localização:** `k8s/` (ausência de NetworkPolicy)
- **Problema:** Qualquer pod no cluster pode alcançar o backend diretamente na porta 8080, ignorando o ingress.
- **Impacto:** Bypass de autenticação não é possível (JWT valida), mas expõe a superfície de ataque a pods comprometidos.
- **Correção:** NetworkPolicy criada restringindo ingress do backend a frontend + ingress-nginx.
- **Status:** ✅ Corrigido em `k8s/network-policy.yaml`

#### A-04 — JWT sem revogação (sem logout invalidante)
- **Localização:** `backend/main.go` — sem endpoint de logout com blacklist
- **Problema:** Tokens JWT têm validade de 24h. Após logout, o token permanece válido até expirar.
- **Impacto:** Token capturado pode ser usado até 24h após logout ou troca de senha.
- **Correção:** Blacklist de tokens em memória (`tokenBlacklist`). Endpoint `POST /api/auth/logout` revoga o token atual. Goroutine `cleanTokenBlacklist()` remove entradas expiradas de hora em hora. `validateToken()` rejeita tokens revogados.
- **Status:** ✅ Corrigido em `backend/main.go`

---

### 2.3 Médias

#### M-01 — Sem headers de segurança HTTP no frontend
- **Localização:** `frontend/nginx.conf`
- **Problema:** Nenhum header de segurança HTTP era enviado (CSP, X-Frame-Options, etc).
- **Impacto:** Clickjacking, XSS via injeção de conteúdo.
- **Correção:** Headers adicionados: `Content-Security-Policy`, `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`.
- **Status:** ✅ Corrigido em `frontend/nginx.conf`

#### M-02 — Segredos TOTP armazenados em SQLite plaintext
- **Localização:** `backend/main.go` — tabela `users`, coluna `totp_secret`
- **Problema:** Seeds TOTP dos usuários ficam em texto claro no arquivo SQLite.
- **Impacto:** Comprometimento do PVC expõe todos os seeds MFA, permitindo bypass de 2FA.
- **Correção:** AES-256-GCM com chave derivada de `JWT_SECRET + "totp-key-v1"` via SHA-256. Valores cifrados têm prefixo `enc:`. Migração automática de registros plaintext legados no startup via `migratePlaintextTOTP()`. Em memória os seeds ficam decifrados; apenas o DB armazena cifrado.
- **Status:** ✅ Corrigido em `backend/main.go`

#### M-03 — Sem audit log persistente para operações sensíveis
- **Localização:** `backend/main.go` — operações de deleção de cluster, alteração de usuários
- **Problema:** O `auditLog()` existe mas grava no DB sem exportação para sistema externo.
- **Impacto:** Sem trilha auditável para compliance ou forense.
- **Correção:** `auditLog()` agora dispara `triggerWebhooks("audit", ...)` em goroutine após cada gravação no DB. Webhooks configurados com `events="audit"` ou `events="*"` recebem notificação em tempo real de cada operação sensível (criação/deleção de usuário, mudança de senha, deleção de cluster, alteração de role, setup/reset de MFA, etc.). O payload inclui `username`, `role`, `action`, `detail` e `ip`.
- **Status:** ✅ Corrigido em `backend/main.go`

#### M-04 — CORS permissivo (`Access-Control-Allow-Origin: *`)
- **Localização:** `backend/main.go` — `corsMiddleware()`
- **Problema:** O CORS aceita requisições de qualquer origem.
- **Impacto:** Requisições cross-origin podem ser feitas de qualquer site (mitigado pelo JWT, mas aumenta superfície de ataque).
- **Correção:** Variável de ambiente `FRONTEND_ORIGIN`. Se definida, apenas requisições com `Origin:` igual ao valor configurado recebem o header CORS. Aviso no startup se não configurada. Para produção: definir `FRONTEND_ORIGIN=https://pod-monitor.seudominio.com`.
- **Status:** ✅ Corrigido em `backend/main.go`

#### M-05 — `imagePullPolicy: Never` no manifest raw
- **Localização:** `k8s/backend.yaml`
- **Problema:** Não funciona em clusters remotos ou após pull de nova imagem.
- **Impacto:** Impossibilita deploy em ambientes diferentes do desenvolvimento local.
- **Correção:** Usar `IfNotPresent` com registry configurado. Helm chart já está correto.
- **Status:** ⚠️ Pendente (mantido para dev local; usar Helm em produção)

#### M-06 — Sem Pod Security Standards
- **Localização:** `k8s/namespace.yaml`
- **Problema:** Namespace sem label de Pod Security Standard.
- **Impacto:** Pods podem rodar com capacidades desnecessárias.
- **Correção:** Labels PSS adicionadas em `k8s/namespace.yaml`: `enforce: baseline` (bloqueia containers privilegiados, hostPID, hostNet), `warn: restricted` e `audit: restricted` (gera avisos e logs para violações do nível mais restritivo sem bloquear). `securityContext` adicionado nos containers: backend com `runAsNonRoot`, `readOnlyRootFilesystem`, `capabilities: drop ALL`, `seccompProfile: RuntimeDefault`; frontend com `allowPrivilegeEscalation: false`, `drop ALL + add NET_BIND_SERVICE`. Dockerfile do backend atualizado para rodar como usuário não-root (`appuser`).
- **Status:** ✅ Corrigido em `k8s/namespace.yaml`, `k8s/backend.yaml`, `k8s/frontend.yaml`, `backend/Dockerfile`

---

### 2.4 Baixas

#### B-01 — Bcrypt cost não explicitamente documentado
- **Localização:** `backend/main.go` — `bcrypt.DefaultCost`
- **Problema:** `DefaultCost` é 10; recomendado ≥ 12 para produção.
- **Correção:** Constante `bcryptCost = 12` definida explicitamente, substituindo `bcrypt.DefaultCost` nos 4 pontos de geração de hash (admin padrão, criação de usuário, troca de senha, seed de usuários).
- **Status:** ✅ Corrigido em `backend/main.go`

#### B-02 — Sem paginação nas APIs de recursos
- **Localização:** `backend/main.go` — `handleResources`, `handleNodes`
- **Problema:** Retorna todos os pods/nós em uma única resposta. Em clusters com 500+ pods, causa latência e uso de memória elevados.
- **Correção:** Paginação opt-in via query params `page` e `page_size` (máx 500, padrão 100). Sem `page`, o comportamento original é preservado (backward-compatible). Com `page`, a resposta é `{"items":[...],"total":N,"page":N,"page_size":N,"total_pages":N}`. Implementado com função genérica `paginate[T any]()`. Exemplos: `GET /api/resources?namespace=default&page=1&page_size=50`, `GET /api/nodes?page=2`.
- **Status:** ✅ Corrigido em `backend/main.go`

#### B-03 — Sem OpenAPI/Swagger
- **Localização:** `backend/main.go`
- **Problema:** 40+ endpoints sem documentação formal. Dificulta auditoria e integração.
- **Correção:** Spec OpenAPI 3.0 completa em `backend/openapi.yaml` (embutida no binário via `//go:embed`). Todos os 40+ endpoints documentados com parâmetros, schemas de request/response, roles e exemplos. Disponível em `GET /openapi.yaml`. Swagger UI interativo em `GET /docs` (carrega da CDN unpkg).
- **Status:** ✅ Corrigido em `backend/main.go` + `backend/openapi.yaml`

#### B-04 — `/healthz` não verifica conectividade
- **Localização:** `backend/main.go` — handler `/healthz`
- **Problema:** Retorna 200 independente do estado do DB ou da conexão Kubernetes.
- **Correção:** `handleHealthz` verifica DB via `db.PingContext()` (se configurado) e reporta número de clusters registrados. Retorna JSON `{"status":"ok|degraded","db":"ok|error:...","clusters":N}` com HTTP 200 (saudável) ou 503 (DB inacessível).
- **Status:** ✅ Corrigido em `backend/main.go`

---

## 3. Comparação com Ferramentas de Mercado

| Dimensão | Pod Monitor | Lens | k9s | Rancher | Grafana+Prometheus | Datadog |
|----------|------------|------|-----|---------|-------------------|---------|
| Tipo | Web dashboard | IDE desktop | CLI | Plataforma | Stack observ. | SaaS APM |
| Multi-cluster | ✅ Nativo | ✅ | ❌ | ✅ | ✅ federation | ✅ |
| RBAC próprio + MFA | ✅ | ❌ | ❌ | ✅ | ✅ (Grafana) | ✅ |
| Histórico integrado | ✅ SQLite 7d | ❌ | ❌ | ❌ | ✅ TSDB ilimitado | ✅ |
| Docker/Podman | ✅ Nativo | ❌ | ❌ | ❌ | Parcial | ✅ |
| Topology graph | ✅ D3 | ✅ | ❌ | ✅ | ❌ | ✅ |
| AlertManager | Básico (%) | ❌ | ❌ | ✅ | ✅ AlertManager | ✅ |
| Custo | Open source | Freemium | Open source | Open source | Open source | $$$$ |
| Instalação | Simples | Muito simples | Muito simples | Complexa | Complexa | Simples |
| Rastreamento APM | ❌ | ❌ | ❌ | ❌ | Parcial (Tempo) | ✅ |

**Vantagens competitivas do Pod Monitor:**
- Único com RBAC próprio + MFA em instalação simples (single `kubectl apply`)
- Suporte nativo a Docker/Podman no mesmo painel
- Interface em Português (PT-BR)
- Dashboard customizável drag-and-drop sem configuração de datasource

**Lacunas vs. mercado:**
- Retenção de histórico limitada (SQLite 7d vs. Prometheus com anos)
- Alertas apenas por CPU/mem%; Grafana tem alertas compostos e routing (PagerDuty, Slack)
- Sem APM/tracing distribuído (Jaeger, Zipkin)
- Escalabilidade limitada pelo SQLite (sem HA nativa)

---

## 4. Plano de Melhorias — Roadmap

### Curto Prazo (aplicadas nesta sessão)
| # | Melhoria | Arquivo | Status |
|---|----------|---------|--------|
| 1 | Rate limiting no login/MFA | `backend/main.go` | ✅ |
| 2 | Aviso proeminente de senha padrão | `backend/main.go` | ✅ |
| 3 | CSP + Security Headers no nginx | `frontend/nginx.conf` | ✅ |
| 4 | Anotações de segurança + TLS no ingress | `k8s/ingress.yaml` | ✅ |
| 5 | NetworkPolicy backend e frontend | `k8s/network-policy.yaml` | ✅ |
| 6 | Documentação de risco do docker.sock | `k8s/backend.yaml` | ✅ |

### Médio Prazo (aplicadas em 2026-05-21)
| # | Melhoria | Esforço | Status |
|---|----------|---------|--------|
| 7 | JWT blacklist/revogação no logout (`/api/auth/logout`) | Médio | ✅ |
| 8 | Criptografia AES-256-GCM dos seeds TOTP | Médio | ✅ |
| 9 | CORS restritivo via `FRONTEND_ORIGIN` env var | Baixo | ✅ |
| 10 | Socket proxy sidecar Docker (k8s + Helm chart) | Médio | ✅ |
| 11 | Encryption at rest no etcd | Documentação | ⚠️ (cluster admin) |

### Longo Prazo (refatoração arquitetural)
| # | Melhoria | Esforço |
|---|----------|---------|
| 12 | Dividir `backend/main.go` em packages | Alto |
| 13 | Migrar frontend para TypeScript | Alto |
| 14 | Substituir SQLite por PostgreSQL/TimescaleDB | Alto |
| 15 | Adicionar testes unitários e de integração | Alto |
| 16 | Gerar OpenAPI spec (swaggo/swag) | Médio |
| 17 | Paginação nas APIs de recursos | Médio |
| 18 | Pod Security Standards no namespace | Baixo |
| 19 | `/healthz` com verificação de dependências | Baixo |
| 20 | Exportar métricas Prometheus do próprio app | Médio |

---

## 5. Comandos de Referência

### Habilitar TLS com cert-manager

```bash
# Instalar cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml

# Criar ClusterIssuer para Let's Encrypt
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: seu-email@dominio.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
EOF

# Atualizar ingress.yaml: descomentar bloco TLS e adicionar anotação cert-manager
```

### Rotacionar JWT Secret

```bash
kubectl create secret generic pod-monitor-secrets \
  --from-literal=jwt-secret=$(openssl rand -base64 32) \
  -n pod-monitor \
  --dry-run=client -o yaml | kubectl apply -f -

# Reiniciar o backend para carregar o novo secret
kubectl rollout restart deployment/backend -n pod-monitor
```

### Verificar NetworkPolicy

```bash
# Testar que pod externo não alcança o backend diretamente
kubectl run test --image=busybox -it --rm -- wget -qO- http://backend-svc.pod-monitor.svc.cluster.local:8080/healthz
```

### Habilitar encryption at rest no etcd (K8s padrão)

```bash
# Documentação oficial:
# https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/
```

---

## 6. Referências

- [OWASP Kubernetes Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Kubernetes_Security_Cheat_Sheet.html)
- [CIS Kubernetes Benchmark](https://www.cisecurity.org/benchmark/kubernetes)
- [NSA/CISA Kubernetes Hardening Guide](https://media.defense.gov/2022/Aug/29/2003066362/-1/-1/0/CTR_KUBERNETES_HARDENING_GUIDANCE_1.2_20220829.PDF)
- [NIST SP 800-190 — Application Container Security](https://csrc.nist.gov/publications/detail/sp/800-190/final)
