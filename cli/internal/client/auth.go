package client

import "fmt"

type LoginResult struct {
	// Preenchido em caso de sucesso.
	Token             string   `json:"token"`
	Username          string   `json:"username"`
	Role              string   `json:"role"`
	AllowedClusters   []string `json:"allowed_clusters"`
	AllowedNamespaces []string `json:"allowed_namespaces"`

	// Preenchido quando o backend pede um segundo passo em vez do token.
	MFARequired      bool   `json:"mfa_required"`
	MFAToken         string `json:"mfa_token"`
	MFASetupRequired bool   `json:"mfa_setup_required"`
	SetupToken       string `json:"setup_token"`
}

// Login chama POST /api/auth/login. O resultado pode já vir com o token
// (sem MFA configurado) ou pedir um segundo passo — ver LoginResult.
func (c *Client) Login(username, password string) (*LoginResult, error) {
	var res LoginResult
	body := map[string]string{"username": username, "password": password}
	if err := c.do("POST", "/api/auth/login", nil, body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type mfaSetupInfo struct {
	Secret string `json:"secret"`
}

// MFASetupSecret busca o secret TOTP de um login que pediu configuração pela
// primeira vez (mfa_setup_required). Não renderiza QR no terminal na v1 —
// só imprime o secret pra digitar manualmente no autenticador.
func (c *Client) MFASetupSecret(setupToken string) (string, error) {
	var info mfaSetupInfo
	if err := c.Get("/api/auth/mfa/setup", Query("setup_token", setupToken), &info); err != nil {
		return "", err
	}
	return info.Secret, nil
}

// ValidateMFA troca o mfa_token + código TOTP pelo token de sessão de verdade.
func (c *Client) ValidateMFA(mfaToken, code string) (*LoginResult, error) {
	var res LoginResult
	body := map[string]string{"mfa_token": mfaToken, "code": code}
	if err := c.do("POST", "/api/auth/mfa/validate", nil, body, &res); err != nil {
		return nil, err
	}
	if res.Token == "" {
		return nil, fmt.Errorf("código MFA inválido")
	}
	return &res, nil
}

// Logout revoga o token atual no backend (o config local é limpo por quem
// chama, em internal/config).
func (c *Client) Logout() error {
	return c.do("POST", "/api/auth/logout", nil, nil, nil)
}
