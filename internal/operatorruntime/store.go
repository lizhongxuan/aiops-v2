package operatorruntime

import "context"

type Store interface {
	SaveResource(context.Context, ManagedResource) error
	ListResources(context.Context) ([]ManagedResource, error)
	GetResource(context.Context, string) (ManagedResource, bool, error)

	SavePGCluster(context.Context, PGCluster) error
	ListPGClusters(context.Context) ([]PGCluster, error)
	GetPGCluster(context.Context, string) (PGCluster, bool, error)

	SaveInspectionTemplate(context.Context, InspectionTemplate) error
	ListInspectionTemplates(context.Context) ([]InspectionTemplate, error)
	GetInspectionTemplate(context.Context, string) (InspectionTemplate, bool, error)

	SaveProblemType(context.Context, ProblemType) error
	ListProblemTypes(context.Context) ([]ProblemType, error)
	GetProblemType(context.Context, string) (ProblemType, bool, error)

	SaveAction(context.Context, ActionCatalogItem) error
	ListActions(context.Context) ([]ActionCatalogItem, error)
	GetAction(context.Context, string) (ActionCatalogItem, bool, error)

	SaveWorkflowBinding(context.Context, WorkflowBinding) error
	ListWorkflowBindings(context.Context) ([]WorkflowBinding, error)
	GetWorkflowBinding(context.Context, string) (WorkflowBinding, bool, error)

	SaveGuardRule(context.Context, GuardRule) error
	ListGuardRules(context.Context) ([]GuardRule, error)
	GetGuardRule(context.Context, string) (GuardRule, bool, error)
	SetGuardRuleEnabled(context.Context, string, bool) (GuardRule, error)

	CreateGuardRun(context.Context, GuardRun) error
	AppendGuardRunEvent(context.Context, string, GuardRunEvent) error
	UpdateGuardRunState(context.Context, string, GuardRunState) error
	ListGuardRuns(context.Context) ([]GuardRun, error)
	GetGuardRun(context.Context, string) (GuardRun, bool, error)
}
