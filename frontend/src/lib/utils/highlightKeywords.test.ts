import { describe, it, expect } from 'vitest';
import { highlightKeywords } from './highlightKeywords';
import type { TranscriptSegment } from './highlightKeywords';

// Helper: reassemble segments back into original string
function join(segs: TranscriptSegment[]): string {
  return segs.map(s => s.text).join('');
}

describe('highlightKeywords', () => {
  // -----------------------------------------------------------------------
  // Empty / degenerate inputs
  // -----------------------------------------------------------------------

  it('returns empty array for empty transcript', () => {
    expect(highlightKeywords('', ['fire'])).toEqual([]);
  });

  it('returns single no-match segment when keywords is undefined', () => {
    const text = 'hello world';
    const segs = highlightKeywords(text, undefined);
    expect(segs).toEqual([{ text, match: false }]);
  });

  it('returns single no-match segment when keywords is empty array', () => {
    const text = 'hello world';
    const segs = highlightKeywords(text, []);
    expect(segs).toEqual([{ text, match: false }]);
  });

  it('returns single no-match segment when all keywords are blank strings', () => {
    const text = 'hello world';
    const segs = highlightKeywords(text, ['  ', '']);
    expect(segs).toEqual([{ text, match: false }]);
  });

  // -----------------------------------------------------------------------
  // Single keyword match
  // -----------------------------------------------------------------------

  it('highlights a single keyword in the middle of text', () => {
    const segs = highlightKeywords('call the police now', ['police']);
    expect(segs).toEqual([
      { text: 'call the ', match: false },
      { text: 'police', match: true },
      { text: ' now', match: false },
    ]);
  });

  it('highlights a keyword at the start of text', () => {
    const segs = highlightKeywords('fire in the building', ['fire']);
    expect(segs).toEqual([
      { text: 'fire', match: true },
      { text: ' in the building', match: false },
    ]);
  });

  it('highlights a keyword at the end of text', () => {
    const segs = highlightKeywords('we called for help', ['help']);
    expect(segs).toEqual([
      { text: 'we called for ', match: false },
      { text: 'help', match: true },
    ]);
  });

  it('highlights a keyword that is the entire transcript', () => {
    const segs = highlightKeywords('police', ['police']);
    expect(segs).toEqual([{ text: 'police', match: true }]);
  });

  // -----------------------------------------------------------------------
  // Multiple keywords
  // -----------------------------------------------------------------------

  it('highlights multiple distinct keywords', () => {
    const segs = highlightKeywords('call the police and fire department', ['police', 'fire']);
    const matchTexts = segs.filter(s => s.match).map(s => s.text);
    expect(matchTexts).toContain('police');
    expect(matchTexts).toContain('fire');
    // Non-matching text must not be marked
    segs
      .filter(s => !s.match)
      .forEach(s => {
        expect(['police', 'fire']).not.toContain(s.text.trim());
      });
  });

  it('handles multiple occurrences of the same keyword', () => {
    const segs = highlightKeywords('help me please help', ['help']);
    const matchSegs = segs.filter(s => s.match);
    expect(matchSegs.length).toBe(2);
    matchSegs.forEach(s => expect(s.text.toLowerCase()).toBe('help'));
  });

  // -----------------------------------------------------------------------
  // Case-insensitivity
  // -----------------------------------------------------------------------

  it('matches keyword regardless of case in transcript', () => {
    const segs = highlightKeywords('POLICE are here', ['police']);
    expect(segs.some(s => s.match && s.text === 'POLICE')).toBe(true);
  });

  it('matches keyword regardless of case in keyword list', () => {
    const segs = highlightKeywords('police are here', ['POLICE']);
    expect(segs.some(s => s.match && s.text === 'police')).toBe(true);
  });

  it('de-duplicates case-insensitive keywords', () => {
    // 'fire' and 'FIRE' should behave as a single keyword
    const segs = highlightKeywords('fire alarm', ['fire', 'FIRE']);
    const matchCount = segs.filter(s => s.match).length;
    expect(matchCount).toBe(1);
  });

  // -----------------------------------------------------------------------
  // Word-boundary enforcement (no partial-word match)
  // -----------------------------------------------------------------------

  it('does not highlight "cat" inside "category"', () => {
    const segs = highlightKeywords('browse the category list', ['cat']);
    expect(segs.filter(s => s.match).length).toBe(0);
  });

  it('does not highlight "fire" inside "fireplace"', () => {
    const segs = highlightKeywords('warm by the fireplace', ['fire']);
    expect(segs.filter(s => s.match).length).toBe(0);
  });

  it('highlights exact word "fire" when surrounded by spaces', () => {
    const segs = highlightKeywords('fire is dangerous', ['fire']);
    expect(segs.some(s => s.match && s.text === 'fire')).toBe(true);
  });

  it('highlights keyword that appears before punctuation', () => {
    // Word boundary exists between 'p' and '!'
    const segs = highlightKeywords('please help!', ['help']);
    expect(segs.some(s => s.match && s.text === 'help')).toBe(true);
  });

  // -----------------------------------------------------------------------
  // Regex metacharacter keyword treated literally
  // -----------------------------------------------------------------------

  it('treats "fire." as a literal string — does not match "firex"', () => {
    // Without escaping, the dot in "fire." would match any character
    const segs = highlightKeywords('firex in building', ['fire.']);
    expect(segs.filter(s => s.match).length).toBe(0);
  });

  it('does not throw for keyword containing (parentheses)', () => {
    expect(() => highlightKeywords('call (police) now', ['(police)'])).not.toThrow();
  });

  it('does not throw for keyword containing brackets []', () => {
    expect(() => highlightKeywords('test [value] here', ['[value]'])).not.toThrow();
  });

  it('does not throw for keyword with plus, star, question mark', () => {
    expect(() => highlightKeywords('value+extra', ['value+'])).not.toThrow();
    expect(() => highlightKeywords('value*extra', ['value*'])).not.toThrow();
    expect(() => highlightKeywords('value?extra', ['value?'])).not.toThrow();
  });

  // -----------------------------------------------------------------------
  // Round-trip: concatenated segments === original input
  // -----------------------------------------------------------------------

  it('round-trip: segments reconstruct the original transcript exactly', () => {
    const transcript = 'the police arrived and fire trucks followed';
    expect(join(highlightKeywords(transcript, ['police', 'fire']))).toBe(transcript);
  });

  it('round-trip with no matches', () => {
    const transcript = 'nothing interesting here';
    expect(join(highlightKeywords(transcript, ['police']))).toBe(transcript);
  });

  it('round-trip with match at very start and very end', () => {
    const transcript = 'police arrived at the scene and called for help';
    expect(join(highlightKeywords(transcript, ['police', 'help']))).toBe(transcript);
  });

  it('round-trip with empty keywords', () => {
    const transcript = 'just some text';
    expect(join(highlightKeywords(transcript, []))).toBe(transcript);
  });

  // -----------------------------------------------------------------------
  // Newline and whitespace preservation
  // -----------------------------------------------------------------------

  it('preserves newlines in non-matching segments', () => {
    const transcript = 'first line\nsecond line with police\nthird line';
    expect(join(highlightKeywords(transcript, ['police']))).toBe(transcript);
  });

  it('preserves leading and trailing whitespace', () => {
    const transcript = '  fire in the building  ';
    expect(join(highlightKeywords(transcript, ['fire']))).toBe(transcript);
  });

  it('preserves tabs and mixed whitespace', () => {
    const transcript = 'help\there\nand police\tthere';
    expect(join(highlightKeywords(transcript, ['help', 'police']))).toBe(transcript);
  });

  it('correctly marks keyword surrounded by newlines', () => {
    const transcript = 'before\npolice\nafter';
    const segs = highlightKeywords(transcript, ['police']);
    expect(segs.some(s => s.match && s.text === 'police')).toBe(true);
    expect(join(segs)).toBe(transcript);
  });

  // -----------------------------------------------------------------------
  // Keyword cap (defensive)
  // -----------------------------------------------------------------------

  it('accepts exactly 100 keywords without error', () => {
    const keywords = Array.from({ length: 100 }, (_, i) => `word${i}`);
    expect(() => highlightKeywords('word0 word1 word99', keywords)).not.toThrow();
  });

  it('silently ignores keywords beyond the 100-item cap', () => {
    // word100 is the 101st entry — it must not be highlighted
    const keywords = Array.from({ length: 101 }, (_, i) => `word${i}`);
    const segs = highlightKeywords('word100 in transcript', keywords);
    expect(segs.filter(s => s.match).every(s => s.text !== 'word100')).toBe(true);
  });
});
