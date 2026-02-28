package llm

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// logLevel controls debug output
var logLevel = strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))

var (
	debugMu     sync.RWMutex
	debugLogger func(format string, args ...any)
)

// SetDebugLogger sets an optional sink for debug logs.
// If unset, logs are printed to stdout when LOG_LEVEL=debug.
func SetDebugLogger(fn func(format string, args ...any)) {
	debugMu.Lock()
	defer debugMu.Unlock()
	debugLogger = fn
}

func isDebugEnabled() bool {
	return logLevel == "debug"
}

// Debug logs a debug message if LOG_LEVEL=debug
func Debug(format string, args ...any) {
	if !isDebugEnabled() {
		return
	}

	debugMu.RLock()
	fn := debugLogger
	debugMu.RUnlock()

	if fn != nil {
		fn(format, args...)
		return
	}

	fmt.Printf("[debug] "+format+"\n", args...)
}
