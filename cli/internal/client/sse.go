package client

import (
	"bufio"
	"context"
	"strings"
)

// Event é uma mensagem do stream de /api/sse/events — o backend manda
// "event: <nome>\ndata: <json>\n\n" (ver ssePublish em backend/main.go).
type Event struct {
	Name string
	Data string
}

// StreamEvents conecta em /api/sse/events e manda cada evento recebido pro
// canal retornado. Fecha o canal quando ctx é cancelado ou a conexão cai.
// Não usa nenhuma biblioteca de SSE — o formato já é texto plano simples o
// bastante pra um bufio.Scanner linha a linha.
func (c *Client) StreamEvents(ctx context.Context) (<-chan Event, error) {
	resp, err := c.RawGet(ctx, "/api/sse/events", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, &APIError{Status: resp.StatusCode}
	}

	events := make(chan Event, 8)
	go func() {
		defer resp.Body.Close()
		defer close(events)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var pending Event
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				pending.Name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				pending.Data = strings.TrimPrefix(line, "data: ")
			case line == "":
				if pending.Name != "" {
					select {
					case events <- pending:
					case <-ctx.Done():
						return
					}
				}
				pending = Event{}
			}
		}
	}()
	return events, nil
}
