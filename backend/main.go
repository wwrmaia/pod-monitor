package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"image/png"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	corev1         "k8s.io/api/core/v1"
	rbacv1         "k8s.io/api/rbac/v1"
	k8serrors      "k8s.io/apimachinery/pkg/api/errors"
	metav1         "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi    "k8s.io/client-go/tools/clientcmd/api"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient  "k8s.io/metrics/pkg/client/clientset/versioned"
	_ "github.com/lib/pq"
)

// ── Tipos ─────────────────────────────────────────────────────────────────────

type clusterClients struct {
	k8s     *kubernetes.Clientset
	metrics *metricsclient.Clientset
}

type DashboardConfig struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Widgets  string `json:"widgets"`
}

type ContainerResources struct {
	Name          string `json:"name"`
	Image         string `json:"image,omitempty"`
	CPURequest    string `json:"cpu_request"`
	MemoryRequest string `json:"memory_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryLimit   string `json:"memory_limit"`
	CPUUsage      string `json:"cpu_usage,omitempty"`
	MemoryUsage   string `json:"memory_usage,omitempty"`
}

type PodResources struct {
	Name       string               `json:"pod"`
	Namespace  string               `json:"namespace"`
	Node       string               `json:"node"`
	Phase      string               `json:"phase"`
	Containers []ContainerResources `json:"containers"`
}

type NodeResources struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Role           string `json:"role"`
	CPUAllocatable string `json:"cpu_allocatable"`
	MemAllocatable string `json:"mem_allocatable"`
	CPUUsage       string `json:"cpu_usage,omitempty"`
	MemUsage       string `json:"mem_usage,omitempty"`
}

type PVCInfo struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Capacity    string `json:"capacity"`
	Status      string `json:"status"`
	StorageClass string `json:"storage_class"`
	Volume      string `json:"volume"`
	AccessModes string `json:"access_modes"`
}

type PagedResponse[T any] struct {
	Items      []T `json:"items"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

func paginate[T any](items []T, page, pageSize int) PagedResponse[T] {
	total := len(items)
	if pageSize < 1 { pageSize = 1 }
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 { totalPages = 1 }
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total { start = total }
	if end > total { end = total }
	slice := items[start:end]
	if slice == nil { slice = []T{} }
	return PagedResponse[T]{Items: slice, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}
}

type OrphanPVC struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Capacity     string `json:"capacity"`
	StorageClass string `json:"storage_class"`
	Age          string `json:"age"`
}

type OrphanService struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Selector  string `json:"selector"`
	Age       string `json:"age"`
}

type OrphanConfigMap struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Age       string `json:"age"`
}

type OrphanSecret struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	SecretType string `json:"type"`
	Age       string `json:"age"`
}

type OrphanIngress struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Service   string `json:"service"`
	Age       string `json:"age"`
}

type OrphanServiceAccount struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Age       string `json:"age"`
}

type OrphanedResources struct {
	PVCs            []OrphanPVC            `json:"pvcs"`
	Services        []OrphanService        `json:"services"`
	ConfigMaps      []OrphanConfigMap      `json:"config_maps"`
	Secrets         []OrphanSecret         `json:"secrets"`
	Ingresses       []OrphanIngress        `json:"ingresses"`
	ServiceAccounts []OrphanServiceAccount `json:"service_accounts"`
}

type HistoryRecord struct {
	SessionID     string `json:"session_id"`
	CapturedAt    string `json:"captured_at"`
	Cluster       string `json:"cluster"`
	Pod           string `json:"pod"`
	Namespace     string `json:"namespace"`
	Node          string `json:"node"`
	Container     string `json:"container"`
	CPURequest    string `json:"cpu_request"`
	MemoryRequest string `json:"memory_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryLimit   string `json:"memory_limit"`
	CPUUsage      string `json:"cpu_usage"`
	MemoryUsage   string `json:"memory_usage"`
}

// ── Docker/Podman ─────────────────────────────────────────────────────────────

type DockerContainerInfo struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Image    string  `json:"image"`
	State    string  `json:"state"`
	Status   string  `json:"status"`
	Created  int64   `json:"created"`
	CPUPct   float64 `json:"cpu_pct"`
	MemUsage int64   `json:"mem_usage"`
	MemLimit int64   `json:"mem_limit"`
	MemPct   float64 `json:"mem_pct"`
}

type ClusterContainerInfo struct {
	ID        string  `json:"id"`
	Container string  `json:"container"`
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	Image     string  `json:"image"`
	State     string  `json:"state"`
	Status    string  `json:"status"`
	Host      string  `json:"host"`
	CPUPct    float64 `json:"cpu_pct"`
	MemUsage  int64   `json:"mem_usage"`
	MemLimit  int64   `json:"mem_limit"`
	MemPct    float64 `json:"mem_pct"`
}

var dockerSockets     = map[string]string{} // name → addr (unix path ou tcp://host:port)
var dockerAPIVer      = map[string]string{} // name → versão da API detectada (ex: "1.44")
var dockerAPIVerMu    sync.RWMutex
var dockerIsCluster   = map[string]bool{}   // name → true se tem containers k8s
var dockerIsClusterMu sync.RWMutex

func initDockerHosts() {
	hostsEnv := os.Getenv("DOCKER_HOSTS")
	// Formato: nome=/var/run/docker.sock  ou  nome=tcp://host:2375
	// O separador '=' evita conflito com ':' dos endereços TCP.
	// Suporte legado: nome:/caminho/unix (split no primeiro ':')
	if hostsEnv != "" {
		for _, entry := range strings.Split(hostsEnv, ",") {
			entry = strings.TrimSpace(entry)
			var name, addr string
			if idx := strings.Index(entry, "="); idx > 0 {
				name, addr = entry[:idx], entry[idx+1:]
			} else if idx := strings.Index(entry, ":"); idx > 0 {
				name, addr = entry[:idx], entry[idx+1:]
			} else {
				continue
			}
			dockerSockets[name] = addr
			log.Printf("Host Docker registrado: %s -> %s", name, addr)
		}
		return
	}
	defaults := []struct{ name, path string }{
		{"docker", "/var/run/docker.sock"},
		{"podman", "/run/podman/podman.sock"},
	}
	for _, d := range defaults {
		if _, err := os.Stat(d.path); err == nil {
			dockerSockets[d.name] = d.path
			log.Printf("Host Docker detectado: %s -> %s", d.name, d.path)
		}
	}
	if len(dockerSockets) == 0 {
		log.Println("Nenhum socket Docker/Podman encontrado")
	}
}

// detectDockerHostTypes detecta em background se cada host tem containers k8s
func detectDockerHostTypes() {
	var wg sync.WaitGroup
	for name, addr := range dockerSockets {
		wg.Add(1)
		go func(n, a string) {
			defer wg.Done()
			client, baseURL := newDockerClient(a)
			apiPath := dockerAPIPath(n, client, baseURL)
			raw, err := listDockerContainers(client, baseURL, apiPath)
			isCluster := false
			if err == nil {
				for _, c := range raw {
					if isK8sContainer(c.Labels) {
						isCluster = true
						break
					}
				}
			}
			dockerIsClusterMu.Lock()
			dockerIsCluster[n] = isCluster
			dockerIsClusterMu.Unlock()
			log.Printf("Host Docker %s: cluster=%v", n, isCluster)
		}(name, addr)
	}
	wg.Wait()
}

// newDockerClient retorna (httpClient, baseURL) para unix socket ou tcp://host:porta
func newDockerClient(addr string) (*http.Client, string) {
	if strings.HasPrefix(addr, "tcp://") {
		host := strings.TrimPrefix(addr, "tcp://")
		return &http.Client{Timeout: 15 * time.Second}, "http://" + host
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", addr)
			},
		},
	}
	return client, "http://localhost"
}

func dockerGet(client *http.Client, baseURL, path string, v interface{}) error {
	resp, err := client.Get(baseURL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// dockerAPIPath retorna o prefixo versionado da API para o host, detectando na primeira chamada.
func dockerAPIPath(name string, client *http.Client, baseURL string) string {
	dockerAPIVerMu.RLock()
	ver, ok := dockerAPIVer[name]
	dockerAPIVerMu.RUnlock()
	if ok {
		return "/v" + ver
	}
	var info struct {
		ApiVersion string `json:"ApiVersion"`
	}
	if err := dockerGet(client, baseURL, "/version", &info); err == nil && info.ApiVersion != "" {
		dockerAPIVerMu.Lock()
		dockerAPIVer[name] = info.ApiVersion
		dockerAPIVerMu.Unlock()
		return "/v" + info.ApiVersion
	}
	return "" // sem versão → usa endpoint legado
}

func getDockerStats(client *http.Client, baseURL, apiPath, id string) (cpuPct float64, memUsage, memLimit int64, memPct float64) {
	var stats struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs     int    `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64            `json:"usage"`
			Limit uint64            `json:"limit"`
			Stats map[string]uint64 `json:"stats"`
		} `json:"memory_stats"`
	}
	if err := dockerGet(client, baseURL, apiPath+"/containers/"+id+"/stats?stream=false", &stats); err != nil {
		return
	}
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(stats.CPUStats.SystemCPUUsage) - float64(stats.PreCPUStats.SystemCPUUsage)
	numCPUs := stats.CPUStats.OnlineCPUs
	if numCPUs == 0 {
		numCPUs = 1
	}
	if sysDelta > 0 {
		cpuPct = math.Round((cpuDelta/sysDelta)*float64(numCPUs)*100*100) / 100
	}
	usage := stats.MemoryStats.Usage
	if cache, ok := stats.MemoryStats.Stats["cache"]; ok && cache < usage {
		usage -= cache
	}
	limit := stats.MemoryStats.Limit
	memUsage = int64(usage)
	memLimit = int64(limit)
	if limit > 0 {
		memPct = math.Round(float64(usage)/float64(limit)*100*100) / 100
	}
	return
}

func handleDockerHosts(w http.ResponseWriter, r *http.Request) {
	type dockerHostInfo struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Cluster bool   `json:"cluster"`
	}
	hosts := []dockerHostInfo{}
	for name, addr := range dockerSockets {
		dockerIsClusterMu.RLock()
		cluster := dockerIsCluster[name]
		dockerIsClusterMu.RUnlock()
		hosts = append(hosts, dockerHostInfo{Name: name, Address: addr, Cluster: cluster})
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hosts)
}

type dockerRawContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Created int64            `json:"Created"`
	Labels map[string]string `json:"Labels"`
}

func isK8sContainer(labels map[string]string) bool {
	_, ok := labels["io.kubernetes.pod.name"]
	return ok
}

func listDockerContainers(client *http.Client, baseURL, apiPath string) ([]dockerRawContainer, error) {
	var raw []dockerRawContainer
	err := dockerGet(client, baseURL, apiPath+"/containers/json?all=true", &raw)
	return raw, err
}

func buildDockerContainerInfos(client *http.Client, baseURL, apiPath string, containers []dockerRawContainer) []DockerContainerInfo {
	type statsResult struct {
		cpuPct             float64
		memUsage, memLimit int64
		memPct             float64
	}
	results := make([]statsResult, len(containers))
	var wg sync.WaitGroup
	for i, c := range containers {
		if c.State != "running" {
			continue
		}
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			cpu, mu, ml, mp := getDockerStats(client, baseURL, apiPath, id)
			results[idx] = statsResult{cpu, mu, ml, mp}
		}(i, c.ID)
	}
	wg.Wait()

	out := make([]DockerContainerInfo, len(containers))
	for i, c := range containers {
		name := c.ID
		if len(name) > 12 {
			name = name[:12]
		}
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		idShort := c.ID
		if len(idShort) > 12 {
			idShort = idShort[:12]
		}
		r := results[i]
		out[i] = DockerContainerInfo{
			ID:       idShort,
			Name:     name,
			Image:    c.Image,
			State:    c.State,
			Status:   c.Status,
			Created:  c.Created,
			CPUPct:   r.cpuPct,
			MemUsage: r.memUsage,
			MemLimit: r.memLimit,
			MemPct:   r.memPct,
		}
	}
	return out
}

func handleDockerContainers(w http.ResponseWriter, r *http.Request) {
	hostName := r.URL.Query().Get("host")
	socketPath, ok := dockerSockets[hostName]
	if !ok {
		for name, path := range dockerSockets {
			hostName, socketPath = name, path
			break
		}
		if socketPath == "" {
			http.Error(w, "nenhum host Docker configurado", 400)
			return
		}
	}

	client, baseURL := newDockerClient(socketPath)
	apiPath := dockerAPIPath(hostName, client, baseURL)

	raw, err := listDockerContainers(client, baseURL, apiPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("erro ao listar containers: %v", err), 500)
		return
	}

	// filtra apenas containers externos (não gerenciados pelo Kubernetes)
	var filtered []dockerRawContainer
	for _, c := range raw {
		if !isK8sContainer(c.Labels) {
			filtered = append(filtered, c)
		}
	}

	out := buildDockerContainerInfos(client, baseURL, apiPath, filtered)
	sort.Slice(out, func(i, j int) bool {
		if out[i].State != out[j].State {
			return out[i].State < out[j].State
		}
		return out[i].Name < out[j].Name
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// ── Helm releases ─────────────────────────────────────────────────────────────

type HelmReleaseInfo struct {
	Name         string `json:"name"`
	Chart        string `json:"chart"`
	ChartVersion string `json:"chart_version"`
	AppVersion   string `json:"app_version"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"`
	Revision     int    `json:"revision"`
	LastDeployed string `json:"last_deployed"`
}

type helmReleaseData struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
	Info      struct {
		LastDeployed string `json:"last_deployed"`
		Status       string `json:"status"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
}

func decodeHelmRelease(data []byte) (*helmReleaseData, error) {
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil { return nil, err }
	gr, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil { return nil, err }
	defer gr.Close()
	var rel helmReleaseData
	if err := json.NewDecoder(gr).Decode(&rel); err != nil { return nil, err }
	return &rel, nil
}

// ── Deployments ───────────────────────────────────────────────────────────────

type DeploymentInfo struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Desired     int32    `json:"desired"`
	Ready       int32    `json:"ready"`
	Available   int32    `json:"available"`
	Unavailable int32    `json:"unavailable"`
	UpToDate    int32    `json:"up_to_date"`
	Images      []string `json:"images"`
	Strategy    string   `json:"strategy"`
	Revision    string   `json:"revision"`
	Age         string   `json:"age"`
	Conditions  []string `json:"conditions"`
}

func handleDeployments(w http.ResponseWriter, r *http.Request) {
	clients, _, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	ns := r.URL.Query().Get("namespace")
	ctx := context.Background()
	deps, err := clients.k8s.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil { http.Error(w, fmt.Sprintf("erro ao listar deployments: %v", err), 500); return }

	out := make([]DeploymentInfo, 0, len(deps.Items))
	for _, d := range deps.Items {
		images := []string{}
		for _, c := range d.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}
		strategy := string(d.Spec.Strategy.Type)
		revision := d.Annotations["deployment.kubernetes.io/revision"]
		age := ""
		if !d.CreationTimestamp.IsZero() {
			dur := time.Since(d.CreationTimestamp.Time)
			switch {
			case dur < time.Hour:
				age = fmt.Sprintf("%dm", int(dur.Minutes()))
			case dur < 24*time.Hour:
				age = fmt.Sprintf("%dh", int(dur.Hours()))
			default:
				age = fmt.Sprintf("%dd", int(dur.Hours()/24))
			}
		}
		conds := []string{}
		for _, c := range d.Status.Conditions {
			if string(c.Status) != "True" {
				conds = append(conds, fmt.Sprintf("%s=False", c.Type))
			}
		}
		desired := int32(1)
		if d.Spec.Replicas != nil { desired = *d.Spec.Replicas }
		out = append(out, DeploymentInfo{
			Name:        d.Name,
			Namespace:   d.Namespace,
			Desired:     desired,
			Ready:       d.Status.ReadyReplicas,
			Available:   d.Status.AvailableReplicas,
			Unavailable: d.Status.UnavailableReplicas,
			UpToDate:    d.Status.UpdatedReplicas,
			Images:      images,
			Strategy:    strategy,
			Revision:    revision,
			Age:         age,
			Conditions:  conds,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace { return out[i].Namespace < out[j].Namespace }
		return out[i].Name < out[j].Name
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func handleHelmReleases(w http.ResponseWriter, r *http.Request) {
	clients, _, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	ns := r.URL.Query().Get("namespace")
	ctx := context.Background()
	secrets, err := clients.k8s.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{LabelSelector: "owner=helm"})
	if err != nil { http.Error(w, fmt.Sprintf("erro ao listar releases: %v", err), 500); return }

	type releaseKey struct{ name, ns string }
	latest := map[releaseKey]*corev1.Secret{}
	for i, sec := range secrets.Items {
		if sec.Labels["status"] == "superseded" { continue }
		key := releaseKey{sec.Labels["name"], sec.Namespace}
		existing, ok := latest[key]
		if !ok { latest[key] = &secrets.Items[i]; continue }
		var ev, nv int
		fmt.Sscanf(existing.Labels["version"], "%d", &ev)
		fmt.Sscanf(sec.Labels["version"], "%d", &nv)
		if nv > ev { latest[key] = &secrets.Items[i] }
	}

	out := []HelmReleaseInfo{}
	for _, sec := range latest {
		rel, err := decodeHelmRelease(sec.Data["release"])
		if err != nil {
			rev := 0; fmt.Sscanf(sec.Labels["version"], "%d", &rev)
			out = append(out, HelmReleaseInfo{
				Name: sec.Labels["name"], Namespace: sec.Namespace,
				Status: sec.Labels["status"], Revision: rev,
			})
			continue
		}
		out = append(out, HelmReleaseInfo{
			Name: rel.Name, Chart: rel.Chart.Metadata.Name,
			ChartVersion: rel.Chart.Metadata.Version, AppVersion: rel.Chart.Metadata.AppVersion,
			Namespace: rel.Namespace, Status: rel.Info.Status,
			Revision: rel.Version, LastDeployed: rel.Info.LastDeployed,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace { return out[i].Namespace < out[j].Namespace }
		return out[i].Name < out[j].Name
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func parseQuantityMilli(s string) float64 {
	q, err := resource.ParseQuantity(s)
	if err != nil || s == "0" { return 0 }
	return float64(q.MilliValue())
}

func parseQuantityBytes(s string) int64 {
	q, err := resource.ParseQuantity(s)
	if err != nil || s == "0" { return 0 }
	return q.Value()
}

func handleContainers(w http.ResponseWriter, r *http.Request) {
	if clusterName := r.URL.Query().Get("cluster"); clusterName != "" {
		clients, _, err := getCluster(r)
		if err != nil { http.Error(w, err.Error(), 400); return }
		pods, err := getPodsResources(clients, "")
		if err != nil { http.Error(w, err.Error(), 500); return }
		out := []ClusterContainerInfo{}
		for _, pod := range pods {
			state := strings.ToLower(pod.Phase)
			for _, c := range pod.Containers {
				memUsage := parseQuantityBytes(c.MemoryUsage)
				memLimit := parseQuantityBytes(c.MemoryLimit)
				memPct := 0.0
				if memLimit > 0 { memPct = float64(memUsage) / float64(memLimit) * 100 }
				cpuUsage := parseQuantityMilli(c.CPUUsage)
				cpuLimit := parseQuantityMilli(c.CPULimit)
				cpuPct := 0.0
				if cpuLimit > 0 { cpuPct = cpuUsage / cpuLimit * 100 }
				out = append(out, ClusterContainerInfo{
					ID:        pod.Name + "/" + c.Name,
					Container: c.Name,
					Pod:       pod.Name,
					Namespace: pod.Namespace,
					Image:     c.Image,
					State:     state,
					Status:    pod.Phase,
					CPUPct:    cpuPct,
					MemUsage:  memUsage,
					MemLimit:  memLimit,
					MemPct:    memPct,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
		return
	}

	hostName := r.URL.Query().Get("host")
	socketPath, ok := dockerSockets[hostName]
	if !ok {
		for name, path := range dockerSockets {
			hostName, socketPath = name, path
			break
		}
		if socketPath == "" {
			http.Error(w, "nenhum host Docker configurado", 400)
			return
		}
	}

	client, baseURL := newDockerClient(socketPath)
	apiPath := dockerAPIPath(hostName, client, baseURL)

	raw, err := listDockerContainers(client, baseURL, apiPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("erro ao listar containers: %v", err), 500)
		return
	}

	type statsResult struct {
		cpuPct             float64
		memUsage, memLimit int64
		memPct             float64
	}
	var k8sContainers []dockerRawContainer
	for _, c := range raw {
		if isK8sContainer(c.Labels) && c.Labels["io.kubernetes.container.name"] != "POD" {
			k8sContainers = append(k8sContainers, c)
		}
	}

	results := make([]statsResult, len(k8sContainers))
	var wg sync.WaitGroup
	for i, c := range k8sContainers {
		if c.State != "running" {
			continue
		}
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			cpu, mu, ml, mp := getDockerStats(client, baseURL, apiPath, id)
			results[idx] = statsResult{cpu, mu, ml, mp}
		}(i, c.ID)
	}
	wg.Wait()

	out := make([]ClusterContainerInfo, len(k8sContainers))
	for i, c := range k8sContainers {
		idShort := c.ID
		if len(idShort) > 12 {
			idShort = idShort[:12]
		}
		r := results[i]
		out[i] = ClusterContainerInfo{
			ID:        idShort,
			Container: c.Labels["io.kubernetes.container.name"],
			Pod:       c.Labels["io.kubernetes.pod.name"],
			Namespace: c.Labels["io.kubernetes.pod.namespace"],
			Image:     c.Image,
			State:     c.State,
			Status:    c.Status,
			Host:      hostName,
			CPUPct:    r.cpuPct,
			MemUsage:  r.memUsage,
			MemLimit:  r.memLimit,
			MemPct:    r.memPct,
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		if out[i].Pod != out[j].Pod {
			return out[i].Pod < out[j].Pod
		}
		return out[i].Container < out[j].Container
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// ── Auth ──────────────────────────────────────────────────────────────────────

const (
	RoleAdmin  = "administration"
	RoleReader = "reader"
	RoleDev    = "dev"

	bcryptCost = 12 // custo mínimo recomendado para produção (padrão Go é 10)
)

type User struct {
	Username          string   `json:"username"`
	Password          string   `json:"-"`
	Role              string   `json:"role"`
	GroupName         string   `json:"group_name,omitempty"`
	AllowedClusters   []string `json:"allowed_clusters,omitempty"`
	AllowedNamespaces []string `json:"allowed_namespaces,omitempty"`
	TOTPSecret        string   `json:"-"`
	TOTPEnabled       bool     `json:"totp_enabled"`
}

type Group struct {
	Name              string   `json:"name"`
	Role              string   `json:"role"`
	TOTPEnabled       bool     `json:"totp_enabled"`
	AllowedClusters   []string `json:"allowed_clusters,omitempty"`
	AllowedNamespaces []string `json:"allowed_namespaces,omitempty"`
	Members           []string `json:"members,omitempty"`
}

type TokenClaims struct {
	Username          string   `json:"sub"`
	Role              string   `json:"role"`
	Exp               int64    `json:"exp"`
	AllowedClusters   []string `json:"allowed_clusters,omitempty"`
	AllowedNamespaces []string `json:"allowed_namespaces,omitempty"`
}

type contextKey string
const claimsCtxKey contextKey = "claims"

type mfaEntry struct {
	username string
	exp      time.Time
}

type mfaSetupEntry struct {
	username string
	secret   string
	qrCode   string
	exp      time.Time
}

var (
	jwtSecret      []byte
	frontendOrigin string // origem CORS permitida; lida de FRONTEND_ORIGIN no startup
	usersMap       = map[string]User{}
	usersMu        sync.RWMutex
	mfaTokens      = map[string]mfaEntry{}
	mfaTokensMu    sync.Mutex
	mfaSetupTokens   = map[string]mfaSetupEntry{}
	mfaSetupTokensMu sync.Mutex
	groupsMap      = map[string]*Group{}
	groupsMu       sync.RWMutex
)

// ── Criptografia AES-256-GCM para seeds TOTP ─────────────────────────────────
//
// Seeds TOTP são armazenados cifrados no SQLite com o prefixo "enc:".
// A chave AES-256 é derivada do JWT_SECRET via SHA-256 com contexto "totp-key".
// Registros sem prefixo "enc:" são tratados como legados (plaintext) e
// re-cifrados automaticamente no startup por migratePlaintextTOTP().

func totpEncryptionKey() []byte {
	h := sha256.Sum256(append(jwtSecret, []byte("totp-key-v1")...))
	return h[:]
}

func encryptTOTP(plaintext string) (string, error) {
	if plaintext == "" { return "", nil }
	key := totpEncryptionKey()
	block, err := aes.NewCipher(key)
	if err != nil { return "", err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return "", err }
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil { return "", err }
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptTOTP(stored string) (string, error) {
	if stored == "" { return "", nil }
	if !strings.HasPrefix(stored, "enc:") {
		return stored, nil // legado plaintext
	}
	key := totpEncryptionKey()
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "enc:"))
	if err != nil { return "", err }
	block, err := aes.NewCipher(key)
	if err != nil { return "", err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return "", err }
	if len(data) < gcm.NonceSize() { return "", fmt.Errorf("totp ciphertext muito curto") }
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil { return "", err }
	return string(plain), nil
}

// migratePlaintextTOTP re-cifra no DB seeds TOTP que ainda estão em plaintext.
func migratePlaintextTOTP() {
	if db == nil { return }
	usersMu.RLock()
	type rec struct{ username, plain string }
	var toMigrate []rec
	for _, u := range usersMap {
		if u.TOTPSecret != "" {
			// Se estava em plaintext no DB (já foi decifrado em initAuth sem prefixo)
			// precisamos verificar o valor cru do DB
			toMigrate = append(toMigrate, rec{u.Username, u.TOTPSecret})
		}
	}
	usersMu.RUnlock()

	rows, err := db.Query(`SELECT username, totp_secret FROM users WHERE totp_secret != '' AND totp_secret NOT LIKE 'enc:%'`)
	if err != nil { return }
	defer rows.Close()
	var legacyRecs []rec
	for rows.Next() {
		var r rec
		if rows.Scan(&r.username, &r.plain) == nil && r.plain != "" {
			legacyRecs = append(legacyRecs, r)
		}
	}
	_ = toMigrate
	for _, r := range legacyRecs {
		encrypted, err := encryptTOTP(r.plain)
		if err != nil {
			log.Printf("[TOTP] Falha ao cifrar seed de '%s': %v", r.username, err)
			continue
		}
		db.Exec(`UPDATE users SET totp_secret=$1 WHERE username=$2`, encrypted, r.username)
		log.Printf("[TOTP] Seed de '%s' migrado para AES-256-GCM", r.username)
	}
}

// ── JWT token blacklist (revogação no logout) ─────────────────────────────────

var (
	tokenBlacklist   = map[string]time.Time{} // token → tempo de expiração
	tokenBlacklistMu sync.Mutex
)

func blacklistToken(tokenStr string, exp time.Time) {
	tokenBlacklistMu.Lock()
	tokenBlacklist[tokenStr] = exp
	tokenBlacklistMu.Unlock()
}

func isTokenBlacklisted(tokenStr string) bool {
	tokenBlacklistMu.Lock()
	exp, ok := tokenBlacklist[tokenStr]
	tokenBlacklistMu.Unlock()
	return ok && time.Now().Before(exp)
}

func cleanTokenBlacklist() {
	ticker := time.NewTicker(time.Hour)
	for range ticker.C {
		tokenBlacklistMu.Lock()
		now := time.Now()
		for tok, exp := range tokenBlacklist {
			if now.After(exp) { delete(tokenBlacklist, tok) }
		}
		tokenBlacklistMu.Unlock()
	}
}

// ── Rate limiter de autenticação ──────────────────────────────────────────────

const (
	rateLimitWindow  = 5 * time.Minute
	rateLimitMax     = 10
	rateLimitLockout = 15 * time.Minute
)

type loginAttempt struct {
	count        int
	firstSeen    time.Time
	blockedUntil time.Time
}

var (
	loginAttemptsMu sync.Mutex
	loginAttempts   = map[string]*loginAttempt{}
)

// clientIP extrai o IP real do cliente respeitando o header X-Real-IP do nginx.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// checkRateLimit retorna false se o IP excedeu o limite e deve ser bloqueado.
func checkRateLimit(ip string) bool {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	now := time.Now()
	a, ok := loginAttempts[ip]
	if !ok {
		loginAttempts[ip] = &loginAttempt{count: 1, firstSeen: now}
		return true
	}
	if now.Before(a.blockedUntil) {
		return false
	}
	if now.After(a.firstSeen.Add(rateLimitWindow)) {
		a.count = 1
		a.firstSeen = now
		a.blockedUntil = time.Time{}
		return true
	}
	a.count++
	if a.count > rateLimitMax {
		a.blockedUntil = now.Add(rateLimitLockout)
		log.Printf("[SECURITY] IP bloqueado por rate limit: %s (%d tentativas em %s)", ip, a.count, rateLimitWindow)
		return false
	}
	return true
}

// recordLoginSuccess reseta o contador do IP após autenticação bem-sucedida.
func recordLoginSuccess(ip string) {
	loginAttemptsMu.Lock()
	delete(loginAttempts, ip)
	loginAttemptsMu.Unlock()
}

// cleanRateLimiter remove entradas expiradas periodicamente.
func cleanRateLimiter() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		loginAttemptsMu.Lock()
		now := time.Now()
		for ip, a := range loginAttempts {
			if now.After(a.firstSeen.Add(rateLimitWindow)) && now.After(a.blockedUntil) {
				delete(loginAttempts, ip)
			}
		}
		loginAttemptsMu.Unlock()
	}
}

// effectiveUser aplica as configurações do grupo sobre o usuário (role, acesso, MFA).
// Usuários sem grupo mantêm suas configurações individuais.
func effectiveUser(u User) User {
	if u.GroupName == "" { return u }
	groupsMu.RLock()
	g, ok := groupsMap[u.GroupName]
	groupsMu.RUnlock()
	if !ok { return u }
	u.AllowedClusters  = g.AllowedClusters
	u.AllowedNamespaces = g.AllowedNamespaces
	u.TOTPEnabled      = g.TOTPEnabled
	return u
}

func initAuth() {
	// JWT secret
	secret := os.Getenv("JWT_SECRET")
	if secret != "" {
		jwtSecret = []byte(secret)
	} else {
		jwtSecret = make([]byte, 32)
		rand.Read(jwtSecret)
		log.Println("JWT_SECRET nao definido, usando chave aleatoria (tokens invalidados ao reiniciar)")
	}

	// Carrega usuários do DB se disponível
	if db != nil {
		rows, err := db.Query(`SELECT username, password, role, group_name, allowed_clusters, allowed_namespaces, totp_secret, totp_enabled FROM users`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var u User
				var acStr, anStr, totpSecret string
				var totpEnabled int
				if rows.Scan(&u.Username, &u.Password, &u.Role, &u.GroupName, &acStr, &anStr, &totpSecret, &totpEnabled) == nil {
					if acStr != ""  { u.AllowedClusters   = strings.Split(acStr, ",") }
					if anStr != ""  { u.AllowedNamespaces = strings.Split(anStr, ",") }
					plain, _ := decryptTOTP(totpSecret)
					u.TOTPSecret  = plain
					u.TOTPEnabled = totpEnabled == 1
					usersMap[u.Username] = u
				}
			}
		}
	}

	// Goroutine de limpeza de tokens MFA temporários
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			mfaTokensMu.Lock()
			for k, v := range mfaTokens {
				if time.Now().After(v.exp) { delete(mfaTokens, k) }
			}
			mfaTokensMu.Unlock()
			mfaSetupTokensMu.Lock()
			for k, v := range mfaSetupTokens {
				if time.Now().After(v.exp) { delete(mfaSetupTokens, k) }
			}
			mfaSetupTokensMu.Unlock()
		}
	}()

	// Cria admin padrão se não existir nenhum usuário admin
	usersMu.RLock()
	hasAdmin := false
	for _, u := range usersMap {
		if u.Role == RoleAdmin { hasAdmin = true; break }
	}
	usersMu.RUnlock()

	if !hasAdmin {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcryptCost)
		u := User{Username: "admin", Password: string(hash), Role: RoleAdmin}
		usersMu.Lock()
		usersMap["admin"] = u
		usersMu.Unlock()
		if db != nil {
			db.Exec(`INSERT INTO users (username, password, role) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
				u.Username, u.Password, u.Role)
		}
		log.Println("Usuário admin padrão criado (senha: admin)")
	}

	// Aviso crítico se a senha padrão 'admin' ainda estiver em uso
	usersMu.RLock()
	adminUser, adminExists := usersMap["admin"]
	usersMu.RUnlock()
	if adminExists && bcrypt.CompareHashAndPassword([]byte(adminUser.Password), []byte("admin")) == nil {
		log.Println("╔══════════════════════════════════════════════════════════════════╗")
		log.Println("║  [SEGURANÇA] AVISO CRÍTICO: senha padrão 'admin' detectada!      ║")
		log.Println("║  Altere imediatamente em: Administração > Usuários > admin        ║")
		log.Println("╚══════════════════════════════════════════════════════════════════╝")
	}

	// Migra seeds TOTP em plaintext legados para AES-256-GCM
	migratePlaintextTOTP()
}

func createToken(u User) (string, error) {
	claims := TokenClaims{
		Username: u.Username, Role: u.Role, Exp: time.Now().Add(24 * time.Hour).Unix(),
		AllowedClusters: u.AllowedClusters, AllowedNamespaces: u.AllowedNamespaces,
	}
	payload, err := json.Marshal(claims)
	if err != nil { return "", err }
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + sig, nil
}

func validateToken(tokenStr string) (*TokenClaims, error) {
	parts := strings.SplitN(tokenStr, ".", 2)
	if len(parts) != 2 { return nil, fmt.Errorf("token invalido") }
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return nil, fmt.Errorf("assinatura invalida")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil { return nil, err }
	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil { return nil, err }
	if time.Now().Unix() > claims.Exp { return nil, fmt.Errorf("token expirado") }
	if isTokenBlacklisted(tokenStr) { return nil, fmt.Errorf("token revogado") }
	return &claims, nil
}

func extractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") { return strings.TrimPrefix(h, "Bearer ") }
	return ""
}

// ── Middlewares ───────────────────────────────────────────────────────────────

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if frontendOrigin == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			// Apenas reflete a origem se coincidir com o valor configurado
			if origin := r.Header.Get("Origin"); origin == frontendOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions { w.WriteHeader(http.StatusOK); return }
		next(w, r)
	}
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		claims, err := validateToken(extractToken(r))
		if err != nil { http.Error(w, "Unauthorized", 401); return }
		ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
		next(w, r.WithContext(ctx))
	})
}

func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
		if claims.Role != RoleAdmin { http.Error(w, "Forbidden", 403); return }
		next(w, r)
	})
}

func devOrAdminOnly(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
		if claims.Role != RoleAdmin && claims.Role != RoleDev {
			http.Error(w, "Forbidden", 403)
			return
		}
		next(w, r)
	})
}

// readerOrAdminOnly bloqueia devs (que só podem acessar /api/logs)
func readerOrAdminOnly(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
		if claims.Role == RoleDev {
			http.Error(w, "Forbidden", 403)
			return
		}
		next(w, r)
	})
}

// devAllowedCluster verifica se o dev tem acesso ao cluster; não-devs sempre têm acesso.
func devAllowedCluster(claims *TokenClaims, clusterName string) bool {
	if claims.Role != RoleDev { return true }
	for _, c := range claims.AllowedClusters {
		if c == clusterName { return true }
	}
	return false
}

// devAllowedNamespace verifica se o dev tem acesso ao namespace; não-devs sempre têm acesso.
func devAllowedNamespace(claims *TokenClaims, ns string) bool {
	if claims.Role != RoleDev { return true }
	for _, n := range claims.AllowedNamespaces {
		if n == ns { return true }
	}
	return false
}

// ── Handlers de autenticação ──────────────────────────────────────────────────

func handleLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tokenStr := extractToken(r)
	if claims, err := validateToken(tokenStr); err == nil {
		blacklistToken(tokenStr, time.Unix(claims.Exp, 0))
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "Logout realizado com sucesso"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ip := clientIP(r)
	if !checkRateLimit(ip) {
		w.Header().Set("Retry-After", "900")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]string{"error": "Muitas tentativas de login. Tente novamente em 15 minutos."})
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	usersMu.RLock()
	u, ok := usersMap[body.Username]
	usersMu.RUnlock()
	if !ok || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(body.Password)) != nil {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Usuário ou senha inválidos"})
		return
	}
	recordLoginSuccess(ip)
	eu := effectiveUser(u)
	// MFA habilitado no grupo mas usuário ainda não configurou → fluxo de setup
	if eu.TOTPEnabled && u.TOTPSecret == "" {
		key, err := totp.Generate(totp.GenerateOpts{Issuer: "PodMonitor", AccountName: u.Username})
		if err != nil { http.Error(w, err.Error(), 500); return }
		img, err := key.Image(200, 200)
		if err != nil { http.Error(w, err.Error(), 500); return }
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil { http.Error(w, err.Error(), 500); return }
		qrCode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
		b := make([]byte, 16); rand.Read(b)
		setupTok := fmt.Sprintf("%x", b)
		mfaSetupTokensMu.Lock()
		mfaSetupTokens[setupTok] = mfaSetupEntry{username: u.Username, secret: key.Secret(), qrCode: qrCode, exp: time.Now().Add(15 * time.Minute)}
		mfaSetupTokensMu.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mfa_setup_required": true,
			"setup_token":        setupTok,
		})
		return
	}
	// MFA habilitado e já configurado → pede o código TOTP
	if eu.TOTPEnabled {
		b := make([]byte, 16); rand.Read(b)
		mfaTok := fmt.Sprintf("%x", b)
		mfaTokensMu.Lock()
		mfaTokens[mfaTok] = mfaEntry{username: u.Username, exp: time.Now().Add(5 * time.Minute)}
		mfaTokensMu.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mfa_required": true,
			"mfa_token":    mfaTok,
		})
		return
	}
	token, err := createToken(eu)
	if err != nil { http.Error(w, err.Error(), 500); return }
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":              token,
		"username":           eu.Username,
		"role":               eu.Role,
		"allowed_clusters":   eu.AllowedClusters,
		"allowed_namespaces": eu.AllowedNamespaces,
	})
}

func handleGetUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	usersMu.RLock()
	defer usersMu.RUnlock()
	list := make([]map[string]interface{}, 0, len(usersMap))
	for _, u := range usersMap {
		eu := effectiveUser(u)
		list = append(list, map[string]interface{}{
			"username":           u.Username,
			"role":               u.Role,
			"group_name":         u.GroupName,
			"allowed_clusters":   eu.AllowedClusters,
			"allowed_namespaces": eu.AllowedNamespaces,
			"totp_enabled":       eu.TOTPEnabled,
			"totp_configured":    u.TOTPSecret != "",
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i]["username"].(string) < list[j]["username"].(string) })
	json.NewEncoder(w).Encode(list)
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Role      string `json:"role"`
		GroupName string `json:"group_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	if body.Username == "" || body.Password == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "username e password obrigatorios"})
		return
	}
	// Valida grupo se informado
	if body.GroupName != "" {
		groupsMu.RLock()
		_, ok := groupsMap[body.GroupName]
		groupsMu.RUnlock()
		if !ok {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "grupo não encontrado"})
			return
		}
	}
	if body.Role != RoleAdmin && body.Role != RoleReader && body.Role != RoleDev {
		body.Role = RoleReader
	}
	usersMu.RLock()
	_, exists := usersMap[body.Username]
	usersMu.RUnlock()
	if exists {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "usuário já existe"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcryptCost)
	if err != nil { http.Error(w, err.Error(), 500); return }
	u := User{Username: body.Username, Password: string(hash), Role: body.Role, GroupName: body.GroupName}
	usersMu.Lock()
	usersMap[body.Username] = u
	usersMu.Unlock()
	if db != nil {
		db.Exec(`INSERT INTO users (username, password, role, group_name) VALUES ($1,$2,$3,$4)`,
			u.Username, u.Password, u.Role, u.GroupName)
	}
	// Adiciona ao mapa do grupo em memória
	if body.GroupName != "" {
		groupsMu.Lock()
		if g, ok := groupsMap[body.GroupName]; ok {
			g.Members = append(g.Members, body.Username)
		}
		groupsMu.Unlock()
	}
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	auditLog(claims.Username, claims.Role, "user_create", body.Username+" role="+body.Role, r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]string{"message": "usuário criado com sucesso"})
}

func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	if body.Username == "" || body.Password == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "username e password obrigatorios"})
		return
	}
	usersMu.RLock()
	_, exists := usersMap[body.Username]
	usersMu.RUnlock()
	if !exists {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "usuário não encontrado"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcryptCost)
	if err != nil { http.Error(w, err.Error(), 500); return }
	usersMu.Lock()
	u := usersMap[body.Username]
	u.Password = string(hash)
	usersMap[body.Username] = u
	usersMu.Unlock()
	if db != nil {
		db.Exec(`UPDATE users SET password = $1 WHERE username = $2`, string(hash), body.Username)
	}
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	auditLog(claims.Username, claims.Role, "password_change", body.Username, r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]string{"message": "senha alterada com sucesso"})
}

func handleUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	validRoles := map[string]bool{RoleAdmin: true, RoleDev: true, RoleReader: true}
	if !validRoles[body.Role] {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "perfil inválido"}); return
	}
	usersMu.Lock()
	u, ok := usersMap[body.Username]
	if !ok {
		usersMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "usuário não encontrado"}); return
	}
	u.Role = body.Role
	usersMap[body.Username] = u
	usersMu.Unlock()
	if db != nil {
		db.Exec(`UPDATE users SET role=$1 WHERE username=$2`, body.Role, body.Username)
	}
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	auditLog(claims.Username, claims.Role, "role_change", fmt.Sprintf("%s -> %s", body.Username, body.Role), r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]string{"message": "perfil atualizado"})
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// username vem como query param: DELETE /api/auth/users?username=foo
	target := r.URL.Query().Get("username")
	if target == "" { http.Error(w, "username obrigatorio", 400); return }

	// Não pode deletar a si mesmo
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	if claims.Username == target {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "nao e possivel deletar o proprio usuario"})
		return
	}
	usersMu.Lock()
	removed := usersMap[target]
	delete(usersMap, target)
	usersMu.Unlock()
	if db != nil {
		db.Exec(`DELETE FROM users WHERE username = $1`, target)
	}
	// Remove do grupo em memória
	if removed.GroupName != "" {
		groupsMu.Lock()
		if g, ok := groupsMap[removed.GroupName]; ok {
			filtered := g.Members[:0]
			for _, m := range g.Members { if m != target { filtered = append(filtered, m) } }
			g.Members = filtered
		}
		groupsMu.Unlock()
	}
	auditLog(claims.Username, claims.Role, "user_delete", target, r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]string{"message": "usuário removido"})
}

func handleUpdateDevAccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Username          string   `json:"username"`
		AllowedClusters   []string `json:"allowed_clusters"`
		AllowedNamespaces []string `json:"allowed_namespaces"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	if body.Username == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "username obrigatorio"})
		return
	}
	usersMu.Lock()
	u, exists := usersMap[body.Username]
	if !exists {
		usersMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "usuário não encontrado"})
		return
	}
	u.AllowedClusters   = body.AllowedClusters
	u.AllowedNamespaces = body.AllowedNamespaces
	usersMap[body.Username] = u
	usersMu.Unlock()
	if db != nil {
		db.Exec(`UPDATE users SET allowed_clusters=$1, allowed_namespaces=$2 WHERE username=$3`,
			strings.Join(body.AllowedClusters, ","), strings.Join(body.AllowedNamespaces, ","), body.Username)
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "acesso atualizado com sucesso"})
}

// handleMFAValidate valida o código TOTP durante o login e retorna o JWT completo.
func handleMFAValidate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ip := clientIP(r)
	if !checkRateLimit(ip) {
		w.Header().Set("Retry-After", "900")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]string{"error": "Muitas tentativas. Tente novamente em 15 minutos."})
		return
	}

	var body struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	mfaTokensMu.Lock()
	entry, ok := mfaTokens[body.MFAToken]
	if ok { delete(mfaTokens, body.MFAToken) }
	mfaTokensMu.Unlock()

	if !ok || time.Now().After(entry.exp) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sessão expirada, faça login novamente"})
		return
	}
	usersMu.RLock()
	u, exists := usersMap[entry.username]
	usersMu.RUnlock()
	if !exists {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Usuário não encontrado"})
		return
	}
	if !totp.Validate(body.Code, u.TOTPSecret) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Código MFA inválido"})
		return
	}
	recordLoginSuccess(ip)
	eu := effectiveUser(u)
	token, err := createToken(eu)
	if err != nil { http.Error(w, err.Error(), 500); return }
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":              token,
		"username":           eu.Username,
		"role":               eu.Role,
		"allowed_clusters":   eu.AllowedClusters,
		"allowed_namespaces": eu.AllowedNamespaces,
	})
}

// handleAdminToggleMFA habilita ou desabilita MFA para um usuário (admin only).
// handleMFASetupInfo retorna o QR code e o secret associados a um setup token.
func handleMFASetupInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	token := r.URL.Query().Get("token")
	mfaSetupTokensMu.Lock()
	entry, ok := mfaSetupTokens[token]
	mfaSetupTokensMu.Unlock()
	if !ok || time.Now().After(entry.exp) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Link expirado ou inválido, faça login novamente"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"username": entry.username,
		"qr_code":  entry.qrCode,
		"secret":   entry.secret,
	})
}

// handleMFASetupConfirm valida o código TOTP do setup, persiste o secret e retorna o JWT.
func handleMFASetupConfirm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		SetupToken string `json:"setup_token"`
		Code       string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	mfaSetupTokensMu.Lock()
	entry, ok := mfaSetupTokens[body.SetupToken]
	mfaSetupTokensMu.Unlock()
	if !ok || time.Now().After(entry.exp) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Link expirado, faça login novamente"})
		return
	}
	if !totp.Validate(body.Code, entry.secret) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{"error": "Código inválido, tente novamente"})
		return
	}
	// Código correto: persiste e remove o setup token
	mfaSetupTokensMu.Lock()
	delete(mfaSetupTokens, body.SetupToken)
	mfaSetupTokensMu.Unlock()
	usersMu.Lock()
	u, exists := usersMap[entry.username]
	if !exists {
		usersMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "Usuário não encontrado"})
		return
	}
	u.TOTPSecret  = entry.secret
	u.TOTPEnabled = true
	usersMap[entry.username] = u
	usersMu.Unlock()
	if db != nil {
		encSecret, err := encryptTOTP(entry.secret)
		if err != nil { http.Error(w, "erro ao cifrar TOTP", 500); return }
		db.Exec(`UPDATE users SET totp_secret=$1, totp_enabled=1 WHERE username=$2`, encSecret, entry.username)
	}
	auditLog(entry.username, "", "mfa_setup", entry.username, r.RemoteAddr)
	eu := effectiveUser(u)
	token, err := createToken(eu)
	if err != nil { http.Error(w, err.Error(), 500); return }
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":              token,
		"username":           eu.Username,
		"role":               eu.Role,
		"allowed_clusters":   eu.AllowedClusters,
		"allowed_namespaces": eu.AllowedNamespaces,
	})
}

// handleMFAResetUser limpa o secret TOTP de um usuário (admin only).
// No próximo login, o fluxo de setup será acionado novamente.
func handleMFAResetUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct{ Username string `json:"username"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	if body.Username == "" { http.Error(w, "username obrigatorio", 400); return }
	usersMu.Lock()
	u, ok := usersMap[body.Username]
	if !ok {
		usersMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "usuário não encontrado"})
		return
	}
	u.TOTPSecret  = ""
	u.TOTPEnabled = false
	usersMap[body.Username] = u
	usersMu.Unlock()
	if db != nil {
		db.Exec(`UPDATE users SET totp_secret='', totp_enabled=0 WHERE username=$1`, body.Username)
	}
	adminClaims, _ := r.Context().Value(claimsCtxKey).(*TokenClaims)
	if adminClaims != nil {
		auditLog(adminClaims.Username, adminClaims.Role, "mfa_reset", body.Username, r.RemoteAddr)
	}
	json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("MFA de '%s' resetado. No próximo login o setup será solicitado.", body.Username)})
}

func handleAdminToggleMFA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Username string `json:"username"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	if body.Username == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "username obrigatorio"})
		return
	}
	usersMu.Lock()
	u, exists := usersMap[body.Username]
	if !exists {
		usersMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "usuário não encontrado"})
		return
	}
	if !body.Enabled {
		u.TOTPSecret  = ""
		u.TOTPEnabled = false
		usersMap[body.Username] = u
		usersMu.Unlock()
		if db != nil {
			db.Exec(`UPDATE users SET totp_secret='', totp_enabled=0 WHERE username=$1`, body.Username)
		}
		togClaims, _ := r.Context().Value(claimsCtxKey).(*TokenClaims)
		if togClaims != nil {
			auditLog(togClaims.Username, togClaims.Role, "mfa_disable", body.Username, r.RemoteAddr)
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "MFA desativado"})
		return
	}
	// Gera novo secret TOTP
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "PodMonitor",
		AccountName: body.Username,
	})
	if err != nil {
		usersMu.Unlock()
		http.Error(w, err.Error(), 500); return
	}
	u.TOTPSecret  = key.Secret()
	u.TOTPEnabled = true
	usersMap[body.Username] = u
	usersMu.Unlock()
	if db != nil {
		encSecret, _ := encryptTOTP(key.Secret())
		db.Exec(`UPDATE users SET totp_secret=$1, totp_enabled=1 WHERE username=$2`, encSecret, body.Username)
	}
	// Gera QR code como PNG base64
	img, err := key.Image(200, 200)
	if err != nil { http.Error(w, err.Error(), 500); return }
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil { http.Error(w, err.Error(), 500); return }
	qrCode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	togEnClaims, _ := r.Context().Value(claimsCtxKey).(*TokenClaims)
	if togEnClaims != nil {
		auditLog(togEnClaims.Username, togEnClaims.Role, "mfa_enable", body.Username, r.RemoteAddr)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "MFA ativado",
		"qr_code": qrCode,
		"secret":  key.Secret(),
	})
}

// ── Grupos ───────────────────────────────────────────────────────────────────

func initGroups() {
	// Carrega grupos do DB
	if db != nil {
		rows, err := db.Query(`SELECT name, role, totp_enabled, allowed_clusters, allowed_namespaces FROM groups`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var g Group
				var acStr, anStr string
				var totpEnabled int
				if rows.Scan(&g.Name, &g.Role, &totpEnabled, &acStr, &anStr) == nil {
					g.TOTPEnabled = totpEnabled == 1
					if acStr != "" { g.AllowedClusters   = strings.Split(acStr, ",") }
					if anStr != "" { g.AllowedNamespaces = strings.Split(anStr, ",") }
					gc := g
					groupsMap[g.Name] = &gc
				}
			}
		}
		// Carrega membros
		uRows, err := db.Query(`SELECT username, group_name FROM users WHERE group_name != ''`)
		if err == nil {
			defer uRows.Close()
			for uRows.Next() {
				var uname, gname string
				if uRows.Scan(&uname, &gname) == nil {
					groupsMu.Lock()
					if g, ok := groupsMap[gname]; ok {
						g.Members = append(g.Members, uname)
					}
					groupsMu.Unlock()
				}
			}
		}
	}

	// Grupos e usuários padrão
	type seedUser struct{ username, password, group, role string }
	seedGroups := []Group{
		{Name: "Desenvolvedores"},
		{Name: "Readers"},
	}
	seedUsers := []seedUser{
		{"dev",             "dev",             "Desenvolvedores", RoleDev},
		{"dev2",            "dev2",            "Desenvolvedores", RoleDev},
		{"fulano",          "fulano",          "Readers",         RoleReader},
		{"wellington.maia", "wellington.maia", "Readers",         RoleReader},
	}

	for _, sg := range seedGroups {
		groupsMu.RLock()
		_, exists := groupsMap[sg.Name]
		groupsMu.RUnlock()
		if !exists {
			gc := sg
			groupsMu.Lock()
			groupsMap[sg.Name] = &gc
			groupsMu.Unlock()
			if db != nil {
				db.Exec(`INSERT INTO groups (name, role) VALUES ($1,$2) ON CONFLICT DO NOTHING`, sg.Name, sg.Role)
			}
			log.Printf("Grupo padrão criado: %s (%s)", sg.Name, sg.Role)
		}
	}

	for _, su := range seedUsers {
		usersMu.RLock()
		_, exists := usersMap[su.username]
		usersMu.RUnlock()
		if !exists {
			hash, _ := bcrypt.GenerateFromPassword([]byte(su.password), bcryptCost)
			groupsMu.RLock()
			g, _ := groupsMap[su.group]
			groupsMu.RUnlock()
			u := User{Username: su.username, Password: string(hash), Role: su.role, GroupName: su.group}
			usersMu.Lock()
			usersMap[su.username] = u
			usersMu.Unlock()
			if db != nil {
				db.Exec(`INSERT INTO users (username, password, role, group_name) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
					u.Username, u.Password, u.Role, u.GroupName)
			}
			groupsMu.Lock()
			if g != nil { g.Members = append(g.Members, su.username) }
			groupsMu.Unlock()
			log.Printf("Usuário padrão criado: %s (grupo: %s, senha padrão = username)", su.username, su.group)
		}
	}
}

func handleGetGroups(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	groupsMu.RLock()
	defer groupsMu.RUnlock()
	list := make([]*Group, 0, len(groupsMap))
	for _, g := range groupsMap { list = append(list, g) }
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	json.NewEncoder(w).Encode(list)
}

func handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	if body.Name == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "name obrigatorio"})
		return
	}
	if body.Role != RoleAdmin && body.Role != RoleReader && body.Role != RoleDev {
		body.Role = RoleReader
	}
	groupsMu.RLock()
	_, exists := groupsMap[body.Name]
	groupsMu.RUnlock()
	if exists {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "grupo já existe"})
		return
	}
	g := &Group{Name: body.Name, Role: body.Role}
	groupsMu.Lock()
	groupsMap[body.Name] = g
	groupsMu.Unlock()
	if db != nil {
		db.Exec(`INSERT INTO groups (name, role) VALUES ($1,$2)`, g.Name, g.Role)
	}
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	auditLog(claims.Username, claims.Role, "group_create", body.Name+" role="+body.Role, r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]string{"message": "grupo criado com sucesso"})
}

func handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	name := r.URL.Query().Get("name")
	if name == "" { http.Error(w, "name obrigatorio", 400); return }
	groupsMu.Lock()
	g, ok := groupsMap[name]
	if !ok {
		groupsMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "grupo não encontrado"})
		return
	}
	members := append([]string{}, g.Members...)
	delete(groupsMap, name)
	groupsMu.Unlock()
	// Desvincula membros do grupo
	usersMu.Lock()
	for _, m := range members {
		if u, ok := usersMap[m]; ok {
			u.GroupName = ""
			usersMap[m] = u
		}
	}
	usersMu.Unlock()
	if db != nil {
		db.Exec(`DELETE FROM groups WHERE name = $1`, name)
		db.Exec(`UPDATE users SET group_name = '' WHERE group_name = $1`, name)
	}
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	auditLog(claims.Username, claims.Role, "group_delete", name, r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]string{"message": "grupo removido"})
}

func handleGroupAccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Name              string   `json:"name"`
		AllowedClusters   []string `json:"allowed_clusters"`
		AllowedNamespaces []string `json:"allowed_namespaces"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	groupsMu.Lock()
	g, ok := groupsMap[body.Name]
	if !ok {
		groupsMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "grupo não encontrado"})
		return
	}
	g.AllowedClusters   = body.AllowedClusters
	g.AllowedNamespaces = body.AllowedNamespaces
	groupsMu.Unlock()
	if db != nil {
		db.Exec(`UPDATE groups SET allowed_clusters=$1, allowed_namespaces=$2 WHERE name=$3`,
			strings.Join(body.AllowedClusters, ","), strings.Join(body.AllowedNamespaces, ","), body.Name)
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "acesso atualizado"})
}

func handleGroupMFA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	groupsMu.Lock()
	g, ok := groupsMap[body.Name]
	if !ok {
		groupsMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "grupo não encontrado"})
		return
	}
	g.TOTPEnabled = body.Enabled
	members := append([]string{}, g.Members...)
	groupsMu.Unlock()

	if db != nil {
		enabled := 0
		if body.Enabled { enabled = 1 }
		db.Exec(`UPDATE groups SET totp_enabled=$1 WHERE name=$2`, enabled, body.Name)
	}

	if !body.Enabled {
		// Limpa secrets de todos os membros ao desativar
		usersMu.Lock()
		for _, m := range members {
			if u, ok := usersMap[m]; ok {
				u.TOTPSecret  = ""
				u.TOTPEnabled = false
				usersMap[m] = u
			}
		}
		usersMu.Unlock()
		if db != nil {
			for _, m := range members {
				db.Exec(`UPDATE users SET totp_secret='', totp_enabled=0 WHERE username=$1`, m)
			}
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "MFA desativado para o grupo"})
		return
	}
	// Ao ativar: apenas seta a flag. Cada membro configura seu próprio MFA no próximo login.
	json.NewEncoder(w).Encode(map[string]string{"message": "MFA ativado para o grupo. Cada membro configurará o MFA no próximo acesso."})
}

func handleGroupMembers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Group    string `json:"group"`
		Username string `json:"username"`
		Action   string `json:"action"` // "add" | "remove"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400); return
	}
	groupsMu.Lock()
	g, ok := groupsMap[body.Group]
	if !ok {
		groupsMu.Unlock()
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "grupo não encontrado"})
		return
	}
	if body.Action == "add" {
		usersMu.Lock()
		u, uok := usersMap[body.Username]
		if !uok {
			usersMu.Unlock()
			groupsMu.Unlock()
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"error": "usuário não encontrado"})
			return
		}
		// Remove do grupo anterior
		if u.GroupName != "" && u.GroupName != body.Group {
			if oldG, ok := groupsMap[u.GroupName]; ok {
				filtered := oldG.Members[:0]
				for _, m := range oldG.Members { if m != body.Username { filtered = append(filtered, m) } }
				oldG.Members = filtered
			}
		}
		u.GroupName = body.Group
		usersMap[body.Username] = u
		usersMu.Unlock()
		// Evita duplicados
		found := false
		for _, m := range g.Members { if m == body.Username { found = true; break } }
		if !found { g.Members = append(g.Members, body.Username) }
		groupsMu.Unlock()
		if db != nil {
			db.Exec(`UPDATE users SET group_name=$1 WHERE username=$2`, body.Group, body.Username)
		}
	} else if body.Action == "remove" {
		filtered := g.Members[:0]
		for _, m := range g.Members { if m != body.Username { filtered = append(filtered, m) } }
		g.Members = filtered
		groupsMu.Unlock()
		usersMu.Lock()
		if u, ok := usersMap[body.Username]; ok {
			u.GroupName = ""
			usersMap[body.Username] = u
		}
		usersMu.Unlock()
		if db != nil {
			db.Exec(`UPDATE users SET group_name='' WHERE username=$1`, body.Username)
		}
	} else {
		groupsMu.Unlock()
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "action deve ser 'add' ou 'remove'"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
}

// ── Admin: tipos ──────────────────────────────────────────────────────────────

type AdminValidateRequest struct{ Kubeconfig string `json:"kubeconfig"` }
type AdminValidateResponse struct {
	Contexts []string `json:"contexts"`
	Error    string   `json:"error,omitempty"`
}
type AdminCreateSARequest struct {
	Kubeconfig string `json:"kubeconfig"`
	Context    string `json:"context"`
	Namespace  string `json:"namespace"`
}
type AdminCreateSAResponse struct {
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
}
type AdminApplyRequest struct{ Kubeconfig string `json:"kubeconfig"` }
type AdminApplyResponse struct {
	Message  string   `json:"message"`
	Clusters []string `json:"clusters"`
	Error    string   `json:"error,omitempty"`
}

// ── Estado global ─────────────────────────────────────────────────────────────

var (
	clusters       map[string]*clusterClients
	localK8sClient *kubernetes.Clientset
	db             *sql.DB
)

// ── Inicialização ─────────────────────────────────────────────────────────────

func initClients() {
	clusters = map[string]*clusterClients{}
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath != "" {
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
		rawConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules, &clientcmd.ConfigOverrides{},
		).RawConfig()
		if err != nil {
			log.Printf("Aviso: nao foi possivel carregar kubeconfig %s: %v", kubeconfigPath, err)
		} else {
			for ctxName := range rawConfig.Contexts {
				cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
					loadingRules, &clientcmd.ConfigOverrides{CurrentContext: ctxName},
				).ClientConfig()
				if err != nil { log.Printf("Erro config %s: %v", ctxName, err); continue }
				k8s, err := kubernetes.NewForConfig(cfg)
				if err != nil { log.Printf("Erro k8s %s: %v", ctxName, err); continue }
				mc, err := metricsclient.NewForConfig(cfg)
				if err != nil { log.Printf("Erro metrics %s: %v", ctxName, err); continue }
				clusters[ctxName] = &clusterClients{k8s: k8s, metrics: mc}
				log.Printf("Cluster registrado: %s", ctxName)
			}
		}
	}
	if cfg, err := rest.InClusterConfig(); err == nil {
		localK8sClient, _ = kubernetes.NewForConfig(cfg)
		name := os.Getenv("CLUSTER_NAME")
		if name == "" { name = "local" }
		if _, exists := clusters[name]; !exists {
			k8s, err := kubernetes.NewForConfig(cfg)
			if err != nil { log.Fatalf("Erro k8s in-cluster: %v", err) }
			mc, err := metricsclient.NewForConfig(cfg)
			if err != nil { log.Fatalf("Erro metrics in-cluster: %v", err) }
			clusters[name] = &clusterClients{k8s: k8s, metrics: mc}
			log.Printf("Cluster in-cluster registrado: %s", name)
		}
	}
	if len(clusters) == 0 {
		log.Printf("Aviso: nenhum cluster configurado. Adicione clusters via interface de admin.")
	}
}

func clusterNames() []string {
	names := make([]string, 0, len(clusters))
	for n := range clusters { names = append(names, n) }
	sort.Strings(names)
	return names
}

func getCluster(r *http.Request) (*clusterClients, string, error) {
	name := r.URL.Query().Get("cluster")
	if name == "" {
		names := clusterNames()
		if len(names) == 0 { return nil, "", fmt.Errorf("nenhum cluster disponivel") }
		name = names[0]
	}
	c, ok := clusters[name]
	if !ok { return nil, "", fmt.Errorf("cluster %q nao encontrado", name) }
	return c, name, nil
}

func getOwnNamespace() string {
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return strings.TrimSpace(string(data))
	}
	if ns := os.Getenv("NAMESPACE"); ns != "" { return ns }
	return "pod-monitor"
}

// ── PostgreSQL ────────────────────────────────────────────────────────────────

func initDB() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Println("DATABASE_URL não configurado, histórico e usuários não persistidos")
		return
	}
	if connectDB(dsn) {
		go cleanupLoop()
		return
	}
	// Falha na subida (ex.: DNS do cluster ainda não pronto) não deve deixar o
	// backend rodando sem persistência até um restart manual — tenta de novo
	// em segundo plano com backoff exponencial, sem bloquear o startup do HTTP.
	go dbReconnectLoop(dsn)
}

// dbReconnectLoop tenta reconectar ao PostgreSQL indefinidamente, com backoff
// exponencial (5s até um teto de 2min), até obter sucesso.
func dbReconnectLoop(dsn string) {
	backoff := 5 * time.Second
	const maxBackoff = 2 * time.Minute
	for {
		log.Printf("Tentando reconectar ao PostgreSQL em %s...", backoff)
		time.Sleep(backoff)
		if connectDB(dsn) {
			log.Println("PostgreSQL reconectado com sucesso")
			go cleanupLoop()
			return
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connectDB abre a conexão e aplica as migrações; só publica em db se tudo
// tiver sucesso. Retorna false sem alterar db em caso de qualquer falha.
func connectDB(dsn string) bool {
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Printf("Erro ao abrir PostgreSQL: %v", err)
		return false
	}
	if err = conn.Ping(); err != nil {
		log.Printf("Erro ao conectar ao PostgreSQL: %v", err)
		conn.Close()
		return false
	}
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	stmts := []string{
		// captured_at e timestamp armazenados como TEXT ISO-8601 para compatibilidade com frontend
		`CREATE TABLE IF NOT EXISTS users (
			username           TEXT PRIMARY KEY,
			password           TEXT NOT NULL,
			role               TEXT NOT NULL DEFAULT 'reader',
			allowed_clusters   TEXT NOT NULL DEFAULT '',
			allowed_namespaces TEXT NOT NULL DEFAULT '',
			totp_secret        TEXT NOT NULL DEFAULT '',
			totp_enabled       INTEGER NOT NULL DEFAULT 0,
			group_name         TEXT NOT NULL DEFAULT ''
		)`,
		// ADD COLUMN IF NOT EXISTS é seguro para re-execução em DBs existentes
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS group_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS allowed_clusters TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS allowed_namespaces TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS groups (
			name               TEXT PRIMARY KEY,
			role               TEXT NOT NULL DEFAULT 'reader',
			totp_enabled       INTEGER NOT NULL DEFAULT 0,
			allowed_clusters   TEXT NOT NULL DEFAULT '',
			allowed_namespaces TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS pod_snapshots (
			id          BIGSERIAL PRIMARY KEY,
			session_id  TEXT NOT NULL,
			captured_at TEXT NOT NULL DEFAULT to_char(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			cluster     TEXT NOT NULL DEFAULT '',
			namespace   TEXT,
			pod         TEXT,
			node        TEXT,
			container   TEXT,
			cpu_request TEXT,
			cpu_limit   TEXT,
			mem_request TEXT,
			mem_limit   TEXT,
			cpu_usage   TEXT,
			mem_usage   TEXT
		)`,
		`ALTER TABLE pod_snapshots ADD COLUMN IF NOT EXISTS cluster TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_ns_time      ON pod_snapshots(namespace, captured_at)`,
		`CREATE INDEX IF NOT EXISTS idx_cluster_time ON pod_snapshots(cluster, captured_at)`,
		`CREATE TABLE IF NOT EXISTS dashboards (
			id       BIGSERIAL PRIMARY KEY,
			username TEXT NOT NULL,
			name     TEXT NOT NULL DEFAULT 'Meu Dashboard',
			widgets  TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE TABLE IF NOT EXISTS alert_thresholds (
			id        BIGSERIAL PRIMARY KEY,
			cluster   TEXT NOT NULL DEFAULT '',
			namespace TEXT NOT NULL DEFAULT '',
			warn_pct  INTEGER NOT NULL DEFAULT 85,
			crit_pct  INTEGER NOT NULL DEFAULT 90,
			UNIQUE(cluster, namespace)
		)`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			id        BIGSERIAL PRIMARY KEY,
			timestamp TEXT NOT NULL DEFAULT to_char(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			username  TEXT NOT NULL,
			role      TEXT NOT NULL DEFAULT '',
			action    TEXT NOT NULL,
			detail    TEXT NOT NULL DEFAULT '',
			ip        TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS webhooks (
			id      BIGSERIAL PRIMARY KEY,
			name    TEXT NOT NULL,
			url     TEXT NOT NULL,
			events  TEXT NOT NULL DEFAULT 'critical',
			enabled INTEGER NOT NULL DEFAULT 1
		)`,
	}
	for _, s := range stmts {
		if _, err = conn.Exec(s); err != nil {
			log.Printf("Erro ao inicializar tabelas: %v — %.80s", err, s)
			conn.Close()
			return false
		}
	}
	db = conn
	log.Println("PostgreSQL inicializado")
	return true
}

func cleanupLoop() {
	ticker := time.NewTicker(6 * time.Hour)
	for range ticker.C {
		if db == nil { continue }
		cutoff := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)
		res, err := db.Exec("DELETE FROM pod_snapshots WHERE captured_at < $1", cutoff)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 { log.Printf("Limpeza: %d snapshots removidos", n) }
		}
	}
}

func newSessionID() string {
	b := make([]byte, 6); rand.Read(b); return fmt.Sprintf("%x", b)
}

func saveSnapshot(sessionID, cluster string, pods []PodResources) {
	if db == nil { return }
	tx, err := db.Begin()
	if err != nil { return }
	stmt, err := tx.Prepare(`INSERT INTO pod_snapshots
		(session_id,cluster,namespace,pod,node,container,cpu_request,cpu_limit,mem_request,mem_limit,cpu_usage,mem_usage)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`)
	if err != nil { tx.Rollback(); return }
	defer stmt.Close()
	for _, pod := range pods {
		for _, c := range pod.Containers {
			stmt.Exec(sessionID, cluster, pod.Namespace, pod.Name, pod.Node, c.Name,
				c.CPURequest, c.CPULimit, c.MemoryRequest, c.MemoryLimit, c.CPUUsage, c.MemoryUsage)
		}
	}
	tx.Commit()
}

// ── Lógica de monitoramento ───────────────────────────────────────────────────

func getPodsResources(clients *clusterClients, namespace string) ([]PodResources, error) {
	ctx := context.Background()
	pods, err := clients.k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil { return nil, fmt.Errorf("listar pods: %w", err) }
	podMetrics, err := clients.metrics.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil { log.Printf("metrics-server indisponivel: %v", err); podMetrics = &metricsv1beta1.PodMetricsList{} }
	metricsMap := map[string]metricsv1beta1.PodMetrics{}
	for _, pm := range podMetrics.Items { metricsMap[pm.Name] = pm }
	var result []PodResources
	for _, pod := range pods.Items {
		phase := string(pod.Status.Phase)
		if phase == "" { phase = "Unknown" }
		// Derivar status mais detalhado (ex: ImagePullBackOff, CrashLoopBackOff)
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				phase = cs.State.Waiting.Reason
				break
			}
		}
		pr := PodResources{Name: pod.Name, Namespace: pod.Namespace, Node: pod.Spec.NodeName, Phase: phase}
		cmMap := map[string]metricsv1beta1.ContainerMetrics{}
		for _, cm := range metricsMap[pod.Name].Containers { cmMap[cm.Name] = cm }
		for _, c := range pod.Spec.Containers {
			cr := ContainerResources{
				Name: c.Name, Image: c.Image,
				CPURequest: c.Resources.Requests.Cpu().String(), MemoryRequest: c.Resources.Requests.Memory().String(),
				CPULimit:   c.Resources.Limits.Cpu().String(),   MemoryLimit:   c.Resources.Limits.Memory().String(),
			}
			if cm, ok := cmMap[c.Name]; ok { cr.CPUUsage = cm.Usage.Cpu().String(); cr.MemoryUsage = cm.Usage.Memory().String() }
			pr.Containers = append(pr.Containers, cr)
		}
		result = append(result, pr)
	}
	return result, nil
}

func getNodesResources(clients *clusterClients) ([]NodeResources, error) {
	ctx := context.Background()
	nodes, err := clients.k8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil { return nil, fmt.Errorf("listar nodes: %w", err) }
	nodeMetrics, err := clients.metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil { log.Printf("metrics-server indisponivel para nodes: %v", err); nodeMetrics = &metricsv1beta1.NodeMetricsList{} }
	metricsMap := map[string]metricsv1beta1.NodeMetrics{}
	for _, nm := range nodeMetrics.Items { metricsMap[nm.Name] = nm }
	var result []NodeResources
	for _, node := range nodes.Items {
		status := "NotReady"
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue { status = "Ready" }
		}
		role := "worker"
		roleSet := map[string]bool{}
		for label := range node.Labels {
			if label == "node-role.kubernetes.io/control-plane" || label == "node-role.kubernetes.io/master" {
				roleSet["control-plane"] = true
			} else if strings.HasPrefix(label, "node-role.kubernetes.io/") {
				roleSet[strings.TrimPrefix(label, "node-role.kubernetes.io/")] = true
			}
		}
		if len(roleSet) > 0 {
			rl := make([]string, 0, len(roleSet)); for r := range roleSet { rl = append(rl, r) }
			sort.Strings(rl); role = strings.Join(rl, ",")
		}
		nr := NodeResources{Name: node.Name, Status: status, Role: role,
			CPUAllocatable: node.Status.Allocatable.Cpu().String(),
			MemAllocatable: node.Status.Allocatable.Memory().String()}
		if nm, ok := metricsMap[node.Name]; ok { nr.CPUUsage = nm.Usage.Cpu().String(); nr.MemUsage = nm.Usage.Memory().String() }
		result = append(result, nr)
	}
	return result, nil
}

// ── Admin: lógica ─────────────────────────────────────────────────────────────

func clientFromKubeconfig(kubeconfigYAML, contextName string) (*kubernetes.Clientset, error) {
	rawConfig, err := clientcmd.Load([]byte(kubeconfigYAML))
	if err != nil { return nil, fmt.Errorf("kubeconfig invalido: %w", err) }
	restCfg, err := clientcmd.NewNonInteractiveClientConfig(*rawConfig, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil { return nil, fmt.Errorf("erro ao criar rest.Config: %w", err) }
	return kubernetes.NewForConfig(restCfg)
}

func mergeKubeconfigs(newYAML string) (*clientcmdapi.Config, error) {
	newCfg, err := clientcmd.Load([]byte(newYAML))
	if err != nil { return nil, fmt.Errorf("kubeconfig invalido: %w", err) }
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" { return newCfg, nil }
	existingBytes, err := os.ReadFile(kubeconfigPath)
	if err != nil { return newCfg, nil }
	existing, err := clientcmd.Load(existingBytes)
	if err != nil { return nil, fmt.Errorf("erro ao ler kubeconfig existente: %w", err) }
	for k, v := range newCfg.Clusters  { existing.Clusters[k]  = v }
	for k, v := range newCfg.AuthInfos { existing.AuthInfos[k] = v }
	for k, v := range newCfg.Contexts  { existing.Contexts[k]  = v }
	return existing, nil
}

// removeClusterFromKubeconfig apaga o contexto (e seu cluster/authinfo, se não
// usados por mais ninguém) do kubeconfig persistido — Secret em modo k8s, ou
// arquivo local em modo standalone (docker-compose) — para que o cluster
// removido não volte a ser registrado num restart do pod.
func removeClusterFromKubeconfig(name string) error {
	if localK8sClient == nil {
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" { kubeconfigPath = "/data/kubeconfig" }
		raw, err := os.ReadFile(kubeconfigPath)
		if err != nil {
			if os.IsNotExist(err) { return nil }
			return err
		}
		cfg, err := clientcmd.Load(raw)
		if err != nil { return err }
		if !deleteContextFromConfig(cfg, name) { return nil }
		out, err := clientcmd.Write(*cfg)
		if err != nil { return err }
		return os.WriteFile(kubeconfigPath, out, 0600)
	}

	ctx := context.Background()
	namespace := getOwnNamespace()
	secretName := os.Getenv("KUBECONFIG_SECRET_NAME")
	if secretName == "" { secretName = "pod-monitor-kubeconfig" }
	secret, err := localK8sClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) { return nil }
	if err != nil { return err }
	cfg, err := clientcmd.Load(secret.Data["config"])
	if err != nil { return err }
	if !deleteContextFromConfig(cfg, name) { return nil }
	out, err := clientcmd.Write(*cfg)
	if err != nil { return err }
	secret.Data["config"] = out
	_, err = localK8sClient.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}

// deleteContextFromConfig remove o contexto ctxName de cfg, junto com seu
// cluster/authinfo caso não sejam referenciados por nenhum outro contexto.
// Retorna false se o contexto não existia (nada a persistir).
func deleteContextFromConfig(cfg *clientcmdapi.Config, ctxName string) bool {
	ctxObj, ok := cfg.Contexts[ctxName]
	if !ok { return false }
	delete(cfg.Contexts, ctxName)
	clusterInUse, authInUse := false, false
	for _, c := range cfg.Contexts {
		if c.Cluster == ctxObj.Cluster { clusterInUse = true }
		if c.AuthInfo == ctxObj.AuthInfo { authInUse = true }
	}
	if !clusterInUse { delete(cfg.Clusters, ctxObj.Cluster) }
	if !authInUse { delete(cfg.AuthInfos, ctxObj.AuthInfo) }
	if cfg.CurrentContext == ctxName { cfg.CurrentContext = "" }
	return true
}

func reloadCluster(kubeconfigYAML, contextName string) {
	rawConfig, err := clientcmd.Load([]byte(kubeconfigYAML))
	if err != nil { return }
	restCfg, err := clientcmd.NewNonInteractiveClientConfig(*rawConfig, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil { return }
	k8s, err := kubernetes.NewForConfig(restCfg)
	if err != nil { return }
	mc, err := metricsclient.NewForConfig(restCfg)
	if err != nil { return }
	clusters[contextName] = &clusterClients{k8s: k8s, metrics: mc}
	log.Printf("Cluster adicionado em memória: %s", contextName)
}

// ── Admin: handlers ───────────────────────────────────────────────────────────

func handleAdminValidate(w http.ResponseWriter, r *http.Request) {
	var req AdminValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), 400); return }
	w.Header().Set("Content-Type", "application/json")
	cfg, err := clientcmd.Load([]byte(req.Kubeconfig))
	if err != nil { json.NewEncoder(w).Encode(AdminValidateResponse{Error: "kubeconfig invalido: " + err.Error()}); return }
	contexts := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts { contexts = append(contexts, name) }
	sort.Strings(contexts)
	json.NewEncoder(w).Encode(AdminValidateResponse{Contexts: contexts})
}

func handleAdminCreateSA(w http.ResponseWriter, r *http.Request) {
	var req AdminCreateSARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), 400); return }
	w.Header().Set("Content-Type", "application/json")
	if req.Namespace == "" { req.Namespace = "default" }
	if req.Context == "" { json.NewEncoder(w).Encode(AdminCreateSAResponse{Error: "contexto nao informado"}); return }
	targetK8s, err := clientFromKubeconfig(req.Kubeconfig, req.Context)
	if err != nil { json.NewEncoder(w).Encode(AdminCreateSAResponse{Error: err.Error()}); return }
	ctx := context.Background()
	sa := "pod-monitor"
	_, err = targetK8s.CoreV1().ServiceAccounts(req.Namespace).Create(ctx,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: sa, Namespace: req.Namespace}}, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		json.NewEncoder(w).Encode(AdminCreateSAResponse{Error: "erro ao criar ServiceAccount: " + err.Error()}); return
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-monitor-role"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods", "namespaces", "nodes", "persistentvolumeclaims", "services", "endpoints", "configmaps", "secrets", "serviceaccounts", "resourcequotas", "limitranges"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{""}, Resources: []string{"pods/log"}, Verbs: []string{"get"}},
			{APIGroups: []string{"apps"}, Resources: []string{"deployments", "replicasets", "statefulsets", "daemonsets"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{"networking.k8s.io"}, Resources: []string{"ingresses"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{"metrics.k8s.io"}, Resources: []string{"pods", "nodes"}, Verbs: []string{"get", "list"}},
		},
	}
	existing, crErr := targetK8s.RbacV1().ClusterRoles().Get(ctx, "pod-monitor-role", metav1.GetOptions{})
	if k8serrors.IsNotFound(crErr) {
		_, err = targetK8s.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
	} else if crErr == nil {
		existing.Rules = cr.Rules
		_, err = targetK8s.RbacV1().ClusterRoles().Update(ctx, existing, metav1.UpdateOptions{})
	}
	if err != nil { log.Printf("Aviso ClusterRole: %v", err) }
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-monitor-binding"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: sa, Namespace: req.Namespace}},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "pod-monitor-role"},
	}
	_, err = targetK8s.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		json.NewEncoder(w).Encode(AdminCreateSAResponse{Error: "erro ao criar ClusterRoleBinding: " + err.Error()}); return
	}
	_, err = targetK8s.CoreV1().Secrets(req.Namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: sa + "-token", Namespace: req.Namespace,
			Annotations: map[string]string{"kubernetes.io/service-account.name": sa}},
		Type: corev1.SecretTypeServiceAccountToken,
	}, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		json.NewEncoder(w).Encode(AdminCreateSAResponse{Error: "erro ao criar token secret: " + err.Error()}); return
	}
	var token string; var caData []byte
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		sec, err := targetK8s.CoreV1().Secrets(req.Namespace).Get(ctx, sa+"-token", metav1.GetOptions{})
		if err == nil && len(sec.Data["token"]) > 0 { token = string(sec.Data["token"]); caData = sec.Data["ca.crt"]; break }
	}
	if token == "" { json.NewEncoder(w).Encode(AdminCreateSAResponse{Error: "timeout aguardando token do ServiceAccount"}); return }
	rawCfg, _ := clientcmd.Load([]byte(req.Kubeconfig))
	serverURL := ""
	if ctxObj, ok := rawCfg.Contexts[req.Context]; ok {
		if cl, ok := rawCfg.Clusters[ctxObj.Cluster]; ok {
			serverURL = cl.Server
			if len(caData) == 0 { caData = cl.CertificateAuthorityData }
		}
	}
	saConfig := clientcmdapi.NewConfig()
	saConfig.Clusters[req.Context]  = &clientcmdapi.Cluster{Server: serverURL, CertificateAuthorityData: caData}
	saConfig.AuthInfos[req.Context] = &clientcmdapi.AuthInfo{Token: token}
	saConfig.Contexts[req.Context]  = &clientcmdapi.Context{Cluster: req.Context, AuthInfo: req.Context, Namespace: req.Namespace}
	saConfig.CurrentContext          = req.Context
	out, err := clientcmd.Write(*saConfig)
	if err != nil { json.NewEncoder(w).Encode(AdminCreateSAResponse{Error: "erro ao serializar: " + err.Error()}); return }
	json.NewEncoder(w).Encode(AdminCreateSAResponse{
		Kubeconfig: string(out),
		Message:    fmt.Sprintf("ServiceAccount '%s' criado no namespace '%s' com sucesso.", sa, req.Namespace),
	})
}

func handleAdminApply(w http.ResponseWriter, r *http.Request) {
	var req AdminApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, err.Error(), 400); return }
	w.Header().Set("Content-Type", "application/json")
	merged, err := mergeKubeconfigs(req.Kubeconfig)
	if err != nil { json.NewEncoder(w).Encode(AdminApplyResponse{Error: err.Error()}); return }
	mergedBytes, err := clientcmd.Write(*merged)
	if err != nil { json.NewEncoder(w).Encode(AdminApplyResponse{Error: "erro ao serializar: " + err.Error()}); return }

	if localK8sClient == nil {
		// Modo standalone (docker-compose): persiste no arquivo KUBECONFIG local
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" { kubeconfigPath = "/data/kubeconfig" }
		if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0700); err != nil {
			json.NewEncoder(w).Encode(AdminApplyResponse{Error: "erro ao criar diretorio: " + err.Error()}); return
		}
		if err := os.WriteFile(kubeconfigPath, mergedBytes, 0600); err != nil {
			json.NewEncoder(w).Encode(AdminApplyResponse{Error: "erro ao salvar kubeconfig: " + err.Error()}); return
		}
		for ctxName := range merged.Contexts {
			if _, exists := clusters[ctxName]; !exists { reloadCluster(string(mergedBytes), ctxName) }
		}
		ctxNames := make([]string, 0, len(merged.Contexts))
		for name := range merged.Contexts { ctxNames = append(ctxNames, name) }
		sort.Strings(ctxNames)
		json.NewEncoder(w).Encode(AdminApplyResponse{
			Message:  "Kubeconfig salvo. Clusters carregados sem necessidade de reinicio.",
			Clusters: ctxNames,
		})
		return
	}

	ctx := context.Background()
	namespace  := getOwnNamespace()
	secretName := os.Getenv("KUBECONFIG_SECRET_NAME")
	if secretName == "" { secretName = "pod-monitor-kubeconfig" }
	existing, err := localK8sClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = localK8sClient.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			Data: map[string][]byte{"config": mergedBytes},
		}, metav1.CreateOptions{})
	} else if err == nil {
		existing.Data["config"] = mergedBytes
		_, err = localK8sClient.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	}
	if err != nil { json.NewEncoder(w).Encode(AdminApplyResponse{Error: "erro ao atualizar Secret: " + err.Error()}); return }
	deployName := os.Getenv("DEPLOYMENT_NAME")
	if deployName == "" { deployName = "backend" }
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().UTC().Format(time.RFC3339))
	localK8sClient.AppsV1().Deployments(namespace).Patch(ctx, deployName, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	for ctxName := range merged.Contexts {
		if _, exists := clusters[ctxName]; !exists { reloadCluster(string(mergedBytes), ctxName) }
	}
	ctxNames := make([]string, 0, len(merged.Contexts))
	for name := range merged.Contexts { ctxNames = append(ctxNames, name) }
	sort.Strings(ctxNames)
	json.NewEncoder(w).Encode(AdminApplyResponse{
		Message:  "Kubeconfig atualizado. O backend sera reiniciado em instantes.",
		Clusters: ctxNames,
	})
}

// ── Remoção de cluster ────────────────────────────────────────────────────────

func handleAdminDeleteCluster(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", 405)
		return
	}

	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	if body.Name == "" {
		http.Error(w, "name obrigatório", 400)
		return
	}

	// verifica senha do usuário autenticado (deve ser admin)
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	usersMu.RLock()
	u, ok := usersMap[claims.Username]
	usersMu.RUnlock()
	if !ok || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(body.Password)) != nil {
		http.Error(w, "senha incorreta", 403)
		return
	}

	_, exists := clusters[body.Name]
	if !exists {
		http.Error(w, fmt.Sprintf("cluster %q não encontrado", body.Name), 404)
		return
	}
	delete(clusters, body.Name)

	// Sem isso, o cluster removido volta a ser registrado no próximo restart
	// do pod, pois initClients() recarrega todos os contextos persistidos.
	if err := removeClusterFromKubeconfig(body.Name); err != nil {
		log.Printf("Aviso: não foi possível remover %q do kubeconfig persistido: %v", body.Name, err)
	}

	log.Printf("Cluster %q removido por %s", body.Name, claims.Username)
	auditLog(claims.Username, claims.Role, "cluster_delete", body.Name, r.RemoteAddr)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  fmt.Sprintf("Cluster %q removido com sucesso", body.Name),
		"clusters": clusterNames(),
	})
}

// ── Docker host admin ─────────────────────────────────────────────────────────

func buildDockerHostsValue() string {
	parts := make([]string, 0, len(dockerSockets))
	for name, addr := range dockerSockets {
		parts = append(parts, name+"="+addr)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func updateDockerHostsEnv() error {
	if localK8sClient == nil {
		return fmt.Errorf("in-cluster client não disponível")
	}
	ctx := context.Background()
	namespace := getOwnNamespace()
	deployName := os.Getenv("DEPLOYMENT_NAME")
	if deployName == "" {
		deployName = "backend"
	}
	newValue := buildDockerHostsValue()
	patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"backend","env":[{"name":"DOCKER_HOSTS","value":%q}]}]}}}}`, newValue)
	_, err := localK8sClient.AppsV1().Deployments(namespace).Patch(
		ctx, deployName, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func handleAdminDockerHost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400); return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Address = strings.TrimSpace(req.Address)
		if req.Name == "" || req.Address == "" {
			http.Error(w, "nome e endereço são obrigatórios", 400); return
		}
		dockerSockets[req.Name] = req.Address
		go func() {
			client, baseURL := newDockerClient(req.Address)
			apiPath := dockerAPIPath(req.Name, client, baseURL)
			raw, err := listDockerContainers(client, baseURL, apiPath)
			isCluster := false
			if err == nil {
				for _, c := range raw {
					if isK8sContainer(c.Labels) { isCluster = true; break }
				}
			}
			dockerIsClusterMu.Lock()
			dockerIsCluster[req.Name] = isCluster
			dockerIsClusterMu.Unlock()
			log.Printf("Host Docker adicionado: %s -> %s (cluster=%v)", req.Name, req.Address, isCluster)
		}()
		if err := updateDockerHostsEnv(); err != nil {
			log.Printf("aviso: host adicionado em memória mas falhou ao persistir: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "host adicionado"})

	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "parâmetro name obrigatório", 400); return
		}
		delete(dockerSockets, name)
		dockerIsClusterMu.Lock()
		delete(dockerIsCluster, name)
		dockerIsClusterMu.Unlock()
		if err := updateDockerHostsEnv(); err != nil {
			log.Printf("aviso: host removido da memória mas falhou ao persistir: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "host removido"})

	default:
		http.Error(w, "method not allowed", 405)
	}
}

// ── Handlers de monitoramento ─────────────────────────────────────────────────

func handleClusters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, _ := r.Context().Value(claimsCtxKey).(*TokenClaims)
	all := clusterNames()
	if claims != nil && claims.Role == RoleDev {
		filtered := []string{}
		for _, c := range all {
			if devAllowedCluster(claims, c) { filtered = append(filtered, c) }
		}
		json.NewEncoder(w).Encode(filtered)
		return
	}
	json.NewEncoder(w).Encode(all)
}

func handleNamespaces(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(claimsCtxKey).(*TokenClaims)
	clients, clusterName, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	if claims != nil && !devAllowedCluster(claims, clusterName) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]string{})
		return
	}
	ctx := context.Background()
	nsList, err := clients.k8s.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil { http.Error(w, err.Error(), 500); return }
	names := []string{}
	for _, ns := range nsList.Items {
		if claims == nil || devAllowedNamespace(claims, ns.Name) {
			names = append(names, ns.Name)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

func handleResources(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(claimsCtxKey).(*TokenClaims)
	clients, clusterName, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	if claims != nil && !devAllowedCluster(claims, clusterName) {
		http.Error(w, "Forbidden", 403); return
	}
	ns := r.URL.Query().Get("namespace")
	if claims != nil && ns != "" && !devAllowedNamespace(claims, ns) {
		http.Error(w, "Forbidden", 403); return
	}
	data, err := getPodsResources(clients, ns)
	if err != nil { http.Error(w, err.Error(), 500); return }
	w.Header().Set("Content-Type", "application/json")
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		page, _ := strconv.Atoi(pageStr)
		if page < 1 { page = 1 }
		pageSize := 100
		if psStr := r.URL.Query().Get("page_size"); psStr != "" {
			if ps, err2 := strconv.Atoi(psStr); err2 == nil && ps > 0 && ps <= 500 {
				pageSize = ps
			}
		}
		json.NewEncoder(w).Encode(paginate(data, page, pageSize))
	} else {
		json.NewEncoder(w).Encode(data)
	}
	if claims == nil || claims.Role != RoleDev {
		go saveSnapshot(newSessionID(), clusterName, data)
	}
}

func handleNodes(w http.ResponseWriter, r *http.Request) {
	clients, _, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	data, err := getNodesResources(clients)
	if err != nil { http.Error(w, err.Error(), 500); return }
	w.Header().Set("Content-Type", "application/json")
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		page, _ := strconv.Atoi(pageStr)
		if page < 1 { page = 1 }
		pageSize := 100
		if psStr := r.URL.Query().Get("page_size"); psStr != "" {
			if ps, err2 := strconv.Atoi(psStr); err2 == nil && ps > 0 && ps <= 500 {
				pageSize = ps
			}
		}
		json.NewEncoder(w).Encode(paginate(data, page, pageSize))
	} else {
		json.NewEncoder(w).Encode(data)
	}
}

func handleStorage(w http.ResponseWriter, r *http.Request) {
	clients, _, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	ctx := context.Background()
	pvcList, err := clients.k8s.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil { http.Error(w, err.Error(), 500); return }
	result := []PVCInfo{}
	for _, pvc := range pvcList.Items {
		cap := ""
		if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			cap = q.String()
		}
		modes := []string{}
		for _, m := range pvc.Spec.AccessModes {
			modes = append(modes, string(m))
		}
		sc := ""
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}
		result = append(result, PVCInfo{
			Namespace:    pvc.Namespace,
			Name:         pvc.Name,
			Capacity:     cap,
			Status:       string(pvc.Status.Phase),
			StorageClass: sc,
			Volume:       pvc.Spec.VolumeName,
			AccessModes:  strings.Join(modes, ", "),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace { return result[i].Namespace < result[j].Namespace }
		return result[i].Name < result[j].Name
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func resourceAge(t time.Time) string {
	d := time.Since(t)
	if d.Hours() >= 24*365 { return fmt.Sprintf("%dy", int(d.Hours()/(24*365))) }
	if d.Hours() >= 24     { return fmt.Sprintf("%dd", int(d.Hours()/24)) }
	if d.Hours() >= 1      { return fmt.Sprintf("%dh", int(d.Hours())) }
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

var defaultExcludedNS = map[string]bool{
	"ingress-nginx":   true,
	"cert-manager":    true,
	"monitoring":      true,
	"prometheus":      true,
	"grafana":         true,
	"logging":         true,
	"elastic-system":  true,
	"istio-system":    true,
	"linkerd":         true,
	"velero":          true,
	"argocd":          true,
	"flux-system":     true,
}

func handleOrphans(w http.ResponseWriter, r *http.Request) {
	clients, _, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	ctx := context.Background()

	// Build excluded namespace set
	excluded := map[string]bool{}
	for k, v := range defaultExcludedNS { excluded[k] = v }
	if extra := r.URL.Query().Get("exclude"); extra != "" {
		for _, ns := range strings.Split(extra, ",") {
			if ns = strings.TrimSpace(ns); ns != "" { excluded[ns] = true }
		}
	}
	isExcluded := func(ns string) bool {
		return strings.HasPrefix(ns, "kube-") || excluded[ns]
	}

	// Collect all pod references
	podList, err := clients.k8s.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil { http.Error(w, err.Error(), 500); return }

	usedPVCs := map[string]bool{}
	usedCMs  := map[string]bool{}
	usedSecs := map[string]bool{}
	usedSAs  := map[string]bool{}

	collectContainerRefs := func(ns string, containers []corev1.Container) {
		for _, c := range containers {
			for _, env := range c.Env {
				if env.ValueFrom == nil { continue }
				if env.ValueFrom.ConfigMapKeyRef != nil { usedCMs[ns+"/"+env.ValueFrom.ConfigMapKeyRef.Name] = true }
				if env.ValueFrom.SecretKeyRef  != nil { usedSecs[ns+"/"+env.ValueFrom.SecretKeyRef.Name]  = true }
			}
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil { usedCMs[ns+"/"+ef.ConfigMapRef.Name] = true }
				if ef.SecretRef    != nil { usedSecs[ns+"/"+ef.SecretRef.Name]    = true }
			}
		}
	}

	for _, pod := range podList.Items {
		ns := pod.Namespace
		sa := pod.Spec.ServiceAccountName
		if sa == "" { sa = "default" }
		usedSAs[ns+"/"+sa] = true
		for _, v := range pod.Spec.Volumes {
			if v.PersistentVolumeClaim != nil { usedPVCs[ns+"/"+v.PersistentVolumeClaim.ClaimName] = true }
			if v.ConfigMap             != nil { usedCMs[ns+"/"+v.ConfigMap.Name]                   = true }
			if v.Secret               != nil { usedSecs[ns+"/"+v.Secret.SecretName]                = true }
		}
		for _, ips := range pod.Spec.ImagePullSecrets { usedSecs[ns+"/"+ips.Name] = true }
		collectContainerRefs(ns, pod.Spec.Containers)
		collectContainerRefs(ns, pod.Spec.InitContainers)
	}

	result := OrphanedResources{
		PVCs: []OrphanPVC{}, Services: []OrphanService{},
		ConfigMaps: []OrphanConfigMap{}, Secrets: []OrphanSecret{},
		Ingresses: []OrphanIngress{}, ServiceAccounts: []OrphanServiceAccount{},
	}

	// Orphaned PVCs
	if pvcList, e := clients.k8s.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{}); e == nil {
		for _, pvc := range pvcList.Items {
			if isExcluded(pvc.Namespace) || usedPVCs[pvc.Namespace+"/"+pvc.Name] { continue }
			cap := ""
			if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok { cap = q.String() }
			sc := ""
			if pvc.Spec.StorageClassName != nil { sc = *pvc.Spec.StorageClassName }
			result.PVCs = append(result.PVCs, OrphanPVC{
				Namespace: pvc.Namespace, Name: pvc.Name,
				Capacity: cap, StorageClass: sc, Age: resourceAge(pvc.CreationTimestamp.Time),
			})
		}
	}

	// Build endpoints map (namespace/svcname -> has ready address)
	epMap := map[string]bool{}
	if epList, e := clients.k8s.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{}); e == nil {
		for _, ep := range epList.Items {
			for _, sub := range ep.Subsets {
				if len(sub.Addresses) > 0 { epMap[ep.Namespace+"/"+ep.Name] = true; break }
			}
		}
	}

	// Orphaned Services (has selector but no endpoints)
	if svcList, e := clients.k8s.CoreV1().Services("").List(ctx, metav1.ListOptions{}); e == nil {
		for _, svc := range svcList.Items {
			if isExcluded(svc.Namespace) { continue }
			if svc.Name == "kubernetes" || len(svc.Spec.Selector) == 0 { continue }
			if epMap[svc.Namespace+"/"+svc.Name] { continue }
			selParts := []string{}
			for k, v := range svc.Spec.Selector { selParts = append(selParts, k+"="+v) }
			sort.Strings(selParts)
			result.Services = append(result.Services, OrphanService{
				Namespace: svc.Namespace, Name: svc.Name,
				Type: string(svc.Spec.Type), Selector: strings.Join(selParts, ", "),
				Age: resourceAge(svc.CreationTimestamp.Time),
			})
		}
	}

	// Orphaned ConfigMaps
	systemCMs := map[string]bool{"kube-root-ca.crt": true}
	if cmList, e := clients.k8s.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{}); e == nil {
		for _, cm := range cmList.Items {
			if isExcluded(cm.Namespace) || systemCMs[cm.Name] { continue }
			if usedCMs[cm.Namespace+"/"+cm.Name] { continue }
			result.ConfigMaps = append(result.ConfigMaps, OrphanConfigMap{
				Namespace: cm.Namespace, Name: cm.Name,
				Age: resourceAge(cm.CreationTimestamp.Time),
			})
		}
	}

	// Orphaned Secrets (skip auto-generated and system)
	if secList, e := clients.k8s.CoreV1().Secrets("").List(ctx, metav1.ListOptions{}); e == nil {
		for _, sec := range secList.Items {
			if isExcluded(sec.Namespace) { continue }
			if sec.Type == corev1.SecretTypeServiceAccountToken { continue }
			if usedSecs[sec.Namespace+"/"+sec.Name] { continue }
			result.Secrets = append(result.Secrets, OrphanSecret{
				Namespace: sec.Namespace, Name: sec.Name,
				SecretType: string(sec.Type), Age: resourceAge(sec.CreationTimestamp.Time),
			})
		}
	}

	// Orphaned Ingresses (all backends have no endpoints)
	if ingList, e := clients.k8s.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{}); e == nil {
		for _, ing := range ingList.Items {
			if isExcluded(ing.Namespace) { continue }
			allDead := true
			hosts, deadSvcs := []string{}, []string{}
			for _, rule := range ing.Spec.Rules {
				if rule.Host != "" { hosts = append(hosts, rule.Host) }
				if rule.HTTP == nil { continue }
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service == nil { continue }
					svcName := path.Backend.Service.Name
					if epMap[ing.Namespace+"/"+svcName] { allDead = false } else {
						deadSvcs = append(deadSvcs, svcName)
					}
				}
			}
			if !allDead { continue }
			result.Ingresses = append(result.Ingresses, OrphanIngress{
				Namespace: ing.Namespace, Name: ing.Name,
				Host: strings.Join(hosts, ", "), Service: strings.Join(deadSvcs, ", "),
				Age: resourceAge(ing.CreationTimestamp.Time),
			})
		}
	}

	// Orphaned ServiceAccounts
	if saList, e := clients.k8s.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{}); e == nil {
		for _, sa := range saList.Items {
			if isExcluded(sa.Namespace) || sa.Name == "default" { continue }
			if usedSAs[sa.Namespace+"/"+sa.Name] { continue }
			result.ServiceAccounts = append(result.ServiceAccounts, OrphanServiceAccount{
				Namespace: sa.Namespace, Name: sa.Name,
				Age: resourceAge(sa.CreationTimestamp.Time),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func queryHistory(clusterName, ns, start, end string) ([]HistoryRecord, error) {
	args := []interface{}{clusterName}
	where := `WHERE cluster=$1 `
	if start != "" {
		args = append(args, start)
		where += fmt.Sprintf(`AND captured_at>=$%d `, len(args))
	} else {
		args = append(args, time.Now().Add(-7*24*time.Hour).UTC().Format(time.RFC3339))
		where += fmt.Sprintf(`AND captured_at>=$%d `, len(args))
	}
	if end != "" {
		args = append(args, end)
		where += fmt.Sprintf(`AND captured_at<$%d `, len(args))
	}
	if ns != "" {
		args = append(args, ns)
		where += fmt.Sprintf(`AND namespace=$%d `, len(args))
	}
	q := `SELECT session_id,captured_at,cluster,namespace,pod,node,container,
		cpu_request,cpu_limit,mem_request,mem_limit,cpu_usage,mem_usage FROM pod_snapshots ` +
		where + `ORDER BY captured_at DESC LIMIT 10000`
	rows, err := db.Query(q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var records []HistoryRecord
	for rows.Next() {
		var rec HistoryRecord
		rows.Scan(&rec.SessionID, &rec.CapturedAt, &rec.Cluster, &rec.Namespace, &rec.Pod, &rec.Node, &rec.Container,
			&rec.CPURequest, &rec.CPULimit, &rec.MemoryRequest, &rec.MemoryLimit, &rec.CPUUsage, &rec.MemoryUsage)
		records = append(records, rec)
	}
	if records == nil { records = []HistoryRecord{} }
	return records, nil
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(claimsCtxKey).(*TokenClaims)
	clients, clusterName, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	if claims != nil && !devAllowedCluster(claims, clusterName) {
		http.Error(w, "Forbidden", 403); return
	}

	ns        := r.URL.Query().Get("namespace")
	pod       := r.URL.Query().Get("pod")
	container := r.URL.Query().Get("container")
	tailStr   := r.URL.Query().Get("tail")

	if ns == "" || pod == "" {
		http.Error(w, "namespace e pod são obrigatórios", 400)
		return
	}
	if claims != nil && !devAllowedNamespace(claims, ns) {
		http.Error(w, "Forbidden", 403); return
	}

	tail := int64(200)
	if tailStr != "" {
		if v, err := fmt.Sscanf(tailStr, "%d", &tail); v == 0 || err != nil {
			tail = 200
		}
	}

	opts := &corev1.PodLogOptions{TailLines: &tail}
	if container != "" {
		opts.Container = container
	}

	ctx := context.Background()
	req := clients.k8s.CoreV1().Pods(ns).GetLogs(pod, opts)
	logs, err := req.DoRaw(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("erro ao obter logs: %v", err), 500)
		return
	}

	if claims != nil {
		auditLog(claims.Username, claims.Role, "pod_logs",
			fmt.Sprintf("cluster=%s ns=%s pod=%s container=%s", clusterName, ns, pod, container),
			r.RemoteAddr)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(logs)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if db == nil { json.NewEncoder(w).Encode([]HistoryRecord{}); return }
	_, clusterName, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	q := r.URL.Query()
	records, err := queryHistory(clusterName, q.Get("namespace"), q.Get("start"), q.Get("end"))
	if err != nil { http.Error(w, err.Error(), 500); return }
	json.NewEncoder(w).Encode(records)
}

func handleHistoryCSV(w http.ResponseWriter, r *http.Request) {
	if db == nil { http.Error(w, "histórico não habilitado", 503); return }
	_, clusterName, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	q := r.URL.Query()
	records, err := queryHistory(clusterName, q.Get("namespace"), q.Get("start"), q.Get("end"))
	if err != nil { http.Error(w, err.Error(), 500); return }

	start, end := q.Get("start"), q.Get("end")
	fname := "historico"
	if start != "" { fname += "_" + strings.ReplaceAll(start[:10], "-", "") }
	if end   != "" { fname += "_" + strings.ReplaceAll(end[:10],   "-", "") }
	fname += ".csv"

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`"`)

	fmt.Fprintln(w, "captured_at,cluster,namespace,pod,node,container,cpu_request,cpu_limit,cpu_usage,mem_request,mem_limit,mem_usage")
	for _, rec := range records {
		fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
			rec.CapturedAt, rec.Cluster, rec.Namespace, rec.Pod, rec.Node, rec.Container,
			rec.CPURequest, rec.CPULimit, rec.CPUUsage,
			rec.MemoryRequest, rec.MemoryLimit, rec.MemoryUsage)
	}
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

type DashWidgetContainer struct {
	Name      string `json:"name"`
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	CPUUsage  string `json:"cpu_usage,omitempty"`
	CPULimit  string `json:"cpu_limit,omitempty"`
	MemUsage  string `json:"mem_usage,omitempty"`
	MemLimit  string `json:"mem_limit,omitempty"`
	Pct       int    `json:"pct"`
}

type DashSummary struct {
	Pods struct {
		Statuses   map[string]int      `json:"statuses"`
		StatusPods map[string][]string `json:"status_pods"`
	} `json:"pods"`
	Nodes struct {
		Ready         int      `json:"ready"`
		NotReady      int      `json:"not_ready"`
		Total         int      `json:"total"`
		NotReadyNames []string `json:"not_ready_names"`
	} `json:"nodes"`
	TopCPU   []DashWidgetContainer `json:"top_cpu"`
	TopMem   []DashWidgetContainer `json:"top_mem"`
	Alerts   struct {
		Critical int `json:"critical"`
		Warning  int `json:"warning"`
	} `json:"alerts"`
	AlertPods []struct {
		Pod       string `json:"pod"`
		Namespace string `json:"namespace"`
		Container string `json:"container"`
		Reason    string `json:"reason"`
		Severity  string `json:"severity"` // "critical" | "warning"
	} `json:"alert_pods"`
	Helm struct {
		Deployed int `json:"deployed"`
		Failed   int `json:"failed"`
		Total    int `json:"total"`
	} `json:"helm"`
	Docker struct {
		Hosts   int `json:"hosts"`
		Running int `json:"running"`
	} `json:"docker"`
}

func parseCPUm(s string) float64 {
	if s == "" || s == "0" { return 0 }
	q, err := resource.ParseQuantity(s)
	if err != nil { return 0 }
	return float64(q.MilliValue())
}

func parseMemMi(s string) float64 {
	if s == "" || s == "0" { return 0 }
	q, err := resource.ParseQuantity(s)
	if err != nil { return 0 }
	return float64(q.Value()) / (1024 * 1024)
}

func handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)

	clusterName := r.URL.Query().Get("cluster")
	if clusterName == "" {
		names := clusterNames()
		if len(names) == 0 {
			http.Error(w, "nenhum cluster disponível", 400)
			return
		}
		clusterName = names[0]
	}
	if !devAllowedCluster(claims, clusterName) {
		http.Error(w, "Forbidden", 403)
		return
	}
	clients, ok := clusters[clusterName]
	if !ok {
		http.Error(w, fmt.Sprintf("cluster %q não encontrado", clusterName), 400)
		return
	}

	ctx := context.Background()
	var summary DashSummary

	// Pods
	pods, err := getPodsResources(clients, "")
	if err == nil {
		// persiste snapshot para alimentar o gráfico de série temporal
		go saveSnapshot(newSessionID(), clusterName, pods)

		summary.Pods.Statuses   = make(map[string]int)
		summary.Pods.StatusPods = make(map[string][]string)
		for _, pod := range pods {
			status := pod.Phase
			if status == "" {
				status = "Unknown"
			}
			summary.Pods.Statuses[status]++
			summary.Pods.StatusPods[status] = append(summary.Pods.StatusPods[status], pod.Namespace+"/"+pod.Name)
			for _, c := range pod.Containers {
				cpuUsageM := parseCPUm(c.CPUUsage)
				cpuLimitM := parseCPUm(c.CPULimit)
				memUsageMi := parseMemMi(c.MemoryUsage)
				memLimitMi := parseMemMi(c.MemoryLimit)

				// calcula percentuais (só quando há limite definido)
				cpuPct := 0.0
				if cpuLimitM > 0 { cpuPct = cpuUsageM / cpuLimitM * 100 }
				memPct := 0.0
				if memLimitMi > 0 { memPct = memUsageMi / memLimitMi * 100 }

				maxPct := cpuPct
				if memPct > maxPct { maxPct = memPct }

				warnPct, critPct := getThreshold(clusterName, pod.Namespace)
				warnF, critF := float64(warnPct), float64(critPct)

				severity := ""
				if maxPct >= critF {
					severity = "critical"
					summary.Alerts.Critical++
				} else if maxPct >= warnF {
					severity = "warning"
					summary.Alerts.Warning++
				}

				if severity != "" {
					reasons := []string{}
					if cpuPct >= warnF { reasons = append(reasons, fmt.Sprintf("CPU %.0f%%", cpuPct)) }
					if memPct >= warnF { reasons = append(reasons, fmt.Sprintf("Mem %.0f%%", memPct)) }
					reason := strings.Join(reasons, " + ")
					summary.AlertPods = append(summary.AlertPods, struct {
						Pod       string `json:"pod"`
						Namespace string `json:"namespace"`
						Container string `json:"container"`
						Reason    string `json:"reason"`
						Severity  string `json:"severity"`
					}{Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name, Reason: reason, Severity: severity})
				}
				if cpuLimitM > 0 {
					pct := int(math.Round(cpuUsageM / cpuLimitM * 100))
					summary.TopCPU = append(summary.TopCPU, DashWidgetContainer{
						Name: c.Name, Pod: pod.Name, Namespace: pod.Namespace,
						CPUUsage: c.CPUUsage, CPULimit: c.CPULimit, Pct: pct,
					})
				}
				if memLimitMi > 0 {
					pct := int(math.Round(memUsageMi / memLimitMi * 100))
					summary.TopMem = append(summary.TopMem, DashWidgetContainer{
						Name: c.Name, Pod: pod.Name, Namespace: pod.Namespace,
						MemUsage: c.MemoryUsage, MemLimit: c.MemoryLimit, Pct: pct,
					})
				}
			}
		}
		sort.Slice(summary.TopCPU, func(i, j int) bool { return summary.TopCPU[i].Pct > summary.TopCPU[j].Pct })
		sort.Slice(summary.TopMem, func(i, j int) bool { return summary.TopMem[i].Pct > summary.TopMem[j].Pct })
		if len(summary.TopCPU) > 5 { summary.TopCPU = summary.TopCPU[:5] }
		if len(summary.TopMem) > 5 { summary.TopMem = summary.TopMem[:5] }
	}

	// Nodes
	nodes, err := clients.k8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, node := range nodes.Items {
			summary.Nodes.Total++
			ready := false
			for _, cond := range node.Status.Conditions {
				if cond.Type == "Ready" && cond.Status == "True" {
					ready = true
					break
				}
			}
			if ready {
				summary.Nodes.Ready++
			} else {
				summary.Nodes.NotReady++
				summary.Nodes.NotReadyNames = append(summary.Nodes.NotReadyNames, node.Name)
			}
		}
	}

	// Helm
	secrets, err := clients.k8s.CoreV1().Secrets("").List(ctx, metav1.ListOptions{LabelSelector: "owner=helm,status=deployed"})
	if err == nil {
		type releaseKey struct{ name, ns string }
		latest := map[releaseKey]*corev1.Secret{}
		for i, sec := range secrets.Items {
			if sec.Labels["status"] == "superseded" { continue }
			key := releaseKey{sec.Labels["name"], sec.Namespace}
			existing, ok := latest[key]
			if !ok { latest[key] = &secrets.Items[i]; continue }
			var ev, nv int
			fmt.Sscanf(existing.Labels["version"], "%d", &ev)
			fmt.Sscanf(sec.Labels["version"], "%d", &nv)
			if nv > ev { latest[key] = &secrets.Items[i] }
		}
		for _, sec := range latest {
			summary.Helm.Total++
			switch sec.Labels["status"] {
			case "deployed":
				summary.Helm.Deployed++
			case "failed":
				summary.Helm.Failed++
			}
		}
	}

	// Docker
	summary.Docker.Hosts = len(dockerSockets)
	for name, addr := range dockerSockets {
		client, baseURL := newDockerClient(addr)
		apiPath := dockerAPIPath(name, client, baseURL)
		containers, err := listDockerContainers(client, baseURL, apiPath)
		if err == nil {
			for _, c := range containers {
				if c.State == "running" {
					summary.Docker.Running++
				}
			}
		}
	}

	// Dispara webhooks para alertas críticos e avisa via SSE
	if summary.Alerts.Critical > 0 {
		go triggerWebhooks("critical", map[string]interface{}{
			"cluster":  clusterName,
			"critical": summary.Alerts.Critical,
			"warning":  summary.Alerts.Warning,
			"pods":     summary.AlertPods,
		})
	} else if summary.Alerts.Warning > 0 {
		go triggerWebhooks("warning", map[string]interface{}{
			"cluster": clusterName,
			"warning": summary.Alerts.Warning,
			"pods":    summary.AlertPods,
		})
	}

	// Publica evento SSE para clientes conectados
	if summaryJSON, err2 := json.Marshal(summary); err2 == nil {
		go ssePublish("summary", string(summaryJSON))
	}

	json.NewEncoder(w).Encode(summary)
}

// ── Timeseries ─────────────────────────────────────────────────────────────────

type TSPoint struct {
	Ts    string  `json:"ts"`
	CPUm  float64 `json:"cpu_m"`
	MemMi float64 `json:"mem_mi"`
}

func handleDashTimeseries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	empty := map[string]interface{}{"points": []TSPoint{}, "pods": []string{}, "pod_cpu": []map[string]interface{}{}, "pod_mem": []map[string]interface{}{}}
	if db == nil {
		json.NewEncoder(w).Encode(empty)
		return
	}
	q := r.URL.Query()
	clusterName := q.Get("cluster")
	if clusterName == "" {
		clusterName = "default"
	}
	hours := 24
	if h := q.Get("hours"); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 && n <= 168 {
			hours = n
		}
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

	rows, err := db.Query(`
		SELECT session_id, captured_at, pod, cpu_usage, mem_usage
		FROM pod_snapshots
		WHERE cluster=$1 AND captured_at>=$2
		ORDER BY captured_at ASC
		LIMIT 50000
	`, clusterName, since)
	if err != nil {
		json.NewEncoder(w).Encode(empty)
		return
	}
	defer rows.Close()

	type podVal struct{ cpuM, memMi float64 }
	sessionTs   := map[string]string{}
	sessionOrder := []string{}
	// sid → pod → accumulated values
	sessionPods  := map[string]map[string]*podVal{}
	podTotals    := map[string]float64{} // pod → total cpuM across all sessions

	for rows.Next() {
		var sid, ts, pod, cpuStr, memStr string
		rows.Scan(&sid, &ts, &pod, &cpuStr, &memStr)
		if _, ok := sessionTs[sid]; !ok {
			sessionTs[sid] = ts
			sessionOrder = append(sessionOrder, sid)
			sessionPods[sid] = map[string]*podVal{}
		}
		if sessionPods[sid][pod] == nil {
			sessionPods[sid][pod] = &podVal{}
		}
		cpu := parseCPUm(cpuStr)
		mem := parseMemMi(memStr)
		sessionPods[sid][pod].cpuM  += cpu
		sessionPods[sid][pod].memMi += mem
		podTotals[pod] += cpu
	}

	// Build total points
	totalPoints := make([]TSPoint, 0, len(sessionOrder))
	for _, sid := range sessionOrder {
		pt := TSPoint{Ts: sessionTs[sid]}
		for _, v := range sessionPods[sid] {
			pt.CPUm  += v.cpuM
			pt.MemMi += v.memMi
		}
		pt.CPUm  = math.Round(pt.CPUm)
		pt.MemMi = math.Round(pt.MemMi*10) / 10
		totalPoints = append(totalPoints, pt)
	}

	// Top 20 pods by total CPU
	type podEntry struct{ name string; total float64 }
	podList := make([]podEntry, 0, len(podTotals))
	for name, total := range podTotals {
		podList = append(podList, podEntry{name, total})
	}
	sort.Slice(podList, func(i, j int) bool { return podList[i].total > podList[j].total })
	if len(podList) > 20 {
		podList = podList[:20]
	}
	topPodNames := make([]string, len(podList))
	topPodSet   := map[string]bool{}
	for i, p := range podList {
		topPodNames[i] = p.name
		topPodSet[p.name] = true
	}

	// Build wide-format per-pod points
	podCPUPoints := make([]map[string]interface{}, 0, len(sessionOrder))
	podMemPoints := make([]map[string]interface{}, 0, len(sessionOrder))
	for _, sid := range sessionOrder {
		cpuPt := map[string]interface{}{"ts": sessionTs[sid]}
		memPt := map[string]interface{}{"ts": sessionTs[sid]}
		for podName, v := range sessionPods[sid] {
			if topPodSet[podName] {
				cpuPt[podName] = math.Round(v.cpuM)
				memPt[podName] = math.Round(v.memMi*10) / 10
			}
		}
		podCPUPoints = append(podCPUPoints, cpuPt)
		podMemPoints = append(podMemPoints, memPt)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"points":  totalPoints,
		"pods":    topPodNames,
		"pod_cpu": podCPUPoints,
		"pod_mem": podMemPoints,
	})
}

func handleGetDashboards(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	if db == nil { json.NewEncoder(w).Encode([]DashboardConfig{}); return }
	rows, err := db.Query(`SELECT id, name, username, widgets FROM dashboards WHERE username=$1 ORDER BY id`, claims.Username)
	if err != nil { json.NewEncoder(w).Encode([]DashboardConfig{}); return }
	defer rows.Close()
	list := []DashboardConfig{}
	for rows.Next() {
		var d DashboardConfig
		rows.Scan(&d.ID, &d.Name, &d.Username, &d.Widgets)
		list = append(list, d)
	}
	json.NewEncoder(w).Encode(list)
}

func handleSaveDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	var body DashboardConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, err.Error(), 400); return }
	if db == nil { http.Error(w, "db not configured", 500); return }
	if body.ID == 0 {
		res, err := db.Exec(`INSERT INTO dashboards (username, name, widgets) VALUES ($1,$2,$3)`, claims.Username, body.Name, body.Widgets)
		if err != nil { http.Error(w, err.Error(), 500); return }
		id, _ := res.LastInsertId()
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
	} else {
		db.Exec(`UPDATE dashboards SET name=$1, widgets=$2 WHERE id=$3 AND username=$4`, body.Name, body.Widgets, body.ID, claims.Username)
		json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	}
}

func handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	idStr := r.URL.Query().Get("id")
	if db != nil { db.Exec(`DELETE FROM dashboards WHERE id=$1 AND username=$2`, idStr, claims.Username) }
	json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
}

// ── Análise de boas práticas ──────────────────────────────────────────────────

type AnalysisFinding struct {
	Severity       string `json:"severity"`        // "critical" | "warning" | "info"
	Category       string `json:"category"`        // "security" | "reliability" | "resources" | "best-practices"
	ResourceKind   string `json:"resource_kind"`
	ResourceName   string `json:"resource_name"`
	Namespace      string `json:"namespace"`
	Container      string `json:"container,omitempty"`
	Message        string `json:"message"`
	Recommendation string `json:"recommendation"`
}

type AnalysisSummary struct {
	Critical           int   `json:"critical"`
	Warning            int   `json:"warning"`
	Info               int   `json:"info"`
	ScannedNamespaces  int   `json:"scanned_namespaces"`
	ScannedPods        int   `json:"scanned_pods"`
	ScannedDeployments int   `json:"scanned_deployments"`
	ScannedNodes       int   `json:"scanned_nodes"`
	DurationMs         int64 `json:"duration_ms"`
}

type AnalysisResult struct {
	Findings []AnalysisFinding `json:"findings"`
	Summary  AnalysisSummary   `json:"summary"`
	Nodes    []string          `json:"nodes"`
}

func analysisImageIsLatest(image string) bool {
	if strings.Contains(image, "@sha256:") {
		return false // pinned by digest — seguro
	}
	name := image
	if idx := strings.LastIndex(image, "/"); idx >= 0 {
		name = image[idx+1:]
	}
	idx := strings.LastIndex(name, ":")
	if idx < 0 {
		return true // sem tag = latest implícito
	}
	tag := name[idx+1:]
	return tag == "latest" || tag == ""
}

func handleAnalysis(w http.ResponseWriter, r *http.Request) {
	clients, _, err := getCluster(r)
	if err != nil { http.Error(w, err.Error(), 400); return }
	ns := r.URL.Query().Get("namespace")
	ctx := context.Background()
	start := time.Now()

	// ── Coleta paralela dos recursos ──────────────────────────────────────────
	type collected struct {
		pods         []corev1.Pod
		namespaces   []corev1.Namespace
		nodes        []corev1.Node
		quotas       map[string]bool  // namespaces que têm ResourceQuota
		limitRanges  map[string]bool  // namespaces que têm LimitRange
		depReplicas  map[string]int32 // "ns/name" → replicas desejadas (Deployment)
		ssReplicas   map[string]int32 // "ns/name" → replicas desejadas (StatefulSet)
		totalDeps    int
	}
	col := collected{
		quotas:      make(map[string]bool),
		limitRanges: make(map[string]bool),
		depReplicas: make(map[string]int32),
		ssReplicas:  make(map[string]int32),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	collectErr := make([]string, 0)

	addErr := func(msg string) { mu.Lock(); collectErr = append(collectErr, msg); mu.Unlock() }

	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := clients.k8s.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil { addErr(fmt.Sprintf("pods: %v", err)); return }
		mu.Lock(); col.pods = list.Items; mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := clients.k8s.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil { addErr(fmt.Sprintf("namespaces: %v", err)); return }
		mu.Lock(); col.namespaces = list.Items; mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := clients.k8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil { return }
		mu.Lock(); col.nodes = list.Items; mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := clients.k8s.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
		if err != nil { return }
		mu.Lock()
		for _, q := range list.Items { col.quotas[q.Namespace] = true }
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := clients.k8s.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
		if err != nil { return }
		mu.Lock()
		for _, lr := range list.Items { col.limitRanges[lr.Namespace] = true }
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := clients.k8s.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
		if err != nil { return }
		mu.Lock()
		col.totalDeps = len(list.Items)
		for _, d := range list.Items {
			replicas := int32(1)
			if d.Spec.Replicas != nil { replicas = *d.Spec.Replicas }
			col.depReplicas[d.Namespace+"/"+d.Name] = replicas
		}
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := clients.k8s.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
		if err != nil { return }
		mu.Lock()
		for _, ss := range list.Items {
			replicas := int32(1)
			if ss.Spec.Replicas != nil { replicas = *ss.Spec.Replicas }
			col.ssReplicas[ss.Namespace+"/"+ss.Name] = replicas
		}
		mu.Unlock()
	}()

	wg.Wait()

	// ── Análise ───────────────────────────────────────────────────────────────
	//
	// As regras abaixo são baseadas em documentação oficial e estável do Kubernetes.
	// Cada bloco indica a fonte de referência para facilitar revisões futuras.
	//
	// Fontes principais:
	//   [K8s-CONFIG]   https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	//   [K8s-PROBES]   https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//   [K8s-SECURITY] https://kubernetes.io/docs/concepts/security/pod-security-standards/
	//   [K8s-HA]       https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#replicas
	//   [K8s-QUOTA]    https://kubernetes.io/docs/concepts/policy/resource-quotas/
	//   [K8s-LIMIT]    https://kubernetes.io/docs/concepts/policy/limit-range/
	//   [CIS-K8S]      CIS Kubernetes Benchmark v1.9 (Center for Internet Security)
	//                  Seções 5.1 (RBAC), 5.2 (Pod Security), 5.7 (Namespaces)

	findings := []AnalysisFinding{}
	add := func(f AnalysisFinding) { findings = append(findings, f) }

	// ── 1. Checks por container ───────────────────────────────────────────────
	for _, pod := range col.pods {
		podNS := pod.Namespace
		if ns != "" && podNS != ns { continue }

		// Restarts elevados
		// Não há threshold oficial; valores práticos amplamente adotados pela comunidade.
		// Referência: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-restart-policy
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount >= 10 {
				sev := "warning"
				if cs.RestartCount >= 50 { sev = "critical" }
				add(AnalysisFinding{
					Severity: sev, Category: "reliability",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: cs.Name,
					Message:        fmt.Sprintf("Container reiniciou %d vez(es)", cs.RestartCount),
					Recommendation: "Verifique os logs do container para identificar a causa dos crashes. Considere adicionar liveness probe e ajustar os recursos.",
				})
			}
		}

		// Pod em estado problemático
		// Referência: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase
		phase := string(pod.Status.Phase)
		if phase == "Failed" || phase == "Unknown" {
			add(AnalysisFinding{
				Severity: "critical", Category: "reliability",
				ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS,
				Message:        fmt.Sprintf("Pod em fase %s", phase),
				Recommendation: "Investigue os eventos do pod com `kubectl describe pod` e verifique os logs.",
			})
		}

		// Estados de espera críticos (CrashLoopBackOff, OOMKilled, ImagePullBackOff)
		// Referência: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#container-states
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				reason := cs.State.Waiting.Reason
				if reason == "CrashLoopBackOff" || reason == "OOMKilled" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					add(AnalysisFinding{
						Severity: "critical", Category: "reliability",
						ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: cs.Name,
						Message:        fmt.Sprintf("Container em estado %s", reason),
						Recommendation: "Verifique os logs do container e os eventos do pod.",
					})
				}
			}
		}

		for _, c := range pod.Spec.Containers {
			// Imagem com tag :latest ou sem tag
			// [K8s-SECURITY] CIS Benchmark 5.5.1: "Ensure that image tags are immutable"
			// Referência: https://kubernetes.io/docs/concepts/containers/images/#image-names
			if analysisImageIsLatest(c.Image) {
				add(AnalysisFinding{
					Severity: "warning", Category: "security",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        fmt.Sprintf("Imagem sem tag fixa: %s", c.Image),
					Recommendation: "Use uma tag de versão específica (ex: nginx:1.25.3) para garantir builds reproduzíveis.",
				})
			}

			// Container privilegiado
			// [CIS-K8S] 5.2.1: "Minimize the admission of privileged containers"
			// [K8s-SECURITY] Restricted profile proíbe; Baseline profile também proíbe.
			// Referência: https://kubernetes.io/docs/concepts/security/pod-security-standards/
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				add(AnalysisFinding{
					Severity: "critical", Category: "security",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        "Container rodando em modo privilegiado",
					Recommendation: "Evite modo privilegiado. Use capabilities específicas (securityContext.capabilities.add) somente quando necessário.",
				})
			}

			// Rodando como root
			// [CIS-K8S] 5.2.6: "Minimize the admission of root containers"
			// [K8s-SECURITY] Pod Security Standards — Restricted: runAsNonRoot obrigatório.
			// Referência: https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
			runAsRoot := false
			if pod.Spec.SecurityContext == nil || pod.Spec.SecurityContext.RunAsNonRoot == nil || !*pod.Spec.SecurityContext.RunAsNonRoot {
				if c.SecurityContext == nil {
					runAsRoot = true
				} else if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
					if c.SecurityContext.RunAsUser == nil || *c.SecurityContext.RunAsUser == 0 {
						runAsRoot = true
					}
				}
			}
			if runAsRoot {
				add(AnalysisFinding{
					Severity: "warning", Category: "security",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        "Container pode estar rodando como root",
					Recommendation: "Defina securityContext.runAsNonRoot: true e um runAsUser não-zero.",
				})
			}

			// Requests e limits de CPU e memória
			// [K8s-CONFIG] Documentação oficial de gerenciamento de recursos.
			// [CIS-K8S] 5.2.10/5.2.11: "Minimize the admission of containers without resource limits"
			// Referência: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
			if c.Resources.Requests.Cpu().IsZero() {
				add(AnalysisFinding{
					Severity: "warning", Category: "resources",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        "Container sem CPU request definido",
					Recommendation: "Defina resources.requests.cpu para que o scheduler possa alocar o pod adequadamente.",
				})
			}
			if c.Resources.Limits.Cpu().IsZero() {
				add(AnalysisFinding{
					Severity: "warning", Category: "resources",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        "Container sem CPU limit definido",
					Recommendation: "Defina resources.limits.cpu para evitar que um container consuma CPU ilimitada e afete outros workloads.",
				})
			}
			if c.Resources.Requests.Memory().IsZero() {
				add(AnalysisFinding{
					Severity: "warning", Category: "resources",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        "Container sem memory request definido",
					Recommendation: "Defina resources.requests.memory para garantir alocação adequada pelo scheduler.",
				})
			}
			if c.Resources.Limits.Memory().IsZero() {
				add(AnalysisFinding{
					Severity: "critical", Category: "resources",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        "Container sem memory limit definido",
					Recommendation: "Defina resources.limits.memory para evitar OOMKill de outros processos no nó.",
				})
			}

			// Liveness probe
			// [K8s-PROBES] Documentação oficial de probes.
			// Sem liveness, o Kubernetes não consegue detectar containers travados e não os reinicia.
			// Referência: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
			if c.LivenessProbe == nil {
				add(AnalysisFinding{
					Severity: "info", Category: "reliability",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        "Container sem liveness probe",
					Recommendation: "Adicione uma liveness probe para que o Kubernetes reinicie automaticamente o container em caso de travamento.",
				})
			}

			// Readiness probe
			// [K8s-PROBES] Sem readiness, o pod recebe tráfego antes de estar pronto,
			// causando erros durante inicialização ou após rollouts.
			// Referência: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
			if c.ReadinessProbe == nil {
				add(AnalysisFinding{
					Severity: "info", Category: "reliability",
					ResourceKind: "Pod", ResourceName: pod.Name, Namespace: podNS, Container: c.Name,
					Message:        "Container sem readiness probe",
					Recommendation: "Adicione uma readiness probe para que o Kubernetes só envie tráfego ao container quando ele estiver pronto.",
				})
			}
		}
	}

	// ── 2. Checks por Deployment (réplica única) ──────────────────────────────
	// [K8s-HA] Deployments com réplica única não toleram falhas de nó.
	// Referência: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#replicas
	for key, replicas := range col.depReplicas {
		parts := strings.SplitN(key, "/", 2)
		depNS, depName := parts[0], parts[1]
		if ns != "" && depNS != ns { continue }
		if replicas == 1 {
			add(AnalysisFinding{
				Severity: "warning", Category: "reliability",
				ResourceKind: "Deployment", ResourceName: depName, Namespace: depNS,
				Message:        "Deployment com réplica única — sem alta disponibilidade",
				Recommendation: "Considere aumentar spec.replicas para pelo menos 2 para tolerar falhas de nó.",
			})
		}
	}

	// ── 3. Checks por StatefulSet (réplica única) ─────────────────────────────
	// [K8s-HA] StatefulSets com réplica única perdem HA mas podem ser intencionais
	// (ex: banco de dados single-node). Severidade info, não warning.
	// Referência: https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/
	for key, replicas := range col.ssReplicas {
		parts := strings.SplitN(key, "/", 2)
		ssNS, ssName := parts[0], parts[1]
		if ns != "" && ssNS != ns { continue }
		if replicas == 1 {
			add(AnalysisFinding{
				Severity: "info", Category: "reliability",
				ResourceKind: "StatefulSet", ResourceName: ssName, Namespace: ssNS,
				Message:        "StatefulSet com réplica única",
				Recommendation: "Se alta disponibilidade for necessária, aumente spec.replicas e configure o storage adequadamente.",
			})
		}
	}

	// ── 4. Checks por Namespace (ResourceQuota / LimitRange) ──────────────────
	// [K8s-QUOTA] ResourceQuota impede que um namespace consuma recursos ilimitados.
	// [K8s-LIMIT] LimitRange define defaults para containers sem requests/limits explícitos.
	// [CIS-K8S] 5.7.1: "Create administrative boundaries between resources using namespaces"
	// Namespaces de sistema são ignorados pois não devem ter quotas de usuário.
	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}
	for _, nsObj := range col.namespaces {
		if systemNS[nsObj.Name] { continue }
		if ns != "" && nsObj.Name != ns { continue }
		if !col.quotas[nsObj.Name] {
			add(AnalysisFinding{
				Severity: "warning", Category: "resources",
				ResourceKind: "Namespace", ResourceName: nsObj.Name, Namespace: nsObj.Name,
				Message:        "Namespace sem ResourceQuota definida",
				Recommendation: "Defina uma ResourceQuota para limitar o consumo total de CPU, memória e objetos no namespace.",
			})
		}
		if !col.limitRanges[nsObj.Name] {
			add(AnalysisFinding{
				Severity: "info", Category: "resources",
				ResourceKind: "Namespace", ResourceName: nsObj.Name, Namespace: nsObj.Name,
				Message:        "Namespace sem LimitRange definido",
				Recommendation: "Defina um LimitRange para garantir valores padrão de requests/limits para containers sem configuração explícita.",
			})
		}
	}

	// ── 5. Checks por Node ────────────────────────────────────────────────────
	//
	// Nodes são recursos de infraestrutura — checks se aplicam sempre (sem filtro de namespace).
	//
	// [K8s-NODE] Condições de node: https://kubernetes.io/docs/concepts/architecture/nodes/#condition
	// [CIS-K8S]  5.3: "Configure Network Policies" — node NotReady pode indicar problema de rede.
	// Checks de pressão (MemoryPressure, DiskPressure, PIDPressure) são condições nativas do kubelet.
	for _, node := range col.nodes {
		nodeName := node.Name

		// Node não está Ready
		// Referência: https://kubernetes.io/docs/concepts/architecture/nodes/#condition
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				if cond.Status != corev1.ConditionTrue {
					add(AnalysisFinding{
						Severity: "critical", Category: "reliability",
						ResourceKind: "Node", ResourceName: nodeName,
						Message:        fmt.Sprintf("Node não está Ready (status: %s)", cond.Status),
						Recommendation: "Verifique o kubelet e os eventos do node com `kubectl describe node`. Pode indicar problema de rede, disco ou recursos esgotados.",
					})
				}
			}
		}

		// Pressão de memória
		// O kubelet sinaliza MemoryPressure quando a memória disponível está abaixo do threshold configurado.
		// Referência: https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue {
				add(AnalysisFinding{
					Severity: "critical", Category: "resources",
					ResourceKind: "Node", ResourceName: nodeName,
					Message:        "Node com pressão de memória (MemoryPressure=True)",
					Recommendation: "O kubelet pode estar evictando pods. Verifique o consumo de memória e considere adicionar nodes ou reduzir workloads.",
				})
			}
		}

		// Pressão de disco
		// Referência: https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue {
				add(AnalysisFinding{
					Severity: "warning", Category: "resources",
					ResourceKind: "Node", ResourceName: nodeName,
					Message:        "Node com pressão de disco (DiskPressure=True)",
					Recommendation: "Limpe imagens não utilizadas com `docker system prune` ou aumente o espaço em disco do node.",
				})
			}
		}

		// Pressão de PID
		// Referência: https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodePIDPressure && cond.Status == corev1.ConditionTrue {
				add(AnalysisFinding{
					Severity: "warning", Category: "resources",
					ResourceKind: "Node", ResourceName: nodeName,
					Message:        "Node com pressão de PID (PIDPressure=True)",
					Recommendation: "O limite de processos do node está próximo do máximo. Verifique processos zumbis ou containers com muitos threads.",
				})
			}
		}

		// Node marcado como unschedulable (cordonado)
		// Referência: https://kubernetes.io/docs/concepts/architecture/nodes/#manual-node-administration
		if node.Spec.Unschedulable {
			add(AnalysisFinding{
				Severity: "warning", Category: "reliability",
				ResourceKind: "Node", ResourceName: nodeName,
				Message:        "Node está cordonado (unschedulable)",
				Recommendation: "Node não receberá novos pods. Se não for intencional, execute `kubectl uncordon` para reabilitá-lo.",
			})
		}
	}

	// ── Ordenação: critical → warning → info, depois por namespace/recurso ────
	severityOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(findings, func(i, j int) bool {
		si, sj := severityOrder[findings[i].Severity], severityOrder[findings[j].Severity]
		if si != sj { return si < sj }
		if findings[i].Namespace != findings[j].Namespace { return findings[i].Namespace < findings[j].Namespace }
		return findings[i].ResourceName < findings[j].ResourceName
	})

	// ── Resumo ────────────────────────────────────────────────────────────────
	summary := AnalysisSummary{
		ScannedNamespaces:  len(col.namespaces),
		ScannedPods:        len(col.pods),
		ScannedDeployments: col.totalDeps,
		ScannedNodes:       len(col.nodes),
		DurationMs:         time.Since(start).Milliseconds(),
	}
	for _, f := range findings {
		switch f.Severity {
		case "critical": summary.Critical++
		case "warning":  summary.Warning++
		case "info":     summary.Info++
		}
	}
	if findings == nil { findings = []AnalysisFinding{} }

	nodeNames := make([]string, 0, len(col.nodes))
	for _, n := range col.nodes { nodeNames = append(nodeNames, n.Name) }
	sort.Strings(nodeNames)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AnalysisResult{Findings: findings, Summary: summary, Nodes: nodeNames})
}

// ── Novos tipos ───────────────────────────────────────────────────────────────

type AlertThreshold struct {
	ID        int64  `json:"id"`
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	WarnPct   int    `json:"warn_pct"`
	CritPct   int    `json:"crit_pct"`
}

type AuditEntry struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	IP        string `json:"ip"`
}

type WebhookConfig struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Events  string `json:"events"`
	Enabled bool   `json:"enabled"`
}

type TopologyNode struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Status    string            `json:"status"`
	Meta      map[string]string `json:"meta,omitempty"`
}

type TopologyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type TopologyGraph struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

type QuotaResource struct {
	Hard string `json:"hard"`
	Used string `json:"used"`
}

type QuotaInfo struct {
	Name      string                    `json:"name"`
	Resources map[string]*QuotaResource `json:"resources"`
}

type LimitRangeInfo struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Default map[string]string `json:"default,omitempty"`
	Min     map[string]string `json:"min,omitempty"`
	Max     map[string]string `json:"max,omitempty"`
}

type NamespaceQuota struct {
	Namespace   string           `json:"namespace"`
	Quotas      []QuotaInfo      `json:"quotas"`
	LimitRanges []LimitRangeInfo `json:"limit_ranges"`
}

// ── SSE Hub ───────────────────────────────────────────────────────────────────

type sseClient struct {
	ch chan []byte
}

var (
	sseClientsMu sync.RWMutex
	sseClients   = map[*sseClient]struct{}{}
)

func ssePublish(event, data string) {
	msg := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
	sseClientsMu.RLock()
	defer sseClientsMu.RUnlock()
	for c := range sseClients {
		select {
		case c.ch <- msg:
		default:
		}
	}
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := &sseClient{ch: make(chan []byte, 8)}
	sseClientsMu.Lock()
	sseClients[client] = struct{}{}
	sseClientsMu.Unlock()
	defer func() {
		sseClientsMu.Lock()
		delete(sseClients, client)
		sseClientsMu.Unlock()
	}()

	// heartbeat inicial
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-client.ch:
			w.Write(msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ── Audit Log ─────────────────────────────────────────────────────────────────

func auditLog(username, role, action, detail, ip string) {
	if db == nil {
		return
	}
	db.Exec(`INSERT INTO audit_log (username, role, action, detail, ip) VALUES ($1,$2,$3,$4,$5)`,
		username, role, action, detail, ip)
	go triggerWebhooks("audit", map[string]interface{}{
		"username": username,
		"role":     role,
		"action":   action,
		"detail":   detail,
		"ip":       ip,
	})
}

func handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]AuditEntry{})
		return
	}
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	rows, err := db.Query(`SELECT id, timestamp, username, role, action, detail, ip FROM audit_log ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	entries := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		rows.Scan(&e.ID, &e.Timestamp, &e.Username, &e.Role, &e.Action, &e.Detail, &e.IP)
		entries = append(entries, e)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

func handleGetWebhooks(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]WebhookConfig{})
		return
	}
	rows, err := db.Query(`SELECT id, name, url, events, enabled FROM webhooks ORDER BY id`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	result := []WebhookConfig{}
	for rows.Next() {
		var wh WebhookConfig
		var enabled int
		rows.Scan(&wh.ID, &wh.Name, &wh.URL, &wh.Events, &enabled)
		wh.Enabled = enabled == 1
		result = append(result, wh)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleSaveWebhook(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		http.Error(w, "banco não configurado", 400)
		return
	}
	var wh WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
		http.Error(w, "payload inválido", 400)
		return
	}
	if wh.URL == "" {
		http.Error(w, "url obrigatória", 400)
		return
	}
	enabled := 0
	if wh.Enabled {
		enabled = 1
	}
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	if wh.ID == 0 {
		res, err := db.Exec(`INSERT INTO webhooks (name, url, events, enabled) VALUES ($1,$2,$3,$4)`,
			wh.Name, wh.URL, wh.Events, enabled)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		wh.ID, _ = res.LastInsertId()
		auditLog(claims.Username, claims.Role, "webhook_create", wh.Name+" "+wh.URL, r.RemoteAddr)
	} else {
		_, err := db.Exec(`UPDATE webhooks SET name=$1, url=$2, events=$3, enabled=$4 WHERE id=$5`,
			wh.Name, wh.URL, wh.Events, enabled, wh.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		auditLog(claims.Username, claims.Role, "webhook_update", wh.Name+" "+wh.URL, r.RemoteAddr)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wh)
}

func handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		http.Error(w, "banco não configurado", 400)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id obrigatório", 400)
		return
	}
	db.Exec(`DELETE FROM webhooks WHERE id=$1`, id)
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	auditLog(claims.Username, claims.Role, "webhook_delete", "id="+id, r.RemoteAddr)
	w.WriteHeader(204)
}

// dispatchWebhook envia o payload para url com até 3 tentativas e backoff exponencial.
// Retentar em: erro de rede, timeout, HTTP 5xx.
// Não retentar em: HTTP 4xx (erro do cliente — URL errada, auth inválida, etc.).
var webhookBackoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}

func dispatchWebhook(url string, payload []byte) {
	client := &http.Client{Timeout: 10 * time.Second}
	for attempt, delay := range webhookBackoff {
		resp, err := client.Post(url, "application/json", strings.NewReader(string(payload)))
		if err == nil {
			code := resp.StatusCode
			resp.Body.Close()
			if code < 500 {
				if code >= 400 {
					log.Printf("webhook %s recusado (HTTP %d) — sem retry", url, code)
				}
				return
			}
			log.Printf("webhook %s falhou (HTTP %d), tentativa %d/3, próxima em %s", url, code, attempt+1, delay)
		} else {
			log.Printf("webhook %s erro: %v, tentativa %d/3, próxima em %s", url, err, attempt+1, delay)
		}
		if attempt < len(webhookBackoff)-1 {
			time.Sleep(delay)
		}
	}
	log.Printf("webhook %s abandonado após 3 tentativas", url)
}

func triggerWebhooks(eventName string, payloadObj interface{}) {
	if db == nil {
		return
	}
	rows, err := db.Query(`SELECT url FROM webhooks WHERE enabled=1 AND (events='*' OR strpos(events,$1)>0)`, eventName)
	if err != nil {
		return
	}
	var urls []string
	for rows.Next() {
		var u string
		rows.Scan(&u)
		urls = append(urls, u)
	}
	rows.Close()
	if len(urls) == 0 {
		return
	}
	payload, err := json.Marshal(map[string]interface{}{
		"event":     eventName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      payloadObj,
	})
	if err != nil {
		return
	}
	for _, u := range urls {
		go dispatchWebhook(u, payload)
	}
}

// ── Alert Thresholds ──────────────────────────────────────────────────────────

func getThreshold(cluster, namespace string) (warnPct, critPct int) {
	warnPct, critPct = 85, 90
	if db == nil {
		return
	}
	// tenta match exato cluster+namespace, depois só cluster, depois default global
	type candidate struct{ cluster, namespace string }
	for _, c := range []candidate{{cluster, namespace}, {cluster, ""}, {"", ""}} {
		var w, cr int
		err := db.QueryRow(`SELECT warn_pct, crit_pct FROM alert_thresholds WHERE cluster=$1 AND namespace=$2 LIMIT 1`,
			c.cluster, c.namespace).Scan(&w, &cr)
		if err == nil {
			return w, cr
		}
	}
	return
}

func handleGetThresholds(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]AlertThreshold{})
		return
	}
	rows, err := db.Query(`SELECT id, cluster, namespace, warn_pct, crit_pct FROM alert_thresholds ORDER BY cluster, namespace`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	result := []AlertThreshold{}
	for rows.Next() {
		var at AlertThreshold
		rows.Scan(&at.ID, &at.Cluster, &at.Namespace, &at.WarnPct, &at.CritPct)
		result = append(result, at)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleSaveThreshold(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		http.Error(w, "banco não configurado", 400)
		return
	}
	var at AlertThreshold
	if err := json.NewDecoder(r.Body).Decode(&at); err != nil {
		http.Error(w, "payload inválido", 400)
		return
	}
	if at.WarnPct <= 0 || at.CritPct <= 0 || at.WarnPct >= at.CritPct {
		http.Error(w, "thresholds inválidos: warn < crit e ambos > 0", 400)
		return
	}
	_, err := db.Exec(`INSERT INTO alert_thresholds (cluster, namespace, warn_pct, crit_pct)
		VALUES ($1,$2,$3,$4) ON CONFLICT(cluster,namespace) DO UPDATE SET warn_pct=EXCLUDED.warn_pct, crit_pct=EXCLUDED.crit_pct`,
		at.Cluster, at.Namespace, at.WarnPct, at.CritPct)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	auditLog(claims.Username, claims.Role, "threshold_save",
		fmt.Sprintf("cluster=%s ns=%s warn=%d crit=%d", at.Cluster, at.Namespace, at.WarnPct, at.CritPct),
		r.RemoteAddr)
	db.QueryRow(`SELECT id FROM alert_thresholds WHERE cluster=$1 AND namespace=$2`, at.Cluster, at.Namespace).Scan(&at.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(at)
}

func handleDeleteThreshold(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		http.Error(w, "banco não configurado", 400)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id obrigatório", 400)
		return
	}
	db.Exec(`DELETE FROM alert_thresholds WHERE id=$1`, id)
	claims := r.Context().Value(claimsCtxKey).(*TokenClaims)
	auditLog(claims.Username, claims.Role, "threshold_delete", "id="+id, r.RemoteAddr)
	w.WriteHeader(204)
}

// ── Topology ──────────────────────────────────────────────────────────────────

func handleTopology(w http.ResponseWriter, r *http.Request) {
	clients, clusterName, err := getCluster(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ns := r.URL.Query().Get("namespace")
	ctx := context.Background()

	graph := TopologyGraph{
		Nodes: []TopologyNode{},
		Edges: []TopologyEdge{},
	}

	nodeID := func(kind, namespace, name string) string {
		return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
	}
	addNode := func(kind, namespace, name, status string, meta ...map[string]string) {
		n := TopologyNode{
			ID:        nodeID(kind, namespace, name),
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
			Status:    status,
		}
		if len(meta) > 0 {
			n.Meta = meta[0]
		}
		graph.Nodes = append(graph.Nodes, n)
	}
	addEdge := func(fromKind, fromNS, fromName, toKind, toNS, toName, edgeType string) {
		graph.Edges = append(graph.Edges, TopologyEdge{
			From: nodeID(fromKind, fromNS, fromName),
			To:   nodeID(toKind, toNS, toName),
			Type: edgeType,
		})
	}
	_ = clusterName

	// Services, Secrets e ConfigMaps são carregados adiantado para permitir
	// a detecção de conexões "connects" (hostname de Service embutido em
	// valores de env var / secret) durante o processamento dos Pods.
	svcs, _ := clients.k8s.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	allSecrets, _ := clients.k8s.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	allConfigMaps, _ := clients.k8s.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})

	secretByKey := map[string]*corev1.Secret{}
	if allSecrets != nil {
		for i := range allSecrets.Items {
			s := &allSecrets.Items[i]
			secretByKey[s.Namespace+"/"+s.Name] = s
		}
	}
	cmByKey := map[string]*corev1.ConfigMap{}
	if allConfigMaps != nil {
		for i := range allConfigMaps.Items {
			c := &allConfigMaps.Items[i]
			cmByKey[c.Namespace+"/"+c.Name] = c
		}
	}
	// nodeID "Service/ns/name" → regex que casa o nome do Service como palavra inteira
	svcNameRe := map[string]*regexp.Regexp{}
	if svcs != nil {
		for _, svc := range svcs.Items {
			if svc.Name == "kubernetes" && svc.Namespace == "default" {
				continue
			}
			svcNameRe[nodeID("Service", svc.Namespace, svc.Name)] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(svc.Name) + `\b`)
		}
	}
	// detectConnects escaneia um valor (env var / conteúdo de secret ou configmap)
	// em busca do nome de algum Service conhecido, criando uma aresta "connects".
	// O valor decodificado NUNCA é incluído na resposta da API, só usado aqui.
	detectConnects := func(podNS, podName, value string) {
		if value == "" {
			return
		}
		for svcID, re := range svcNameRe {
			if re.MatchString(value) {
				parts := strings.SplitN(svcID, "/", 3)
				if len(parts) == 3 {
					addEdge("Pod", podNS, podName, "Service", parts[1], parts[2], "connects")
				}
			}
		}
	}

	// Pods
	pods, err := clients.k8s.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	podLabels := map[string]map[string]string{} // namespace/name → labels
	for _, pod := range pods.Items {
		status := "ok"
		if pod.Status.Phase == "Failed" {
			status = "error"
		} else if pod.Status.Phase == "Pending" {
			status = "warn"
		}
		addNode("Pod", pod.Namespace, pod.Name, status)
		podLabels[pod.Namespace+"/"+pod.Name] = pod.Labels

		// Pod → ConfigMap refs
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				addEdge("Pod", pod.Namespace, pod.Name, "ConfigMap", pod.Namespace, vol.ConfigMap.Name, "mounts")
			}
			if vol.Secret != nil {
				addEdge("Pod", pod.Namespace, pod.Name, "Secret", pod.Namespace, vol.Secret.SecretName, "mounts")
			}
		}
		for _, c := range pod.Spec.Containers {
			for _, env := range c.EnvFrom {
				if env.ConfigMapRef != nil {
					addEdge("Pod", pod.Namespace, pod.Name, "ConfigMap", pod.Namespace, env.ConfigMapRef.Name, "env")
				}
				if env.SecretRef != nil {
					addEdge("Pod", pod.Namespace, pod.Name, "Secret", pod.Namespace, env.SecretRef.Name, "env")
				}
			}
			// env[].valueFrom (referências por chave individual, ex.: DATABASE_URL)
			for _, ev := range c.Env {
				if ev.ValueFrom != nil {
					if ref := ev.ValueFrom.ConfigMapKeyRef; ref != nil {
						addEdge("Pod", pod.Namespace, pod.Name, "ConfigMap", pod.Namespace, ref.Name, "env")
						if cm, ok := cmByKey[pod.Namespace+"/"+ref.Name]; ok {
							detectConnects(pod.Namespace, pod.Name, cm.Data[ref.Key])
						}
					}
					if ref := ev.ValueFrom.SecretKeyRef; ref != nil {
						addEdge("Pod", pod.Namespace, pod.Name, "Secret", pod.Namespace, ref.Name, "env")
						if sec, ok := secretByKey[pod.Namespace+"/"+ref.Name]; ok {
							detectConnects(pod.Namespace, pod.Name, string(sec.Data[ref.Key]))
						}
					}
				} else if ev.Value != "" {
					detectConnects(pod.Namespace, pod.Name, ev.Value)
				}
			}
		}
	}

	// Deployments
	deps, _ := clients.k8s.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if deps != nil {
		for _, dep := range deps.Items {
			status := "ok"
			if dep.Status.UnavailableReplicas > 0 {
				status = "warn"
			}
			addNode("Deployment", dep.Namespace, dep.Name, status)
			// Deployment → Pod (via label selector)
			if dep.Spec.Selector != nil {
				for key, val := range dep.Spec.Selector.MatchLabels {
					for _, pod := range pods.Items {
						if pod.Namespace != dep.Namespace {
							continue
						}
						if v, ok := pod.Labels[key]; ok && v == val {
							addEdge("Deployment", dep.Namespace, dep.Name, "Pod", pod.Namespace, pod.Name, "owns")
						}
					}
				}
			}
		}
	}

	// HPAs (autoscaling/v2)
	hpas, _ := clients.k8s.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if hpas != nil {
		for _, hpa := range hpas.Items {
			status := "ok"
			if hpa.Status.CurrentReplicas < hpa.Status.DesiredReplicas {
				status = "warn"
			}
			minR := int32(1)
			if hpa.Spec.MinReplicas != nil {
				minR = *hpa.Spec.MinReplicas
			}
			meta := map[string]string{
				"minReplicas":     fmt.Sprintf("%d", minR),
				"maxReplicas":     fmt.Sprintf("%d", hpa.Spec.MaxReplicas),
				"currentReplicas": fmt.Sprintf("%d", hpa.Status.CurrentReplicas),
				"desiredReplicas": fmt.Sprintf("%d", hpa.Status.DesiredReplicas),
				"targetKind":      hpa.Spec.ScaleTargetRef.Kind,
				"targetName":      hpa.Spec.ScaleTargetRef.Name,
			}
			addNode("HPA", hpa.Namespace, hpa.Name, status, meta)
			// HPA → target (Deployment, StatefulSet, etc.)
			addEdge("HPA", hpa.Namespace, hpa.Name, hpa.Spec.ScaleTargetRef.Kind, hpa.Namespace, hpa.Spec.ScaleTargetRef.Name, "scales")
		}
	}

	// Services (já carregado no início desta função, ver svcs acima)
	if svcs != nil {
		for _, svc := range svcs.Items {
			if svc.Name == "kubernetes" && svc.Namespace == "default" {
				continue
			}
			addNode("Service", svc.Namespace, svc.Name, "ok")
			// Service → Pod (via label selector)
			if svc.Spec.Selector != nil {
				for _, pod := range pods.Items {
					if pod.Namespace != svc.Namespace {
						continue
					}
					match := true
					for k, v := range svc.Spec.Selector {
						if pod.Labels[k] != v {
							match = false
							break
						}
					}
					if match {
						addEdge("Service", svc.Namespace, svc.Name, "Pod", pod.Namespace, pod.Name, "selects")
					}
				}
			}
		}
	}

	// StatefulSets
	stss, _ := clients.k8s.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if stss != nil {
		for _, sts := range stss.Items {
			status := "ok"
			if sts.Status.ReadyReplicas < sts.Status.Replicas {
				status = "warn"
			}
			addNode("StatefulSet", sts.Namespace, sts.Name, status)
			if sts.Spec.Selector != nil {
				for key, val := range sts.Spec.Selector.MatchLabels {
					for _, pod := range pods.Items {
						if pod.Namespace != sts.Namespace { continue }
						if v, ok := pod.Labels[key]; ok && v == val {
							addEdge("StatefulSet", sts.Namespace, sts.Name, "Pod", pod.Namespace, pod.Name, "owns")
						}
					}
				}
			}
		}
	}

	// DaemonSets
	dss, _ := clients.k8s.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if dss != nil {
		for _, ds := range dss.Items {
			status := "ok"
			if ds.Status.NumberUnavailable > 0 {
				status = "warn"
			}
			addNode("DaemonSet", ds.Namespace, ds.Name, status)
			if ds.Spec.Selector != nil {
				for key, val := range ds.Spec.Selector.MatchLabels {
					for _, pod := range pods.Items {
						if pod.Namespace != ds.Namespace { continue }
						if v, ok := pod.Labels[key]; ok && v == val {
							addEdge("DaemonSet", ds.Namespace, ds.Name, "Pod", pod.Namespace, pod.Name, "owns")
						}
					}
				}
			}
		}
	}

	// ReplicaSets
	rss, _ := clients.k8s.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if rss != nil {
		for _, rs := range rss.Items {
			status := "ok"
			if rs.Status.AvailableReplicas < rs.Status.Replicas {
				status = "warn"
			}
			meta := map[string]string{
				"replicas":          fmt.Sprintf("%d", rs.Status.Replicas),
				"availableReplicas": fmt.Sprintf("%d", rs.Status.AvailableReplicas),
			}
			addNode("ReplicaSet", rs.Namespace, rs.Name, status, meta)
			// Deployment → ReplicaSet via ownerReferences
			for _, ownerRef := range rs.OwnerReferences {
				if ownerRef.Kind == "Deployment" {
					addEdge("Deployment", rs.Namespace, ownerRef.Name, "ReplicaSet", rs.Namespace, rs.Name, "owns")
				}
			}
			// ReplicaSet → Pod via ownerReferences on pods
			for _, pod := range pods.Items {
				if pod.Namespace != rs.Namespace { continue }
				for _, ownerRef := range pod.OwnerReferences {
					if ownerRef.Kind == "ReplicaSet" && ownerRef.Name == rs.Name {
						addEdge("ReplicaSet", rs.Namespace, rs.Name, "Pod", pod.Namespace, pod.Name, "owns")
					}
				}
			}
		}
	}

	// Jobs
	jobs, _ := clients.k8s.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
	if jobs != nil {
		for _, job := range jobs.Items {
			status := "ok"
			if job.Status.Failed > 0 {
				status = "error"
			} else if job.Status.Active > 0 {
				status = "warn"
			}
			addNode("Job", job.Namespace, job.Name, status)
			// Job → Pod via ownerReferences on pods
			for _, pod := range pods.Items {
				if pod.Namespace != job.Namespace { continue }
				for _, ownerRef := range pod.OwnerReferences {
					if ownerRef.Kind == "Job" && ownerRef.Name == job.Name {
						addEdge("Job", job.Namespace, job.Name, "Pod", pod.Namespace, pod.Name, "owns")
					}
				}
			}
		}
	}

	// CronJobs
	cronJobs, _ := clients.k8s.BatchV1().CronJobs(ns).List(ctx, metav1.ListOptions{})
	if cronJobs != nil {
		for _, cj := range cronJobs.Items {
			status := "ok"
			if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
				status = "warn"
			}
			addNode("CronJob", cj.Namespace, cj.Name, status)
			// CronJob → Job via ownerReferences on jobs
			if jobs != nil {
				for _, job := range jobs.Items {
					if job.Namespace != cj.Namespace { continue }
					for _, ownerRef := range job.OwnerReferences {
						if ownerRef.Kind == "CronJob" && ownerRef.Name == cj.Name {
							addEdge("CronJob", cj.Namespace, cj.Name, "Job", job.Namespace, job.Name, "spawns")
						}
					}
				}
			}
		}
	}

	// Ingresses
	ings, _ := clients.k8s.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if ings != nil {
		for _, ing := range ings.Items {
			addNode("Ingress", ing.Namespace, ing.Name, "ok")
			for _, rule := range ing.Spec.Rules {
				if rule.HTTP == nil { continue }
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service != nil {
						addEdge("Ingress", ing.Namespace, ing.Name, "Service", ing.Namespace, path.Backend.Service.Name, "routes")
					}
				}
			}
		}
	}

	// ConfigMaps (apenas os referenciados por pods, excluindo system)
	referencedCMs := map[string]bool{}
	referencedSecrets := map[string]bool{}
	for _, e := range graph.Edges {
		if e.Type == "mounts" || e.Type == "env" {
			// parse id: kind/ns/name
			parts := strings.SplitN(e.To, "/", 3)
			if len(parts) == 3 {
				if parts[0] == "ConfigMap" {
					referencedCMs[parts[1]+"/"+parts[2]] = true
				} else if parts[0] == "Secret" {
					referencedSecrets[parts[1]+"/"+parts[2]] = true
				}
			}
		}
	}
	for key := range referencedCMs {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 {
			addNode("ConfigMap", parts[0], parts[1], "ok")
		}
	}
	for key := range referencedSecrets {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 {
			addNode("Secret", parts[0], parts[1], "ok")
		}
	}

	// Dedup edges
	edgeSet := map[string]bool{}
	deduped := []TopologyEdge{}
	for _, e := range graph.Edges {
		key := e.From + "|" + e.To + "|" + e.Type
		if !edgeSet[key] {
			edgeSet[key] = true
			deduped = append(deduped, e)
		}
	}
	graph.Edges = deduped

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}

// ── Quotas ────────────────────────────────────────────────────────────────────

func handleQuotas(w http.ResponseWriter, r *http.Request) {
	clients, _, err := getCluster(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ns := r.URL.Query().Get("namespace")
	ctx := context.Background()

	// ResourceQuotas
	quotas, err := clients.k8s.CoreV1().ResourceQuotas(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if k8serrors.IsForbidden(err) {
			http.Error(w, "Permissão negada pelo Kubernetes (RBAC). Aplique k8s/rbac.yaml no cluster: kubectl apply -f k8s/rbac.yaml", 403)
		} else {
			http.Error(w, err.Error(), 500)
		}
		return
	}

	// LimitRanges
	limitRanges, err := clients.k8s.CoreV1().LimitRanges(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		limitRanges = &corev1.LimitRangeList{}
	}

	// Agrupar por namespace
	nsMap := map[string]*NamespaceQuota{}
	ensureNS := func(namespace string) *NamespaceQuota {
		if _, ok := nsMap[namespace]; !ok {
			nsMap[namespace] = &NamespaceQuota{
				Namespace:   namespace,
				Quotas:      []QuotaInfo{},
				LimitRanges: []LimitRangeInfo{},
			}
		}
		return nsMap[namespace]
	}

	for _, q := range quotas.Items {
		entry := QuotaInfo{
			Name:      q.Name,
			Resources: map[string]*QuotaResource{},
		}
		for res, hard := range q.Status.Hard {
			used := q.Status.Used[res]
			entry.Resources[string(res)] = &QuotaResource{
				Hard: hard.String(),
				Used: used.String(),
			}
		}
		ensureNS(q.Namespace).Quotas = append(ensureNS(q.Namespace).Quotas, entry)
	}

	for _, lr := range limitRanges.Items {
		for _, limit := range lr.Spec.Limits {
			info := LimitRangeInfo{
				Name:    lr.Name,
				Type:    string(limit.Type),
				Default: map[string]string{},
				Min:     map[string]string{},
				Max:     map[string]string{},
			}
			for res, val := range limit.Default {
				info.Default[string(res)] = val.String()
			}
			for res, val := range limit.Min {
				info.Min[string(res)] = val.String()
			}
			for res, val := range limit.Max {
				info.Max[string(res)] = val.String()
			}
			ensureNS(lr.Namespace).LimitRanges = append(ensureNS(lr.Namespace).LimitRanges, info)
		}
	}

	result := []NamespaceQuota{}
	for _, nq := range nsMap {
		result = append(result, *nq)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Namespace < result[j].Namespace })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ── Routers para thresholds e webhooks (GET/POST/DELETE no mesmo path) ─────────

func handleThresholdsRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetThresholds(w, r)
	case http.MethodPost:
		handleSaveThreshold(w, r)
	case http.MethodDelete:
		handleDeleteThreshold(w, r)
	default:
		http.Error(w, "Method Not Allowed", 405)
	}
}

func handleWebhooksRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetWebhooks(w, r)
	case http.MethodPost:
		handleSaveWebhook(w, r)
	case http.MethodDelete:
		handleDeleteWebhook(w, r)
	default:
		http.Error(w, "Method Not Allowed", 405)
	}
}

func handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "url obrigatória", 400)
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"event":     "test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data": map[string]interface{}{
			"message":   "Webhook de teste do Pod Monitor",
			"namespace": "default",
			"pod":       "exemplo-pod-abc123",
			"container": "app",
			"resource":  "cpu",
			"usagePct":  95.2,
		},
	})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(req.URL, "application/json", bytes.NewReader(payload))
	if err != nil {
		http.Error(w, fmt.Sprintf("Erro ao enviar: %v", err), 502)
		return
	}
	resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "status": resp.StatusCode})
}

// ── Health check ─────────────────────────────────────────────────────────────

//go:embed openapi.yaml
var openapiSpec []byte

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	type healthStatus struct {
		Status   string `json:"status"`
		DB       string `json:"db"`
		Clusters int    `json:"clusters"`
	}
	h := healthStatus{Status: "ok", DB: "ok", Clusters: len(clusters)}
	code := 200
	if db != nil {
		if err := db.PingContext(r.Context()); err != nil {
			h.Status = "degraded"
			h.DB = "error: " + err.Error()
			code = 503
		}
	} else {
		h.DB = "not configured"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(h)
}

func handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(openapiSpec)
}

const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Pod Monitor API</title>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({
  url: "/openapi.yaml",
  dom_id: "#swagger-ui",
  presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
  layout: "BaseLayout",
  deepLinking: true
})
</script>
</body>
</html>`

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	initClients()
	initDB()
	initAuth()
	initGroups()
	initDockerHosts()
	go detectDockerHostTypes()
	go cleanRateLimiter()
	go cleanTokenBlacklist()

	// CORS: usa origem específica se FRONTEND_ORIGIN estiver definida
	frontendOrigin = os.Getenv("FRONTEND_ORIGIN")
	if frontendOrigin == "" {
		frontendOrigin = "*"
		log.Println("[CORS] FRONTEND_ORIGIN não definido — usando wildcard (*). Configure em produção.")
	} else {
		log.Printf("[CORS] Origem permitida: %s", frontendOrigin)
	}

	// Notificação periódica de topologia via SSE (a cada 30s)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			names := clusterNames()
			if len(names) == 0 {
				continue
			}
			payload, _ := json.Marshal(map[string]interface{}{"clusters": names})
			ssePublish("topology_refresh", string(payload))
		}
	}()

	// Públicos
	http.HandleFunc("/api/auth/login",          corsMiddleware(handleLogin))
	http.HandleFunc("/api/auth/logout",         authMiddleware(handleLogout))
	http.HandleFunc("/api/auth/mfa/validate",       corsMiddleware(handleMFAValidate))
	http.HandleFunc("/api/auth/mfa/setup",          corsMiddleware(handleMFASetupInfo))
	http.HandleFunc("/api/auth/mfa/setup/confirm",  corsMiddleware(handleMFASetupConfirm))
	http.HandleFunc("/api/auth/mfa/reset-user",     adminOnly(handleMFAResetUser))

	// Dev + admin/reader (devs veem apenas clusters/namespaces/logs permitidos)
	http.HandleFunc("/api/clusters",   authMiddleware(handleClusters))
	http.HandleFunc("/api/namespaces", authMiddleware(handleNamespaces))
	http.HandleFunc("/api/logs",       devOrAdminOnly(handleLogs))

	// Recursos: dev pode chamar (só para listar pods no contexto de logs)
	http.HandleFunc("/api/resources",  authMiddleware(handleResources))
	http.HandleFunc("/api/nodes",      readerOrAdminOnly(handleNodes))
	http.HandleFunc("/api/storage",    readerOrAdminOnly(handleStorage))
	http.HandleFunc("/api/orphans",    readerOrAdminOnly(handleOrphans))
	http.HandleFunc("/api/history",     readerOrAdminOnly(handleHistory))
	http.HandleFunc("/api/history/csv", readerOrAdminOnly(handleHistoryCSV))
	http.HandleFunc("/api/docker/hosts",      readerOrAdminOnly(handleDockerHosts))
	http.HandleFunc("/api/docker/containers", readerOrAdminOnly(handleDockerContainers))
	http.HandleFunc("/api/containers",        readerOrAdminOnly(handleContainers))
	http.HandleFunc("/api/helm/releases",     readerOrAdminOnly(handleHelmReleases))
	http.HandleFunc("/api/deployments",       readerOrAdminOnly(handleDeployments))
	http.HandleFunc("/api/analysis",          readerOrAdminOnly(handleAnalysis))
	http.HandleFunc("/api/dashboard/summary",    authMiddleware(handleDashboardSummary))
	http.HandleFunc("/api/dashboard/timeseries", authMiddleware(handleDashTimeseries))
	http.HandleFunc("/api/dashboards",           authMiddleware(handleGetDashboards))
	http.HandleFunc("/api/dashboards/save",    authMiddleware(handleSaveDashboard))
	http.HandleFunc("/api/dashboards/delete",  authMiddleware(handleDeleteDashboard))

	// Somente administration
	http.HandleFunc("/api/auth/users",            adminOnly(handleGetUsers))
	http.HandleFunc("/api/auth/users/create",     adminOnly(handleCreateUser))
	http.HandleFunc("/api/auth/users/delete",     adminOnly(handleDeleteUser))
	http.HandleFunc("/api/auth/users/password",    adminOnly(handleChangePassword))
	http.HandleFunc("/api/auth/users/role",        adminOnly(handleUpdateUserRole))
	http.HandleFunc("/api/auth/users/dev-access", adminOnly(handleUpdateDevAccess))
	http.HandleFunc("/api/auth/mfa/toggle",       adminOnly(handleAdminToggleMFA))
	http.HandleFunc("/api/auth/groups",           adminOnly(handleGetGroups))
	http.HandleFunc("/api/auth/groups/create",    adminOnly(handleCreateGroup))
	http.HandleFunc("/api/auth/groups/delete",    adminOnly(handleDeleteGroup))
	http.HandleFunc("/api/auth/groups/access",    adminOnly(handleGroupAccess))
	http.HandleFunc("/api/auth/groups/mfa",       adminOnly(handleGroupMFA))
	http.HandleFunc("/api/auth/groups/members",   adminOnly(handleGroupMembers))
	http.HandleFunc("/api/admin/validate",     adminOnly(handleAdminValidate))
	http.HandleFunc("/api/admin/create-sa",   adminOnly(handleAdminCreateSA))
	http.HandleFunc("/api/admin/apply",       adminOnly(handleAdminApply))
	http.HandleFunc("/api/admin/docker-host",    adminOnly(handleAdminDockerHost))
	http.HandleFunc("/api/admin/cluster/delete", adminOnly(handleAdminDeleteCluster))

	// Novos endpoints
	http.HandleFunc("/api/topology",          readerOrAdminOnly(handleTopology))
	http.HandleFunc("/api/quotas",            readerOrAdminOnly(handleQuotas))
	http.HandleFunc("/api/audit",             adminOnly(handleAuditLog))
	http.HandleFunc("/api/thresholds",        adminOnly(handleThresholdsRouter))
	http.HandleFunc("/api/webhooks",          adminOnly(handleWebhooksRouter))
	http.HandleFunc("/api/webhooks/test",     adminOnly(handleWebhookTest))
	http.HandleFunc("/api/sse/events",        authMiddleware(handleSSE))

	http.HandleFunc("/healthz",      handleHealthz)
	http.HandleFunc("/openapi.yaml", handleOpenAPISpec)
	http.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, swaggerUIHTML)
	})
	fmt.Println("Backend rodando na porta 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
