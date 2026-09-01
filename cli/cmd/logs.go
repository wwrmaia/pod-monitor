package cmd

import (
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
)

var (
	flagLogsContainer string
	flagLogsTail      string
	flagLogsFollow    bool
)

var logsCmd = &cobra.Command{
	Use:   "logs <pod>",
	Short: "Logs de um pod — snapshot único, ou -f para seguir em tempo real",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagNamespace == "" {
			return fmt.Errorf("--namespace é obrigatório pra 'podmon logs'")
		}
		c, err := authedClient()
		if err != nil {
			return err
		}
		query := client.Query(
			"cluster", flagCluster,
			"namespace", flagNamespace,
			"pod", args[0],
			"container", flagLogsContainer,
			"tail", flagLogsTail,
		)
		if !flagLogsFollow {
			text, err := c.GetText("/api/logs", query)
			if err != nil {
				return err
			}
			fmt.Print(text)
			return nil
		}

		// -f: Ctrl+C cancela o ctx em vez de matar o processo na marra — sem
		// isso a conexão HTTP ficaria pendurada até o servidor decidir
		// fechar (ver StreamLogs/handleLogsStream).
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer stop()
		body, err := c.StreamLogs(ctx, query)
		if err != nil {
			return err
		}
		defer body.Close()
		if _, err := io.Copy(os.Stdout, body); err != nil && ctx.Err() == nil {
			return err
		}
		return nil
	},
}

func init() {
	logsCmd.Flags().StringVar(&flagLogsContainer, "container", "", "Container específico (vazio = primeiro do pod)")
	logsCmd.Flags().StringVar(&flagLogsTail, "tail", "", "Número de linhas (vazio = 200, padrão do servidor; ignorado sem -f só se o backend também ignorar)")
	logsCmd.Flags().BoolVarP(&flagLogsFollow, "follow", "f", false, "Segue os logs em tempo real (Ctrl+C para sair)")
	rootCmd.AddCommand(logsCmd)
}
