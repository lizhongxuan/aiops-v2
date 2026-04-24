package workflow

type Workflow struct {
	Version       string         `json:"version" yaml:"version"`
	Name          string         `json:"name" yaml:"name"`
	Description   string         `json:"description" yaml:"description"`
	EnvPackages   []string       `json:"env_packages" yaml:"env_packages"`
	ValidationEnv string         `json:"validation_env" yaml:"validation_env"`
	Inventory     Inventory      `json:"inventory" yaml:"inventory"`
	Vars          map[string]any `json:"vars" yaml:"vars"`
	Plan          Plan           `json:"plan" yaml:"plan"`
	Steps         []Step         `json:"steps" yaml:"steps"`
	Handlers      []Handler      `json:"handlers" yaml:"handlers"`
	Tests         []Test         `json:"tests" yaml:"tests"`
}

type Inventory struct {
	Groups map[string]Group `json:"groups" yaml:"groups"`
	Hosts  map[string]Host  `json:"hosts" yaml:"hosts"`
	Vars   map[string]any   `json:"vars" yaml:"vars"`
}

type Group struct {
	Hosts []string       `json:"hosts" yaml:"hosts"`
	Vars  map[string]any `json:"vars" yaml:"vars"`
}

type Host struct {
	Address string         `json:"address" yaml:"address"`
	Vars    map[string]any `json:"vars" yaml:"vars"`
}

type Plan struct {
	Mode     string `json:"mode" yaml:"mode"`
	Strategy string `json:"strategy" yaml:"strategy"`
}

type Step struct {
	Name            string         `json:"name" yaml:"name"`
	Targets         []string       `json:"targets" yaml:"targets"`
	Action          string         `json:"action" yaml:"action"`
	Args            map[string]any `json:"args" yaml:"args"`
	MustVars        []string       `json:"must_vars" yaml:"must_vars"`
	When            string         `json:"when" yaml:"when"`
	Loop            []any          `json:"loop" yaml:"loop"`
	Retries         int            `json:"retries" yaml:"retries"`
	Timeout         string         `json:"timeout" yaml:"timeout"`
	ContinueOnError bool           `json:"continue_on_error" yaml:"continue_on_error"`
	ExpectVars      []string       `json:"expect_vars" yaml:"expect_vars"`
	Notify          []string       `json:"notify" yaml:"notify"`
}

type Handler struct {
	Name    string         `json:"name" yaml:"name"`
	Action  string         `json:"action" yaml:"action"`
	Args    map[string]any `json:"args" yaml:"args"`
	When    string         `json:"when" yaml:"when"`
	Retries int            `json:"retries" yaml:"retries"`
	Timeout string         `json:"timeout" yaml:"timeout"`
}

type Test struct {
	Name   string         `json:"name" yaml:"name"`
	Action string         `json:"action" yaml:"action"`
	Args   map[string]any `json:"args" yaml:"args"`
}
