package qjs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fastschema/qjs"

	"agent-compose/pkg/agentcompose/loader"
)

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

func parseLoaderAgentRequest(args []*qjs.Value) (LoaderAgentRequest, error) {
	request := LoaderAgentRequest{}
	if len(args) < 2 || args[1] == nil || args[1].IsUndefined() || args[1].IsNull() {
		return request, nil
	}
	options, err := loaderAgentOptionsWithoutSchema(args[1])
	if err != nil {
		return LoaderAgentRequest{}, fmt.Errorf("decode scheduler.agent options: %w", err)
	}
	request.Agent = normalizeAgentKind(loaderStringOption(options, "agent"))
	request.SessionPolicy = loader.NormalizeSessionPolicy(loaderStringOption(options, "sessionPolicy", "session_policy"))
	request.Timeout, err = loaderDurationOption(options, "timeout", "agentTimeout", "agent_timeout")
	if err != nil {
		return LoaderAgentRequest{}, fmt.Errorf("decode scheduler.agent timeout: %w", err)
	}
	request.Title = loaderStringOption(options, "title")
	request.Driver = loaderStringOption(options, "driver")
	request.GuestImage = loaderStringOption(options, "guestImage", "guest_image")
	request.WorkspaceID = loaderStringOption(options, "workspaceId", "workspace_id")
	request.SessionEnv, err = loaderSessionEnvOption(options)
	if err != nil {
		return LoaderAgentRequest{}, err
	}
	return request, nil
}

func loaderAgentOptionsWithoutSchema(value *qjs.Value) (map[string]any, error) {
	if value == nil || value.IsUndefined() || value.IsNull() {
		return map[string]any{}, nil
	}
	if !value.IsObject() || value.IsArray() {
		return qjs.ToGoValue[map[string]any](value)
	}
	rawJSON, err := value.JSONStringify()
	if err != nil {
		return nil, err
	}
	var options map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &options); err != nil {
		return nil, err
	}
	delete(options, "outputSchema")
	delete(options, "schema")
	return options, nil
}

func parseLoaderOutputSchema(jsctx *qjs.Context, args []*qjs.Value, apiName string) (string, *qjs.Value, error) {
	if len(args) < 2 || args[1] == nil || args[1].IsUndefined() || args[1].IsNull() {
		return "", nil, nil
	}
	if !args[1].IsObject() || args[1].IsArray() {
		return "", nil, nil
	}
	options := args[1]
	for _, key := range []string{"outputSchema", "schema"} {
		schemaValue := options.GetPropertyStr(key)
		if schemaValue == nil || schemaValue.IsUndefined() || schemaValue.IsNull() {
			continue
		}
		schemaJSON, err := loaderOutputSchemaJSON(jsctx, schemaValue)
		if err != nil {
			return "", nil, fmt.Errorf("decode %s %s: %w", apiName, key, err)
		}
		return schemaJSON, schemaValue, nil
	}
	return "", nil, nil
}

func loaderOutputSchemaJSON(jsctx *qjs.Context, value *qjs.Value) (string, error) {
	if !value.IsObject() || value.IsArray() {
		return "", fmt.Errorf("must be an object")
	}
	toJSONSchema := value.GetPropertyStr("toJSONSchema")
	if toJSONSchema != nil && toJSONSchema.IsFunction() {
		converted, err := jsctx.Invoke(toJSONSchema, value)
		if err != nil {
			return "", err
		}
		if converted == nil || converted.IsUndefined() || converted.IsNull() || !converted.IsObject() || converted.IsArray() {
			return "", fmt.Errorf("toJSONSchema must return an object")
		}
		return jsValueToJSON(converted)
	}
	return jsValueToJSON(value)
}

func validateLoaderJSONWithSchema(jsctx *qjs.Context, schemaValue, responseValue *qjs.Value, apiName string) error {
	if schemaValue == nil || responseValue == nil || !schemaValue.IsObject() {
		return nil
	}
	parseFn := schemaValue.GetPropertyStr("parse")
	if parseFn == nil || !parseFn.IsFunction() {
		return nil
	}
	jsonValue := responseValue.GetPropertyStr("json")
	if jsonValue == nil || jsonValue.IsUndefined() || jsonValue.IsNull() {
		return nil
	}
	if _, err := jsctx.Invoke(parseFn, schemaValue, jsonValue); err != nil {
		return fmt.Errorf("%s JSON output does not match outputSchema: %w", apiName, err)
	}
	return nil
}

func parseLoaderExecRequest(args []*qjs.Value) (LoaderCommandRequest, error) {
	if len(args) != 1 || args[0] == nil || args[0].IsUndefined() || args[0].IsNull() || !args[0].IsObject() || args[0].IsArray() {
		return LoaderCommandRequest{}, fmt.Errorf("scheduler.exec requires a request object")
	}
	options, err := qjs.ToGoValue[map[string]any](args[0])
	if err != nil {
		return LoaderCommandRequest{}, fmt.Errorf("decode scheduler.exec request: %w", err)
	}
	request, err := loaderCommandRequestFromOptions(options, "scheduler.exec")
	if err != nil {
		return LoaderCommandRequest{}, err
	}
	request.Mode = "exec"
	request.Command = loaderStringOption(options, "command")
	if strings.TrimSpace(request.Command) == "" {
		return LoaderCommandRequest{}, fmt.Errorf("scheduler.exec requires a non-empty command")
	}
	request.Args, err = loaderStringArrayOption(options, "args")
	if err != nil {
		return LoaderCommandRequest{}, fmt.Errorf("decode scheduler.exec args: %w", err)
	}
	return request, nil
}

func parseLoaderShellRequest(args []*qjs.Value) (LoaderCommandRequest, error) {
	if len(args) == 0 || args[0] == nil || args[0].IsUndefined() || args[0].IsNull() {
		return LoaderCommandRequest{}, fmt.Errorf("scheduler.shell requires a script")
	}
	if len(args) > 2 {
		return LoaderCommandRequest{}, fmt.Errorf("scheduler.shell accepts a script and optional options object")
	}
	script := args[0].String()
	if strings.TrimSpace(script) == "" {
		return LoaderCommandRequest{}, fmt.Errorf("scheduler.shell requires a non-empty script")
	}
	options := map[string]any{}
	if len(args) > 1 && args[1] != nil && !args[1].IsUndefined() && !args[1].IsNull() {
		if !args[1].IsObject() || args[1].IsArray() {
			return LoaderCommandRequest{}, fmt.Errorf("scheduler.shell options must be an object")
		}
		decoded, err := qjs.ToGoValue[map[string]any](args[1])
		if err != nil {
			return LoaderCommandRequest{}, fmt.Errorf("decode scheduler.shell options: %w", err)
		}
		options = decoded
	}
	request, err := loaderCommandRequestFromOptions(options, "scheduler.shell")
	if err != nil {
		return LoaderCommandRequest{}, err
	}
	request.Mode = "shell"
	request.Script = script
	return request, nil
}

func loaderCommandRequestFromOptions(options map[string]any, apiName string) (LoaderCommandRequest, error) {
	var err error
	request := LoaderCommandRequest{
		Cwd:           loaderStringOption(options, "cwd"),
		SessionPolicy: loader.NormalizeSessionPolicy(loaderStringOption(options, "sessionPolicy", "session_policy")),
		Title:         loaderStringOption(options, "title"),
		Driver:        loaderStringOption(options, "driver"),
		GuestImage:    loaderStringOption(options, "guestImage", "guest_image"),
		WorkspaceID:   loaderStringOption(options, "workspaceId", "workspace_id"),
	}
	request.Env, err = loaderStringMapOption(options, "env")
	if err != nil {
		return LoaderCommandRequest{}, fmt.Errorf("decode %s env: %w", apiName, err)
	}
	request.TimeoutMs, err = loaderInt64Option(options, "timeoutMs", "timeout_ms")
	if err != nil {
		return LoaderCommandRequest{}, fmt.Errorf("decode %s timeoutMs: %w", apiName, err)
	}
	request.MaxOutputBytes, err = loaderInt64Option(options, "maxOutputBytes", "max_output_bytes")
	if err != nil {
		return LoaderCommandRequest{}, fmt.Errorf("decode %s maxOutputBytes: %w", apiName, err)
	}
	request.SessionEnv, err = loaderSessionEnvOption(options)
	if err != nil {
		return LoaderCommandRequest{}, fmt.Errorf("%s", strings.Replace(err.Error(), "scheduler.agent", apiName, 1))
	}
	return request, nil
}

func loaderDurationOption(options map[string]any, keys ...string) (time.Duration, error) {
	for _, key := range keys {
		value, ok := options[key]
		if !ok || value == nil {
			continue
		}
		switch raw := value.(type) {
		case string:
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				return 0, nil
			}
			parsed, err := time.ParseDuration(trimmed)
			if err != nil {
				return 0, err
			}
			if parsed <= 0 {
				return 0, fmt.Errorf("duration must be positive")
			}
			return parsed, nil
		default:
			return 0, fmt.Errorf("duration must be a string")
		}
	}
	return 0, nil
}

func loaderStringOption(options map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := options[key]
		if !ok || value == nil {
			continue
		}
		if raw, ok := value.(string); ok {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

func loaderStringArrayOption(options map[string]any, keys ...string) ([]string, error) {
	for _, key := range keys {
		value, ok := options[key]
		if !ok || value == nil {
			continue
		}
		rawItems, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("must be an array")
		}
		items := make([]string, 0, len(rawItems))
		for index, rawItem := range rawItems {
			if rawItem == nil {
				return nil, fmt.Errorf("item %d must be a string", index)
			}
			item, ok := rawItem.(string)
			if !ok {
				return nil, fmt.Errorf("item %d must be a string", index)
			}
			items = append(items, item)
		}
		return items, nil
	}
	return nil, nil
}

func loaderStringMapOption(options map[string]any, keys ...string) (map[string]string, error) {
	for _, key := range keys {
		value, ok := options[key]
		if !ok || value == nil {
			continue
		}
		rawItems, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("must be an object")
		}
		items := make(map[string]string, len(rawItems))
		for rawName, rawValue := range rawItems {
			name := strings.TrimSpace(rawName)
			if name == "" {
				continue
			}
			if rawValue == nil {
				items[name] = ""
				continue
			}
			value, ok := rawValue.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", name)
			}
			items[name] = value
		}
		return items, nil
	}
	return nil, nil
}

func loaderInt64Option(options map[string]any, keys ...string) (int64, error) {
	for _, key := range keys {
		value, ok := options[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int64(typed), nil
		case int64:
			return typed, nil
		case int:
			return int64(typed), nil
		default:
			return 0, fmt.Errorf("must be a number")
		}
	}
	return 0, nil
}

func loaderSessionEnvOption(options map[string]any) ([]SessionEnvVar, error) {
	for _, key := range []string{"sessionEnv", "session_env"} {
		value, ok := options[key]
		if !ok {
			continue
		}
		items, err := loaderSessionEnvItems(value)
		if err != nil {
			return nil, fmt.Errorf("decode scheduler.agent %s: %w", key, err)
		}
		return items, nil
	}
	return nil, nil
}

func loaderSessionEnvItems(value any) ([]SessionEnvVar, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			if strings.TrimSpace(key) == "" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items := make([]SessionEnvVar, 0, len(keys))
		for _, key := range keys {
			name := strings.TrimSpace(key)
			if name == "" {
				continue
			}
			envValue, secret, err := loaderSessionEnvValue(name, typed[key])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			items = append(items, SessionEnvVar{Name: name, Value: envValue, Secret: secret})
		}
		return normalizeEnvItems(items), nil
	case []any:
		items := make([]SessionEnvVar, 0, len(typed))
		for index, rawItem := range typed {
			entry, ok := rawItem.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("item %d must be an object", index)
			}
			name := loaderStringOption(entry, "name")
			if name == "" {
				return nil, fmt.Errorf("item %d requires a non-empty name", index)
			}
			envValue, secret, err := loaderSessionEnvValue(name, entry["value"])
			if err != nil {
				return nil, fmt.Errorf("item %d: %w", index, err)
			}
			if rawSecret, ok := entry["secret"]; ok && rawSecret != nil {
				switch typedSecret := rawSecret.(type) {
				case bool:
					secret = typedSecret
				case string:
					secret = strings.EqualFold(strings.TrimSpace(typedSecret), "true")
				case float64:
					secret = typedSecret != 0
				default:
					return nil, fmt.Errorf("item %d secret must be a boolean", index)
				}
			}
			items = append(items, SessionEnvVar{Name: name, Value: envValue, Secret: secret})
		}
		return normalizeEnvItems(items), nil
	default:
		return nil, fmt.Errorf("must be an object map or array")
	}
}

func loaderSessionEnvValue(name string, value any) (string, bool, error) {
	secret := loaderSecretEnvName(name)
	switch typed := value.(type) {
	case nil:
		return "", secret, nil
	case string:
		return typed, secret, nil
	case bool:
		if typed {
			return "true", secret, nil
		}
		return "false", secret, nil
	case float64:
		return strings.TrimSpace(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", typed), "0"), ".")), secret, nil
	case map[string]any:
		envValue, nestedSecret, err := loaderSessionEnvValue(name, typed["value"])
		if err != nil {
			return "", false, err
		}
		secret = nestedSecret
		if rawSecret, ok := typed["secret"]; ok && rawSecret != nil {
			switch typedSecret := rawSecret.(type) {
			case bool:
				secret = typedSecret
			case string:
				secret = strings.EqualFold(strings.TrimSpace(typedSecret), "true")
			case float64:
				secret = typedSecret != 0
			default:
				return "", false, fmt.Errorf("secret must be a boolean")
			}
		}
		return envValue, secret, nil
	default:
		return fmt.Sprint(typed), secret, nil
	}
}

func loaderSecretEnvName(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if strings.Contains(name, "PASSWORD") || strings.HasSuffix(name, "_TOKEN") || strings.HasSuffix(name, "_SECRET") || strings.HasSuffix(name, "_KEY") {
		return true
	}
	switch name {
	case "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY", "LLM_API_KEY":
		return true
	default:
		return false
	}
}

func parseLoaderLLMRequest(args []*qjs.Value) (LoaderLLMRequest, error) {
	request := LoaderLLMRequest{}
	if len(args) < 2 || args[1] == nil || args[1].IsUndefined() || args[1].IsNull() {
		return request, nil
	}
	options, err := loaderAgentOptionsWithoutSchema(args[1])
	if err != nil {
		return LoaderLLMRequest{}, fmt.Errorf("decode scheduler.llm options: %w", err)
	}
	if value, ok := options["model"].(string); ok {
		request.Model = strings.TrimSpace(value)
	}
	return request, nil
}

func loaderRPCRequestJSON(args []*qjs.Value, apiName string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) > 1 {
		return "", fmt.Errorf("%s accepts at most one request object", apiName)
	}
	value := args[0]
	if value == nil || value.IsNull() || value.IsUndefined() {
		return "", nil
	}
	requestJSON, err := jsValueToJSON(value)
	if err != nil {
		return "", fmt.Errorf("encode %s request: %w", apiName, err)
	}
	return strings.TrimSpace(requestJSON), nil
}

func lowerFirstASCII(value string) string {
	if value == "" {
		return ""
	}
	if len(value) == 1 {
		return strings.ToLower(value)
	}
	return strings.ToLower(value[:1]) + value[1:]
}
