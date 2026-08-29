// Package config lida com o arquivo de configuração local da CLI
// (~/.config/pod-monitor-cli/config.yaml) — servidor, token, expiração e
// identidade do usuário logado. É a camada que uma futura TUI (nível 2 do
// roadmap) reaproveita sem mudança nenhuma.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server            string   `yaml:"server"`
	Token             string   `yaml:"token"`
	Username          string   `yaml:"username"`
	Role              string   `yaml:"role"`
	AllowedClusters   []string `yaml:"allowed_clusters,omitempty"`
	AllowedNamespaces []string `yaml:"allowed_namespaces,omitempty"`
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "pod-monitor-cli"), nil
}

func path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

// Load lê a config salva. Retorna uma Config zerada (sem erro) se o arquivo
// ainda não existir — chamadas que precisam de login tratam Token=="" como
// "não autenticado".
func Load() (*Config, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config inválida em %s: %w", p, err)
	}
	return &c, nil
}

// Save grava a config com permissão 0600 — o arquivo guarda um token Bearer,
// não pode ficar legível por outros usuários da máquina.
func Save(c *Config) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	p, err := path()
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// Clear remove a config local (usado por "podmon logout").
func Clear() error {
	p, err := path()
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
