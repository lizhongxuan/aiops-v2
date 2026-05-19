package opsmanual

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLocalResourceDiscoveryDiscoversDockerMiddleware(t *testing.T) {
	discovery := NewLocalResourceDiscoveryWithRunner(func(_ context.Context, command string, args ...string) ([]byte, error) {
		switch command {
		case "docker":
			if len(args) > 0 && args[0] == "inspect" {
				return []byte(`[{
					"Created": "2026-05-19T01:02:03Z",
					"Mounts": [{"Source": "/data/redis", "Destination": "/data"}],
					"NetworkSettings": {"Networks": {"aiops-net": {}}},
					"State": {"Health": {"Status": "healthy"}}
				}]`), nil
			}
			return []byte(strings.Join([]string{
				"abc123\taiops-redis\tredis:7\t0.0.0.0:6379->6379/tcp\tUp 10 minutes (healthy)\t2026-05-19 01:02:03 +0000 UTC",
				"def456\taiops-postgres\tpostgres:16\t0.0.0.0:5432->5432/tcp\tUp 8 minutes",
				"ghi789\taiops-mysql\tmysql:8\t0.0.0.0:3306->3306/tcp\tUp 6 minutes",
			}, "\n")), nil
		case "ps":
			return []byte("COMM ARGS\n"), nil
		default:
			return nil, errors.New("unexpected command")
		}
	})

	resources, err := discovery.DiscoverHostResources(context.Background(), "server-local")
	if err != nil {
		t.Fatalf("DiscoverHostResources() error = %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("resources = %#v, want three docker resources", resources)
	}
	if resources[0].ID != "docker:aiops-redis" || resources[0].Type != "redis" || resources[0].Surface != "docker exec aiops-redis" {
		t.Fatalf("redis resource = %#v", resources[0])
	}
	if resources[0].Image != "redis:7" || !containsString(resources[0].Ports, "0.0.0.0:6379->6379/tcp") || resources[0].Health != "healthy" {
		t.Fatalf("redis docker metadata = %#v", resources[0])
	}
	if !containsString(resources[0].Mounts, "/data/redis:/data") || !containsString(resources[0].Networks, "aiops-net") || resources[0].CreatedAt == "" {
		t.Fatalf("redis docker inspect metadata = %#v", resources[0])
	}
	if resources[1].Type != "postgresql" {
		t.Fatalf("postgres resource = %#v", resources[1])
	}
	if resources[2].ID != "docker:aiops-mysql" || resources[2].Type != "mysql" || resources[2].Surface != "docker exec aiops-mysql" {
		t.Fatalf("mysql resource = %#v", resources[2])
	}
}

func TestLocalResourceDiscoveryDockerFailureFallsBackToHostProcesses(t *testing.T) {
	discovery := NewLocalResourceDiscoveryWithRunner(func(_ context.Context, command string, args ...string) ([]byte, error) {
		switch command {
		case "docker":
			return nil, errors.New("docker not found")
		case "ps":
			return []byte("USER COMM ARGS\nredis redis-server redis-server 7.2.1 *:6379\nmysql mysqld /usr/sbin/mysqld --port=3306\n"), nil
		default:
			return nil, errors.New("unexpected command")
		}
	})

	resources, err := discovery.DiscoverHostResources(context.Background(), "db-01")
	if err != nil {
		t.Fatalf("DiscoverHostResources() error = %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("resources = %#v, want host redis and mysql", resources)
	}
	if resources[0].Type != "redis" || resources[0].Surface != "ssh db-01" {
		t.Fatalf("host redis resource = %#v", resources[0])
	}
	if resources[0].ProcessOwner != "redis" || !containsString(resources[0].ListeningPorts, "6379") || resources[0].SystemdService != "redis-server.service" || resources[0].Version != "7.2.1" {
		t.Fatalf("host redis metadata = %#v", resources[0])
	}
	if resources[1].Type != "mysql" {
		t.Fatalf("host mysql resource = %#v", resources[1])
	}
}

func TestLocalResourceDiscoveryHostProcessesIgnoreRequestTextNoise(t *testing.T) {
	discovery := NewLocalResourceDiscoveryWithRunner(func(_ context.Context, command string, args ...string) ([]byte, error) {
		switch command {
		case "docker":
			return nil, errors.New("docker not found")
		case "ps":
			return []byte("COMM ARGS\nzsh zsh -lc curl --data '{\"request_text\":\"请按运维手册给本机 PostgreSQL 做备份\"}'\ncurl curl --data '{\"manual_id\":\"manual-pg-backup-ubuntu\"}'\n"), nil
		default:
			return nil, errors.New("unexpected command")
		}
	})

	resources, err := discovery.DiscoverHostResources(context.Background(), "server-local")
	if err != nil {
		t.Fatalf("DiscoverHostResources() error = %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("resources = %#v, want no middleware candidates from shell/curl request text", resources)
	}
}

func TestLocalResourceDiscoveryExecutionSurfacesUseDiscoveredResourceSurface(t *testing.T) {
	discovery := NewLocalResourceDiscoveryWithRunner(func(_ context.Context, command string, args ...string) ([]byte, error) {
		switch command {
		case "docker":
			return []byte("abc123\taiops-redis\tredis:7\t0.0.0.0:6379->6379/tcp\tUp 10 minutes\n"), nil
		case "ps":
			return []byte("COMM ARGS\n"), nil
		default:
			return nil, errors.New("unexpected command")
		}
	})

	surfaces, err := discovery.DiscoverExecutionSurfaces(context.Background(), "server-local")
	if err != nil {
		t.Fatalf("DiscoverExecutionSurfaces() error = %v", err)
	}
	if len(surfaces) != 1 || surfaces[0].Value != "docker exec aiops-redis" {
		t.Fatalf("surfaces = %#v, want docker exec aiops-redis", surfaces)
	}
}

func TestLocalResourceDiscoveryDiscoversK8sPodsAndServices(t *testing.T) {
	discovery := NewLocalResourceDiscoveryWithRunner(func(_ context.Context, command string, args ...string) ([]byte, error) {
		switch command {
		case "docker":
			return []byte(""), nil
		case "ps":
			return []byte("COMM ARGS\n"), nil
		case "kubectl":
			joined := strings.Join(args, " ")
			switch joined {
			case "config current-context":
				return []byte("dev-cluster\n"), nil
			case "get pods -A -o json":
				return []byte(`{
					"items": [{
						"metadata": {
							"name": "redis-0",
							"namespace": "cache",
							"labels": {"app": "redis", "tier": "cache"}
						},
						"spec": {"containers": [{"name": "redis", "image": "redis:7"}]},
						"status": {"phase": "Running"}
					}]
				}`), nil
			case "get services -A -o json":
				return []byte(`{
					"items": [{
						"metadata": {
							"name": "postgresql",
							"namespace": "data",
							"labels": {"app.kubernetes.io/name": "postgresql"}
						},
						"spec": {
							"type": "ClusterIP",
							"clusterIP": "10.96.0.10",
							"selector": {"app": "postgres"},
							"ports": [{"name": "postgresql", "port": 5432}]
						}
					}]
				}`), nil
			default:
				return nil, errors.New("unexpected kubectl args: " + joined)
			}
		default:
			return nil, errors.New("unexpected command")
		}
	})

	resources, err := discovery.DiscoverHostResources(context.Background(), "server-local")
	if err != nil {
		t.Fatalf("DiscoverHostResources() error = %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("resources = %#v, want k8s pod and service", resources)
	}
	pod := resources[0]
	if pod.ID != "k8s:pod:cache/redis-0" || pod.Type != "redis" || pod.Cluster != "dev-cluster" || pod.Namespace != "cache" || pod.Pod != "redis-0" {
		t.Fatalf("pod resource = %#v", pod)
	}
	if pod.Surface != "kubectl -n cache exec redis-0 --" || pod.Labels["app"] != "redis" {
		t.Fatalf("pod surface/labels = %#v", pod)
	}
	if pod.Phase != "Running" || !containsString(pod.ContainerImages, "redis:7") {
		t.Fatalf("pod metadata = %#v", pod)
	}
	service := resources[1]
	if service.ID != "k8s:service:data/postgresql" || service.Type != "postgresql" || service.Namespace != "data" || service.Service != "postgresql" {
		t.Fatalf("service resource = %#v", service)
	}
	if service.Surface != "kubectl -n data get service postgresql" || service.Labels["app.kubernetes.io/name"] != "postgresql" {
		t.Fatalf("service surface/labels = %#v", service)
	}
}

func TestCorootResolverReportsProviderUnavailableWithoutCandidate(t *testing.T) {
	registry := NewDefaultParamResolverRegistry(fakeResourceDiscovery{})
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement: ParamRequirement{ID: "target_instance", Type: "resource_ref"},
		Manual:      OpsManual{Operation: OperationProfile{TargetType: "redis"}},
	})
	if result.Message == "" || !strings.Contains(result.Message, "coroot provider unavailable") {
		t.Fatalf("message = %q, want coroot provider unavailable evidence", result.Message)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want no fabricated coroot candidate", result.Candidates)
	}
}

func TestLocalResourceDiscoveryK8sUnavailableKeepsDockerAndHost(t *testing.T) {
	discovery := NewLocalResourceDiscoveryWithRunner(func(_ context.Context, command string, args ...string) ([]byte, error) {
		switch command {
		case "docker":
			return []byte("abc123\taiops-redis\tredis:7\t0.0.0.0:6379->6379/tcp\tUp 10 minutes\n"), nil
		case "ps":
			return []byte("COMM ARGS\nmysqld /usr/sbin/mysqld\n"), nil
		case "kubectl":
			return nil, errors.New("kubectl unavailable")
		default:
			return nil, errors.New("unexpected command")
		}
	})

	resources, err := discovery.DiscoverHostResources(context.Background(), "db-01")
	if err != nil {
		t.Fatalf("DiscoverHostResources() error = %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("resources = %#v, want docker and host candidates only", resources)
	}
	if resources[0].Source != "docker" || resources[0].Type != "redis" {
		t.Fatalf("docker resource = %#v", resources[0])
	}
	if resources[1].Source != "host_readonly" || resources[1].Type != "mysql" {
		t.Fatalf("host resource = %#v", resources[1])
	}
}

func TestLocalResourceDiscoveryCorootPlaceholderDoesNotCallNetwork(t *testing.T) {
	var commands []string
	discovery := NewLocalResourceDiscoveryWithRunner(func(_ context.Context, command string, args ...string) ([]byte, error) {
		commands = append(commands, command+" "+strings.Join(args, " "))
		switch command {
		case "docker", "ps":
			return []byte(""), nil
		case "kubectl":
			return nil, errors.New("kubectl unavailable")
		default:
			return nil, errors.New("unexpected command")
		}
	})

	resources, err := discovery.DiscoverHostResources(context.Background(), "server-local")
	if err != nil {
		t.Fatalf("DiscoverHostResources() error = %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("resources = %#v, want no candidates", resources)
	}
	for _, command := range commands {
		if strings.HasPrefix(command, "curl ") || strings.HasPrefix(command, "wget ") {
			t.Fatalf("coroot placeholder made network command: %s", command)
		}
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
