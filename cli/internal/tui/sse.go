package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"pod-monitor/cli/internal/client"
)

const (
	sseInitialBackoff = 2 * time.Second
	sseMaxBackoff     = 60 * time.Second
)

// startSSE abre a conexão /api/sse/events e devolve o resultado como
// sseStartedMsg (ou sseClosedMsg se a conexão falhar de cara — o handler de
// sseClosedMsg já sabe agendar uma nova tentativa com backoff).
func startSSE(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		events, err := c.StreamEvents(ctx)
		if err != nil {
			cancel()
			return sseClosedMsg{}
		}
		return sseStartedMsg{ctx: ctx, cancel: cancel, events: events}
	}
}

// waitForSSE lê o próximo evento de um canal já aberto. Bubble Tea roda cada
// tea.Cmd na sua própria goroutine, então esse Cmd precisa ser reemitido a
// cada mensagem tratada em Update (nunca lido em loop dentro do próprio Cmd)
// — é essa reemissão que garante que o Model só é tocado a partir do loop
// Update de uma goroutine só, mesmo com um channel de vida longa por trás.
func waitForSSE(events <-chan client.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return sseClosedMsg{}
		}
		return sseEventMsg{event: ev}
	}
}

func sseReconnectAfter(backoff time.Duration) tea.Cmd {
	return tea.Tick(backoff, func(time.Time) tea.Msg {
		return sseReconnectMsg{}
	})
}
