package experiencepack

import (
	"fmt"
	"strings"
)

type VariantContext struct {
	OSDistribution     string
	OSVersion          string
	PackageManager     string
	Kernel             string
	Architecture       string
	KubernetesVersion  string
	ContainerRuntime   string
	MiddlewareVersions map[string]string
}

func VariantContextFromFingerprint(fp EnvironmentFingerprint) VariantContext {
	return VariantContext{
		OSDistribution: strings.ToLower(fp.OSDistribution),
		OSVersion:      fp.OSVersion, PackageManager: strings.ToLower(fp.PackageManager), Kernel: fp.Kernel,
		Architecture: fp.Architecture, KubernetesVersion: fp.KubernetesVersion,
		ContainerRuntime: fp.ContainerRuntime, MiddlewareVersions: fp.MiddlewareVersions,
	}
}

func ResolveVariant(ctx VariantContext, genes []GEPGene, bindings []RunnerBinding) (GEPGene, RunnerBinding, []string) {
	if ctx.OSDistribution == "" && hasVariantSelectors(genes, bindings) {
		return firstBaseGene(genes), RunnerBinding{}, []string{"需要确认目标主机操作系统"}
	}
	gene := bestVariantGene(ctx, genes)
	binding := bestBinding(ctx, gene, bindings)
	if binding.ID == "" && hasVariantSelectors(genes, bindings) && ctx.OSDistribution != "" {
		return gene, binding, []string{fmt.Sprintf("当前 OS %s 暂无可用 Runner Binding", ctx.OSDistribution)}
	}
	return gene, binding, nil
}

func bestVariantGene(ctx VariantContext, genes []GEPGene) GEPGene {
	var fallback GEPGene
	for _, gene := range genes {
		if fallback.ID == "" {
			fallback = gene
		}
		if selectorMatches(ctx, gene.EnvSelector) {
			return gene
		}
	}
	return fallback
}

func bestBinding(ctx VariantContext, gene GEPGene, bindings []RunnerBinding) RunnerBinding {
	var fallback RunnerBinding
	for _, binding := range bindings {
		if gene.ID != "" && binding.GeneID != "" && binding.GeneID != gene.ID {
			continue
		}
		if fallback.ID == "" && binding.Published {
			fallback = binding
		}
		if binding.Published && selectorMatches(ctx, binding.EnvSelector) {
			return binding
		}
	}
	return fallback
}

func hasVariantSelectors(genes []GEPGene, bindings []RunnerBinding) bool {
	for _, gene := range genes {
		if len(gene.EnvSelector) > 0 {
			return true
		}
	}
	for _, binding := range bindings {
		if len(binding.EnvSelector) > 0 {
			return true
		}
	}
	return false
}

func firstBaseGene(genes []GEPGene) GEPGene {
	for _, gene := range genes {
		if len(gene.EnvSelector) == 0 {
			return gene
		}
	}
	if len(genes) > 0 {
		return genes[0]
	}
	return GEPGene{}
}

func selectorMatches(ctx VariantContext, selector map[string]any) bool {
	if len(selector) == 0 {
		return false
	}
	if value, ok := selector["os_distribution"]; ok {
		if strings.ToLower(fmt.Sprint(value)) != strings.ToLower(ctx.OSDistribution) {
			return false
		}
	}
	if value, ok := selector["package_manager"]; ok {
		if ctx.PackageManager != "" && strings.ToLower(fmt.Sprint(value)) != strings.ToLower(ctx.PackageManager) {
			return false
		}
	}
	return true
}
