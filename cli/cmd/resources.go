package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type containerResources struct {
	Name          string `json:"name"`
	CPURequest    string `json:"cpu_request"`
	MemoryRequest string `json:"memory_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryLimit   string `json:"memory_limit"`
	CPUUsage      string `json:"cpu_usage"`
	MemoryUsage   string `json:"memory_usage"`
}

type podResources struct {
	Name       string               `json:"pod"`
	Namespace  string               `json:"namespace"`
	Node       string               `json:"node"`
	Phase      string               `json:"phase"`
	Containers []containerResources `json:"containers"`
}

var resourcesCmd = &cobra.Command{
	Use:   "resources",
	Short: "Pods com requests/limits/uso de CPU e memória (aba Monitor)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var pods []podResources
		if err := c.Get("/api/resources", client.Query(clusterQuery()...), &pods); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(pods)
		}
		var rows [][]string
		for _, p := range pods {
			for _, ct := range p.Containers {
				rows = append(rows, []string{
					p.Namespace, p.Name, ct.Name, p.Node, p.Phase,
					ct.CPURequest, ct.CPULimit, ct.CPUUsage,
					ct.MemoryRequest, ct.MemoryLimit, ct.MemoryUsage,
				})
			}
		}
		output.Table([]string{
			"NAMESPACE", "POD", "CONTAINER", "NODE", "PHASE",
			"CPU_REQ", "CPU_LIM", "CPU_USE", "MEM_REQ", "MEM_LIM", "MEM_USE",
		}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(resourcesCmd)
}
