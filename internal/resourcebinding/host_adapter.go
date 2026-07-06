package resourcebinding

import (
	"strings"
)

const HostVerifierHostopsResolver = "hostops.Resolver"

type HostBindingInput struct {
	HostID      string
	DisplayName string
	Namespace   string
	Provider    string
	Source      string
	VerifiedBy  string
	Verified    bool
	Raw         string
}

type HostMentionProjection struct {
	Raw         string
	HostID      string
	Address     string
	DisplayName string
	Source      string
	Resolved    bool
	Confidence  float64
}

func HostBindingFromMention(mention HostMentionProjection) ResourceBindingSnapshot {
	id := firstNonEmpty(
		mention.HostID,
		mention.Address,
		strings.TrimPrefix(strings.TrimSpace(mention.DisplayName), "@"),
		strings.TrimPrefix(strings.TrimSpace(mention.Raw), "@"),
	)
	verified := mention.Resolved && strings.TrimSpace(mention.HostID) != ""
	verifiedBy := ""
	if verified {
		verifiedBy = HostVerifierHostopsResolver
	}
	return BuildHostBinding(HostBindingInput{
		HostID:      id,
		DisplayName: firstNonEmpty(mention.DisplayName, mention.Raw, id),
		Source:      BindingSourceMention,
		VerifiedBy:  verifiedBy,
		Verified:    verified,
		Raw:         mention.Raw,
	})
}

func HostBindingFromSessionTarget(hostID, displayName, verifiedBy string) ResourceBindingSnapshot {
	return BuildHostBinding(HostBindingInput{
		HostID:      hostID,
		DisplayName: displayName,
		Source:      BindingSourceSessionTarget,
		VerifiedBy:  verifiedBy,
		Verified:    strings.TrimSpace(verifiedBy) != "",
	})
}

func HostBindingFromRouteMetadata(hostID, displayName, verifiedBy string) ResourceBindingSnapshot {
	return BuildHostBinding(HostBindingInput{
		HostID:      hostID,
		DisplayName: displayName,
		Source:      BindingSourceRouteMetadata,
		VerifiedBy:  verifiedBy,
		Verified:    strings.TrimSpace(verifiedBy) != "",
	})
}

func BuildHostBinding(input HostBindingInput) ResourceBindingSnapshot {
	ref := ResourceRef{
		Type:        ResourceTypeHost,
		ID:          firstNonEmpty(input.HostID, strings.TrimPrefix(strings.TrimSpace(input.Raw), "@")),
		DisplayName: firstNonEmpty(input.DisplayName, input.HostID),
		Namespace:   input.Namespace,
		Provider:    input.Provider,
	}
	trustLevel := TrustLevelRejected
	verifiedBy := strings.TrimSpace(input.VerifiedBy)
	if input.Verified && verifiedBy != "" && ref.ID != "" {
		trustLevel = TrustLevelVerified
	} else if normalizeBindingSource(input.Source) == BindingSourceMention && (strings.TrimSpace(input.Raw) != "" || strings.TrimSpace(input.DisplayName) != "") {
		trustLevel = TrustLevelWeak
	}
	return NewBindingSnapshot(ref, BindingOptions{
		Source:     input.Source,
		VerifiedBy: verifiedBy,
		TrustLevel: trustLevel,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
