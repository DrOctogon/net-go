// speakers.go: API v2 endpoints for the household speaker roster.
// Users assign human-readable names to voice-print speaker clusters
// (ids like "spk_3"); the frontend joins these names onto detections
// client-side via GET /api/v2/speakers.
package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/voicewatch/internal/datastore"
)

// SpeakerNameEntry is one roster row in API responses: a voice-print speaker
// cluster id and its user-assigned display name.
type SpeakerNameEntry struct {
	SpeakerID string `json:"speakerId"`
	Name      string `json:"name"`
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

	speakerGroup := c.Group.Group("/speakers", c.authMiddleware)
	speakerGroup.GET("", c.GetSpeakers)
	speakerGroup.PUT("/:id/name", c.UpdateSpeakerName)
}

// GetSpeakers returns the household speaker roster: every speaker cluster id
// that has a user-assigned name, as [{speakerId, name}].
func (c *Controller) GetSpeakers(ctx echo.Context) error {
	names, err := c.DS.GetSpeakerNames(ctx.Request().Context())
	if err != nil {
		return c.HandleError(ctx, err, "Failed to get speaker names", http.StatusInternalServerError)
	}

	entries := make([]SpeakerNameEntry, 0, len(names))
	for i := range names {
		entries = append(entries, SpeakerNameEntry{
			SpeakerID: names[i].SpeakerID,
			Name:      names[i].Name,
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
		return c.HandleError(ctx, err, "Failed to update speaker name", http.StatusInternalServerError)
	}

	return ctx.JSON(http.StatusOK, SpeakerNameEntry{SpeakerID: speakerID, Name: name})
}
