package hostops

import "time"

// HostMentionSource identifies how a textual @mention was interpreted.
type HostMentionSource string

const (
	HostMentionSourceInventory       HostMentionSource = "inventory"
	HostMentionSourceIPLiteral       HostMentionSource = "ip_literal"
	HostMentionSourceHostnameLiteral HostMentionSource = "hostname_literal"
)

// HostMention is the server-side representation of one @host token.
type HostMention struct {
	TokenID     string            `json:"tokenId"`
	Raw         string            `json:"raw"`
	SpanStart   int               `json:"spanStart"`
	SpanEnd     int               `json:"spanEnd"`
	HostID      string            `json:"hostId,omitempty"`
	Address     string            `json:"address,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	Source      HostMentionSource `json:"source"`
	Resolved    bool              `json:"resolved"`
	Confidence  float64           `json:"confidence"`
	CreatedAt   time.Time         `json:"createdAt"`
}
