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
	var body string
	if len(m.clusterList.Items()) == 0 {
		body = titleStyle.Render("Clusters") + "\n" + dimStyle.Render(loadingOrEmpty(m.loadingClusters))
	} else {
		body = m.clusterList.View()
	}
	return body + "\n" + helpStyle.Render(overlayHelpFooter)
}
