# daemon-runtime 统一双向交互设计

## 背景

`agent-compose` 当前有多条从入口到 runtime 的执行链路：

- CLI/API `exec`
- CLI/API `run --command`
- CLI/API `run --prompt`
- notebook cell
- loader script 中的 `scheduler.exec` / `scheduler.shell`
- loader script 中的 `scheduler.agent`
- runtime 内 agent 访问 daemon LLM facade
- workspace 文件上传下载、session RPC、run log/cell stream/session watch

这些链路的输入/输出模式并不一致。`exec`、`run --command`、loader command 会先把结构化请求写成 `command-request.json`，再在 guest 中执行 `agent-compose-runtime exec --request-file ...`。agent prompt 会把 prompt/schema/system prompt 写成文件，再执行 `agent-compose-runtime prompt --message-file ...`。cell 会把 source 写成脚本文件，再直接执行解释器。输出则主要依赖 `ExecStream`、`RunAgentStream`、`ExecuteCellStream`、`WatchSession`、`FollowRunLogs` 这类 server-stream 或文件 tail。

这种模式已经能支撑批处理式执行，但它把 daemon 到 runtime 的交互固定成“一次性启动参数 + 单向输出流”。当我们要支持 `run/exec -i/-t`、统一 loader command、减少 `request.json` 文件协议、支持 runtime wrapper 与 daemon 更直接地交换结构化事件时，现有边界会变得分散且难以扩展。

本文从全局视角重新设计 daemon 与 runtime 之间的统一双向流协议。目标不是只为 CLI `-it` 增加一个特例，而是收敛 runtime 执行面的输入/输出交互模型。

这里的“统一”指 daemon 内部看到统一的 `RuntimeInteraction` 语义，不要求所有 driver 都用完全相同的底层 transport。Docker hijack、Microsandbox event stream、BoxLite stream、guest wrapper framed protocol 可以是不同 transport，但都必须被 adapter 归一成同一组 start/stdin/stdout/stderr/resize/signal/result/artifact 事件。否则只是新增另一条特例链路，不能解决当前执行面分散的问题。

## 目标

1. 统一 daemon -> runtime -> daemon 的执行交互协议，减少 `command-request.json`、prompt file、script file 等临时文件协议在控制面上的职责。
2. 支持同一条通道内持续发送 stdin、stdin EOF、terminal resize、cancel、structured request、artifact chunk、result、heartbeat 等事件。
3. 兼容现有入口语义：CLI/API、loader、cell、agent prompt 可以继续保留各自领域模型，但进入 runtime 时尽量复用统一 session/stream contract。
4. 支持 `exec -it`、`run -it --command` 等进程级交互。
5. 为未来 agent interactive session、长连接工具调用、runtime 侧主动请求 daemon 能力预留协议空间。
6. 避免把所有系统交互都硬塞进 daemon-runtime stream：不经过 runtime 的 daemon 内调用应保持原有更简单的 API。

## 非目标

1. 本设计不要求一次性重写所有 client-facing API。`ExecStream`、`RunAgentStream`、`ExecuteCellStream` 可以先作为外层兼容 API 存在。
2. 本设计不把 workspace 大文件上传下载合并到 command stream。大文件仍应走文件/归档 API 或挂载目录。
3. 本设计不把 LLM 上游协议统一改写成 command stream。runtime LLM facade 已经是 HTTP/SSE 语义，应保留。
4. 本设计不把 loader script 本身迁移到 runtime 执行。当前 loader script 在 daemon 内运行，调用 host capability；是否迁移是独立架构议题。

## 当前输入/输出模式盘点

### 1. command request 文件模式

覆盖：

- API/CLI `exec`
- API/CLI `run --command`
- loader `scheduler.exec`
- loader `scheduler.shell`

当前机制：

1. daemon 构造 `RuntimeCommandRequest`。
2. daemon 写入 guest 可见的 `command-request.json`。
3. daemon 通过 runtime driver 执行 `agent-compose-runtime exec --request-file <path>`。
4. guest wrapper 读取 request file，启动目标子进程。
5. wrapper 捕获 stdout/stderr/output，写 artifact，并把 stdout/stderr 转发到 wrapper 自身 stdout/stderr。
6. driver 通过 `ExecStreamWriter` 把输出 chunk 回传给上层。
7. daemon 解析 wrapper 输出中的 command result，再镜像 artifact。

关键实现：

- `pkg/agentcompose/api/exec.go`: `executeProjectCommand`
- `pkg/runs/controller.go`: `executeProjectRunCommand`
- `pkg/agentcompose/adapters/loader_command_executor.go`: `ExecuteLoaderCommand`
- `pkg/execution/command_runtime.go`: `BuildRuntimeCommandExecSpec`
- `runtime/javascript/src/command.ts`: `runExecCommand`, `runProcess`

优点：

- request/result/artifact 文件容易调试和回放。
- wrapper 能统一处理 stdout/stderr 捕获、输出截断、result JSON。
- runtime driver 只需要会执行一个普通进程并读取 stdout/stderr。

问题：

- request 在进程启动前一次性写完，无法表达运行中 stdin。
- 无法表达 TTY、resize、stdin EOF。
- wrapper 与 daemon 之间没有结构化事件通道，result 需要通过文件或特殊 stdout payload 再解析。
- `run --command`、`exec`、loader command 的控制逻辑分散在各自 adapter/controller。

适合迁移到统一双向流：是。它们是最应该优先迁移的一类。

### 2. cell 脚本文件模式

覆盖：

- `KernelService.ExecuteCellStream`
- shell/javascript/python notebook cell

当前机制：

1. daemon 把 cell source 写入 session state 目录下的脚本文件。
2. daemon 根据 cell type 构造解释器命令。
3. runtime driver 执行解释器。
4. stdout/stderr 通过 `ExecStream` 回到 daemon。
5. daemon 发布 cell output，写 cell artifact 和 session event。

关键实现：

- `pkg/agentcompose/adapters/cell_executor.go`: `executeCell`
- `pkg/agentcompose/api/kernel.go`: `ExecuteCellStream`

优点：

- 对批量脚本非常直接。
- source 文件天然可作为 artifact。

问题：

- 运行中 stdin 仍不可用。
- 解释器输入和控制输入分离不清：source 是脚本文件，stdin 没有协议位置。
- 输出只有 stdout/stderr chunk，没有统一 result/event envelope。

适合迁移到统一双向流：部分适合。source 仍可以作为 artifact 保存，但 daemon-runtime 启动和输出结果可以走统一 stream；stdin/TTY 对 cell 可作为可选能力，而不是默认能力。

### 3. agent prompt 文件模式

覆盖：

- API/CLI `run --prompt`
- loader `scheduler.agent`

当前机制：

1. daemon 写 prompt file。
2. daemon 写 output schema file。
3. daemon 写 system prompt file。
4. daemon 构造 `agent-compose-runtime prompt --message-file ... --output-schema-file ...`。
5. runtime prompt runner 调用 Codex/Claude/Gemini/OpenCode。
6. agent runner 输出 stdout/stderr 和最终结果，daemon 解析 result。

关键实现：

- `pkg/agentcompose/adapters/agent_runner.go`: `ExecuteAgentRun`, `BuildAgentExecSpec`
- `runtime/javascript/src/prompt.ts`: `runPromptCommand`
- `runtime/javascript/src/runners/*`

优点：

- prompt/schema/system prompt 文件可审计。
- provider runner 可以隐藏在 guest wrapper 内。
- 对一次性 agent 任务足够稳定。

问题：

- prompt 输入被绑定到文件，无法形成长连接 agent session。
- runner 事件不能作为结构化事件直接回到 daemon，只能通过 stdout/result 解析。
- 不适合表达 agent 运行期间的 human input、tool confirmation、interrupt 等交互。

适合迁移到统一双向流：适合迁移启动请求、runner event、result。prompt 本身可以通过 stream 首包传递，也可以继续落 artifact。`--prompt -t` 应互斥，因为 prompt 不是 terminal 进程。`--prompt -i` 不应在 daemon-runtime 协议层强制互斥；如果 CLI 想保留“从 stdin 读取 prompt 文本或多轮 prompt”的入口语义，可以继续使用 `-i`，但它不是 process stdin attach。

### 4. loader script 与 scheduler API

覆盖：

- `scheduler.exec`
- `scheduler.shell`
- `scheduler.agent`
- `scheduler.llm`
- `sessionRPC`
- topic publish / loader event / state get/set/delete

当前机制：

loader script 在 daemon 内执行。`RuntimeHost` 暴露 host capability：

- `Command` 最终进入 loader command executor，再进入 command request 文件模式。
- `Agent` 最终进入 agent executor，再进入 agent prompt 文件模式。
- `LLM` 直接调用 daemon LLM client。
- `CallSessionRPC` 调 daemon 内 session RPC bridge。
- state/event/topic 基本都是 daemon 内存储和事件能力。

关键实现：

- `pkg/loaders/engine.go`
- `pkg/loaders/run_host.go`: `RuntimeHost.Command`, `Agent`, `LLM`, `CallSessionRPC`
- `pkg/agentcompose/adapters/loader_command_executor.go`

适合迁移到统一双向流：

- `scheduler.exec/shell`: 适合，和 command request 合并。
- `scheduler.agent`: 适合，和 agent prompt 合并。
- `scheduler.llm`: 不适合，它不经过 runtime，是 daemon 到 LLM provider。
- `sessionRPC`: 不适合直接合并，它是 daemon 内 session control plane。
- state/event/topic: 不适合，它们是 loader host capability，不是 runtime process IO。

注意：loader script 本身不应因为统一 daemon-runtime stream 而被迫迁移。迁移对象是 loader 调用 runtime 的那些 operation。

### 5. driver ExecStream 模式

覆盖：

- Docker driver
- BoxLite driver
- Microsandbox driver

当前 contract：

```go
ExecStream(context.Context, *Session, VMState, ExecSpec, ExecStreamWriter) (ExecResult, error)
```

`ExecSpec` 只有 command、args、env、cwd 等启动参数。`ExecStreamWriter` 只有 runtime -> daemon 的输出方向。

Docker 当前只设置：

- `AttachStdout: true`
- `AttachStderr: true`
- `Cmd`
- `Env`
- `WorkingDir`

没有设置：

- `AttachStdin`
- `OpenStdin`
- `Tty`
- resize

Microsandbox 当前 `ExecStream` 从 handle 接收 stdout/stderr/exited 等事件，看起来底层已有事件化 exec 概念，但当前 agent-compose driver contract 没有把 stdin/resize 暴露出来。

适合迁移到统一双向流：是。这是 daemon-runtime 统一交互的核心边界。

### 6. runtime LLM facade

覆盖：

- runtime 内 agent/loader 通过 HTTP 调 daemon LLM facade
- daemon 转发 OpenAI/Anthropic compatible API
- 支持 request/response 和 SSE

关键实现：

- `pkg/agentcompose/proxy/runtime_llm.go`

适合迁移到统一双向流：不建议。它是 runtime -> daemon 的 HTTP 应用协议，且需要兼容上游 LLM SDK。保留 HTTP/SSE 更合理。统一 stream 可以复用它的 token/session 鉴权模型，但不替代它。

### 7. workspace upload/download

覆盖：

- workspace 文件上传
- archive 上传
- 文件下载

适合迁移到统一双向流：不建议。大文件传输应继续走文件 API、HTTP streaming 或挂载目录。command stream 可以传小型 inline payload，但不应承担 bulk data plane。

### 8. output watch / tail

覆盖：

- `ExecStream`
- `RunAgentStream`
- `ExecuteCellStream`
- `WatchSession`
- `FollowRunLogs`

这些是 client-facing 输出同步机制，不完全等同于 daemon-runtime 交互。统一 daemon-runtime stream 后，daemon 仍需要把 runtime event 投影到这些外层 API，保证兼容性。

适合迁移到统一双向流：内部 runtime 输出适合；外部 watch API 不必全部废弃。

## 统一抽象：Runtime Interaction Session

建议新增 daemon-runtime 内部抽象：`RuntimeInteractionSession`。

它表示 daemon 在某个 session/runtime 内启动一个 operation，并在生命周期中双向交换事件。operation 可以是：

- command exec
- shell script
- cell execution
- agent prompt
- future agent interactive session
- future tool sub-process

外层入口仍保留自己的业务语义：

```text
CLI/API/loader/cell
    |
    v
daemon domain operation
    |
    v
RuntimeInteractionSession
    |
    v
driver/runtime transport
    |
    v
guest process or guest runtime wrapper
```

关键原则：

1. 外层 API 可以多，daemon-runtime 协议尽量少。
2. stream 传控制事件和进程/agent 事件，不传大文件主体。
3. artifact 仍落盘，但 artifact 不再是控制协议的唯一载体。
4. request/result 从“必须通过文件交换”变为“stream 首包/尾包为准，文件是审计副本”。

## 合理性与边界判断

统一双向流的收益主要来自“生命周期内交互”的一致性，而不是把所有 IO 都搬进一个管道。判断某条链路是否应该合并，使用以下标准：

1. 是否需要在同一个 runtime operation 生命周期内持续交换输入、输出、控制事件或结构化结果。
2. 是否需要 daemon 统一处理 timeout、cancel、artifact、stdout/stderr、exit code、capability unsupported。
3. 是否以 runtime 内进程或 runtime wrapper 为执行主体。
4. 是否会因继续使用文件协议导致 EOF、resize、result、event、artifact mirror 语义不清。

满足这些条件的链路，例如 command、cell、agent prompt 的 daemon-runtime 部分，适合迁移。反之，workspace 大文件、daemon 内 session RPC、loader state/event/topic、runtime LLM facade HTTP/SSE 不应强行合并。它们不是同一个进程生命周期内的 stdin/stdout/control 交互，迁移后只会把协议变复杂。

因此本设计的边界是：

- runtime operation control plane 统一。
- runtime bulk data plane 不统一。
- client-facing watch/tail API 不强制统一。
- daemon 内部 host capability 不强制统一。

## 协议分层

建议分三层，而不是为每个入口做一套 RPC。

### 第一层：client-facing API

保留现有 API：

- `Exec`
- `ExecStream`
- `RunAgent`
- `RunAgentStream`
- `ExecuteCellStream`
- loader APIs

新增外部双向 stream 只用于需要 client 参与持续输入的场景，例如：

- `exec -i/-t`
- `run -i/-t --command`
- 未来 web terminal
- 未来 agent attach

外部 API 不是本文重点，本文重点是 daemon-runtime。

### 第二层：daemon domain operation

daemon 把各种入口归一成 operation：

```go
type RuntimeOperationKind string

const (
	RuntimeOperationCommand RuntimeOperationKind = "command"
	RuntimeOperationShell   RuntimeOperationKind = "shell"
	RuntimeOperationCell    RuntimeOperationKind = "cell"
	RuntimeOperationAgent   RuntimeOperationKind = "agent"
)
```

每类 operation 有自己的 typed start payload，但共享 IO/control event：

- stdin
- stdin EOF
- stdout
- stderr
- terminal resize
- signal/cancel
- artifact metadata
- result
- error

### 第三层：runtime transport

daemon 到 runtime 的执行可以有两种实现方式：

1. driver-native stream：Docker hijack、Microsandbox stream、BoxLite stream 等。
2. guest runtime wrapper stream：daemon 启动 `agent-compose-runtime session`，再用 stdin/stdout framed protocol 与 wrapper 通信。

短期建议两者并存：

- 对 `exec -it bash` 这类直接命令，Docker 可优先走 driver-native stream，减少 wrapper 干扰 TTY。
- 对 command/agent/cell/loader 这类需要 runtime SDK 统一处理 artifact/result 的 operation，优先走 guest runtime wrapper stream。

长期目标是让 driver 只负责建立字节通道，operation 语义尽量由 `agent-compose-runtime` wrapper 处理。

这里需要避免一个潜在矛盾：文档同时提出 driver-native stream 和 guest wrapper stream，看起来像两套协议。实际设计应要求二者在 daemon adapter 以上表现为同一 `RuntimeInteraction`：

- driver-native stream 由 adapter 把 Docker/Microsandbox/BoxLite 的 native stdout/stderr/exit/resize API 翻译成 `RuntimeOutputFrame`。
- guest wrapper stream 由 wrapper 直接读写 framed protocol，adapter 只负责搬运 frame。
- 上层 controller、loader、cell、agent runner 不感知底层 transport 差异。
- transport 选择只能由 capability 和 operation kind 决定，不能让业务层散落 `if docker && tty` 这类判断。

## 建议的 daemon-runtime 事件协议

建议新增一个 framed protocol，可以承载在：

- Connect/gRPC 双向流，适用于 daemon 与远端 runtime service。
- Docker exec attach stdin/stdout，适用于本地 Docker guest wrapper。
- WebSocket，适用于 future browser/web terminal 或远端 runtime。

事件 envelope：

```proto
enum RuntimeFrameDirection {
  RUNTIME_FRAME_DIRECTION_UNSPECIFIED = 0;
  RUNTIME_FRAME_DIRECTION_DAEMON_TO_RUNTIME = 1;
  RUNTIME_FRAME_DIRECTION_RUNTIME_TO_DAEMON = 2;
}

message RuntimeStreamFrame {
  string stream_id = 1;
  uint64 seq = 2;
  RuntimeFrameDirection direction = 3;
  oneof frame {
    RuntimeStart start = 10;
    RuntimeStdin stdin = 11;
    RuntimeStdinEOF stdin_eof = 12;
    RuntimeSignal signal = 13;
    RuntimeResize resize = 14;
    RuntimeStdout stdout = 20;
    RuntimeStderr stderr = 21;
    RuntimeEvent event = 22;
    RuntimeArtifact artifact = 23;
    RuntimeResult result = 24;
    RuntimeError error = 25;
    RuntimeHeartbeat heartbeat = 26;
  }
}
```

start payload：

```proto
message RuntimeStart {
  string operation_id = 1;
  RuntimeOperationKind kind = 2;
  string cwd = 3;
  repeated EnvVar env = 4;
  RuntimeIOOptions io = 5;
  RuntimeArtifactOptions artifacts = 6;
  RuntimeProtocolOptions protocol = 7;
  oneof spec {
    CommandSpec command = 20;
    ShellSpec shell = 21;
    CellSpec cell = 22;
    AgentPromptSpec agent = 23;
  }
}

message RuntimeIOOptions {
  bool attach_stdin = 1;
  bool tty = 2;
  uint32 rows = 3;
  uint32 cols = 4;
  int64 timeout_ms = 5;
  int64 max_output_bytes = 6;
}

message RuntimeProtocolOptions {
  uint32 version = 1;
  repeated string required_features = 2;
  repeated string optional_features = 3;
}
```

operation specs：

```proto
message CommandSpec {
  string command = 1;
  repeated string args = 2;
}

message ShellSpec {
  string script = 1;
}

message CellSpec {
  string cell_id = 1;
  string cell_type = 2;
  string source = 3;
}

message AgentPromptSpec {
  string agent = 1;
  string model = 2;
  string prompt = 3;
  string output_schema_json = 4;
  string system_prompt = 5;
}
```

输出与结果：

```proto
message RuntimeStdout {
  bytes data = 1;
}

message RuntimeStderr {
  bytes data = 1;
}

message RuntimeEvent {
  string type = 1;
  string level = 2;
  bytes json_payload = 3;
}

message RuntimeArtifact {
  string name = 1;
  string guest_path = 2;
  string media_type = 3;
  int64 size = 4;
  bytes inline_data = 5;
}

message RuntimeResult {
  int32 exit_code = 1;
  bool success = 2;
  bytes stdout = 3;
  bytes stderr = 4;
  bytes output = 5;
  bytes json_payload = 6;
  repeated RuntimeArtifact artifacts = 7;
}
```

协议规则：

1. 第一帧必须是 `start`。
2. `start` 只能由 daemon 发往 runtime。
3. `stdin`、`stdin_eof`、`signal`、`resize` 只能由 daemon 发往 runtime。
4. `stdout`、`stderr`、`event`、`artifact`、`result`、`error` 只能由 runtime 发往 daemon。
5. `tty=true` 必须满足 `attach_stdin=true`，除非未来明确支持只读 TTY。
6. `tty=true` 时 runtime 主要发送 `stdout`，stderr 由 PTY 合流。
7. `attach_stdin=false` 时 runtime 必须关闭子进程 stdin；等待 stdin 的程序应按 EOF 行为退出或继续但不可卡在 daemon 持有的空管道上。
8. `stdin_eof` 表示 daemon 不再发送 stdin。runtime 收到后必须关闭子进程 stdin，而不是只停止读 frame。
9. `signal` 可表达 SIGINT/SIGTERM/KILL，driver 不支持时返回 structured error。
10. `resize` 只对 TTY operation 有效。
11. `result` 是 operation 的最终结果。`result` 后只能发送 heartbeat/transport close，不应再发送 stdout/stderr。
12. artifact 可 inline 小数据；大文件只发路径和 metadata。
13. `seq` 在每个方向单调递增，用于日志定位和测试断言；第一阶段不要求重传。
14. `version` 不匹配或缺少 required feature 时，runtime 必须在启动目标进程前返回 `RuntimeError`。
15. optional feature 不支持时可以忽略，但 runtime 应通过 `RuntimeEvent` 汇报实际启用能力，便于 daemon 记录和调试。

## 生命周期状态机

为了避免不同入口各自解释 stream，需要显式定义状态机：

```text
Created
  -> Started          daemon sends start
  -> Running          runtime acknowledges by first stdout/stderr/event or started event
  -> StdinClosed      daemon sends stdin_eof or attach_stdin=false
  -> Terminating      daemon sends signal/cancel or context deadline
  -> Completed        runtime sends result(success or non-zero exit)
  -> Failed           runtime sends error or transport fails before result
  -> Closed           transport closed and daemon has persisted projection
```

状态规则：

- `Completed` 是业务完成，必须有 `RuntimeResult`。
- `Failed` 是协议、transport、wrapper 或 driver 失败，可以没有 `RuntimeResult`，daemon 需要用已收集 stdout/stderr 合成失败结果。
- `context cancel` 必须转成 runtime cancel，并关闭 stdin/attach connection。
- client disconnect 不等同于 operation 成功；对于 foreground interactive operation 应 cancel runtime，对于 detached/background operation 应由入口策略决定是否继续。
- timeout 属于 daemon 控制事件，优先发送 SIGTERM，宽限期后 SIGKILL 或 driver 等价操作。

这个状态机是闭环关键：每个 operation 必须最终落到 `Completed` 或 `Failed`，并被 daemon 投影成现有 run/cell/exec/loader 的结果与 artifact。

## 外层投影闭环

统一 daemon-runtime stream 后，daemon 仍要负责把 runtime frame 投影到现有业务模型：

| Runtime frame | exec | run --command | cell | loader command | agent prompt |
| --- | --- | --- | --- | --- | --- |
| stdout/stderr | ExecStream chunk、transcript | RunAgentStream chunk、run log | cell output、session stream | cell output、loader result capture | agent run transcript |
| event | ExecStream transcript event 或内部日志 | run event/log | session event | loader event | agent event |
| artifact | exec artifact | run artifact | cell artifact | loader command artifact | prompt/result artifact |
| result | ExecResult | ProjectRun transition | NotebookCell final state | LoaderCommandResult | AgentRunResult |
| error | failed ExecResult | failed run transition | failed cell | failed LoaderCommandResult | failed AgentRunResult |

闭环要求：

- runtime frame 是事实来源。
- 现有 artifact 文件是 projection，不再是唯一事实来源。
- projection 必须幂等，避免 stream retry 或 daemon crash recovery 时重复写坏状态。
- TTY operation 的 stdout/stderr projection 必须标记 `tty=true`，不能假装 stderr 可可靠拆分。

## 迁移策略

### 阶段 1：新增内部 contract，不改变外部 API

新增 domain contract：

```go
type RuntimeInteraction interface {
	RunInteraction(ctx context.Context, session *domain.Session, vmState domain.VMState, start RuntimeStart, io RuntimeInteractionIO) (RuntimeResult, error)
}

type RuntimeInteractionIO struct {
	Inbound  <-chan RuntimeInputFrame
	Outbound chan<- RuntimeOutputFrame
}
```

driver adapter 增加：

- `RunInteraction`
- capability discovery，例如 `SupportsTTY`、`SupportsStdin`、`SupportsRuntimeWrapperStream`

旧 `ExecStream` 先保留，并可由 `RunInteraction` 包一层实现，或继续并行存在。

第一阶段还必须补一个底层能力：当前 `ExecStream` 只有 stdout/stderr 回调，没有 daemon -> runtime 输入方向。即使先不暴露外部 `-it`，wrapper stream 也需要 driver 能启动 wrapper 后写 stdin、读 stdout/stderr、关闭 stdin、取消进程。因此 contract 不能只新增高层 `RunInteraction`，还要在 driver adapter 中定义“可双向附着进程”的最小能力，或由各 driver 直接实现 `RunInteraction`。

### 阶段 2：迁移 command request

优先迁移：

- `exec`
- `run --command`
- loader `scheduler.exec/shell`

原 `RuntimeCommandRequest` 字段映射到 `RuntimeStart`：

- `mode=exec` -> `CommandSpec`
- `mode=shell` -> `ShellSpec`
- `cwd/env/timeout/maxOutputBytes/artifactDir` -> start options

`command-request.json` 不再是 wrapper 启动必需输入，但仍可作为 artifact mirror：

- daemon 可保存 `runtime-start.json`。
- runtime 可保存 normalized request。
- result 通过 stream `RuntimeResult` 返回，同时写 `command-result.json` 作为审计副本。

兼容策略：

- 第一阶段保留 `command-request.json` 文件名，内容从 `RuntimeStart` mirror 生成。
- 新增 `runtime-start.json` 只能作为补充，不能立即替换旧文件名。
- `ParseCommandExecResult` 这类旧解析逻辑应逐步下沉到 projection 层，不能继续依赖 stdout 中的特殊 payload。
- daemon crash recovery 可以优先读取 artifact 中的 `command-result.json`，但正常在线路径以 stream result 为准。

### 阶段 3：迁移 agent prompt

把 prompt/schema/system prompt 从文件控制面迁移到 `AgentPromptSpec`：

- prompt 文本在 start frame 中传递。
- output schema/system prompt 也在 start frame 中传递。
- runtime wrapper 仍可在 guest 内写 prompt artifact。
- provider runner 的事件可以转成 `RuntimeEvent`。
- 最终 agent result 通过 `RuntimeResult.json_payload` 返回。

CLI 语义修正：

- `--prompt -t` 互斥，因为 prompt 不是 terminal。
- `--prompt -i` 不在协议层互斥；CLI 可定义为从 stdin 读取 prompt、重复提交 prompt，或未来 attach agent session。但不能把它解释为子进程 stdin，除非进入明确的 agent interactive session 模式。

### 阶段 4：迁移 cell

cell source 进入 `CellSpec.source`：

- daemon 仍保存 cell source artifact。
- runtime wrapper 可按 cell type 写临时脚本再执行。
- stdout/stderr/result 通过 stream 返回。
- 若未来 cell 支持 stdin，可复用 `RuntimeStdin`。

### 阶段 5：支持 TTY / stdin / resize

实现顺序：

1. Docker driver-native `exec -it`。
2. wrapper stream 的 non-TTY stdin。
3. wrapper stream 的 TTY，需要 guest 内 pty 支持。
4. Microsandbox/BoxLite 根据底层能力补齐或明确 unsupported。

## daemon 到 runtime 的实现形态

### Docker driver-native

适用：

- `exec -it bash`
- `run -it --command bash`
- 不需要 runtime wrapper artifact/result 的简单 command

Docker 映射：

- `AttachStdin: attach_stdin`
- `OpenStdin: attach_stdin`
- `Tty: tty`
- `AttachStdout: true`
- `AttachStderr: !tty`
- resize -> Docker exec resize API
- stdin -> hijacked connection
- stdout/stderr -> hijacked connection / stdcopy

优点：

- 最贴近 Docker 行为。
- TTY 稳定。
- 不受 wrapper stdio framing 影响。

缺点：

- artifact/result 需要 daemon 自己收集。
- agent/cell 等复杂 operation 不适合直接走 native command。

driver-native 虽然不经过 guest wrapper framed protocol，但仍必须返回 `RuntimeInteraction` frame。也就是说 native path 不是“绕过统一协议”，而是由 adapter 负责把 native IO 翻译成统一 frame。否则 `exec -it` 会再次变成一条孤立链路。

### Guest wrapper stream

适用：

- command request 替代
- loader command
- cell
- agent prompt
- future agent interactive session

daemon 启动：

```text
agent-compose-runtime session --protocol runtime-stream-v1
```

然后通过 stdin/stdout 交换 framed frame。stderr 可保留为 wrapper diagnostic，或也纳入 frame。

这里必须固定 stdio 约定，否则很容易与现有 stdout/stderr 捕获冲突：

- wrapper stdout 只输出 framed protocol，不允许混入人类可读日志。
- wrapper stdin 只接收 framed protocol。
- wrapper stderr 可以作为 wrapper diagnostic，但 daemon 必须把它作为 transport diagnostic，而不是目标进程 stderr。
- 目标进程 stdout/stderr 必须被 wrapper 封装成 `RuntimeStdout` / `RuntimeStderr` frame。
- 如果 wrapper 自身崩溃且只留下 stderr diagnostic，daemon 应生成 `RuntimeError` 并合成失败 result。

优点：

- request/result/event/artifact 语义统一。
- runtime SDK 可以集中处理 max output、artifact、agent provider 事件。
- driver 只要能启动 wrapper 并建立 stdin/stdout 双向管道即可。

缺点：

- framing 必须处理二进制安全、backpressure、半关闭、错误恢复。
- TTY 模式不能直接把 framed protocol 和 terminal raw bytes 混在同一 stdout；需要单独 channel 或切换到 native TTY。

结论：

- 非 TTY operation 优先走 wrapper stream。
- TTY terminal operation 优先走 driver-native stream。
- 如果未来必须在 wrapper 内支持 TTY，应让 wrapper 自己分配 pty，并用 framed `stdout` 承载 PTY bytes，不能把 protocol frame 和 terminal raw stream 混写。

## 哪些交互应该合并

应该合并到统一 Runtime Interaction：

- `exec` 普通命令
- `exec -i/-t`
- `run --command`
- `run -i/-t --command`
- loader `scheduler.exec`
- loader `scheduler.shell`
- cell execution
- agent prompt run 的 daemon-runtime 部分
- future agent attach / interactive agent session
- runtime wrapper 产出的 structured event/result/artifact metadata

不应合并，或只复用鉴权/事件模型：

- daemon 内 loader state/event/topic
- daemon 内 session RPC bridge
- daemon LLM client 直接请求
- runtime LLM facade HTTP/SSE
- workspace 大文件 upload/download
- session lifecycle API 本身
- run log/cell/session watch 的外层 client-facing projection

## 外部 CLI/API 语义

### run/exec --command

- `-i`：attach local stdin 到 runtime operation stdin。
- `-t`：要求 `-i`，分配 TTY。
- 不启用 `-i`：stdin 必须关闭，行为接近 Docker。
- `--json` 与 `-i/-t` 互斥，因为交互输出不是稳定 JSON response。

### run --prompt

- `--prompt -t` 互斥。
- `--prompt -i` 不在协议层互斥，但 CLI 必须明确语义：
  - 可以表示从 stdin 读取 prompt 文本。
  - 可以保留当前“逐行重复提交 prompt/command”的语义。
  - 不应表示把 stdin attach 到 agent 子进程，除非启用未来专门的 agent interactive session。

### loader script

loader script API 不直接暴露 `-i/-t`，但 `scheduler.exec/shell` 可以在未来增加：

- `stdin`
- `interactive`
- `tty`
- `timeout`
- `maxOutputBytes`

其中 `stdin` 可以是 string/bytes，一次性写入后发送 EOF；真正交互式 loader command 需要 loader script 自己持有 async stream handle，设计复杂度更高，可后置。

## 破坏性分析

### API 兼容性

新增 daemon-runtime internal contract 不破坏外部 API。新增外部双向 RPC 时：

- Go generated proto 会变化。
- TypeScript client package 会变化。
- 下游如依赖 service descriptor，会看到新增方法。

旧 server-stream API 应继续保留，直到 UI/CLI/SDK 完成迁移。

外部 `--prompt -i` 的语义需要单独迁移决策。当前全局设计只要求 `--prompt -t` 互斥，不在协议层禁止 `--prompt -i`。如果 CLI 当前已有 `run -i` 逐行提交 prompt/command 的行为，后续改为“stdin attach”会破坏用户预期。因此：

- `--command -i` 可以定义为 process stdin attach。
- `--prompt -i` 应继续表示入口层读取 prompt，或明确改名/新增模式。
- agent interactive session 应使用独立命令或显式 flag，不能偷换 `--prompt -i` 含义。

### runtime artifact 兼容性

从 request file 转 stream 后，artifact 仍应保留：

- `command-request.json` 可改名或并存为 `runtime-start.json`。
- `command-result.json` 继续写。
- `stdout.txt`、`stderr.txt`、`output.txt` 继续写。

短期为了兼容测试和已有用户，建议保留原文件名，内容可由 stream start/result mirror 生成。

### 日志与 transcript

TTY 模式 stdout/stderr 合流：

- transcript 应标记 `tty=true`。
- stderr 可能为空。
- 回放应按 terminal bytes 或文本 chunk 展示。

非 TTY 模式继续区分 stdout/stderr。

### driver 能力差异

Docker 最容易完整支持 stdin/TTY/resize。

Microsandbox 当前已有 event stream 形态，但是否支持 stdin/TTY/resize 要看底层 SDK。BoxLite 也需要确认能力。

原则：

- capability 不足时返回 unsupported。
- 不要静默降级成普通 `ExecStream`。
- CLI 错误必须明确说明当前 driver 不支持哪项能力。

建议 capability 用结构化矩阵表达，而不是若干 bool 零散判断：

```go
type RuntimeInteractionCapabilities struct {
	WrapperStream bool
	NativeExec    bool
	Stdin         bool
	StdinEOF      bool
	TTY           bool
	Resize        bool
	Signal        bool
	Artifacts     bool
}
```

operation 选择 transport 时先做 capability resolution：

- command non-TTY 优先 wrapper stream，缺失时可回退 native exec，但必须保证 result/artifact projection 等价。
- command TTY 优先 native exec，缺失时只有 wrapper pty 能力完整才可使用 wrapper stream。
- agent/cell 需要 wrapper stream；不能回退 native exec，除非 daemon 重新实现对应 wrapper 逻辑。

### 安全与资源

双向 stream 会带来长期连接：

- 必须处理 client disconnect。
- 必须关闭 stdin pipe / attach connection。
- 必须 cancel runtime process。
- 必须限制并发 interactive session 数。
- 必须防止未消费输出导致 goroutine 堵塞。
- 必须沿用 session token、loader source、project permission 等现有鉴权。

### 协议复杂度

统一协议不能变成无限扩展的垃圾桶。需要约束：

- start payload typed。
- event type registry。
- artifact 大小限制。
- backpressure 策略。
- frame version。
- feature negotiation。

## 测试计划

单元测试：

- start frame 首包校验。
- `tty=true` 必须 `attach_stdin=true`。
- `attach_stdin=false` 时 stdin EOF/关闭。
- stdout/stderr/result 顺序。
- artifact mirror。
- unsupported capability error。

集成测试：

- Docker `exec -i`：`cat` 能接收 stdin 并 EOF。
- Docker `exec -it`：`bash -lc 'stty size; read x; echo $x'`。
- resize 事件传递。
- client disconnect 后进程清理。
- loader `scheduler.shell` 通过 Runtime Interaction 执行。
- agent prompt 通过 Runtime Interaction 返回结构化 result。
- cell 通过 Runtime Interaction 保存 cell artifact。

兼容测试：

- 旧 `ExecStream` 输出不变。
- 旧 `RunAgentStream` 输出不变。
- 旧 `ExecuteCellStream` 输出不变。
- 原 artifact 文件仍存在。

## 推荐落地路线

1. 设计并引入 internal `RuntimeInteraction` contract 和 frame model。
2. 用 wrapper stream 迁移 command request，但保留旧 artifact 文件名。
3. 将 loader `scheduler.exec/shell` 切到同一 command operation。
4. 实现 Docker native `exec -it`，外部 CLI 双向 RPC 接入。
5. 将 `run -it --command` 接入相同 command operation。
6. 迁移 cell 到 `CellSpec`，保留 source artifact。
7. 迁移 agent prompt 到 `AgentPromptSpec`，保留 prompt/schema/system prompt artifact。
8. 评估 wrapper stream TTY 和 future agent interactive session。

这条路线可以先统一 daemon-runtime 协议，再逐步收敛外层 API，不需要一次性重写所有入口。
