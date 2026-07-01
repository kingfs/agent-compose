package agentcompose

import (
	sessionmodel "agent-compose/pkg/agentcompose/session"
	"time"
)

const defaultSessionListLimit = 50

func normalizeSessionTriggerSource(value string, tags []SessionTag) string {
	return sessionmodel.NormalizeTriggerSource(value, tags)
}

func sessionTypeFromTriggerSource(value string) string {
	return sessionmodel.TypeFromTriggerSource(value)
}

func normalizeSessionListBounds(offset, limit int) (int, int) {
	return sessionmodel.NormalizeListBounds(offset, limit)
}

func paginateSessions(items []*Session, offset, limit int) []*Session {
	return sessionmodel.Paginate(items, offset, limit)
}

func sessionMatchesListOptions(session *Session, options SessionListOptions) bool {
	return sessionmodel.MatchesListOptions(session, options)
}

func matchesTimeRange(value, from, to time.Time) bool {
	return sessionmodel.MatchesTimeRange(value, from, to)
}
