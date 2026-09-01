package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) refreshCertsTable() {
	columns := []table.Column{
		{Title: "NAMESPACE", Width: 16},
		{Title: "SECRET", Width: 22},
		{Title: "DOMÍNIO (CN)", Width: 24},
		{Title: "EMISSOR", Width: 18},
		{Title: "DIAS_RESTANTES", Width: 14},
		{Title: "BUCKET", Width: 10},
	}
	rows := make([]table.Row, 0, len(m.certs))
	for _, c := range m.certs {
		rows = append(rows, table.Row{
			c.Namespace, c.SecretName, dashIfEmpty(c.CommonName), dashIfEmpty(c.Issuer),
			fmt.Sprintf("%d", c.DaysLeft), c.Bucket,
		})
	}
	m.certsTable.SetRows(nil)
	m.certsTable.SetColumns(columns)
	m.certsTable.SetRows(rows)
}

func (m model) updateCerts(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.certsTable, cmd = m.certsTable.Update(msg)
	return m, cmd
}

func (m model) viewCerts() string {
	header := fmt.Sprintf("Cluster: %s   Namespace: %s   Certificados TLS", m.cluster, nsLabel(m.namespace))
	var content string
	if len(m.certsTable.Rows()) == 0 {
		content = m.loadingOrEmpty(m.loadingCerts)
	} else {
		content = m.certsTable.View()
	}
	return titleStyle.Render(header) + "\n" + renderPanelFit(content, m.width) + "\n" +
		helpStyle.Render("bucket: ok/notice/warning/critical/expired (calculado pelo backend) | r atualizar | esc voltar | q sair")
}
