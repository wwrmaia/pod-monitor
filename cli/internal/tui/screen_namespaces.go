package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateNamespaces(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Enter) {
		if it, ok := m.nsList.SelectedItem().(simpleItem); ok {
			sel := string(it)
			if sel == allNamespacesLabel {
				m.namespace = ""
			} else {
				m.namespace = sel
			}
			m.screen = screenPods
			m.pods = nil
			m.loadingPods = true
			m.refreshPodTable()
			return m, tea.Batch(
				fetchResources(m.client, m.cluster, m.namespace),
				tickAfter(resourcesPollInterval, screenPods),
			)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.nsList, cmd = m.nsList.Update(msg)
	return m, cmd
}

func (m model) viewNamespaces() string {
	header := titleStyle.Render("Cluster: " + m.cluster)
	var content string
	if len(m.nsList.Items()) == 0 {
		content = m.loadingOrEmpty(m.loadingNamespaces)
	} else {
		content = m.nsList.View()
	}
	return header + "\n" + renderPanel(content) + "\n" + helpStyle.Render(overlayHelpFooter)
}
