package tui

import tea "github.com/charmbracelet/bubbletea"

// helpText é estático — a tela de ajuda não busca nada, só lista os atalhos
// já existentes (nenhum atalho novo é criado só pra esta tela existir).
const helpText = `Navegação
  ↑/↓ (ou k/j)     mover seleção
  enter            entrar / selecionar
  esc, backspace   voltar
  r                atualizar a tela atual
  q, ctrl+c        sair

Telas (de qualquer tela-base: clusters → namespaces → pods)
  d   dashboard/alertas (dados autoritativos do backend)
  n   nodes
  s   storage (PVCs)
  u   quotas
  o   auditoria (recursos órfãos)
  p   custos
  t   certificados TLS

Tabela de pods
  /   filtrar por nome de pod/container
  c   ciclar ordenação (nome → CPU% → MEM%)

Barra de comando
  :   abrir a barra de comando — digite um nome (ex.: "nodes", "costs",
      "dashboard") e enter pra pular pra essa tela; esc cancela

Ajuda
  ?   esta tela`

func (m model) viewHelp() string {
	return titleStyle.Render("Ajuda") + "\n" + renderPanel(helpText) + "\n" + helpStyle.Render("esc fechar | q sair")
}

// updateHelp não faz nada com teclas que sobrarem do dispatch global — a
// tela é só leitura de texto estático, sem componente interativo.
func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, nil
}
