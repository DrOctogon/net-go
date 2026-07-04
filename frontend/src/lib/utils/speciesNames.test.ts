import { describe, it, expect } from 'vitest';
import { normalizeForLookup } from './speciesNames';

describe('normalizeForLookup', () => {
  it('lowercases input', () => {
    expect(normalizeForLookup('Tawny Owl')).toBe('tawny owl');
  });

  it('normalizes NFD input to NFC before comparison', () => {
    // U+00E4 is precomposed NFC 'ä'; the decomposed NFD form is 'a' + U+0308.
    const nfc = 'ä';
    const nfd = 'ä';
    expect(normalizeForLookup(nfd)).toBe(normalizeForLookup(nfc));
  });

  it('returns an empty string unchanged', () => {
    expect(normalizeForLookup('')).toBe('');
  });
});
