package experiencepack

import "testing"

func TestVariantResolverSelectsOSSpecificGeneAndBinding(t *testing.T) {
	base := testGene("gene_base")
	ubuntu := testGene("gene_ubuntu")
	ubuntu.EnvSelector = map[string]any{"os_distribution": "ubuntu"}
	rhel := testGene("gene_rhel")
	rhel.EnvSelector = map[string]any{"os_distribution": "rhel"}
	binding := RunnerBinding{ID: "binding_ubuntu", GeneID: "gene_ubuntu", Published: true, EnvSelector: map[string]any{"os_distribution": "ubuntu"}}
	gene, selected, gaps := ResolveVariant(VariantContext{OSDistribution: "ubuntu"}, []GEPGene{base, ubuntu, rhel}, []RunnerBinding{binding})
	if gene.ID != "gene_ubuntu" || selected.ID != "binding_ubuntu" || len(gaps) != 0 {
		t.Fatalf("unexpected ubuntu variant: gene=%s binding=%s gaps=%v", gene.ID, selected.ID, gaps)
	}
	_, _, gaps = ResolveVariant(VariantContext{}, []GEPGene{base, ubuntu, rhel}, []RunnerBinding{binding})
	if !contains(gaps, "需要确认目标主机操作系统") {
		t.Fatalf("expected OS gap, got %v", gaps)
	}
}
