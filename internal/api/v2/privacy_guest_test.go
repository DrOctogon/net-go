// privacy_guest_test.go: tests that speech/speaker data never reaches
// unauthenticated clients on the public detection endpoints, that free-text
// query parameters are redacted from request logs, and that the shared
// per-IP rate limiter returns 429 after its burst.
//
// PrivateMode interplay: when Security.PrivateMode is on, the privateModeAuth
// middleware (api.go) rejects unauthenticated requests before any handler
// runs; that behavior is unchanged by the field stripping and is covered by
// TestMediaByIDRoutesGatedByPrivateMode and TestIsPrivateModeExempt.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/api/auth"
	"github.com/tphakala/voicewatch/internal/datastore"
)

// authServiceStub is a minimal auth.Service used to flip isClientAuthenticated
// in handler tests. Only IsAuthenticated matters; the rest are inert.
type authServiceStub struct {
	authenticated bool
}

var _ auth.Service = (*authServiceStub)(nil)

func (s *authServiceStub) CheckAccess(echo.Context) error          { return nil }
func (s *authServiceStub) IsAuthRequired(echo.Context) bool        { return !s.authenticated }
func (s *authServiceStub) GetUsername(echo.Context) string         { return "" }
func (s *authServiceStub) GetAuthMethod(echo.Context) auth.AuthMethod {
	return auth.AuthMethodNone
}
func (s *authServiceStub) ValidateToken(string) error { return nil }
func (s *authServiceStub) AuthenticateBasic(echo.Context, string, string) (string, error) {
	return "", nil
}
func (s *authServiceStub) Logout(echo.Context) error { return nil }
func (s *authServiceStub) ExchangeAuthCode(context.Context, string) (string, error) {
	return "", nil
}
func (s *authServiceStub) EstablishSession(echo.Context, string) error { return nil }
func (s *authServiceStub) IsAuthenticated(echo.Context) bool           { return s.authenticated }
func (s *authServiceStub) GetProviderLogoutURL(echo.Context, string) string {
	return ""
}

// File-local constants for strings goconst would otherwise flag as repeated.
const (
	fieldSource     = "source"
	fieldTranscript = "transcript"
	paramSearch     = "search"
	paramLimit      = "limit"
	testGender      = "female"
	testAgeBand     = "adult"
)

// speechNote returns a Note populated with every speech/speaker field that
// must be withheld from guests, plus a source.
func speechNote() datastore.Note {
	return datastore.Note{
		ID:               42,
		Date:             "2026-09-25",
		Time:             "13:45:10",
		ScientificName:   "Homo sapiens",
		CommonName:       "Human vocal",
		Confidence:       0.9,
		Source:           testRealtimeSource(),
		Transcript:       "meet me at the bank at noon",
		TranscriptLang:   "en",
		Flagged:          true,
		KeywordsHit:      "bank,noon",
		Gender:           testGender,
		GenderConfidence: 0.8,
		AgeBand:          testAgeBand,
		AgeConfidence:    0.7,
		SpeakerID:        "spk_3",
	}
}

// sensitiveDetectionJSONKeys are the wire names of every field
// stripSensitiveDetectionFields blanks. omitempty on each means a stripped
// field disappears from the JSON object entirely.
var sensitiveDetectionJSONKeys = []string{
	fieldSource,
	fieldTranscript,
	"transcriptLang",
	"flagged",
	"keywordsHit",
	"gender",
	"genderConfidence",
	"ageBand",
	"ageConfidence",
	"speakerId",
}

// assertSensitiveKeys asserts that all sensitive keys are present (authenticated)
// or absent (guest) in one serialized detection object.
func assertSensitiveKeys(t *testing.T, detection map[string]any, authenticated bool) {
	t.Helper()
	for _, key := range sensitiveDetectionJSONKeys {
		_, present := detection[key]
		if authenticated {
			assert.True(t, present, "authenticated response must include %q", key)
		} else {
			assert.False(t, present, "guest response must not include %q", key)
		}
	}
	// Sanity: non-sensitive fields survive stripping either way.
	assert.EqualValues(t, 42, detection["id"], "id must always be present")
	assert.Equal(t, "Human vocal", detection["commonName"], "commonName must always be present")
}

// TestDetectionEndpoints_SpeechFieldsStrippedForGuests exercises each public
// detection endpoint with and without authentication and asserts the
// speech/speaker fields (and source) are present only for authenticated
// clients. Not parallel: setupTestEnvironment publishes global settings.
func TestDetectionEndpoints_SpeechFieldsStrippedForGuests(t *testing.T) {
	endpoints := []struct {
		name string
		call func(t *testing.T, authenticated bool) map[string]any
	}{
		{
			name: "GET /detections/recent",
			call: func(t *testing.T, authenticated bool) map[string]any {
				t.Helper()
				e, mockDS, controller := setupTestEnvironment(t)
				controller.authService = &authServiceStub{authenticated: authenticated}
				mockDS.On("GetLastDetections", 10).Return([]datastore.Note{speechNote()}, nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v2/detections/recent", http.NoBody)
				rec := httptest.NewRecorder()
				ctx := e.NewContext(req, rec)
				require.NoError(t, controller.GetRecentDetections(ctx))
				require.Equal(t, http.StatusOK, rec.Code)

				var detections []map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detections))
				require.Len(t, detections, 1)
				return detections[0]
			},
		},
		{
			name: "GET /detections (list)",
			call: func(t *testing.T, authenticated bool) map[string]any {
				t.Helper()
				e, mockDS, controller := setupTestEnvironment(t)
				controller.authService = &authServiceStub{authenticated: authenticated}
				mockDS.On("SearchNotes", "", false, 100, 0).
					Return([]datastore.Note{speechNote()}, int64(1), nil)

				req := httptest.NewRequest(http.MethodGet, "/api/v2/detections", http.NoBody)
				rec := httptest.NewRecorder()
				ctx := e.NewContext(req, rec)
				require.NoError(t, controller.GetDetections(ctx))
				require.Equal(t, http.StatusOK, rec.Code)

				detections, _ := decodePaginated(t, rec.Body.Bytes())
				require.Len(t, detections, 1)
				return detections[0]
			},
		},
		{
			name: "GET /detections/:id",
			call: func(t *testing.T, authenticated bool) map[string]any {
				t.Helper()
				e, mockDS, controller := setupTestEnvironment(t)
				controller.authService = &authServiceStub{authenticated: authenticated}
				mockDS.On("Get", "42").Return(speechNote(), nil)
				mockDS.On("GetHourlyWeather", "2026-09-25").
					Return([]datastore.HourlyWeather{}, nil).Maybe()

				req := httptest.NewRequest(http.MethodGet, "/api/v2/detections/42", http.NoBody)
				rec := httptest.NewRecorder()
				ctx := e.NewContext(req, rec)
				ctx.SetParamNames("id")
				ctx.SetParamValues("42")
				require.NoError(t, controller.GetDetection(ctx))
				require.Equal(t, http.StatusOK, rec.Code)

				var detection map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detection))
				return detection
			},
		},
	}

	for _, ep := range endpoints {
		for _, authenticated := range []bool{true, false} {
			mode := "guest"
			if authenticated {
				mode = "authenticated"
			}
			t.Run(ep.name+" "+mode, func(t *testing.T) {
				detection := ep.call(t, authenticated)
				assertSensitiveKeys(t, detection, authenticated)
			})
		}
	}
}

// TestHandleSearch_SourceBlankedForGuests verifies the POST /search guest
// path blanks the source (the only sensitive field DetectionRecord carries;
// see TestSearchAndSSEWireStructsCarryNoSpeechFields for the shape guard).
func TestHandleSearch_SourceBlankedForGuests(t *testing.T) {
	for _, authenticated := range []bool{true, false} {
		mode := "guest"
		if authenticated {
			mode = "authenticated"
		}
		t.Run(mode, func(t *testing.T) {
			e, mockDS, controller := setupTestEnvironment(t)
			controller.authService = &authServiceStub{authenticated: authenticated}
			records := []datastore.DetectionRecord{{ID: "1", Source: "rtsp-cam-back-yard"}}
			mockDS.On("SearchDetections", mock.AnythingOfType("*datastore.SearchFilters")).
				Return(records, 1, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/v2/search", strings.NewReader(`{}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)
			require.NoError(t, controller.HandleSearch(ctx))
			require.Equal(t, http.StatusOK, rec.Code)

			var resp SearchResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Len(t, resp.Results, 1)
			if authenticated {
				assert.Equal(t, "rtsp-cam-back-yard", resp.Results[0].Source)
			} else {
				assert.Empty(t, resp.Results[0].Source, "guest search results must not carry the source")
			}
		})
	}
}

// forbiddenWireFieldNames are JSON field names (lowercased) that must never
// appear on structs served to potentially-unauthenticated clients without
// guest stripping (search results, public SSE detection events). Derived from
// sensitiveDetectionJSONKeys minus source: those structs intentionally carry a
// source field that is blanked for guests at their own choke points.
func forbiddenWireFieldNames() map[string]struct{} {
	forbidden := make(map[string]struct{}, len(sensitiveDetectionJSONKeys))
	for _, key := range sensitiveDetectionJSONKeys {
		if key == fieldSource {
			continue
		}
		forbidden[strings.ToLower(key)] = struct{}{}
	}
	return forbidden
}

// assertNoForbiddenJSONFields fails if any struct field's JSON name is one of
// the speech/speaker fields. This is the tripwire referenced by the comments
// in search.go and sse.go: adding such a field to these wire structs requires
// adding guest stripping at their choke points first.
func assertNoForbiddenJSONFields(t *testing.T, structType reflect.Type) {
	t.Helper()
	forbiddenNames := forbiddenWireFieldNames()
	for field := range structType.Fields() {
		name := field.Name
		if tag, ok := field.Tag.Lookup("json"); ok {
			if tagName, _, _ := strings.Cut(tag, ","); tagName != "" && tagName != "-" {
				name = tagName
			}
		}
		_, forbidden := forbiddenNames[strings.ToLower(name)]
		assert.False(t, forbidden,
			"%s.%s is a speech/speaker field on a struct served to guests; add guest stripping before shipping it",
			structType.Name(), field.Name)
	}
}

// TestSearchAndSSEWireStructsCarryNoSpeechFields guards the wire shape of the
// two public payload structs that do NOT go through
// stripSensitiveDetectionFields: search results (datastore.DetectionRecord)
// and the public SSE detection stream (SSEDetectionData).
func TestSearchAndSSEWireStructsCarryNoSpeechFields(t *testing.T) {
	t.Parallel()
	assertNoForbiddenJSONFields(t, reflect.TypeOf(datastore.DetectionRecord{}))
	assertNoForbiddenJSONFields(t, reflect.TypeOf(SSEDetectionData{}))
}

// TestRedactSensitiveQuery verifies free-text query parameters are redacted
// from the request-log line while other parameters survive.
func TestRedactSensitiveQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rawQuery string
		want     map[string]string // expected decoded key -> value; nil means expect verbatim passthrough
	}{
		{
			name:     "empty query",
			rawQuery: "",
			want:     nil,
		},
		{
			name:     "no sensitive params passes through verbatim",
			rawQuery: "limit=5&queryType=hourly",
			want:     nil,
		},
		{
			name:     "search value redacted, others kept",
			rawQuery: "search=my+private+words&limit=5",
			want:     map[string]string{paramSearch: redactedQueryValue, paramLimit: "5"},
		},
		{
			name:     "transcript value redacted",
			rawQuery: "transcript=secret%20plan",
			want:     map[string]string{fieldTranscript: redactedQueryValue},
		},
		{
			name:     "both sensitive params redacted",
			rawQuery: "transcript=a&search=b",
			want:     map[string]string{fieldTranscript: redactedQueryValue, paramSearch: redactedQueryValue},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := redactSensitiveQuery(tt.rawQuery)
			if tt.want == nil {
				assert.Equal(t, tt.rawQuery, got, "query without sensitive params must pass through verbatim")
				return
			}
			values, err := url.ParseQuery(got)
			require.NoError(t, err)
			for key, want := range tt.want {
				assert.Equal(t, want, values.Get(key), "param %q", key)
			}
			assert.NotContains(t, got, "private", "free text must not survive redaction")
			assert.NotContains(t, got, "secret", "free text must not survive redaction")
		})
	}

	t.Run("unparseable query redacted wholesale", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, redactedQueryValue, redactSensitiveQuery("transcript=%zz"))
	})
}

// TestNewIPRateLimiter_Returns429AfterBurst smoke-tests the shared per-IP
// limiter used by the speakers group and /detections/:id/similar: requests
// beyond the burst within the window get 429.
func TestNewIPRateLimiter_Returns429AfterBurst(t *testing.T) {
	t.Parallel()

	const burst = 3
	e := echo.New()
	e.GET("/limited", func(ctx echo.Context) error {
		return ctx.NoContent(http.StatusOK)
	}, newIPRateLimiter(burst))

	doRequest := func() int {
		req := httptest.NewRequest(http.MethodGet, "/limited", http.NoBody)
		req.RemoteAddr = "203.0.113.7:1234" // same client IP every time
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := range burst {
		assert.Equal(t, http.StatusOK, doRequest(), "request %d within burst must pass", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, doRequest(), "request beyond burst must be rejected")

	// A different client IP has its own budget.
	req := httptest.NewRequest(http.MethodGet, "/limited", http.NoBody)
	req.RemoteAddr = "203.0.113.8:1234"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "another client IP must not share the exhausted budget")
}
