package compose

type ClientConfig struct {
	BaseURL       string
	SocketPath    string
	Source        string
	SourceValue   string
	UseUnixSocket bool
}

type Options struct {
	Host        string
	ComposeFile string
	ProjectName string
	JSON        bool
}

type composeConfigOptions struct {
	Quiet bool
}

type composeRunOptions struct {
	Prompt      string
	SessionID   string
	KeepRunning bool
}

type composeLogsOptions struct {
	AgentName string
	RunID     string
	SessionID string
	Follow    bool
}

type composeExecOptions struct {
	AgentName string
	RunID     string
	SessionID string
	Cwd       string
}

type composeImageListOptions struct {
	Query string
	All   bool
}

type composeImagePullOptions struct {
	Platform string
}

type composeImageRemoveOptions struct {
	Force         bool
	PruneChildren bool
}
