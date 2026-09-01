package tui

import (
	"fmt"
	"strings"
)

// viewDashboard renderiza o overlay de dashboard/alertas. Ao contrário da
// tela de pods, não faz nenhum cálculo de severidade client-side — usa os
// campos já computados pelo backend (AlertPods[].Severity, TopCPU/TopMem[].Pct),
// que são autoritativos (a tela de pods usa uma aproximação, ver units.go).
func (m model) viewDashboard() string {
	header := titleStyle.Render(fmt.Sprintf("Dashboard — cluster %s", m.cluster))
	if !m.dashLoaded {
		return header + "\n" + renderPanel(m.loadingOrEmpty(true))
	}
	d := m.dash

	var b strings.Builder

	b.WriteString(fmt.Sprintf("Nodes: %d/%d prontos", d.Nodes.Ready, d.Nodes.Total))
	if len(d.Nodes.NotReadyNames) > 0 {
		b.WriteString(warningStyle.Render(fmt.Sprintf("  (não prontos: %s)", strings.Join(d.Nodes.NotReadyNames, ", "))))
	}
	b.WriteString("\n")

	b.WriteString("Pods por fase: ")
	for phase, n := range d.Pods.Statuses {
		b.WriteString(fmt.Sprintf("%s=%d  ", phase, n))
	}
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Alertas: %s críticos, %s avisos\n",
		criticalStyle.Render(fmt.Sprintf("%d", d.Alerts.Critical)),
		warningStyle.Render(fmt.Sprintf("%d", d.Alerts.Warning))))
	if len(d.AlertPods) == 0 {
		b.WriteString(dimStyle.Render("  (nenhum alerta ativo)") + "\n")
	}
	for _, ap := range d.AlertPods {
		st := styleForSeverity(ap.Severity)
		b.WriteString(st.Render(fmt.Sprintf("  [%s] %s/%s (%s) — %s", ap.Severity, ap.Namespace, ap.Pod, ap.Container, ap.Reason)) + "\n")
	}

	b.WriteString("\nTop CPU:\n")
	if len(d.TopCPU) == 0 {
		b.WriteString(dimStyle.Render("  (nenhum resultado)") + "\n")
	}
	for _, w := range d.TopCPU {
		b.WriteString(fmt.Sprintf("  %s/%s (%s) — %d%%\n", w.Namespace, w.Pod, w.Name, w.Pct))
	}

	b.WriteString("\nTop Memória:\n")
	if len(d.TopMem) == 0 {
		b.WriteString(dimStyle.Render("  (nenhum resultado)") + "\n")
	}
	for _, w := range d.TopMem {
		b.WriteString(fmt.Sprintf("  %s/%s (%s) — %d%%\n", w.Namespace, w.Pod, w.Name, w.Pct))
	}

	return header + "\n" + renderPanel(b.String()) + "\n" + helpStyle.Render("r atualizar | esc fechar | q sair")
}
