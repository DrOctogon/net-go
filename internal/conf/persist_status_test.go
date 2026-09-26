package conf

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigPersistFailureRecording(t *testing.T) {
	// Not parallel: exercises process-wide state.
	recordConfigPersistFailure("session_secret", "/tmp/config.yaml", errors.New("disk full"))

	f := LastConfigPersistFailure()
	require.NotNil(t, f)
	assert.Equal(t, "session_secret", f.Operation)
	assert.Equal(t, "/tmp/config.yaml", f.Path)
	assert.Equal(t, "disk full", f.Error)
	assert.False(t, f.At.IsZero())
}
