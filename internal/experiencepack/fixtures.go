package experiencepack

import "context"

func SeedPGClusterFixture(ctx context.Context, svc *Service) error {
	_, err := svc.GenerateAndPersistCandidate(ctx, CandidateInput{
		PackID:   "pack_aiops_pg_primary_standby",
		Name:     "PostgreSQL 主从集群部署经验",
		Summary:  "在多台主机上部署 PostgreSQL 主从并配置 pg_mon 监控",
		Category: CategoryInnovate,
		Trajectory: Trajectory{
			CaseID: "case-pg-cluster", ChatSessionID: "chat-pg-cluster",
			UserGoal: "我要对 xxA 主机和 xxB 主机上部署 pg，并且搭建成 pg 主从集群，pg_mon 放在 xxC 主机",
			Commands: []string{"check host xxA", "check host xxB", "install pg", "init primary", "base backup", "deploy pg_mon"},
			ProofID:  "proof-pg-cluster", Outcome: "success",
			Environment: EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", OSVersion: "22.04", PackageManager: "apt", HostCount: 3, MiddlewareVersions: map[string]string{"postgres": "16"}},
		},
		Env: EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", OSVersion: "22.04", PackageManager: "apt", HostCount: 3, MiddlewareVersions: map[string]string{"postgres": "16"}},
	})
	if err != nil {
		return err
	}
	manifest, err := svc.Review(ctx, "pack_aiops_pg_primary_standby", true)
	if err != nil {
		return err
	}
	_, err = svc.SaveScopes(ctx, manifest.ID, []AuthorizationScope{{ID: "env:prod", Type: "environment", Value: "prod", Searchable: true, Reason: "fixture"}})
	if err != nil {
		return err
	}
	_, err = svc.Enable(ctx, manifest.ID, true)
	return err
}
