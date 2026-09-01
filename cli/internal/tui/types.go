package tui

// Cópias locais e mínimas dos DTOs de resposta da API — mesmo padrão já usado
// em cmd/resources.go e cmd/dashboard.go (cada camada de apresentação define
// sua própria visão do formato de rede; encoding/json casa por tag, não por
// identidade de tipo). Ver decisão registrada no plano: não extrair um
// internal/types compartilhado.

type containerResources struct {
	Name          string `json:"name"`
	CPURequest    string `json:"cpu_request"`
	MemoryRequest string `json:"memory_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryLimit   string `json:"memory_limit"`
	CPUUsage      string `json:"cpu_usage"`
	MemoryUsage   string `json:"memory_usage"`
}

type podResources struct {
	Name       string               `json:"pod"`
	Namespace  string               `json:"namespace"`
	Node       string               `json:"node"`
	Phase      string               `json:"phase"`
	Containers []containerResources `json:"containers"`
}

type dashWidgetContainer struct {
	Name      string `json:"name"`
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	CPUUsage  string `json:"cpu_usage"`
	MemUsage  string `json:"mem_usage"`
	Pct       int    `json:"pct"`
}

type alertPod struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

type dashSummary struct {
	Pods struct {
		Statuses map[string]int `json:"statuses"`
	} `json:"pods"`
	Nodes struct {
		Ready         int      `json:"ready"`
		NotReady      int      `json:"not_ready"`
		Total         int      `json:"total"`
		NotReadyNames []string `json:"not_ready_names"`
	} `json:"nodes"`
	TopCPU []dashWidgetContainer `json:"top_cpu"`
	TopMem []dashWidgetContainer `json:"top_mem"`
	Alerts struct {
		Critical int `json:"critical"`
		Warning  int `json:"warning"`
	} `json:"alerts"`
	AlertPods []alertPod `json:"alert_pods"`
}

// DTOs das seis telas adicionadas depois do núcleo (nodes/costs/storage/
// quotas/orphans/certs) — paridade de campos com os comandos nível 1
// equivalentes (cmd/nodes.go, cmd/costs.go, etc.), decisão registrada no
// plano: não enriquecer com campos que o backend já devolve mas que nada
// aqui precisa (ex.: "volume" em PVC, "sans" em certs, "limit_ranges" em
// quotas, "selector"/"type"/"service" nos vários tipos de órfão).

type nodeResources struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Role           string `json:"role"`
	CPUAllocatable string `json:"cpu_allocatable"`
	MemAllocatable string `json:"mem_allocatable"`
	CPUUsage       string `json:"cpu_usage"`
	MemUsage       string `json:"mem_usage"`
}

type pvcInfo struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Capacity     string `json:"capacity"`
	Status       string `json:"status"`
	StorageClass string `json:"storage_class"`
	AccessModes  string `json:"access_modes"`
}

type namespaceCost struct {
	Namespace string  `json:"namespace"`
	CPUCores  float64 `json:"cpu_cores"`
	MemGiB    float64 `json:"mem_gib"`
	CostHour  float64 `json:"cost_hour"`
	CostMonth float64 `json:"cost_month"`
}

type costsResponse struct {
	Enabled    bool            `json:"enabled"`
	Configured bool            `json:"configured"`
	Currency   string          `json:"currency"`
	Items      []namespaceCost `json:"items"`
}

type quotaResource struct {
	Hard string `json:"hard"`
	Used string `json:"used"`
}

type quotaInfo struct {
	Name      string                    `json:"name"`
	Resources map[string]*quotaResource `json:"resources"`
}

type namespaceQuota struct {
	Namespace string      `json:"namespace"`
	Quotas    []quotaInfo `json:"quotas"`
}

type orphanItem struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Age       string `json:"age"`
}

type orphanedResources struct {
	PVCs            []orphanItem `json:"pvcs"`
	Services        []orphanItem `json:"services"`
	ConfigMaps      []orphanItem `json:"config_maps"`
	Secrets         []orphanItem `json:"secrets"`
	Ingresses       []orphanItem `json:"ingresses"`
	ServiceAccounts []orphanItem `json:"service_accounts"`
}

type certInfo struct {
	Cluster    string `json:"cluster"`
	Namespace  string `json:"namespace"`
	SecretName string `json:"secret_name"`
	CommonName string `json:"common_name"`
	Issuer     string `json:"issuer"`
	NotAfter   string `json:"not_after"`
	DaysLeft   int    `json:"days_left"`
	Bucket     string `json:"bucket"`
}

// simpleItem é um list.Item/list.DefaultItem trivial — usado nas telas de
// clusters e namespaces, que só precisam exibir um nome.
type simpleItem string

func (i simpleItem) FilterValue() string { return string(i) }
func (i simpleItem) Title() string       { return string(i) }
func (i simpleItem) Description() string { return "" }

const allNamespacesLabel = "(todos os namespaces)"
