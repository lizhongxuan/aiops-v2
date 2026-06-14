package resourceio

const (
	DefaultMaxReadBytes = 4096
	MaxQueryMatches     = 20
)

type Format string

const (
	FormatText     Format = "text"
	FormatJSON     Format = "json"
	FormatMetadata Format = "metadata"
)

type Range struct {
	Offset int64  `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Query  string `json:"query,omitempty"`
	Page   int    `json:"page,omitempty"`
	Format string `json:"format,omitempty"`
}

type ReadRequest struct {
	ID     string `json:"id,omitempty"`
	URI    string `json:"uri,omitempty"`
	Range  Range  `json:"range,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Query  string `json:"query,omitempty"`
	Page   int    `json:"page,omitempty"`
	Format string `json:"format,omitempty"`
}

type ReadResult struct {
	Content      string    `json:"content,omitempty"`
	Matches      []Match   `json:"matches,omitempty"`
	Ref          Reference `json:"ref"`
	Offset       int64     `json:"offset,omitempty"`
	Limit        int       `json:"limit,omitempty"`
	Page         int       `json:"page,omitempty"`
	Truncated    bool      `json:"truncated,omitempty"`
	MetadataOnly bool      `json:"metadataOnly,omitempty"`
}

type Match struct {
	Offset  int64  `json:"offset"`
	Content string `json:"content"`
}

type Reference struct {
	Kind        string `json:"kind,omitempty"`
	URI         string `json:"uri,omitempty"`
	FilePath    string `json:"filePath,omitempty"`
	Title       string `json:"title,omitempty"`
	Summary     string `json:"summary,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Digest      string `json:"digest,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	Range       Range  `json:"range,omitempty"`
	Version     string `json:"version,omitempty"`
}
