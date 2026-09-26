package v2only

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tphakala/voicewatch/internal/logger"
)

// fakeResolver is a minimal datastore.SpeciesNameResolver for tests.
type fakeResolver struct{ m map[string]string }

func (f fakeResolver) Resolve(sci, _ string) string { return f.m[sci] }

func (f fakeResolver) ResolveLocal(sci string) (string, bool) {
	v, ok := f.m[sci]
	return v, ok
}

func TestResolveCommonName_ResolverOverridesLabelMap(t *testing.T) {
	ds := &Datastore{log: logger.NewConsoleLogger("v2only_test", logger.LogLevelError)}
	ds.names.Store(buildNameMaps([]string{"Turdus merula_LabelName"}, nil))

	// Without a resolver: the label map wins.
	assert.Equal(t, "LabelName", ds.resolveCommonName("Turdus merula"))

	// With a resolver: override even though the label map already has a name.
	ds.SetNameResolver(fakeResolver{m: map[string]string{"Turdus merula": "Localized"}})
	assert.Equal(t, "Localized", ds.resolveCommonName("Turdus merula"))

	// Resolver miss on a species not in labels: falls back to the scientific name.
	assert.Equal(t, "Myotis myotis", ds.resolveCommonName("Myotis myotis"))
}

func TestBuildNameMaps_ScientificOnlyLabelSearchable(t *testing.T) {
	// Scientific-only labels (no "_", e.g. Perch v2 / bats) become searchable when
	// the resolver supplies a common name.
	nm := buildNameMaps([]string{"Myotis myotis"},
		fakeResolver{m: map[string]string{"Myotis myotis": "Mustakorvayokko"}})
	assert.Equal(t, "Mustakorvayokko", nm.common["Myotis myotis"])
	assert.Equal(t, "Myotis myotis", nm.species["mustakorvayokko"])

	// Without a resolver, a scientific-only label has no common name and is skipped.
	bare := buildNameMaps([]string{"Myotis myotis"}, nil)
	_, ok := bare.common["Myotis myotis"]
	assert.False(t, ok)
}

func TestBuildNameMaps_ResolverLocalizesReverseMap(t *testing.T) {
	// The reverse (search) maps must carry the localized name so search matches display.
	nm := buildNameMaps([]string{"Turdus merula_LabelName"},
		fakeResolver{m: map[string]string{"Turdus merula": "Mustarastas"}})
	assert.Equal(t, "Mustarastas", nm.common["Turdus merula"])
	assert.Equal(t, "Turdus merula", nm.species["mustarastas"])
}
