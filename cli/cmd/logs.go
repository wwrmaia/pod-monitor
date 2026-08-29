package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
)

var (
	flagLogsContainer string
	flagLogsTail      string
)

var logsCmd = &cobra.Command{
	Use:   "logs <pod>",
	Short: "Logs de um pod — snapshot único, não é streaming (ver README)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagNamespace == "" {
			return fmt.Errorf("--namespace é obrigatório pra 'podmon logs'")
		}
		c, err := authedClient()
		if err != nil {
			return err
		}
		text, err := c.GetText("/api/logs", client.Query(
			"cluster", flagCluster,
			"namespace", flagNamespace,
			"pod", args[0],
			"container", flagLogsContainer,
			"tail", flagLogsTail,
		))
		if err != nil {
			return err
		}
		fmt.Print(text)
		return nil
	},
}

func init() {
	logsCmd.Flags().StringVar(&flagLogsContainer, "container", "", "Container específico (vazio = primeiro do pod)")
	logsCmd.Flags().StringVar(&flagLogsTail, "tail", "", "Número de linhas (vazio = 200, padrão do servidor)")
	rootCmd.AddCommand(logsCmd)
}
