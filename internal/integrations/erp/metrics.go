package erp

type BusinessMetric struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
	Status    string  `json:"status"`
}

type TenantImpact struct {
	TenantID     string   `json:"tenantId"`
	TenantName   string   `json:"tenantName"`
	Severity     string   `json:"severity"`
	KeyProcesses []string `json:"keyProcesses"`
}

type JobStatus struct {
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	QueueDepth    int      `json:"queueDepth"`
	RecentFailure string   `json:"recentFailure,omitempty"`
	Dependencies  []string `json:"dependencies,omitempty"`
}
