// detections_similar_test.go: tests for the GetSimilarDetections handler.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/datastore"
)

// callGetSimilarDetections builds an echo.Context for the given path id and
// invokes the handler, returning the recorder and any handler error.
func callGetSimilarDetections(t *testing.T, e *echo.Echo, controller *Controller, idParam string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/detections/"+idParam+"/similar", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idParam)
	return rec, controller.GetSimilarDetections(c)
}

// decodeSimilarDetections unmarshals the response body into []SimilarDetection.
func decodeSimilarDetections(t *testing.T, rec *httptest.ResponseRecorder) []SimilarDetection {
	t.Helper()
	var out []SimilarDetection
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

func TestGetSimilarDetections_InvalidID(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)

	rec, err := callGetSimilarDetections(t, e, controller, "not-a-number")

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "Invalid detection ID", errResp.Message)

	// ParseUint fails before any datastore access.
	mockDS.AssertNotCalled(t, "Get", mock.Anything)
	mockDS.AssertExpectations(t)
}

func TestGetSimilarDetections_DetectionNotFound(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("Get", "999").Return(datastore.Note{}, errors.New("record not found"))

	rec, err := callGetSimilarDetections(t, e, controller, "999")

	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "Detection not found", errResp.Message)

	mockDS.AssertExpectations(t)
}

func TestGetSimilarDetections_NoEmbeddingOnTarget(t *testing.T) {
	testCases := []struct {
		name      string
		embedding []float32
	}{
		{name: "nil embedding", embedding: nil},
		{name: "empty embedding", embedding: []float32{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)
			mockDS.On("Get", "1").Return(datastore.Note{ID: 1, VoicePrintEmbedding: tc.embedding}, nil)

			rec, err := callGetSimilarDetections(t, e, controller, "1")

			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, []SimilarDetection{}, decodeSimilarDetections(t, rec))

			// No embedding to compare against -> the candidate scan must be skipped entirely.
			mockDS.AssertNotCalled(t, "SearchNotesAdvanced", mock.Anything)
			mockDS.AssertExpectations(t)
		})
	}
}

func TestGetSimilarDetections_HappyPath(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)

	targetEmbedding := []float32{1, 0, 0}
	mockDS.On("Get", "1").Return(datastore.Note{ID: 1, VoicePrintEmbedding: targetEmbedding}, nil)

	candidates := []datastore.Note{
		// Same speaker as target (identical vector) -> cosine 1.0.
		{ID: 2, VoicePrintEmbedding: []float32{1, 0, 0}, Gender: "male", AgeBand: "adult", Date: "2025-01-02", Time: "09:45:12"},
		// Somewhat similar -> cosine ~0.707, still above the 0.5 floor.
		{ID: 3, VoicePrintEmbedding: []float32{1, 1, 0}, Gender: "female", AgeBand: "child", Date: "2025-01-03", Time: "11:00:00"},
		// Orthogonal -> cosine 0, below the similarity floor, must be dropped.
		{ID: 4, VoicePrintEmbedding: []float32{0, 1, 0}},
		// No embedding -> must be skipped before scoring.
		{ID: 5, VoicePrintEmbedding: nil},
		// Same ID as the target (defensive: the target itself must never appear
		// in its own similar-detections list even if returned by the candidate scan).
		{ID: 1, VoicePrintEmbedding: []float32{1, 0, 0}},
	}
	mockDS.On("SearchNotesAdvanced", mock.MatchedBy(func(f *datastore.AdvancedSearchFilters) bool {
		return f.Limit == similarCandidateLimit && f.SortBy == "date_desc"
	})).Return(candidates, int64(len(candidates)), nil)

	rec, err := callGetSimilarDetections(t, e, controller, "1")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	got := decodeSimilarDetections(t, rec)

	require.Len(t, got, 2)
	// Sorted by score descending: the identical vector (score 1.0) comes first.
	assert.Equal(t, uint(2), got[0].ID)
	assert.InDelta(t, 1.0, got[0].Score, 1e-9)
	assert.Equal(t, "male", got[0].Gender)
	assert.Equal(t, "adult", got[0].AgeBand)
	assert.Equal(t, "2025-01-02", got[0].Date)
	assert.Equal(t, "09:45:12", got[0].Time)

	assert.Equal(t, uint(3), got[1].ID)
	assert.InDelta(t, 0.7071067811865475, got[1].Score, 1e-9)

	mockDS.AssertExpectations(t)
}

func TestGetSimilarDetections_TopKTruncation(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)

	targetEmbedding := []float32{1, 0}
	mockDS.On("Get", "1").Return(datastore.Note{ID: 1, VoicePrintEmbedding: targetEmbedding}, nil)

	// similarTopK+5 candidates, all identical to the target vector (score 1.0),
	// to verify the handler truncates to similarTopK entries.
	candidates := make([]datastore.Note, 0, similarTopK+5)
	for i := uint(2); i < 2+uint(similarTopK)+5; i++ {
		candidates = append(candidates, datastore.Note{ID: i, VoicePrintEmbedding: []float32{1, 0}})
	}
	mockDS.On("SearchNotesAdvanced", mock.Anything).Return(candidates, int64(len(candidates)), nil)

	rec, err := callGetSimilarDetections(t, e, controller, "1")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	got := decodeSimilarDetections(t, rec)
	assert.Len(t, got, similarTopK)

	mockDS.AssertExpectations(t)
}

func TestGetSimilarDetections_CandidateLoadFailure(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)

	mockDS.On("Get", "1").Return(datastore.Note{ID: 1, VoicePrintEmbedding: []float32{1, 0, 0}}, nil)
	mockDS.On("SearchNotesAdvanced", mock.Anything).Return(nil, int64(0), errors.New("db unavailable"))

	rec, err := callGetSimilarDetections(t, e, controller, "1")

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "Failed to load detections", errResp.Message)

	mockDS.AssertExpectations(t)
}
