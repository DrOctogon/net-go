// sse_guest_strip_test.go: verifies unauthenticated SSE subscribers receive
// detection events and pending snapshots with source identity and station
// location stripped, mirroring the REST guest policy.
package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/analysis/processor"
)

const testSourceDisplayName = "Backyard Cam"

func TestStripSSEDetectionForGuest(t *testing.T) {
	t.Parallel()

	d := SSEDetectionData{
		ID:             7,
		ScientificName: "Homo sapiens",
		Latitude:       60.17,
		Longitude:      24.94,
		Source:         &SSESourceInfo{ID: "rtsp-1", DisplayName: testSourceDisplayName},
	}
	stripSSEDetectionForGuest(&d)

	assert.Nil(t, d.Source)
	assert.Zero(t, d.Latitude)
	assert.Zero(t, d.Longitude)
	// Non-sensitive fields untouched.
	assert.Equal(t, uint(7), d.ID)
	assert.Equal(t, "Homo sapiens", d.ScientificName)
}

func TestStripSSEPendingForGuest(t *testing.T) {
	t.Parallel()

	t.Run("snapshot sources blanked, original untouched", func(t *testing.T) {
		t.Parallel()
		original := []processor.SSEPendingDetection{
			{Species: "Human Voice", Source: testSourceDisplayName, SourceID: "rtsp-1", HitCount: 3},
			{Species: "Human Voice", Source: "Porch", SourceID: "rtsp-2", HitCount: 1},
		}
		out := stripSSEPendingForGuest(original)

		stripped, ok := out.([]processor.SSEPendingDetection)
		require.True(t, ok)
		require.Len(t, stripped, 2)
		for _, item := range stripped {
			assert.Empty(t, item.Source)
			assert.Empty(t, item.SourceID)
		}
		assert.Equal(t, "Human Voice", stripped[0].Species)
		assert.Equal(t, 3, stripped[0].HitCount)
		// The broadcast slice is shared across clients: authenticated
		// subscribers must still see the original values.
		assert.Equal(t, testSourceDisplayName, original[0].Source)
		assert.Equal(t, "rtsp-1", original[0].SourceID)
	})

	t.Run("non-snapshot payload passes through", func(t *testing.T) {
		t.Parallel()
		out := stripSSEPendingForGuest("not-a-snapshot")
		assert.Equal(t, "not-a-snapshot", out)
	})
}
