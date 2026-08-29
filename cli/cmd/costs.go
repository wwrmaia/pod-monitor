package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type namespaceCost struct {
	Namespace string  `json:"namespace"`
	CPUCores  float64 `json:"cpu_cores"`
	MemGiB    float64 `json:"mem_gib"`
	CostHour  float64 `json:"cost_hour"`
	CostMonth float64 `json:"cost_month"`
}

type costsResponse struct {
	Enabled    bool            `json:"enabled"`
	Configured bool            `json:"configured"`
	Currency   string          `json:"currency"`
	Items      []namespaceCost `json:"items"`
}

var costsCmd = &cobra.Command{
	Use:   "costs",
	Short: "Estimativa de custo por namespace (aba Custos)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var res costsResponse
		if err := c.Get("/api/costs", client.Query(clusterQuery()...), &res); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(res)
		}
		if !res.Enabled {
			fmt.Println("Estimativa de custo desativada para este cluster (ative em Admin › Custos na web — só faz sentido em cluster de nuvem com billing por hora).")
			return nil
		}
		if !res.Configured {
			fmt.Println("Custo ativado, mas nenhum preço configurado ainda (Admin › Custos na web).")
			return nil
		}
		var rows [][]string
		for _, i := range res.Items {
			rows = append(rows, []string{
				i.Namespace,
				fmt.Sprintf("%.2f", i.CPUCores),
				fmt.Sprintf("%.2f", i.MemGiB),
				fmt.Sprintf("%s %.2f", res.Currency, i.CostHour),
				fmt.Sprintf("%s %.2f", res.Currency, i.CostMonth),
			})
		}
		output.Table([]string{"NAMESPACE", "CPU_CORES", "MEM_GIB", "CUSTO/HORA", "CUSTO/MÊS"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(costsCmd)
}
