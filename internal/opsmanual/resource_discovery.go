package opsmanual

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ResourceCandidate struct {
	ID         string            `json:"id"`
	Name       string            `json:"name,omitempty"`
	Type       string            `json:"type,omitempty"`
	Host       string            `json:"host,omitempty"`
	Surface    string            `json:"surface,omitempty"`
	Source     string            `json:"source,omitempty"`
	Evidence   string            `json:"evidence,omitempty"`
	Confidence float64           `json:"confidence,omitempty"`
	Cluster    string            `json:"cluster,omitempty"`
	Namespace  string            `json:"namespace,omitempty"`
	Pod        string            `json:"pod,omitempty"`
	Service    string            `json:"service,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type ResourceDiscovery interface {
	DiscoverHostResources(ctx context.Context, host string) ([]ResourceCandidate, error)
	DiscoverExecutionSurfaces(ctx context.Context, host string) ([]ParamCandidate, error)
}

type DiscoveryCommandRunner func(ctx context.Context, command string, args ...string) ([]byte, error)

type localResourceDiscovery struct {
	run     DiscoveryCommandRunner
	timeout time.Duration
}

func NewLocalResourceDiscovery() ResourceDiscovery {
	return NewLocalResourceDiscoveryWithRunner(nil)
}

func NewLocalResourceDiscoveryWithRunner(runner DiscoveryCommandRunner) ResourceDiscovery {
	if runner == nil {
		runner = defaultDiscoveryCommandRunner
	}
	return localResourceDiscovery{run: runner, timeout: 2 * time.Second}
}

type noopResourceDiscovery struct{}

func (noopResourceDiscovery) DiscoverHostResources(context.Context, string) ([]ResourceCandidate, error) {
	return nil, nil
}

func (noopResourceDiscovery) DiscoverExecutionSurfaces(context.Context, string) ([]ParamCandidate, error) {
	return nil, nil
}

func (d localResourceDiscovery) DiscoverHostResources(ctx context.Context, host string) ([]ResourceCandidate, error) {
	var out []ResourceCandidate
	out = append(out, d.discoverDockerResources(ctx, host)...)
	out = append(out, d.discoverHostProcessResources(ctx, host)...)
	out = append(out, d.discoverK8sResources(ctx, host)...)
	out = append(out, d.discoverCorootResources(ctx, host)...)
	return dedupeResourceCandidates(out), nil
}

func (d localResourceDiscovery) DiscoverExecutionSurfaces(ctx context.Context, host string) ([]ParamCandidate, error) {
	resources, err := d.DiscoverHostResources(ctx, host)
	if err != nil {
		return nil, nil
	}
	var out []ParamCandidate
	for _, resource := range resources {
		surface := strings.TrimSpace(resource.Surface)
		if surface == "" {
			continue
		}
		out = append(out, ParamCandidate{
			Value:      surface,
			Label:      surface,
			Source:     firstNonEmpty(resource.Source, "resource_discovery"),
			Confidence: resource.Confidence,
			Evidence:   firstNonEmpty(resource.Evidence, "read-only resource discovery"),
		})
	}
	return dedupeParamCandidates(out), nil
}

func (d localResourceDiscovery) discoverDockerResources(ctx context.Context, host string) []ResourceCandidate {
	output, err := d.runWithTimeout(ctx, "docker", "ps", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return nil
	}
	var out []ResourceCandidate
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		id := strings.TrimSpace(fields[0])
		name := strings.TrimSpace(fields[1])
		image := strings.TrimSpace(fields[2])
		ports := ""
		status := ""
		if len(fields) > 3 {
			ports = strings.TrimSpace(fields[3])
		}
		if len(fields) > 4 {
			status = strings.TrimSpace(fields[4])
		}
		resourceType := middlewareTypeFromText(name + " " + image)
		if resourceType == "" || name == "" {
			continue
		}
		out = append(out, ResourceCandidate{
			ID:         "docker:" + name,
			Name:       name,
			Type:       resourceType,
			Host:       strings.TrimSpace(host),
			Surface:    "docker exec " + name,
			Source:     "docker",
			Evidence:   strings.TrimSpace(fmt.Sprintf("docker ps: id=%s image=%s ports=%s status=%s", id, image, ports, status)),
			Confidence: 0.92,
		})
	}
	return out
}

func (d localResourceDiscovery) discoverHostProcessResources(ctx context.Context, host string) []ResourceCandidate {
	output, err := d.runWithTimeout(ctx, "ps", "-axo", "comm,args")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return nil
	}
	var out []ResourceCandidate
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		if lower == "" || strings.HasPrefix(lower, "comm ") {
			continue
		}
		resourceType := middlewareTypeFromHostProcessLine(line)
		if resourceType == "" {
			continue
		}
		name := hostProcessResourceName(resourceType, line)
		out = append(out, ResourceCandidate{
			ID:         "host:" + resourceType + ":" + name,
			Name:       name,
			Type:       resourceType,
			Host:       strings.TrimSpace(host),
			Surface:    hostExecutionSurface(host),
			Source:     "host_readonly",
			Evidence:   "ps: " + strings.TrimSpace(line),
			Confidence: 0.78,
		})
	}
	return out
}

func (d localResourceDiscovery) discoverK8sResources(ctx context.Context, host string) []ResourceCandidate {
	cluster := d.discoverK8sCluster(ctx)
	var out []ResourceCandidate
	out = append(out, d.discoverK8sPods(ctx, host, cluster)...)
	out = append(out, d.discoverK8sServices(ctx, host, cluster)...)
	return out
}

func (d localResourceDiscovery) discoverK8sCluster(ctx context.Context) string {
	output, err := d.runWithTimeout(ctx, "kubectl", "config", "current-context")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (d localResourceDiscovery) discoverK8sPods(ctx context.Context, host string, cluster string) []ResourceCandidate {
	output, err := d.runWithTimeout(ctx, "kubectl", "get", "pods", "-A", "-o", "json")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return nil
	}
	var list k8sPodList
	if err := json.Unmarshal(output, &list); err != nil {
		return nil
	}
	var out []ResourceCandidate
	for _, pod := range list.Items {
		namespace := strings.TrimSpace(pod.Metadata.Namespace)
		name := strings.TrimSpace(pod.Metadata.Name)
		if namespace == "" || name == "" {
			continue
		}
		resourceType := middlewareTypeFromText(k8sPodDiscoveryText(pod))
		if resourceType == "" {
			continue
		}
		out = append(out, ResourceCandidate{
			ID:         "k8s:pod:" + namespace + "/" + name,
			Name:       name,
			Type:       resourceType,
			Host:       strings.TrimSpace(host),
			Surface:    "kubectl -n " + namespace + " exec " + name + " --",
			Source:     "k8s",
			Evidence:   strings.TrimSpace(fmt.Sprintf("kubectl get pods -A -o json: namespace=%s pod=%s phase=%s", namespace, name, pod.Status.Phase)),
			Confidence: 0.88,
			Cluster:    strings.TrimSpace(cluster),
			Namespace:  namespace,
			Pod:        name,
			Labels:     cloneStringMap(pod.Metadata.Labels),
		})
	}
	return out
}

func (d localResourceDiscovery) discoverK8sServices(ctx context.Context, host string, cluster string) []ResourceCandidate {
	output, err := d.runWithTimeout(ctx, "kubectl", "get", "services", "-A", "-o", "json")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return nil
	}
	var list k8sServiceList
	if err := json.Unmarshal(output, &list); err != nil {
		return nil
	}
	var out []ResourceCandidate
	for _, service := range list.Items {
		namespace := strings.TrimSpace(service.Metadata.Namespace)
		name := strings.TrimSpace(service.Metadata.Name)
		if namespace == "" || name == "" {
			continue
		}
		resourceType := middlewareTypeFromText(k8sServiceDiscoveryText(service))
		if resourceType == "" {
			continue
		}
		out = append(out, ResourceCandidate{
			ID:         "k8s:service:" + namespace + "/" + name,
			Name:       name,
			Type:       resourceType,
			Host:       strings.TrimSpace(host),
			Surface:    "kubectl -n " + namespace + " get service " + name,
			Source:     "k8s",
			Evidence:   strings.TrimSpace(fmt.Sprintf("kubectl get services -A -o json: namespace=%s service=%s type=%s clusterIP=%s", namespace, name, service.Spec.Type, service.Spec.ClusterIP)),
			Confidence: 0.82,
			Cluster:    strings.TrimSpace(cluster),
			Namespace:  namespace,
			Service:    name,
			Labels:     cloneStringMap(service.Metadata.Labels),
		})
	}
	return out
}

func (d localResourceDiscovery) discoverCorootResources(context.Context, string) []ResourceCandidate {
	return nil
}

func (d localResourceDiscovery) runWithTimeout(ctx context.Context, command string, args ...string) ([]byte, error) {
	timeout := d.timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return d.run(runCtx, command, args...)
}

type k8sObjectMetadata struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

type k8sContainer struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type k8sPodList struct {
	Items []k8sPod `json:"items"`
}

type k8sPod struct {
	Metadata k8sObjectMetadata `json:"metadata"`
	Spec     struct {
		Containers []k8sContainer `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

type k8sServiceList struct {
	Items []k8sService `json:"items"`
}

type k8sService struct {
	Metadata k8sObjectMetadata `json:"metadata"`
	Spec     struct {
		Type      string            `json:"type"`
		ClusterIP string            `json:"clusterIP"`
		Selector  map[string]string `json:"selector"`
		Ports     []struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		} `json:"ports"`
	} `json:"spec"`
}

func k8sPodDiscoveryText(pod k8sPod) string {
	parts := []string{pod.Metadata.Name, labelsDiscoveryText(pod.Metadata.Labels)}
	for _, container := range pod.Spec.Containers {
		parts = append(parts, container.Name, container.Image)
	}
	return strings.Join(parts, " ")
}

func k8sServiceDiscoveryText(service k8sService) string {
	parts := []string{service.Metadata.Name, labelsDiscoveryText(service.Metadata.Labels), labelsDiscoveryText(service.Spec.Selector)}
	for _, port := range service.Spec.Ports {
		parts = append(parts, port.Name, fmt.Sprint(port.Port))
	}
	return strings.Join(parts, " ")
}

func labelsDiscoveryText(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels)*2)
	for key, value := range labels {
		parts = append(parts, key, value)
	}
	return strings.Join(parts, " ")
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func defaultDiscoveryCommandRunner(ctx context.Context, command string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, command, args...).Output()
}

func middlewareTypeFromText(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "redis"):
		return "redis"
	case strings.Contains(lower, "postgres") || strings.Contains(lower, "postgresql") || strings.Contains(lower, "pgvector"):
		return "postgresql"
	case strings.Contains(lower, "mysqld") || strings.Contains(lower, "mysql"):
		return "mysql"
	case strings.Contains(lower, "kafka"):
		return "kafka"
	case strings.Contains(lower, "nginx"):
		return "nginx"
	case strings.Contains(lower, "elasticsearch"):
		return "elasticsearch"
	default:
		return ""
	}
}

func middlewareTypeFromHostProcessLine(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	if resourceType := middlewareTypeFromProcessName(fields[0]); resourceType != "" {
		return resourceType
	}
	if len(fields) > 1 {
		return middlewareTypeFromProcessName(fields[1])
	}
	return ""
}

func middlewareTypeFromProcessName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return ""
	}
	if idx := strings.LastIndexAny(lower, `/\`); idx >= 0 {
		lower = lower[idx+1:]
	}
	switch {
	case lower == "redis-server" || lower == "redis-sentinel":
		return "redis"
	case lower == "postgres" || lower == "postmaster":
		return "postgresql"
	case lower == "mysqld" || lower == "mariadbd":
		return "mysql"
	case lower == "kafka-server-start" || lower == "kafka":
		return "kafka"
	case lower == "nginx":
		return "nginx"
	case lower == "elasticsearch":
		return "elasticsearch"
	default:
		return ""
	}
}

func hostProcessResourceName(resourceType string, line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return resourceType
	}
	switch resourceType {
	case "postgresql":
		return "postgres"
	case "mysql":
		return "mysqld"
	default:
		return strings.TrimSpace(fields[0])
	}
}

func hostExecutionSurface(host string) string {
	host = strings.TrimSpace(host)
	switch strings.ToLower(host) {
	case "", "localhost", "127.0.0.1", "server-local", "local":
		return "local shell"
	default:
		return "ssh " + host
	}
}

func dedupeResourceCandidates(candidates []ResourceCandidate) []ResourceCandidate {
	var out []ResourceCandidate
	seen := map[string]bool{}
	for _, candidate := range candidates {
		key := strings.TrimSpace(candidate.ID)
		if key == "" {
			key = strings.TrimSpace(candidate.Source + ":" + candidate.Type + ":" + candidate.Name)
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}
