package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Acompanha em tempo real os eventos do pod-monitor (SSE) — Ctrl+C pra sair",
	Long: `Conecta em /api/sse/events e imprime cada evento assim que chega:
alertas de recursos (critical/warning), certificados, auditoria, restart_storm,
oom_killed, e as atualizações periódicas de dashboard/topologia.

Isso é uma leitura crua do stream — sem interatividade, sem drill-down. Pra
navegação/filtragem em tempo real, essa é justamente a lacuna que a futura TUI
(nível 2 do roadmap) resolve.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		events, err := c.StreamEvents(ctx)
		if err != nil {
			return err
		}
		fmt.Println("Conectado — aguardando eventos (Ctrl+C pra sair)...")
		for ev := range events {
			fmt.Printf("[%s] %-16s %s\n", time.Now().Format("15:04:05"), ev.Name, ev.Data)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(eventsCmd)
}
