package tui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// Cores aproximam a paleta de severidade já usada no dashboard web
// (frontend/src/App.jsx: SEVERITY_COLOR — #f87171 crítico, #fbbf24 warning).
var (
	criticalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle    = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("196")).Padding(0, 1)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	accentColor = lipgloss.Color("62") // roxo/azulado — cor de marca do podmon nesta TUI

	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentColor).Padding(0, 1)
	headerStyle = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("252")).Padding(0, 1)
	logoStyle   = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
)

// panelOverhead: 1 coluna/linha de borda arredondada de cada lado + o
// padding horizontal de renderPanel — usado em model.go pra dimensionar as
// tabelas/listas de modo que a caixa inteira (conteúdo+borda) ainda caiba no
// terminal, e não só o conteúdo.
const (
	panelOverheadWidth  = 4 // 2 de borda + 2 de padding (Padding(0,1) nos dois lados)
	panelOverheadHeight = 2 // 2 de borda (topo/base) — sem padding vertical
)

// renderPanel envolve o conteúdo principal de uma tela numa caixa — é o que
// dá a cada tela a cara de "painel" em vez de texto solto contra o fundo do
// terminal, mesmo espírito visual do k9s.
func renderPanel(body string) string {
	return panelStyle.Render(body)
}

// renderPanelFit só desenha a borda se o conteúdo couber em maxWidth — sem
// isso, uma tabela de muitas colunas (a de pods, com 13-14) fica com a
// borda quebrada em terminais mais estreitos, e a causa é uma limitação real
// do bubbles/table: o CABEÇALHO da tabela (headersView()) nunca é truncado
// pela largura configurada via SetWidth — só as linhas de dado passam pelo
// corte do viewport. Numa tabela larga, o cabeçalho renderiza na largura
// "natural" (soma das colunas + padding), mais larga que o terminal; embrulhar
// isso numa borda produz uma caixa do tamanho do cabeçalho, que o próprio
// terminal quebra de um jeito visualmente quebrado. Preferível degradar
// graciosamente (sem borda) a mostrar uma borda corrompida — mesmo
// princípio já usado pra esconder a logo/header em terminal pequeno.
func renderPanelFit(body string, maxWidth int) string {
	if lipgloss.Width(body) > maxWidth-panelOverheadWidth {
		return body
	}
	return panelStyle.Render(body)
}

// tableStyles customiza o destaque de seleção do bubbles/table — o padrão da
// lib é sutil demais pra ficar "k9s-like"; aqui a linha selecionada vira uma
// barra de fundo sólido, parecida com a barra invertida do k9s.
func tableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Selected = s.Selected.Background(accentColor).Foreground(lipgloss.Color("230")).Bold(true)
	s.Header = s.Header.Bold(true).BorderBottom(true).BorderForeground(accentColor)
	return s
}

// loadingOrEmpty escolhe o texto de estado pra uma lista/tabela vazia —
// distingue "ainda buscando" (spinner animado) de "resultado genuinamente
// vazio" (ver model.go, campos loading*). Método (não função solta) porque
// precisa do spinner do model pra renderizar o quadro atual da animação.
func (m model) loadingOrEmpty(loading bool) string {
	if loading {
		return m.spinner.View() + " " + dimStyle.Render("carregando...")
	}
	return dimStyle.Render("(nenhum resultado)")
}

func styleForSeverity(sev string) lipgloss.Style {
	switch sev {
	case "critical":
		return criticalStyle
	case "warning":
		return warningStyle
	default:
		return lipgloss.NewStyle()
	}
}
