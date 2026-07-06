// Package analysis provides the continuous full-audio recording service.
// This file implements ContinuousRecorder, which archives raw audio from every
// enabled RTSP stream in timestamped segments for later voice-print / speaker-ID
// analysis. It is fully decoupled from the detection pipeline.
package analysis

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/logger"
)

// componentContinuousRecorder is the logging component identifier for the recorder.
const componentContinuousRecorder = "analysis.recorder"

// Continuous recording constants. All durations are named to avoid magic numbers.
const (
	// continuousRecorderMaxRetentionCheckInterval caps the retention-check wakeup
	// frequency so that a long segment (e.g. 3600 s) does not delay cleanup for
	// an entire hour.
	continuousRecorderMaxRetentionCheckInterval = 900 * time.Second

	// continuousRecorderInitialBackoff is the starting sleep between ffmpeg restarts.
	continuousRecorderInitialBackoff = 5 * time.Second

	// continuousRecorderMaxBackoff caps the exponential back-off for failed streams.
	continuousRecorderMaxBackoff = 2 * time.Minute

	// continuousRecorderStableRuntime is the minimum ffmpeg uptime considered
	// a stable session; after such a run the back-off resets to initial.
	continuousRecorderStableRuntime = 2 * time.Minute

	// continuousRecorderCodecFLAC is the ffmpeg audio codec name for FLAC output.
	continuousRecorderCodecFLAC = "flac"

	// continuousRecorderCodecWAV is the ffmpeg audio codec name for WAV (PCM 16-bit LE) output.
	continuousRecorderCodecWAV = "pcm_s16le"

	// continuousRecorderExtFLAC is the file extension for FLAC segments.
	continuousRecorderExtFLAC = "flac"

	// continuousRecorderExtWAV is the file extension for WAV segments.
	continuousRecorderExtWAV = "wav"

	// continuousRecorderFormatFLAC is the config format name selecting FLAC encoding.
	continuousRecorderFormatFLAC = "flac"

	// continuousRecorderFormatWAV is the config format name selecting WAV encoding.
	continuousRecorderFormatWAV = "wav"

	// continuousRecorderChannels is the number of audio channels per segment (mono).
	continuousRecorderChannels = 1

	// continuousRecorderTransportDefault is the RTSP transport used when the stream
	// does not specify one explicitly.
	continuousRecorderTransportDefault = "tcp"

	// continuousRecorderDefaultBinary is the binary name resolved via exec.LookPath
	// when settings.Realtime.Audio.FfmpegPath is empty.
	continuousRecorderDefaultBinary = "ffmpeg"

	// continuousRecorderFilenamePattern is the strftime pattern embedded in each
	// segment filename so files sort chronologically.
	continuousRecorderFilenamePattern = "%Y%m%d_%H%M%S"

	// continuousRecorderDirPerm is the Unix permission mode for created directories.
	continuousRecorderDirPerm = 0o755
)

// unsafeNameChars matches any character that is not alphanumeric, underscore, or hyphen.
// Used to sanitize stream names into safe directory name components.
var unsafeNameChars = regexp.MustCompile(`[^a-zA-Z0-9_\-]+`)

// sanitizeSourceName replaces characters that are unsafe for directory names with
// underscores, then trims leading and trailing underscores. Returns "stream" when
// the result would otherwise be empty so the caller always has a valid path component.
func sanitizeSourceName(name string) string {
	safe := unsafeNameChars.ReplaceAllString(name, "_")
	safe = strings.Trim(safe, "_")
	if safe == "" {
		return "stream"
	}
	return safe
}

// isExpired reports whether a file's modification time is older than retentionHours
// relative to now. It is a pure, side-effect-free function that serves as the single
// testable decision point for the retention cleanup goroutine.
func isExpired(modTime, now time.Time, retentionHours int) bool {
	return now.Sub(modTime) > time.Duration(retentionHours)*time.Hour
}

// ContinuousRecorder manages one ffmpeg process per enabled RTSP stream,
// writing timestamped audio segment files for later voice-print / speaker-ID
// analysis. Recording is independent of detections: all audio is captured.
//
// Lifecycle: construct with NewContinuousRecorder, call Start once to begin
// recording, call Stop to terminate all processes and wait for goroutines.
// Stop is safe to call before Start (no-op).
type ContinuousRecorder struct {
	settings *conf.Settings

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex // guards started
	started bool
}

// NewContinuousRecorder creates a ContinuousRecorder from the given settings.
// Call Start to begin recording. Stop is safe to call before Start.
func NewContinuousRecorder(settings *conf.Settings) *ContinuousRecorder {
	return &ContinuousRecorder{settings: settings}
}

// Start begins continuous recording for all enabled RTSP streams. It is a no-op
// when continuous recording is disabled in settings. If the ffmpeg binary cannot
// be located a warning is logged and Start returns nil (graceful degradation).
// Calling Start more than once without an intervening Stop is safe (idempotent).
func (r *ContinuousRecorder) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return nil
	}

	cfg := r.settings.Realtime.Audio.Continuous
	if !cfg.Enabled {
		return nil
	}

	log := GetLogger()

	// Resolve the ffmpeg binary.
	ffmpegBin := r.settings.Realtime.Audio.FfmpegPath
	if ffmpegBin == "" {
		var lookErr error
		ffmpegBin, lookErr = exec.LookPath(continuousRecorderDefaultBinary)
		if lookErr != nil {
			log.Warn("ffmpeg not found; continuous recording will not start",
				logger.String("component", componentContinuousRecorder),
				logger.String("operation", "start"),
				logger.Error(lookErr))
			return nil
		}
	}

	recCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.started = true

	log.Info("starting continuous full-audio recorder",
		logger.String("path", cfg.Path),
		logger.Int("segment_seconds", cfg.SegmentSeconds),
		logger.Int("retention_hours", cfg.RetentionHours),
		logger.String("format", cfg.Format),
		logger.String("component", componentContinuousRecorder),
		logger.String("operation", "start"))

	// Determine codec and file extension from the configured format.
	codec := continuousRecorderCodecFLAC
	ext := continuousRecorderExtFLAC
	if cfg.Format == continuousRecorderFormatWAV {
		codec = continuousRecorderCodecWAV
		ext = continuousRecorderExtWAV
	}

	// Collect enabled streams before launching goroutines so the iterator is
	// not held open across goroutine boundaries.
	enabledStreams := make([]*conf.StreamConfig, 0, len(r.settings.Realtime.RTSP.Streams))
	for _, s := range r.settings.Realtime.RTSP.EnabledStreams() {
		enabledStreams = append(enabledStreams, s)
	}

	// Launch one ffmpeg goroutine per enabled RTSP stream.
	for _, s := range enabledStreams {
		stream := *s // copy by value to avoid aliasing the slice element
		safeName := sanitizeSourceName(stream.Name)
		outputDir := filepath.Join(cfg.Path, safeName)
		outputPattern := filepath.Join(outputDir, continuousRecorderFilenamePattern+"."+ext)
		args := buildContinuousFFmpegArgs(&stream, cfg, codec, outputPattern)

		r.wg.Go(func() {
			runContinuousStreamRecorder(recCtx, ffmpegBin, args, stream.Name, outputDir)
		})
	}

	// Single retention-cleanup goroutine covers all per-stream subdirectories.
	checkInterval := time.Duration(cfg.SegmentSeconds) * time.Second
	if checkInterval > continuousRecorderMaxRetentionCheckInterval {
		checkInterval = continuousRecorderMaxRetentionCheckInterval
	}
	r.wg.Go(func() {
		runContinuousRetentionLoop(recCtx, cfg, checkInterval)
	})

	return nil
}

// Stop cancels all ffmpeg processes and the retention goroutine, then waits for
// every goroutine to exit. Stop is idempotent and safe to call before Start.
func (r *ContinuousRecorder) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	started := r.started
	r.mu.Unlock()

	if !started || cancel == nil {
		return
	}

	cancel()
	r.wg.Wait()
}

// buildContinuousFFmpegArgs constructs the argument list for a single stream's
// ffmpeg segmenter process.
//
// The resulting command is equivalent to:
//
//	ffmpeg -hide_banner -loglevel error -rtsp_transport <transport> -i <url>
//	       -vn -ac 1 [-ar <samplerate>] -c:a <codec>
//	       -f segment -segment_time <secs> -strftime 1 -reset_timestamps 1
//	       <outputPattern>
func buildContinuousFFmpegArgs(stream *conf.StreamConfig, cfg conf.ContinuousRecordingSettings, codec, outputPattern string) []string {
	transport := stream.Transport
	if transport == "" {
		transport = continuousRecorderTransportDefault
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-rtsp_transport", transport,
		"-i", stream.URL,
		"-vn",
		"-ac", fmt.Sprintf("%d", continuousRecorderChannels),
	}

	if cfg.SampleRate > 0 {
		args = append(args, "-ar", fmt.Sprintf("%d", cfg.SampleRate))
	}

	args = append(args,
		"-c:a", codec,
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", cfg.SegmentSeconds),
		"-strftime", "1",
		"-reset_timestamps", "1",
		outputPattern,
	)

	return args
}

// runContinuousStreamRecorder creates outputDir, then runs ffmpeg in a restart
// loop with exponential back-off whenever the process exits unexpectedly.
// It exits cleanly when recCtx is cancelled.
func runContinuousStreamRecorder(
	recCtx context.Context,
	ffmpegBin string,
	args []string,
	streamName string,
	outputDir string,
) {
	log := GetLogger()

	// Create the output directory once; skip the stream on failure.
	if mkdirErr := os.MkdirAll(outputDir, continuousRecorderDirPerm); mkdirErr != nil {
		log.Warn("failed to create continuous recording directory, skipping stream",
			logger.String("stream", streamName),
			logger.String("dir", outputDir),
			logger.Error(mkdirErr),
			logger.String("component", componentContinuousRecorder),
			logger.String("operation", "create_dir"))
		return
	}

	backoff := continuousRecorderInitialBackoff

	for {
		// Honour cancellation before each attempt.
		select {
		case <-recCtx.Done():
			return
		default:
		}

		startTime := time.Now()
		//nolint:gosec // G204: ffmpegBin is from validated settings; args are built internally
		cmd := exec.CommandContext(recCtx, ffmpegBin, args...)

		if runErr := cmd.Run(); runErr != nil {
			// If the context was cancelled, treat the error as a clean exit.
			select {
			case <-recCtx.Done():
				return
			default:
			}

			log.Warn("continuous recording process exited unexpectedly, will restart",
				logger.String("stream", streamName),
				logger.Error(runErr),
				logger.Duration("backoff", backoff),
				logger.String("component", componentContinuousRecorder),
				logger.String("operation", "stream_restart"))

			// Reset back-off after a sufficiently long stable run.
			if time.Since(startTime) >= continuousRecorderStableRuntime {
				backoff = continuousRecorderInitialBackoff
			}
		} else {
			// Clean exit (context cancelled before Run returned an error).
			return
		}

		// Wait with back-off before restarting.
		select {
		case <-recCtx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential back-off with a hard cap.
		backoff *= 2
		if backoff > continuousRecorderMaxBackoff {
			backoff = continuousRecorderMaxBackoff
		}
	}
}

// runContinuousRetentionLoop ticks on checkInterval and deletes segment files
// whose mtime exceeds cfg.RetentionHours. Exits when recCtx is cancelled.
func runContinuousRetentionLoop(
	recCtx context.Context,
	cfg conf.ContinuousRecordingSettings,
	checkInterval time.Duration,
) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-recCtx.Done():
			return
		case <-ticker.C:
			runContinuousRetentionCleanup(cfg)
		}
	}
}

// runContinuousRetentionCleanup walks cfg.Path and removes regular files whose
// mtime is older than cfg.RetentionHours.
func runContinuousRetentionCleanup(cfg conf.ContinuousRecordingSettings) {
	log := GetLogger()
	now := time.Now()

	if walkErr := filepath.WalkDir(cfg.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries rather than aborting the entire walk.
			return nil //nolint:nilerr // intentional: skip the unreadable entry and continue the walk
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil //nolint:nilerr // intentional: skip entries whose info is unavailable, continue the walk
		}
		if isExpired(info.ModTime(), now, cfg.RetentionHours) {
			if removeErr := os.Remove(path); removeErr != nil {
				log.Warn("failed to remove expired recording segment",
					logger.String("path", path),
					logger.Error(removeErr),
					logger.String("component", componentContinuousRecorder),
					logger.String("operation", "retention_cleanup"))
			} else {
				log.Debug("removed expired recording segment",
					logger.String("path", path),
					logger.String("component", componentContinuousRecorder),
					logger.String("operation", "retention_cleanup"))
			}
		}
		return nil
	}); walkErr != nil {
		log.Warn("retention cleanup walk failed",
			logger.String("path", cfg.Path),
			logger.Error(walkErr),
			logger.String("component", componentContinuousRecorder),
			logger.String("operation", "retention_cleanup"))
	}
}
