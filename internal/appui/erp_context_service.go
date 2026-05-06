package appui

import (
	"context"
	"strings"
)

type ERPHealthCommand struct {
	Environment string `json:"environment,omitempty"`
}

type ERPMetricCommand struct {
	Capability  string `json:"capability,omitempty"`
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
}

type ERPTenantImpactCommand struct {
	Capability  string `json:"capability,omitempty"`
	Environment string `json:"environment,omitempty"`
}

type ERPHealthCapabilityView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

type ERPBusinessMetricView struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
	Status    string  `json:"status"`
}

type ERPTenantImpactView struct {
	TenantID     string   `json:"tenantId"`
	TenantName   string   `json:"tenantName"`
	Severity     string   `json:"severity"`
	KeyProcesses []string `json:"keyProcesses"`
}

type ERPHealthView struct {
	Environment  string                    `json:"environment"`
	Capabilities []ERPHealthCapabilityView `json:"capabilities"`
}

type ERPContextService interface {
	Health(ctx context.Context, cmd ERPHealthCommand) (ERPHealthView, error)
	BusinessMetrics(ctx context.Context, cmd ERPMetricCommand) ([]ERPBusinessMetricView, error)
	TenantImpact(ctx context.Context, cmd ERPTenantImpactCommand) ([]ERPTenantImpactView, error)
}

type defaultERPContextService struct{}

func NewERPContextService() ERPContextService {
	return &defaultERPContextService{}
}

func (s *defaultERPContextService) Health(_ context.Context, cmd ERPHealthCommand) (ERPHealthView, error) {
	environment := firstNonEmptyString(cmd.Environment, "prod")
	return ERPHealthView{
		Environment: environment,
		Capabilities: []ERPHealthCapabilityView{
			{ID: "capability.order.submit", Name: "订单提交", Status: "degraded", Summary: "p95 latency above threshold and success rate below SLO"},
			{ID: "capability.inventory.deduct", Name: "库存扣减", Status: "healthy", Summary: "reservation path within threshold"},
			{ID: "capability.report.job", Name: "报表任务", Status: "degraded", Summary: "queue depth elevated for finance reporting jobs"},
		},
	}, nil
}

func (s *defaultERPContextService) BusinessMetrics(_ context.Context, cmd ERPMetricCommand) ([]ERPBusinessMetricView, error) {
	service := strings.TrimSpace(cmd.Service)
	if service == "" {
		service = "order-api"
	}
	return []ERPBusinessMetricView{
		{Name: service + ".order_submit_success_rate", Value: 98.7, Unit: "%", Threshold: 99.5, Status: "warning"},
		{Name: service + ".order_submit_p95_latency_ms", Value: 1420, Unit: "ms", Threshold: 800, Status: "critical"},
		{Name: "payment_callback_lag_s", Value: 18, Unit: "s", Threshold: 60, Status: "ok"},
	}, nil
}

func (s *defaultERPContextService) TenantImpact(context.Context, ERPTenantImpactCommand) ([]ERPTenantImpactView, error) {
	return []ERPTenantImpactView{
		{TenantID: "tenant-east-01", TenantName: "华东直营网点", Severity: "high", KeyProcesses: []string{"订单提交", "库存预留", "财务入账"}},
		{TenantID: "tenant-north-03", TenantName: "华北渠道", Severity: "medium", KeyProcesses: []string{"订单提交", "发票申请"}},
	}, nil
}
