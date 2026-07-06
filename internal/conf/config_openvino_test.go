package conf

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBirdNETBackendPreferenceField(t *testing.T) {
	t.Parallel()
	var s Settings
	s.VoiceWatch.Backend = BackendPrefOpenVINO
	s.VoiceWatch.OpenVINOPath = "/usr/lib/libopenvino_c.so"
	assert.Equal(t, "openvino", s.VoiceWatch.Backend)
	assert.Equal(t, "/usr/lib/libopenvino_c.so", s.VoiceWatch.OpenVINOPath)
}

func TestBirdNETOpenVINODeviceField(t *testing.T) {
	t.Parallel()
	var s Settings
	s.VoiceWatch.OpenVINODevice = OVDeviceGPU
	assert.Equal(t, "gpu", s.VoiceWatch.OpenVINODevice)
}

// TestValidateOpenVINODevice verifies that known device preferences are accepted
// and an unknown value produces a (non-fatal) warning that falls back to auto.
func TestValidateOpenVINODevice(t *testing.T) {
	t.Parallel()
	hasDeviceWarning := func(r ValidationResult) bool {
		for _, w := range r.Warnings {
			if strings.Contains(w, "openvinodevice") {
				return true
			}
		}
		return false
	}

	for _, dev := range []string{"", OVDeviceAuto, OVDeviceCPU, OVDeviceGPU} {
		cfg := &VoiceWatchConfig{OpenVINODevice: dev}
		assert.Falsef(t, hasDeviceWarning(ValidateVoiceWatchSettings(cfg)),
			"device %q must be accepted without a warning", dev)
	}

	cfg := &VoiceWatchConfig{OpenVINODevice: "tpu"}
	res := ValidateVoiceWatchSettings(cfg)
	assert.True(t, hasDeviceWarning(res), "an unknown openvinodevice must warn")
	assert.True(t, res.Valid, "an unknown openvinodevice must not invalidate the config")
}
