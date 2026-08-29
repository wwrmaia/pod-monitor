package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"pod-monitor/cli/internal/client"
	"pod-monitor/cli/internal/config"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Autentica no backend do Pod Monitor e salva a sessão localmente",
	RunE:  runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	server := flagServer
	if server == "" {
		if cfg, err := config.Load(); err == nil && cfg.Server != "" {
			server = cfg.Server
		}
	}
	if server == "" {
		return fmt.Errorf("informe --server http://host:porta na primeira vez que logar")
	}
	server = strings.TrimRight(server, "/")

	username, err := readLine("Usuário: ")
	if err != nil {
		return err
	}

	password, err := readPassword("Senha: ")
	if err != nil {
		return err
	}

	c := client.NewUnauthenticated(server)
	res, err := c.Login(username, password)
	if err != nil {
		return err
	}

	switch {
	case res.MFASetupRequired:
		secret, err := c.MFASetupSecret(res.SetupToken)
		if err != nil {
			return fmt.Errorf("login pediu configuração de MFA, mas não consegui buscar o secret: %w", err)
		}
		fmt.Println()
		fmt.Println("Este usuário ainda não tem MFA configurado. Secret TOTP (adicione manualmente")
		fmt.Println("no seu autenticador — pra escanear QR, use a interface web uma vez):")
		fmt.Println()
		fmt.Println("  " + secret)
		fmt.Println()
		return fmt.Errorf("configure o autenticador com o secret acima e rode 'podmon login' de novo")

	case res.MFARequired:
		code, err := readLine("Código MFA: ")
		if err != nil {
			return err
		}
		res, err = c.ValidateMFA(res.MFAToken, code)
		if err != nil {
			return err
		}
	}

	if res.Token == "" {
		return fmt.Errorf("login não retornou um token válido")
	}

	newCfg := &config.Config{
		Server:            server,
		Token:             res.Token,
		Username:          res.Username,
		Role:              res.Role,
		AllowedClusters:   res.AllowedClusters,
		AllowedNamespaces: res.AllowedNamespaces,
	}
	if err := config.Save(newCfg); err != nil {
		return fmt.Errorf("login funcionou mas não consegui salvar a sessão: %w", err)
	}
	fmt.Printf("Login ok — %s (%s) em %s\n", res.Username, res.Role, server)
	return nil
}

// stdin é compartilhado por todos os prompts de um mesmo comando — um
// bufio.Reader novo a cada chamada descartaria bytes já lidos do buffer
// interno (ex.: usuário+senha entregues juntos por um pipe), quebrando o
// prompt seguinte (MFA) mesmo em uso interativo normal.
var stdin = bufio.NewReader(os.Stdin)

func readLine(prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := stdin.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// readPassword lê a senha sem ecoar no terminal (mesma técnica do próprio
// kubectl/ssh — term.ReadPassword desliga o echo do tty). Se stdin não for um
// terminal de verdade (pipe, script, CI), cai pra leitura de linha normal —
// mesma tolerância que ferramentas como o gh CLI têm pra uso não-interativo.
func readPassword(prompt string) (string, error) {
	if !term.IsTerminal(int(syscall.Stdin)) {
		return readLine(prompt)
	}
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
