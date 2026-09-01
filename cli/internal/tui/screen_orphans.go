package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// orphanRows achata as 6 categorias de orphanedResources numa lista única
// com uma coluna TIPO — mesma união que cmd/orphans.go já faz pra tabela.
func orphanRows(o orphanedResources) []table.Row {
	var rows []table.Row
	add := func(tipo string, items []orphanItem) {
		for _, it := range items {
			rows = append(rows, table.Row{tipo, it.Namespace, it.Name, it.Age})
		}
	}
	add("PVC", o.PVCs)
	add("Service", o.Services)
	add("ConfigMap", o.ConfigMaps)
	add("Secret", o.Secrets)
	add("Ingress", o.Ingresses)
	add("ServiceAccount", o.ServiceAccounts)
	return rows
}

func (m *model) refreshOrphansTable() {
	columns := []table.Column{
		{Title: "TIPO", Width: 16},
		{Title: "NAMESPACE", Width: 18},
		{Title: "NOME", Width: 30},
		{Title: "IDADE", Width: 10},
	}
	rows := orphanRows(m.orphans)
	m.orphansTable.SetRows(nil)
	m.orphansTable.SetColumns(columns)
	m.orphansTable.SetRows(rows)
}

func (m model) updateOrphans(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.orphansTable, cmd = m.orphansTable.Update(msg)
	return m, cmd
}

func (m model) viewOrphans() string {
	header := fmt.Sprintf("Cluster: %s   Auditoria (recursos órfãos, todos os namespaces)", m.cluster)
	view := titleStyle.Render(header) + "\n"
	if len(m.orphansTable.Rows()) == 0 {
		view += dimStyle.Render(loadingOrEmpty(m.loadingOrphans))
	} else {
		view += m.orphansTable.View()
	}
	view += "\n" + helpStyle.Render("r atualizar | esc voltar | q sair")
	return view
}
