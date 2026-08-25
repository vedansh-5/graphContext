package store

import (
	"fmt"
	"strings"
	"unicode"
)

// SplitIdentifier breaks a programming identifier into lowercase subword tokens.
// It handles snake_case, camelCase, PascalCase, kebab-case, dotted qualified
// names, and acronym runs.
//
//	checkRateLimit      -> [check rate limit]
//	check_rate_limit    -> [check rate limit]
//	parseHTTPResponse   -> [parse http response]
//	AuthService.login   -> [auth service login]
//
// This is what makes FTS5 useful without a custom tokenizer: SQLite's default
// unicode61 tokenizer does no subword splitting, so we split at write time.
func SplitIdentifier(id string) []string {
	var tokens []string
	for _, run := range strings.FieldsFunc(id, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		tokens = append(tokens, splitCase(run)...)
	}
	return tokens
}

// splitCase splits a single alphanumeric run on case boundaries.
func splitCase(run string) []string {
	rs := []rune(run)
	if len(rs) == 0 {
		return nil
	}
	var out []string
	start := 0
	for i := 1; i < len(rs); i++ {
		prev, cur := rs[i-1], rs[i]
		boundary := false
		switch {
		// lower|digit -> Upper  ("checkRate" -> check|Rate)
		case !unicode.IsUpper(prev) && unicode.IsUpper(cur):
			boundary = true
		// Upper -> Upper followed by lower  ("HTTPResponse" -> HTTP|Response)
		case unicode.IsUpper(prev) && unicode.IsUpper(cur) &&
			i+1 < len(rs) && unicode.IsLower(rs[i+1]):
			boundary = true
		}
		if boundary {
			out = append(out, strings.ToLower(string(rs[start:i])))
			start = i
		}
	}
	out = append(out, strings.ToLower(string(rs[start:])))
	return out
}

// ftsTokens builds the searchable token string for a node: the original name and
// qualified name plus all of their subword tokens, so both "checkRateLimit" and
// "rate limit" match.
func ftsTokens(n Node) string {
	seen := map[string]bool{}
	var parts []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[strings.ToLower(s)] {
			return
		}
		seen[strings.ToLower(s)] = true
		parts = append(parts, s)
	}
	add(n.Name)
	add(n.QualifiedName)
	for _, t := range SplitIdentifier(n.Name) {
		add(t)
	}
	for _, t := range SplitIdentifier(n.QualifiedName) {
		add(t)
	}
	return strings.Join(parts, " ")
}

// SearchHit is one full-text search result.
type SearchHit struct {
	NodeID string
	Rank   float64
}

// Search runs an FTS5 MATCH over symbol tokens and docstrings.
// Query terms are treated as literals, so caller input never breaks FTS syntax.
func (s *Store) Search(query string, limit int) ([]SearchHit, error) {
	terms := SplitIdentifier(query)
	if len(terms) == 0 {
		return nil, nil
	}
	// Quote each term so FTS5 operators in user input are inert.
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + t + `"`
	}
	match := strings.Join(quoted, " ")

	if limit <= 0 {
		limit = 20
	}
	rows, err := s.conn.Query(
		`SELECT node_id, rank FROM symbols_fts WHERE symbols_fts MATCH ?
		 ORDER BY rank LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search %q: %w", query, err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.NodeID, &h.Rank); err != nil {
			return nil, fmt.Errorf("scan search hit: %w", err)
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
