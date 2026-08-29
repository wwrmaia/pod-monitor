package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

var namespacesCmd = &cobra.Command{
	Use:   "namespaces",
	Short: "Lista os namespaces de um cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var names []string
		if err := c.Get("/api/namespaces", client.Query("cluster", flagCluster), &names); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(names)
		}
		rows := make([][]string, len(names))
		for i, n := range names {
			rows[i] = []string{n}
		}
		output.Table([]string{"NAMESPACE"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(namespacesCmd)
}
