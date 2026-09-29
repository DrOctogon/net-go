// speakers_manage_test.go: tests for the speaker cluster management handlers
// (POST /speakers/:id/merge, DELETE /speakers/:id).
//
// The happy paths exercise the real Clusterer through Processor, which owns it
// as an unexported field; those live in
// internal/analysis/processor/speaker_cluster_ops_test.go. Here we cover what the
// handler itself decides: id validation, the self-merge conflict, the
// unavailable-capability response, and the error -> status mapping.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/analysis/processor"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// invalidSpeakerIDs are path/body ids the spk_<n> filter must reject.
var invalidSpeakerIDs = []string{
	"3",            // bare number
	"spk_",         // missing counter
	"SPK_3",        // wrong case
	"spk_3x",       // trailing garbage
	"spk_3;DROP",   // injection attempt
	"speaker_3",    // wrong prefix
	"spk_-1",       // negative
	"spk_3%20",     // encoded whitespace
	"..%2Fspk_3",   // traversal attempt
	"spk_3 OR 1=1", // SQL injection attempt
}

// callMergeSpeakers invokes MergeSpeakers with the given path id and JSON body.
func callMergeSpeakers(t *testing.T, e *echo.Echo, controller *Controller, idParam, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	// Fixed request target: the handler reads the id from the Echo path
	// parameter, and raw candidate ids would make httptest reject the URL.
	req := httptest.NewRequest(http.MethodPost, "/api/v2/speakers/id/merge", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idParam)
	return rec, controller.MergeSpeakers(c)
}

// callForgetSpeaker invokes ForgetSpeaker with the given path id.
func callForgetSpeaker(t *testing.T, e *echo.Echo, controller *Controller, idParam string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/speakers/id", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idParam)
	return rec, controller.ForgetSpeaker(c)
}

func TestMergeSpeakers_InvalidTargetID(t *testing.T) {
	for _, id := range invalidSpeakerIDs {
		t.Run(id, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)

			rec, err := callMergeSpeakers(t, e, controller, id, `{"sourceId":"spk_4"}`)

			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			mockDS.AssertNotCalled(t, "ReassignSpeakerID", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestMergeSpeakers_InvalidSourceID(t *testing.T) {
	for _, id := range invalidSpeakerIDs {
		t.Run(id, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)

			body, err := json.Marshal(map[string]string{"sourceId": id})
			require.NoError(t, err)
			rec, err := callMergeSpeakers(t, e, controller, testRosterSpeakerID, string(body))

			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			mockDS.AssertNotCalled(t, "ReassignSpeakerID", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestMergeSpeakers_MissingAndMalformedBody(t *testing.T) {
	testCases := []struct {
		name string
		body string
		want int
	}{
		{name: "malformed json", body: `{not json`, want: http.StatusBadRequest},
		{name: "missing sourceId", body: `{}`, want: http.StatusBadRequest},
		{name: "empty sourceId", body: `{"sourceId":""}`, want: http.StatusBadRequest},
		{name: "whitespace sourceId", body: `{"sourceId":"   "}`, want: http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)

			rec, err := callMergeSpeakers(t, e, controller, testRosterSpeakerID, tc.body)

			require.NoError(t, err)
			assert.Equal(t, tc.want, rec.Code)
			mockDS.AssertNotCalled(t, "ReassignSpeakerID", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestMergeSpeakers_SameIDIsConflict(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)

	body := fmt.Sprintf(`{"sourceId":%q}`, testRosterSpeakerID)
	rec, err := callMergeSpeakers(t, e, controller, testRosterSpeakerID, body)

	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, rec.Code)
	mockDS.AssertNotCalled(t, "ReassignSpeakerID", mock.Anything, mock.Anything, mock.Anything)
}

func TestSpeakerManagement_NoProcessorIsUnavailable(t *testing.T) {
	e, _, controller := setupTestEnvironment(t)
	require.Nil(t, controller.Processor, "test controller has no processor attached")

	rec, err := callMergeSpeakers(t, e, controller, testRosterSpeakerID, `{"sourceId":"spk_4"}`)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	rec, err = callForgetSpeaker(t, e, controller, testRosterSpeakerID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestSpeakerManagement_ClusteringDisabledIsUnavailable(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	// A processor exists but voice-print clustering is off: no clusterer.
	controller.Processor = &processor.Processor{Ds: mockDS}

	rec, err := callMergeSpeakers(t, e, controller, testRosterSpeakerID, `{"sourceId":"spk_4"}`)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	rec, err = callForgetSpeaker(t, e, controller, testRosterSpeakerID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	// The capability check must precede any database work.
	mockDS.AssertNotCalled(t, "ReassignSpeakerID", mock.Anything, mock.Anything, mock.Anything)
	mockDS.AssertNotCalled(t, "ClearSpeakerID", mock.Anything, mock.Anything)
}

func TestForgetSpeaker_InvalidID(t *testing.T) {
	for _, id := range invalidSpeakerIDs {
		t.Run(id, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)

			rec, err := callForgetSpeaker(t, e, controller, id)

			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			mockDS.AssertNotCalled(t, "ClearSpeakerID", mock.Anything, mock.Anything)
		})
	}
}

func TestSpeakerClusterOpStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "unknown cluster is not found",
			err:  fmt.Errorf("wrapped: %w", speaker.ErrUnknownCluster),
			want: http.StatusNotFound,
		},
		{
			name: "self merge is conflict",
			err:  fmt.Errorf("wrapped: %w", speaker.ErrSameCluster),
			want: http.StatusConflict,
		},
		{
			name: "clustering disabled is unavailable",
			err:  fmt.Errorf("wrapped: %w", processor.ErrSpeakerClusteringUnavailable),
			want: http.StatusServiceUnavailable,
		},
		{
			name: "datastore missing is unavailable",
			err:  fmt.Errorf("wrapped: %w", processor.ErrDatastoreUnavailable),
			want: http.StatusServiceUnavailable,
		},
		{
			name: "dimension mismatch is a server error",
			err:  fmt.Errorf("wrapped: %w", speaker.ErrCentroidDimensionMismatch),
			want: http.StatusInternalServerError,
		},
		{
			name: "anything else is a server error",
			err:  assert.AnError,
			want: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, speakerClusterOpStatus(tt.err))
		})
	}
}
