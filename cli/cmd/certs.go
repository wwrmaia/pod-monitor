package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/output"
)

type certInfo struct {
	Cluster    string `json:"cluster"`
	Namespace  string `json:"namespace"`
	SecretName string `json:"secret_name"`
	CommonName string `json:"common_name"`
	Issuer     string `json:"issuer"`
	NotAfter   string `json:"not_after"`
	DaysLeft   int    `json:"days_left"`
	Bucket     string `json:"bucket"`
}

var certsCmd = &cobra.Command{
	Use:   "certs",
	Short: "Certificados TLS e dias restantes até expirar (aba Certificados)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := authedClient()
		if err != nil {
			return err
		}
		var certs []certInfo
		if err := c.Get("/api/certificates", client.Query(clusterQuery()...), &certs); err != nil {
			return err
		}
		if isJSON() {
			return output.JSON(certs)
		}
		var rows [][]string
		for _, ct := range certs {
			rows = append(rows, []string{
				ct.Namespace, ct.SecretName, ct.CommonName, ct.Issuer,
				fmt.Sprintf("%d", ct.DaysLeft), ct.Bucket,
			})
		}
		output.Table([]string{"NAMESPACE", "SECRET", "DOMÍNIO (CN)", "EMISSOR", "DIAS_RESTANTES", "BUCKET"}, rows)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(certsCmd)
}
