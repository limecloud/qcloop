package executor

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func executorTimeoutFromEnv(defaultTimeout time.Duration, names ...string) time.Duration {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
			return duration
		}
		if milliseconds, err := strconv.Atoi(value); err == nil && milliseconds > 0 {
			return time.Duration(milliseconds) * time.Millisecond
		}
	}
	return defaultTimeout
}
