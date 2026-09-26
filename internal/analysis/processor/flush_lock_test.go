// flush_lock_test.go: verifies flushPendingDetections does not hold
// pendingMutex across the heavy per-detection work (speaker-attribute
// inference), so detection ingestion is never stalled behind a slow model.
package processor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/analysis/jobqueue"
	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/speaker"
)

// gatedSpeakerAnalyzer blocks inside Analyze until released, signalling entry.
type gatedSpeakerAnalyzer struct {
	entered chan struct{}
	release chan struct{}
}

func (g *gatedSpeakerAnalyzer) Analyze(_ context.Context, _ [][]float32) (speaker.Attributes, error) {
	close(g.entered)
	<-g.release
	return speaker.Attributes{}, nil
}

func TestFlushRunsSpeakerAnalysisOutsidePendingMutex(t *testing.T) {
	// Not parallel: uses the global settings fallback (withFallbackSettings).
	withFallbackSettings(t)

	settings := &conf.Settings{}
	settings.Realtime.Audio.SpeakerAttributes.Enabled = true
	settings.Realtime.Species.Config = make(map[string]conf.SpeciesConfig)

	queue := jobqueue.NewJobQueue()
	queue.Start()
	t.Cleanup(func() { _ = queue.Stop() })

	analyzer := &gatedSpeakerAnalyzer{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	p := &Processor{
		Settings:          settings,
		JobQueue:          queue,
		speakerAnalyzer:   analyzer,
		pendingDetections: make(map[string]PendingDetection),
	}

	item := *pendingDetectionWithPCM()
	item.Count = 100 // comfortably above any minimum-detections threshold
	item.FlushDeadline = time.Now().Add(-time.Second)
	p.pendingDetections["k"] = item

	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		p.flushPendingDetections()
	}()

	// Wait until the flusher is inside the (slow) speaker analysis.
	select {
	case <-analyzer.entered:
	case <-time.After(5 * time.Second):
		close(analyzer.release)
		t.Fatal("speaker analysis was never reached by the flush")
	}

	// Ingestion must be able to take pendingMutex while analysis is running.
	acquired := make(chan struct{})
	go func() {
		p.pendingMutex.Lock()
		defer p.pendingMutex.Unlock()
		close(acquired)
	}()

	select {
	case <-acquired:
		// Lock acquirable mid-analysis: inference is outside the mutex.
	case <-time.After(2 * time.Second):
		close(analyzer.release)
		t.Fatal("pendingMutex held during speaker analysis: ingestion would stall behind inference")
	}

	close(analyzer.release)
	select {
	case <-flushDone:
	case <-time.After(5 * time.Second):
		t.Fatal("flush did not complete after analyzer release")
	}

	require.Empty(t, p.pendingDetections, "approved detection should be removed from the pending map")
	assert.NotNil(t, analyzer)
}
