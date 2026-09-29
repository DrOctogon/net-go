// speaker_cluster_ops_test.go: tests for the operator-facing speaker cluster
// seam (merge / forget) — ordering, name adoption, nil-safety, and immediate
// snapshot persistence.
package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/tphakala/voicewatch/internal/conf"
	"github.com/tphakala/voicewatch/internal/datastore"
	"github.com/tphakala/voicewatch/internal/datastore/mocks"
	"github.com/tphakala/voicewatch/internal/speaker"
)

const (
	opsTargetID  = "spk_1"
	opsSourceID  = "spk_2"
	opsAliceName = "Alice"
)

// newClusterOpsProcessor returns a Processor with SQLite-backed snapshot
// persistence in a temp dir, a mocked datastore, and two orthogonal clusters
// (spk_1 and spk_2) already assigned.
func newClusterOpsProcessor(t *testing.T) (p *Processor, mockDS *mocks.MockInterface) {
	t.Helper()
	mockDS = mocks.NewMockInterface(t)
	p = &Processor{Settings: sqliteSettings(t.TempDir()), Ds: mockDS}
	p.speakerClusterer = speaker.NewClusterer(0)
	require.Equal(t, opsTargetID, p.speakerClusterer.Assign(speakerOneHot(8, 0)))
	require.Equal(t, opsSourceID, p.speakerClusterer.Assign(speakerOneHot(8, 4)))
	return p, mockDS
}

func TestMergeSpeakerClusters_HappyPath(t *testing.T) {
	t.Parallel()

	p, mockDS := newClusterOpsProcessor(t)
	mockDS.EXPECT().ReassignSpeakerID(mock.Anything, opsSourceID, opsTargetID).Return(3, nil).Once()
	// Neither cluster is named: nothing to adopt, no name row to delete.
	mockDS.EXPECT().GetSpeakerNames(mock.Anything).Return(nil, nil).Once()

	require.NoError(t, p.MergeSpeakerClusters(t.Context(), opsTargetID, opsSourceID))

	assert.Equal(t, 1, p.speakerClusterer.NumClusters())
	assert.False(t, p.speakerClusterer.HasCluster(opsSourceID))

	// The snapshot was written immediately, not deferred to shutdown: a fresh
	// processor restoring from the same directory sees the merged state.
	restored := &Processor{Settings: p.Settings}
	restored.speakerClusterer = speaker.NewClusterer(0)
	restored.restoreSpeakerClusters()
	assert.Equal(t, 1, restored.speakerClusterer.NumClusters())
	assert.True(t, restored.speakerClusterer.HasCluster(opsTargetID))
}

func TestMergeSpeakerClusters_UnnamedTargetAdoptsSourceName(t *testing.T) {
	t.Parallel()

	p, mockDS := newClusterOpsProcessor(t)
	mockDS.EXPECT().ReassignSpeakerID(mock.Anything, opsSourceID, opsTargetID).Return(1, nil).Once()
	mockDS.EXPECT().GetSpeakerNames(mock.Anything).
		Return([]datastore.SpeakerName{{SpeakerID: opsSourceID, Name: opsAliceName}}, nil).Once()
	// Target is unnamed, so it adopts "Alice"; the source's row is deleted.
	mockDS.EXPECT().SetSpeakerName(mock.Anything, opsTargetID, opsAliceName).Return(nil).Once()
	mockDS.EXPECT().SetSpeakerName(mock.Anything, opsSourceID, "").Return(nil).Once()

	require.NoError(t, p.MergeSpeakerClusters(t.Context(), opsTargetID, opsSourceID))
}

func TestMergeSpeakerClusters_NamedTargetKeepsItsOwnName(t *testing.T) {
	t.Parallel()

	p, mockDS := newClusterOpsProcessor(t)
	mockDS.EXPECT().ReassignSpeakerID(mock.Anything, opsSourceID, opsTargetID).Return(1, nil).Once()
	mockDS.EXPECT().GetSpeakerNames(mock.Anything).Return([]datastore.SpeakerName{
		{SpeakerID: opsTargetID, Name: "Bob"},
		{SpeakerID: opsSourceID, Name: opsAliceName},
	}, nil).Once()
	// Only the source row is deleted; the target keeps "Bob".
	mockDS.EXPECT().SetSpeakerName(mock.Anything, opsSourceID, "").Return(nil).Once()

	require.NoError(t, p.MergeSpeakerClusters(t.Context(), opsTargetID, opsSourceID))

	mockDS.AssertNotCalled(t, "SetSpeakerName", mock.Anything, opsTargetID, mock.Anything)
}

func TestMergeSpeakerClusters_ValidationBeforeAnyWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  string
		source  string
		wantErr error
	}{
		{name: "same id", target: opsTargetID, source: opsTargetID, wantErr: speaker.ErrSameCluster},
		{name: "unknown target", target: "spk_9", source: opsSourceID, wantErr: speaker.ErrUnknownCluster},
		{name: "unknown source", target: opsTargetID, source: "spk_9", wantErr: speaker.ErrUnknownCluster},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, mockDS := newClusterOpsProcessor(t)

			err := p.MergeSpeakerClusters(t.Context(), tt.target, tt.source)

			require.ErrorIs(t, err, tt.wantErr)
			// Nothing may reach the database when the ids do not check out.
			mockDS.AssertNotCalled(t, "ReassignSpeakerID", mock.Anything, mock.Anything, mock.Anything)
			mockDS.AssertNotCalled(t, "GetSpeakerNames", mock.Anything)
			assert.Equal(t, 2, p.speakerClusterer.NumClusters())
		})
	}
}

func TestMergeSpeakerClusters_DatastoreFailureLeavesClustererUntouched(t *testing.T) {
	t.Parallel()

	p, mockDS := newClusterOpsProcessor(t)
	mockDS.EXPECT().ReassignSpeakerID(mock.Anything, opsSourceID, opsTargetID).
		Return(0, assert.AnError).Once()

	err := p.MergeSpeakerClusters(t.Context(), opsTargetID, opsSourceID)

	require.Error(t, err)
	assert.Equal(t, 2, p.speakerClusterer.NumClusters(),
		"database first: a failed relabel must leave both clusters in memory so the call is retryable")
	assert.True(t, p.speakerClusterer.HasCluster(opsSourceID))
}

func TestForgetSpeakerCluster_HappyPath(t *testing.T) {
	t.Parallel()

	p, mockDS := newClusterOpsProcessor(t)
	mockDS.EXPECT().ClearSpeakerID(mock.Anything, opsSourceID).Return(4, nil).Once()
	mockDS.EXPECT().SetSpeakerName(mock.Anything, opsSourceID, "").Return(nil).Once()

	require.NoError(t, p.ForgetSpeakerCluster(t.Context(), opsSourceID))

	assert.Equal(t, 1, p.speakerClusterer.NumClusters())
	assert.False(t, p.speakerClusterer.HasCluster(opsSourceID))

	// Forgotten ids are retired, never reissued — even after a restart.
	restored := &Processor{Settings: p.Settings}
	restored.speakerClusterer = speaker.NewClusterer(0)
	restored.restoreSpeakerClusters()
	assert.Equal(t, "spk_3", restored.speakerClusterer.Assign(speakerOneHot(8, 7)))
}

func TestForgetSpeakerCluster_UnknownIDDoesNotWrite(t *testing.T) {
	t.Parallel()

	p, mockDS := newClusterOpsProcessor(t)

	err := p.ForgetSpeakerCluster(t.Context(), "spk_9")

	require.ErrorIs(t, err, speaker.ErrUnknownCluster)
	mockDS.AssertNotCalled(t, "ClearSpeakerID", mock.Anything, mock.Anything)
	assert.Equal(t, 2, p.speakerClusterer.NumClusters())
}

func TestForgetSpeakerCluster_DatastoreFailureLeavesClustererUntouched(t *testing.T) {
	t.Parallel()

	p, mockDS := newClusterOpsProcessor(t)
	mockDS.EXPECT().ClearSpeakerID(mock.Anything, opsSourceID).Return(0, assert.AnError).Once()

	err := p.ForgetSpeakerCluster(t.Context(), opsSourceID)

	require.Error(t, err)
	assert.Equal(t, 2, p.speakerClusterer.NumClusters())
}

func TestSpeakerClusterOps_UnavailableWhenClusteringDisabled(t *testing.T) {
	t.Parallel()

	// Voice-print attributes disabled: no clusterer at all.
	p := &Processor{Settings: &conf.Settings{}}

	require.ErrorIs(t, p.MergeSpeakerClusters(t.Context(), opsTargetID, opsSourceID),
		ErrSpeakerClusteringUnavailable)
	require.ErrorIs(t, p.ForgetSpeakerCluster(t.Context(), opsTargetID),
		ErrSpeakerClusteringUnavailable)
}

func TestSpeakerClusterOps_UnavailableWithoutDatastore(t *testing.T) {
	t.Parallel()

	p := &Processor{Settings: &conf.Settings{}}
	p.speakerClusterer = speaker.NewClusterer(0)

	require.ErrorIs(t, p.MergeSpeakerClusters(t.Context(), opsTargetID, opsSourceID),
		ErrDatastoreUnavailable)
	require.ErrorIs(t, p.ForgetSpeakerCluster(t.Context(), opsTargetID),
		ErrDatastoreUnavailable)
}
