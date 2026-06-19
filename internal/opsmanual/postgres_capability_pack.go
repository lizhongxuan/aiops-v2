package opsmanual

func PostgresCapabilityPack() OpsManualCapabilityPack {
	return OpsManualCapabilityPack{
		ID:       "builtin.postgresql",
		BuiltIn:  true,
		Enabled:  true,
		Priority: 1100,
		ObjectAliases: []CapabilityAlias{{
			Value:   "postgresql",
			Needles: []string{"postgresql", "postgres", " pg ", "pg-", "pg ", "pg主", "pg从", "pg集群"},
		}},
		MiddlewareAliases: []CapabilityAlias{{
			Value:   "postgresql",
			Needles: []string{"postgresql", "postgres", " pg ", "pg-", "pg ", "pg主", "pg从", "pg集群"},
		}},
		StatefulTargetTypes: []string{"postgresql"},
		ParameterHints: []CapabilityParameterHint{
			{ID: "target_instance", TargetType: "postgresql", Action: "repair", Required: true, Source: "operation_frame"},
			{ID: "execution_surface", TargetType: "postgresql", Action: "repair", Required: true, Source: "operation_frame"},
		},
		PreflightProbes: []CapabilityPreflightProbe{
			{ID: "postgres_member_health", TargetType: "postgresql", Action: "repair", RiskLevel: "low", ReadOnly: true},
			{ID: "postgres_replication_status", TargetType: "postgresql", Action: "repair", RiskLevel: "low", ReadOnly: true},
			{ID: "postgres_storage_health", TargetType: "postgresql", Action: "repair", RiskLevel: "low", ReadOnly: true},
		},
	}
}
