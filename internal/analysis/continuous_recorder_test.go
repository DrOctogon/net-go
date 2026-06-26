package analysis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
)

// ---------------------------------------------------------------------------
// isExpired — pure decision function
// ---------------------------------------------------------------------------

func TestIsExpired_NotExpired(t *testing.T) {
	t.Parallel()
	now := time.Now()
	// file modified 23 hours ago, retention 24 hours → not yet expired
	modTime := now.Add(-23 * time.Hour)
	assert.False(t, isExpired(modTime, now, 24), "file modified 23h ago should not be expired with 24h retention")
}

func TestIsExpired_ExactBoundary(t *testing.T) {
	t.Parallel()
	now := time.Now()
	// file modified exactly 24 hours ago — age == retention, not strictly greater
	modTime := now.Add(-24 * time.Hour)
	assert.False(t, isExpired(modTime, now, 24), "file modified exactly 24h ago should not be expired (boundary is exclusive)")
}

func TestIsExpired_JustPastBoundary(t *testing.T) {
	t.Parallel()
	now := time.Now()
	// file modified 24h + 1s ago → expired
	modTime := now.Add(-24*time.Hour - time.Second)
	assert.True(t, isExpired(modTime, now, 24), "file modified 24h+1s ago should be expired with 24h retention")
}

func TestIsExpired_FarPast(t *testing.T) {
	t.Parallel()
	now := time.Now()
	modTime := now.Add(-168 * time.Hour) // 7 days
	assert.True(t, isExpired(modTime, now, 24), "file modified 7 days ago should be expired with 24h retention")
}

func TestIsExpired_ZeroRetention(t *testing.T) {
	t.Parallel()
	now := time.Now()
	// retentionHours=0 means anything older than 0 is expired
	modTime := now.Add(-time.Second)
	assert.True(t, isExpired(modTime, now, 0), "any file should be expired when retentionHours is 0")
}

func TestIsExpired_FutureModTime(t *testing.T) {
	t.Parallel()
	now := time.Now()
	// mtime in the future (clock skew) → definitely not expired
	modTime := now.Add(time.Hour)
	assert.False(t, isExpired(modTime, now, 24), "file with future mtime should not be expired")
}

func TestIsExpired_ShortRetention(t *testing.T) {
	t.Parallel()
	now := time.Now()
	modTime := now.Add(-2 * time.Hour)
	assert.True(t, isExpired(modTime, now, 1), "file modified 2h ago should be expired with 1h retention")
	assert.False(t, isExpired(modTime, now, 3), "file modified 2h ago should not be expired with 3h retention")
}

// ---------------------------------------------------------------------------
// sanitizeSourceName — pure string transformer
// ---------------------------------------------------------------------------

func TestSanitizeSourceName_AlphanumericAndAllowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"FrontYard", "FrontYard"},
		{"front_yard", "front_yard"},
		{"stream-1", "stream-1"},
		{"Stream_A-1", "Stream_A-1"},
		{"abc123", "abc123"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, sanitizeSourceName(tc.input))
		})
	}
}

func TestSanitizeSourceName_ReplacesUnsafeChars(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"Front Yard", "Front_Yard"},
		{"stream/one", "stream_one"},
		{"A.B.C", "A_B_C"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, sanitizeSourceName(tc.input))
		})
	}
}

func TestSanitizeSourceName_TrimsLeadingTrailingUnderscores(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "foo", sanitizeSourceName("__foo__"))
	assert.Equal(t, "foo_bar", sanitizeSourceName("_foo_bar_"))
}

func TestSanitizeSourceName_EmptyFallback(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "stream", sanitizeSourceName(""), "empty name should fall back to 'stream'")
	assert.Equal(t, "stream", sanitizeSourceName("___"), "all-unsafe name should fall back to 'stream'")
	assert.Equal(t, "stream", sanitizeSourceName("!!!"), "all-unsafe name should fall back to 'stream'")
}

// ---------------------------------------------------------------------------
// ContinuousRecorder lifecycle — no ffmpeg / no RTSP needed
// ---------------------------------------------------------------------------

// disabledRecorderSettings returns a *conf.Settings with continuous recording disabled.
func disabledRecorderSettings() *conf.Settings {
	s := &conf.Settings{}
	s.Realtime.Audio.Continuous.Enabled = false
	return s
}

// TestContinuousRecorder_DisabledIsNoOp verifies that Start is a no-op when
// continuous recording is disabled, and that Stop is safe afterwards.
func TestContinuousRecorder_DisabledIsNoOp(t *testing.T) {
	t.Parallel()
	r := NewContinuousRecorder(disabledRecorderSettings())

	err := r.Start(context.Background())
	require.NoError(t, err, "Start should not error when disabled")

	// Stop must not block or panic.
	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop() blocked for >1s on a disabled recorder")
	}
}

// TestContinuousRecorder_StopBeforeStart verifies that Stop is safe to call
// before Start has ever been called.
func TestContinuousRecorder_StopBeforeStart(t *testing.T) {
	t.Parallel()
	r := NewContinuousRecorder(disabledRecorderSettings())

	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop() before Start() blocked for >1s")
	}
}

// TestContinuousRecorder_MultipleStopSafe verifies that calling Stop more than
// once does not panic or block.
func TestContinuousRecorder_MultipleStopSafe(t *testing.T) {
	t.Parallel()
	r := NewContinuousRecorder(disabledRecorderSettings())

	require.NoError(t, r.Start(context.Background()))
	r.Stop()
	r.Stop() // second call must not panic or block
}

// TestContinuousRecorder_StartIdempotent verifies that calling Start more than
// once on the same recorder (disabled) is safe and always returns nil.
func TestContinuousRecorder_StartIdempotent(t *testing.T) {
	t.Parallel()
	r := NewContinuousRecorder(disabledRecorderSettings())

	require.NoError(t, r.Start(context.Background()))
	require.NoError(t, r.Start(context.Background()), "second Start call should be a no-op")
	r.Stop()
}
