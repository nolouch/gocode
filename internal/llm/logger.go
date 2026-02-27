package llm

import (
	"fmt"
	"os"
)

// logLevel controls debug output
var logLevel = os.Getenv("LOG_LEVEL")

// Debug logs a debug message if LOG_LEVEL=debug
func Debug(format string, args ...interface{}) {
	if logLevel == "debug" {
		fmt.Printf("[debug] "+format+"\n", args...)
	}
}
