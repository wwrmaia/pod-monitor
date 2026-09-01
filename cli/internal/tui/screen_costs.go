package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// refreshCostsTable só monta linhas de tabela quando enabled+configured —
// os outros dois estados (desabilitado / habilitado sem preço) são
// mensagens de texto puro, iguais ao que "podmon costs" já mostra hoje
// (ver CLAUDE.md: custo é opt-in por cluster, default off).
func (m *model) refreshCostsTable() {
	if !m.costs.Enabled || !m.costs.Configured {
		m.costsTable.SetRows(nil)
		m.costsTable.SetColumns(nil)
		return
	}
	columns := []table.Column{
		{Title: "NAMESPACE", Width: 18},
		{Title: "CPU_CORES", Width: 10},
		{Title: "MEM_GIB", Width: 10},
		{Title: "CUSTO/HORA", Width: 12},
		{Title: "CUSTO/MÊS", Width: 12},
	}
	rows := make([]table.Row, 0, len(m.costs.Items))
	for _, i := range m.costs.Items {
		rows = append(rows, table.Row{
			i.Namespace,
			fmt.Sprintf("%.2f", i.CPUCores),
			fmt.Sprintf("%.2f", i.MemGiB),
			fmt.Sprintf("%s %.2f", m.costs.Currency, i.CostHour),
			fmt.Sprintf("%s %.2f", m.costs.Currency, i.CostMonth),
		})
	}
	m.costsTable.SetRows(nil)
	m.costsTable.SetColumns(columns)
	m.costsTable.SetRows(rows)
}

func (m model) updateCosts(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.costsLoaded || !m.costs.Enabled || !m.costs.Configured {
		return m, nil
	}
	var cmd tea.Cmd
	m.costsTable, cmd = m.costsTable.Update(msg)
	return m, cmd
}

func (m model) viewCosts() string {
	header := fmt.Sprintf("Cluster: %s   Namespace: %s   Custos", m.cluster, nsLabel(m.namespace))
	view := titleStyle.Render(header) + "\n"

	switch {
	case !m.costsLoaded:
		view += dimStyle.Render(loadingOrEmpty(m.loadingCosts))
	case !m.costs.Enabled:
		view += dimStyle.Render("Estimativa de custo desabilitada pra este cluster (padrão off — só faz sentido pra clusters de nuvem com cobrança por hora).")
	case !m.costs.Configured:
		view += dimStyle.Render("Estimativa de custo habilitada, mas sem preço configurado ainda.")
	case len(m.costsTable.Rows()) == 0:
		view += dimStyle.Render("(nenhum resultado)")
	default:
		view += m.costsTable.View()
	}

	view += "\n" + helpStyle.Render("r atualizar | esc voltar | q sair")
	return view
}
