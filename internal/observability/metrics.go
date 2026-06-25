// Package observability provides metrics and monitoring capabilities for the VoiceWatch application.
package observability

import (
	"fmt"
	stdlog "log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tphakala/voicewatch/internal/diskmanager"
	"github.com/tphakala/voicewatch/internal/observability/metrics"
)

// Metrics holds all the metric collectors for the application.
type Metrics struct {
	registry      *prometheus.Registry
	MQTT          *metrics.MQTTMetrics
	VoiceWatch       *metrics.VoiceWatchMetrics
	ImageProvider *metrics.ImageProviderMetrics
	DiskManager   *metrics.DiskManagerMetrics
	Weather       *metrics.WeatherMetrics
	SunCalc       *metrics.SunCalcMetrics
	Datastore     *metrics.DatastoreMetrics
	MyAudio       *metrics.MyAudioMetrics
	SoundLevel    *metrics.SoundLevelMetrics
	HTTP          *metrics.HTTPMetrics
	Notification  *metrics.NotificationMetrics
}

// NewMetrics creates a new instance of Metrics, initializing all metric collectors.
// It returns an error if any metric collector fails to initialize.
func NewMetrics() (*Metrics, error) {
	registry := prometheus.NewRegistry()

	mqttMetrics, err := metrics.NewMQTTMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create MQTT metrics: %w", err)
	}

	voicewatchMetrics, err := metrics.NewVoiceWatchMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create VoiceWatch metrics: %w", err)
	}

	imageProviderMetrics, err := metrics.NewImageProviderMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create ImageProvider metrics: %w", err)
	}

	diskManagerMetrics, err := metrics.NewDiskManagerMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create DiskManager metrics: %w", err)
	}

	weatherMetrics, err := metrics.NewWeatherMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create Weather metrics: %w", err)
	}

	sunCalcMetrics, err := metrics.NewSunCalcMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create SunCalc metrics: %w", err)
	}

	datastoreMetrics, err := metrics.NewDatastoreMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create Datastore metrics: %w", err)
	}

	myAudioMetrics, err := metrics.NewMyAudioMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create MyAudio metrics: %w", err)
	}

	soundLevelMetrics, err := metrics.NewSoundLevelMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create SoundLevel metrics: %w", err)
	}

	httpMetrics, err := metrics.NewHTTPMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP metrics: %w", err)
	}

	notificationMetrics, err := metrics.NewNotificationMetrics(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to create Notification metrics: %w", err)
	}

	m := &Metrics{
		registry:      registry,
		MQTT:          mqttMetrics,
		VoiceWatch:    voicewatchMetrics,
		ImageProvider: imageProviderMetrics,
		DiskManager:   diskManagerMetrics,
		Weather:       weatherMetrics,
		SunCalc:       sunCalcMetrics,
		Datastore:     datastoreMetrics,
		MyAudio:       myAudioMetrics,
		SoundLevel:    soundLevelMetrics,
		HTTP:          httpMetrics,
		Notification:  notificationMetrics,
	}

	// Initialize tracing with metrics
	initializeTracing(voicewatchMetrics)

	// Initialize diskmanager with metrics
	diskmanager.SetMetrics(diskManagerMetrics)

	return m, nil
}

// RegisterHandlers registers the metrics endpoint with the provided http.ServeMux.
func (m *Metrics) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/metrics", m.metricsHandler)
}

// metricsHandler is the HTTP handler for the /metrics endpoint.
func (m *Metrics) metricsHandler(w http.ResponseWriter, r *http.Request) {
	h := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		ErrorLog:      stdlog.New(os.Stderr, "metrics handler: ", stdlog.LstdFlags),
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	h.ServeHTTP(w, r)
}

// initializeTracing previously wired metrics into the classifier tracing layer.
// The classifier tracing/span subsystem was removed in the human-voice pivot, so
// this is now a no-op retained for call-site and test compatibility.
func initializeTracing(_ *metrics.VoiceWatchMetrics) {}
