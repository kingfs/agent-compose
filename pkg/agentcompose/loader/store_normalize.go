package loader

import (
	driverpkg "agent-compose/pkg/driver"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func normalizeLoader(item Loader, assignID bool) (Loader, error) {
	now := time.Now().UTC()
	item.Summary.ID = strings.TrimSpace(item.Summary.ID)
	if assignID && item.Summary.ID == "" {
		item.Summary.ID = uuid.NewString()
	}
	if item.Summary.ID == "" {
		return Loader{}, fmt.Errorf("loader id is required")
	}
	item.Summary.Name = strings.TrimSpace(item.Summary.Name)
	if item.Summary.Name == "" {
		item.Summary.Name = defaultLoaderName(now)
	}
	item.Summary.Description = strings.TrimSpace(item.Summary.Description)
	runtime, err := normalizeLoaderRuntime(item.Summary.Runtime)
	if err != nil {
		return Loader{}, err
	}
	item.Summary.Runtime = runtime
	item.Script = strings.ReplaceAll(item.Script, "\r\n", "\n")
	if strings.TrimSpace(item.Script) == "" {
		return Loader{}, fmt.Errorf("loader script is required")
	}
	item.Summary.WorkspaceID = strings.TrimSpace(item.Summary.WorkspaceID)
	item.Summary.AgentID = strings.TrimSpace(item.Summary.AgentID)
	item.Summary.Driver = strings.TrimSpace(item.Summary.Driver)
	if item.Summary.Driver != "" {
		driver, err := driverpkg.ResolveSessionRuntimeDriver(item.Summary.Driver, item.Summary.Driver)
		if err != nil {
			return Loader{}, err
		}
		item.Summary.Driver = driver
	}
	item.Summary.GuestImage = strings.TrimSpace(item.Summary.GuestImage)
	item.Summary.DefaultAgent = normalizeAgentKind(item.Summary.DefaultAgent)
	if item.Summary.DefaultAgent == "" {
		item.Summary.DefaultAgent = "codex"
	}
	item.Summary.SessionPolicy = normalizeLoaderSessionPolicy(item.Summary.SessionPolicy)
	item.Summary.ConcurrencyPolicy = normalizeLoaderConcurrencyPolicy(item.Summary.ConcurrencyPolicy)
	item.Summary.CapsetIDs = normalizeCapsetIDs(item.Summary.CapsetIDs)
	item.Summary.ManagedProjectID = strings.TrimSpace(item.Summary.ManagedProjectID)
	item.Summary.ManagedAgentName = strings.TrimSpace(item.Summary.ManagedAgentName)
	item.Summary.ManagedSchedulerID = strings.TrimSpace(item.Summary.ManagedSchedulerID)
	if item.Summary.ManagedProjectID == "" {
		item.Summary.ManagedRevision = 0
		item.Summary.ManagedAgentName = ""
		item.Summary.ManagedSchedulerID = ""
	} else {
		if item.Summary.ManagedAgentName == "" || item.Summary.ManagedSchedulerID == "" {
			return Loader{}, fmt.Errorf("managed loader agent name and scheduler id are required")
		}
		if item.Summary.ManagedRevision < 0 {
			return Loader{}, fmt.Errorf("managed loader project revision cannot be negative")
		}
	}
	item.EnvItems = normalizeEnvItems(item.EnvItems)
	item.Triggers = append([]LoaderTrigger(nil), item.Triggers...)
	return item, nil
}

func Normalize(item Loader, assignID bool) (Loader, error) {
	return normalizeLoader(item, assignID)
}

func normalizeLoaderTrigger(loaderID string, trigger LoaderTrigger) (LoaderTrigger, error) {
	trigger.LoaderID = strings.TrimSpace(loaderID)
	trigger.ID = strings.TrimSpace(trigger.ID)
	if trigger.LoaderID == "" {
		return LoaderTrigger{}, fmt.Errorf("loader id is required")
	}
	if trigger.ID == "" {
		return LoaderTrigger{}, fmt.Errorf("loader trigger id is required")
	}
	kind, err := normalizeLoaderTriggerKind(trigger.Kind)
	if err != nil {
		return LoaderTrigger{}, err
	}
	trigger.Kind = kind
	trigger.Topic = strings.TrimSpace(trigger.Topic)
	switch trigger.Kind {
	case LoaderTriggerKindInterval:
		if trigger.IntervalMs <= 0 {
			return LoaderTrigger{}, fmt.Errorf("loader interval trigger %s requires a positive interval", trigger.ID)
		}
		trigger.Topic = ""
	case LoaderTriggerKindEvent:
		if trigger.Topic == "" {
			return LoaderTrigger{}, fmt.Errorf("loader event trigger %s requires a topic", trigger.ID)
		}
		trigger.IntervalMs = 0
	case LoaderTriggerKindTimeout:
		if trigger.IntervalMs <= 0 {
			return LoaderTrigger{}, fmt.Errorf("loader timeout trigger %s requires a positive delay", trigger.ID)
		}
		trigger.Topic = ""
	case LoaderTriggerKindCron:
		trigger.Topic = ""
		trigger.IntervalMs = 0
		normalizedSpecJSON, err := normalizeLoaderCronSpecJSON(trigger.SpecJSON)
		if err != nil {
			return LoaderTrigger{}, fmt.Errorf("loader cron trigger %s: %w", trigger.ID, err)
		}
		trigger.SpecJSON = normalizedSpecJSON
	}
	trigger.SpecJSON = strings.TrimSpace(trigger.SpecJSON)
	if trigger.SpecJSON == "" {
		trigger.SpecJSON = "{}"
	}
	if !timeIsSet(trigger.NextFireAt) {
		trigger.NextFireAt = time.Time{}
	} else {
		trigger.NextFireAt = trigger.NextFireAt.UTC()
	}
	if !timeIsSet(trigger.LastFiredAt) {
		trigger.LastFiredAt = time.Time{}
	} else {
		trigger.LastFiredAt = trigger.LastFiredAt.UTC()
	}
	return trigger, nil
}

func NormalizeTrigger(loaderID string, trigger LoaderTrigger) (LoaderTrigger, error) {
	return normalizeLoaderTrigger(loaderID, trigger)
}

func encodeLoaderEnvItems(items []SessionEnvVar) (string, error) {
	normalized := normalizeEnvItems(items)
	if normalized == nil {
		normalized = []SessionEnvVar{}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode loader env items: %w", err)
	}
	return string(data), nil
}

func decodeLoaderEnvItems(raw string) ([]SessionEnvVar, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var items []SessionEnvVar
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("decode loader env items: %w", err)
	}
	return normalizeEnvItems(items), nil
}

// encodeCapsetIDs marshals the capset id set to the JSON stored in the
// capset_ids column ("[]" when empty).
func encodeCapsetIDs(ids []string) (string, error) {
	normalized := normalizeCapsetIDs(ids)
	if normalized == nil {
		normalized = []string{}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode capset ids: %w", err)
	}
	return string(data), nil
}

func EncodeCapsetIDs(ids []string) (string, error) {
	return encodeCapsetIDs(ids)
}

func decodeCapsetIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return normalizeCapsetIDs(ids)
}

func DecodeCapsetIDs(raw string) []string {
	return decodeCapsetIDs(raw)
}

func normalizeAgentKind(agent string) string {
	agent = strings.ToLower(strings.TrimSpace(agent))
	switch agent {
	case "":
		return ""
	case "codex":
		return "codex"
	case "claude", "claude-code", "claude_code":
		return "claude"
	case "gemini", "gemini-cli", "gemini_cli":
		return "gemini"
	case "opencode", "open-code", "open_code":
		return "opencode"
	default:
		return agent
	}
}

func normalizeCapsetIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func normalizeEnvItems(items []SessionEnvVar) []SessionEnvVar {
	if len(items) == 0 {
		return nil
	}
	merged := make(map[string]SessionEnvVar, len(items))
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
	result := make([]SessionEnvVar, 0, len(keys))
	for _, key := range keys {
		result = append(result, merged[key])
	}
	return result
}
