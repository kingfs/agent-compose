package main

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"agent-compose/internal/cli"
	composecli "agent-compose/internal/cli/compose"
	"agent-compose/internal/daemon"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type daemonRunner = cli.DaemonRunner
type DaemonOptions = daemon.Options
type DaemonApp = daemon.App
type cliClientConfig = composecli.ClientConfig
type composeUpOutput = composecli.ComposeUpOutput
type composeUpChangeOutput = composecli.ComposeUpChangeOutput
type composeRunOutput = composecli.ComposeRunOutput
type composeLogsOutput = composecli.ComposeLogsOutput
type composePSOutput = composecli.ComposePSOutput
type composeProjectOutput = composecli.ComposeProjectOutput
type composeAgentInspectOutput = composecli.ComposeAgentInspectOutput
type composeSessionOutput = composecli.ComposeSessionOutput
type composeExecOutput = composecli.ComposeExecOutput
type composeImageListOutput = composecli.ComposeImageListOutput
type composeImageInspectOutput = composecli.ComposeImageInspectOutput
type composeImagePullOutput = composecli.ComposeImagePullOutput
type composeImageRemoveOutput = composecli.ComposeImageRemoveOutput
type composeImageOutput = composecli.ComposeImageOutput

const (
	exitCodeGeneral     = composecli.ExitCodeGeneral
	exitCodeUsage       = composecli.ExitCodeUsage
	exitCodeUnavailable = composecli.ExitCodeUnavailable
)

func NewDaemonApp(ctx context.Context, opts DaemonOptions) (*DaemonApp, error) {
	return daemon.NewApp(ctx, opts)
}

func executeCLI(ctx context.Context, out, errOut io.Writer, args []string, runDaemon daemonRunner) int {
	return cli.Execute(ctx, out, errOut, args, runDaemon)
}

func newRootCommand(out, errOut io.Writer, runDaemon daemonRunner) *cobra.Command {
	return cli.NewRootCommand(out, errOut, runDaemon)
}

func resolveCLIClientConfig(hostFlag string) (cliClientConfig, error) {
	return composecli.ResolveClientConfig(hostFlag)
}

func composeImageOutputFromProto(image *agentcomposev2.Image) composeImageOutput {
	return composecli.ComposeImageOutputFromProto(image)
}
