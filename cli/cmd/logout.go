package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/config"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Revoga a sessão atual e limpa a configuração local",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			// já não está logado — nada a revogar, só garante que o arquivo local sumiu.
			return config.Clear()
		}
		if err := c.Logout(); err != nil {
			fmt.Println("aviso: não consegui revogar o token no servidor:", err)
		}
		if err := config.Clear(); err != nil {
			return err
		}
		fmt.Println("Sessão encerrada.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
