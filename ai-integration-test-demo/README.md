# AI-Driven Integration Testing Demo

基于 [Integration Testing Guide](../README.md) 架构模式的项目示例，用 Go + WebSocket + OpenAI 实现游戏服务器的 AI 驱动集成测试。

## 架构

```
┌──────────────┐   WebSocket+JSON   ┌──────────────────┐
│   AI Agent   │ ◄───────────────► │   Game Server    │
│  (OpenAI)    │                   │   (Go)           │
│              │  cmd / next / log │                  │
└──────────────┘                   └──────────────────┘
```

### 核心概念

| 概念 | 实现 |
|------|------|
| **看数据** | `playermgr` 命令查询背包/任务/成就状态 |
| **断点** | `next` 命令单步执行队列中的操作，返回增量日志 |
| **看日志** | 每次 `next` 返回 `log` 数组，避免日志瀑布 |
| **跨模块关联** | 事件总线驱动：物品添加 → 任务进度 → 成就解锁 |
| **AI 自主推理** | OpenAI Function Calling，AI 决定查什么、推几步 |

### 模块间关联

```
添加物品 (2001) ──→ 任务进度 (3001) ──→ 成就解锁 (4001: first_task)
添加物品 (2002) ──→ 任务进度 (3002) ──→ 成就解锁 (4002: task_master)
2+ 成就解锁 ──────────────────────────→ 成就解锁 (4003: collector_100)
```

## 快速开始

```bash
# 构建
make build

# 启动服务器（手动模式，可用 wscat 等工具连接）
make run

# AI 驱动测试
export OPENAI_API_KEY=sk-xxx
make test-basic      # 基础场景
make test-cross      # 跨模块关联场景
make test-edge       # 边界测试场景
```

## 协议

通过 WebSocket 连接 `ws://127.0.0.1:5400/ws`，发送 JSON 请求：

```bash
# 查看背包
> {"cmd": "playermgr", "playerId": 10001, "sub": "bag"}
< {"ok": true, "data": []}

# 添加物品（入队列，不立即执行）
> {"cmd": "additem", "playerId": 10001, "itemId": 2001, "count": 5}
< {"ok": true, "data": {"queued": true, "pendingOps": 1}}

# 单步执行，返回增量日志
> {"cmd": "next"}
< {"ok": true, "log": ["[Bag] add item 2001 x5", "[Task] trigger 3001 progress+1 (now 1/1)", "[Task] task 3001 completed", "[Achievement] unlocked: first_task (id=4001)"]}

# 查看任务状态
> {"cmd": "playermgr", "playerId": 10001, "sub": "task", "taskId": 3001}
< {"ok": true, "data": {"taskId": 3001, "target": 1, "progress": 1, "state": "completed"}}
```

## 项目结构

```
├── cmd/server/main.go          # 入口：server 模式 / test 模式
├── internal/
│   ├── server/server.go        # WebSocket 服务器 + 消息路由
│   ├── breakpoint/controller.go # 断点控制器（next 推进）
│   ├── player/manager.go       # 玩家管理器
│   ├── bag/bag.go              # 背包系统
│   ├── task/task.go            # 任务系统（订阅 item.added）
│   ├── achievement/achievement.go # 成就系统（订阅 task.completed）
│   └── event/bus.go            # 事件总线
├── ai/
│   ├── agent/agent.go          # AI Agent（OpenAI Function Calling 循环）
│   ├── tools/tools.go          # AI 可用工具定义
│   ├── prompt/system.go        # System Prompt
│   └── session/session.go      # WebSocket 会话封装
├── Makefile
└── README.md
```

## 测试场景

### basic — 基础流程验证
AI 查看初始状态 → 添加物品 → 单步执行 → 验证跨模块触发

### cross-module — 跨模块关联
验证完整的 物品→任务→成就 触发链，包括 collector_100 的条件解锁

### edge-case — 边界测试
AI 自主测试 count=0、负数、移除不存在的物品等边界情况

## 设计说明

### 断点控制器
操作入队列后不立即执行，AI 通过 `next` 命令逐步推进。这等价于传统调试中的"单步执行"，让 AI 可以在每一步之后观察日志和状态变化。

### 事件驱动
所有模块间通信通过事件总线完成，不直接调用。背包添加物品发布 `item.added` 事件，任务系统订阅该事件并推进进度，完成后发布 `task.completed` 事件，成就系统订阅并解锁。

### Go 并发模型
按照文章建议，围绕"状态归属 + 消息流"设计，goroutine 仅作为运行时载体。断点控制器使用 channel 缓冲操作队列，`Next()` 从 channel 中取出一条操作执行。
