// speakers_test.go: tests for the speaker naming / household roster handlers.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/datastore"
)

const (
	// testRosterSpeakerID is the speaker cluster id used across roster tests.
	testRosterSpeakerID = "spk_3"
	// testRosterName is the display name used across roster tests.
	testRosterName = "Alice"
	// testRosterNameBody is a valid PUT body assigning testRosterName.
	testRosterNameBody = `{"name":"Alice"}`
)

// callGetSpeakers invokes the GetSpeakers handler and returns the recorder
// and any handler error.
func callGetSpeakers(t *testing.T, e *echo.Echo, controller *Controller) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/speakers", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return rec, controller.GetSpeakers(c)
}

// callUpdateSpeakerName invokes the UpdateSpeakerName handler with the given
// path id and JSON body, returning the recorder and any handler error.
func callUpdateSpeakerName(t *testing.T, e *echo.Echo, controller *Controller, idParam, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	// Use a fixed request target: the handler reads the id from the Echo path
	// parameter (set below), and raw candidate ids (spaces, traversal bytes)
	// would make httptest.NewRequest reject the URL before the handler runs.
	req := httptest.NewRequest(http.MethodPut, "/api/v2/speakers/id/name", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idParam)
	return rec, controller.UpdateSpeakerName(c)
}

func TestGetSpeakers_Empty(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("GetSpeakerNames", mock.Anything).Return([]datastore.SpeakerName{}, nil)

	rec, err := callGetSpeakers(t, e, controller)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out []SpeakerNameEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	// Empty roster must serialize as [], not null.
	assert.Equal(t, []SpeakerNameEntry{}, out)
	assert.JSONEq(t, "[]", rec.Body.String())

	mockDS.AssertExpectations(t)
}

func TestGetSpeakers_HappyPath(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("GetSpeakerNames", mock.Anything).Return([]datastore.SpeakerName{
		{SpeakerID: "spk_7", Name: testRosterName},
		{SpeakerID: testRosterSpeakerID, Name: "Bob"},
	}, nil)

	rec, err := callGetSpeakers(t, e, controller)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out []SpeakerNameEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, []SpeakerNameEntry{
		{SpeakerID: "spk_7", Name: testRosterName},
		{SpeakerID: testRosterSpeakerID, Name: "Bob"},
	}, out)

	mockDS.AssertExpectations(t)
}

func TestGetSpeakers_DatastoreError(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("GetSpeakerNames", mock.Anything).Return(nil, errors.New("db down"))

	rec, err := callGetSpeakers(t, e, controller)

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	mockDS.AssertExpectations(t)
}

func TestUpdateSpeakerName_InvalidID(t *testing.T) {
	invalidIDs := []string{
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

	for _, id := range invalidIDs {
		t.Run(id, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)

			rec, err := callUpdateSpeakerName(t, e, controller, id, testRosterNameBody)

			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			// Validation must reject before any datastore access.
			mockDS.AssertNotCalled(t, "SetSpeakerName", mock.Anything, mock.Anything, mock.Anything)
			mockDS.AssertExpectations(t)
		})
	}
}

func TestUpdateSpeakerName_InvalidBody(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)

	rec, err := callUpdateSpeakerName(t, e, controller, testRosterSpeakerID, `{not json`)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	mockDS.AssertNotCalled(t, "SetSpeakerName", mock.Anything, mock.Anything, mock.Anything)
	mockDS.AssertExpectations(t)
}

func TestUpdateSpeakerName_TooLong(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)

	tooLong := strings.Repeat("a", datastore.MaxSpeakerNameLength+1)
	rec, err := callUpdateSpeakerName(t, e, controller, testRosterSpeakerID, `{"name":"`+tooLong+`"}`)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	mockDS.AssertNotCalled(t, "SetSpeakerName", mock.Anything, mock.Anything, mock.Anything)
	mockDS.AssertExpectations(t)
}

func TestUpdateSpeakerName_MaxLengthAfterTrimAccepted(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)

	// Exactly the cap after surrounding whitespace is trimmed - must be accepted.
	maxName := strings.Repeat("a", datastore.MaxSpeakerNameLength)
	mockDS.On("SetSpeakerName", mock.Anything, testRosterSpeakerID, maxName).Return(nil)

	rec, err := callUpdateSpeakerName(t, e, controller, testRosterSpeakerID, `{"name":"  `+maxName+`  "}`)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	mockDS.AssertExpectations(t)
}

func TestUpdateSpeakerName_HappyPath(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("SetSpeakerName", mock.Anything, testRosterSpeakerID, testRosterName).Return(nil)

	rec, err := callUpdateSpeakerName(t, e, controller, testRosterSpeakerID, `{"name":"  Alice  "}`)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out SpeakerNameEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, SpeakerNameEntry{SpeakerID: testRosterSpeakerID, Name: testRosterName}, out)

	mockDS.AssertExpectations(t)
}

func TestUpdateSpeakerName_EmptyNameClearsMapping(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "empty name string", body: `{"name":""}`},
		{name: "whitespace only", body: `{"name":"   "}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)
			mockDS.On("SetSpeakerName", mock.Anything, testRosterSpeakerID, "").Return(nil)

			rec, err := callUpdateSpeakerName(t, e, controller, testRosterSpeakerID, tc.body)

			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			var out SpeakerNameEntry
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, SpeakerNameEntry{SpeakerID: testRosterSpeakerID, Name: ""}, out)

			mockDS.AssertExpectations(t)
		})
	}
}

func TestUpdateSpeakerName_DatastoreError(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("SetSpeakerName", mock.Anything, testRosterSpeakerID, testRosterName).Return(errors.New("db down"))

	rec, err := callUpdateSpeakerName(t, e, controller, testRosterSpeakerID, testRosterNameBody)

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	mockDS.AssertExpectations(t)
}
