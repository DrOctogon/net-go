package processor

import (
	"time"

	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/logger"
	"github.com/tphakala/voicewatch/internal/suncalc"
)

// SetSunCalc injects the sun calculator into the processor. This is called
// after processor creation when the suncalc instance becomes available.
func (p *Processor) SetSunCalc(sc *suncalc.SunCalc) {
	p.sunCalc = sc
	p.initDaylightFilter()
}

// initDaylightFilter logs the daylight filter state at startup. The filter is
// global: when enabled it applies to every detection, so there is no species
// list to resolve. Safe to re-call on settings refresh.
func (p *Processor) initDaylightFilter() {
	settings := p.currentSettings()

	if !settings.Realtime.DaylightFilter.Enabled {
		return
	}

	// Warn if location has not been explicitly configured by the user. Without a
	// configured location the daylight window cannot be computed, so the filter
	// stays inactive (see checkDaylightFilter).
	if !settings.VoiceWatch.LocationConfigured {
		GetLogger().Warn("Daylight filter enabled but location not configured, filter will not be active",
			logger.String("operation", "daylight_filter_init"))
		return
	}

	GetLogger().Info("Daylight filter enabled for all detections",
		logger.Int("offset_hours", settings.Realtime.DaylightFilter.Offset),
		logger.String("operation", "daylight_filter_init"))
}

// isDaylight checks if a time falls within the daylight window.
// The daylight window is defined as [CivilDawn + offset, CivilDusk - offset).
// A positive offset shrinks the window (more lenient), a negative offset expands it (stricter).
func (p *Processor) isDaylight(t time.Time) (bool, error) {
	if p.sunCalc == nil {
		return false, errors.Newf("sun calculator not initialized").
			Component("analysis.processor").
			Category(errors.CategoryConfiguration).
			Context("operation", "check_daylight").
			Build()
	}

	sunTimes, err := p.sunCalc.GetSunEventTimes(t)
	if err != nil {
		return false, err
	}

	settings := p.currentSettings()
	offset := time.Duration(settings.Realtime.DaylightFilter.Offset) * time.Hour
	daylightStart := sunTimes.CivilDawn.Add(offset)
	daylightEnd := sunTimes.CivilDusk.Add(-offset)

	// Guard: if offset inverts the window, no time is considered daylight
	if !daylightStart.Before(daylightEnd) {
		return false, nil
	}

	// t is in [daylightStart, daylightEnd)
	return !t.Before(daylightStart) && t.Before(daylightEnd), nil
}

// checkDaylightFilter returns true if the detection should be discarded.
// The filter is global: any detection whose time falls within the daylight
// window is discarded when the filter is enabled. Fails open on suncalc errors
// and stays inactive until a location has been configured.
func (p *Processor) checkDaylightFilter(scientificName string, detectionTime time.Time) bool {
	settings := p.currentSettings()

	if !settings.Realtime.DaylightFilter.Enabled {
		return false
	}

	// Without a configured location the daylight window cannot be computed,
	// so the filter stays inactive rather than filtering on a bogus window.
	if !settings.VoiceWatch.LocationConfigured {
		return false
	}

	daylight, err := p.isDaylight(detectionTime)
	if err != nil {
		// Fail open: if we can't determine daylight, don't discard
		GetLogger().Warn("Failed to determine daylight status, allowing detection",
			logger.Any("error", err),
			logger.String("species", scientificName),
			logger.String("operation", "daylight_filter_check"))
		return false
	}

	return daylight
}
