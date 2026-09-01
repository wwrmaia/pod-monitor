package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) refreshStorageTable() {
	columns := []table.Column{
		{Title: "NAMESPACE", Width: 16},
		{Title: "PVC", Width: 24},
		{Title: "CAPACIDADE", Width: 10},
		{Title: "STATUS", Width: 10},
		{Title: "STORAGE_CLASS", Width: 14},
		{Title: "ACCESS_MODES", Width: 14},
	}
	rows := make([]table.Row, 0, len(m.storage))
	for _, p := range m.storage {
		rows = append(rows, table.Row{
			p.Namespace, p.Name, dashIfEmpty(p.Capacity), p.Status,
			dashIfEmpty(p.StorageClass), dashIfEmpty(p.AccessModes),
		})
	}
	m.storageTable.SetRows(nil)
	m.storageTable.SetColumns(columns)
	m.storageTable.SetRows(rows)
}

func (m model) updateStorage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.storageTable, cmd = m.storageTable.Update(msg)
	return m, cmd
}

func (m model) viewStorage() string {
	header := fmt.Sprintf("Cluster: %s   Storage (PVCs)", m.cluster)
	view := titleStyle.Render(header) + "\n"
	if len(m.storageTable.Rows()) == 0 {
		view += dimStyle.Render(loadingOrEmpty(m.loadingStorage))
	} else {
		view += m.storageTable.View()
	}
	view += "\n" + helpStyle.Render("r atualizar | esc voltar | q sair")
	return view
}
