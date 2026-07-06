// mqtt_api_contract_test.go: Tests for MQTT JSON payload backward compatibility.
//
// IMPORTANT: These tests verify the MQTT API contract. The JSON field names tested here
// are part of the PUBLIC API used by Home Assistant and other MQTT integrations.
//
// DO NOT MODIFY these expected field names without:
// 1. Explicit approval from maintainers
// 2. A migration plan for existing integrations
// 3. Documentation in release notes
//
// Breaking changes to these field names will break user integrations.
// See: https://github.com/tphakala/voicewatch/discussions/1759
package processor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/datastore"

)

// =============================================================================
// MQTT API CONTRACT - FIXED FIELD NAMES
// =============================================================================
//
// The following field names are part of the MQTT API contract and MUST NOT be
// changed without explicit maintainer approval. These are used by:
// - Home Assistant MQTT integrations
// - Custom MQTT consumers
// - Third-party automation tools
//
// Any change to these names is a BREAKING CHANGE.
// =============================================================================

// mqttAPIContractFields defines the expected JSON field names for MQTT messages.
// These are FROZEN and must not be changed without explicit approval.
//
// MODIFICATION POLICY:
// - DO NOT change existing field names
// - New fields may be added (with camelCase for new fields, but existing PascalCase must be preserved)
// - Removal of fields requires deprecation notice and migration period
var mqttAPIContractFields = struct {
	// Detection message root-level fields (from embedded datastore.Note)
	// These use PascalCase because Go's default JSON marshaling is used
	CommonName     string
	ScientificName string
	Confidence     string
	Date           string
	Time           string
	Latitude       string
	Longitude      string
	ClipName       string
	ProcessingTime string

	// Detection message root-level fields (explicit JSON tags)
	DetectionID string // camelCase - database ID for URL construction (issue #1748)
	SourceID    string // camelCase - added for Home Assistant discovery
	SourceName  string // camelCase - display name for stable source mapping
	Occurrence  string // lowercase with omitempty
}{
	// Root-level fields from embedded Note (PascalCase - Go default)
	CommonName:     "CommonName",
	ScientificName: "ScientificName",
	Confidence:     "Confidence",
	Date:           "Date",
	Time:           "Time",
	Latitude:       "Latitude",
	Longitude:      "Longitude",
	ClipName:       "ClipName",
	ProcessingTime: "ProcessingTime",

	// Root-level fields with explicit tags
	DetectionID: "detectionId", // camelCase - database ID for URL construction (issue #1748)
	SourceID:    "sourceId",    // camelCase - new field for HA
	SourceName:  "sourceName",  // camelCase - display name for stable source mapping
	Occurrence:  "occurrence",  // lowercase with omitempty
}

// TestMQTTAPIContract_NoteWithBirdImage_FieldNames verifies that the MQTT JSON
// payload uses the correct field names for backward compatibility.
//
// IMPORTANT: This test is a CONTRACT test. If it fails, you are likely breaking
// existing MQTT integrations. DO NOT modify the expected values without explicit
// maintainer approval and a migration plan.
func TestMQTTAPIContract_NoteWithBirdImage_FieldNames(t *testing.T) {
	t.Parallel()

	// Create a complete NoteWithBirdImage struct with all fields populated
	note := NoteWithBirdImage{
		Note: datastore.Note{
			ID:             12345, // Simulated database ID
			CommonName:     "American Robin",
			ScientificName: "Turdus migratorius",
			Confidence:     0.95,
			Date:           "2024-01-15",
			Time:           "12:00:00",
			Latitude:       42.3601,
			Longitude:      -71.0589,
			ClipName:       "test_clip.wav",
			ProcessingTime: 150 * time.Millisecond,
			Occurrence:     0.75,
			Source:         testAudioSource(),
		},
		DetectionID: 12345, // Should match Note.ID for URL construction
		SourceID:    "test-source-1",
		SourceName:  testAudioSource().DisplayName, // "test-source"
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(note)
	require.NoError(t, err, "Failed to marshal NoteWithBirdImage to JSON")

	// Parse back to generic map to check field names
	var jsonMap map[string]any
	err = json.Unmarshal(jsonData, &jsonMap)
	require.NoError(t, err, "Failed to unmarshal JSON to map")

	// Log the actual JSON for debugging
	t.Logf("MQTT JSON payload:\n%s", string(jsonData))

	// ==========================================================================
	// CONTRACT ASSERTIONS - DO NOT MODIFY EXPECTED VALUES
	// ==========================================================================
	// These assertions verify the API contract. Changing the expected values
	// here means you are accepting a breaking change to the MQTT API.
	// ==========================================================================

	t.Run("Root level Note fields use PascalCase", func(t *testing.T) {
		// FROZEN: These field names are part of the API contract
		assert.Contains(t, jsonMap, mqttAPIContractFields.CommonName,
			"MQTT API CONTRACT VIOLATION: CommonName field must be PascalCase")
		assert.Contains(t, jsonMap, mqttAPIContractFields.ScientificName,
			"MQTT API CONTRACT VIOLATION: ScientificName field must be PascalCase")
		assert.Contains(t, jsonMap, mqttAPIContractFields.Confidence,
			"MQTT API CONTRACT VIOLATION: Confidence field must be PascalCase")
		assert.Contains(t, jsonMap, mqttAPIContractFields.Date,
			"MQTT API CONTRACT VIOLATION: Date field must be PascalCase")
		assert.Contains(t, jsonMap, mqttAPIContractFields.Time,
			"MQTT API CONTRACT VIOLATION: Time field must be PascalCase")
		assert.Contains(t, jsonMap, mqttAPIContractFields.ClipName,
			"MQTT API CONTRACT VIOLATION: ClipName field must be PascalCase")
	})

	t.Run("DetectionID uses camelCase (for URL construction)", func(t *testing.T) {
		// This is a new field for constructing API URLs (issue #1748)
		assert.Contains(t, jsonMap, mqttAPIContractFields.DetectionID,
			"MQTT API CONTRACT: detectionId field must be present for URL construction")
		detectionID, ok := jsonMap[mqttAPIContractFields.DetectionID].(float64)
		require.True(t, ok, "detectionId must be a number")
		assert.InDelta(t, 12345, detectionID, 0.001,
			"detectionId value mismatch")
	})

	t.Run("SourceID uses camelCase (new field for HA)", func(t *testing.T) {
		// This is a new field added for Home Assistant, uses camelCase
		assert.Contains(t, jsonMap, mqttAPIContractFields.SourceID,
			"MQTT API CONTRACT: sourceId field must be present for HA filtering")
		assert.Equal(t, "test-source-1", jsonMap[mqttAPIContractFields.SourceID],
			"sourceId value mismatch")
	})

	t.Run("SourceName uses camelCase (display name for stable mapping)", func(t *testing.T) {
		assert.Contains(t, jsonMap, mqttAPIContractFields.SourceName,
			"MQTT API CONTRACT: sourceName field must be present for stable source mapping")
		assert.Equal(t, "test-source", jsonMap[mqttAPIContractFields.SourceName],
			"sourceName must match the audio source DisplayName")
	})

	t.Run("Occurrence uses lowercase with omitempty", func(t *testing.T) {
		// occurrence field uses lowercase (from Note struct JSON tag)
		assert.Contains(t, jsonMap, mqttAPIContractFields.Occurrence,
			"MQTT API CONTRACT: occurrence field must be present when non-zero")
		occurrence, ok := jsonMap[mqttAPIContractFields.Occurrence].(float64)
		require.True(t, ok, "occurrence must be a number")
		assert.InDelta(t, 0.75, occurrence, 0.001, "occurrence value mismatch")
	})
}

// TestMQTTAPIContract_OccurrenceOmittedWhenZero verifies the omitempty behavior.
func TestMQTTAPIContract_OccurrenceOmittedWhenZero(t *testing.T) {
	t.Parallel()

	note := NoteWithBirdImage{
		Note: datastore.Note{
			CommonName:     "Blue Jay",
			ScientificName: "Cyanocitta cristata",
			Confidence:     0.92,
			Occurrence:     0.0, // Zero - should be omitted
			Source:         testAudioSource(),
		},
		SourceID: "test-source",
	}

	jsonData, err := json.Marshal(note)
	require.NoError(t, err)

	var jsonMap map[string]any
	err = json.Unmarshal(jsonData, &jsonMap)
	require.NoError(t, err)

	// occurrence should be omitted when zero (omitempty)
	_, hasOccurrence := jsonMap["occurrence"]
	assert.False(t, hasOccurrence,
		"MQTT API CONTRACT: occurrence field should be omitted when value is zero (omitempty)")
}

// TestMQTTAPIContract_AllExpectedFieldsPresent is a comprehensive check that all
// expected fields are present in the MQTT payload.
func TestMQTTAPIContract_AllExpectedFieldsPresent(t *testing.T) {
	t.Parallel()

	note := NoteWithBirdImage{
		Note: datastore.Note{
			ID:             67890, // Database primary key
			CommonName:     "House Sparrow",
			ScientificName: "Passer domesticus",
			Confidence:     0.87,
			Date:           "2024-06-15",
			Time:           "08:30:00",
			Latitude:       34.0522,
			Longitude:      -118.2437,
			ClipName:       "detection_001.wav",
			ProcessingTime: 125 * time.Millisecond,
			Occurrence:     0.65,
			Source:         testAudioSource(),
		},
		DetectionID: 67890, // Should match Note.ID for URL construction
		SourceID:    "garden-mic",
	}

	jsonData, err := json.Marshal(note)
	require.NoError(t, err)

	var jsonMap map[string]any
	err = json.Unmarshal(jsonData, &jsonMap)
	require.NoError(t, err)

	// ==========================================================================
	// EXPECTED ROOT-LEVEL FIELDS (FROZEN CONTRACT)
	// ==========================================================================
	expectedRootFields := []string{
		"CommonName",     // PascalCase - from embedded Note
		"ScientificName", // PascalCase - from embedded Note
		"Confidence",     // PascalCase - from embedded Note
		"Date",           // PascalCase - from embedded Note
		"Time",           // PascalCase - from embedded Note
		"Latitude",       // PascalCase - from embedded Note
		"Longitude",      // PascalCase - from embedded Note
		"ClipName",       // PascalCase - from embedded Note
		"ProcessingTime", // PascalCase - from embedded Note
		"occurrence",     // lowercase - from embedded Note (explicit tag)
		"detectionId",    // camelCase - database ID for URL construction (issue #1748)
		"sourceId",       // camelCase - new field for HA discovery
	}

	for _, field := range expectedRootFields {
		assert.Contains(t, jsonMap, field,
			"MQTT API CONTRACT: Expected field '%s' not found in JSON payload", field)
	}
}

// TestMQTTAPIContract_NoRedundantDuplicateFields verifies that the MQTT payload
// does not contain redundant fields that duplicate the canonical identifiers.
//
// Previously, the embedded Note struct leaked "ID" (duplicate of "detectionId")
// and "Source" (duplicate of "sourceId") into the JSON payload (GitHub #109).
func TestMQTTAPIContract_NoRedundantDuplicateFields(t *testing.T) {
	t.Parallel()

	note := NoteWithBirdImage{
		Note: datastore.Note{
			ID:             12345,
			CommonName:     "Test Bird",
			ScientificName: "Testus birdus",
			Confidence:     0.9,
			Source:         testAudioSource(),
		},
		DetectionID: 12345,
		SourceID:    "test-source",
	}

	jsonData, err := json.Marshal(note)
	require.NoError(t, err)

	var jsonMap map[string]any
	err = json.Unmarshal(jsonData, &jsonMap)
	require.NoError(t, err)

	// "ID" from embedded Note must NOT appear — "detectionId" is the canonical field
	assert.NotContains(t, jsonMap, "ID",
		"Redundant field 'ID' must not appear in MQTT payload — use 'detectionId' (GitHub #109)")

	// "Source" from embedded Note must NOT appear — "sourceId" is the canonical field
	assert.NotContains(t, jsonMap, "Source",
		"Redundant field 'Source' must not appear in MQTT payload — use 'sourceId' (GitHub #109)")

	// Canonical fields must still be present
	assert.Contains(t, jsonMap, "detectionId",
		"Canonical field 'detectionId' must be present")
	assert.Contains(t, jsonMap, "sourceId",
		"Canonical field 'sourceId' must be present")
}

// TestMQTTAPIContract_SourceName_OmittedWhenEmpty verifies that sourceName is omitted
// from the MQTT payload when DisplayName is empty, per the omitempty tag.
func TestMQTTAPIContract_SourceName_OmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	src := testAudioSource()
	src.DisplayName = ""

	note := NoteWithBirdImage{
		Note: datastore.Note{
			CommonName:     "House Sparrow",
			ScientificName: "Passer domesticus",
			Confidence:     0.91,
			Source:         src,
		},
		SourceID: "rtsp_4d50dd0d",
	}

	jsonData, err := json.Marshal(note)
	require.NoError(t, err)

	var jsonMap map[string]any
	err = json.Unmarshal(jsonData, &jsonMap)
	require.NoError(t, err)

	assert.NotContains(t, jsonMap, "sourceName",
		"sourceName must be omitted when DisplayName is empty (omitempty)")
}

// TestMQTTAPIContract_NoUnexpectedCamelCaseConversions verifies that fields that
// should be PascalCase have not been accidentally converted to camelCase.
//
// This test catches the exact bug that was introduced in PR #1749.
func TestMQTTAPIContract_NoUnexpectedCamelCaseConversions(t *testing.T) {
	t.Parallel()

	note := NoteWithBirdImage{
		Note: datastore.Note{
			CommonName:     "Test Bird",
			ScientificName: "Testus birdus",
			Confidence:     0.9,
			Source:         testAudioSource(),
		},
		SourceID: "test",
	}

	jsonData, err := json.Marshal(note)
	require.NoError(t, err)

	jsonStr := string(jsonData)

	// ==========================================================================
	// FORBIDDEN FIELD NAMES - These indicate breaking changes
	// ==========================================================================
	// If any of these appear in the JSON, it means a field was incorrectly
	// changed from PascalCase to camelCase, breaking the API contract.
	// ==========================================================================

	forbiddenFields := []struct {
		wrong   string
		correct string
		reason  string
	}{
		{"birdImage", "BirdImage", "PR #1749 regression - breaks Home Assistant integrations"},
		{"commonName", "CommonName", "Would break existing MQTT consumers"},
		{"scientificName", "ScientificName", "Would break existing MQTT consumers"},
		{"confidence", "Confidence", "Would break existing MQTT consumers"},
		{"clipName", "ClipName", "Would break existing MQTT consumers"},
		{"processingTime", "ProcessingTime", "Would break existing MQTT consumers"},
	}

	for _, f := range forbiddenFields {
		// Check that the wrong (camelCase) version does NOT appear as a key
		// We need to check for it as a JSON key, not just substring
		assert.NotContains(t, jsonStr, `"`+f.wrong+`":`,
			"MQTT API CONTRACT VIOLATION: Found '%s' but should be '%s'. Reason: %s",
			f.wrong, f.correct, f.reason)
	}
}
