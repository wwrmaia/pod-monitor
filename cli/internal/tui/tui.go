// Package tui implementa o "nível 2" do roadmap da CLI: um painel
// interativo estilo k9s sobre a mesma API REST que internal/client já fala.
// Reaproveita internal/client e internal/config sem nenhuma mudança — só
// substitui a camada de apresentação (cmd/+internal/output) por uma UI de
// terminal (Bubble Tea) que fica rodando e se atualiza sozinha.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"pod-monitor/cli/internal/client"
)

// Run inicia o programa da TUI. username/role só alimentam o header (barra
// de contexto). presetCluster/presetNamespace, se não vazios, pulam direto
// pra tela de pods assim que a respectiva lista carregar (ver
// clustersLoadedMsg/namespacesLoadedMsg em model.go).
func Run(c *client.Client, username, role, presetCluster, presetNamespace string) error {
	p := tea.NewProgram(newModel(c, username, role, presetCluster, presetNamespace), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
