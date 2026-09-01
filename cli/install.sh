#!/usr/bin/env bash
# podmon — script de instalação (Linux/macOS)
#
# Detecta SO/arquitetura, procura um binário pronto (ao lado deste script,
# ou em ./dist/ ou ./podmon_tui/) e instala em ~/.local/bin (sem precisar de
# sudo). Se não achar um binário pronto, tenta compilar do código-fonte com
# "go build" (precisa estar dentro de cli/, com Go instalado).
#
# Uso:
#   ./install.sh                          # instala em ~/.local/bin
#   ./install.sh --prefix /usr/local/bin  # instala em outro lugar (pode pedir sudo)
#
# Windows não é suportado por este script — ver cli/INSTALL.md.
set -euo pipefail

PREFIX="${PODMON_INSTALL_PREFIX:-$HOME/.local/bin}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)
      PREFIX="$2"
      shift 2
      ;;
    -h|--help)
      echo "Uso: $0 [--prefix CAMINHO]"
      echo "  --prefix CAMINHO   pasta de instalação (padrão: \$HOME/.local/bin)"
      exit 0
      ;;
    *)
      echo "opção desconhecida: $1" >&2
      exit 1
      ;;
  esac
done

os_name="$(uname -s)"
arch_name="$(uname -m)"

case "$os_name" in
  Linux) goos=linux ;;
  Darwin) goos=darwin ;;
  *)
    echo "erro: SO não suportado por este script ($os_name)." >&2
    echo "Windows: veja a seção 'Windows' em cli/INSTALL.md." >&2
    exit 1
    ;;
esac

case "$arch_name" in
  x86_64|amd64) goarch=amd64 ;;
  arm64|aarch64) goarch=arm64 ;;
  *)
    echo "erro: arquitetura não suportada ($arch_name)." >&2
    exit 1
    ;;
esac

binname="podmon-${goos}-${goarch}"
if [[ "$goos" == "windows" ]]; then
  binname="${binname}.exe"
fi

candidates=(
  "$SCRIPT_DIR/$binname"
  "$SCRIPT_DIR/dist/$binname"
  "$SCRIPT_DIR/podmon_tui/$binname"
)

found=""
for c in "${candidates[@]}"; do
  if [[ -f "$c" ]]; then
    found="$c"
    break
  fi
done

mkdir -p "$PREFIX"
dest="$PREFIX/podmon"

if [[ -n "$found" ]]; then
  echo "Binário pronto encontrado: $found"
  cp "$found" "$dest"
else
  echo "Nenhum binário pronto pra $goos/$goarch encontrado ao lado deste script."
  echo "Tentando compilar do código-fonte..."
  if ! command -v go >/dev/null 2>&1; then
    echo "erro: Go não encontrado no PATH. Instale Go 1.25+ (https://go.dev/dl/)," >&2
    echo "ou copie um binário pronto (podmon-${goos}-${goarch}) pra $SCRIPT_DIR e rode de novo." >&2
    exit 1
  fi
  if [[ ! -f "$SCRIPT_DIR/go.mod" ]]; then
    echo "erro: não encontrei go.mod em $SCRIPT_DIR." >&2
    echo "Rode este script de dentro da pasta cli/ de um clone do repositório." >&2
    exit 1
  fi
  (cd "$SCRIPT_DIR" && go build -o "$dest" .)
fi

chmod +x "$dest"

if [[ "$goos" == "darwin" ]] && command -v xattr >/dev/null 2>&1; then
  # remove a quarentena do Gatekeeper — sem isso, o macOS bloqueia o binário
  # na primeira execução por não ter assinatura de desenvolvedor Apple.
  xattr -d com.apple.quarantine "$dest" 2>/dev/null || true
fi

case ":${PATH}:" in
  *":${PREFIX}:"*)
    path_ok=1
    ;;
  *)
    path_ok=0
    ;;
esac

echo
echo "podmon instalado em: $dest"

if [[ "$path_ok" -eq 0 ]]; then
  shell_rc="~/.bashrc (ou ~/.zshrc, dependendo do seu shell)"
  echo
  echo "Aviso: $PREFIX não está no seu PATH ainda."
  echo "Adicione esta linha no seu $shell_rc:"
  echo "  export PATH=\"$PREFIX:\$PATH\""
  echo "e abra um terminal novo (ou rode 'source ~/.bashrc')."
fi

echo
if "$dest" --help >/dev/null 2>&1; then
  echo "Instalação OK. Pra começar:"
  echo "  podmon login --server http://SEU-BACKEND:PORTA"
  echo "  podmon tui"
else
  echo "Aviso: não consegui rodar '$dest --help' — confira manualmente."
fi
