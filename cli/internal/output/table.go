// Package output formata dados pra exibição — tabela (padrão) ou JSON.
// Fica isolado de client/config de propósito: é a única camada que a TUI
// (nível 2 do roadmap) vai substituir por completo.
package output

import (
	"fmt"
	"os"
	"text/tabwriter"
)

// Table imprime um cabeçalho e linhas alinhadas em colunas via text/tabwriter
// (biblioteca padrão do Go — sem dependência nova).
func Table(header []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println("(nenhum resultado)")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	defer w.Flush()
	printRow(w, header)
	for _, r := range rows {
		printRow(w, r)
	}
}

func printRow(w *tabwriter.Writer, cols []string) {
	for i, c := range cols {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, c)
	}
	fmt.Fprintln(w)
}
