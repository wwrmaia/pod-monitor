package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type nodeResources struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Role           string `json:"role"`
	CPUAllocatable string `json:"cpu_allocatable"`
	MemAllocatable string `json:"mem_allocatable"`
	CPUUsage       string `json:"cpu_usage"`
	MemUsage       string `json:"mem_usage"`
}

var nodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "Nodes do cluster com capacidade e uso",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var nodes []nodeResources
		if err := c.Get("/api/nodes", client.Query("cluster", flagCluster), &nodes); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(nodes)
		}
		var rows [][]string
		for _, n := range nodes {
			rows = append(rows, []string{n.Name, n.Status, n.Role, n.CPUAllocatable, n.MemAllocatable, n.CPUUsage, n.MemUsage})
		}
		output.Table([]string{"NODE", "STATUS", "ROLE", "CPU_ALLOC", "MEM_ALLOC", "CPU_USE", "MEM_USE"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(nodesCmd)
}
