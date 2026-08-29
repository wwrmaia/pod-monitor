package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type quotaResource struct {
	Hard string `json:"hard"`
	Used string `json:"used"`
}

type quotaInfo struct {
	Name      string                    `json:"name"`
	Resources map[string]*quotaResource `json:"resources"`
}

type namespaceQuota struct {
	Namespace string      `json:"namespace"`
	Quotas    []quotaInfo `json:"quotas"`
}

var quotasCmd = &cobra.Command{
	Use:   "quotas",
	Short: "ResourceQuotas por namespace (aba Quotas)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var nqs []namespaceQuota
		if err := c.Get("/api/quotas", client.Query(clusterQuery()...), &nqs); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(nqs)
		}
		var rows [][]string
		for _, nq := range nqs {
			for _, q := range nq.Quotas {
				for resName, r := range q.Resources {
					rows = append(rows, []string{nq.Namespace, q.Name, resName, r.Hard, r.Used})
				}
			}
		}
		output.Table([]string{"NAMESPACE", "QUOTA", "RECURSO", "HARD", "USADO"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(quotasCmd)
}
