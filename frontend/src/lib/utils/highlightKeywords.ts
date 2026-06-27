/**
 * highlightKeywords.ts
 *
 * Splits a transcript string into contiguous segments, marking which segments
 * match one of the provided keywords.  Used for segment-based rendering in
 * Svelte so that matched spans can be wrapped in a <mark> element without
 * resorting to {@html} (which would violate the XSS / ast:security rules).
 */

export interface TranscriptSegment {
  text: string;
  match: boolean;
}

/** Maximum number of keywords accepted to prevent catastrophic regex. */
const MAX_KEYWORDS = 100;

/**
 * Escapes all regex metacharacters in `s` so it can be embedded verbatim in a
 * RegExp pattern.  A keyword like "fire." is treated as the literal string
 * "fire." rather than "fire" followed by any character.
 */
function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/**
 * Splits `transcript` into an array of non-overlapping, contiguous segments.
 *
 * Guarantees:
 *  - `segments.map(s => s.text).join('') === transcript` (round-trip)
 *  - Newlines and all whitespace in the original string are preserved
 *  - Matching is word-boundary, case-insensitive (mirrors backend matchKeywords)
 *  - Regex metacharacters in keyword strings are treated as literals
 *  - Empty/undefined keywords or empty transcript return a safe fallback
 *  - Never throws
 */
export function highlightKeywords(
  transcript: string,
  keywords: string[] | undefined
): TranscriptSegment[] {
  if (!transcript) return [];

  // Cap, trim, and filter blank keywords before any further processing.
  const raw = (keywords ?? [])
    .slice(0, MAX_KEYWORDS)
    .map(k => k.trim())
    .filter(k => k.length > 0);

  if (raw.length === 0) {
    return [{ text: transcript, match: false }];
  }

  // De-duplicate keywords (case-insensitive) to keep the regex manageable.
  const seen = new Set<string>();
  const unique = raw.filter(k => {
    const lower = k.toLowerCase();
    if (seen.has(lower)) return false;
    seen.add(lower);
    return true;
  });

  // Build one combined alternation: each keyword is wrapped in \b…\b so we
  // match whole words only, and regex metacharacters are escaped.
  const pattern = unique.map(k => `\\b${escapeRegExp(k)}\\b`).join('|');
  // eslint-disable-next-line security/detect-non-literal-regexp -- pattern is built only from escapeRegExp'd literal keywords joined by \b anchors; no user-controlled quantifiers, so it is injection/ReDoS-safe.
  const regex = new RegExp(pattern, 'gi');

  const segments: TranscriptSegment[] = [];
  let lastIndex = 0;
  let m: RegExpExecArray | null;

  while ((m = regex.exec(transcript)) !== null) {
    const start = m.index;
    const end = start + m[0].length;

    // Text between the previous match end and this match start.
    if (start > lastIndex) {
      segments.push({ text: transcript.slice(lastIndex, start), match: false });
    }

    segments.push({ text: transcript.slice(start, end), match: true });
    lastIndex = end;

    // Guard against zero-length match infinite loop (shouldn't happen with \b
    // patterns, but is good defensive practice).
    if (m[0].length === 0) {
      regex.lastIndex++;
    }
  }

  // Remainder after the last match.
  if (lastIndex < transcript.length) {
    segments.push({ text: transcript.slice(lastIndex), match: false });
  }

  return segments;
}
