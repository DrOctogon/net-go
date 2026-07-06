/**
 * Speaker-attribute helpers for VoiceWatch.
 *
 * Speaker attributes (estimated gender + relative age band) are an opt-in,
 * privacy-first feature. The backend omits the fields entirely when there is no
 * estimate, so an absent/empty value means "no estimate" and the corresponding
 * chip must be hidden. These are demographic *estimates*, not identity
 * recognition.
 *
 * This module is intentionally i18n-free and side-effect-free so it can be unit
 * tested in isolation: it maps raw detection values to structured chip data
 * (canonical value, i18n label key, formatted confidence). Components resolve
 * the label key via `t()` and render the chip.
 */

import type { Detection } from '$lib/types/detection.types';

export type SpeakerGender = 'male' | 'female' | 'unknown';
export type SpeakerAgeBand = 'child' | 'teen' | 'adult' | 'senior';

// Recognized values, in display order. Used to validate raw values and to build
// the search-filter dropdowns so the UI cannot drift from the accepted set.
export const SPEAKER_GENDERS: readonly SpeakerGender[] = ['male', 'female', 'unknown'];
export const SPEAKER_AGE_BANDS: readonly SpeakerAgeBand[] = ['child', 'teen', 'adult', 'senior'];

const GENDER_SET = new Set<string>(SPEAKER_GENDERS);
const AGE_SET = new Set<string>(SPEAKER_AGE_BANDS);

export interface SpeakerChipData {
  kind: 'gender' | 'age';
  value: string; // canonical lowercased value (e.g. "female")
  labelKey: string; // i18n key for the human-readable label
  confidencePct: number | null; // rounded 0..100, or null when no usable confidence
}

/**
 * Convert a 0..1 model confidence into a 0..100 integer percentage.
 * Returns null for absent / non-finite / non-positive values: the backend omits
 * the confidence field when there is no estimate, so 0 or undefined means
 * "no confidence to show".
 */
export function toConfidencePct(confidence?: number): number | null {
  if (confidence == null || !Number.isFinite(confidence) || confidence <= 0) {
    return null;
  }
  return Math.round(Math.min(confidence, 1) * 100);
}

/** Build chip data for an estimated speaker gender, or null when there is no estimate. */
export function getGenderChip(gender?: string, confidence?: number): SpeakerChipData | null {
  if (!gender) return null;
  const value = gender.toLowerCase();
  if (!GENDER_SET.has(value)) return null;
  return {
    kind: 'gender',
    value,
    labelKey: `detections.speaker.gender.${value}`,
    confidencePct: toConfidencePct(confidence),
  };
}

/** Build chip data for an estimated age band, or null when there is no estimate. */
export function getAgeChip(ageBand?: string, confidence?: number): SpeakerChipData | null {
  if (!ageBand) return null;
  const value = ageBand.toLowerCase();
  if (!AGE_SET.has(value)) return null;
  return {
    kind: 'age',
    value,
    labelKey: `detections.speaker.age.${value}`,
    confidencePct: toConfidencePct(confidence),
  };
}

/**
 * Collect the speaker-attribute chips for a detection. Returns an empty array
 * when the detection has no estimates (the common case), so callers can simply
 * skip rendering when the result is empty.
 */
export function getSpeakerChips(
  detection: Pick<Detection, 'gender' | 'genderConfidence' | 'ageBand' | 'ageConfidence'>
): SpeakerChipData[] {
  const chips: SpeakerChipData[] = [];
  const genderChip = getGenderChip(detection.gender, detection.genderConfidence);
  if (genderChip) chips.push(genderChip);
  const ageChip = getAgeChip(detection.ageBand, detection.ageConfidence);
  if (ageChip) chips.push(ageChip);
  return chips;
}
