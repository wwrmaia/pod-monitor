package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"pod-monitor/cli/internal/client"
)

type screen int

const (
	screenClusters screen = iota
	screenNamespaces
	screenPods
	screenDashboard
	screenNodes
	screenCosts
	screenStorage
	screenQuotas
	screenOrphans
	screenCerts
	screenHelp
)

// isBaseScreen reporta se s é uma das 3 telas do fluxo linear principal
// (cluster → namespace → pods) — as demais são "overlays" que se abrem de
// qualquer uma delas e voltam pra elas ao fechar (ver openOverlay/isOverlayScreen).
func isBaseScreen(s screen) bool {
	return s == screenClusters || s == screenNamespaces || s == screenPods
}

func isOverlayScreen(s screen) bool {
	return !isBaseScreen(s)
}

const (
	resourcesPollInterval = 8 * time.Second
	dashboardPollInterval = 10 * time.Second
)

type model struct {
	client *client.Client

	// só alimentam o header (barra de contexto) — nenhuma lógica de
	// autorização depende disso na TUI, o backend já fez RBAC.
	username, role string

	screen     screen
	prevScreen screen

	cluster   string
	namespace string

	clusterList list.Model
	nsList      list.Model
	podTable    table.Model
	filterInput textinput.Model
	filtering   bool

	spinner spinner.Model

	commanding bool
	cmdInput   textinput.Model

	pods    []podResources
	sortCol int // 0=nome, 1=CPU%, 2=MEM%

	// loading* distingue "ainda buscando" de "lista genuinamente vazia" —
	// sem isso a tela mostra "(nenhum resultado)" por um instante a cada
	// navegação, antes do primeiro fetch responder (visto ao testar
	// interativamente). Só cobre o fetch inicial de cada tela; refresh em
	// segundo plano (tick/"r") mantém o último dado bom visível.
	loadingClusters   bool
	loadingNamespaces bool
	loadingPods       bool

	dash            dashSummary
	dashLoaded      bool
	lastDashFetchAt time.Time

	nodes     []nodeResources
	nodeTable table.Model

	storage      []pvcInfo
	storageTable table.Model

	costs       costsResponse
	costsLoaded bool
	costsTable  table.Model

	quotas      []namespaceQuota
	quotasTable table.Model

	orphans      orphanedResources
	orphansTable table.Model

	certs      []certInfo
	certsTable table.Model

	loadingNodes, loadingStorage, loadingCosts, loadingQuotas, loadingOrphans, loadingCerts bool

	sseCtx     context.Context
	sseCancel  context.CancelFunc
	sseEvents  <-chan client.Event
	sseBackoff time.Duration

	err           string
	authExpired   bool
	width, height int

	// pré-seleção vinda de --cluster/--namespace; consumida assim que a
	// respectiva lista carrega, ver clustersLoadedMsg/namespacesLoadedMsg.
	presetCluster   string
	presetNamespace string
}

func newModel(c *client.Client, username, role, presetCluster, presetNamespace string) model {
	fi := textinput.New()
	fi.Placeholder = "filtrar por pod/container..."

	ci := textinput.New()
	ci.Prompt = ":"
	ci.Placeholder = "nodes, storage, quotas, orphans, costs, certs, dashboard, help..."

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = logoStyle

	return model{
		client:          c,
		username:        username,
		role:            role,
		screen:          screenClusters,
		clusterList:     newSelectList("Clusters"),
		nsList:          newSelectList("Namespaces"),
		podTable:        newTable(),
		nodeTable:       newTable(),
		storageTable:    newTable(),
		costsTable:      newTable(),
		quotasTable:     newTable(),
		orphansTable:    newTable(),
		certsTable:      newTable(),
		filterInput:     fi,
		cmdInput:        ci,
		spinner:         sp,
		sseBackoff:      sseInitialBackoff,
		presetCluster:   presetCluster,
		presetNamespace: presetNamespace,
		loadingClusters: true,
	}
}

func newSelectList(title string) list.Model {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = title
	l.SetShowHelp(false)
	return l
}

func newTable() table.Model {
	return table.New(table.WithFocused(true), table.WithStyles(tableStyles()))
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchClusters(m.client), startSSE(m.client), m.spinner.Tick)
}

func toListItems(names []string) []list.Item {
	items := make([]list.Item, len(names))
	for i, n := range names {
		items[i] = simpleItem(n)
	}
	return items
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

		// Largura/altura de conteúdo descontam a borda do painel (item de
		// polimento visual) e, quando o header está visível (terminal alto o
		// bastante — ver renderHeader), as linhas que ele consome. Números
		// de partida — ajustados olhando a renderização real, layout
		// vertical não dá pra acertar só lendo código.
		//
		// -1 extra de margem de segurança: com contentWidth+panelOverheadWidth
		// batendo exatamente na largura do terminal (zero folga), a borda
		// direita do painel some — o terminal quebra a linha sozinho bem no
		// último caractere antes de desenhá-la (visto ao testar num terminal
		// de 140 colunas). Sobrar 1 coluna evita o encontro exato.
		contentWidth := msg.Width - panelOverheadWidth - 1
		if contentWidth < 20 {
			contentWidth = msg.Width
		}
		reserved := 4 + panelOverheadHeight // linha de título da tela + rodapé de ajuda + borda
		if msg.Height >= headerMinHeight {
			reserved += headerHeight
		}
		contentHeight := msg.Height - reserved
		if contentHeight < 5 {
			contentHeight = 5
		}

		m.clusterList.SetSize(contentWidth, contentHeight)
		m.nsList.SetSize(contentWidth, contentHeight)
		for _, t := range []*table.Model{&m.podTable, &m.nodeTable, &m.storageTable, &m.costsTable, &m.quotasTable, &m.orphansTable, &m.certsTable} {
			t.SetWidth(contentWidth)
			t.SetHeight(contentHeight)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		// Fica ticando pra sempre desde o Init() — mais simples do que
		// iniciar/parar o tick em cada um dos ~10 pontos que setam algum
		// loading*=true, e sem efeito visível quando ocioso (só é
		// *renderizado* — ver loadingOrEmpty em styles.go — quando algum
		// loading* está true; o tick em si é barato pra uma TUI em
		// primeiro plano).
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case clustersLoadedMsg:
		m.clusterList.SetItems(toListItems(msg.clusters))
		m.loadingClusters = false
		m.err = ""
		if m.presetCluster != "" {
			for _, c := range msg.clusters {
				if c == m.presetCluster {
					m.cluster = c
					m.screen = screenNamespaces
					m.loadingNamespaces = true
					return m, fetchNamespaces(m.client, m.cluster)
				}
			}
			m.presetCluster = "" // preset não bate com nenhum cluster disponível
		}
		return m, nil

	case namespacesLoadedMsg:
		if msg.cluster != m.cluster {
			return m, nil // resposta de uma seleção de cluster já superada
		}
		m.nsList.SetItems(toListItems(append([]string{allNamespacesLabel}, msg.namespaces...)))
		m.loadingNamespaces = false
		m.err = ""
		if m.presetCluster != "" || m.presetNamespace != "" {
			m.namespace = m.presetNamespace
			m.presetCluster, m.presetNamespace = "", ""
			m.screen = screenPods
			m.loadingPods = true
			m.refreshPodTable()
			return m, tea.Batch(fetchResources(m.client, m.cluster, m.namespace), tickAfter(resourcesPollInterval, screenPods))
		}
		return m, nil

	case resourcesLoadedMsg:
		if msg.cluster != m.cluster || msg.namespace != m.namespace {
			return m, nil
		}
		m.pods = msg.pods
		m.refreshPodTable()
		m.loadingPods = false
		m.err = ""
		return m, nil

	case dashboardLoadedMsg:
		if msg.cluster != m.cluster {
			return m, nil
		}
		m.dash = msg.summary
		m.dashLoaded = true
		m.err = ""
		return m, nil

	case nodesLoadedMsg:
		if msg.cluster != m.cluster {
			return m, nil
		}
		m.nodes = msg.nodes
		m.refreshNodesTable()
		m.loadingNodes = false
		m.err = ""
		return m, nil

	case storageLoadedMsg:
		if msg.cluster != m.cluster {
			return m, nil
		}
		m.storage = msg.pvcs
		m.refreshStorageTable()
		m.loadingStorage = false
		m.err = ""
		return m, nil

	case costsLoadedMsg:
		if msg.cluster != m.cluster || msg.namespace != m.namespace {
			return m, nil
		}
		m.costs = msg.costs
		m.costsLoaded = true
		m.refreshCostsTable()
		m.loadingCosts = false
		m.err = ""
		return m, nil

	case quotasLoadedMsg:
		if msg.cluster != m.cluster || msg.namespace != m.namespace {
			return m, nil
		}
		m.quotas = msg.quotas
		m.refreshQuotasTable()
		m.loadingQuotas = false
		m.err = ""
		return m, nil

	case orphansLoadedMsg:
		if msg.cluster != m.cluster {
			return m, nil
		}
		m.orphans = msg.orphans
		m.refreshOrphansTable()
		m.loadingOrphans = false
		m.err = ""
		return m, nil

	case certsLoadedMsg:
		if msg.cluster != m.cluster || msg.namespace != m.namespace {
			return m, nil
		}
		m.certs = msg.certs
		m.refreshCertsTable()
		m.loadingCerts = false
		m.err = ""
		return m, nil

	case tickMsg:
		if m.authExpired || msg.forScreen != m.screen {
			return m, nil // tela mudou desde que o tick foi armado — não rearma
		}
		switch msg.forScreen {
		case screenPods:
			return m, tea.Batch(fetchResources(m.client, m.cluster, m.namespace), tickAfter(resourcesPollInterval, screenPods))
		case screenDashboard:
			m.lastDashFetchAt = time.Now()
			return m, tea.Batch(fetchDashboard(m.client, m.cluster), tickAfter(dashboardPollInterval, screenDashboard))
		}
		return m, nil

	case sseStartedMsg:
		m.sseCtx, m.sseCancel, m.sseEvents = msg.ctx, msg.cancel, msg.events
		m.sseBackoff = sseInitialBackoff
		return m, waitForSSE(m.sseEvents)

	case sseEventMsg:
		cmds := []tea.Cmd{waitForSSE(m.sseEvents)}
		switch msg.event.Name {
		case "summary":
			// O payload não identifica o cluster (ver plano §4) — tratamos
			// qualquer "summary" como "algo mudou em algum lugar, re-busque
			// a própria seleção atual", nunca desserializamos o Data.
			if m.screen == screenPods {
				cmds = append(cmds, fetchResources(m.client, m.cluster, m.namespace))
			}
			// Debounce contra loop de auto-amplificação: /api/dashboard/summary
			// é o que PUBLICA este mesmo evento "summary" via SSE (ver
			// backend main.go, handleDashboardSummary) — sem esse debounce,
			// esta própria busca reaciona o evento que ela mesma gera, e
			// qualquer TUI parada na tela de dashboard vira um loop
			// autossustentado (visto na prática: ~1-2 req/s ininterruptos,
			// e cada uma redispara os webhooks de alerta crítico sem dedup
			// no backend — gerou uma tempestade de e-mail bloqueada por
			// rate-limit do Gmail). dashboardPollInterval já cobre o
			// refresh periódico; aqui só reage a eventos de OUTRAS origens
			// que cheguem fora dessa janela.
			if m.screen == screenDashboard && time.Since(m.lastDashFetchAt) >= dashboardPollInterval {
				m.lastDashFetchAt = time.Now()
				cmds = append(cmds, fetchDashboard(m.client, m.cluster))
			}
		case "topology_refresh":
			if m.screen == screenClusters {
				cmds = append(cmds, fetchClusters(m.client))
			}
		}
		return m, tea.Batch(cmds...)

	case sseClosedMsg:
		if m.sseCtx != nil && m.sseCtx.Err() != nil {
			return m, nil // fomos nós que cancelamos (saindo do app)
		}
		backoff := m.sseBackoff
		if backoff <= 0 {
			backoff = sseInitialBackoff
		}
		m.sseBackoff = backoff * 2
		if m.sseBackoff > sseMaxBackoff {
			m.sseBackoff = sseMaxBackoff
		}
		return m, sseReconnectAfter(backoff)

	case sseReconnectMsg:
		return m, startSSE(m.client)

	case errMsg:
		m.err = msg.err.Error()
		m.clearLoadingFlags()
		return m, nil

	case authExpiredMsg:
		m.authExpired = true
		m.clearLoadingFlags()
		return m, nil
	}

	return m, nil
}

func (m *model) clearLoadingFlags() {
	m.loadingClusters, m.loadingNamespaces, m.loadingPods = false, false, false
	m.loadingNodes, m.loadingStorage, m.loadingCosts = false, false, false
	m.loadingQuotas, m.loadingOrphans, m.loadingCerts = false, false, false
}

// openOverlay troca pra uma tela "overlay" (dashboard ou uma das 6 telas de
// recurso) a partir de qualquer tela-base, guardando aonde voltar. Pular
// direto de um overlay pra outro (ex.: "d" depois "n") não perde o
// prevScreen original, porque só grava quando a tela atual ainda é uma das
// 3 telas-base. É idempotente: reabrir a tela em que já se está não refaz o
// fetch.
func (m model) openOverlay(target screen, cmds ...tea.Cmd) (model, tea.Cmd) {
	if m.screen == target {
		return m, nil
	}
	if isBaseScreen(m.screen) {
		m.prevScreen = m.screen
	}
	m.screen = target
	return m, tea.Batch(cmds...)
}

// refreshCmdFor devolve o Cmd de busca correspondente à tela atual — usado
// tanto pelo "r" (refresh manual) quanto poderia ser reaproveitado por um
// futuro tick genérico.
func refreshCmdFor(m model) tea.Cmd {
	switch m.screen {
	case screenClusters:
		return fetchClusters(m.client)
	case screenNamespaces:
		return fetchNamespaces(m.client, m.cluster)
	case screenPods:
		return fetchResources(m.client, m.cluster, m.namespace)
	case screenDashboard:
		return fetchDashboard(m.client, m.cluster)
	case screenNodes:
		return fetchNodes(m.client, m.cluster)
	case screenStorage:
		return fetchStorage(m.client, m.cluster)
	case screenCosts:
		return fetchCosts(m.client, m.cluster, m.namespace)
	case screenQuotas:
		return fetchQuotas(m.client, m.cluster, m.namespace)
	case screenOrphans:
		return fetchOrphans(m.client, m.cluster)
	case screenCerts:
		return fetchCerts(m.client, m.cluster, m.namespace)
	}
	return nil
}

// gotoDashboard/gotoNodes/.../gotoCerts são o corpo de cada tecla de atalho
// de overlay, extraídos pra método — tanto handleKey (tecla de atalho)
// quanto gotoScreen (barra de comando, ver command.go) chamam o mesmo
// código, nunca duplicado entre os dois pontos de entrada.

func (m model) gotoDashboard() (model, tea.Cmd) {
	if m.screen == screenDashboard {
		return m, nil
	}
	m.dashLoaded = false
	m.lastDashFetchAt = time.Now()
	return m.openOverlay(screenDashboard, fetchDashboard(m.client, m.cluster), tickAfter(dashboardPollInterval, screenDashboard))
}

func (m model) gotoNodes() (model, tea.Cmd) {
	if m.screen == screenNodes {
		return m, nil
	}
	m.loadingNodes = true
	return m.openOverlay(screenNodes, fetchNodes(m.client, m.cluster))
}

func (m model) gotoStorage() (model, tea.Cmd) {
	if m.screen == screenStorage {
		return m, nil
	}
	m.loadingStorage = true
	return m.openOverlay(screenStorage, fetchStorage(m.client, m.cluster))
}

func (m model) gotoQuotas() (model, tea.Cmd) {
	if m.screen == screenQuotas {
		return m, nil
	}
	m.loadingQuotas = true
	return m.openOverlay(screenQuotas, fetchQuotas(m.client, m.cluster, m.namespace))
}

func (m model) gotoOrphans() (model, tea.Cmd) {
	if m.screen == screenOrphans {
		return m, nil
	}
	m.loadingOrphans = true
	return m.openOverlay(screenOrphans, fetchOrphans(m.client, m.cluster))
}

func (m model) gotoCosts() (model, tea.Cmd) {
	if m.screen == screenCosts {
		return m, nil
	}
	m.costsLoaded = false
	m.loadingCosts = true
	return m.openOverlay(screenCosts, fetchCosts(m.client, m.cluster, m.namespace))
}

func (m model) gotoCerts() (model, tea.Cmd) {
	if m.screen == screenCerts {
		return m, nil
	}
	m.loadingCerts = true
	return m.openOverlay(screenCerts, fetchCerts(m.client, m.cluster, m.namespace))
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.authExpired {
		if key.Matches(msg, keys.Quit) {
			return m, tea.Quit
		}
		return m, nil
	}

	if m.commanding {
		return m.handleCommandMode(msg)
	}

	if m.filtering {
		switch {
		case key.Matches(msg, keys.Enter):
			m.filtering = false
			return m, nil
		case key.Matches(msg, keys.Back):
			m.filterInput.SetValue("")
			m.filtering = false
			m.refreshPodTable()
			return m, nil
		default:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.refreshPodTable()
			return m, cmd
		}
	}

	switch {
	case key.Matches(msg, keys.Quit):
		if m.sseCancel != nil {
			m.sseCancel()
		}
		return m, tea.Quit

	case key.Matches(msg, keys.Dashboard):
		return m.gotoDashboard()

	case key.Matches(msg, keys.Nodes):
		return m.gotoNodes()

	case key.Matches(msg, keys.Storage):
		return m.gotoStorage()

	case key.Matches(msg, keys.Quotas):
		return m.gotoQuotas()

	case key.Matches(msg, keys.Orphans):
		return m.gotoOrphans()

	case key.Matches(msg, keys.Costs):
		return m.gotoCosts()

	case key.Matches(msg, keys.Certs):
		return m.gotoCerts()

	case key.Matches(msg, keys.Command):
		m.commanding = true
		m.cmdInput.Focus()
		m.cmdInput.SetValue("")
		return m, nil

	case key.Matches(msg, keys.Help):
		return m.gotoScreen(screenHelp)

	case key.Matches(msg, keys.Back):
		switch {
		case isOverlayScreen(m.screen):
			m.screen = m.prevScreen
		case m.screen == screenPods && m.filterInput.Value() != "":
			// Esc já limpa o filtro enquanto se digita (bloco m.filtering
			// acima) — aqui cobre o caso de um filtro já CONFIRMADO (Enter),
			// onde Esc antes caía direto no "voltar pra namespaces" e deixava
			// o filtro aplicado silenciosamente sem forma de limpar num só
			// toque. Primeiro Esc limpa; segundo Esc (filtro já vazio) volta.
			m.filterInput.SetValue("")
			m.refreshPodTable()
		case m.screen == screenPods:
			m.screen = screenNamespaces
		case m.screen == screenNamespaces:
			m.screen = screenClusters
		}
		return m, nil

	case key.Matches(msg, keys.Refresh):
		return m, refreshCmdFor(m)
	}

	switch m.screen {
	case screenClusters:
		return m.updateClusters(msg)
	case screenNamespaces:
		return m.updateNamespaces(msg)
	case screenPods:
		return m.updatePods(msg)
	case screenNodes:
		return m.updateNodes(msg)
	case screenStorage:
		return m.updateStorage(msg)
	case screenQuotas:
		return m.updateQuotas(msg)
	case screenOrphans:
		return m.updateOrphans(msg)
	case screenCosts:
		return m.updateCosts(msg)
	case screenCerts:
		return m.updateCerts(msg)
	case screenHelp:
		return m.updateHelp(msg)
	}
	return m, nil
}

func (m model) View() string {
	if m.authExpired {
		return errStyle.Render("Sessão expirada — rode 'podmon login' em outro terminal e reinicie a TUI, ou pressione q para sair.")
	}

	var body string
	switch m.screen {
	case screenClusters:
		body = m.viewClusters()
	case screenNamespaces:
		body = m.viewNamespaces()
	case screenPods:
		body = m.viewPods()
	case screenDashboard:
		body = m.viewDashboard()
	case screenNodes:
		body = m.viewNodes()
	case screenStorage:
		body = m.viewStorage()
	case screenQuotas:
		body = m.viewQuotas()
	case screenOrphans:
		body = m.viewOrphans()
	case screenCosts:
		body = m.viewCosts()
	case screenCerts:
		body = m.viewCerts()
	case screenHelp:
		body = m.viewHelp()
	}

	out := body
	if header := renderHeader(m); header != "" {
		out = header + "\n" + body
	}
	if m.commanding {
		out += "\n" + m.cmdInput.View()
	}
	if m.err != "" {
		out += "\n" + errStyle.Render("erro: "+m.err)
	}
	return out
}
