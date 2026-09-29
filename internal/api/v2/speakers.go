// speakers.go: API v2 endpoints for the household speaker roster.
// Users assign human-readable names to voice-print speaker clusters
// (ids like "spk_3"); the frontend joins these names onto detections
// client-side via GET /api/v2/speakers.
package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/voicewatch/internal/analysis/processor"
	"github.com/tphakala/voicewatch/internal/datastore"
	"github.com/tphakala/voicewatch/internal/errors"
	"github.com/tphakala/voicewatch/internal/speaker"
)

const (
	// speakerActivityDefaultDays is the window returned by GET
	// /speakers/activity when no start date is given: the last 30 days
	// (inclusive of today).
	speakerActivityDefaultDays = 30
	// speakerActivityMaxRangeDays caps the requested date range so a single
	// request cannot ask for an unbounded scan (366 covers a leap year).
	speakerActivityMaxRangeDays = 366
	// speakerClusteringUnavailableMessage is returned when cluster management is
	// requested while voice-print clustering is not running.
	speakerClusteringUnavailableMessage = "Voice-print speaker clustering is not enabled"
)

// SpeakerNameEntry is the PUT /speakers/:id/name response: a voice-print
// speaker cluster id and its user-assigned display name.
type SpeakerNameEntry struct {
	SpeakerID string `json:"speakerId"`
	Name      string `json:"name"`
}

// SpeakerRosterEntry is one GET /speakers roster row: a voice-print speaker
// cluster id, its user-assigned display name ("" when unnamed), and how many
// detections currently reference it. The added detections field is
// backward-compatible with consumers that only read speakerId/name.
type SpeakerRosterEntry struct {
	SpeakerID  string `json:"speakerId"`
	Name       string `json:"name"`
	Detections int64  `json:"detections"`
}

// speakerNameRequest is the PUT /speakers/:id/name request body.
type speakerNameRequest struct {
	Name string `json:"name"`
}

// initSpeakerRoutes registers the speaker roster endpoints. The whole group is
// auth-gated: the roster maps voice prints to household members' identities and
// mutations change user data.
func (c *Controller) initSpeakerRoutes() {
	// Honor the constructor's "datastore disabled" mode (NewWithOptions permits
	// a nil datastore) instead of registering handlers that would panic.
	if c.DS == nil {
		c.logWarnIfEnabled("Skipping speaker routes: datastore is not available")
		return
	}

	// Rate limiting runs before auth so unauthenticated hammering is bounded
	// too; the name mutation gets an additional, stricter budget.
	speakerGroup := c.Group.Group("/speakers",
		newIPRateLimiter(speakerRosterRateLimitPerMinute), c.authMiddleware)
	speakerGroup.GET("", c.GetSpeakers)
	speakerGroup.GET("/activity", c.GetSpeakerActivity)
	speakerGroup.PUT("/:id/name", c.UpdateSpeakerName,
		newIPRateLimiter(speakerNameRateLimitPerMinute))
	// Cluster management mutates voice-print state and bulk-relabels detections,
	// so it shares the stricter budget of the name mutation.
	speakerGroup.POST("/:id/merge", c.MergeSpeakers,
		newIPRateLimiter(speakerNameRateLimitPerMinute))
	speakerGroup.DELETE("/:id", c.ForgetSpeaker,
		newIPRateLimiter(speakerNameRateLimitPerMinute))
}

// speakerMergeRequest is the POST /speakers/:id/merge request body: the cluster
// to fold into the path id.
type speakerMergeRequest struct {
	SourceID string `json:"sourceId"`
}

// SpeakerMergeResult is the POST /speakers/:id/merge response: the surviving
// target cluster id and the retired source id.
type SpeakerMergeResult struct {
	SpeakerID string `json:"speakerId"`
	SourceID  string `json:"sourceId"`
}

// speakerClusterOpStatus maps a processor cluster-management error to an HTTP
// status: unknown cluster -> 404, self-merge -> 409, voice-print clustering or
// datastore unavailable -> 503, anything else (including the defensive
// centroid-dimension mismatch) -> 500.
func speakerClusterOpStatus(err error) int {
	switch {
	case errors.Is(err, speaker.ErrUnknownCluster):
		return http.StatusNotFound
	case errors.Is(err, speaker.ErrSameCluster):
		return http.StatusConflict
	case errors.Is(err, processor.ErrSpeakerClusteringUnavailable),
		errors.Is(err, processor.ErrDatastoreUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// MergeSpeakers folds one voice-print cluster into another, correcting the
// online clusterer splitting a single person across two ids. The path id is the
// surviving target; the body's sourceId is retired. Its detections are
// relabelled and its centroid folded into the target's (count-weighted).
func (c *Controller) MergeSpeakers(ctx echo.Context) error {
	targetID := validSpeakerIDFilter(ctx.Param("id"))
	if targetID == "" {
		return c.HandleError(ctx, fmt.Errorf("invalid speaker id"),
			"Invalid speaker ID, expected format spk_<n>", http.StatusBadRequest)
	}

	req := &speakerMergeRequest{}
	if err := ctx.Bind(req); err != nil {
		return c.HandleError(ctx, err, "Invalid request format", http.StatusBadRequest)
	}
	sourceID := validSpeakerIDFilter(strings.TrimSpace(req.SourceID))
	if sourceID == "" {
		return c.HandleError(ctx, fmt.Errorf("invalid source speaker id"),
			"Invalid sourceId, expected format spk_<n>", http.StatusBadRequest)
	}
	if sourceID == targetID {
		return c.HandleError(ctx, fmt.Errorf("%w: %s", speaker.ErrSameCluster, targetID),
			"Cannot merge a speaker into itself", http.StatusConflict)
	}

	if c.Processor == nil {
		return c.HandleError(ctx, processor.ErrSpeakerClusteringUnavailable,
			speakerClusteringUnavailableMessage, http.StatusServiceUnavailable)
	}
	if err := c.Processor.MergeSpeakerClusters(ctx.Request().Context(), targetID, sourceID); err != nil {
		return c.HandleError(ctx, err, "Failed to merge speakers", speakerClusterOpStatus(err))
	}

	return ctx.JSON(http.StatusOK, SpeakerMergeResult{SpeakerID: targetID, SourceID: sourceID})
}

// ForgetSpeaker removes a voice-print cluster: its detections are unlabelled
// (the clips are kept), its display name is deleted, and the cluster is dropped
// from the clusterer. The id is retired and never reissued.
func (c *Controller) ForgetSpeaker(ctx echo.Context) error {
	speakerID := validSpeakerIDFilter(ctx.Param("id"))
	if speakerID == "" {
		return c.HandleError(ctx, fmt.Errorf("invalid speaker id"),
			"Invalid speaker ID, expected format spk_<n>", http.StatusBadRequest)
	}

	if c.Processor == nil {
		return c.HandleError(ctx, processor.ErrSpeakerClusteringUnavailable,
			speakerClusteringUnavailableMessage, http.StatusServiceUnavailable)
	}
	if err := c.Processor.ForgetSpeakerCluster(ctx.Request().Context(), speakerID); err != nil {
		return c.HandleError(ctx, err, "Failed to forget speaker", speakerClusterOpStatus(err))
	}

	return ctx.NoContent(http.StatusNoContent)
}

// GetSpeakers returns the full household speaker roster: every speaker
// cluster id referenced by detections (named or not) plus every user-named
// speaker, as [{speakerId, name, detections}].
func (c *Controller) GetSpeakers(ctx echo.Context) error {
	roster, err := c.DS.GetSpeakerRoster(ctx.Request().Context())
	if err != nil {
		return c.HandleError(ctx, err, "Failed to get speaker roster", http.StatusInternalServerError)
	}

	entries := make([]SpeakerRosterEntry, 0, len(roster))
	for i := range roster {
		entries = append(entries, SpeakerRosterEntry{
			SpeakerID:  roster[i].SpeakerID,
			Name:       roster[i].Name,
			Detections: roster[i].Detections,
		})
	}

	return ctx.JSON(http.StatusOK, entries)
}

// SpeakerDailyActivityEntry is one GET /speakers/activity row: how many
// detections one voice-print speaker cluster produced on one day. Display
// names are joined client-side from the roster (GET /speakers).
type SpeakerDailyActivityEntry struct {
	SpeakerID string `json:"speakerId"`
	Date      string `json:"date"`
	Count     int    `json:"count"`
}

// parseSpeakerActivityRange validates the optional start/end query params
// (YYYY-MM-DD) and applies defaults: end defaults to today, start to
// speakerActivityDefaultDays before end (inclusive window). Returns the
// resolved dates or a non-empty message describing the validation failure.
func parseSpeakerActivityRange(startParam, endParam string, now time.Time) (startDate, endDate, errMsg string) {
	end := now
	if endParam != "" {
		parsed, err := time.Parse(time.DateOnly, endParam)
		if err != nil {
			return "", "", "Invalid end date format. Use YYYY-MM-DD"
		}
		end = parsed
	}

	start := end.AddDate(0, 0, -(speakerActivityDefaultDays - 1))
	if startParam != "" {
		parsed, err := time.Parse(time.DateOnly, startParam)
		if err != nil {
			return "", "", "Invalid start date format. Use YYYY-MM-DD"
		}
		start = parsed
	}

	if start.After(end) {
		return "", "", "start date cannot be after end date"
	}
	if start.AddDate(0, 0, speakerActivityMaxRangeDays).Before(end) {
		return "", "", fmt.Sprintf("Date range too large. Maximum: %d days", speakerActivityMaxRangeDays)
	}

	return start.Format(time.DateOnly), end.Format(time.DateOnly), ""
}

// GetSpeakerActivity returns per-speaker daily detection counts as
// [{speakerId, date, count}], ordered by date then speaker id. Optional
// start/end query params (YYYY-MM-DD, inclusive) bound the range; the
// default window is the last speakerActivityDefaultDays days.
func (c *Controller) GetSpeakerActivity(ctx echo.Context) error {
	startDate, endDate, errMsg := parseSpeakerActivityRange(
		ctx.QueryParam("start"), ctx.QueryParam("end"), time.Now())
	if errMsg != "" {
		return c.HandleError(ctx, fmt.Errorf("invalid speaker activity range"), errMsg, http.StatusBadRequest)
	}

	activity, err := c.DS.GetSpeakerDailyActivity(ctx.Request().Context(), startDate, endDate)
	if err != nil {
		return c.HandleError(ctx, err, "Failed to get speaker activity", http.StatusInternalServerError)
	}

	entries := make([]SpeakerDailyActivityEntry, 0, len(activity))
	for i := range activity {
		entries = append(entries, SpeakerDailyActivityEntry{
			SpeakerID: activity[i].SpeakerID,
			Date:      activity[i].Date,
			Count:     activity[i].Count,
		})
	}

	return ctx.JSON(http.StatusOK, entries)
}

// UpdateSpeakerName assigns, renames, or clears the display name of one
// speaker cluster. An empty or whitespace-only name clears the mapping.
func (c *Controller) UpdateSpeakerName(ctx echo.Context) error {
	speakerID := validSpeakerIDFilter(ctx.Param("id"))
	if speakerID == "" {
		return c.HandleError(ctx, fmt.Errorf("invalid speaker id"),
			"Invalid speaker ID, expected format spk_<n>", http.StatusBadRequest)
	}

	req := &speakerNameRequest{}
	if err := ctx.Bind(req); err != nil {
		return c.HandleError(ctx, err, "Invalid request format", http.StatusBadRequest)
	}

	name := strings.TrimSpace(req.Name)
	if len(name) > datastore.MaxSpeakerNameLength {
		return c.HandleError(ctx, fmt.Errorf("name too long: %d bytes", len(name)),
			fmt.Sprintf("Name must be at most %d characters", datastore.MaxSpeakerNameLength),
			http.StatusBadRequest)
	}

	if err := c.DS.SetSpeakerName(ctx.Request().Context(), speakerID, name); err != nil {
		if errors.IsCategory(err, errors.CategoryValidation) {
			return c.HandleError(ctx, err, "Invalid speaker name", http.StatusBadRequest)
		}
		return c.HandleError(ctx, err, "Failed to update speaker name", http.StatusInternalServerError)
	}

	return ctx.JSON(http.StatusOK, SpeakerNameEntry{SpeakerID: speakerID, Name: name})
}
