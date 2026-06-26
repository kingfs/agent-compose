# agent-compose 概念定位与对外使用设计分析

本文档从产品、架构和研发实现角度，分析 agent-compose 当前能力、概念边界、对外使用方式，以及下一阶段更科学的产品化方向。

本文不是具体实现任务清单，也不要求 agent-compose 实现具体业务逻辑。本文讨论的是：agent-compose 应该如何提供一套稳定、清晰、可组合、可治理的基础能力，让外部用户或上层平台能够基于它编写和运行自己的业务逻辑。

## 1. 背景和核心问题

agent-compose 当前已经具备一套可运行的 agent/session 控制面：

- daemon 负责 project、agent definition、loader、run、session、runtime、image、capability、LLM facade 等生命周期管理。
- compose 文件负责描述 project、agent、workspace、runtime driver、scheduler 等配置。
- loader/scheduler 可以通过 YAML trigger 或 inline JavaScript 触发 agent、LLM、exec、shell 等能力。
- guest runtime 内提供 `agent-compose-runtime-js` 和 `@chaitin-ai/agent-compose-runtime-sdk`，用于适配 provider CLI 和支持 guest 内脚本调用平台能力。

但当前概念仍存在明显混合：

- agent 同时承担业务身份、provider 配置、runtime 配置、workspace 绑定和能力绑定。
- scheduler/loader JS 既能表达触发方式，也能直接承载业务逻辑。
- JS SDK 已经具备业务编排基础能力，但其对应的业务逻辑代码没有成为明确的一等概念。
- output schema 只在部分调用路径中可选存在，input schema 基本缺失。
- compose 文件描述了运行配置，但还没有形成完整、严格、可导入、可复用的 project manifest bundle。

因此，需要重新梳理 agent-compose 的定位和边界：

```text
agent-compose 不应该负责实现用户的具体业务逻辑；
agent-compose 应该负责提供定义、运行、触发、编排、校验、治理业务逻辑所需的基础能力。
```

## 2. 总体定位

agent-compose 建议定位为：

```text
面向 agent/service 工作负载的 project manifest 控制面和 runtime 执行平台。
```

更具体地说：

- daemon 是控制面，负责接收配置、校验配置、准备资源、管理生命周期、调度运行、记录状态和对外提供 API。
- runtime 是执行面，负责在受控 sandbox 中运行 agent provider、命令、脚本和未来更明确的业务逻辑入口。
- compose/manifest 是声明式入口，负责表达一个 project 的期望状态。
- loader/scheduler 是触发层，负责表达何时运行、由什么事件运行、运行后如何路由。
- JS SDK 是业务逻辑代码调用平台能力的标准工具箱。
- 业务逻辑代码由用户或上层平台编写，agent-compose 提供运行规范、SDK、schema 校验、资源注入和生命周期管理。

## 3. 关键概念定义

### 3.1 Project

Project 是一组 agent、业务逻辑入口、触发器、workspace、runtime 约束和权限配置的集合。

Project 不代表一次运行。Project 是可版本化、可导入、可更新、可回滚的期望状态。

当前代码中，Project 已经存在于 v2 API 和持久化模型中。下一阶段应强化 Project 作为 manifest 的根对象。

### 3.2 Compose File / Project Manifest

Compose file 是 project manifest 的主要文本形态。

当前 `agent-compose.yml` 已经支持描述 project name、variables、workspace、agents、scheduler 和 network。未来更科学的方向是让它成为严格 schema 化的 manifest：

- 只描述结构和引用，不承载大段业务实现代码。
- 可以引用外部文件，例如 service JS、schema JSON、prompt markdown。
- 可以被 daemon API 或 CLI 导入。
- 可以被校验、打包、发布和回滚。

### 3.3 Agent

当前 agent 是一个可运行的 AI provider 配置，主要包含 provider、model、system prompt、workspace、runtime、env、capset 等。

建议后续明确 agent 的定位：

```text
Agent 是 AI provider profile 或 AI 能力依赖，不等同于完整业务服务。
```

换句话说，agent 描述的是“使用哪个 AI provider、哪个 model、以什么身份和上下文执行”，而不是完整描述一个业务能力的输入、输出和实现逻辑。

### 3.4 Business Logic / Service Code

本文使用“业务逻辑代码”指用户为完成具体业务目标而编写的代码，例如风险分析、代码审查、报告生成、数据同步、事件处理等。

需要强调：agent-compose 不需要实现这些业务逻辑。agent-compose 应该提供以下基础能力：

- 运行这类代码的 runtime 环境。
- 调用 agent、LLM、命令、能力网关、事件、状态、日志和 artifact 的 SDK。
- input/output schema 校验能力。
- 权限、secret、workspace、capability 注入能力。
- 调度、重试、超时、并发、审计和生命周期管理。

因此，“service JS”不是指 agent-compose 内置大量业务代码，而是指 agent-compose 应支持一种标准的业务逻辑代码入口形态。

### 3.5 JS SDK

JS SDK 是业务逻辑代码调用 agent-compose 平台能力的标准方式。

当前已有 `@chaitin-ai/agent-compose-runtime-sdk`，包含 `runtime.exec`、`runtime.shell`、`runtime.agent`、`runtime.llm` 等基础能力。

未来 JS SDK 应该继续承担以下职责：

- 封装平台能力，避免业务代码直接依赖内部文件路径、magic stdout payload 或 daemon 私有协议。
- 提供清晰 TypeScript 类型和文档，方便研发直接使用。
- 提供机器可读说明，方便 AI 根据业务需求生成可运行代码。
- 提供测试、mock、schema 校验和本地 dry-run 能力。

### 3.6 Loader / Scheduler

Loader/Scheduler 是触发和轻量编排层。

当前已有两种形式：

- YAML 声明式 trigger：适合 cron、interval、timeout、event 等常见触发。
- inline `scheduler.script`：适合复杂触发、条件判断、状态路由和高级编排。

建议继续保留两种形式，但明确边界：

- YAML trigger 是推荐路径，面向常见场景。
- loader JS 是高级 escape hatch，面向 YAML 难以表达的复杂触发逻辑。
- loader JS 不应成为承载复杂业务逻辑的推荐方式。
- 复杂业务逻辑应通过 SDK 在 runtime 内运行。

### 3.7 Runtime

Runtime 是受控执行环境。

当前 runtime driver 包括 docker、boxlite、microsandbox。runtime 根据 daemon 准备的 session、workspace、env、image、driver、capability 等约束运行 agent provider、命令和脚本。

未来 runtime 应继续向“标准业务逻辑执行容器”演进，但核心职责仍然是执行，不是控制。

## 4. 三类职责必须分离

### 4.1 agent-compose 运行必要配置

这类配置是平台运行所需，例如：

- runtime driver
- guest image
- workspace
- env 和 secret
- capset/capability
- model/provider credential 注入
- session cleanup policy
- network policy
- timeout 和资源限制

它们决定“在哪里、以什么权限和资源运行”。

### 4.2 触发方式

触发方式决定“什么时候运行”，例如：

- 手动运行
- API 调用
- cron
- interval
- timeout
- event
- webhook
- 后续可能的队列消息、审批流、外部系统回调

触发方式不应该决定业务逻辑本身。

### 4.3 业务逻辑

业务逻辑决定“输入是什么、执行什么、输出什么”。

它应该尽量与触发方式无关。同一个业务逻辑入口可以被手动调用、API 调用、定时调用、事件调用，也可以被另一个业务逻辑入口调用。

这是对外产品化的关键：只有业务逻辑入口具备清晰 input/output schema，系统才有可能支持 UI 表单、API 自动调用、服务串联、服务并联、审计和复用。

## 5. 当前方案与新定位方案

### 5.1 当前方案概括

当前方案可以概括为：

```text
compose file -> project -> agent definition -> scheduler/loader -> runtime session -> agent provider / shell / JS runtime
```

优点：

- 已经可以运行 agent。
- 已经支持 project apply、manual run、scheduler、event、runtime driver、workspace、image、LLM facade。
- YAML trigger 和 inline scheduler JS 已经能覆盖不少自动化场景。
- guest SDK 已经具备初步业务编排能力。

不足：

- agent、trigger、runtime 和业务逻辑边界不清。
- 业务逻辑没有一等入口定义。
- input schema 缺失。
- output schema 是调用时能力，不是业务能力固有契约。
- loader JS 容易被误用为复杂业务实现层。
- compose 文件对外表达能力偏运行配置，而不是完整 project manifest。
- JS SDK 能力和文档还不足以支撑大规模 AI 生成和维护业务代码。

### 5.2 新定位方案概括

新定位方案不是让 agent-compose 写业务逻辑，而是让 agent-compose 提供业务逻辑运行基础。

```text
project manifest
  -> runtime/resource/capability constraints
  -> agent provider profiles
  -> business logic entries with schema and code references
  -> triggers invoking those entries
  -> daemon validates, prepares, schedules, runs and records
```

在新定位下：

- agent-compose daemon 接收严格 manifest 或 API 配置。
- manifest 可以引用业务逻辑代码文件和 schema 文件。
- 业务逻辑代码通过 JS SDK 调用平台能力。
- loader/scheduler 负责触发，不负责承载复杂业务。
- runtime 负责在受控环境中运行代码。
- input/output schema 成为服务组合、外部调用和 AI 生成代码的基础。

## 6. 新方案的概念模型

```text
Project Manifest
  |
  +-- Runtime Constraints
  |     +-- driver
  |     +-- image
  |     +-- workspace
  |     +-- env / secret
  |     +-- network / capability
  |
  +-- Agent Profiles
  |     +-- provider
  |     +-- model
  |     +-- system prompt
  |
  +-- Business Logic Entries
  |     +-- description
  |     +-- entry file reference
  |     +-- input schema
  |     +-- output schema
  |     +-- timeout / retry / permissions
  |
  +-- Triggers
        +-- manual / api / cron / event / webhook
        +-- target business logic entry
        +-- static or mapped input
```

注意：Business Logic Entry 不表示 agent-compose 内置业务代码，而表示 agent-compose 能识别、校验、运行和管理的业务代码入口。

## 7. Manifest 设计方向

未来 manifest 应具备以下特征：

- 严格 schema。
- 支持外部文件引用。
- 支持 bundle 导入。
- 支持版本和 revision。
- 支持 dry-run validation。
- 支持导入后生成受管资源。
- 支持回滚和差异展示。

示意结构：

```yaml
apiVersion: agent-compose/v1alpha1
kind: Project

metadata:
  name: review-project
  version: 0.1.0

runtime:
  driver: docker
  image: ghcr.io/org/agent-compose-guest:latest

workspace:
  provider: git
  url: https://github.com/org/repo.git
  branch: main

agents:
  reviewer:
    provider: codex
    model: gpt-5
    systemPrompt: prompts/reviewer.md

services:
  riskReview:
    description: Review workspace risk and return structured result.
    runtime: node
    entry: services/risk-review.js
    inputSchema: schemas/risk-review.input.json
    outputSchema: schemas/risk-review.output.json
    timeout: 10m
    permissions:
      agents:
        - reviewer
      capabilities:
        - repo.read
        - llm.generate

triggers:
  dailyRiskReview:
    type: cron
    cron: "0 9 * * *"
    target: riskReview
    input:
      scope: daily
```

以上只是方向示例，不是当前已实现接口。

## 8. 对外接口设计方向

### 8.1 Manifest 接口

需要支持两类入口：

- CLI 导入：读取本地 manifest/bundle，调用 daemon。
- API 导入：外部系统直接提交符合 schema 的 project manifest。

建议能力：

- `ValidateProjectManifest`
- `ApplyProjectManifest`
- `GetProjectManifest`
- `DiffProjectManifest`
- `RemoveProject`
- `ListProjectRevisions`
- `RollbackProjectRevision`

### 8.2 运行接口

当前已有 `RunAgent`。未来更科学的对外调用应增加业务逻辑入口调用概念：

- `InvokeService`
- `InvokeServiceStream`
- `GetRun`
- `ListRuns`
- `StopRun`

调用输入应是 JSON object，并按 input schema 校验。

返回结果应包含标准 envelope：

```json
{
  "runId": "...",
  "status": "succeeded",
  "output": {},
  "error": null,
  "artifacts": [],
  "logs": [],
  "metrics": {},
  "startedAt": "...",
  "completedAt": "..."
}
```

### 8.3 SDK 接口

JS SDK 应稳定提供业务代码所需能力：

- `runtime.agent(...)`
- `runtime.llm(...)`
- `runtime.exec(...)`
- `runtime.shell(...)`
- `runtime.state.get/set/delete(...)`
- `runtime.event.publish(...)`
- `runtime.artifact.write/read/list(...)`
- `runtime.log(...)`
- `runtime.secret.get(...)`
- `runtime.capability.call(...)`
- `runtime.context`
- `runtime.invokeService(...)`

其中部分能力当前已有，部分属于后续方向。

## 9. Input Schema 和 Output Schema

### 9.1 当前状态

当前 agent 运行主要输入是 prompt/message 字符串。v2 `RunAgentRequest` 支持 `output_schema_json`，loader 和 SDK 也支持部分 `outputSchema` 能力。

缺口：

- agent definition 没有固定 input schema。
- agent definition 没有固定 output schema。
- output schema 多数是调用时传入，不是业务能力本身声明。
- 缺少统一错误 schema 和结果 envelope。

### 9.2 新方案要求

业务逻辑入口应具备：

- `inputSchema`：描述调用方必须提供什么参数。
- `outputSchema`：描述成功输出的结构。
- `errorSchema`：描述失败时的结构化错误，可选但建议支持。
- `examples`：用于 UI、测试和 AI 生成代码。

这样才能支持：

- 外部 API 安全调用。
- UI 自动生成表单。
- AI 自动生成调用代码。
- 服务串联和并联。
- 静态校验和 dry-run。
- 审计和可观测。

## 10. Loader JS 的定位

Loader JS 应保留，但定位要明确。

### 10.1 推荐路径：YAML trigger

常见场景应使用 YAML：

```yaml
triggers:
  hourly:
    type: interval
    every: 1h
    target: riskReview
    input:
      scope: hourly
```

daemon 可以根据 YAML 生成或维护内部 loader/trigger。

### 10.2 高级路径：Loader JS

复杂场景使用 loader JS：

- 多个事件路由到不同 target。
- 根据状态决定是否运行。
- 对事件 payload 做轻量转换。
- 调用多个业务入口并合并结果。
- 发布新事件。

但不建议 loader JS 承载复杂业务算法、npm 依赖和长时间任务。这些应交给 runtime 中的业务逻辑代码。

## 11. Daemon 和 Runtime 职责

### 11.1 Daemon

daemon 是控制面，应负责：

- manifest 接收和校验。
- project revision 管理。
- 受管资源生成和 reconcile。
- workspace、image、session、runtime 生命周期。
- scheduler 和 event loop。
- run 状态、日志、artifact、event 持久化。
- 权限、secret、capability 注入。
- 对外 API。

daemon 不应负责实现具体业务逻辑。

### 11.2 Runtime

runtime 是执行面，应负责：

- 在指定 driver 和 image 中运行代码。
- 挂载 workspace、state、runtime、logs。
- 注入 env、secret、LLM facade token、capability 配置。
- 执行业务逻辑入口、agent provider 或命令。
- 返回结构化结果、日志和 artifact。
- 支持取消、超时和 cleanup。

## 12. 新方案相对当前方案的优势

| 维度 | 当前方案 | 新定位方案 | 优势 |
| --- | --- | --- | --- |
| 用户心智 | agent、scheduler、runtime、业务逻辑混合 | project、agent profile、business entry、trigger 分层 | 更容易理解和对外说明 |
| 业务复用 | 业务逻辑常混在 prompt 或 loader JS 中 | 业务逻辑入口独立，带 schema | 可复用、可测试、可组合 |
| 触发方式 | trigger 与 agent prompt 绑定较紧 | trigger 调用业务入口 | 一份业务逻辑可被多种方式触发 |
| Schema | 有 proto 和部分 output schema | manifest、input、output、error schema 明确 | 支持治理、UI、AI 生成和串并联 |
| SDK | 初步封装 agent/llm/exec/shell | 明确作为业务代码平台能力面 | 降低业务代码开发成本 |
| Loader JS | 容易承载复杂业务 | 作为高级触发编排 escape hatch | 降低复杂度和误用风险 |
| Daemon 职责 | 控制面能力已有，但概念偏混合 | 明确控制面，不实现业务 | 边界清晰，长期可维护 |
| Runtime 职责 | 运行 agent provider 和命令 | 运行标准业务入口和 provider | 执行模型更统一 |
| 外部集成 | 主要围绕 agent run | 围绕 manifest 和 service invocation | 更适合平台化和生态化 |
| 治理审计 | 可记录 run/session，但业务语义弱 | 输入、输出、权限、artifact 标准化 | 更适合企业场景 |

## 13. 两种方案的全面对比

### 13.1 当前方案适合的场景

当前方案适合：

- 快速启动 agent session。
- 使用 Codex/Claude/Gemini/OpenCode 做 workspace 任务。
- 简单定时或事件触发 agent prompt。
- 使用 loader JS 做轻量自动化。
- 内部研发和探索型使用。

当前方案优势是实现成本低，已有代码可用，概念数量少，能快速验证 agent-compose 的基础价值。

### 13.2 当前方案的长期风险

长期风险包括：

- 业务逻辑散落在 prompt、scheduler JS、shell 脚本和 guest SDK 调用中。
- 难以做强 schema 校验。
- 难以做服务间组合。
- 难以让 AI 稳定生成和维护业务代码。
- 难以给产品经理和外部用户解释清楚“agent 到底是什么”。
- loader JS 可能演变成不可治理的脚本系统。
- API 对外表达仍偏 agent/session，而不是业务能力。

### 13.3 新定位方案适合的场景

新定位方案适合：

- 多团队共享 agent-compose 平台能力。
- 外部用户导入 project manifest。
- 业务能力需要长期维护、审计和复用。
- 业务能力需要 UI 表单、API 调用、串联、并联、权限审核。
- 需要让 AI 根据 SDK 和 schema 生成可运行代码。
- 需要把 agent-compose 从工具项目提升为平台产品。

### 13.4 新定位方案的成本

新方案也有成本：

- 需要新增或稳定 manifest schema。
- 需要强化 JS SDK。
- 需要明确业务逻辑入口运行协议。
- 需要补 input/output/error schema 校验。
- 需要考虑 bundle 导入、版本、回滚和兼容。
- 需要对现有 agent-centric API 做兼容迁移。

因此，新方案不应一次性推翻现有实现。更合理的路径是渐进式演进。

## 14. 建议演进路径

### 阶段一：概念收敛和文档统一

- 明确 daemon、runtime、agent、loader、SDK、business logic entry 的定义。
- 在文档中明确 loader JS 不推荐承载复杂业务。
- 把 JS SDK 定义为业务逻辑代码的标准平台能力面。
- 梳理当前 API 和 manifest 的职责边界。

### 阶段二：Schema 和 Manifest 强化

- 为 compose/manifest 提供严格 schema。
- 增加 input/output schema 的 manifest 表达。
- 支持 schema 文件引用。
- 支持 manifest validation 和 dry-run。

### 阶段三：业务逻辑入口标准化

- 定义业务逻辑入口的运行协议。
- 支持 entry 文件引用和 bundle 导入。
- 支持标准 result envelope。
- 支持 SDK mock 和本地测试。

### 阶段四：对外 API 产品化

- 增加 service invocation API。
- 支持 service run、run logs、artifacts、metrics。
- 支持 manifest revision、diff、rollback。
- 支持 UI 展示 input/output schema 和运行历史。

## 15. 结论

agent-compose 当前已经具备 agent/session 控制面的基础能力，但如果要面向更多外部用户提供稳定平台能力，需要从“agent runner + scheduler”升级为“manifest 驱动的 agent/service 运行平台”。

这个升级的关键不是让 agent-compose 编写业务逻辑，而是让 agent-compose 提供支撑业务逻辑的基础设施：

- 严格 manifest。
- 清晰概念边界。
- 标准 runtime 执行协议。
- 足够完整的 JS SDK。
- input/output schema。
- 触发与业务逻辑解耦。
- daemon 控制面和 runtime 执行面的明确分工。

从产品角度，新方案更容易对外解释、售卖和生态化。从研发角度，新方案更容易维护、测试、组合和治理。从管理层角度，新方案能把 agent-compose 从内部工具推进为具备平台价值的基础产品。

