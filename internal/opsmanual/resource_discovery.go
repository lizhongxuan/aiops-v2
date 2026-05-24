package opsmanual

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
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
	Image      string            `json:"image,omitempty"`
	Ports      []string          `json:"ports,omitempty"`
	Health     string            `json:"health,omitempty"`
	Mounts     []string          `json:"mounts,omitempty"`
	Networks   []string          `json:"networks,omitempty"`
	CreatedAt  string            `json:"created_at,omitempty"`

	ListeningPorts  []string `json:"listening_ports,omitempty"`
	SystemdService  string   `json:"systemd_service,omitempty"`
	ProcessOwner    string   `json:"process_owner,omitempty"`
	Version         string   `json:"version,omitempty"`
	Phase           string   `json:"phase,omitempty"`
	ContainerImages []string `json:"container_images,omitempty"`
}

type ResourceDiscovery interface {
	DiscoverHostResources(ctx context.Context, host string) ([]ResourceCandidate, error)
	DiscoverExecutionSurfaces(ctx context.Context, host string) ([]ParamCandidate, error)
}

type DiscoveryCommandRunner func(ctx context.Context, command string, args ...string) ([]byte, error)

type localResourceDiscovery struct {
	run      DiscoveryCommandRunner
	timeout  time.Duration
	registry *CapabilityRegistry
}

func NewLocalResourceDiscovery() ResourceDiscovery {
	return NewLocalResourceDiscoveryWithRunner(nil)
}

func NewLocalResourceDiscoveryWithRunner(runner DiscoveryCommandRunner) ResourceDiscovery {
	return NewLocalResourceDiscoveryWithRunnerAndRegistry(runner, nil)
}

func NewLocalResourceDiscoveryWithRunnerAndRegistry(runner DiscoveryCommandRunner, registry *CapabilityRegistry) ResourceDiscovery {
	if runner == nil {
		runner = defaultDiscoveryCommandRunner
	}
	if registry == nil {
		registry = DefaultOpsManualCapabilityRegistry()
	}
	return localResourceDiscovery{run: runner, timeout: 2 * time.Second, registry: registry}
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
	providers := d.registry.ResourceDiscoveryProviders()
	if len(providers) == 0 {
		return nil, fmt.Errorf("%s", d.registry.UnavailableMessage("resource discovery"))
	}
	for _, provider := range providers {
		switch strings.TrimSpace(provider.ID) {
		case resourceProviderDocker:
			out = append(out, d.discoverDockerResources(ctx, host)...)
		case resourceProviderHost:
			out = append(out, d.discoverHostProcessResources(ctx, host)...)
		case resourceProviderKubernetes:
			out = append(out, d.discoverK8sResources(ctx, host)...)
		}
	}
	return dedupeResourceCandidates(out), nil
}

func (d localResourceDiscovery) DiscoverExecutionSurfaces(ctx context.Context, host string) ([]ParamCandidate, error) {
	resources, err := d.DiscoverHostResources(ctx, host)
	if err != nil {
		return nil, err
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
	output, err := d.runWithTimeout(ctx, "docker", "ps", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}\t{{.CreatedAt}}")
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
		createdAt := ""
		if len(fields) > 5 {
			createdAt = strings.TrimSpace(fields[5])
		}
		resourceType := middlewareTypeFromText(name + " " + image)
		if resourceType == "" || name == "" {
			continue
		}
		metadata := d.inspectDockerResource(ctx, firstNonEmpty(id, name))
		portsList := splitListField(ports)
		candidate := ResourceCandidate{
			ID:         "docker:" + name,
			Name:       name,
			Type:       resourceType,
			Host:       strings.TrimSpace(host),
			Surface:    "docker exec " + name,
			Source:     "docker",
			Evidence:   strings.TrimSpace(fmt.Sprintf("docker ps: id=%s image=%s ports=%s status=%s", id, image, ports, status)),
			Confidence: 0.92,
			Image:      firstNonEmpty(metadata.Image, image),
			Ports:      portsList,
			Health:     firstNonEmpty(metadata.Health, healthFromDockerStatus(status)),
			Mounts:     metadata.Mounts,
			Networks:   metadata.Networks,
			CreatedAt:  firstNonEmpty(metadata.CreatedAt, createdAt),
		}
		out = append(out, candidate)
	}
	return out
}

func (d localResourceDiscovery) discoverHostProcessResources(ctx context.Context, host string) []ResourceCandidate {
	output, err := d.runWithTimeout(ctx, "ps", "-axo", "user,comm,args")
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
		metadata := hostProcessMetadata(resourceType, line)
		name := firstNonEmpty(metadata.Name, hostProcessResourceName(resourceType, line))
		out = append(out, ResourceCandidate{
			ID:             "host:" + resourceType + ":" + name,
			Name:           name,
			Type:           resourceType,
			Host:           strings.TrimSpace(host),
			Surface:        hostExecutionSurface(host),
			Source:         "host_readonly",
			Evidence:       hostProcessEvidence(line, metadata),
			Confidence:     0.78,
			ListeningPorts: metadata.ListeningPorts,
			SystemdService: metadata.SystemdService,
			ProcessOwner:   metadata.ProcessOwner,
			Version:        metadata.Version,
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
			ID:              "k8s:pod:" + namespace + "/" + name,
			Name:            name,
			Type:            resourceType,
			Host:            strings.TrimSpace(host),
			Surface:         "kubectl -n " + namespace + " exec " + name + " --",
			Source:          "k8s",
			Evidence:        strings.TrimSpace(fmt.Sprintf("kubectl get pods -A -o json: namespace=%s pod=%s phase=%s", namespace, name, pod.Status.Phase)),
			Confidence:      0.88,
			Cluster:         strings.TrimSpace(cluster),
			Namespace:       namespace,
			Pod:             name,
			Labels:          cloneStringMap(pod.Metadata.Labels),
			Phase:           strings.TrimSpace(pod.Status.Phase),
			ContainerImages: k8sContainerImages(pod.Spec.Containers),
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
			Ports:      k8sServicePorts(service),
		})
	}
	return out
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

type dockerInspectMetadata struct {
	Image     string
	Health    string
	Mounts    []string
	Networks  []string
	CreatedAt string
}

type dockerInspectContainer struct {
	Created string `json:"Created"`
	Config  struct {
		Image string `json:"Image"`
	} `json:"Config"`
	State struct {
		Status string `json:"Status"`
		Health struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Mounts []struct {
		Name        string `json:"Name"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		Type        string `json:"Type"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]any `json:"Networks"`
	} `json:"NetworkSettings"`
}

func (d localResourceDiscovery) inspectDockerResource(ctx context.Context, idOrName string) dockerInspectMetadata {
	idOrName = strings.TrimSpace(idOrName)
	if idOrName == "" {
		return dockerInspectMetadata{}
	}
	output, err := d.runWithTimeout(ctx, "docker", "inspect", idOrName)
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return dockerInspectMetadata{}
	}
	var containers []dockerInspectContainer
	if err := json.Unmarshal(output, &containers); err != nil || len(containers) == 0 {
		return dockerInspectMetadata{}
	}
	container := containers[0]
	networks := make([]string, 0, len(container.NetworkSettings.Networks))
	for network := range container.NetworkSettings.Networks {
		if strings.TrimSpace(network) != "" {
			networks = append(networks, strings.TrimSpace(network))
		}
	}
	sort.Strings(networks)
	mounts := make([]string, 0, len(container.Mounts))
	for _, mount := range container.Mounts {
		left := firstNonEmpty(mount.Source, mount.Name, mount.Type)
		if left == "" && mount.Destination == "" {
			continue
		}
		if mount.Destination == "" {
			mounts = append(mounts, left)
			continue
		}
		mounts = append(mounts, strings.TrimSpace(left+":"+mount.Destination))
	}
	return dockerInspectMetadata{
		Image:     strings.TrimSpace(container.Config.Image),
		Health:    firstNonEmpty(container.State.Health.Status, container.State.Status),
		Mounts:    mounts,
		Networks:  networks,
		CreatedAt: strings.TrimSpace(container.Created),
	}
}

func splitListField(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func healthFromDockerStatus(status string) string {
	lower := strings.ToLower(status)
	switch {
	case strings.Contains(lower, "(healthy)"):
		return "healthy"
	case strings.Contains(lower, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(lower, "starting"):
		return "starting"
	default:
		return ""
	}
}

type hostProcessInfo struct {
	Name           string
	ProcessOwner   string
	ListeningPorts []string
	SystemdService string
	Version        string
}

var hostPortPattern = regexp.MustCompile(`(?::|=)(\d{2,5})\b`)
var hostVersionPattern = regexp.MustCompile(`\b\d+\.\d+(?:\.\d+)?\b`)

func hostProcessMetadata(resourceType string, line string) hostProcessInfo {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return hostProcessInfo{}
	}
	owner := ""
	command := fields[0]
	argsStart := 0
	if len(fields) > 1 && middlewareTypeFromProcessName(fields[0]) == "" && middlewareTypeFromProcessName(fields[1]) != "" {
		owner = fields[0]
		command = fields[1]
		argsStart = 1
	}
	args := strings.Join(fields[argsStart:], " ")
	ports := uniqueStrings(regexpSubmatches(hostPortPattern, args))
	return hostProcessInfo{
		Name:           hostProcessName(resourceType, command),
		ProcessOwner:   owner,
		ListeningPorts: ports,
		SystemdService: systemdServiceName(resourceType, command),
		Version:        firstRegexMatch(hostVersionPattern, args),
	}
}

func hostProcessEvidence(line string, info hostProcessInfo) string {
	parts := []string{"ps: " + strings.TrimSpace(line)}
	if info.ProcessOwner != "" {
		parts = append(parts, "owner="+info.ProcessOwner)
	}
	if len(info.ListeningPorts) > 0 {
		parts = append(parts, "ports="+strings.Join(info.ListeningPorts, ","))
	}
	if info.Version != "" {
		parts = append(parts, "version="+info.Version)
	}
	if info.SystemdService != "" {
		parts = append(parts, "service="+info.SystemdService)
	}
	return strings.Join(parts, " ")
}

func hostProcessName(resourceType string, command string) string {
	command = strings.TrimSpace(command)
	switch resourceType {
	case "postgresql":
		return "postgres"
	case "mysql":
		return "mysqld"
	default:
		return command
	}
}

func systemdServiceName(resourceType string, command string) string {
	command = strings.TrimSpace(command)
	switch resourceType {
	case "redis":
		if command == "" {
			return "redis.service"
		}
		return command + ".service"
	case "postgresql":
		return "postgresql.service"
	case "mysql":
		return "mysqld.service"
	case "kafka":
		return "kafka.service"
	case "nginx":
		return "nginx.service"
	case "elasticsearch":
		return "elasticsearch.service"
	default:
		return ""
	}
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

func k8sContainerImages(containers []k8sContainer) []string {
	out := make([]string, 0, len(containers))
	for _, container := range containers {
		image := strings.TrimSpace(container.Image)
		if image != "" {
			out = append(out, image)
		}
	}
	return uniqueStrings(out)
}

func k8sServicePorts(service k8sService) []string {
	out := make([]string, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		if port.Port == 0 {
			continue
		}
		if strings.TrimSpace(port.Name) != "" {
			out = append(out, fmt.Sprintf("%s:%d", strings.TrimSpace(port.Name), port.Port))
			continue
		}
		out = append(out, fmt.Sprint(port.Port))
	}
	return out
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

func regexpSubmatches(pattern *regexp.Regexp, value string) []string {
	matches := pattern.FindAllStringSubmatch(value, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			out = append(out, strings.TrimSpace(match[1]))
		}
	}
	return out
}

func firstRegexMatch(pattern *regexp.Regexp, value string) string {
	match := pattern.FindString(value)
	return strings.TrimSpace(match)
}

func uniqueStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
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
