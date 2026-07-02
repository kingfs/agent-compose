package agentcompose

import (
	"time"

	"agent-compose/pkg/agentcompose/loader"
)

func loaderTriggerNextFireAt(now time.Time, trigger LoaderTrigger, fired bool) (time.Time, error) {
	return loader.TriggerNextFireAt(now, trigger, fired)
}

func loaderTriggerSource(trigger LoaderTrigger) string {
	return loader.TriggerSource(trigger)
}

func normalizeLoaderCronSpecJSON(raw string) (string, error) {
	return loader.NormalizeCronSpecJSON(raw)
}

func loaderCronSpecJSON(expr, timezone string) (string, error) {
	return loader.CronSpecJSON(expr, timezone)
}
