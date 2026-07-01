package project

import "strings"

type SessionRelationFilter struct {
	ProjectID string
	AgentName string
	SessionID string
	Statuses  []string
	Limit     int
}

func NormalizeRunStatusFilter(statuses []string) []string {
	seen := make(map[string]struct{}, len(statuses))
	normalized := make([]string, 0, len(statuses))
	for _, status := range statuses {
		status = strings.ToLower(strings.TrimSpace(status))
		if status == "" {
			continue
		}
		switch status {
		case RunStatusPending, RunStatusRunning, RunStatusSucceeded, RunStatusFailed, RunStatusCanceled:
		default:
			continue
		}
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		normalized = append(normalized, status)
	}
	return normalized
}

func Placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	values := make([]string, count)
	for i := range values {
		values[i] = "?"
	}
	return strings.Join(values, ",")
}
