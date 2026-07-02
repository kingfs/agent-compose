package qjs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fastschema/qjs"
)

func marshalJSONCompact(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode json payload: %w", err)
	}
	return string(data), nil
}

func loaderJSONResult(text, outputSchemaJSON, sourceName string) (any, error) {
	if strings.TrimSpace(outputSchemaJSON) == "" {
		return nil, nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON for outputSchema: %w", sourceName, err)
	}
	return parsed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func loaderCommandResultValue(jsctx *qjs.Context, apiName string, response LoaderCommandResult) (*qjs.Value, error) {
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode %s response: %w", apiName, err)
	}
	value, err := payloadValueFromJSON(jsctx, string(data))
	if err != nil {
		return nil, fmt.Errorf("decode %s response: %w", apiName, err)
	}
	return value, nil
}

func payloadValueFromJSON(jsctx *qjs.Context, payloadJSON string) (*qjs.Value, error) {
	payloadJSON = strings.TrimSpace(payloadJSON)
	if payloadJSON == "" {
		return jsctx.NewUndefined(), nil
	}
	return jsctx.ParseJSON(payloadJSON), nil
}

func jsValueToJSON(value *qjs.Value) (string, error) {
	if value == nil || value.IsUndefined() {
		return "", nil
	}
	if value.IsNull() {
		return "null", nil
	}
	if value.IsBool() {
		if value.Bool() {
			return "true", nil
		}
		return "false", nil
	}
	if value.IsString() || value.IsBigInt() {
		data, err := json.Marshal(value.String())
		if err != nil {
			return "", fmt.Errorf("encode js string value: %w", err)
		}
		return string(data), nil
	}
	if value.IsNumber() {
		raw := strings.TrimSpace(value.String())
		if raw == "" {
			return "", nil
		}
		if raw == "NaN" || raw == "Infinity" || raw == "-Infinity" {
			data, err := json.Marshal(raw)
			if err != nil {
				return "", fmt.Errorf("encode js numeric sentinel: %w", err)
			}
			return string(data), nil
		}
		if json.Valid([]byte(raw)) {
			return raw, nil
		}
	}
	jsonValue, err := value.JSONStringify()
	if err == nil && strings.TrimSpace(jsonValue) != "" && json.Valid([]byte(jsonValue)) {
		return jsonValue, nil
	}
	return "", nil
}

func loaderResultJSON(value *qjs.Value) (string, bool, error) {
	if value == nil || value.IsUndefined() {
		return "", false, nil
	}
	jsonValue, err := jsValueToJSON(value)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(jsonValue) == "" {
		return "", false, nil
	}
	return jsonValue, true, nil
}
