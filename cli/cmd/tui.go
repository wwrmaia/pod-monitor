package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Painel interativo estilo k9s — navegação ao vivo entre clusters/namespaces/pods",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		return tui.Run(c, flagCluster, flagNamespace)
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
