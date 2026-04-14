# BST-Agent Demo

基于 [Integration Testing Guide](../README.md) 的断点步进自主集成测试原型，用 Go + WebSocket + LLM 实现。

## 架构

```
┌──────────────────────────────────────────────────────┐
│ 启动时：代码分析预处理（Go AST，毫秒级）              │
│                                                      │
│  codeanalyzer → Code Summary (结构体/函数/事件流映射) │
│         │                                            │
│         ▼ 注入 System Prompt                         │
│  ┌─────────────────────────────────────────────────┐ │
│  │        BST-Agent (LLM)                          │ │
│  │                                                 │ │
│  │  System Prompt = 角色定义 + 协议 + Code Summary  │ │
│  │              [+ 需求文档 (Level 1+)]             │ │
│  │              [+ 人工规则 (Level 2)]              │ │
│  │                    │                             │ │
│  │              ┌─────▼──────┐                      │ │
│  │              │ send_command│ ← 唯一运行时工具     │ │
│  │              │ (Query/    │                      │ │
│  │              │  Inject/   │                      │ │
│  │              │  Step)     │                      │ │
│  │              └─────┬──────┘                      │ │
│  └────────────────────┼────────────────────────────┘ │
└───────────────────────┼──────────────────────────────┘
                        │ WebSocket+JSON
                  ┌─────▼──────────┐
                  │  Game Server   │
                  │  (6 modules)   │
                  │  Event Bus     │
                    │  Event Bus     │
                    │  Breakpoint    │
                    └────────────────┘
```

### 模块间关联（事件总线解耦）

```
additem(2001) → Bag → Publish("item.added")
  ├── Task.onItemAdded() → Progress(3001, 1) → Publish("task.completed")
  │     └── Achievement.onTaskCompleted() → Unlock(4001) → Publish("achievement.unlocked")
  │           └── Mail.onAchievementUnlocked() → SendMail()
  ├── Achievement.onItemAdded() → 检查 collector_100 条件
  └── Equipment.onItemAdded() → 检查是否可装备 → Publish("equip.success")
        └── Achievement.onEquipSuccess() → 检查 fully_equipped 条件

checkin(day=1) → SignIn → Publish("signin.claimed")
  └── Mail.onSignInClaimed() → SendRewardMail()
```

## 快速开始

```bash
# 构建
make build

# 自主发现测试（双通道，Level 0 Zero Prompt）
make test-discovery API_KEY=xxx MODEL=glm-5.1 BASE_URL=https://open.bigmodel.cn/api/paas/v4

# 消融实验：仅代码通道
make test-code-only API_KEY=xxx MODEL=glm-5.1 BASE_URL=https://open.bigmodel.cn/api/paas/v4

# 消融实验：仅日志通道
make test-log-only API_KEY=xxx MODEL=glm-5.1 BASE_URL=https://open.bigmodel.cn/api/paas/v4

# 手动交互模式
./bin/server -port 5400
# 另一个终端: wscat -c ws://127.0.0.1:5400/ws
```

## 六模块系统

| 模块 | 功能 | 发布事件 | 订阅事件 |
|------|------|----------|----------|
| Bag | AddItem / RemoveItem | `item.added`, `item.removed` | — |
| Task | Progress / Complete | `task.completed` | `item.added` |
| Achievement | Unlock（幂等） | `achievement.unlocked` | `task.completed`, `item.added`, `equip.success` |
| Equipment | Equip / Unequip | `equip.success`, `equip.unequipped` | `item.added` |
| SignIn | CheckIn / ClaimReward | `signin.claimed` | — |
| Mail | SendMail / ClaimAttachment | `mail.claimed` | `signin.claimed`, `achievement.unlocked` |

### 预埋缺陷

| Bug | 位置 | 描述 | 严重度 |
|-----|------|------|--------|
| #1 | `task.go` | Progress 增量硬编码为 1 | Medium |
| #2 | `bag.go` | RemoveItem 缺少 count≤0 校验 | Critical |
| #3 | `signin.go` | ClaimReward 可重复领取 | High |

## 协议

```bash
# 查询
> {"cmd": "playermgr", "playerId": 10001, "sub": "bag"}
> {"cmd": "playermgr", "playerId": 10001, "sub": "task"}
> {"cmd": "playermgr", "playerId": 10001, "sub": "achievement"}
> {"cmd": "playermgr", "playerId": 10001, "sub": "equipment"}
> {"cmd": "playermgr", "playerId": 10001, "sub": "signin"}
> {"cmd": "playermgr", "playerId": 10001, "sub": "mail"}

# 操作（入队列）
> {"cmd": "additem", "playerId": 10001, "itemId": 2001, "count": 5}
> {"cmd": "removeitem", "playerId": 10001, "itemId": 2001, "count": 3}
> {"cmd": "checkin", "playerId": 10001, "day": 1}
> {"cmd": "claimreward", "playerId": 10001, "day": 1}
> {"cmd": "equip", "playerId": 10001, "slot": "weapon", "itemId": 3001}
> {"cmd": "unequip", "playerId": 10001, "slot": "weapon"}
> {"cmd": "claimmail", "playerId": 10001, "mailId": 1}

# 单步执行
> {"cmd": "next"}
< {"ok": true, "log": ["[Bag] add item 2001 x5", "[Task] trigger 3001 progress+1 ..."]}
```

## 项目结构

```
├── cmd/server/main.go              # 入口：server / test 模式
├── internal/
│   ├── server/server.go            # WebSocket 服务器 + 消息路由
│   ├── breakpoint/controller.go    # 断点控制器（next 推进）
│   ├── player/manager.go           # 玩家管理器（6 模块组合）
│   ├── bag/bag.go                  # 背包模块
│   ├── task/task.go                # 任务模块（订阅 item.added）
│   ├── achievement/achievement.go  # 成就模块（订阅 task.completed + equip.success）
│   ├── equipment/equipment.go      # 装备模块（订阅 item.added）
│   ├── signin/signin.go            # 签到模块
│   ├── mail/mail.go                # 邮件模块（订阅 signin.claimed + achievement.unlocked）
│   └── event/bus.go               # 事件总线
├── ai/
│   ├── agent/agent.go              # Agent 循环（双通道调度）
│   ├── tools/tools.go              # 工具定义（代码通道 + 运行时通道）
│   ├── prompt/system.go            # 系统提示词（双通道 / 仅代码 / 仅日志，支持多等级 Prompt）
│   └── session/session.go          # WebSocket 会话封装
├── scripts/
│   └── summarize_results.py        # 多次运行结果汇总
├── Makefile
└── README.md
```
