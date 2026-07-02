package loader

import (
	"strconv"
	"strings"
	"time"
)

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseStoredTime(value any) time.Time {
	switch v := value.(type) {
	case nil:
		return time.Time{}
	case int64:
		return storedUnixTime(v)
	case int:
		return storedUnixTime(int64(v))
	case float64:
		return storedUnixTime(int64(v))
	case []byte:
		return parseStoredTimeString(string(v))
	case string:
		return parseStoredTimeString(v)
	default:
		return time.Time{}
	}
}

func parseStoredLoaderTriggerTime(value any) time.Time {
	return parseStoredTime(value)
}

func storedUnixTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value >= storedUnixMillisecondThreshold {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func parseStoredTimeString(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if numeric, err := strconv.ParseInt(value, 10, 64); err == nil {
		return storedUnixTime(numeric)
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
