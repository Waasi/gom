// Package inflect converts between the naming forms gom generates: model type
// names, field names, package names, collections and bson keys.
package inflect

import (
	"strings"
	"unicode"
)

var irregular = map[string]string{
	"person": "people",
	"child":  "children",
	"man":    "men",
	"woman":  "women",
	"mouse":  "mice",
	"goose":  "geese",
	"foot":   "feet",
	"tooth":  "teeth",
	"leaf":   "leaves",
	"life":   "lives",
	"knife":  "knives",
	"wife":   "wives",
	"half":   "halves",
	"index":  "indexes",
	"matrix": "matrices",
	"vertex": "vertices",
	"quiz":   "quizzes",
	"ox":     "oxen",
}

var uncountable = map[string]bool{
	"data": true, "metadata": true, "info": true, "information": true,
	"news": true, "equipment": true, "series": true, "species": true,
	"media": true, "feedback": true, "sheep": true, "fish": true,
}

var singularIrregular = func() map[string]string {
	m := make(map[string]string, len(irregular))
	for k, v := range irregular {
		m[v] = k
	}
	return m
}()

// initialisms are rendered upper-case in Go identifiers, as golint expects.
var initialisms = map[string]bool{
	"id": true, "url": true, "uri": true, "api": true, "http": true, "https": true,
	"json": true, "xml": true, "html": true, "ip": true, "sql": true, "ssh": true,
	"uuid": true, "ui": true, "ttl": true, "acl": true, "sms": true, "cdn": true,
	"dns": true, "jwt": true, "smtp": true, "tls": true, "ssl": true, "cpu": true,
}

// Words splits an identifier in any common form (snake, kebab, camel, Pascal,
// with acronyms) into lower-case words: "HTTPServerID" → [http server id].
func Words(s string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			flush()
		case unicode.IsUpper(r):
			// Start a word at a lower→upper boundary, or at the last capital of
			// an acronym that runs into a lower-case word (the "S" in "HTTPServer").
			if len(cur) > 0 {
				prev := rs[i-1]
				nextLower := i+1 < len(rs) && unicode.IsLower(rs[i+1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
					flush()
				}
			}
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

func title(w string) string {
	if strings.HasSuffix(w, "s") && initialisms[w[:len(w)-1]] {
		return strings.ToUpper(w[:len(w)-1]) + "s" // ids → IDs, urls → URLs
	}
	if initialisms[w] {
		return strings.ToUpper(w)
	}
	if w == "" {
		return w
	}
	r := []rune(w)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// Pascal renders an exported Go identifier: "company_id" → "CompanyID".
func Pascal(s string) string {
	var b strings.Builder
	for _, w := range Words(s) {
		b.WriteString(title(w))
	}
	return b.String()
}

// Camel renders a bson/json key: "company_id" → "companyId". Initialisms are
// not upper-cased here, matching keys such as projectId.
func Camel(s string) string {
	var b strings.Builder
	for i, w := range Words(s) {
		if i == 0 {
			b.WriteString(w)
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	return b.String()
}

// Snake renders snake_case: "InvoiceItem" → "invoice_item".
func Snake(s string) string { return strings.Join(Words(s), "_") }

// Lower renders words joined in lower case, the Go package style:
// "rate_limits" → "ratelimits".
func Lower(s string) string { return strings.Join(Words(s), "") }

// Pluralize pluralizes the last word of s, keeping s's separators and case
// style for the rest: "invoice_item" → "invoice_items", "person" → "people".
func Pluralize(s string) string { return mapLast(s, pluralWord) }

// Singularize is the inverse of Pluralize.
func Singularize(s string) string { return mapLast(s, singularWord) }

func mapLast(s string, f func(string) string) string {
	ws := Words(s)
	if len(ws) == 0 {
		return s
	}
	ws[len(ws)-1] = f(ws[len(ws)-1])
	return strings.Join(ws, "_")
}

func pluralWord(w string) string {
	if uncountable[w] {
		return w
	}
	if p, ok := irregular[w]; ok {
		return p
	}
	if _, ok := singularIrregular[w]; ok {
		return w // already plural
	}
	switch {
	case strings.HasSuffix(w, "y") && len(w) > 1 && !isVowel(w[len(w)-2]):
		return w[:len(w)-1] + "ies"
	case strings.HasSuffix(w, "s"), strings.HasSuffix(w, "x"), strings.HasSuffix(w, "z"),
		strings.HasSuffix(w, "ch"), strings.HasSuffix(w, "sh"):
		return w + "es"
	}
	return w + "s"
}

func singularWord(w string) string {
	if uncountable[w] {
		return w
	}
	if s, ok := singularIrregular[w]; ok {
		return s
	}
	if _, ok := irregular[w]; ok {
		return w // already singular
	}
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 3:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "sses"), strings.HasSuffix(w, "shes"), strings.HasSuffix(w, "ches"),
		strings.HasSuffix(w, "xes"), strings.HasSuffix(w, "zes"), strings.HasSuffix(w, "uses"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "ss"), strings.HasSuffix(w, "us"), strings.HasSuffix(w, "is"):
		return w
	case strings.HasSuffix(w, "s") && len(w) > 1:
		return w[:len(w)-1]
	}
	return w
}

func isVowel(c byte) bool { return strings.IndexByte("aeiou", c) >= 0 }
