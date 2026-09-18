package compact

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// excerpt uses literal lines. Omission markers are never presented as source.
func excerpt(text, goal string, limit int) string {
	if len(text) <= limit {
		return text
	}
	lines := strings.Split(text, "\n")
	terms := strings.FieldsFunc(strings.ToLower(goal), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	type ranked struct{ index, score int }
	ranks := make([]ranked, len(lines))
	for i, line := range lines {
		score := 0
		if i < 2 || i >= len(lines)-2 {
			score += 2
		}
		lower := strings.ToLower(line)
		for _, key := range []string{"error", "failed", "panic", "exception", "fatal", "hata"} {
			if strings.Contains(lower, key) {
				score += 8
			}
		}
		for _, term := range terms {
			if len(term) >= 3 && strings.Contains(lower, term) {
				score += 3
			}
		}
		ranks[i] = ranked{index: i, score: score}
	}
	slices.SortStableFunc(ranks, func(a, b ranked) int { return b.score - a.score })
	selected := make(map[int]string)
	remaining := max(0, limit-64)
	for _, rank := range ranks {
		if remaining < 32 {
			break
		}
		line := lines[rank.index]
		piece := clip(line, min(remaining-24, max(80, limit/4)))
		if len(piece) < len(line) {
			piece += " [line truncated]"
		}
		selected[rank.index] = fmt.Sprintf("L%d: %s", rank.index+1, piece)
		remaining -= len(selected[rank.index]) + 1
	}
	var b strings.Builder
	b.WriteString("[literal excerpts; other text omitted]\n")
	for i := range lines {
		if line, ok := selected[i]; ok {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func clip(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	end := max(0, limit)
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end]
}
