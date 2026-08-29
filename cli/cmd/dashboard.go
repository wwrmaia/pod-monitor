package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type dashWidgetContainer struct {
	Name      string `json:"name"`
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	CPUUsage  string `json:"cpu_usage"`
	MemUsage  string `json:"mem_usage"`
	Pct       int    `json:"pct"`
}

type dashSummary struct {
	Pods struct {
		Statuses map[string]int `json:"statuses"`
	} `json:"pods"`
	Nodes struct {
		Ready    int `json:"ready"`
		NotReady int `json:"not_ready"`
		Total    int `json:"total"`
	} `json:"nodes"`
	TopCPU []dashWidgetContainer `json:"top_cpu"`
	TopMem []dashWidgetContainer `json:"top_mem"`
	Alerts struct {
		Critical int `json:"critical"`
		Warning  int `json:"warning"`
	} `json:"alerts"`
}

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Resumo agregado do cluster: pods, nodes, alertas, top CPU/mem",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var d dashSummary
		if err := c.Get("/api/dashboard/summary", client.Query("cluster", flagCluster), &d); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(d)
		}
		fmt.Printf("Nodes: %d prontos / %d total   |   Alertas: %d críticos, %d avisos\n",
			d.Nodes.Ready, d.Nodes.Total, d.Alerts.Critical, d.Alerts.Warning)
		fmt.Print("Pods por fase: ")
		for phase, n := range d.Pods.Statuses {
			fmt.Printf("%s=%d  ", phase, n)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}
