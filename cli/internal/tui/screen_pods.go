package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

type podRow struct {
	namespace, pod, container, node, phase         string
	cpuReq, cpuLim, cpuUse, memReq, memLim, memUse string
	cpuPct, memPct                                 int
	cpuOK, memOK                                   bool
	severity                                       string
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func buildPodRows(pods []podResources) []podRow {
	var rows []podRow
	for _, p := range pods {
		for _, ct := range p.Containers {
			cpuPct, cpuOK := pctOf(ct.CPUUsage, ct.CPULimit, parseCPUMilli)
			memPct, memOK := pctOf(ct.MemoryUsage, ct.MemoryLimit, parseMemMiB)
			rows = append(rows, podRow{
				namespace: p.Namespace, pod: p.Name, container: ct.Name,
				node: p.Node, phase: p.Phase,
				cpuReq: dashIfEmpty(ct.CPURequest), cpuLim: dashIfEmpty(ct.CPULimit), cpuUse: dashIfEmpty(ct.CPUUsage),
				memReq: dashIfEmpty(ct.MemoryRequest), memLim: dashIfEmpty(ct.MemoryLimit), memUse: dashIfEmpty(ct.MemoryUsage),
				cpuPct: cpuPct, memPct: memPct, cpuOK: cpuOK, memOK: memOK,
				severity: maxSeverity(sevForPct(cpuPct, cpuOK), sevForPct(memPct, memOK)),
			})
		}
	}
	return rows
}

func filterRows(rows []podRow, q string) []podRow {
	if q == "" {
		return rows
	}
	q = strings.ToLower(q)
	var out []podRow
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.pod), q) || strings.Contains(strings.ToLower(r.container), q) {
			out = append(out, r)
		}
	}
	return out
}

func sortRows(rows []podRow, col int) {
	switch col {
	case 1:
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].cpuPct > rows[j].cpuPct })
	case 2:
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].memPct > rows[j].memPct })
	default:
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].pod < rows[j].pod })
	}
}

func sortLabel(col int) string {
	switch col {
	case 1:
		return "CPU%"
	case 2:
		return "MEM%"
	default:
		return "nome"
	}
}

func sevLabel(sev string) string {
	switch sev {
	case "critical":
		return "CRIT"
	case "warning":
		return "WARN"
	default:
		return ""
	}
}

func nsLabel(ns string) string {
	if ns == "" {
		return allNamespacesLabel
	}
	return ns
}

// refreshPodTable reconstrói colunas e linhas da tabela a partir de m.pods,
// aplicando o filtro/ordenação correntes. Chamado sempre que m.pods, o texto
// do filtro, a coluna de ordenação ou o namespace selecionado mudam.
func (m *model) refreshPodTable() {
	rows := buildPodRows(m.pods)
	sortRows(rows, m.sortCol)
	rows = filterRows(rows, m.filterInput.Value())

	showNamespace := m.namespace == ""
	var columns []table.Column
	if showNamespace {
		columns = append(columns, table.Column{Title: "NAMESPACE", Width: 14})
	}
	columns = append(columns,
		table.Column{Title: "POD", Width: 22},
		table.Column{Title: "CONTAINER", Width: 16},
		table.Column{Title: "NODE", Width: 14},
		table.Column{Title: "PHASE", Width: 9},
		table.Column{Title: "SEV", Width: 4},
		table.Column{Title: "CPU_REQ", Width: 8},
		table.Column{Title: "CPU_LIM", Width: 8},
		table.Column{Title: "CPU_USE", Width: 8},
		table.Column{Title: "CPU%", Width: 6},
		table.Column{Title: "MEM_REQ", Width: 8},
		table.Column{Title: "MEM_LIM", Width: 8},
		table.Column{Title: "MEM_USE", Width: 8},
		table.Column{Title: "MEM%", Width: 6},
	)

	trows := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		cpuPctStr, memPctStr := "-", "-"
		if r.cpuOK {
			cpuPctStr = fmt.Sprintf("%d%%", r.cpuPct)
		}
		if r.memOK {
			memPctStr = fmt.Sprintf("%d%%", r.memPct)
		}

		row := table.Row{}
		if showNamespace {
			row = append(row, r.namespace)
		}
		// Severidade vira uma coluna de texto puro (SEV), nunca ANSI dentro
		// de uma célula: bubbles/table trunca célula por contagem de bytes,
		// não por largura visual, então uma célula pré-colorida com
		// lipgloss.Render (que embute códigos de escape) pode ser cortada no
		// meio do escape e corromper a linha inteira — observado ao testar
		// contra um pod real com severidade crítica (memória em 100%).
		row = append(row, r.pod, r.container, r.node, r.phase, sevLabel(r.severity),
			r.cpuReq, r.cpuLim, r.cpuUse, cpuPctStr,
			r.memReq, r.memLim, r.memUse, memPctStr)
		trows = append(trows, row)
	}

	// bubbles/table.SetColumns redesenha na hora usando as linhas AINDA
	// guardadas do estado anterior (SetColumns -> UpdateViewport ->
	// renderRow, que indexa célula por célula usando as colunas recém-
	// trocadas). Se o número de colunas mudou (ex.: alternando entre "um
	// namespace" e "todos os namespaces", que acrescenta a coluna
	// NAMESPACE), as linhas antigas têm largura diferente da nova e
	// renderRow estoura o índice — crash reproduzido na prática ao trocar de
	// namespace duas vezes seguidas. Zerar as linhas antes de trocar as
	// colunas garante que nunca exista, mesmo que por um instante, um par
	// (colunas novas, linhas antigas) de larguras diferentes.
	m.podTable.SetRows(nil)
	m.podTable.SetColumns(columns)
	m.podTable.SetRows(trows)
}

func (m model) updatePods(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Filter):
		m.filtering = true
		m.filterInput.Focus()
		return m, nil
	case key.Matches(msg, keys.Sort):
		m.sortCol = (m.sortCol + 1) % 3
		m.refreshPodTable()
		return m, nil
	}
	var cmd tea.Cmd
	m.podTable, cmd = m.podTable.Update(msg)
	return m, cmd
}

func (m model) viewPods() string {
	header := fmt.Sprintf("Cluster: %s   Namespace: %s   Ordenar: %s",
		m.cluster, nsLabel(m.namespace), sortLabel(m.sortCol))
	view := titleStyle.Render(header) + "\n"
	if len(m.podTable.Rows()) == 0 {
		view += dimStyle.Render(loadingOrEmpty(m.loadingPods))
	} else {
		view += m.podTable.View()
	}
	if m.filtering || m.filterInput.Value() != "" {
		view += "\nFiltro: " + m.filterInput.View()
	}
	view += "\n" + helpStyle.Render("severidade aproximada (85%/90%) — pressione d para alertas autoritativos | / filtrar | c ordenar | esc voltar")
	view += "\n" + helpStyle.Render(overlayHelpFooter)
	return view
}
