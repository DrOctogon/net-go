// ratelimit.go: shared per-IP rate limiter construction for API v2 routes.
// Individual endpoints with bespoke error handling (auth login, SSE streams)
// keep their own inline configs; this helper covers the common case of
// "N requests per minute per client IP, 429 otherwise".
package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

const (
	// speakerRosterRateLimitPerMinute bounds all /speakers endpoints per IP.
	// The dashboard fetches the roster once per page load to join names onto
	// detections, so 30/min tolerates aggressive refreshing while stopping
	// enumeration hammering of the household identity roster.
	speakerRosterRateLimitPerMinute = 30
	// speakerNameRateLimitPerMinute additionally bounds PUT /speakers/:id/name.
	// Renames are rare, deliberate user actions; 10/min stops scripted
	// mass-mutation of the roster while never impeding a human.
	speakerNameRateLimitPerMinute = 10
	// similarDetectionsRateLimitPerMinute bounds GET /detections/:id/similar,
	// which fans out to embedding comparisons and is therefore the most
	// expensive detection read.
	similarDetectionsRateLimitPerMinute = 30
	// ipRateLimiterExpiry is how long an idle client's limiter state is kept
	// before the in-memory store evicts it.
	ipRateLimiterExpiry = 3 * time.Minute
)

// newIPRateLimiter returns Echo middleware limiting each client IP to
// perMinute requests per minute (with a burst of the same size). Exceeding
// the limit returns 429 with a JSON error body. State is in-memory and
// per-process, matching the existing SSE and stream-health limiters.
func newIPRateLimiter(perMinute int) echo.MiddlewareFunc {
	tooMany := func(echo.Context) error {
		return echo.NewHTTPError(http.StatusTooManyRequests,
			"Rate limit exceeded, please slow down")
	}
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      rate.Limit(float64(perMinute) / float64(SecondsPerMinute)),
				Burst:     perMinute,
				ExpiresIn: ipRateLimiterExpiry,
			},
		),
		IdentifierExtractor: middleware.DefaultRateLimiterConfig.IdentifierExtractor,
		ErrorHandler: func(ctx echo.Context, err error) error {
			return tooMany(ctx)
		},
		DenyHandler: func(ctx echo.Context, identifier string, err error) error {
			return tooMany(ctx)
		},
	})
}
