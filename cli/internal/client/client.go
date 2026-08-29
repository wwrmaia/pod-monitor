// Package client fala com a API REST do backend do pod-monitor. É a camada
// que uma futura TUI (nível 2 do roadmap) reaproveita sem mudança nenhuma —
// não sabe nada sobre Cobra, tabela ou terminal.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"pod-monitor/cli/internal/config"
)

// APIError carrega o status HTTP pra quem chama poder distinguir 401 (token
// expirado, pedir novo login) de 403 (RBAC do usuário, não é erro de auth) de
// qualquer outra falha.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

type Client struct {
	Server string
	Token  string
	http   *http.Client
}

// New monta um Client a partir da config salva (~/.config/pod-monitor-cli).
// Server pode ser sobrescrito pela flag --server sem precisar logar de novo.
func New(cfg *config.Config, serverOverride string) (*Client, error) {
	server := cfg.Server
	if serverOverride != "" {
		server = serverOverride
	}
	if server == "" {
		return nil, fmt.Errorf("nenhum servidor configurado — rode 'podmon login --server http://host:porta' primeiro")
	}
	return &Client{
		Server: server,
		Token:  cfg.Token,
		http:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// NewUnauthenticated é usado só por "podmon login", antes de ter um token.
func NewUnauthenticated(server string) *Client {
	return &Client{Server: server, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) url(path string, query url.Values) string {
	u := c.Server + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (c *Client) do(method, path string, query url.Values, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.url(path, query), reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("não foi possível conectar em %s: %w", c.Server, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(respBody))}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("resposta inesperada do servidor: %w", err)
		}
	}
	return nil
}

// Query monta um url.Values a partir de pares chave/valor, ignorando
// entradas com valor vazio — evita cada comando repetir esse boilerplate.
func Query(pairs ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			v.Set(pairs[i], pairs[i+1])
		}
	}
	return v
}

// Get faz um GET autenticado e decodifica o JSON de resposta em out.
func (c *Client) Get(path string, query url.Values, out interface{}) error {
	return c.do(http.MethodGet, path, query, nil, out)
}

// Post faz um POST autenticado com corpo JSON e decodifica a resposta em out
// (pode ser nil se a chamada não devolve corpo relevante, ex.: logout).
func (c *Client) Post(path string, body interface{}, out interface{}) error {
	return c.do(http.MethodPost, path, nil, body, out)
}

// GetText faz um GET autenticado e devolve o corpo cru como string — usado só
// por /api/logs, que responde text/plain em vez de JSON.
func (c *Client) GetText(path string, query url.Values) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.url(path, query), nil)
	if err != nil {
		return "", err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("não foi possível conectar em %s: %w", c.Server, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", &APIError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(body))}
	}
	return string(body), nil
}

// RawGet devolve a resposta bruta — usado por SSE, que não é JSON de resposta
// única (é um stream de "event:"/"data:" linha a linha). Usa um http.Client
// próprio sem timeout total, já que a conexão fica aberta indefinidamente.
func (c *Client) RawGet(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.url(path, nil), nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	streamClient := &http.Client{} // sem Timeout — SSE é uma conexão longa
	return streamClient.Do(req)
}
