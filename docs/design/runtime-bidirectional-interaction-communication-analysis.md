# Runtime 双向交互核心通信链路分析

## 背景

`agent-compose` 当前有多条从 client、loader、session API 进入 runtime 的执行链路。这些链路在业务入口上不同，但大多最终收敛到 driver 的 `ExecStream`：

```go
ExecStream(context.Context, *Session, VMState, ExecSpec, ExecStreamWriter) (ExecResult, error)
```

现有 `ExecStream` 只有启动参数和 runtime 到 daemon 的单向输出能力。`ExecSpec` 只包含 `Command`、`Args`、`Env`、`Cwd`，`ExecStreamWriter` 只接收 stdout/stderr chunk。因此，当前系统通过 request file、prompt file、script file、result artifact 等文件协议补足结构化输入输出。

如果引入统一双向 stream，新协议的目标不是替换所有 client-facing API，而是在 daemon-runtime 边界统一执行交互语义。外层的 `ExecStream`、`RunAgentStream`、`ExecuteCellStream`、`WatchSession`、`FollowRunLogs` 仍可作为兼容投影存在。

## 核心结论

| 序号 | 链路 | 当前通信模型 | 是否应进入新双向 stream | 迁移重点 |
|---:|---|---|---|---|
| 1 | CLI/API `exec` | `command-request.json` + 单向 stdout/stderr stream | 是 | 支持 stdin、TTY、resize、signal、structured result |
| 2 | CLI/API `run --command` | `command-request.json` + run stream/log | 是 | 复用 command interaction，保留 ProjectRun 投影 |
| 3 | loader `scheduler.exec/shell` | `command-request.json` + notebook cell/session event 投影 | 是 | 迁 runtime 边界，保留 loader/cell 领域事件 |
| 4 | notebook cell | script file + 单向 stdout/stderr stream | 部分是 | source 可继续落 artifact，执行事件统一 |
| 5 | agent prompt / loader `scheduler.agent` | prompt/schema/system prompt file + runtime prompt wrapper | 是 | 统一 agent start/event/result，保留 prompt artifact |
| 6 | driver `ExecStream` | `ExecSpec` + `ExecStreamWriter` 单向输出 | 是，且是核心边界 | 替换或扩展为 `RuntimeInteraction` |
| 7 | runtime LLM facade | runtime 到 daemon 的 HTTP/SSE | 否 | 保留 HTTP/SSE，复用 token/session 鉴权即可 |
| 8 | `WatchSession` / `FollowRunLogs` / workspace 文件 API | daemon 到 client 的事件、tail、文件传输 | 否 | 继续作为 client-facing 投影和数据面 |

## 最终形态：分层架构与数据流

最终架构应先把 daemon-runtime 的最底层数据通道设计成双向交互通道。上层 API、CLI、loader、cell、agent 不直接关心 Docker hijack、Microsandbox event stream、BoxLite guest channel 或 wrapper stdio，而是只依赖统一的 execution interaction contract。

整体分层如下：

```text
+--------------------------------------------------------------+
| 上层应用接口                                                   |
| CLI/API exec | RunAgentStream | ExecuteCellStream             |
| loader scheduler.* | Agent API | Session watch/log projection |
+------------------------------+-------------------------------+
                               |
                               v
+--------------------------------------------------------------+
| 应用适配层                                                     |
| validate request, resolve session, create run/cell records     |
| map domain request -> RuntimeStartSpec                         |
| map RuntimeEvent -> ExecResult/Run status/Cell output/Events   |
+------------------------------+-------------------------------+
                               |
                               v
+--------------------------------------------------------------+
| 统一执行交互 contract                                           |
| StartSpec | RuntimeInput | RuntimeEvent | RuntimeResult        |
| command/cell/agent execution semantics                         |
+------------------------------+-------------------------------+
                               |
                               v
+--------------------------------------------------------------+
| 双向数据通道                                                    |
| Open | Send(stdin/resize/signal/cancel) | Recv(stdout/result) |
+------------------------------+-------------------------------+
                               |
                               v
+--------------------------------------------------------------+
| driver / transport adapter                                     |
| Docker | Microsandbox | BoxLite | guest wrapper framed protocol|
+--------------------------------------------------------------+
```

这个设计的核心是：底层永远是双向能力，上层可以选择只使用其中的单向子集。也就是说，原来“一次输入，多次输出”的命令执行，不需要保留一套独立单向协议，而是可以运行在双向通道之上：

```text
一次性命令模式：
  daemon -> Start(command)
  daemon -> CloseInput or no stdin
  runtime -> Started
  runtime -> Stdout*
  runtime -> Stderr*
  runtime -> Result

交互式命令模式：
  daemon -> Start(command, tty=true, stdin=true)
  runtime -> Started
  daemon -> Stdin*
  daemon -> Resize*
  daemon -> Signal?
  runtime -> Stdout/Stderr*
  runtime -> Result
```

因此，`run/exec -it` 不是一条新特殊链路，而是同一条 runtime interaction 在启用 stdin/TTY/resize 后的自然形态。现有不需要交互的 API 仍然可以通过同一个通道执行，只是不会发送 stdin、resize、signal。

### 最终形态中的数据类型

| 类型 | 方向 | 作用 | 上层是否直接感知 |
|---|---|---|---|
| `RuntimeStartSpec` | daemon -> runtime | 描述执行单元：command、cell、agent、cwd、env、timeout、artifact dir、TTY 等 | 否，上层 request 经适配层转换 |
| `RuntimeInput.Stdin` | daemon -> runtime | 传输用户输入字节 | 只有 `-i` 或未来交互入口感知 |
| `RuntimeInput.StdinEOF` | daemon -> runtime | 表示输入结束 | 交互入口或一次性 stdin 入口感知 |
| `RuntimeInput.Resize` | daemon -> runtime | 传输 terminal rows/cols | 只有 TTY 入口感知 |
| `RuntimeInput.Signal` | daemon -> runtime | 传输 interrupt/terminate 等控制信号 | CLI/API cancel 可映射 |
| `RuntimeEvent.Started` | runtime -> daemon | runtime 确认执行已启动 | 通常投影为 started event |
| `RuntimeEvent.Stdout/Stderr` | runtime -> daemon | 输出流 | 投影到 exec stream、run log、cell output |
| `RuntimeEvent.Result` | runtime -> daemon | 结构化完成结果 | 投影到 ExecResult、Run status、Cell success |
| `RuntimeEvent.Artifact` | runtime -> daemon | 产物引用或产物写入通知 | 投影到 artifact 列表、cell/run state |
| `RuntimeEvent.Heartbeat/Progress` | runtime -> daemon | 长任务存活和进度 | 可投影到 watch/log |

### 指令和数据如何流转

最终形态下，所有执行入口都走同一条抽象路径：

```text
1. 上层入口接收业务请求
   ExecRequest / RunAgentRequest / ExecuteCellRequest / LoaderCommandRequest

2. 应用适配层解析业务语义
   validate -> resolve session -> create run/cell/event records

3. 适配层生成 RuntimeStartSpec
   kind=command/cell/agent
   origin=exec/project_run/loader/cell/agent
   cwd/env/timeout/artifactDir/tty/stdinMode

4. 底层打开双向数据通道
   interaction = runtime.OpenInteraction(ctx, session, vmState, startSpec)

5. daemon 发送输入/控制事件
   no-stdin command: 不发送 stdin 或立即 CloseInput
   stdin command: 发送 Stdin* + StdinEOF
   TTY command: 发送 Stdin* + Resize* + Signal?

6. daemon 接收 runtime event
   Started -> 输出 started projection
   Stdout/Stderr -> 输出 stream/log/cell projection
   Artifact -> 更新 artifact state
   Result -> 结束业务状态机

7. 投影层保持上层行为
   ExecStreamResponse / RunAgentStreamResponse / ExecuteCellStreamResponse
   WatchSession event / FollowRunLogs transcript / artifact files
```

### 上层接口与底层通道的关系

上层接口不需要因为底层变成双向通道而全部改成双向 API。是否暴露双向能力由入口语义决定。

| 上层入口 | 是否需要 client-facing 双向 | 是否使用底层双向通道 | 最终行为 |
|---|---:|---:|---|
| `exec` 普通模式 | 否 | 是 | `Start` 后只接收多次输出和 result |
| `exec -i` | 是 | 是 | client stdin 映射为 `RuntimeInput.Stdin` |
| `exec -it` | 是 | 是 | stdin + TTY + resize 全量启用 |
| `run --command` 普通模式 | 否 | 是 | 一次 start，多次 run output，最终 run status |
| `run --command -it` | 是 | 是 | run stream 同时承载用户输入或新增双向 run API |
| notebook cell | 通常否 | 是 | source/start 一次输入，多次输出 |
| loader command | 否 | 是 | loader script 等待 result，daemon 继续投影 cell event |
| agent prompt | 不一定 | 是 | prompt start + agent event/result；未来可扩展 human input |

这能把架构问题拆开：底层统一提供双向能力，上层只在需要交互时开放双向入口；不需要交互的路径天然兼容为“双向通道上的单向使用”。

### 最终形态中的模块职责

| 模块 | 负责 | 不负责 |
|---|---|---|
| API/CLI/loader handler | 解析用户请求、维持原有 API 语义 | driver transport、runtime event 细节 |
| Execution adapter | request 到 `RuntimeStartSpec` 的转换，event 到领域结果的聚合 | Connect stream、HTTP、Docker API |
| Runtime interaction contract | 定义 start/input/event/result 的稳定语义 | ProjectRun 状态机、cell 存储 |
| Driver transport adapter | 把 Docker/Microsandbox/BoxLite 能力归一成 interaction | 上层业务投影 |
| Projection layer | 把 runtime event 写入 stream/log/cell/session/artifact | runtime transport |

依赖方向应保持单向：

```text
上层应用接口
  -> Execution adapter
  -> Runtime interaction contract
  -> Driver transport adapter

RuntimeEvent
  -> Projection layer
  -> stores / streams / artifacts / logs
```

这样上层原有逻辑可以尽量不变：Exec 仍然返回 ExecResult，Run 仍然维护 ProjectRun，Cell 仍然产生 cell output，Loader 仍然拿到 LoaderCommandResult。变化集中在它们如何进入 runtime，以及 stdout/stderr/result 如何从统一 event 投影回来。

## Contract 设计细化：数据通道与应用接口

在最终形态中，底层数据通道、上层应用接口、产物与投影层需要有清晰边界：

| 层次 | 关注点 | 当前问题 | 新架构目标 |
|---|---|---|---|
| 底层数据通道 | daemon 与 runtime 如何启动执行、传输输入、接收输出、返回结果 | contract 过窄，只覆盖 `ExecSpec` 和 stdout/stderr writer | 定义清晰的双向 execution interaction contract |
| 上层应用接口 | CLI/API/loader/cell/agent 如何表达业务语义 | 每个入口各自拼 file protocol 和输出投影 | 保留入口语义，把它们适配到统一 runtime interaction |
| 产物与兼容层 | 文件 artifact、transcript、历史 API 行为 | 文件同时承担协议、审计、调试多种职责 | 文件保留为 artifact，控制语义逐步迁到 event |

### 当前 daemon-runtime contract 是否清晰

现有底层 contract 形式上清晰，但语义上不完整。

```go
ExecStream(context.Context, *Session, VMState, ExecSpec, ExecStreamWriter) (ExecResult, error)
```

它清楚定义了“启动一个进程，并把 stdout/stderr 写回来”，但没有定义更完整的执行交互：

| 能力 | 现有 contract 是否表达 | 当前补救方式 | 问题 |
|---|---|---|---|
| 启动命令、参数、环境变量、工作目录 | 是 | `ExecSpec` | 基本可用 |
| stdin | 否 | 无统一方式 | 无法支持 `exec -i` |
| TTY | 否 | 无统一方式 | 无法支持 `exec -t` |
| terminal resize | 否 | 无统一方式 | 交互式终端不可用 |
| signal / cancel reason | 部分 | context cancel | 缺少结构化 signal 语义 |
| structured request | 否 | `command-request.json`、prompt file | 文件变成事实控制协议 |
| structured result | 否 | wrapper stdout payload、result file、parse helper | result 与 stdout 混杂 |
| artifact event | 否 | daemon 事后 mirror 文件 | artifact 生命周期不清晰 |
| heartbeat/progress | 否 | 各链路自行处理或没有 | 长任务观测不足 |
| runtime 主动请求 daemon 能力 | 否 | LLM facade 另走 HTTP，其他能力无统一口 | 可扩展性不足 |

因此，现有 contract 的问题不是“完全没有抽象”，而是抽象停留在最小进程执行层，无法承载今天上层已经形成的 command、cell、agent、loader 等执行语义。上层为了补齐缺口，只能不断增加 file protocol、stdout 特殊 payload 和各自的 parse/mirror 逻辑。

### 上层入口与底层 contract 的关系

上层入口不应该直接决定底层 transport。入口负责表达业务意图，底层 contract 负责表达 runtime 执行交互。二者之间需要一个适配层。

```text
上层入口
  CLI/API exec
  CLI/API run --command
  notebook cell
  loader scheduler.exec/shell
  agent prompt
  loader scheduler.agent
        |
        v
应用适配层
  validate request
  resolve session/runtime
  create run/cell/event records
  choose execution kind
  map domain request to RuntimeStartSpec
  map RuntimeEvent back to domain output
        |
        v
底层数据通道
  RuntimeInteraction
  Start / Stdin / Resize / Signal
  Stdout / Stderr / Result / Artifact
        |
        v
driver / runtime wrapper
  Docker / Microsandbox / BoxLite / guest wrapper
```

这意味着新架构不要求上层入口合并成一个 API。`ExecStream`、`RunAgentStream`、`ExecuteCellStream`、loader host API 仍然可以存在，因为它们服务的是不同用户场景。真正需要统一的是它们进入 runtime 之后的 execution contract。

| 上层入口 | 应保留的业务语义 | 映射到底层 interaction 的方式 |
|---|---|---|
| `exec` | 临时命令执行、可交互 stdin/TTY、直接返回 exec result | `Start{kind:command, origin:exec}` + stdio/tty events |
| `run --command` | ProjectRun 生命周期、run log、status transition | `Start{kind:command, origin:project_run, runID}` + run projection |
| loader command | loader run、临时或复用 session、cell artifact、loader event | `Start{kind:command, origin:loader, loaderRunID, cellID}` + cell/loader projection |
| notebook cell | cell source、cell output、kernel event | `Start{kind:cell, cellID, language, sourceRef}` + cell projection |
| agent prompt | provider/model/prompt/schema/resume、agent result | `Start{kind:agent, provider, model, prompt, schema}` + agent event/result projection |

### 新架构应该避免的问题

如果只在某个入口上直接加 `-it` 特例，会继续扩大现有分裂：

```text
exec -it special path
run --command old request file path
loader command old cell request path
cell direct interpreter path
agent prompt file path
```

这样短期可以解决一个 CLI 功能，但底层 contract 仍然不清晰。后续每新增一种交互能力，都要在多个入口重复补丁。

更合理的方向是先定义底层能力边界：

| 能力 | 应属于底层数据通道 | 应属于上层应用接口 |
|---|---|---|
| stdin bytes、stdin EOF | 是 | 决定是否暴露给用户 |
| TTY allocation、resize | 是 | CLI/API 决定是否启用 |
| stdout/stderr chunk | 是 | 上层决定写 transcript、cell output 还是 run stream |
| process exit code | 是 | 上层决定映射成 ExecResult、Run status、Cell success |
| command/cell/agent kind | 是，作为 execution kind | 上层决定如何从业务 request 构造 |
| ProjectRun 状态机 | 否 | 是 |
| notebook cell 存储和事件 | 否 | 是 |
| loader event/topic/state | 否 | 是 |
| prompt/schema artifact | 产物层保留 | 上层负责生成和引用 |

### 建议的模块边界

为了降低风险，可以把新架构拆成三个内部模块，而不是直接把所有 handler 改成新协议：

| 模块 | 职责 | 不应承担的职责 |
|---|---|---|
| `RuntimeInteraction` | 定义 daemon-runtime 双向事件 contract | 不处理 ProjectRun、cell、loader 业务状态 |
| `ExecutionAdapter` | 把 command/cell/agent domain request 映射为 `RuntimeStartSpec`，把 event 聚合为 result | 不直接暴露 Connect/CLI API |
| `ProjectionLayer` | 把 runtime event 投影为 `ExecStreamResponse`、`RunAgentStreamResponse`、cell output、session event、artifact | 不决定底层 driver transport |

建议依赖方向：

```text
API / loader host / run controller / cell executor
        |
        v
ExecutionAdapter
        |
        v
RuntimeInteraction
        |
        v
driver transport

RuntimeEvent
        |
        v
ProjectionLayer
        |
        v
existing streams, stores, files, artifacts
```

### Contract 设计重点

新 contract 应该稳定定义“执行交互事实”，而不是绑定某个入口或某个 driver。

| Contract 字段/事件 | 设计要求 |
|---|---|
| `StartSpec.Kind` | 至少区分 `command`、`cell`、`agent`，但不要包含上层 API 名称 |
| `StartSpec.Origin` | 记录来源，如 `exec`、`project_run`、`loader`，用于审计和 artifact 路径 |
| `StartSpec.ArtifactDir` | 明确 artifact 落点，支持旧文件镜像 |
| `Input.Stdin` | bytes 级输入，不解释业务语义 |
| `Input.Resize` | terminal rows/cols，只有 TTY 模式有效 |
| `Event.Stdout/Stderr` | 只表达 runtime 输出，不直接写 cell/run |
| `Event.Result` | 结构化 exit/success/output/truncation/metadata |
| `Event.Artifact` | 表达 artifact 产生、更新或引用 |
| `Event.Agent*` | 可选扩展，表达 agent message/tool/result 等高层 runtime event |

上层入口只依赖这些稳定语义，不直接依赖 Docker hijack、Microsandbox event kind 或 wrapper stdout payload。

### 渐进式架构落点

不激进的落地方式是先在 daemon 内建立新抽象，但底层仍可桥接旧实现：

```text
Phase 1:
  API/loader/cell still call old code
  introduce RuntimeInteraction interfaces and adapters
  old ExecStream can be wrapped as RuntimeInteraction

Phase 2:
  exec/run command use ExecutionAdapter
  adapter still writes command-request.json
  runtime may still read command-request.json
  daemon starts treating RuntimeEvent.Result as internal result

Phase 3:
  loader command and agent use same adapter
  cell/run/session projections unchanged
  artifact output remains compatible

Phase 4:
  selected drivers implement native bidirectional transport
  compatibility adapter remains for unsupported drivers

Phase 5:
  reduce internal dependence on stdout payload parsing
  keep user-visible artifacts unless explicitly deprecated
```

这个方向的核心是：先统一“数据通道和 contract”，再迁移“应用入口的实现”，最后才考虑“旧文件协议内部依赖的清理”。用户使用方式和可见产物应保持稳定。

## 统一双向交互目标模型

建议 daemon-runtime 边界统一成如下抽象：

```text
daemon
  -> RuntimeInteraction.Open(StartSpec)
  -> Send(Stdin | StdinEOF | Resize | Signal | Cancel | CapabilityResponse)
  <- Recv(Started | Stdout | Stderr | Result | Artifact | Heartbeat | CapabilityRequest | Error)
runtime
```

不同 driver 可以使用不同底层 transport：

| Driver / Wrapper | 可能 transport | 统一后暴露给 daemon 的语义 |
|---|---|---|
| Docker | exec hijack / attach stdin/stdout/stderr / resize API | start、stdin、stdout、stderr、resize、signal、result |
| Microsandbox | 底层 exec event stream | start、stdin、stdout、stderr、exited、failed、result |
| BoxLite | VM stream / guest channel | start、stdin、stdout、stderr、resize、signal、result |
| guest wrapper | framed protocol over stdio/socket | structured request、artifact、result、heartbeat |

关键要求：transport 可以不同，但 adapter 必须归一成同一组 runtime interaction 事件。否则只是新增一条特例链路，无法收敛现有执行面。

## 链路 1：CLI/API `exec`

### 现有逻辑

关键实现：

| 文件 | 责任 |
|---|---|
| `pkg/agentcompose/api/exec.go` | `ExecHandler.executeProjectCommand` 处理 API exec |
| `pkg/execution/command_runtime.go` | `BuildRuntimeCommandExecSpec` 构造 runtime wrapper 命令 |
| `runtime/javascript/src/command.ts` | `agent-compose-runtime exec` 读取 request file 并执行命令 |

现有流程：

```text
client
  -> ExecStream(ExecRequest)
  -> ExecHandler.executeProjectCommand
  -> resolve session / VM state
  -> build RuntimeCommandRequest
  -> write state/exec/<execID>/command-request.json
  -> runtime.ExecStream(sh -lc "agent-compose-runtime exec --request-file ...")
  -> guest wrapper reads command-request.json
  -> wrapper starts target process
  <- stdout/stderr chunks through ExecStreamWriter
  -> daemon filters wrapper payload and appends transcript.txt
  -> ParseCommandExecResult
  -> MirrorRuntimeCommandArtifacts
  <- ExecStreamResponse(started/output/completed)
```

### 现有输入输出

| 方向 | 数据 | 承载方式 | 局限 |
|---|---|---|---|
| client -> daemon | command、args、env、cwd、timeout、max output | `ExecRequest` | API 层结构化，但只到 daemon |
| daemon -> runtime | command request | guest 可见 `command-request.json` | 启动前一次性写完，无法表达运行中 stdin/resize |
| runtime -> daemon | stdout/stderr | `ExecStreamWriter(ExecChunk)` | 没有 event envelope |
| runtime -> daemon | command result/artifact | wrapper stdout payload + result/artifact file | result 需要解析，控制面和数据面混杂 |
| daemon -> client | started/output/completed | `ExecStreamResponse` server stream | 单向输出，无法接收 client stdin |

### 新方案输入输出

```text
client
  -> bidirectional Exec API or compatibility adapter
daemon
  -> RuntimeInteraction.Start{
       kind: "command",
       origin: "exec",
       command,
       args,
       env,
       cwd,
       tty,
       stdinMode,
       timeout,
       maxOutputBytes,
       artifactDir,
     }
  -> RuntimeInput.Stdin / StdinEOF / Resize / Signal / Cancel
runtime
  <- RuntimeEvent.Started
  <- RuntimeEvent.Stdout
  <- RuntimeEvent.Stderr
  <- RuntimeEvent.Result
  <- RuntimeEvent.Artifact
daemon
  -> ExecStreamResponse projection
```

兼容要求：`command-request.json`、`command-result.json`、`stdout.txt`、`stderr.txt`、`output.txt`、`transcript.txt` 第一阶段继续生成。新 stream 的 start/result event 应成为 daemon 内部权威数据，旧文件作为镜像 artifact 和回放输入保留。

`exec -i/-t` 应在这条链路上实现：

| CLI 行为 | daemon-runtime 事件 |
|---|---|
| `-i` attach stdin | `RuntimeInput.Stdin` + `RuntimeInput.StdinEOF` |
| `-t` allocate TTY | `Start.tty = true` |
| terminal resize | `RuntimeInput.Resize{cols, rows}` |
| Ctrl-C / cancel | `RuntimeInput.Signal` 或 context cancel |

## 链路 2：CLI/API `run --command`

### 现有逻辑

关键实现：

| 文件 | 责任 |
|---|---|
| `pkg/runs/controller.go` | `executeProjectRunCommand` 执行 command run |
| `pkg/agentcompose/app/run_controller.go` | `RunAgentStream` 将 run 输出投影到 Connect stream |

现有流程：

```text
client
  -> RunAgentStream(RunAgentRequest with command)
  -> runController.RunProjectAgent
  -> executeProjectRunCommand
  -> write runs/<runID>/command-request.json
  -> runtime.ExecStream(agent-compose-runtime exec --request-file ...)
  <- stdout/stderr chunks
  -> append run transcript.txt
  -> StreamSink.SendChunk
  <- RunAgentStreamResponse(started/output/completed)
  -> transition run status
```

### 现有输入输出

| 方向 | 数据 | 承载方式 | 说明 |
|---|---|---|---|
| client -> daemon | run request、command text、env | `RunAgentRequest` | 外层是 ProjectRun 领域模型 |
| daemon -> runtime | shell command request | `command-request.json` | 和 exec 类似 |
| runtime -> daemon | stdout/stderr | `ExecStreamWriter` | 被写入 run transcript |
| daemon -> client | started/output/completed | `RunAgentStreamResponse` | run 语义投影 |
| daemon -> client | historical/follow logs | `FollowRunLogs` tail `transcript.txt` | 文件 tail，不是 runtime stream |

### 新方案输入输出

`run --command` 应复用 `Start{kind:"command"}`，但保留 ProjectRun 投影：

```text
RunAgentRequest(command)
  -> create ProjectRunRecord
  -> RuntimeInteraction.Start{kind:"command", origin:"project_run", runID, command}
  <- RuntimeEvent.Stdout/Stderr
      -> append transcript.txt
      -> RunAgentStreamResponse(output)
  <- RuntimeEvent.Result
      -> transition run succeeded/failed
      -> RunAgentStreamResponse(completed)
```

兼容要求：run 的 `command-request.json` 和 `transcript.txt` 继续保留，`FollowRunLogs` 仍可 tail `transcript.txt`。即使 runtime 输出已经通过 event 返回，daemon 也应继续写 transcript，避免破坏现有 UI、CLI 或人工排障流程。

## 链路 3：loader `scheduler.exec` / `scheduler.shell`

### 现有逻辑

关键实现：

| 文件 | 责任 |
|---|---|
| `pkg/loaders/run_host.go` | `RuntimeHost.Command` 暴露 loader host API |
| `pkg/agentcompose/adapters/loader_command_executor.go` | `ExecuteLoaderCommand` 执行 loader command 并投影为 notebook cell |

现有流程：

```text
loader script runs in daemon
  -> scheduler.exec/shell
  -> RuntimeHost.Command
  -> ensure command session
  -> LoaderCommandExecutor.ExecuteLoaderCommand
  -> create notebook cell
  -> write cells/<cellID>/command-request.json
  -> runtime.ExecStream(agent-compose-runtime exec --request-file ...)
  <- stdout/stderr chunks
  -> update cell stdout/stderr/output
  -> PublishCellOutput
  -> ParseCommandExecResult
  -> MirrorRuntimeCommandArtifacts
  -> Store.AddCell(completed)
  -> PublishCellCompleted
  -> loader.command.completed/failed event
  <- LoaderCommandResult
```

### 现有输入输出

| 方向 | 数据 | 承载方式 | 说明 |
|---|---|---|---|
| loader script -> daemon | `LoaderCommandRequest` | JS binding / `RuntimeHost.Command` | loader script 本身在 daemon 内运行 |
| daemon -> runtime | command request | `command-request.json` | 和 exec/run command 同源 |
| runtime -> daemon | stdout/stderr | `ExecStreamWriter` | 同时更新 cell 和 loader result |
| daemon -> session watchers | cell started/output/completed | `Streams.PublishCell*` | 外层投影，不是 runtime protocol |
| daemon -> loader script | stdout/stderr/output/exitCode/artifacts | `LoaderCommandResult` | loader API 返回值 |

### 新方案输入输出

迁移时需要保留 loader 的 cell 投影：

```text
scheduler.exec/shell
  -> RuntimeInteraction.Start{
       kind: "command",
       origin: "loader",
       loaderID,
       loaderRunID,
       cellID,
       command or shell,
       env,
       timeout,
       artifactDir,
     }
  <- RuntimeEvent.Stdout/Stderr
      -> streamed accumulator
      -> Store.AddCell(snapshot)
      -> PublishCellOutput
  <- RuntimeEvent.Result
      -> LoaderCommandResult
      -> Store.AddCell(completed)
      -> PublishCellCompleted
      -> loader.command.completed/failed
```

兼容要求：loader command 当前会产生 cell artifact，包括 request/result/stdout/stderr/output。迁移后这些 artifact 仍应继续写入，因为 loader 返回值、session watch、用户调试和历史记录都依赖这一层稳定性。

注意：统一 daemon-runtime stream 不要求 loader script 本身迁移到 runtime。迁移对象只是 loader 调用 runtime 的 command operation。

## 链路 4：notebook cell

### 现有逻辑

关键实现：

| 文件 | 责任 |
|---|---|
| `pkg/agentcompose/api/kernel.go` | `ExecuteCellStream` Connect handler |
| `pkg/agentcompose/adapters/cell_executor.go` | `executeCell` 写脚本并执行解释器 |

现有流程：

```text
client
  -> ExecuteCellStream(ExecuteCellRequest)
  -> CellExecutor.executeCell
  -> normalize cell type
  -> write cells/<cellID>/<script>
  -> runtime.ExecStream(command=python/node/sh, args=<script>)
  <- stdout/stderr chunks
  -> ExecuteCellStreamResponse(output)
  -> WriteCellArtifacts
  -> Store.AddCell(completed)
  -> PublishCellCompleted
  -> session event kernel.cell.succeeded/failed
```

### 现有输入输出

| 方向 | 数据 | 承载方式 | 局限 |
|---|---|---|---|
| client -> daemon | cell type、source | `ExecuteCellRequest` | 结构化到 daemon |
| daemon -> runtime | source | guest script file | source 与 stdin 语义分离 |
| daemon -> runtime | interpreter command | `ExecSpec` | 无 TTY/stdin/resize |
| runtime -> daemon | stdout/stderr | `ExecStreamWriter` | 无 structured result envelope |
| daemon -> client/session | started/output/completed | `ExecuteCellStreamResponse` + session stream | 外层投影 |

### 新方案输入输出

cell 可以部分迁移。source 文件仍可作为可审计 artifact 保存，但执行交互应统一：

```text
ExecuteCellRequest
  -> save source artifact
  -> RuntimeInteraction.Start{
       kind: "cell",
       cellID,
       language,
       sourceRef or inlineSource,
       cwd,
       artifactDir,
     }
  -> optional RuntimeInput.Stdin / StdinEOF / Cancel
  <- RuntimeEvent.Started
  <- RuntimeEvent.Stdout/Stderr
      -> ExecuteCellStreamResponse(output)
      -> PublishCellOutput
  <- RuntimeEvent.Result
      -> WriteCellArtifacts
      -> Store.AddCell(completed)
      -> PublishCellCompleted
```

兼容要求：cell source script 应继续写入 cell state 目录。即使未来 runtime 支持 inline source，script file 仍是最自然的审计和复现 artifact，不建议第一阶段删除。

cell 的 stdin/TTY 应作为可选能力，不应默认改变当前批处理式 notebook 语义。

## 链路 5：agent prompt / loader `scheduler.agent`

### 现有逻辑

关键实现：

| 文件 | 责任 |
|---|---|
| `pkg/agentcompose/adapters/agent_runner.go` | `ExecuteAgentRun` 写 prompt/schema/system prompt 并执行 runtime prompt |
| `pkg/agentcompose/adapters/agent_executor.go` | `ExecuteAgentRequest` 将 agent run 投影成 notebook cell/session event |
| `pkg/agentcompose/adapters/loader_host.go` | loader `scheduler.agent` 入口适配 |
| `runtime/javascript/src/prompt.ts` | `agent-compose-runtime prompt` |

普通 run agent 流程：

```text
RunProjectAgent / AgentExecutor
  -> ExecuteAgentRun
  -> write prompt file
  -> write output schema file
  -> write system prompt file
  -> runtime.ExecStream(agent-compose-runtime prompt --message-file ...)
  -> runtime prompt runner calls Codex/Claude/Gemini/OpenCode
  <- stdout/stderr chunks
  -> ParseAgentExecResult
  <- AgentRunResult
```

loader agent 额外投影为 cell：

```text
scheduler.agent
  -> RuntimeHost.Agent
  -> LoaderHostAgentExecutor.ExecuteAgent
  -> AgentExecutor.ExecuteAgentRequest
  -> ExecuteAgentRun
  <- stdout/stderr/result
  -> update agent cell
  -> add agent.user / agent.assistant session events
  <- LoaderAgentResult
```

### 现有输入输出

| 方向 | 数据 | 承载方式 | 局限 |
|---|---|---|---|
| daemon -> runtime | prompt | prompt file | 启动前一次性输入 |
| daemon -> runtime | output schema | schema file | 无运行时 schema/control event |
| daemon -> runtime | system prompt | system prompt file | 文件协议分散 |
| runtime -> daemon | runner stdout/stderr | `ExecStreamWriter` | agent event 不结构化 |
| runtime -> daemon | final result/session info | wrapper output/result parse + artifact | 需要解析，难支持 human input/tool confirmation |
| daemon -> session/watchers | agent cell/event | `PublishCell*` + `PublishEventAdded` | 外层投影 |

### 新方案输入输出

```text
daemon
  -> RuntimeInteraction.Start{
       kind: "agent",
       provider,
       model,
       runID,
       cellID,
       prompt,
       systemPrompt,
       outputSchema,
       resumeInfo,
       artifactDir,
     }
  -> optional RuntimeInput.HumanMessage / Cancel
runtime
  <- RuntimeEvent.AgentStarted
  <- RuntimeEvent.AgentMessage
  <- RuntimeEvent.ToolCall
  <- RuntimeEvent.ToolResult
  <- RuntimeEvent.Stdout/Stderr
  <- RuntimeEvent.Result{
       finalText,
       json,
       agentSessionID,
       stopReason,
       success,
     }
  <- RuntimeEvent.Artifact(agent-session.json, transcript)
daemon
  -> RunAgentStream / cell / session event projections
```

兼容要求：prompt file、schema file、system prompt file 和 `agent-session.json` 继续保留。agent start event 可以携带结构化 prompt/schema，但文件仍可作为 provider wrapper 的兼容输入和审计 artifact。

建议语义边界：

| CLI/API 能力 | 建议解释 |
|---|---|
| `run --prompt -t` | 不建议映射为 terminal TTY，prompt agent 不是 shell 进程 |
| `run --prompt -i` | 可以表示从 stdin 读取 prompt 或未来 human input，但不等同 process stdin attach |
| agent interactive session | 应使用 agent event/human message 语义，而不是裸 stdin/stdout |

## 链路 6：driver `ExecStream`

### 现有逻辑

现有类型：

```go
type ExecChunk struct {
    Text   string
    Stream StdioStream
}

type ExecSpec struct {
    Command string            `json:"command"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
    Cwd     string            `json:"cwd,omitempty"`
}

type ExecResult struct {
    ExitCode int
    Stdout   string
    Stderr   string
    Output   string
    Success  bool
}

type ExecStreamWriter func(ExecChunk)
```

当前 driver 能力：

| Driver | 当前实现状态 | 缺口 |
|---|---|---|
| Docker | `AttachStdout=true`、`AttachStderr=true`，使用 `stdcopy.StdCopy` | 未设置 `AttachStdin`、`OpenStdin`、`Tty`，没有 resize/signal |
| Microsandbox | 从底层 handle 接收 stdout/stderr/exited/failed/stdin-error event | agent-compose contract 未暴露 stdin/resize |
| BoxLite | 对外同样实现 `ExecStream` | 受限于单向 contract |

### 新方案 contract

建议引入新边界，并让旧 `ExecStream` 作为兼容 wrapper：

```go
type RuntimeInteraction interface {
    Send(RuntimeInput) error
    Recv() (RuntimeEvent, error)
    CloseSend() error
    Wait() (RuntimeResult, error)
}

type RuntimeInteractor interface {
    OpenInteraction(ctx context.Context, session *Session, vmState VMState, spec RuntimeStartSpec) (RuntimeInteraction, error)
}
```

事件建议：

| 类别 | daemon -> runtime | runtime -> daemon |
|---|---|---|
| lifecycle | `Start`、`Cancel` | `Started`、`Exited`、`Failed`、`Result` |
| stdio | `Stdin`、`StdinEOF` | `Stdout`、`Stderr` |
| terminal | `Resize`、`Signal` | terminal ack/error |
| structured | `CapabilityResponse` | `CapabilityRequest`、`Artifact`、`Heartbeat` |
| diagnostics | cancel reason / deadline | structured error / warning |

兼容 wrapper：

```text
old ExecStream(spec, writer)
  -> OpenInteraction(StartSpec from ExecSpec)
  -> read RuntimeEvent.Stdout/Stderr and call writer
  -> wait RuntimeEvent.Result
  -> return ExecResult
```

这样可以先迁核心入口，不要求一次性重写全部调用点。

## 链路 7：runtime LLM facade

### 现有逻辑

关键实现：

| 文件 | 责任 |
|---|---|
| `pkg/agentcompose/proxy/runtime_llm.go` | runtime LLM facade token 校验、协议转换、HTTP/SSE 转发 |

现有流程：

```text
runtime agent SDK / CLI provider
  -> HTTP POST /agent-compose/session/<session_id>/...
  -> daemon validates facade token/session/model/provider
  -> protocol adapter decodes request
  -> daemon calls upstream LLM provider
  <- JSON response or SSE stream
  <- runtime receives provider-compatible response
```

### 是否进入 RuntimeInteraction

不建议。

| 原因 | 说明 |
|---|---|
| 协议语义已经清晰 | LLM facade 是 runtime -> daemon 的 HTTP/SSE 应用协议 |
| 需要兼容上游 SDK | Codex/Claude/OpenAI-compatible SDK 期望 HTTP endpoint |
| 数据方向不同 | 这是 runtime 主动访问 daemon 能力，不是 daemon 启动 runtime process |
| SSE 已能表达 token stream | 没必要包进 command stream |

新双向 stream 可以复用 facade 的 token/session 鉴权模型，但不替代它。

## 链路 8：client-facing watch、tail、workspace 文件 API

### 现有逻辑

| API | 当前模型 | 是否是 daemon-runtime 通道 |
|---|---|---|
| `WatchSession` | 订阅 daemon 内 session stream，推送 session/cell/event 更新 | 否 |
| `FollowRunLogs` | tail run `transcript.txt` 文件 | 否 |
| workspace upload/download | HTTP/file/archive API 或挂载目录 | 否 |
| session RPC | loader 调 daemon 内 session RPC bridge | 否 |
| loader state/event/topic | daemon 内 host capability | 否 |

这些链路不应该并入 RuntimeInteraction。新方案后，它们仍然作为投影或独立数据面存在：

```text
RuntimeInteraction event
  -> daemon domain state
  -> Store.AddCell / AddEvent / UpdateRun
  -> Streams.PublishCellOutput / PublishEventAdded
  -> WatchSession
  -> FollowRunLogs
  -> client-facing stream APIs
```

## 新旧模型对照

### 当前模型

```text
            +------------------+
client ---->| Exec/Run/Cell API |
            +------------------+
                    |
                    v
        +------------------------+
        | daemon adapter/controller
        +------------------------+
          |          |          |
          v          v          v
 request.json   prompt files   script file
          \          |          /
           \         |         /
            v        v        v
          +----------------------+
          | runtime.ExecStream   |
          | ExecSpec + writer    |
          +----------------------+
                    |
                    v
              guest process
                    |
                    v
          stdout/stderr/result parse
                    |
                    v
       ExecStream / RunStream / CellStream
```

### 新模型

```text
            +------------------+
client ---->| Exec/Run/Cell API |
            +------------------+
                    |
                    v
        +------------------------+
        | daemon domain adapter  |
        +------------------------+
                    |
                    v
        +------------------------+
        | RuntimeInteraction     |
        | Start/Input/Event      |
        +------------------------+
          ^                  |
          |                  v
 stdin/resize/signal   stdout/stderr/result/artifact
          |                  |
          +-------- runtime -+
                    |
                    v
          daemon projection layer
                    |
                    v
 ExecStream / RunAgentStream / ExecuteCellStream / WatchSession / FollowRunLogs
```

## 推荐迁移顺序

| 阶段 | 范围 | 目标 | 风险控制 |
|---:|---|---|---|
| 1 | driver 边界 | 引入 `RuntimeInteraction`，保留旧 `ExecStream` wrapper | 不改外层 API，不删除旧 artifact |
| 2 | artifact 镜像层 | 明确 request/result/output/transcript 的生成规范 | 先保证新旧路径产物一致 |
| 3 | `exec` / `run --command` | command request 从文件权威逐步迁到 stream start/input/result 权威 | 继续写 `command-request.json`、result、transcript |
| 4 | loader `scheduler.exec/shell` | 复用 command interaction | 保留 notebook cell/session event 投影和 cell artifact |
| 5 | agent prompt | 统一 agent start/event/result | prompt/schema/system prompt 和 agent-session artifact 继续保留 |
| 6 | notebook cell | 统一 cell execution event envelope | source script 继续作为 artifact，stdin/TTY 作为可选能力 |
| 7 | 清理旧协议依赖 | 减少 `command-request.json`、stdout result parse 的控制面职责 | 只清理内部依赖，不移除用户可见产物 |
| 8 | 可选产物清理 | 对确认无人依赖的旧产物提供配置化清理 | 需要版本公告、迁移文档和回滚开关 |

## 设计边界

| 应统一 | 不应统一 |
|---|---|
| daemon 到 runtime 的 process/agent/cell 执行交互 | runtime LLM facade HTTP/SSE |
| stdin、stdin EOF、resize、signal、cancel | workspace 大文件上传下载 |
| stdout、stderr、result、artifact、heartbeat | `WatchSession` 订阅协议 |
| command、cell、agent 的 runtime start/result envelope | `FollowRunLogs` 文件 tail 协议 |
| driver transport adapter 事件语义 | loader state/event/topic host capability |

最终目标是：入口保留各自领域模型，daemon-runtime 边界统一成可双向交互的执行协议，daemon 再把统一 runtime event 投影回现有 API、日志、cell、session event 和 artifact。
