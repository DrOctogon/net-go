// orchestrator.go hosts the single active classifier model.
//
// Human-voice pivot: the multi-model orchestrator (primary BirdNET + secondary
// Perch/Bat, range-filter coordination, name-resolver chain, RSS tracking,
// nighttime scheduling, model gallery) has been replaced by a thin single-model
// host. The concrete ModelInstance (humanvoice.Model) is constructed in the
// analysis layer and injected here, which avoids a humanvoice -> classifier
// import cycle.
package classifier

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore"
	"github.com/tphakala/birdnet-go/internal/errors"
)

// Orchestrator owns the single loaded model and serializes inference against it.
type Orchestrator struct {
	settingsAtomic atomic.Pointer[conf.Settings]
	// inferenceMu serializes Predict calls; ModelInstance implementations are not
	// guaranteed goroutine-safe.
	inferenceMu sync.Mutex
	primary     ModelInstance
}

// NewOrchestrator creates a single-model orchestrator around the provided
// model. The model is constructed by the caller (analysis layer) to avoid an
// import cycle. A nil model is rejected.
func NewOrchestrator(settings *conf.Settings, primary ModelInstance) (*Orchestrator, error) {
	if primary == nil {
		return nil, errors.Newf("classifier: nil model instance").
			Category(errors.CategoryModelInit).
			Component("classifier").
			Build()
	}
	o := &Orchestrator{primary: primary}
	o.settingsAtomic.Store(settings)
	return o, nil
}

// CurrentSettings returns the most recently stored settings snapshot.
func (o *Orchestrator) CurrentSettings() *conf.Settings {
	return o.settingsAtomic.Load()
}

// SetSettings stores a new settings snapshot (hot-reload).
func (o *Orchestrator) SetSettings(settings *conf.Settings) {
	o.settingsAtomic.Store(settings)
}

// PredictModel runs inference on the active model. The modelID parameter is
// retained for call-site compatibility but ignored: there is exactly one model.
func (o *Orchestrator) PredictModel(ctx context.Context, _ string, sample [][]float32) ([]datastore.Results, error) {
	o.inferenceMu.Lock()
	defer o.inferenceMu.Unlock()
	return o.primary.Predict(ctx, sample)
}

// Predict runs inference on the active model.
func (o *Orchestrator) Predict(ctx context.Context, sample [][]float32) ([]datastore.Results, error) {
	return o.PredictModel(ctx, o.primary.ModelID(), sample)
}

// ModelSpecFor returns the audio spec of the active model. The modelID is
// ignored (single model).
func (o *Orchestrator) ModelSpecFor(_ string) (ModelSpec, error) {
	return o.primary.Spec(), nil
}

// Spec returns the active model's audio spec.
func (o *Orchestrator) Spec() ModelSpec { return o.primary.Spec() }

// NumSpecies returns the active model's class count.
func (o *Orchestrator) NumSpecies() int { return o.primary.NumSpecies() }

// Labels returns the active model's labels.
func (o *Orchestrator) Labels() []string { return o.primary.Labels() }

// AllLabels returns the active model's labels (single model, so identical to Labels).
func (o *Orchestrator) AllLabels() []string { return o.primary.Labels() }

// ModelID returns the active model's registry ID.
func (o *Orchestrator) ModelID() string { return o.primary.ModelID() }

// ModelInfos returns metadata for the loaded model.
func (o *Orchestrator) ModelInfos() []ModelInfo {
	info := ModelRegistry[o.primary.ModelID()]
	info.Spec = o.primary.Spec()
	info.NumSpecies = o.primary.NumSpecies()
	return []ModelInfo{info}
}

// ReloadModel is a no-op in the single-model host: the model is constructed and
// owned by the analysis layer, which rebuilds the orchestrator on config change.
func (o *Orchestrator) ReloadModel() error { return nil }

// Delete releases the loaded model.
func (o *Orchestrator) Delete() {
	if o.primary != nil {
		_ = o.primary.Close()
		o.primary = nil
	}
}
