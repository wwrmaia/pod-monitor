package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Enter     key.Binding
	Back      key.Binding
	Quit      key.Binding
	Refresh   key.Binding
	Dashboard key.Binding
	Filter    key.Binding
	Sort      key.Binding
	Nodes     key.Binding
	Storage   key.Binding
	Quotas    key.Binding
	Orphans   key.Binding
	Costs     key.Binding
	Certs     key.Binding
	Command   key.Binding
	Help      key.Binding
}

// overlayHelpFooter lista as teclas globais que abrem uma tela cheia a
// partir de qualquer tela-base (clusters/namespaces/pods) — repetido nas
// três telas-base pra descoberta, já que são 7 overlays hoje (d + 6 novos).
const overlayHelpFooter = "d dashboard | n nodes | s storage | u quotas | o orphans | p custos | t certs | : comando | ? ajuda | r atualizar | q sair"

var keys = keyMap{
	Enter:     key.NewBinding(key.WithKeys("enter")),
	Back:      key.NewBinding(key.WithKeys("esc", "backspace")),
	Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c")),
	Refresh:   key.NewBinding(key.WithKeys("r")),
	Dashboard: key.NewBinding(key.WithKeys("d")),
	Filter:    key.NewBinding(key.WithKeys("/")),
	Sort:      key.NewBinding(key.WithKeys("c")),
	Nodes:     key.NewBinding(key.WithKeys("n")),
	Storage:   key.NewBinding(key.WithKeys("s")),
	Quotas:    key.NewBinding(key.WithKeys("u")),
	Orphans:   key.NewBinding(key.WithKeys("o")),
	Costs:     key.NewBinding(key.WithKeys("p")),
	Certs:     key.NewBinding(key.WithKeys("t")),
	Command:   key.NewBinding(key.WithKeys(":")),
	Help:      key.NewBinding(key.WithKeys("?")),
}

// commandAliases mapeia o que o usuário digita na barra de comando (":") pro
// destino correspondente — mesmos 7 overlays que as teclas de atalho já
// abrem, só uma forma alternativa e mais descobrível de chegar neles (não
// adiciona destino novo nenhum).
var commandAliases = map[string]screen{
	"dashboard": screenDashboard, "dash": screenDashboard, "alerts": screenDashboard,
	"nodes": screenNodes, "no": screenNodes,
	"storage": screenStorage, "pvc": screenStorage, "pvcs": screenStorage,
	"quotas": screenQuotas, "quota": screenQuotas,
	"orphans": screenOrphans, "orph": screenOrphans, "audit": screenOrphans,
	"costs": screenCosts, "cost": screenCosts,
	"certs": screenCerts, "cert": screenCerts, "certificates": screenCerts,
	"help": screenHelp, "?": screenHelp,
}
