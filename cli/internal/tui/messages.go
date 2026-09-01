package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"pod-monitor/cli/internal/client"
)

type clustersLoadedMsg struct {
	clusters []string
}

type namespacesLoadedMsg struct {
	cluster    string
	namespaces []string
}

type resourcesLoadedMsg struct {
	cluster   string
	namespace string
	pods      []podResources
}

type dashboardLoadedMsg struct {
	cluster string
	summary dashSummary
}

type nodesLoadedMsg struct {
	cluster string
	nodes   []nodeResources
}

type storageLoadedMsg struct {
	cluster string
	pvcs    []pvcInfo
}

type costsLoadedMsg struct {
	cluster   string
	namespace string
	costs     costsResponse
}

type quotasLoadedMsg struct {
	cluster   string
	namespace string
	quotas    []namespaceQuota
}

type orphansLoadedMsg struct {
	cluster string
	orphans orphanedResources
}

type certsLoadedMsg struct {
	cluster   string
	namespace string
	certs     []certInfo
}

type tickMsg struct {
	forScreen screen
}

type sseStartedMsg struct {
	ctx    context.Context
	cancel context.CancelFunc
	events <-chan client.Event
}

type sseEventMsg struct {
	event client.Event
}

type sseClosedMsg struct{}

type sseReconnectMsg struct{}

type errMsg struct {
	err error
}

type authExpiredMsg struct{}

// classifyErr distingue 401 (token expirado) de qualquer outro erro — o
// client.APIError já carrega o status HTTP pra isso.
func classifyErr(err error) tea.Msg {
	if apiErr, ok := err.(*client.APIError); ok && apiErr.Status == 401 {
		return authExpiredMsg{}
	}
	return errMsg{err: err}
}

func fetchClusters(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		var clusters []string
		if err := c.Get("/api/clusters", nil, &clusters); err != nil {
			return classifyErr(err)
		}
		return clustersLoadedMsg{clusters: clusters}
	}
}

func fetchNamespaces(c *client.Client, cluster string) tea.Cmd {
	return func() tea.Msg {
		var namespaces []string
		if err := c.Get("/api/namespaces", client.Query("cluster", cluster), &namespaces); err != nil {
			return classifyErr(err)
		}
		return namespacesLoadedMsg{cluster: cluster, namespaces: namespaces}
	}
}

func fetchResources(c *client.Client, cluster, namespace string) tea.Cmd {
	return func() tea.Msg {
		var pods []podResources
		if err := c.Get("/api/resources", client.Query("cluster", cluster, "namespace", namespace), &pods); err != nil {
			return classifyErr(err)
		}
		return resourcesLoadedMsg{cluster: cluster, namespace: namespace, pods: pods}
	}
}

func fetchDashboard(c *client.Client, cluster string) tea.Cmd {
	return func() tea.Msg {
		var d dashSummary
		if err := c.Get("/api/dashboard/summary", client.Query("cluster", cluster), &d); err != nil {
			return classifyErr(err)
		}
		return dashboardLoadedMsg{cluster: cluster, summary: d}
	}
}

func fetchNodes(c *client.Client, cluster string) tea.Cmd {
	return func() tea.Msg {
		var nodes []nodeResources
		if err := c.Get("/api/nodes", client.Query("cluster", cluster), &nodes); err != nil {
			return classifyErr(err)
		}
		return nodesLoadedMsg{cluster: cluster, nodes: nodes}
	}
}

func fetchStorage(c *client.Client, cluster string) tea.Cmd {
	return func() tea.Msg {
		var pvcs []pvcInfo
		if err := c.Get("/api/storage", client.Query("cluster", cluster), &pvcs); err != nil {
			return classifyErr(err)
		}
		return storageLoadedMsg{cluster: cluster, pvcs: pvcs}
	}
}

func fetchCosts(c *client.Client, cluster, namespace string) tea.Cmd {
	return func() tea.Msg {
		var resp costsResponse
		if err := c.Get("/api/costs", client.Query("cluster", cluster, "namespace", namespace), &resp); err != nil {
			return classifyErr(err)
		}
		return costsLoadedMsg{cluster: cluster, namespace: namespace, costs: resp}
	}
}

func fetchQuotas(c *client.Client, cluster, namespace string) tea.Cmd {
	return func() tea.Msg {
		var quotas []namespaceQuota
		if err := c.Get("/api/quotas", client.Query("cluster", cluster, "namespace", namespace), &quotas); err != nil {
			return classifyErr(err)
		}
		return quotasLoadedMsg{cluster: cluster, namespace: namespace, quotas: quotas}
	}
}

// fetchOrphans não manda "namespace" — o backend (handleOrphans) nunca lê
// esse parâmetro, sempre lista o cluster inteiro (ver plano). Mandar mesmo
// assim só passaria uma falsa impressão de filtro que não existe.
func fetchOrphans(c *client.Client, cluster string) tea.Cmd {
	return func() tea.Msg {
		var orphans orphanedResources
		if err := c.Get("/api/orphans", client.Query("cluster", cluster), &orphans); err != nil {
			return classifyErr(err)
		}
		return orphansLoadedMsg{cluster: cluster, orphans: orphans}
	}
}

func fetchCerts(c *client.Client, cluster, namespace string) tea.Cmd {
	return func() tea.Msg {
		var certs []certInfo
		if err := c.Get("/api/certificates", client.Query("cluster", cluster, "namespace", namespace), &certs); err != nil {
			return classifyErr(err)
		}
		return certsLoadedMsg{cluster: cluster, namespace: namespace, certs: certs}
	}
}

// tickAfter arma um tea.Tick marcado com a tela a que pertence — o handler de
// tickMsg descarta o tick (não rearma) se a tela atual já mudou, o que evita
// polling de telas que não estão mais visíveis (ver §4 do plano: dashboard só
// deve pollar enquanto o overlay está aberto).
func tickAfter(d time.Duration, forScreen screen) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return tickMsg{forScreen: forScreen}
	})
}
