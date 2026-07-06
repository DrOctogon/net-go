// processor/keywords.go
// Keyword flagging for transcripts. After a clip is transcribed, the transcript
// is matched against the user-configured keyword list. Matching is word-boundary
// based and case-insensitive by default. This is intentionally allocation-light
// and runs off the hot path (inside the transcription job), so per-call regex
// compilation is acceptable.

package processor

import (
	"regexp"
	"strings"
)

// matchKeywords returns the subset of keywords that appear in transcript as
// whole-word matches, preserving the order keywords were configured in and
// deduplicating repeats. Matching is case-insensitive unless caseSensitive is
// true. An empty keyword list or empty transcript yields no matches (no-op).
//
// Blank (whitespace-only) keyword entries are skipped defensively; config
// validation already rejects them, but the matcher must never panic on bad
// input or treat a blank keyword as matching everything.
func matchKeywords(transcript string, keywords []string, caseSensitive bool) []string {
	if len(keywords) == 0 || strings.TrimSpace(transcript) == "" {
		return nil
	}

	hits := make([]string, 0, len(keywords))
	seen := make(map[string]struct{}, len(keywords))

	for _, kw := range keywords {
		trimmed := strings.TrimSpace(kw)
		if trimmed == "" {
			continue
		}
		// Deduplicate on the case-folded form when matching case-insensitively
		// so "Fire" and "fire" don't both appear in the hit list.
		dedupKey := trimmed
		if !caseSensitive {
			dedupKey = strings.ToLower(trimmed)
		}
		if _, ok := seen[dedupKey]; ok {
			continue
		}

		if keywordMatches(transcript, trimmed, caseSensitive) {
			seen[dedupKey] = struct{}{}
			hits = append(hits, trimmed)
		}
	}

	if len(hits) == 0 {
		return nil
	}
	return hits
}

// keywordMatches reports whether keyword appears in transcript on word
// boundaries. The keyword is escaped so regex metacharacters are treated
// literally. \b only anchors at word/non-word transitions, which is the desired
// behavior for alphanumeric keywords and phrases.
func keywordMatches(transcript, keyword string, caseSensitive bool) bool {
	var sb strings.Builder
	if !caseSensitive {
		sb.WriteString("(?i)")
	}
	sb.WriteString(`\b`)
	sb.WriteString(regexp.QuoteMeta(keyword))
	sb.WriteString(`\b`)

	re, err := regexp.Compile(sb.String())
	if err != nil {
		// A keyword that cannot be compiled into a boundary regex (should not
		// happen after QuoteMeta) is treated as a non-match rather than failing
		// the whole transcription job.
		return false
	}
	return re.MatchString(transcript)
}

// joinKeywords renders a hit list into the comma-joined form stored on the
// detection (datastore.Note.KeywordsHit).
func joinKeywords(hits []string) string {
	return strings.Join(hits, ",")
}
