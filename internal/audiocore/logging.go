package audiocore

import "github.com/tphakala/voicewatch/internal/logger"

// GetLogger returns the audiocore module logger.
func GetLogger() logger.Logger {
	return logger.Global().Module("audiocore")
}
