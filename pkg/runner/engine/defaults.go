package engine

import (
	"runner/modules"
	"runner/modules/builtin"
	runnerhttp "runner/modules/http"
	"runner/modules/script"
	"runner/modules/wait"
)

// DefaultRegistry returns a registry populated with built-in modules.
func DefaultRegistry() *modules.Registry {
	reg := modules.NewRegistry()
	reg.Register("script.shell", script.New("shell"))
	reg.Register("script.python", script.New("python"))
	reg.Register("http.request", runnerhttp.New())
	reg.Register("builtin.tcp_ping", builtin.NewTCPPing())
	reg.Register("builtin.http_check", builtin.NewHTTPCheck())
	reg.Register("builtin.ssl_expiry_check", builtin.NewSSLExpiryCheck())
	reg.Register("builtin.dns_resolve", builtin.NewDNSResolve())
	reg.Register("wait.event", wait.NewEvent())
	return reg
}
