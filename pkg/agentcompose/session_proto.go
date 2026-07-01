package agentcompose

import (
	sessionmodel "agent-compose/pkg/agentcompose/session"
	"time"

	agentcomposev1 "agent-compose/proto/agentcompose/v1"
)

func sessionListOptionsFromProto(req *agentcomposev1.ListSessionsRequest) (SessionListOptions, error) {
	return sessionmodel.ListOptionsFromProto(req)
}

func parseOptionalRFC3339(raw, field string) (time.Time, error) {
	return sessionmodel.ParseOptionalRFC3339(raw, field)
}
