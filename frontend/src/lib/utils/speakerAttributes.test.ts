import { describe, it, expect } from 'vitest';
import { toConfidencePct, getGenderChip, getAgeChip, getSpeakerChips } from './speakerAttributes';

describe('toConfidencePct', () => {
  it('converts a 0..1 confidence into a rounded percentage', () => {
    expect(toConfidencePct(0.87)).toBe(87);
    expect(toConfidencePct(0.873)).toBe(87);
    expect(toConfidencePct(0.875)).toBe(88);
    expect(toConfidencePct(1)).toBe(100);
  });

  it('returns null for absent / zero / non-finite confidence', () => {
    expect(toConfidencePct(undefined)).toBeNull();
    expect(toConfidencePct(0)).toBeNull();
    expect(toConfidencePct(Number.NaN)).toBeNull();
    expect(toConfidencePct(-0.5)).toBeNull();
  });

  it('clamps values above 1 to 100%', () => {
    expect(toConfidencePct(1.5)).toBe(100);
  });
});

describe('getGenderChip', () => {
  it('builds chip data for a recognized gender, lowercasing the value', () => {
    expect(getGenderChip('Female', 0.9)).toEqual({
      kind: 'gender',
      value: 'female',
      labelKey: 'detections.speaker.gender.female',
      confidencePct: 90,
    });
  });

  it('returns null for empty / undefined / unrecognized values', () => {
    expect(getGenderChip(undefined)).toBeNull();
    expect(getGenderChip('')).toBeNull();
    expect(getGenderChip('robot', 0.9)).toBeNull();
  });

  it('keeps a null confidence when none is provided', () => {
    expect(getGenderChip('male')?.confidencePct).toBeNull();
  });
});

describe('getAgeChip', () => {
  it('builds chip data for a recognized age band', () => {
    expect(getAgeChip('adult', 0.75)).toEqual({
      kind: 'age',
      value: 'adult',
      labelKey: 'detections.speaker.age.adult',
      confidencePct: 75,
    });
  });

  it('returns null for empty / unrecognized values', () => {
    expect(getAgeChip('')).toBeNull();
    expect(getAgeChip('elderly', 0.8)).toBeNull();
  });
});

describe('getSpeakerChips', () => {
  it('returns both chips when gender and age are present', () => {
    const chips = getSpeakerChips({
      gender: 'female',
      genderConfidence: 0.87,
      ageBand: 'adult',
      ageConfidence: 0.62,
    });
    expect(chips).toHaveLength(2);
    expect(chips[0].kind).toBe('gender');
    expect(chips[1].kind).toBe('age');
  });

  it('omits a chip whose value is empty or absent', () => {
    expect(getSpeakerChips({ gender: 'male', genderConfidence: 0.9 })).toHaveLength(1);
    expect(getSpeakerChips({ ageBand: 'teen' })).toHaveLength(1);
  });

  it('returns an empty array when there are no estimates', () => {
    expect(getSpeakerChips({})).toEqual([]);
    expect(getSpeakerChips({ gender: '', ageBand: '' })).toEqual([]);
  });
});
