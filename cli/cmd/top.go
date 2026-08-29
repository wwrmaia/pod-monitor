package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Top consumidores de CPU e memória (aba Top 10)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var d struct {
			TopCPU []dashWidgetContainer `json:"top_cpu"`
			TopMem []dashWidgetContainer `json:"top_mem"`
		}
		if err := c.Get("/api/dashboard/summary", client.Query("cluster", flagCluster), &d); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(d)
		}
		var rows [][]string
		for _, w := range d.TopCPU {
			rows = append(rows, []string{"CPU", w.Namespace, w.Pod, w.Name, w.CPUUsage})
		}
		for _, w := range d.TopMem {
			rows = append(rows, []string{"MEM", w.Namespace, w.Pod, w.Name, w.MemUsage})
		}
		output.Table([]string{"RECURSO", "NAMESPACE", "POD", "CONTAINER", "USO"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(topCmd)
}
