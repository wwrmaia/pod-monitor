# Manual do `podmon tui`

Painel interativo de terminal do Pod Monitor, estilo `k9s` — fica rodando e
se atualiza sozinho, em vez de um comando por chamada como o resto da CLI
(`podmon resources`, `podmon costs` etc.). Fala com a mesma API REST do
dashboard web, com o mesmo login e as mesmas permissões.

## Sumário

1. [Instalação](#1-instalação)
2. [Primeiro uso](#2-primeiro-uso)
3. [Barra de contexto (topo da tela)](#3-barra-de-contexto-topo-da-tela)
4. [Navegação básica](#4-navegação-básica)
5. [Tabela de pods](#5-tabela-de-pods)
6. [Telas de recurso (overlays)](#6-telas-de-recurso-overlays)
7. [Barra de comando (`:`)](#7-barra-de-comando-)
8. [Tela de ajuda (`?`)](#8-tela-de-ajuda-)
9. [Atualização automática](#9-atualização-automática)
10. [Referência completa de atalhos](#10-referência-completa-de-atalhos)
11. [Limitações conhecidas](#11-limitações-conhecidas)
12. [Solução de problemas](#12-solução-de-problemas)

## 1. Instalação

Ver [`INSTALL.md`](./INSTALL.md) pro guia completo (Linux/macOS/Windows,
checksums, build a partir do código-fonte). Resumo rápido:

```bash
# opção A — script automático (Linux/macOS)
cd pod-monitor/cli
./install.sh

# opção B — build manual, qualquer plataforma com Go 1.25+
git clone https://github.com/wwrmaia/pod-monitor.git
cd pod-monitor/cli
go build -o podmon .
```

## 2. Primeiro uso

A TUI reaproveita a mesma sessão que os outros comandos da CLI — se você já
rodou `podmon login`, é só abrir:

```bash
podmon login --server http://seu-backend:porta   # só na primeira vez, ou após expirar
podmon tui
```

Atalhos úteis na hora de abrir:

```bash
podmon tui --cluster local                          # pula direto pra lista de namespaces desse cluster
podmon tui --cluster local --namespace pod-monitor  # pula direto pra tabela de pods desse namespace
```

## 3. Barra de contexto (topo da tela)

Uma faixa fixa aparece no topo de toda tela (desde que o terminal tenha pelo
menos ~20 linhas de altura — em terminais muito baixos ela some sozinha pra
priorizar espaço de conteúdo):

```
cluster: local   namespace: pod-monitor            ▸ PODMON ◂
usuário: admin (administration)
alertas: 1 críticos, 0 avisos
```

- **cluster/namespace**: o que está selecionado no momento.
- **usuário/role**: quem está logado (vem da sessão salva por `podmon login`).
- **alertas**: resumo do **último** dashboard que você abriu nesta sessão da
  TUI (tecla `d`) — não é atualizado sozinho em segundo plano, porque isso
  exigiria consultar o servidor com frequência mesmo fora da tela de
  dashboard. Abra o dashboard (`d`) de vez em quando pra manter esse número
  fresco, ou confie na tela de dashboard em si quando precisar do valor
  exato e atual.
- A logo `▸ PODMON ◂` só aparece em terminais largos (~100 colunas ou mais).

## 4. Navegação básica

Fluxo principal, de cima pra baixo:

```
Clusters ──(enter)──▶ Namespaces ──(enter)──▶ Pods
   ▲                       │  ▲                  │
   └────────(esc)──────────┘  └──────(esc)────────┘
```

- `↑`/`↓` (ou `k`/`j`): move a seleção.
- `enter`: entra no item selecionado.
- `esc` / `backspace`: volta um nível.
- `r`: força atualizar a tela atual na hora (sem esperar o próximo ciclo automático).
- `q` / `Ctrl+C`: sai da TUI.

Na tela de namespaces, o primeiro item é sempre **"(todos os namespaces)"** —
escolher esse item mostra a tabela de pods de todos os namespaces juntos
(com uma coluna extra `NAMESPACE` pra distinguir).

## 5. Tabela de pods

Colunas: `POD`, `CONTAINER`, `NODE`, `PHASE`, `SEV`, `CPU_REQ`, `CPU_LIM`,
`CPU_USE`, `CPU%`, `MEM_REQ`, `MEM_LIM`, `MEM_USE`, `MEM%` (mais `NAMESPACE`
quando "todos os namespaces" está selecionado).

- **`SEV`**: `CRIT` (≥90% de uso de CPU ou memória em relação ao limite),
  `WARN` (≥85%), ou vazio. **É uma aproximação client-side** — os limiares
  reais configurados pelo administrador só são visíveis via
  `GET /api/thresholds`, que exige permissão de admin. Pra severidade
  **autoritativa** (exatamente como o backend calcula), use o dashboard
  (`d`).
- Campos de uso (`CPU_USE`/`MEM_USE`/`CPU%`/`MEM%`) aparecem como `-` quando
  o metrics-server não está disponível — isso é diferente de "0% de uso",
  não é tratado como dado real.
- `/`: abre um filtro por nome de pod/container (digite, `enter` confirma e
  mantém o filtro aplicado). `esc` limpa o filtro — funciona tanto enquanto
  você digita quanto depois de já ter confirmado com `enter` (nesse segundo
  caso, um `esc` limpa; um segundo `esc`, com o filtro já vazio, volta pra
  namespaces).
- `c`: cicla a ordenação da tabela — nome → CPU% → MEM% → nome...

## 6. Telas de recurso (overlays)

De **qualquer uma das três telas principais** (clusters/namespaces/pods),
uma tecla abre uma tela cheia com outro tipo de recurso. `esc` sempre volta
pra onde você estava antes de abrir a tela (mesmo que você tenha pulado
direto de uma tela de recurso pra outra sem voltar no meio).

| Tecla | Tela | Escopo | Auto-atualiza? |
|---|---|---|---|
| `d` | Dashboard/alertas | cluster | sim, a cada 10s enquanto aberta |
| `n` | Nodes | cluster | não — `r` pra atualizar |
| `s` | Storage (PVCs) | cluster | não — `r` pra atualizar |
| `u` | Quotas | cluster+namespace | não — `r` pra atualizar |
| `o` | Auditoria (recursos órfãos) | cluster (todos os namespaces) | não — `r` pra atualizar |
| `p` | Custos | cluster+namespace | não — `r` pra atualizar |
| `t` | Certificados TLS | cluster+namespace | não — `r` pra atualizar |

Notas específicas:

- **Dashboard**: os números aqui (alertas, top CPU/mem, nodes) são
  calculados pelo backend, iguais ao `podmon dashboard` e ao dashboard web
  — não são uma aproximação como a coluna `SEV` da tabela de pods.
- **Custos**: se o cluster tiver a estimativa de custo desabilitada
  (padrão — só faz sentido pra clusters de nuvem com cobrança por hora) ou
  habilitada mas sem preço configurado, a tela mostra uma mensagem
  explicando em vez de uma tabela vazia.
- **Auditoria**: sempre mostra todos os namespaces, mesmo que você tenha um
  namespace específico selecionado — o backend não filtra esse endpoint por
  namespace.
- **Certificados**: a coluna `BUCKET` (`ok`/`notice`/`warning`/`critical`/
  `expired`) vem pronta do backend, mesma classificação usada nos alertas de
  certificado por e-mail/Slack/Teams.

## 7. Barra de comando (`:`)

Alternativa às teclas de atalho da seção 6, útil se você não decorou os
atalhos ou prefere digitar o nome do que quer:

```
: nodes    (enter)   →  abre a tela de Nodes
: costs    (enter)   →  abre a tela de Custos
```

Aliases aceitos: `dashboard`/`dash`/`alerts`, `nodes`/`no`, `storage`/`pvc`/
`pvcs`, `quotas`/`quota`, `orphans`/`orph`/`audit`, `costs`/`cost`,
`certs`/`cert`/`certificates`, `help`. Um nome não reconhecido mostra
"comando desconhecido" e não navega pra lugar nenhum. `esc` cancela a
qualquer momento sem navegar.

## 8. Tela de ajuda (`?`)

Abre de qualquer lugar, lista todos os atalhos agrupados por categoria —
útil como referência rápida sem precisar consultar este manual. `esc` fecha
e volta pra onde você estava.

## 9. Atualização automática

- **Tabela de pods**: atualiza sozinha a cada ~8 segundos enquanto está
  aberta, mais um empurrão extra quase instantâneo quando o backend avisa
  (via SSE) que algo mudou em algum cluster.
- **Dashboard**: mesmo esquema, a cada ~10 segundos, só enquanto a tela está
  aberta (fechar a tela também para o polling).
- **As seis telas de recurso** (nodes/storage/quotas/orphans/custos/certs):
  **não** atualizam sozinhas — buscam o dado ao abrir a tela e ficam
  paradas até você apertar `r`. Essa é uma escolha deliberada: são dados que
  mudam bem mais devagar que uso de CPU/memória (nodes, PVCs, certificados,
  preços), então polling automático seria desperdício.

## 10. Referência completa de atalhos

| Tecla | Ação |
|---|---|
| `↑`/`k`, `↓`/`j` | mover seleção |
| `enter` | entrar / selecionar |
| `esc`, `backspace` | voltar / fechar tela |
| `r` | atualizar a tela atual agora |
| `q`, `Ctrl+C` | sair |
| `d` | dashboard/alertas |
| `n` | nodes |
| `s` | storage (PVCs) |
| `u` | quotas |
| `o` | auditoria (recursos órfãos) |
| `p` | custos |
| `t` | certificados TLS |
| `/` | filtrar (só na tabela de pods) |
| `c` | ciclar ordenação (só na tabela de pods) |
| `:` | abrir barra de comando |
| `?` | abrir tela de ajuda |

## 11. Limitações conhecidas

- **Somente leitura.** Não há ações de escrita/admin na TUI (nem na CLI em
  geral) — pra editar thresholds, webhooks, usuários, cadastrar cluster
  etc., use a interface web.
- **Sem reautenticação dentro da TUI.** Se a sessão expirar (erro 401), a
  TUI mostra uma tela avisando e não tenta relogar sozinha — é preciso
  rodar `podmon login` em outro terminal e reabrir a TUI. Isso é
  intencional: o fluxo de login interativo (senha oculta, MFA) não é
  compatível com a TUI já estar em modo de tela cheia.
- **Severidade da tabela de pods é aproximada** (ver seção 5) — use o
  dashboard pra números exatos.
- **Sem filtro/ordenação nas seis telas de recurso** — só a tabela de pods
  tem `/` e `c`.
- **Tabelas muito largas perdem a borda decorativa em terminais estreitos**
  (a tabela de pods com muitas colunas, por exemplo) — é uma escolha
  deliberada: melhor sem borda do que com uma borda visualmente quebrada.
  O conteúdo em si continua correto, só potencialmente cortado à direita;
  aumentar a largura do terminal resolve.

## 12. Solução de problemas

| Sintoma | Causa provável | O que fazer |
|---|---|---|
| Tela cheia "Sessão expirada" | Token expirou (401) | `podmon login` em outro terminal, reabra a TUI |
| Banner vermelho "erro: não foi possível conectar..." | Backend fora do ar ou endereço errado | Confira `--server`/o servidor salvo; a TUI mantém tentando sozinha e se recupera quando o backend voltar |
| Tabela sem dados de CPU/memória (`-` em toda linha) | metrics-server indisponível no cluster | Não é bug da TUI — o backend já avisa a mesma coisa nos outros comandos |
| Cores/borda estranhas ao redimensionar o terminal | Renderização em trânsito | Normalmente se resolve sozinho no próximo redesenho; se persistir, `r` força um redesenho |
| `podmon: command not found` depois de instalar | Pasta de instalação não está no `PATH` | Ver aviso do `install.sh`, ou confira `echo $PATH` |
