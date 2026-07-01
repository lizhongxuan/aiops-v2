package publicweb

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	htmlScriptStyleRE = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>|<style\b[^>]*>.*?</style>|<noscript\b[^>]*>.*?</noscript>`)
	htmlTitleRE       = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
	htmlTagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	whitespaceRE      = regexp.MustCompile(`\s+`)
)

func readableText(body, contentType string) (string, string) {
	text := body
	title := ""
	if strings.Contains(strings.ToLower(contentType), "html") || strings.Contains(strings.ToLower(body), "<html") {
		if match := htmlTitleRE.FindStringSubmatch(body); len(match) == 2 {
			title = compactWhitespace(html.UnescapeString(htmlTagRE.ReplaceAllString(match[1], " ")))
		}
		text = htmlScriptStyleRE.ReplaceAllString(body, " ")
		for _, marker := range []string{"</p>", "</div>", "</li>", "</h1>", "</h2>", "</h3>", "<br>", "<br/>", "<br />"} {
			text = strings.ReplaceAll(text, marker, "\n")
		}
		text = htmlTagRE.ReplaceAllString(text, " ")
	}
	text = compactWhitespace(html.UnescapeString(text))
	if text == "" {
		text = "(empty response)"
	}
	return text, title
}

func compactWhitespace(value string) string {
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(value, " "))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateBytes(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return strings.Repeat(".", limit)
	}
	cut := limit - 3
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	if cut <= 0 {
		return "..."
	}
	return value[:cut] + "..."
}
