# AI-Driven Integration Testing for Game Servers — Demo

这是论文 *AI-Driven Integration Testing for Game Servers via Deterministic State Machine Breakpoints* 的**可运行原型**，对应根目录论文主文档 [`../README.md`](../README.md) 中的实验载体。

当前原型已经具备：

- 六模块事件驱动游戏服务端
- `next` / `batch` 执行控制
- 代码理解工具（`read_file` / `search_code` / `update_knowledge`）
- L2 基线场景
- `register_cmd` 最小 MVP
- L0 / L1 场景脚手架

但要特别注意：

> 这并不意味着正式实验已经完成。当前环境仍缺少 `API_KEY`，因此真实的 L0 / L1 / L2 对比结果还没有跑出来。

---

## 1. 当前原型包含什么

### 业务模块

- Bag
- Task
- Achievement
- Equipment
- SignIn
- Mail

### 支撑模块

- Event Bus
- Breakpoint Controller
- WebSocket Server
- AI Agent loop

---

## 2. 已实现能力

### 运行时命令

- `playermgr`
- `additem`
- `removeitem`
- `checkin`
- `claimreward`
- `equip`
- `unequip`
- `claimmail`
- `next`
- `batch`
- `help`

### 新增的接口设计能力

- `register_cmd`
- `listcmd`

`register_cmd` 当前是一个**白名单式最小 MVP**：

它允许 Agent 注册具名测试命令，并把它们绑定到少量允许的原始业务动作上，例如：

- `bag.AddItem`
- `bag.RemoveItem`
- `signin.CheckIn`
- `signin.ClaimReward`
- `equipment.Equip`
- `equipment.Unequip`
- `mail.ClaimAttachment`

这样做的目的是让 Agent 能在**不改业务代码**的前提下，补出验证所需的测试入口。

---

## 3. 当前场景划分

### 运行时 L2 基线

```bash
make test-dual API_KEY=xxx
make test-step-only API_KEY=xxx
make test-code-batch API_KEY=xxx
make test-batch-only API_KEY=xxx
```

### 分析基线

```bash
make test-code-only API_KEY=xxx
```

### L0 / L1 脚手架

```bash
make test-l0 API_KEY=xxx
make test-l1 API_KEY=xxx
```

说明：

- `test-l0` / `test-l1` 现在表示入口和运行模式已接入
- 但正式结果仍依赖真实 LLM 运行
- 当前环境若没有 `API_KEY`，这些场景无法产出真实实验数据

---

## 4. 快速开始

### 构建

```bash
make build
```

### 手动运行

```bash
./bin/server -host 127.0.0.1 -port 5400
```

WebSocket:

- `ws://127.0.0.1:5400/ws`

安全说明：

- 当前 demo 默认面向本地可信环境
- 默认绑定到 `127.0.0.1`
- WebSocket 升级仅接受 `localhost` / `127.0.0.1` 来源

---

## 5. 预埋缺陷

| Bug | 描述 |
|---|---|
| B1 | `RemoveItem` 缺少负数校验 |
| B2 | 任务进度 / `task.completed` 逻辑异常 |
| B3 | `collector_100` 计数对象错误 |
| B4 | 签到奖励重复领取 |
| B5 | 装备不消耗背包物品 |
| B6 | `mail.claimed` 事件无人消费 |
| B7 | 第 7 天奖励 ID 冲突 |

其中最关键的验证点是 **B1**，因为它正对应论文里的 **interface gap**：

Agent 可能已经在理解阶段怀疑 `RemoveItem` 的负数校验有问题，但只有当它真的能够设计并注册一个专门用于验证该异常的命令时，这个怀疑才有机会转化为缺陷证据。

---

## 6. 目录结构

```text
ai-integration-test-demo/
├── cmd/server/main.go
├── internal/
│   ├── server/server.go
│   ├── breakpoint/controller.go
│   ├── player/manager.go
│   ├── bag/
│   ├── task/
│   ├── achievement/
│   ├── equipment/
│   ├── signin/
│   ├── mail/
│   └── event/
├── ai/
│   ├── agent/
│   ├── tools/
│   ├── prompt/
│   ├── knowledge/
│   ├── codeanalyzer/
│   └── session/
├── results/
├── scripts/
└── Makefile
```

---

## 7. 相关文档

- 论文主文档：[`../README.md`](../README.md)
- 当前运行说明：[`../QUICKSTART.md`](../QUICKSTART.md)
- 正式实验状态：[`./results/FORMAL_EXPERIMENT_STATUS.md`](./results/FORMAL_EXPERIMENT_STATUS.md)
