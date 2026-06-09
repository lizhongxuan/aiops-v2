package resourceio

import "strings"

func NormalizeRange(req ReadRequest) Range {
	return NormalizeRangeWithMax(req, DefaultMaxReadBytes)
}

func NormalizeRangeWithMax(req ReadRequest, maxReadBytes int) Range {
	if maxReadBytes <= 0 {
		maxReadBytes = DefaultMaxReadBytes
	}
	rng := req.Range
	if req.Offset != 0 {
		rng.Offset = req.Offset
	}
	if req.Limit != 0 {
		rng.Limit = req.Limit
	}
	if strings.TrimSpace(req.Query) != "" {
		rng.Query = req.Query
	}
	if req.Page != 0 {
		rng.Page = req.Page
	}
	if strings.TrimSpace(req.Format) != "" {
		rng.Format = req.Format
	}
	return NormalizeRangeValue(rng, maxReadBytes)
}

func NormalizeRangeValue(rng Range, maxReadBytes int) Range {
	if maxReadBytes <= 0 {
		maxReadBytes = DefaultMaxReadBytes
	}
	rng.Query = strings.TrimSpace(rng.Query)
	rng.Format = strings.ToLower(strings.TrimSpace(rng.Format))
	if rng.Offset < 0 {
		rng.Offset = 0
	}
	if rng.Limit <= 0 || rng.Limit > maxReadBytes {
		rng.Limit = maxReadBytes
	}
	if rng.Page < 0 {
		rng.Page = 0
	}
	if rng.Page > 0 && rng.Offset == 0 {
		rng.Offset = int64(rng.Page-1) * int64(rng.Limit)
	}
	return rng
}

func MetadataOnly(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), string(FormatMetadata))
}
