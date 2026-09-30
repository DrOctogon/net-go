// diagnostics_marshal_panic_test.go: the diagnostics endpoints must survive a
// health check that puts an unmarshalable value in Result.Details.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/health"
	"github.com/tphakala/voicewatch/internal/observability"
)

// badDetailsCheck is a health check whose Details hold a value that panics when
// encoding/json tries to marshal it. Details is a map[string]any filled in by
// each check, so this is reachable from any current or future check - the
// endpoint must not depend on every check being well behaved.
type badDetailsCheck struct{}

func (badDetailsCheck) Name() string              { return "bad_details" }
func (badDetailsCheck) Category() health.Category { return health.CategoryAudio }

func (badDetailsCheck) Run(_ context.Context) health.Result {
	return health.Result{
		Name:     "bad_details",
		Category: health.CategoryAudio,
		Status:   health.StatusHealthy,
		Message:  "check with unmarshalable details",
		// panicMarshaler (sse_panic_recovery_test.go) panics in MarshalJSON.
		Details:   map[string]any{"broken": &panicMarshaler{}},
		Timestamp: time.Now(),
	}
}

// TestRunDiagnostics_UnmarshalableDetailsDoesNotPanic pins the behaviour that a
// check with unmarshalable Details yields an error response rather than
// unwinding out of the handler. Calling the handler directly, with no echo
// Recover middleware in the way, is exactly the situation that turned this into
// an opaque package-level CI failure rather than a reported test failure.
func TestRunDiagnostics_UnmarshalableDetailsDoesNotPanic(t *testing.T) {
	t.Parallel()

	store := observability.NewHealthMetricsStore()
	store.RecordAt("audio.drops.src1", 5, time.Now())

	e, controller := setupDiagnosticsTest(t, store, nil)
	controller.healthRegistry.Register(badDetailsCheck{})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/diagnostics/run?window=15m", http.NoBody)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	// require.NotPanics is the assertion under test: before the fix this call
	// panicked with "invalid memory address or nil pointer dereference" from
	// inside encoding/json and killed the test binary.
	require.NotPanics(t, func() {
		err := controller.RunDiagnostics(ctx)
		require.NoError(t, err, "handler should report the failure, not return an error")
	})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to encode diagnostics response")
}

// TestRunDiagnostics_WellBehavedDetailsStillSerialize guards against "fixing"
// the panic by never returning a body: a normal report must still come back as
// JSON with its results intact.
func TestRunDiagnostics_WellBehavedDetailsStillSerialize(t *testing.T) {
	t.Parallel()

	store := observability.NewHealthMetricsStore()
	store.RecordAt("audio.drops.src1", 5, time.Now())

	e, controller := setupDiagnosticsTest(t, store, nil)
	controller.healthRegistry.Register(okDetailsCheck{})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/diagnostics/run?window=15m", http.NoBody)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	require.NoError(t, controller.RunDiagnostics(ctx))
	require.Equal(t, http.StatusOK, rec.Code)

	var report health.DiagnosticsReport
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	assert.NotEmpty(t, report.ID)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "ok_details", report.Results[0].Name)
	assert.Equal(t, okDetailValue, report.Results[0].Details[okDetailKey])
}

// Named rather than inline: bare "key"/"value" literals here tip goconst's
// occurrence count for those strings past its threshold across several
// unrelated test files in this package.
const (
	okDetailKey   = "detail_field"
	okDetailValue = "detail_value"
)

// okDetailsCheck is the well-behaved counterpart to badDetailsCheck.
type okDetailsCheck struct{}

func (okDetailsCheck) Name() string              { return "ok_details" }
func (okDetailsCheck) Category() health.Category { return health.CategoryAudio }

func (okDetailsCheck) Run(_ context.Context) health.Result {
	return health.Result{
		Name:      "ok_details",
		Category:  health.CategoryAudio,
		Status:    health.StatusHealthy,
		Message:   "fine",
		Details:   map[string]any{okDetailKey: okDetailValue},
		Timestamp: time.Now(),
	}
}
