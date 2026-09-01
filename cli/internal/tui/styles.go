package tui

import "github.com/charmbracelet/lipgloss"

// Cores aproximam a paleta de severidade já usada no dashboard web
// (frontend/src/App.jsx: SEVERITY_COLOR — #f87171 crítico, #fbbf24 warning).
var (
	criticalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle    = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("196")).Padding(0, 1)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// loadingOrEmpty escolhe o texto de estado pra uma lista/tabela vazia —
// distingue "ainda buscando" de "resultado genuinamente vazio" (ver
// model.go, campos loading*).
func loadingOrEmpty(loading bool) string {
	if loading {
		return "carregando..."
	}
	return "(nenhum resultado)"
}

func styleForSeverity(sev string) lipgloss.Style {
	switch sev {
	case "critical":
		return criticalStyle
	case "warning":
		return warningStyle
	default:
		return lipgloss.NewStyle()
	}
}
