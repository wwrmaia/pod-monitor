#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# Pod Monitor — Deploy via Docker Compose (k3s embedded)
# Uso: ./deploy.sh [--rebuild] [--down]
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

REGISTRY="localhost:5000"
K3S_CTR="pm-k3s"
KUBECTL="docker exec ${K3S_CTR} kubectl"
NAMESPACE="pod-monitor"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}→${NC} $*"; }
success() { echo -e "${GREEN}✓${NC} $*"; }
warn()    { echo -e "${YELLOW}!${NC} $*"; }
die()     { echo -e "${RED}✗ ERRO:${NC} $*"; exit 1; }

# ── Flags ─────────────────────────────────────────────────────────────────────
REBUILD=false
DOWN=false
for arg in "$@"; do
  case $arg in
    --rebuild) REBUILD=true ;;
    --down)    DOWN=true ;;
  esac
done

# ── Teardown ──────────────────────────────────────────────────────────────────
if $DOWN; then
  info "Removendo stack..."
  docker compose down -v
  success "Stack removida."
  exit 0
fi

echo ""
echo -e "${CYAN}╔══════════════════════════════════════════╗"
echo -e "║     Pod Monitor — Docker Compose Deploy  ║"
echo -e "╚══════════════════════════════════════════╝${NC}"
echo ""

# ── Pré-requisitos ────────────────────────────────────────────────────────────
command -v docker  >/dev/null 2>&1 || die "Docker não encontrado."
command -v openssl >/dev/null 2>&1 || die "openssl não encontrado."
docker compose version >/dev/null 2>&1 || die "Docker Compose plugin não encontrado."

# ── 1. Registro local ─────────────────────────────────────────────────────────
info "Iniciando registro local de imagens..."
docker compose up -d registry
# Aguarda o registro estar pronto
until curl -sf http://localhost:5000/v2/ >/dev/null 2>&1; do sleep 1; done
success "Registro local pronto em localhost:5000"

# ── 2. Build das imagens ──────────────────────────────────────────────────────
build_and_push() {
  local name=$1 ctx=$2
  if $REBUILD || ! docker image inspect "${REGISTRY}/${name}:latest" >/dev/null 2>&1; then
    info "Build: ${name}..."
    docker build -t "${REGISTRY}/${name}:latest" "${ctx}"
  else
    info "Imagem ${name} já existe (use --rebuild para forçar re-build)"
  fi
  info "Push: ${name} → registro local..."
  docker push "${REGISTRY}/${name}:latest"
}

build_and_push "pod-monitor-backend"  "./backend"
build_and_push "pod-monitor-frontend" "./frontend"
success "Imagens publicadas no registro local."

# ── 3. Iniciar k3s ────────────────────────────────────────────────────────────
info "Iniciando k3s..."
docker compose up -d k3s

info "Aguardando k3s ficar pronto (pode levar até 2 min na primeira execução)..."
TRIES=0
until docker exec "${K3S_CTR}" kubectl get nodes 2>/dev/null | grep -q " Ready"; do
  TRIES=$((TRIES+1))
  [ $TRIES -gt 48 ] && die "Timeout aguardando k3s. Verifique: docker logs ${K3S_CTR}"
  printf "."
  sleep 5
done
echo ""
success "k3s pronto!"

# ── 4. Aplicar manifests ──────────────────────────────────────────────────────
apply() {
  docker exec -i "${K3S_CTR}" kubectl apply -f - < "$1"
}

info "Aplicando manifests Kubernetes..."
apply k8s/namespace.yaml
apply k8s/rbac.yaml
apply k8s/pvc.yaml

# JWT Secret (cria ou atualiza sem sobrescrever se já existir)
if ! $KUBECTL get secret pod-monitor-secrets -n "${NAMESPACE}" >/dev/null 2>&1; then
  JWT_SECRET=$(openssl rand -base64 32)
  $KUBECTL create secret generic pod-monitor-secrets \
    --from-literal=jwt-secret="${JWT_SECRET}" \
    -n "${NAMESPACE}"
  success "JWT Secret criado."
else
  warn "JWT Secret já existe — mantido sem alteração."
fi

apply k8s/compose/backend.yaml
apply k8s/compose/frontend.yaml
success "Manifests aplicados."

# ── 5. Aguardar pods ──────────────────────────────────────────────────────────
info "Aguardando pods ficarem prontos..."
$KUBECTL wait deployment/backend deployment/frontend \
  -n "${NAMESPACE}" --for=condition=Available --timeout=180s
success "Pods prontos!"

# ── 6. Exportar kubeconfig ────────────────────────────────────────────────────
info "Exportando kubeconfig..."
docker exec "${K3S_CTR}" cat /etc/rancher/k3s/k3s.yaml \
  | sed 's/127\.0\.0\.1/localhost/g' \
  > kubeconfig-compose.yaml
chmod 600 kubeconfig-compose.yaml
success "Kubeconfig salvo em: kubeconfig-compose.yaml"

# ── 7. Status final ───────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}══════════════════════════════════════════${NC}"
echo -e "${GREEN}  Deploy concluído com sucesso!${NC}"
echo -e "${GREEN}══════════════════════════════════════════${NC}"
echo ""
echo -e "  ${CYAN}Pod Monitor:${NC}      http://localhost:8080"
echo -e "  ${CYAN}Kubernetes API:${NC}   https://localhost:6443"
echo -e "  ${CYAN}Kubeconfig:${NC}       $(pwd)/kubeconfig-compose.yaml"
echo ""
echo -e "  Para usar kubectl local:"
echo -e "  ${YELLOW}export KUBECONFIG=\$(pwd)/kubeconfig-compose.yaml${NC}"
echo ""
echo -e "  Para remover tudo:"
echo -e "  ${YELLOW}./deploy.sh --down${NC}"
echo ""
