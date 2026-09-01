package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) refreshNodesTable() {
	columns := []table.Column{
		{Title: "NODE", Width: 20},
		{Title: "STATUS", Width: 10},
		{Title: "ROLE", Width: 14},
		{Title: "CPU_ALLOC", Width: 10},
		{Title: "MEM_ALLOC", Width: 10},
		{Title: "CPU_USE", Width: 10},
		{Title: "MEM_USE", Width: 10},
	}
	rows := make([]table.Row, 0, len(m.nodes))
	for _, n := range m.nodes {
		rows = append(rows, table.Row{
			n.Name, n.Status, n.Role,
			dashIfEmpty(n.CPUAllocatable), dashIfEmpty(n.MemAllocatable),
			dashIfEmpty(n.CPUUsage), dashIfEmpty(n.MemUsage),
		})
	}
	m.nodeTable.SetRows(nil)
	m.nodeTable.SetColumns(columns)
	m.nodeTable.SetRows(rows)
}

func (m model) updateNodes(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.nodeTable, cmd = m.nodeTable.Update(msg)
	return m, cmd
}

func (m model) viewNodes() string {
	header := fmt.Sprintf("Cluster: %s   Nodes", m.cluster)
	view := titleStyle.Render(header) + "\n"
	if len(m.nodeTable.Rows()) == 0 {
		view += dimStyle.Render(loadingOrEmpty(m.loadingNodes))
	} else {
		view += m.nodeTable.View()
	}
	view += "\n" + helpStyle.Render("r atualizar | esc voltar | q sair")
	return view
}
