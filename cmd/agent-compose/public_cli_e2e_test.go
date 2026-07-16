package main

import "testing"

func TestE2ECLIStreamingAndFailureWorkflows(t *testing.T) {
	t.Run("command branches", TestCLICommandBranchSweepWorkflows)
	t.Run("run output branches", TestCLIRunStreamAndDetailEdgeBranches)
	t.Run("run completion failures", TestCLIRunCompletionErrorBranches)
	t.Run("run command edges", TestCLIRunCommandAdditionalEdgeWorkflows)
	t.Run("exec interaction", TestCLIExecInteractiveUsesExecAttachClient)
	t.Run("exec prompt interaction", TestCLIExecPromptAttachUsesExecAttachClient)
	t.Run("daemon HTTP attach", TestDaemonHTTPClientRunAttachBidiUsesH2C)
	t.Run("daemon TCP attach", TestDaemonTCPServerRunAttachBidiUsesH2C)
}
