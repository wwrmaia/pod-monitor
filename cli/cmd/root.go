// Package cmd define os subcomandos da CLI via Cobra. Esta é a camada de
// apresentação — fala só com internal/client e internal/output, nunca faz
// requisição HTTP diretamente. É a camada que será substituída quando a TUI
// (nível 2 do roadmap) entrar; client/config ficam intactos.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/config"
)

var (
	flagServer    string
	flagOutput    string
	flagCluster   string
	flagNamespace string
)

var rootCmd = &cobra.Command{
	Use:   "podmon",
	Short: "CLI do Pod Monitor — acesso de terminal ao mesmo backend do dashboard web",
	Long: `podmon fala com a mesma API REST usada pelo dashboard web do Pod Monitor.
Rode "podmon login --server http://host:porta" primeiro.`,
	SilenceUsage: true,
}

// Execute é chamado por main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagServer, "server", "", "URL do backend (sobrescreve o servidor salvo no login)")
	rootCmd.PersistentFlags().StringVar(&flagOutput, "output", "table", "Formato de saída: table ou json")
	rootCmd.PersistentFlags().StringVar(&flagCluster, "cluster", "", "Cluster monitorado pelo pod-monitor (vazio = primeiro disponível)")
	rootCmd.PersistentFlags().StringVarP(&flagNamespace, "namespace", "n", "", "Namespace (vazio = todos, quando suportado)")
}

// authedClient carrega a config salva e monta um Client autenticado — usado
// por todo comando que não seja login/logout.
func authedClient() (*client.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Token == "" && flagServer == "" {
		return nil, fmt.Errorf("não autenticado — rode 'podmon login --server http://host:porta'")
	}
	return client.New(cfg, flagServer)
}

// isJSON reporta se a flag global --output pede JSON.
func isJSON() bool {
	return flagOutput == "json"
}

// clusterQuery monta os pares cluster/namespace já prontos pra client.Query,
// reaproveitado pela maioria dos comandos de leitura.
func clusterQuery(extra ...string) []string {
	pairs := []string{"cluster", flagCluster, "namespace", flagNamespace}
	return append(pairs, extra...)
}
