package loader

import "strings"

func CommandEventPayload(request LoaderCommandRequest, result LoaderCommandResult) map[string]any {
	payload := map[string]any{
		"mode":            strings.TrimSpace(request.Mode),
		"command":         strings.TrimSpace(request.Command),
		"args":            append([]string(nil), request.Args...),
		"cwd":             strings.TrimSpace(request.Cwd),
		"exitCode":        result.ExitCode,
		"success":         result.Success,
		"stdoutTruncated": result.StdoutTruncated,
		"stderrTruncated": result.StderrTruncated,
		"sessionId":       result.SessionID,
		"cellId":          result.CellID,
	}
	if payload["mode"] == "shell" {
		payload["command"] = ""
	}
	return payload
}
