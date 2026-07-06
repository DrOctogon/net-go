/**
 * Species name normalization utility.
 *
 * The bidirectional common/scientific name lookup that previously lived here was
 * built from the all-species labels endpoint, which was removed in the
 * human-voice pivot. Only the shared normalization helper remains, still used by
 * the settings species-prediction picker and the (parked) species dictionary
 * store.
 */

/**
 * Normalize a name for case- and Unicode-form-insensitive lookup. Mirrors the
 * backend normalizeForLookup (strings.ToLower(norm.NFC.String(s))): labels ship
 * as NFC, but composing keyboards (macOS) submit NFD bytes for diacritics, so both
 * sides are normalized to NFC before lowercasing to prevent silent misses.
 */
export function normalizeForLookup(s: string): string {
  return s.normalize('NFC').toLowerCase();
}
