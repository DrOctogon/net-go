package logger

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIP verifies the IP field constructor hashes client addresses for safe
// logging: the literal address never appears, hashing is deterministic (so log
// lines for the same client still correlate), and the IP category is preserved
// as a prefix for operational triage.
func TestIP(t *testing.T) {
	t.Parallel()

	const publicIP = "203.0.113.5"

	t.Run("public IP is hashed with a public prefix and never leaks the literal", func(t *testing.T) {
		t.Parallel()
		f := IP("ip", publicIP)
		assert.Equal(t, "ip", f.Key, "key is passed through and interned")
		got, ok := f.Value.(string)
		require.True(t, ok, "IP value must be a string")
		assert.True(t, strings.HasPrefix(got, "public-"), "public IP keeps a public- category prefix, got %q", got)
		assert.NotContains(t, got, publicIP, "the literal IP must never appear in the hashed output")
	})

	t.Run("hashing is deterministic for correlation", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, IP("ip", publicIP).Value, IP("client_ip", publicIP).Value,
			"same IP must hash identically regardless of field key so log lines correlate")
	})

	t.Run("distinct IPs produce distinct hashes", func(t *testing.T) {
		t.Parallel()
		assert.NotEqual(t, IP("ip", publicIP).Value, IP("ip", "198.51.100.9").Value)
	})

	t.Run("private IP keeps a private prefix", func(t *testing.T) {
		t.Parallel()
		got, ok := IP("ip", "192.168.1.50").Value.(string)
		require.True(t, ok)
		assert.True(t, strings.HasPrefix(got, "private-"), "got %q", got)
	})

	t.Run("empty IP yields an empty value", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, IP("ip", "").Value)
	})

	t.Run("non-IP input still gets a deterministic hashed fallback, never raw", func(t *testing.T) {
		t.Parallel()
		got, ok := IP("ip", "not-an-ip").Value.(string)
		require.True(t, ok)
		assert.True(t, strings.HasPrefix(got, "invalid-ip-"), "got %q", got)
		assert.NotContains(t, got, "not-an-ip")
	})
}
