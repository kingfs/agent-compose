package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"agent-compose/internal/cli/compose"
	"agent-compose/pkg/config"
)

type DaemonRunner func(context.Context) error

func Execute(ctx context.Context, out, errOut io.Writer, args []string, runDaemon DaemonRunner) int {
	cmd := NewRootCommand(out, errOut, runDaemon)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return compose.CommandExitCode(err)
	}
	return 0
}

func NewRootCommand(out, errOut io.Writer, runDaemon DaemonRunner) *cobra.Command {
	options := compose.Options{}
	root := &cobra.Command{
		Use:           "agent-compose",
		Short:         "agent-compose daemon and CLI",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(out)
	root.SetErr(errOut)
	root.CompletionOptions.DisableDefaultCmd = true

	root.PersistentFlags().StringVar(&options.Host, "host", "", "Daemon HTTP endpoint")
	root.PersistentFlags().StringVarP(&options.ComposeFile, "file", "f", "", "Path to agent-compose.yml")
	root.PersistentFlags().StringVar(&options.ProjectName, "project-name", "", "Override compose project name")
	root.PersistentFlags().BoolVar(&options.JSON, "json", false, "Print machine-readable JSON")

	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the agent-compose daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(cmd.Context())
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print build version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), config.BuildVersion)
			return err
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Query daemon status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientConfig, err := compose.ResolveClientConfig(options.Host)
			if err != nil {
				return err
			}
			body, err := compose.FetchDaemonVersion(cmd.Context(), clientConfig)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return err
		},
	}

	root.AddCommand(daemonCmd, versionCmd, statusCmd)
	compose.AddCommands(root, &options)
	return root
}
