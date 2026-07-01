package appui

import "strings"

const (
	HostConnectionModeAIOPSPull    = "aiops_pull"
	HostConnectionModeNodePushGRPC = "node_push_grpc"
)

func NormalizeHostConnectionMode(value string) string {
	mode := strings.ToLower(strings.TrimSpace(value))
	switch mode {
	case "", "default", "pull", "aiops-pull", HostConnectionModeAIOPSPull:
		return HostConnectionModeAIOPSPull
	case "grpc_reverse", "node-push-grpc", HostConnectionModeNodePushGRPC:
		return HostConnectionModeNodePushGRPC
	default:
		return HostConnectionModeAIOPSPull
	}
}

func hostConnectionModeRequiresCallback(value string) bool {
	return NormalizeHostConnectionMode(value) == HostConnectionModeNodePushGRPC
}
