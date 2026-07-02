package compose

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	agentcomposev2 "agent-compose/proto/agentcompose/v2"
	"agent-compose/proto/agentcompose/v2/agentcomposev2connect"
)

func runComposeRunCommand(cmd *cobra.Command, cli Options, options composeRunOptions, args []string) error {
	_, normalized, projectID, err := resolveComposeProject(cli)
	if err != nil {
		return err
	}
	agentName := strings.TrimSpace(args[0])
	prompt := strings.TrimSpace(options.Prompt)
	if prompt == "" && len(args) > 1 {
		prompt = strings.Join(args[1:], " ")
	}
	clientConfig, err := ResolveClientConfig(cli.Host)
	if err != nil {
		return err
	}
	cleanupPolicy := agentcomposev2.RunSessionCleanupPolicy_RUN_SESSION_CLEANUP_POLICY_STOP_ON_COMPLETION
	if options.KeepRunning {
		cleanupPolicy = agentcomposev2.RunSessionCleanupPolicy_RUN_SESSION_CLEANUP_POLICY_KEEP_RUNNING
	}
	client := agentcomposev2connect.NewRunServiceClient(NewDaemonHTTPClient(clientConfig), clientConfig.BaseURL)
	stream, err := client.RunAgentStream(cmd.Context(), connect.NewRequest(&agentcomposev2.RunAgentRequest{
		ProjectId:       projectID,
		AgentName:       agentName,
		Prompt:          prompt,
		Source:          agentcomposev2.RunSource_RUN_SOURCE_MANUAL,
		SessionId:       strings.TrimSpace(options.SessionID),
		CleanupPolicy:   cleanupPolicy,
		ClientRequestId: manualRunClientRequestID(normalized.Name, agentName, prompt),
	}))
	if err != nil {
		return commandExitErrorForConnect(fmt.Errorf("run project %s agent %s: %w", normalized.Name, agentName, err))
	}
	var completed *agentcomposev2.RunSummary
	for stream.Receive() {
		event := stream.Msg()
		switch event.GetEventType() {
		case agentcomposev2.RunAgentStreamEventType_RUN_AGENT_STREAM_EVENT_TYPE_OUTPUT:
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
		case agentcomposev2.RunAgentStreamEventType_RUN_AGENT_STREAM_EVENT_TYPE_COMPLETED:
			completed = event.GetRun()
		}
	}
	if err := stream.Err(); err != nil {
		return commandExitErrorForConnect(fmt.Errorf("run project %s agent %s: %w", normalized.Name, agentName, err))
	}
	if completed == nil {
		return fmt.Errorf("run project %s agent %s: stream completed without terminal run", normalized.Name, agentName)
	}
	detail, err := getRunDetail(cmd.Context(), client, projectID, completed.GetRunId())
	if err != nil {
		return commandExitErrorForConnect(fmt.Errorf("get run %s for project %s: %w", completed.GetRunId(), normalized.Name, err))
	}
	if cli.JSON {
		data, err := json.MarshalIndent(composeRunOutputFromDetail(detail.Msg.GetRun()), "", "  ")
		if err != nil {
			return err
		}
		if err := writeCommandOutput(cmd.OutOrStdout(), append(data, '\n')); err != nil {
			return err
		}
	}
	if runSummaryFailed(completed) {
		return commandExitError{Code: runSummaryExitCode(completed), Err: fmt.Errorf("run %s for project %s agent %s failed: %s", completed.GetRunId(), normalized.Name, agentName, firstNonEmptyString(completed.GetError(), runStatusText(completed.GetStatus())))}
	}
	return nil
}

func manualRunClientRequestID(projectName, agentName, prompt string) string {
	value := strings.TrimSpace(projectName) + "|" + strings.TrimSpace(agentName) + "|" + strings.TrimSpace(prompt) + "|" + time.Now().UTC().Format(time.RFC3339Nano)
	return value
}
