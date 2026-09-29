// speakers_test.go: tests for the speaker naming / household roster handlers.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/datastore"
	apierrors "github.com/tphakala/voicewatch/internal/errors"
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
	mockDS.On("GetSpeakerRoster", mock.Anything).Return([]datastore.SpeakerRosterEntry{}, nil)

	rec, err := callGetSpeakers(t, e, controller)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out []SpeakerRosterEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	// Empty roster must serialize as [], not null.
	assert.Equal(t, []SpeakerRosterEntry{}, out)
	assert.JSONEq(t, "[]", rec.Body.String())

	mockDS.AssertExpectations(t)
}

func TestGetSpeakers_HappyPath(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	// Roster mixes named and unnamed speakers; unnamed rows have empty names.
	mockDS.On("GetSpeakerRoster", mock.Anything).Return([]datastore.SpeakerRosterEntry{
		{SpeakerID: testRosterSpeakerID, Name: "Bob", Detections: 12},
		{SpeakerID: "spk_5", Name: "", Detections: 3},
		{SpeakerID: "spk_7", Name: testRosterName, Detections: 0},
	}, nil)

	rec, err := callGetSpeakers(t, e, controller)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out []SpeakerRosterEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, []SpeakerRosterEntry{
		{SpeakerID: testRosterSpeakerID, Name: "Bob", Detections: 12},
		{SpeakerID: "spk_5", Name: "", Detections: 3},
		{SpeakerID: "spk_7", Name: testRosterName, Detections: 0},
	}, out)

	mockDS.AssertExpectations(t)
}

func TestGetSpeakers_DatastoreError(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("GetSpeakerRoster", mock.Anything).Return(nil, errors.New("db down"))

	rec, err := callGetSpeakers(t, e, controller)

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	mockDS.AssertExpectations(t)
}

func TestUpdateSpeakerName_InvalidID(t *testing.T) {
	// invalidSpeakerIDs is shared with the cluster-management handler tests
	// (speakers_manage_test.go) so every /speakers mutation rejects the same set.
	for _, id := range invalidSpeakerIDs {
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

// callGetSpeakerActivity invokes the GetSpeakerActivity handler with the
// given query string (e.g. "start=2024-01-01&end=2024-01-31"), returning the
// recorder and any handler error.
func callGetSpeakerActivity(t *testing.T, e *echo.Echo, controller *Controller, query string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	target := "/api/v2/speakers/activity"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return rec, controller.GetSpeakerActivity(c)
}

func TestGetSpeakerActivity_DefaultRangeEmpty(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	// No params: last-30-days window, both bounds set and 29 days apart.
	mockDS.On("GetSpeakerDailyActivity", mock.Anything,
		time.Now().AddDate(0, 0, -29).Format(time.DateOnly),
		time.Now().Format(time.DateOnly)).
		Return([]datastore.SpeakerDailyActivity{}, nil)

	rec, err := callGetSpeakerActivity(t, e, controller, "")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	// Empty activity must serialize as [], not null.
	assert.JSONEq(t, "[]", rec.Body.String())

	mockDS.AssertExpectations(t)
}

// testActivityStart / testActivityEnd bound the explicit range used by the
// speaker activity happy-path test.
const (
	testActivityStart = "2030-01-01"
	testActivityEnd   = "2030-01-31"
)

func TestGetSpeakerActivity_ExplicitRangeHappyPath(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("GetSpeakerDailyActivity", mock.Anything, testActivityStart, testActivityEnd).
		Return([]datastore.SpeakerDailyActivity{
			{SpeakerID: testRosterSpeakerID, Date: testActivityStart, Count: 5},
			{SpeakerID: "spk_5", Date: "2030-01-02", Count: 2},
		}, nil)

	rec, err := callGetSpeakerActivity(t, e, controller, "start="+testActivityStart+"&end="+testActivityEnd)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out []SpeakerDailyActivityEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, []SpeakerDailyActivityEntry{
		{SpeakerID: testRosterSpeakerID, Date: testActivityStart, Count: 5},
		{SpeakerID: "spk_5", Date: "2030-01-02", Count: 2},
	}, out)

	mockDS.AssertExpectations(t)
}

func TestGetSpeakerActivity_InvalidParams(t *testing.T) {
	testCases := []struct {
		name  string
		query string
	}{
		{name: "garbage start", query: "start=notadate"},
		{name: "garbage end", query: "end=2024-99-99"},
		{name: "sql injection start", query: "start=2024-01-01'--"},
		{name: "start after end", query: "start=2024-02-01&end=2024-01-01"},
		{name: "range too large", query: "start=2020-01-01&end=2024-01-01"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)

			rec, err := callGetSpeakerActivity(t, e, controller, tc.query)

			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			// Validation must reject before any datastore access.
			mockDS.AssertNotCalled(t, "GetSpeakerDailyActivity", mock.Anything, mock.Anything, mock.Anything)
			mockDS.AssertExpectations(t)
		})
	}
}

func TestGetSpeakerActivity_DatastoreError(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("GetSpeakerDailyActivity", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("db down"))

	rec, err := callGetSpeakerActivity(t, e, controller, "")

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	mockDS.AssertExpectations(t)
}

func TestUpdateSpeakerName_DatastoreError(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	mockDS.On("SetSpeakerName", mock.Anything, testRosterSpeakerID, testRosterName).Return(errors.New("db down"))

	rec, err := callUpdateSpeakerName(t, e, controller, testRosterSpeakerID, testRosterNameBody)

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	mockDS.AssertExpectations(t)
}

// TestUpdateSpeakerName_ValidationErrorIsBadRequest verifies that a
// CategoryValidation error surfaced by the datastore (e.g. the control-char /
// invalid-UTF-8 sanitization in SetSpeakerName) maps to 400, not the generic
// 500 used for other datastore failures.
func TestUpdateSpeakerName_ValidationErrorIsBadRequest(t *testing.T) {
	e, mockDS, controller := setupTestEnvironment(t)
	validationErr := apierrors.Newf("invalid name: control characters").
		Component("datastore").
		Category(apierrors.CategoryValidation).
		Build()
	mockDS.On("SetSpeakerName", mock.Anything, testRosterSpeakerID, testRosterName).Return(validationErr)

	rec, err := callUpdateSpeakerName(t, e, controller, testRosterSpeakerID, testRosterNameBody)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	mockDS.AssertExpectations(t)
}
