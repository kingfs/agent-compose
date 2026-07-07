# run/exec 交互式命令与 TTY 设计

## 背景与目标

`agent-compose run` 和 `agent-compose exec` 当前已经能把命令输出实时展示到 CLI，也能把输出沉淀到 run log、cell artifact、exec artifact 中。但它们还不能实现真正的进程级交互：本地终端输入无法持续写入 runtime 内进程的 stdin，也没有 TTY 分配、raw mode、窗口尺寸变化转发。

本设计聚焦 `--command` 场景：

- `agent-compose exec -it <sandbox> bash`
- `agent-compose run -it <agent> --command bash`
- `agent-compose run -it <agent> --command <interactive command>`

`--prompt` 不纳入 `-i/-t` 交互式命令设计。`--prompt` 是 agent 任务输入语义，不是 shell/进程 stdin 语义，应与 `-i/-t` 互斥。

目标行为尽量贴近 Docker：

- 不启用 `-i` 时，命令 stdin 应关闭或不可读，等待 stdin 的程序不应因为 daemon/CLI 保持了一个未关闭输入流而长期挂起。
- 启用 `-i` 时，本地 stdin 持续连接到 runtime 内命令进程。
- 启用 `-t` 时，为 runtime 内命令分配 pseudo-TTY，并要求同时启用 `-i`。
- TTY 模式下 stdout/stderr 由 PTY 合流，CLI 按终端字节流直接写出。
- `--json` 与 `-i/-t` 互斥。

## 当前输入/输出模式

### 1. run --command / exec / loader command

当前 command 执行路径不是直接把 CLI 请求变成目标命令，而是通过 guest runtime wrapper 间接执行。

daemon 侧会构造 `RuntimeCommandRequest`，写入 guest 可见的 `command-request.json`。其中包含：

- `mode`
- `command`
- `args`
- `script`
- `cwd`
- `env`
- `timeoutMs`
- `maxOutputBytes`
- `artifactDir`

随后 daemon 通过 runtime driver 执行：

```text
agent-compose-runtime exec --request-file <guest path>/command-request.json ...
```

guest 内 `agent-compose-runtime exec` 再读取 request file，启动真正的子进程。子进程输出会被 runtime SDK 捕获：

- 写入 child stdout/stderr 的 capture buffer。
- 转发到 wrapper 进程 stdout/stderr。
- 写入 artifact 文件，例如 `stdout.txt`、`stderr.txt`、`output.txt`、`command-result.json`。

daemon driver 只看到 wrapper 进程的 stdout/stderr，然后通过 `ExecStreamWriter` 传回上层。

相关路径：

- `pkg/agentcompose/api/exec.go`: `executeProjectCommand`
- `pkg/runs/controller.go`: `executeProjectRunCommand`
- `pkg/agentcompose/adapters/loader_command_executor.go`: `ExecuteLoaderCommand`
- `pkg/execution/command_runtime.go`: `RuntimeCommandRequest`
- `runtime/javascript/src/command.ts`: `runProcess`
- `runtime/agent-compose-runtime-sdk/src/exec.ts`: `runProcess`

这个机制支持一次性结构化输入和实时输出，不支持持续 stdin。

### 2. kernel/cell

cell 执行路径类似 command，但输入不是 request JSON，而是脚本文件。

daemon 将 cell source 写入 session state 下的脚本文件，例如 shell/javascript/python cell。然后根据 cell type 构造 `ExecSpec`，通过 runtime driver 执行解释器。

输出路径：

- driver stream 实时返回 stdout/stderr chunk。
- session stream 发布 cell output 事件。
- cell artifact 记录 stdout/stderr/output。
- cell completed 后写入 store。

相关路径：

- `pkg/agentcompose/adapters/cell_executor.go`: `executeCell`
- `pkg/agentcompose/api/kernel.go`: `ExecuteCellStream`
- `pkg/sessions/stream.go`: `PublishCellOutput`

这个机制支持“一次脚本输入 + 输出流”，不支持运行中继续向 cell 进程写 stdin。

### 3. agent prompt

agent prompt 不是命令 stdin。当前实现会把 prompt、output schema、system prompt 写入文件，再启动 guest 内的 agent runner。

daemon 侧：

- 写 prompt file。
- 写 schema file。
- 写 system prompt file。
- 构造 `agent-compose-runtime prompt --message-file ...` 命令。

runtime 侧：

- Codex/Claude 通过 SDK 发起一次 prompt，并消费事件流。
- Gemini/OpenCode 通过 CLI 参数传入 prompt，且 `stdio` 是 `["ignore", "pipe", "pipe"]`，stdin 明确被忽略。

相关路径：

- `pkg/agentcompose/adapters/agent_runner.go`: `ExecuteAgentRun`, `BuildAgentExecSpec`
- `pkg/execution/agent_files.go`: `WriteAgentPromptFile`, `WriteAgentOutputSchemaFile`
- `runtime/javascript/src/prompt.ts`: `runPromptCommand`
- `runtime/javascript/src/runners/*`

因此，`--prompt` 与 `-i/-t` 不应混用。若未来需要 agent CLI 的交互式会话，应单独设计 agent session，而不是复用 prompt run 的语义。

### 4. loader script API

loader script 能调用：

- `scheduler.exec`
- `scheduler.shell`
- `scheduler.agent`
- `scheduler.llm`
- session RPC
- topic publish

这些都是结构化调用。

`scheduler.exec/shell` 最终转换为 `LoaderCommandRequest`，进入 loader command 路径。`scheduler.agent` 转为一次 agent prompt。`scheduler.llm` 是 daemon LLM client 的一次生成请求。session RPC 是 JSON request/response。

相关路径：

- `pkg/loaders/engine.go`
- `pkg/loaders/run_host.go`: `RuntimeHost.Command`
- `pkg/agentcompose/adapters/loader_host.go`

loader script 当前不存在“本地 stdin 持续接入 runtime 进程”的机制。

### 5. run log / exec stream / cell stream / session watch

当前已有多种输出同步机制：

- `ExecService.ExecStream`: server-stream，实时发送 exec stdout/stderr chunk。
- `RunService.RunAgentStream`: server-stream，实时发送 run output chunk。
- `RunService.FollowRunLogs`: 轮询 run log 文件 offset，类似 tail。
- `KernelService.ExecuteCellStream`: server-stream，实时发送 cell output。
- `SessionService.WatchSession`: 订阅 session stream 事件。

这些都是 daemon 到 client 的单向输出机制。

相关路径：

- `pkg/agentcompose/api/exec.go`: `ExecStream`
- `pkg/agentcompose/app/run_controller.go`: `RunAgentStream`
- `pkg/agentcompose/api/run_handler.go`: `FollowRunLogs`
- `pkg/agentcompose/api/kernel.go`: `ExecuteCellStream`
- `pkg/agentcompose/api/session.go`: `WatchSession`

### 6. runtime LLM facade

runtime LLM facade 是 runtime 内程序主动发 HTTP 请求到 daemon，daemon 校验 token 后转发到上游 LLM。响应可以是普通 JSON，也可以是 SSE 流。

这是 HTTP request/response 代理，不是通用进程交互通道。它能支持 runtime 内 agent 访问 LLM，但不能用来承载本地 terminal stdin、TTY resize 或任意命令 stdout/stderr 合流。

相关路径：

- `pkg/agentcompose/proxy/runtime_llm.go`

### 7. workspace upload/download

workspace API 能上传文件、上传归档、下载文件。这是文件同步能力，不是进程 stdin/stdout 交互能力。

相关路径：

- `pkg/agentcompose/proxy/workspace.go`

## 当前机制的边界

现有机制能覆盖：

- 一次性结构化输入：prompt、script、command request、env、cwd。
- 文件输入：cell script、agent prompt file、command request file、workspace upload。
- 输出实时返回：stdout/stderr chunk、run output、cell output、LLM SSE。
- 输出落盘：stdout/stderr/output/transcript/result artifact。
- 输出回放：run log tail、session/cell watch。

现有机制不能覆盖：

- 本地 stdin 在命令运行期间持续写入 runtime 进程。
- TTY 分配。
- raw mode 终端字节流。
- terminal resize。
- TTY 下 stdout/stderr 合流。
- 对 `bash`、`python` REPL、`vim`、`top`、`less` 等交互式程序的正确支持。

关键原因是当前协议和 runtime contract 都是“请求一次发完，输出持续返回”。

以 exec 为例：

```proto
rpc ExecStream(ExecRequest) returns (stream ExecStreamResponse);
```

`ExecRequest` 在 RPC 开始时发送一次。RPC 建立后，client 无法继续向 server 发送 stdin 数据或 resize 事件。`RunAgentStream`、`ExecuteCellStream` 也是类似形态。

runtime driver contract 也没有 stdin/TTY 字段：

```go
type ExecSpec struct {
	Command string
	Args    []string
	Env     map[string]string
	Cwd     string
}
```

Docker driver 当前只设置 `AttachStdout`、`AttachStderr`，没有 `AttachStdin`、`OpenStdin`、`Tty`，也没有 exec resize。

## 为什么需要双向流

真正的 `-it` 不是“输出更实时”，而是“同一个运行中进程的输入输出同时保持连接”。

交互式命令需要在生命周期内持续交换事件：

- CLI -> daemon -> runtime: stdin 字节。
- CLI -> daemon -> runtime: stdin EOF。
- CLI -> daemon -> runtime: terminal resize。
- CLI -> daemon -> runtime: interrupt / cancel。
- runtime -> daemon -> CLI: stdout/stderr 或 PTY byte stream。
- runtime -> daemon -> CLI: exit code / error。

server-stream 只能覆盖 runtime -> daemon -> CLI 方向。用当前协议勉强模拟会出现几个问题：

- 无法在命令启动后继续发送本地键盘输入。
- 无法表达 resize。
- 无法表达 stdin EOF，从而导致等待输入的命令行为不稳定。
- 无法把 `-t` 的 PTY 字节流与现有 transcript chunk 语义清晰区分。
- 如果通过文件轮询模拟 stdin，会引入延迟、同步复杂度、EOF 语义不清、权限与清理问题，也不能满足终端程序对原始字节流和窗口尺寸的要求。

因此需要新增双向流 RPC，或者引入等价的 WebSocket/hijack 通道。考虑当前项目已有 Connect/gRPC 风格 API，优先建议新增 Connect 双向流 RPC。

## 建议的协议设计

建议抽象为 command terminal session，而不是只服务 exec。`exec -it` 和 `run -it --command` 可以复用一套协议，只是 target 不同。

```proto
rpc CommandSession(stream CommandSessionRequest) returns (stream CommandSessionResponse);

message CommandSessionRequest {
  oneof event {
    CommandSessionStart start = 1;
    bytes stdin = 2;
    TerminalResize resize = 3;
    bool stdin_eof = 4;
    bool interrupt = 5;
  }
}

message CommandSessionStart {
  oneof target {
    ExecCommandTarget exec = 1;
    RunCommandTarget run = 2;
  }
  ExecCommand command = 3;
  string cwd = 4;
  repeated EnvVarSpec env = 5;
  bool attach_stdin = 6;
  bool tty = 7;
  uint32 rows = 8;
  uint32 cols = 9;
}

message ExecCommandTarget {
  oneof target {
    string session_id = 1;
    string run_id = 2;
    ExecSessionSelector selector = 3;
  }
}

message RunCommandTarget {
  string project_id = 1;
  string agent_name = 2;
  string session_id = 3;
  string driver = 4;
  RunSessionCleanupPolicy cleanup_policy = 5;
  RunJupyterSpec jupyter = 6;
}

message TerminalResize {
  uint32 rows = 1;
  uint32 cols = 2;
}

message CommandSessionResponse {
  oneof event {
    CommandSessionStarted started = 1;
    bytes stdout = 2;
    bytes stderr = 3;
    CommandSessionExited exited = 4;
    string error = 5;
  }
}
```

协议规则：

- 第一条 client message 必须是 `start`。
- `start.tty=true` 时，`attach_stdin` 必须为 true。
- `tty=true` 时，server 主要发送 `stdout` 字节流，stderr 由 PTY 合流。
- `tty=false` 时，server 可以继续区分 stdout/stderr。
- `stdin_eof` 表示 client 不再写入 stdin。
- `interrupt` 可映射为 SIGINT 或 driver 支持的等价操作。

## Runtime contract 改造

现有 `ExecStream` 仍保留，用于非交互、兼容旧 API。

新增 terminal 级接口，例如：

```go
type TerminalSpec struct {
	Command      string
	Args         []string
	Env          map[string]string
	Cwd          string
	AttachStdin  bool
	TTY          bool
	Rows         uint32
	Cols         uint32
}

type TerminalSize struct {
	Rows uint32
	Cols uint32
}

type TerminalIO struct {
	Stdin  io.Reader
	Resize <-chan TerminalSize
	Stdout func([]byte)
	Stderr func([]byte)
}

type Runtime interface {
	ExecStream(...)
	ExecTerminal(ctx context.Context, session *domain.Session, vmState domain.VMState, spec TerminalSpec, io TerminalIO) (domain.ExecResult, error)
}
```

Docker 映射：

- `AttachStdin: spec.AttachStdin`
- `OpenStdin: spec.AttachStdin`
- `Tty: spec.TTY`
- `AttachStdout: true`
- `AttachStderr: !spec.TTY`
- `Cmd: append([]string{spec.Command}, spec.Args...)`
- `WorkingDir: spec.Cwd`
- `Env: dockerEnvList(spec.Env)`

执行后通过 `ContainerExecAttach` 得到 hijacked connection：

- 非 TTY：使用 Docker multiplexed stream 或 `stdcopy` 拆分 stdout/stderr。
- TTY：直接把 connection 字节作为 stdout 写回 CLI。
- stdin goroutine 将 `TerminalIO.Stdin` copy 到 hijacked connection。
- resize goroutine 调用 Docker exec resize API。
- context cancel 时关闭 attach connection，并等待 exec inspect 得到 exit code。

BoxLite / Microsandbox：

- 如果底层 API 暂不支持 stdin/TTY，应返回明确 unsupported。
- 不能静默降级到普通 `ExecStream`，否则 `-it` 看似成功但实际无法输入。

## 可改造模块

### CLI

涉及 `cmd/agent-compose/main.go`：

- `run` / `exec` 增加 `-i/-t` 的正式语义。
- `--prompt` 与 `-i/-t` 互斥。
- `--json` 与 `-i/-t` 互斥。
- `-t` 要求 `-i`。
- TTY 模式下设置本地 terminal raw mode。
- 捕获 SIGWINCH 并发送 resize。
- stdin copy、stdout/stderr 写出、退出码处理、terminal restore。

### Proto / generated client

涉及：

- `proto/agentcompose/v2/agentcompose.proto`
- generated Go connect client/server
- generated TypeScript client package

建议新增 RPC，不改旧 RPC 的语义。

### API handler

新增 command session handler：

- 校验首包 start。
- 根据 target 分流 exec/run。
- 建立 stdin pipe、resize channel、output send loop。
- 处理 client EOF、context cancel、runtime exit。

### app / runs controller

`run -it --command` 需要复用现有 run/session 生命周期：

- resolve project/agent。
- create/reuse sandbox。
- 记录 run 状态。
- 处理 cleanup policy。
- 记录 output artifact 与 logs path。

但执行命令的部分要走 terminal runtime，而不是现有 command request wrapper。否则 wrapper 自身仍不接 stdin。

### runtime provider adapter

`pkg/agentcompose/adapters/runtime_provider.go` 需要将 domain terminal contract 映射到 driver terminal contract。

### driver

优先支持 Docker：

- `pkg/driver/docker_runtime.go`

后续评估：

- `pkg/driver/boxlite_cgo.go`
- `pkg/driver/microsandbox_runtime.go`

### runtime SDK

如果 `run -it --command` 直接通过 driver exec 目标命令，可以不先改 `agent-compose-runtime exec`。

如果希望保留 command-request wrapper 路径并让 wrapper 负责子进程交互，则 runtime SDK 也要支持：

- 读取 terminal options。
- child process stdio 使用 pipe/inherit/pty。
- 将父进程 stdin 接到 child stdin。
- TTY 时需要 guest 内 pty 支持。

短期建议绕过 wrapper，直接 driver exec 目标命令，降低链路复杂度。

## 破坏性分析

### API 兼容性

新增 RPC 是向后兼容的。现有 `Exec`、`ExecStream`、`RunAgent`、`RunAgentStream` 不改变语义。

风险点：

- 生成代码会变化，影响 Go proto 和 TypeScript client package。
- 下游如果依赖完整 service descriptor，可能看到新增方法。

### CLI 行为

当前 `run -i` 已有“逐行重复提交 prompt/command”的语义。为了避免混淆：

- `run -i --prompt` 应继续保持或明确废弃，需要单独决策。
- `run -it --command` 是新语义。
- `run -it --prompt` 必须拒绝。
- `exec -i` 从当前 unsupported 变成正式 stdin attach。

需要在 help 和错误文案中明确区分：

- line-oriented run interactive
- process-level terminal interactive

### Runtime 行为

Docker 支持可控。BoxLite/Microsandbox 可能需要底层能力补齐。

如果只实现 Docker：

- 非 Docker driver 的 `-it` 必须返回 unsupported。
- 不能回退到普通 exec。

### 日志与 artifact

TTY 模式下 stdout/stderr 合流，无法可靠区分 stderr。现有 transcript model 需要接受：

- TTY output 作为 stdout 或 terminal stream 记录。
- stderr 为空或不可用。

如果继续写 transcript：

- 需要标记 `tty=true`。
- 回放时按原始 terminal bytes 或文本 chunk 展示。

### 安全与资源

交互式 session 会保持连接并可能长期运行：

- 需要合理处理 client disconnect。
- 需要 context cancel 后关闭 stdin/attach connection。
- 需要避免 goroutine 泄漏。
- 需要限制同时打开的 terminal session 数。
- 需要沿用现有权限和 session token 校验。

### 测试影响

需要新增：

- CLI flag 组合测试。
- proto handler 首包校验测试。
- Docker driver terminal 测试。
- client disconnect 清理测试。
- resize 事件测试。
- TTY stdout/stderr 合流测试。
- unsupported driver 测试。

现有 server-stream 测试应保持不变。

## 分阶段实施建议

第一阶段：语义收敛和拒绝错误

- `run/exec` 增加 `-t` flag。
- `--prompt` 与 `-i/-t` 互斥。
- `--json` 与 `-i/-t` 互斥。
- 尚未实现 terminal 通道时，`-it --command` 返回明确 unsupported。

第二阶段：实现 `exec -it`

- 新增双向流 RPC。
- Docker driver 实现 terminal exec。
- CLI raw mode/stdin/stdout/resize。
- 非 Docker driver 返回 unsupported。

第三阶段：实现 `run -it --command`

- 复用 command session RPC。
- 接入 project run/session 生命周期。
- 记录 run status/log/artifact。
- 支持 cleanup policy。

第四阶段：评估是否需要 agent interactive session

这不是 `--prompt -it`。如果未来要支持 agent CLI 持续会话，应独立设计，例如 `agent-compose agent attach` 或 `run --agent-session`，避免污染 prompt run 的一次性任务语义。
