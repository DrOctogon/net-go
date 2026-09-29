package conf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateDashboardSettingsElementTypes keeps the Go-side element type
// whitelist in parity with the frontend's DashboardElementType union
// (frontend/src/lib/stores/settings.ts): every type the dashboard edit-mode
// picker can add must survive validation, and unknown types must be rejected.
func TestValidateDashboardSettingsElementTypes(t *testing.T) {
	t.Parallel()

	// Mirror of the frontend ALL_ELEMENT_TYPES list plus legacy types the
	// migration may still emit.
	validTypes := []string{
		"banner",
		DashboardElementTypeDailySummary,
		"new-species-highlights",
		"currently-hearing",
		"detections-grid",
		"live-spectrogram",
		"video-embed",
		"voice-activity",
		"recent-speakers",
	}

	for _, typ := range validTypes {
		t.Run("accepts "+typ, func(t *testing.T) {
			t.Parallel()
			settings := &Dashboard{
				Layout: DashboardLayout{
					Elements: []DashboardElement{{ID: typ + "-0", Type: typ, Enabled: true}},
				},
			}
			require.NoError(t, validateDashboardSettings(settings))
		})
	}

	t.Run("rejects unknown type", func(t *testing.T) {
		t.Parallel()
		settings := &Dashboard{
			Layout: DashboardLayout{
				Elements: []DashboardElement{{ID: "bogus-0", Type: "bogus-widget", Enabled: true}},
			},
		}
		err := validateDashboardSettings(settings)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid type")
	})
}
