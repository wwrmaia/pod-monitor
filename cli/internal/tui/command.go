package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// handleCommandMode trata teclas enquanto a barra de comando (":") está
// aberta — mesmo padrão que o modo de filtro da tela de pods já usa
// (m.filtering/m.filterInput), só que global em vez de restrito a uma tela.
func (m model) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Enter):
		text := strings.ToLower(strings.TrimSpace(m.cmdInput.Value()))
		m.commanding = false
		m.cmdInput.SetValue("")
		target, ok := commandAliases[text]
		if !ok {
			m.err = "comando desconhecido: " + text
			return m, nil
		}
		m.err = ""
		return m.gotoScreen(target)

	case key.Matches(msg, keys.Back):
		m.commanding = false
		m.cmdInput.SetValue("")
		return m, nil

	default:
		var cmd tea.Cmd
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		return m, cmd
	}
}

// gotoScreen despacha pro mesmo método gotoX() que a tecla de atalho
// correspondente já chama (ver model.go) — a barra de comando é só uma
// segunda porta de entrada pros mesmos 7 destinos, nunca um destino novo.
func (m model) gotoScreen(target screen) (model, tea.Cmd) {
	switch target {
	case screenDashboard:
		return m.gotoDashboard()
	case screenNodes:
		return m.gotoNodes()
	case screenStorage:
		return m.gotoStorage()
	case screenQuotas:
		return m.gotoQuotas()
	case screenOrphans:
		return m.gotoOrphans()
	case screenCosts:
		return m.gotoCosts()
	case screenCerts:
		return m.gotoCerts()
	case screenHelp:
		if m.screen == screenHelp {
			return m, nil
		}
		return m.openOverlay(screenHelp)
	}
	return m, nil
}
