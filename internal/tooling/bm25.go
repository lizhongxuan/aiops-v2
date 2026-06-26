package tooling

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

const (
	defaultBM25K1 = 1.5
	defaultBM25B  = 0.75
)

type BM25Document struct {
	ID   int
	Text string
}

type BM25Result struct {
	ID    int
	Score float64
}

type BM25Index struct {
	docs      []bm25IndexedDocument
	docFreq   map[string]int
	avgDocLen float64
}

type bm25IndexedDocument struct {
	id       int
	termFreq map[string]int
	length   int
}

func NewBM25Index(documents []BM25Document) *BM25Index {
	index := &BM25Index{docFreq: map[string]int{}}
	totalLength := 0
	for _, doc := range documents {
		tokens := bm25Tokenize(doc.Text)
		if len(tokens) == 0 {
			continue
		}
		indexed := bm25IndexedDocument{
			id:       doc.ID,
			termFreq: map[string]int{},
			length:   len(tokens),
		}
		seen := map[string]struct{}{}
		for _, token := range tokens {
			indexed.termFreq[token]++
			seen[token] = struct{}{}
		}
		for token := range seen {
			index.docFreq[token]++
		}
		totalLength += indexed.length
		index.docs = append(index.docs, indexed)
	}
	if len(index.docs) > 0 {
		index.avgDocLen = float64(totalLength) / float64(len(index.docs))
	}
	return index
}

func (i *BM25Index) Search(query string, limit int) []BM25Result {
	if i == nil || len(i.docs) == 0 {
		return nil
	}
	queryTerms := bm25Tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(i.docs) {
		limit = len(i.docs)
	}
	results := make([]BM25Result, 0, len(i.docs))
	for _, doc := range i.docs {
		score := i.scoreDocument(doc, queryTerms)
		if score <= 0 {
			continue
		}
		results = append(results, BM25Result{ID: doc.id, Score: score})
	}
	sort.Slice(results, func(a, b int) bool {
		if results[a].Score != results[b].Score {
			return results[a].Score > results[b].Score
		}
		return results[a].ID < results[b].ID
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func (i *BM25Index) scoreDocument(doc bm25IndexedDocument, queryTerms []string) float64 {
	if i.avgDocLen <= 0 || doc.length == 0 {
		return 0
	}
	score := 0.0
	totalDocs := float64(len(i.docs))
	for _, term := range queryTerms {
		tf := doc.termFreq[term]
		if tf == 0 {
			continue
		}
		df := float64(i.docFreq[term])
		idf := math.Log(1 + (totalDocs-df+0.5)/(df+0.5))
		tfFloat := float64(tf)
		denominator := tfFloat + defaultBM25K1*(1-defaultBM25B+defaultBM25B*float64(doc.length)/i.avgDocLen)
		score += idf * (tfFloat * (defaultBM25K1 + 1)) / denominator
	}
	return score
}

func bm25Tokenize(text string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if isCJKRune(r) {
				flush()
				tokens = append(tokens, string(r))
			} else {
				current.WriteRune(r)
			}
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func isCJKRune(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}
