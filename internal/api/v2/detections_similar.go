// detections_similar.go
// "Similar voices" lookup: given a detection with a stored voice-print
// embedding, rank other detections by cosine similarity of their embeddings.
//
// This is a scaffold. Until a real voice-print model is vendored, detections
// carry no embeddings, so this endpoint returns an empty list. The ranking
// itself (cosine over stored vectors) is real; only the embeddings are absent.

package api

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/tphakala/voicewatch/internal/datastore"
	"github.com/tphakala/voicewatch/internal/speaker"
)

const (
	// similarCandidateLimit bounds how many recent detections are scanned for
	// embeddings per request (the public endpoint must not scan the whole table).
	similarCandidateLimit = 500
	// similarTopK is the maximum number of similar detections returned.
	similarTopK = 10
	// similarMinScore drops sub-threshold matches so unrelated speakers are not
	// surfaced as "similar". Cosine in [-1, 1]; 0.5 is a conservative floor.
	similarMinScore = 0.5
)

// SimilarDetection is one entry in the similar-voices response. ClipName is
// intentionally omitted: clip filenames can encode source/device details and
// this endpoint only needs the id + score to link detections.
type SimilarDetection struct {
	ID      uint    `json:"id"`
	Score   float64 `json:"score"` // cosine similarity in [-1, 1]; higher = more similar
	Gender  string  `json:"gender,omitempty"`
	AgeBand string  `json:"ageBand,omitempty"`
	Date    string  `json:"date,omitempty"`
	Time    string  `json:"time,omitempty"`
}

// GetSimilarDetections returns detections whose voice-print embedding is most
// similar to the target detection's embedding.
func (c *Controller) GetSimilarDetections(ctx echo.Context) error {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.HandleError(ctx, err, "Invalid detection ID", http.StatusBadRequest)
	}

	target, err := c.DS.Get(idStr)
	if err != nil {
		return c.HandleError(ctx, err, "Detection not found", http.StatusNotFound)
	}

	// No embedding on the target -> nothing to compare against. Return an empty
	// list rather than an error so the UI can render "no similar voices".
	if len(target.VoicePrintEmbedding) == 0 {
		return ctx.JSON(http.StatusOK, []SimilarDetection{})
	}

	candidates, _, err := c.DS.SearchNotesAdvanced(&datastore.AdvancedSearchFilters{
		Limit:  similarCandidateLimit,
		SortBy: "date_desc",
	})
	if err != nil {
		return c.HandleError(ctx, err, "Failed to load detections", http.StatusInternalServerError)
	}

	matches := make([]SimilarDetection, 0, len(candidates))
	for i := range candidates {
		cand := &candidates[i]
		if uint64(cand.ID) == id || len(cand.VoicePrintEmbedding) == 0 {
			continue
		}
		score := speaker.Cosine(target.VoicePrintEmbedding, cand.VoicePrintEmbedding)
		if score < similarMinScore {
			continue
		}
		matches = append(matches, SimilarDetection{
			ID:      cand.ID,
			Score:   score,
			Gender:  cand.Gender,
			AgeBand: cand.AgeBand,
			Date:    cand.Date,
			Time:    cand.Time,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})
	if len(matches) > similarTopK {
		matches = matches[:similarTopK]
	}

	return ctx.JSON(http.StatusOK, matches)
}
