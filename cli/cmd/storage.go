package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type pvcInfo struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Capacity     string `json:"capacity"`
	Status       string `json:"status"`
	StorageClass string `json:"storage_class"`
	AccessModes  string `json:"access_modes"`
}

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "PersistentVolumeClaims do cluster (aba Storage)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var pvcs []pvcInfo
		if err := c.Get("/api/storage", client.Query("cluster", flagCluster), &pvcs); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(pvcs)
		}
		var rows [][]string
		for _, p := range pvcs {
			rows = append(rows, []string{p.Namespace, p.Name, p.Capacity, p.Status, p.StorageClass, p.AccessModes})
		}
		output.Table([]string{"NAMESPACE", "PVC", "CAPACIDADE", "STATUS", "STORAGE_CLASS", "ACCESS_MODES"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(storageCmd)
}
