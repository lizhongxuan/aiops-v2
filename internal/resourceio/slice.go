package resourceio

import "strings"

func ReadText(content string, req ReadRequest) ReadResult {
	return ReadTextWithMax(content, req, DefaultMaxReadBytes)
}

func ReadTextWithMax(content string, req ReadRequest, maxReadBytes int) ReadResult {
	rng := NormalizeRangeWithMax(req, maxReadBytes)
	ref := Reference{
		URI:         strings.TrimSpace(req.URI),
		ContentType: "text/plain",
		Digest:      DigestContent([]byte(content)),
		Bytes:       int64(len(content)),
		Range:       rng,
	}
	result := ReadResult{
		Ref:    ref,
		Offset: rng.Offset,
		Limit:  rng.Limit,
		Page:   rng.Page,
	}
	if MetadataOnly(rng.Format) {
		result.MetadataOnly = true
		return result
	}
	if rng.Query != "" {
		result.Matches = queryText(content, rng.Query, rng.Limit)
		return result
	}
	if rng.Offset > int64(len(content)) {
		rng.Offset = int64(len(content))
		result.Offset = rng.Offset
	}
	end := int(rng.Offset) + rng.Limit
	if end > len(content) {
		end = len(content)
	}
	result.Content = content[int(rng.Offset):end]
	result.Truncated = end < len(content)
	return result
}

func ReadBytes(content []byte, req ReadRequest, contentType string) ReadResult {
	return ReadBytesWithMax(content, req, contentType, DefaultMaxReadBytes)
}

func ReadBytesWithMax(content []byte, req ReadRequest, contentType string, maxReadBytes int) ReadResult {
	contentType = DetectContentType(req.URI, content, contentType)
	rng := NormalizeRangeWithMax(req, maxReadBytes)
	ref := Reference{
		URI:         strings.TrimSpace(req.URI),
		ContentType: contentType,
		Digest:      DigestContent(content),
		Bytes:       int64(len(content)),
		Range:       rng,
	}
	result := ReadResult{
		Ref:    ref,
		Offset: rng.Offset,
		Limit:  rng.Limit,
		Page:   rng.Page,
	}
	if MetadataOnly(rng.Format) || !IsTextContentType(contentType) {
		result.MetadataOnly = true
		return result
	}
	result = ReadTextWithMax(strings.ToValidUTF8(string(content), ""), req, maxReadBytes)
	result.Ref = ref
	return result
}

func queryText(text, query string, limit int) []Match {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	matches := make([]Match, 0, 1)
	searchFrom := 0
	for {
		idx := strings.Index(lowerText[searchFrom:], lowerQuery)
		if idx < 0 {
			break
		}
		pos := searchFrom + idx
		start := queryWindowStart(text, pos, len(query), limit)
		end := start + limit
		if end > len(text) {
			end = len(text)
		}
		if end < start {
			end = start
		}
		matches = append(matches, Match{
			Offset:  int64(pos),
			Content: text[start:end],
		})
		searchFrom = pos + len(query)
		if len(matches) >= MaxQueryMatches {
			break
		}
	}
	return matches
}

func queryWindowStart(text string, pos, queryLen, limit int) int {
	if limit <= 0 {
		return pos
	}
	if queryLen >= limit {
		return pos
	}
	start := pos - (limit-queryLen)/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end < pos+queryLen {
		end = pos + queryLen
		start = end - limit
	}
	if end > len(text) {
		start = len(text) - limit
		if start < 0 {
			start = 0
		}
	}
	return start
}
