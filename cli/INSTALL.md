# podmon CLI — Manual de Instalação e Uso

Cliente de terminal do Pod Monitor — fala com a mesma API do dashboard web,
com o mesmo login e as mesmas permissões. Este guia cobre instalação em
Linux, macOS e Windows.

```
podmon login --server http://seu-servidor:porta
```

## 1. Instalação

Os binários já vêm prontos — não precisa ter Go instalado pra usar (só pra
compilar do zero, ver [seção 6](#6-alternativa-build-a-partir-do-código-fonte)).

### Linux

Escolha o arquivo certo pra sua arquitetura: `podmon-linux-amd64`
(Intel/AMD — a maioria dos PCs e servidores) ou `podmon-linux-arm64` (ARM,
ex.: Raspberry Pi 64-bit).

```bash
# dá permissão de execução
chmod +x podmon-linux-amd64

# move pra uma pasta que já está no PATH
sudo mv podmon-linux-amd64 /usr/local/bin/podmon

# confirma que funcionou
podmon --help
```

Sem `sudo` à mão? Qualquer pasta já listada no seu `$PATH` serve —
`~/.local/bin` e `~/bin` costumam já estar lá por padrão:

```bash
mkdir -p ~/.local/bin
cp podmon-linux-amd64 ~/.local/bin/podmon
chmod +x ~/.local/bin/podmon
```

### macOS

Escolha `podmon-darwin-arm64` (Apple Silicon — M1/M2/M3/M4) ou
`podmon-darwin-amd64` (Mac Intel).

```bash
chmod +x podmon-darwin-arm64
sudo mv podmon-darwin-arm64 /usr/local/bin/podmon
podmon --help
```

> **Gatekeeper vai bloquear na primeira vez.** É um binário sem assinatura da
> Apple — ao rodar, o macOS provavelmente vai dizer que não conseguiu
> verificar o desenvolvedor. Resolve com um comando só, uma vez:
> ```bash
> xattr -d com.apple.quarantine /usr/local/bin/podmon
> ```
> Depois disso ele roda normalmente, sem avisos.

### Windows

Copie `podmon-windows-amd64.exe` pra uma pasta de sua preferência (ex.:
`C:\Ferramentas\`) e renomeie pra `podmon.exe` se quiser digitar só `podmon`
depois.

**Adicionar ao PATH** (PowerShell, como Administrador):

```powershell
# troca pelo caminho real da pasta onde você colocou o .exe
$env:Path += ";C:\Ferramentas"
# pra tornar permanente, adiciona em Configurações do Sistema →
# Variáveis de Ambiente → Path
```

> **SmartScreen vai avisar na primeira vez** — mesmo motivo do Gatekeeper no
> Mac: é um executável sem assinatura de editor reconhecido. Clique em
> **"Mais informações"** e depois **"Executar assim mesmo"**. Só acontece
> uma vez.

Sem mexer no PATH, também funciona chamando o caminho completo direto:

```powershell
.\podmon.exe --help
```

## 2. Primeiro login

Igual em qualquer plataforma — o comando pede usuário e senha (e código MFA,
se sua conta tiver ativado):

```bash
podmon login --server http://192.168.49.2:30080
```

A sessão fica salva localmente — não precisa logar de novo a cada comando,
só quando o token expirar ou depois de `podmon logout`.

> Primeiro acesso e ainda sem MFA configurado? O login mostra o **secret
> TOTP** em texto pra você adicionar manualmente no seu autenticador (Google
> Authenticator, Authy etc.). Pra escanear QR code, use a interface web uma
> vez.

## 3. Comandos disponíveis

Toda flag `--cluster` / `-n` (namespace) é global. Qualquer comando aceita
`--output json` em vez de tabela, pra usar com `jq` ou script.

| Comando | O que mostra |
|---|---|
| `clusters` | Clusters registrados no pod-monitor |
| `namespaces --cluster X` | Namespaces de um cluster |
| `resources --cluster X [-n NS]` | Pods com requests/limits/uso de CPU e memória |
| `nodes --cluster X` | Nodes com capacidade e uso |
| `top --cluster X` | Top consumidores de CPU e memória |
| `costs --cluster X` | Estimativa de custo por namespace |
| `certs --cluster X` | Certificados TLS e dias até expirar |
| `storage --cluster X` | PersistentVolumeClaims |
| `quotas --cluster X` | ResourceQuotas por namespace |
| `orphans --cluster X` | Recursos órfãos (PVC, Service, ConfigMap...) |
| `analysis --cluster X` | Varredura de segurança/confiabilidade sob demanda |
| `logs <pod> -n NS --container C [-f]` | Logs do pod — snapshot único, ou tempo real com `-f`/`--follow` |
| `dashboard --cluster X` | Resumo: pods por fase, nodes, alertas |
| `events --cluster X` | Tail ao vivo dos eventos (Ctrl+C pra sair) |
| `logout` | Encerra a sessão e revoga o token |

## 4. Configuração local

O `login` salva servidor, usuário e token num arquivo local, permissão
restrita (só seu usuário lê):

- **Linux/macOS:** `~/.config/pod-monitor-cli/config.yaml`
- **Windows:** `C:\Users\<seu-usuário>\.config\pod-monitor-cli\config.yaml`

Apagar esse arquivo (ou rodar `podmon logout`) equivale a sair da sessão.

## 5. Limitações conhecidas

- **`podmon logs -f` exige um backend atualizado (2026-09-01 ou mais novo).**
  Contra um backend antigo, sem `/api/logs/stream`, `-f` falha com 404 — use
  sem `-f` (snapshot de até `--tail` linhas) nesse caso.
- **Sem operações de escrita/admin.** Webhooks, thresholds, usuários/grupos,
  cadastro de cluster e afins ficam de fora desta v1 — só leitura.
- **Sem instalador automático.** Sem gerenciador de pacotes (`brew`, `apt`,
  `winget`) por enquanto — é copiar o binário manualmente.

## 6. Alternativa: build a partir do código-fonte

Precisa de [Go 1.25 ou mais novo](https://go.dev/dl/) instalado. Funciona
igual nas três plataformas:

```bash
git clone https://github.com/wwrmaia/pod-monitor.git
cd pod-monitor/cli
go build -o podmon .
```

## 7. Checksums (SHA-256)

Confere depois de transferir o arquivo (scp, pendrive, drive compartilhado)
pra garantir que chegou intacto — `sha256sum arquivo` (Linux/macOS) ou
`Get-FileHash arquivo` (PowerShell).

```
podmon-linux-amd64       f0a3cecf94261b3b631eeaafdf4032880ea6479729698fef6e3fa9171424af59
podmon-linux-arm64       dcdbaaf7f748fd364e2c26614512fba929f996213372d3e580406b6f1617e3a7
podmon-darwin-amd64      273a261477c980f36be8c7cf0fdc9fc31f50929e5ed8633e7632593af1e6788b
podmon-darwin-arm64      78281c2b23d0868b904653dd9db3e040cbc22338368e21310de5ed4aaf33270e
podmon-windows-amd64.exe b1d0b9c9b6c1156bb1cbf57e723ce041d6e378fb706360c93340f8d1d181841d
```

> Esses checksums valem para os binários regenerados em 2026-09-01 (adição de
> `podmon logs -f`/`--follow`, tail de logs em tempo real — precisa do
> backend na mesma versão ou mais nova, que expõe `/api/logs/stream`). Se
> você compilar do código-fonte ou gerar uma nova versão, os hashes mudam —
> recalcule com `sha256sum` (Linux/macOS) ou `Get-FileHash` (Windows) antes
> de comparar. Um `SHA256SUMS.txt` já calculado acompanha `cli/dist/` e
> `cli/podmon_tui/`.

## 8. Instalação automática (Linux/macOS)

Pra pular os passos manuais das seções 1 e usar o mesmo binário certo pra
sua máquina, rode o script de instalação de dentro de `cli/` (funciona com
um binário pronto em `dist/`/`podmon_tui/` ao lado, ou compila do código-fonte
se tiver Go instalado):

```bash
cd pod-monitor/cli
./install.sh                          # instala em ~/.local/bin
./install.sh --prefix /usr/local/bin  # instala em local do sistema (pode pedir permissão)
```

Windows não é coberto pelo script — siga a seção Windows acima.

---

Além dos comandos nível 1 acima, `podmon tui` é um painel interativo estilo
`k9s` (navegação ao vivo, atualização automática) — ver a seção TUI em
`cli/README.md` e `docs/documentacao-completa.md` (seção 31).
