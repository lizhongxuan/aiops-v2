package observability

type Config struct {
	Enabled       bool
	Endpoint      string
	ServiceName   string
	Project       string
	IncludePrompt bool
}
