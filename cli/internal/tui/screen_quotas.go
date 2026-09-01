package tui

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// quotaRows achata namespace -> quotas[] -> resources (map) numa lista única,
// igual cmd/quotas.go já faz — mas ordenando as chaves do map, que o CLI não
// faz (a ordem de iteração de um map em Go é aleatória a cada execução).
func quotaRows(nqs []namespaceQuota) []table.Row {
	var rows []table.Row
	for _, nq := range nqs {
		for _, q := range nq.Quotas {
			keys := make([]string, 0, len(q.Resources))
			for k := range q.Resources {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				r := q.Resources[k]
				rows = append(rows, table.Row{nq.Namespace, q.Name, k, dashIfEmpty(r.Hard), dashIfEmpty(r.Used)})
			}
		}
	}
	return rows
}

func (m *model) refreshQuotasTable() {
	columns := []table.Column{
		{Title: "NAMESPACE", Width: 18},
		{Title: "QUOTA", Width: 20},
		{Title: "RECURSO", Width: 16},
		{Title: "HARD", Width: 10},
		{Title: "USADO", Width: 10},
	}
	rows := quotaRows(m.quotas)
	m.quotasTable.SetRows(nil)
	m.quotasTable.SetColumns(columns)
	m.quotasTable.SetRows(rows)
}

func (m model) updateQuotas(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.quotasTable, cmd = m.quotasTable.Update(msg)
	return m, cmd
}

func (m model) viewQuotas() string {
	header := fmt.Sprintf("Cluster: %s   Namespace: %s   Quotas", m.cluster, nsLabel(m.namespace))
	var content string
	if len(m.quotasTable.Rows()) == 0 {
		content = m.loadingOrEmpty(m.loadingQuotas)
	} else {
		content = m.quotasTable.View()
	}
	return titleStyle.Render(header) + "\n" + renderPanelFit(content, m.width) + "\n" + helpStyle.Render("r atualizar | esc voltar | q sair")
}
