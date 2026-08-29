package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type analysisFinding struct {
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	ResourceKind string `json:"resource_kind"`
	ResourceName string `json:"resource_name"`
	Namespace    string `json:"namespace"`
	Message      string `json:"message"`
}

type analysisSummary struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

type analysisResult struct {
	Findings []analysisFinding `json:"findings"`
	Summary  analysisSummary   `json:"summary"`
}

var analysisCmd = &cobra.Command{
	Use:   "analysis",
	Short: "Roda a análise de boas práticas/segurança/confiabilidade (aba Análise)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var res analysisResult
		if err := c.Get("/api/analysis", client.Query(clusterQuery()...), &res); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(res)
		}
		fmt.Printf("Críticos: %d   Avisos: %d   Dicas: %d\n\n", res.Summary.Critical, res.Summary.Warning, res.Summary.Info)
		var rows [][]string
		for _, f := range res.Findings {
			rows = append(rows, []string{f.Severity, f.Category, f.ResourceKind, f.Namespace + "/" + f.ResourceName, f.Message})
		}
		output.Table([]string{"SEVERIDADE", "CATEGORIA", "TIPO", "RECURSO", "MENSAGEM"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(analysisCmd)
}
