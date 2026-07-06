package processor

import (
	"strings"
	"time"

	"github.com/tphakala/voicewatch/internal/logger"
	"github.com/tphakala/voicewatch/internal/openfauna"
)

// Extended capture timeout thresholds.
const (
	extendedCaptureMinInitialWait  = 15 * time.Second
	extendedCaptureMediumThreshold = 30 * time.Second
	extendedCaptureMediumWait      = 30 * time.Second
	extendedCaptureLongThreshold   = 2 * time.Minute
	extendedCaptureLongWait        = 60 * time.Second
)

// RebuildExtendedCaptureFilter re-applies the extended capture state from the
// current settings. This is called by the control monitor when ExtendedCapture
// settings (Enabled, MaxDuration) change at runtime.
func (p *Processor) RebuildExtendedCaptureFilter() {
	p.initExtendedCapture()
}

// initExtendedCapture logs the extended capture state at startup. Extended
// capture is global: when enabled it applies to every detection, so there is
// no species list to resolve. Called from Processor.New(). Safe to re-call on
// settings refresh.
func (p *Processor) initExtendedCapture() {
	settings := p.currentSettings()

	if !settings.Realtime.ExtendedCapture.Enabled {
		return
	}

	GetLogger().Info("Extended capture enabled for all detections",
		logger.Int("max_duration_seconds", settings.Realtime.ExtendedCapture.MaxDuration),
		logger.String("operation", "extended_capture_init"))
}

// isExtendedCaptureEnabled reports whether extended capture applies. Extended
// capture is global: every detection qualifies when the feature is enabled.
func (p *Processor) isExtendedCaptureEnabled() bool {
	return p.currentSettings().Realtime.ExtendedCapture.Enabled
}

// resolveSpeciesFilter resolves the config species list into a set of scientific names.
// Returns (isAll, resolvedSet) where isAll=true means all species qualify.
func resolveSpeciesFilter(configSpecies, labels []string, locale, operationName string) (isAll bool, resolvedSet map[string]bool) {
	if len(configSpecies) == 0 {
		return true, nil
	}

	resolved := make(map[string]bool)

	// Build common name -> scientific name lookup from the model labels.
	commonToScientific := make(map[string]string)
	scientificNames := make(map[string]bool)
	for _, label := range labels {
		if sci, common, found := strings.Cut(label, "_"); found {
			sciLower := strings.ToLower(sci)
			commonLower := strings.ToLower(common)
			commonToScientific[commonLower] = sciLower
			scientificNames[sciLower] = true
		} else if sci := strings.ToLower(strings.TrimSpace(label)); sci != "" {
			// Scientific-only labels (bats, Perch-unique species) have no embedded
			// common name; index them by scientific name so a scientific-name config
			// entry still matches them.
			scientificNames[sci] = true
		}
	}

	// Config entries that none of the cheap lookups resolved; reverse-resolved
	// through OpenFauna in a single batch pass after the loop.
	var unresolved []string

	for _, entry := range configSpecies {
		entryLower := strings.ToLower(strings.TrimSpace(entry))

		// Try as scientific name first (cheap map lookup, no side effects)
		if scientificNames[entryLower] {
			resolved[entryLower] = true
			continue
		}

		// Try as common name (cheap map lookup, no side effects)
		if sci, ok := commonToScientific[entryLower]; ok {
			resolved[sci] = true
			continue
		}

		// Defer to the OpenFauna reverse lookup below.
		unresolved = append(unresolved, entry)
	}

	// Reverse-resolve any still-unresolved entries through OpenFauna in a single
	// cold-path pass. This canonicalizes localized common names of secondary-model
	// species (e.g. Finnish "mopsilepakko" -> "Barbastella barbastellus") that have
	// no embedded common name in the model labels and are not in the taxonomy DB.
	if len(unresolved) > 0 {
		// The shared helper returns scientific names already lower-cased, keyed per
		// entry so the per-entry "matched" tracking (and the unresolved warning below)
		// still works; it centralizes the lower-casing/locale handling shared with the
		// range-filter exclude matcher.
		reverse := openfauna.ReverseResolveToScientificNames(unresolved, locale)
		stillUnresolved := unresolved[:0]
		for _, entry := range unresolved {
			matched := false
			for _, sci := range reverse[entry] {
				// Only resolve to species a loaded model can actually emit. OpenFauna
				// may return scientific names for the localized common name that are
				// not in any loaded model's labels; resolving to those would silently
				// match a species nothing can detect and skip the unresolved warning.
				if !scientificNames[sci] {
					continue
				}
				resolved[sci] = true
				matched = true
			}
			if !matched {
				stillUnresolved = append(stillUnresolved, entry)
			}
		}
		unresolved = stillUnresolved
	}

	// Anything still unresolved is a likely config typo; warn so users can spot it.
	for _, entry := range unresolved {
		GetLogger().Warn("Species filter entry not resolved",
			logger.String("entry", entry),
			logger.String("operation", operationName+"_species_filter"))
	}

	return false, resolved
}

// normalizeDetectionTimes sets BeginTime/EndTime on an approved detection.
// BeginTime is always backdated to FirstDetected so the audio clip starts from
// the beginning of the event. EndTime handling depends on capture mode:
//   - Extended captures: EndTime = LastUpdated + normal detection window (spans full session)
//   - Normal captures: EndTime = FirstDetected + normal detection window (configured length)
//
// For extended captures the clip name is regenerated to reflect the actual duration.
func (p *Processor) normalizeDetectionTimes(item *PendingDetection) {
	settings := p.currentSettings()
	item.Detection.Result.BeginTime = item.FirstDetected

	captureLength := time.Duration(settings.Realtime.Audio.Export.Length) * time.Second
	preCaptureLength := time.Duration(settings.Realtime.Audio.Export.PreCapture) * time.Second
	normalDetectionWindow := max(time.Duration(0), captureLength-preCaptureLength)

	if item.ExtendedCapture {
		// For extended captures, EndTime reflects the last detection + normal detection window.
		// LastUpdated is always initialized (set on creation and every re-detection).
		item.Detection.Result.EndTime = item.LastUpdated.Add(normalDetectionWindow)

		// Regenerate clip name with actual duration (unknown at createDetection time)
		preCapture := settings.Realtime.Audio.Export.PreCapture
		durationSeconds := int(item.Detection.Result.EndTime.Sub(item.Detection.Result.BeginTime).Seconds()) + preCapture
		item.Detection.Result.ClipName = p.generateClipNameWithDuration(
			settings,
			item.Detection.Result.Species.ScientificName,
			float32(item.Confidence),
			durationSeconds,
			item.Detection.Result.Timestamp,
		)
	} else {
		// For non-extended detections, recalculate EndTime to maintain the configured
		// capture window. BeginTime was backdated to FirstDetected, but EndTime may
		// come from a later higher-confidence detection that replaced the Detection
		// struct during the pending window. Without this recalculation, the time span
		// (EndTime - BeginTime) inflates beyond the configured capture length.
		item.Detection.Result.EndTime = item.FirstDetected.Add(normalDetectionWindow)
	}
}

// applyExtendedCapture applies extended capture logic to a pending detection.
// It sets the ExtendedCapture flag, MaxDeadline, and calculates the scaled flush deadline.
// This is called from processDetections after the pending detection is created/updated.
// Must be called while pendingMutex is held.
func (p *Processor) applyExtendedCapture(mapKey string, now time.Time, normalDetectionWindow time.Duration) {
	settings := p.currentSettings()
	item := p.pendingDetections[mapKey]
	maxDuration := time.Duration(settings.Realtime.ExtendedCapture.MaxDuration) * time.Second

	if !item.ExtendedCapture {
		// First time: set extended capture flag and absolute deadline
		item.ExtendedCapture = true
		item.MaxDeadline = item.FirstDetected.Add(maxDuration)
	}

	item.FlushDeadline = calculateExtendedFlushDeadline(
		now, item.FirstDetected, item.MaxDeadline, normalDetectionWindow,
	)

	p.pendingDetections[mapKey] = item
}

// calculateExtendedFlushDeadline computes the next flush deadline for an extended capture
// detection using the scaled timeout algorithm. The deadline scales with session duration:
//   - Short (<30s): max(15s, normalDetectionWindow)
//   - Medium (30s-2m): 30s after now
//   - Long (>2m): 60s after now
//
// The result is always capped at maxDeadline to enforce the absolute maximum duration.
func calculateExtendedFlushDeadline(now, firstDetected, maxDeadline time.Time, normalDetectionWindow time.Duration) time.Time {
	sessionDuration := now.Sub(firstDetected)

	var deadline time.Time
	switch {
	case sessionDuration < extendedCaptureMediumThreshold:
		deadline = now.Add(max(normalDetectionWindow, extendedCaptureMinInitialWait))
	case sessionDuration < extendedCaptureLongThreshold:
		deadline = now.Add(extendedCaptureMediumWait)
	default:
		deadline = now.Add(extendedCaptureLongWait)
	}

	if deadline.After(maxDeadline) {
		deadline = maxDeadline
	}

	return deadline
}
