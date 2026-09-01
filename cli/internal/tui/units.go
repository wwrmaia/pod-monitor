package tui

import (
	"strconv"
	"strings"
)

// Limiares de severidade aproximados — espelham o default do backend
// (getThreshold, warn=85/crit=90). O valor real por cluster/namespace só é
// exposto em GET /api/thresholds, que é adminOnly; a maioria dos usuários da
// TUI não consegue lê-lo, então esta é uma aproximação deliberada.
const (
	warnPct = 85
	critPct = 90
)

// parseCPUMilli decodifica uma quantidade de CPU do Kubernetes em millicores.
// cpu_request/cpu_limit normalmente vêm em cores ("1") ou millicores ("250m"),
// mas o cpu_usage relatado pelo metrics-server costuma vir em nanocores
// ("146268n") ou microcores ("u") para valores pequenos — os quatro sufixos
// decimalSI de CPU (n, u, m, sem sufixo) precisam ser cobertos, senão a
// severidade de qualquer pod com uso baixo fica sempre "sem dado" (achado ao
// testar contra um cluster real: /api/resources devolve cpu_usage em "n").
// ok=false significa "sem dado" (ex.: metrics-server fora do ar, campo
// vazio) — nunca deve ser tratado como 0.
func parseCPUMilli(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	units := []struct {
		suffix  string
		toMilli float64
	}{
		{"n", 1e-6}, // nanocores -> millicores
		{"u", 1e-3}, // microcores -> millicores
		{"m", 1},    // millicores
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			if err != nil {
				return 0, false
			}
			return v * u.toMilli, true
		}
	}
	v, err := strconv.ParseFloat(s, 64) // cores inteiros/fracionários, sem sufixo
	if err != nil {
		return 0, false
	}
	return v * 1000, true
}

// parseMemMiB decodifica uma quantidade de memória do Kubernetes ("128Mi",
// "1Gi", "512Ki", "1000000") em MiB. Mesma convenção de ok=false para "sem
// dado" que parseCPUMilli.
func parseMemMiB(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	units := []struct {
		suffix string
		toMiB  float64
	}{
		{"Ki", 1.0 / 1024},
		{"Mi", 1},
		{"Gi", 1024},
		{"Ti", 1024 * 1024},
		{"k", 1000.0 / (1024 * 1024)},
		{"M", 1000000.0 / (1024 * 1024)},
		{"G", 1000000000.0 / (1024 * 1024)},
		{"T", 1000000000000.0 / (1024 * 1024)},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			if err != nil {
				return 0, false
			}
			return v * u.toMiB, true
		}
	}
	v, err := strconv.ParseFloat(s, 64) // bytes puros
	if err != nil {
		return 0, false
	}
	return v / (1024 * 1024), true
}

// pctOf calcula usage/limit*100 usando o parser dado. ok=false se qualquer
// um dos dois campos estiver ausente/ilegível ou o limite for zero.
func pctOf(usage, limit string, parse func(string) (float64, bool)) (pct int, ok bool) {
	u, uOK := parse(usage)
	l, lOK := parse(limit)
	if !uOK || !lOK || l <= 0 {
		return 0, false
	}
	return int(u / l * 100), true
}

// sevForPct classifica um percentual em "", "warning" ou "critical".
func sevForPct(pct int, ok bool) string {
	if !ok {
		return ""
	}
	switch {
	case pct >= critPct:
		return "critical"
	case pct >= warnPct:
		return "warning"
	default:
		return ""
	}
}

// maxSeverity devolve a mais severa entre duas classificações.
func maxSeverity(a, b string) string {
	rank := map[string]int{"": 0, "warning": 1, "critical": 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
