package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const StoredUnixMillisecondThreshold int64 = 10_000_000_000

type EnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret,omitempty"`
}

func NormalizeEnvItems(items []EnvVar) []EnvVar {
	if len(items) == 0 {
		return nil
	}
	merged := make(map[string]EnvVar, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		item.Name = name
		merged[name] = item
	}
	if len(merged) == 0 {
		return nil
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]EnvVar, 0, len(keys))
	for _, key := range keys {
		result = append(result, merged[key])
	}
	return result
}

func ParseStoredUnixTimeAuto(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value >= StoredUnixMillisecondThreshold {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func ParseStoredTime(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case int64:
		return ParseStoredUnixTimeAuto(typed)
	case int:
		return ParseStoredUnixTimeAuto(int64(typed))
	case float64:
		return ParseStoredUnixTimeAuto(int64(typed))
	case []byte:
		return ParseStoredTime(string(typed))
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}
		}
		if unixValue, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return ParseStoredUnixTimeAuto(unixValue)
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
			if parsed, err := time.Parse(layout, trimmed); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func ParseStoredLoaderTriggerTime(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case int64:
		return ParseStoredUnixTimeAuto(typed)
	case int:
		return ParseStoredUnixTimeAuto(int64(typed))
	case float64:
		return ParseStoredUnixTimeAuto(int64(typed))
	case []byte:
		return ParseStoredLoaderTriggerTime(string(typed))
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}
		}
		if unixValue, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return ParseStoredUnixTimeAuto(unixValue)
		}
		return ParseStoredTime(trimmed)
	default:
		return ParseStoredTime(value)
	}
}

func NormalizeSQLiteTimestampExpr(columnName string) string {
	return fmt.Sprintf(`CASE
		WHEN trim(COALESCE(%[1]s, '')) = '' THEN CAST(strftime('%%s','now') AS INTEGER)
		WHEN trim(COALESCE(%[1]s, '')) NOT GLOB '*[^0-9]*' THEN CAST(%[1]s AS INTEGER)
		ELSE COALESCE(CAST(strftime('%%s', %[1]s) AS INTEGER), CAST(strftime('%%s','now') AS INTEGER))
	END`, columnName)
}

func BoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
