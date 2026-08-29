package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/output"
)

var clustersCmd = &cobra.Command{
	Use:   "clusters",
	Short: "Lista os clusters registrados no pod-monitor",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var names []string
		if err := c.Get("/api/clusters", nil, &names); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(names)
		}
		rows := make([][]string, len(names))
		for i, n := range names {
			rows[i] = []string{n}
		}
		output.Table([]string{"CLUSTER"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(clustersCmd)
}
