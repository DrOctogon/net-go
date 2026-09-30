// alerts_handlers_test.go: httptest-based handler tests for the alert rule
// REST surface (internal/api/v2/alerts.go). Mirrors the setup idioms in
// speakers_test.go: a full Controller built via NewWithOptions, with the
// alert repository wired to a real (in-memory SQLite) AlertRuleRepository
// rather than a hand-written mock. The interface has 12 methods and no
// mockery-generated mock exists for it yet, so the repository package's own
// test harness (an in-memory GORM/SQLite DB) is the cheaper, higher-fidelity
// choice — it also lets the export/import round-trip test exercise real
// persistence instead of scripted mock returns.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/voicewatch/internal/alerting"
	"github.com/tphakala/voicewatch/internal/datastore/mocks"
	datastoreV2 "github.com/tphakala/voicewatch/internal/datastore/v2"
	"github.com/tphakala/voicewatch/internal/datastore/v2/entities"
	"github.com/tphakala/voicewatch/internal/datastore/v2/repository"
	"github.com/tphakala/voicewatch/internal/notification"
	"github.com/tphakala/voicewatch/internal/observability"
	"github.com/tphakala/voicewatch/internal/suncalc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Shared fixture values, factored out to avoid goconst flagging repeated
// literals across the many small handler tests below.
const (
	testAlertRuleName            = "Rule A"
	testAlertActionBell          = "bell"
	testAlertObjectTypeDevice    = "device"
	testAlertEventDeviceError    = "device.error"
	testAlertConditionConfidence = "confidence"
)

// createTestAlertRule persists a minimal valid, enabled alert rule (named
// testAlertRuleName, with a single "bell" action) and returns it.
func createTestAlertRule(t *testing.T, ctx context.Context, controller *Controller) *entities.AlertRule {
	t.Helper()
	rule := &entities.AlertRule{
		Name: testAlertRuleName, ObjectType: "detection", TriggerType: alerting.TriggerTypeEvent,
		EventName: "x", CooldownSec: 60, Enabled: true,
		Actions: []entities.AlertAction{{Target: testAlertActionBell}},
	}
	require.NoError(t, controller.alertRuleRepo.CreateRule(ctx, rule))
	return rule
}

// setupAlertRuleTestDB creates an in-memory SQLite database with the alert
// schema migrated. Mirrors repository.setupAlertTestDB (internal/datastore/v2/repository).
func setupAlertRuleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_foreign_keys=ON"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err, "failed to open in-memory alert rule database")

	sqlDB, err := db.DB()
	require.NoError(t, err, "failed to get sql.DB")
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close(), "failed to close test database") })

	require.NoError(t, db.AutoMigrate(
		&entities.AlertRule{},
		&entities.AlertCondition{},
		&entities.AlertAction{},
		&entities.AlertHistory{},
	), "failed to migrate alert tables")
	return db
}

// setupAlertTestEnvironment builds a Controller wired for alert rule handler
// tests: a real Controller (settings, logger, etc. all present, as handlers
// expect) with alertRuleRepo pointed at a fresh in-memory SQLite-backed
// repository, and the v2 "enhanced database" mode flag enabled so handlers
// don't short-circuit with the 409 "requires v2" guard.
//
// IMPORTANT: datastoreV2's enhanced-database flag is process-global
// (internal/datastore/v2/mode.go), so tests using this helper must NOT run
// in parallel with each other or with any other test that reads it.
func setupAlertTestEnvironment(t *testing.T) (*echo.Echo, *Controller) {
	t.Helper()

	datastoreV2.SetEnhancedDatabaseMode()
	t.Cleanup(datastoreV2.ResetDatabaseMode)

	e := echo.New()
	mockDS := mocks.NewMockInterface(t)
	settings := newValidTestSettings()
	settings.Realtime.Audio.Export.Path = t.TempDir()
	sunCalc := suncalc.NewSunCalc(testHelsinkiLatitude, testHelsinkiLongitude)
	controlChan := make(chan string, testControlChannelBuf)
	mockMetrics, _ := observability.NewMetrics()
	publishTestSettings(t, settings)

	controller, err := NewWithOptions(e, mockDS, settings, sunCalc, controlChan, mockMetrics, false)
	require.NoError(t, err, "failed to create test API controller")
	controller.DisableSaveSettings = true
	controller.alertRuleRepo = repository.NewAlertRuleRepository(setupAlertRuleTestDB(t), nil)

	t.Cleanup(func() {
		controller.Shutdown()
		close(controlChan)
	})

	return e, controller
}

// decodeErrorKey returns the i18n error_key from a JSON error response body.
func decodeErrorKey(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		ErrorKey string `json:"error_key"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "error response must be JSON")
	return body.ErrorKey
}

// --- request helpers -------------------------------------------------------

func callListAlertRules(t *testing.T, e *echo.Echo, controller *Controller, query string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	target := "/api/v2/alerts/rules"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return rec, controller.ListAlertRules(c)
}

func callGetAlertRule(t *testing.T, e *echo.Echo, controller *Controller, idParam string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/alerts/rules/id", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idParam)
	return rec, controller.GetAlertRule(c)
}

func callCreateAlertRule(t *testing.T, e *echo.Echo, controller *Controller, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/alerts/rules", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return rec, controller.CreateAlertRule(c)
}

func callToggleAlertRule(t *testing.T, e *echo.Echo, controller *Controller, idParam, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v2/alerts/rules/id/toggle", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idParam)
	return rec, controller.ToggleAlertRule(c)
}

func callDeleteAlertRule(t *testing.T, e *echo.Echo, controller *Controller, idParam string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/alerts/rules/id", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idParam)
	return rec, controller.DeleteAlertRule(c)
}

func callResetDefaultAlertRules(t *testing.T, e *echo.Echo, controller *Controller) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/alerts/rules/reset-defaults", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return rec, controller.ResetDefaultAlertRules(c)
}

func callExportAlertRules(t *testing.T, e *echo.Echo, controller *Controller) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/alerts/rules/export", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return rec, controller.ExportAlertRules(c)
}

func callImportAlertRules(t *testing.T, e *echo.Echo, controller *Controller, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/alerts/rules/import", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return rec, controller.ImportAlertRules(c)
}

// validCreateRuleBody is a minimal, valid CreateAlertRule payload.
const validCreateRuleBody = `{
	"name": "High CPU Custom",
	"object_type": "device",
	"trigger_type": "event",
	"event_name": "device.error",
	"cooldown_sec": 60,
	"actions": [{"target": "bell"}]
}`

// --- guard clause (representative of all 13 handlers) ----------------------

func TestListAlertRules_RequiresV2Database(t *testing.T) {
	// Intentionally do not use setupAlertTestEnvironment: this exercises the
	// requireV2 guard that every alert handler starts with, so the v2
	// enhanced-database flag must stay at its default (disabled) value.
	e, _, controller := setupTestEnvironment(t)

	rec, err := callListAlertRules(t, e, controller, "")

	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// --- ListAlertRules / GetAlertRule happy paths ------------------------------

func TestListAlertRules_HappyPath(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)
	createTestAlertRule(t, t.Context(), controller)

	rec, err := callListAlertRules(t, e, controller, "")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.InEpsilon(t, float64(1), out["count"], 0, "expected exactly one rule")
}

func TestGetAlertRule_HappyPath(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)
	rule := createTestAlertRule(t, t.Context(), controller)

	rec, err := callGetAlertRule(t, e, controller, strconv.FormatUint(uint64(rule.ID), 10))

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out entities.AlertRule
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, testAlertRuleName, out.Name)
}

func TestGetAlertRule_NotFound(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)

	rec, err := callGetAlertRule(t, e, controller, "999")

	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetAlertRule_InvalidID(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)

	rec, err := callGetAlertRule(t, e, controller, "not-a-number")

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- CreateAlertRule validation ---------------------------------------------

func TestCreateAlertRule_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{not json`},
		{name: "missing name", body: `{"object_type":"device","trigger_type":"event"}`},
		{name: "missing object_type", body: `{"name":"X","trigger_type":"event"}`},
		{name: "missing trigger_type", body: `{"name":"X","object_type":"device"}`},
		{
			name: "invalid escalation steps (empty array)",
			body: `{"name":"X","object_type":"device","trigger_type":"event","escalation_steps":[]}`,
		},
		{
			name: "invalid escalation steps (negative)",
			body: `{"name":"X","object_type":"device","trigger_type":"event","escalation_steps":[-1]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, controller := setupAlertTestEnvironment(t)

			rec, err := callCreateAlertRule(t, e, controller, tc.body)

			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			listed, listErr := controller.alertRuleRepo.ListRules(t.Context(), repository.AlertRuleFilter{})
			require.NoError(t, listErr)
			assert.Empty(t, listed, "invalid payload must not create a rule")
		})
	}
}

func TestCreateAlertRule_DuplicateNameRejected(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)
	ctx := t.Context()

	require.NoError(t, controller.alertRuleRepo.CreateRule(ctx, &entities.AlertRule{
		Name: "High CPU Custom", ObjectType: testAlertObjectTypeDevice, TriggerType: alerting.TriggerTypeEvent,
		EventName: testAlertEventDeviceError, CooldownSec: 60,
		Actions: []entities.AlertAction{{Target: testAlertActionBell}},
	}))

	rec, err := callCreateAlertRule(t, e, controller, validCreateRuleBody)

	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestCreateAlertRule_HappyPath(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)

	rec, err := callCreateAlertRule(t, e, controller, validCreateRuleBody)

	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var out entities.AlertRule
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotZero(t, out.ID)
	assert.Equal(t, "High CPU Custom", out.Name)

	listed, err := controller.alertRuleRepo.ListRules(t.Context(), repository.AlertRuleFilter{})
	require.NoError(t, err)
	assert.Len(t, listed, 1)
}

// --- ToggleAlertRule ---------------------------------------------------------

func TestToggleAlertRule_HappyPath(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)
	ctx := t.Context()
	rule := createTestAlertRule(t, ctx, controller)

	rec, err := callToggleAlertRule(t, e, controller, strconv.FormatUint(uint64(rule.ID), 10), `{"enabled":false}`)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	got, err := controller.alertRuleRepo.GetRule(ctx, rule.ID)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
}

func TestToggleAlertRule_NotFound(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)

	rec, err := callToggleAlertRule(t, e, controller, "999", `{"enabled":false}`)

	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestToggleAlertRule_InvalidID(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)

	rec, err := callToggleAlertRule(t, e, controller, "abc", `{"enabled":false}`)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- DeleteAlertRule ---------------------------------------------------------

func TestDeleteAlertRule_HappyPath(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)
	ctx := t.Context()
	rule := createTestAlertRule(t, ctx, controller)

	rec, err := callDeleteAlertRule(t, e, controller, strconv.FormatUint(uint64(rule.ID), 10))

	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	_, err = controller.alertRuleRepo.GetRule(ctx, rule.ID)
	assert.ErrorIs(t, err, repository.ErrAlertRuleNotFound)
}

func TestDeleteAlertRule_BuiltInRejected(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)
	ctx := t.Context()

	rule := &entities.AlertRule{
		Name: "Built-in under test", NameKey: "settings.alerts.builtInRules.test.name",
		ObjectType: "detection", TriggerType: alerting.TriggerTypeEvent,
		EventName: "x", CooldownSec: 60, Enabled: true, BuiltIn: true,
		Actions: []entities.AlertAction{{Target: testAlertActionBell}},
	}
	require.NoError(t, controller.alertRuleRepo.CreateRule(ctx, rule))

	rec, err := callDeleteAlertRule(t, e, controller, strconv.FormatUint(uint64(rule.ID), 10))

	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, notification.MsgErrAlertBuiltInDelete, decodeErrorKey(t, rec))

	// The rule must still exist; Toggle remains the supported disable path.
	kept, err := controller.alertRuleRepo.GetRule(ctx, rule.ID)
	require.NoError(t, err)
	assert.True(t, kept.BuiltIn)
}

// TestDeleteAlertRule_NotFound pins 404 (not the built-in 409) for an unknown
// id, the condition that makes collapsing the handler's pre-check safe.
func TestDeleteAlertRule_NotFound(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)

	rec, err := callDeleteAlertRule(t, e, controller, "999")

	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, notification.MsgErrAlertNotFound, decodeErrorKey(t, rec))
}

// --- ResetDefaultAlertRules ---------------------------------------------------

func TestResetDefaultAlertRules_ReseedsBuiltInsKeepsCustom(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)
	ctx := t.Context()

	// A stale built-in (simulating a previous seeding) and a user-created
	// custom rule.
	require.NoError(t, controller.alertRuleRepo.CreateRule(ctx, &entities.AlertRule{
		Name: "Stale Built-in", BuiltIn: true, ObjectType: testAlertObjectTypeDevice, TriggerType: alerting.TriggerTypeEvent,
		EventName: testAlertEventDeviceError, CooldownSec: 60,
		Actions: []entities.AlertAction{{Target: testAlertActionBell}},
	}))
	custom := &entities.AlertRule{
		Name: "My Custom Rule", BuiltIn: false, ObjectType: testAlertObjectTypeDevice, TriggerType: alerting.TriggerTypeEvent,
		EventName: testAlertEventDeviceError, CooldownSec: 60,
		Actions: []entities.AlertAction{{Target: testAlertActionBell}},
	}
	require.NoError(t, controller.alertRuleRepo.CreateRule(ctx, custom))

	rec, err := callResetDefaultAlertRules(t, e, controller)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	after, err := controller.alertRuleRepo.ListRules(ctx, repository.AlertRuleFilter{})
	require.NoError(t, err)

	// Stale built-in gone, replaced by the current defaults; custom rule survives.
	names := make([]string, len(after))
	builtInCount := 0
	customFound := false
	for i, r := range after {
		names[i] = r.Name
		if r.BuiltIn {
			builtInCount++
		}
		if r.Name == "My Custom Rule" {
			customFound = true
		}
	}
	assert.NotContains(t, names, "Stale Built-in")
	assert.True(t, customFound, "custom rule must survive a defaults reset")
	assert.Equal(t, len(alerting.DefaultRules()), builtInCount, "built-in count must match current defaults")
}

// --- Export / Import round trip ---------------------------------------------

func TestAlertRulesExportImport_RoundTrip(t *testing.T) {
	e, controller := setupAlertTestEnvironment(t)
	ctx := t.Context()

	rule1 := &entities.AlertRule{
		Name: "Rule One", ObjectType: "detection", TriggerType: alerting.TriggerTypeEvent,
		EventName: "detection.new_species", CooldownSec: 60,
		Conditions: []entities.AlertCondition{{Property: testAlertConditionConfidence, Operator: "greater_than", Value: "0.9"}},
		Actions:    []entities.AlertAction{{Target: testAlertActionBell}},
	}
	rule2 := &entities.AlertRule{
		Name: "Rule Two", ObjectType: "stream", TriggerType: alerting.TriggerTypeEvent,
		EventName: "stream.disconnected", CooldownSec: 300,
		Actions: []entities.AlertAction{{Target: "telegram-home"}},
	}
	require.NoError(t, controller.alertRuleRepo.CreateRule(ctx, rule1))
	require.NoError(t, controller.alertRuleRepo.CreateRule(ctx, rule2))

	// Export.
	exportRec, err := callExportAlertRules(t, e, controller)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, exportRec.Code)
	exported := exportRec.Body.String()

	// Clear.
	require.NoError(t, controller.alertRuleRepo.DeleteRule(ctx, rule1.ID))
	require.NoError(t, controller.alertRuleRepo.DeleteRule(ctx, rule2.ID))
	cleared, err := controller.alertRuleRepo.ListRules(ctx, repository.AlertRuleFilter{})
	require.NoError(t, err)
	require.Empty(t, cleared)

	// Import.
	importRec, err := callImportAlertRules(t, e, controller, exported)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, importRec.Code)

	var importResp map[string]any
	require.NoError(t, json.Unmarshal(importRec.Body.Bytes(), &importResp))
	assert.InEpsilon(t, float64(2), importResp["imported"], 0)

	after, err := controller.alertRuleRepo.ListRules(ctx, repository.AlertRuleFilter{})
	require.NoError(t, err)
	gotNames := make([]string, len(after))
	for i, r := range after {
		gotNames[i] = r.Name
	}
	assert.ElementsMatch(t, []string{"Rule One", "Rule Two"}, gotNames)
}
