package compose

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

func runComposeExecCommand(cmd *cobra.Command, cli Options, options composeExecOptions, args []string) error {
	_, normalized, projectID, err := resolveComposeProject(cli)
	if err != nil {
		return err
	}
	clients, err := newCLIServiceClients(cli)
	if err != nil {
		return err
	}
	req := &agentcomposev2.ExecRequest{
		Command: &agentcomposev2.ExecCommand{Command: args[0], Args: append([]string(nil), args[1:]...)},
		Cwd:     strings.TrimSpace(options.Cwd),
	}
	switch {
	case strings.TrimSpace(options.SessionID) != "":
		req.Target = &agentcomposev2.ExecRequest_SessionId{SessionId: strings.TrimSpace(options.SessionID)}
	case strings.TrimSpace(options.RunID) != "":
		req.Target = &agentcomposev2.ExecRequest_RunId{RunId: strings.TrimSpace(options.RunID)}
	default:
		req.Target = &agentcomposev2.ExecRequest_Selector{Selector: &agentcomposev2.ExecSessionSelector{
			ProjectId:   projectID,
			ProjectName: normalized.Name,
			AgentName:   strings.TrimSpace(options.AgentName),
		}}
	}
	stream, err := clients.exec.ExecStream(cmd.Context(), connect.NewRequest(req))
	if err != nil {
		return commandExitErrorForConnect(fmt.Errorf("exec project %s: %w", normalized.Name, err))
	}
	var result *agentcomposev2.ExecResult
	for stream.Receive() {
		event := stream.Msg()
		switch event.GetEventType() {
		case agentcomposev2.ExecStreamEventType_EXEC_STREAM_EVENT_TYPE_OUTPUT:
			if cli.JSON {
				continue
			}
			target := cmd.OutOrStdout()
			if event.GetIsStderr() {
				target = cmd.ErrOrStderr()
			}
			if _, err := io.WriteString(target, event.GetChunk()); err != nil {
				return err
			}
		case agentcomposev2.ExecStreamEventType_EXEC_STREAM_EVENT_TYPE_COMPLETED:
			result = event.GetResult()
		}
	}
	if err := stream.Err(); err != nil {
		return commandExitErrorForConnect(fmt.Errorf("exec project %s: %w", normalized.Name, err))
	}
	if result == nil {
		return fmt.Errorf("exec project %s: stream completed without result", normalized.Name)
	}
	if cli.JSON {
		data, err := json.MarshalIndent(composeExecOutputFromResult(result), "", "  ")
		if err != nil {
			return err
		}
		if err := writeCommandOutput(cmd.OutOrStdout(), append(data, '\n')); err != nil {
			return err
		}
	}
	if !result.GetSuccess() {
		return commandExitError{Code: execResultExitCode(result), Err: fmt.Errorf("exec %s in session %s failed: %s", result.GetExecId(), result.GetSessionId(), firstNonEmptyString(result.GetError(), result.GetStderr(), result.GetOutput(), "command failed"))}
	}
	return nil
}

func execResultExitCode(result *agentcomposev2.ExecResult) int {
	if code := int(result.GetExitCode()); code > 0 && code < 126 {
		return code
	}
	return exitCodeGeneral
}
