package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// logoMinWidth/headerMinHeight: degradação graciosa em terminal pequeno —
// mesmo princípio já usado alhures (resize). Terminal estreito perde a logo;
// terminal baixo perde o header inteiro, priorizando espaço pro conteúdo.
const (
	logoMinWidth    = 100
	headerMinHeight = 20
	headerHeight    = 4 // 3 linhas de texto + 1 linha de respiro antes do corpo
)

// logoPalette dá um degradê roxo→rosa→azul, uma letra de cada cor — mais
// "marca" do que uma caixa com borda (que lembra um botão de UI, feedback
// recebido do usuário na primeira versão). Cores do círculo 256 do ANSI.
var logoPalette = []lipgloss.Color{"99", "135", "171", "213", "212", "39"}

// renderLogo estiliza "PODMON" letra a letra, sem caixa/borda — texto puro
// colorido é seguro de desenhar (cada letra é um único caractere, sem risco
// do tipo de desalinhamento que arte ASCII multi-linha teria) e ainda assim
// lê como uma marca, não como um botão clicável.
func renderLogo() string {
	name := "PODMON"
	var b strings.Builder
	b.WriteString(dimStyle.Render("▸ "))
	for i, r := range name {
		c := logoPalette[i%len(logoPalette)]
		b.WriteString(lipgloss.NewStyle().Foreground(c).Bold(true).Render(string(r)))
	}
	b.WriteString(dimStyle.Render(" ◂"))
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// renderHeader monta a barra de contexto fixa, chamada uma única vez em
// View() (não em cada viewX()) e prefixada ao corpo da tela atual — ponto de
// injeção único, qualquer tela (base, overlay ou ajuda) ganha o header de
// graça. O resumo de alertas usa o ÚLTIMO dashboard já carregado (m.dash) —
// não dispara fetch novo só pra isso, pra não reabrir a decisão já tomada de
// não pollar o dashboard fora da tela dele (efeito colateral de snapshot +
// broadcast SSE a cada chamada, ver model.go/messages.go).
func renderHeader(m model) string {
	if m.height < headerMinHeight || m.width <= 0 {
		return ""
	}

	ns := m.namespace
	if ns == "" && m.cluster != "" {
		ns = allNamespacesLabel
	}
	lines := []string{
		fmt.Sprintf("cluster: %s   namespace: %s", orDash(m.cluster), orDash(ns)),
		fmt.Sprintf("usuário: %s (%s)", orDash(m.username), orDash(m.role)),
	}
	if m.dashLoaded {
		lines = append(lines, fmt.Sprintf("alertas: %d críticos, %d avisos", m.dash.Alerts.Critical, m.dash.Alerts.Warning))
	} else {
		lines = append(lines, `alertas: — (abra o dashboard com "d")`)
	}
	text := strings.Join(lines, "\n")

	// -1 de margem de segurança em todo width de header: com o total
	// batendo exatamente na largura do terminal (info+logo, ou só info),
	// o terminal pode quebrar a linha sozinho antes do fundo colorido
	// cobrir a última coluna, deixando uma faixa sem cor no canto direito
	// (mesma causa do bug de borda de painel corrigido em model.go).
	if m.width < logoMinWidth {
		return headerStyle.Width(m.width - 1).Render(text)
	}

	logo := renderLogo()
	infoWidth := m.width - lipgloss.Width(logo) - 1
	if infoWidth < 30 {
		return headerStyle.Width(m.width - 1).Render(text)
	}
	info := headerStyle.Width(infoWidth).Render(text)
	return lipgloss.JoinHorizontal(lipgloss.Top, info, logo)
}
