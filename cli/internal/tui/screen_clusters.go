package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateClusters(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Enter) {
		if it, ok := m.clusterList.SelectedItem().(simpleItem); ok {
			m.cluster = string(it)
			m.namespace = ""
			m.screen = screenNamespaces
			m.loadingNamespaces = true
			return m, fetchNamespaces(m.client, m.cluster)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.clusterList, cmd = m.clusterList.Update(msg)
	return m, cmd
}

func (m model) viewClusters() string {
	var content string
	if len(m.clusterList.Items()) == 0 {
		content = titleStyle.Render("Clusters") + "\n" + m.loadingOrEmpty(m.loadingClusters)
	} else {
		content = m.clusterList.View()
	}
	return renderPanel(content) + "\n" + helpStyle.Render(overlayHelpFooter)
}
