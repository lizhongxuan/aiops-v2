package experiencepack

import "strings"

func AvoidCueFromFailedCapsule(capsule GEPCapsule) AvoidCue {
	warning := "避免重复执行失败路径"
	content := strings.ToLower(capsule.Content + " " + capsule.Summary)
	signals := append([]string{}, capsule.Trigger...)
	if strings.Contains(content, "data dir") || strings.Contains(content, "数据目录") {
		warning = "不要在已有 PostgreSQL 数据目录时初始化或覆盖实例"
		signals = append(signals, "existing data dir", "已有数据目录")
	}
	cue := AvoidCue{
		Type: "AvoidCue", ID: "avoid_" + capsule.ID, GeneID: capsule.Gene,
		Signals: signals, Warning: warning, Evidence: capsule.ID, Severity: "high", Blocking: true,
	}
	cue.AssetID = MustHashCanonicalJSON(cue)
	return cue
}

func ActiveGeneView(gene GEPGene, cues []AvoidCue) GEPGene {
	active := gene
	for _, cue := range cues {
		if cue.GeneID == gene.ID && cue.Warning != "" {
			active.EpigeneticMarks = append(active.EpigeneticMarks, "AVOID: "+cue.Warning)
		}
	}
	return active
}
