package cmd

import (
	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type orphanedResources struct {
	PVCs []struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Age       string `json:"age"`
	} `json:"pvcs"`
	Services []struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		Age       string `json:"age"`
	} `json:"services"`
	ConfigMaps []struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Age       string `json:"age"`
	} `json:"config_maps"`
	Secrets []struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Age       string `json:"age"`
	} `json:"secrets"`
	Ingresses []struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Host      string `json:"host"`
		Age       string `json:"age"`
	} `json:"ingresses"`
	ServiceAccounts []struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Age       string `json:"age"`
	} `json:"service_accounts"`
}

var orphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Recursos órfãos/não referenciados (aba Auditoria)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var o orphanedResources
		if err := c.Get("/api/orphans", client.Query(clusterQuery()...), &o); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(o)
		}
		var rows [][]string
		for _, x := range o.PVCs {
			rows = append(rows, []string{"PVC", x.Namespace, x.Name, x.Age})
		}
		for _, x := range o.Services {
			rows = append(rows, []string{"Service", x.Namespace, x.Name, x.Age})
		}
		for _, x := range o.ConfigMaps {
			rows = append(rows, []string{"ConfigMap", x.Namespace, x.Name, x.Age})
		}
		for _, x := range o.Secrets {
			rows = append(rows, []string{"Secret", x.Namespace, x.Name, x.Age})
		}
		for _, x := range o.Ingresses {
			rows = append(rows, []string{"Ingress", x.Namespace, x.Name, x.Age})
		}
		for _, x := range o.ServiceAccounts {
			rows = append(rows, []string{"ServiceAccount", x.Namespace, x.Name, x.Age})
		}
		output.Table([]string{"TIPO", "NAMESPACE", "NOME", "IDADE"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(orphansCmd)
}
