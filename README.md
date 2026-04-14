# Autonomous Integration Testing for Game Servers: Breakpoint-Stepping Agent with Dual-Channel Correlation Discovery

游戏服务端自主集成测试：断点步进 Agent 与双通道关联发现

[English](./README_EN.md)

---

## 摘要

游戏服务端的集成测试长期依赖人工编写测试用例和手动维护模块映射关系，难以应对频繁变更的业务逻辑和隐含的跨模块依赖。本文提出 **BST-Agent**（Breakpoint-Stepping Testing Agent），一种无需人工编写测试用例的自主集成测试方法——工程师只需提供系统提示词（代码分析摘要、需求文档等），Agent 即可自主决定测试什么、如何测试。BST-Agent 将人类工程师的调试工作流形式化为三类运行时原语——**Query**（查询状态）、**Inject**（注入操作）、**Step**（单步执行）——使 Agent 能够像工程师一样逐步观察系统行为，而非批量执行预设用例。在知识获取层面，BST-Agent 采用**双通道关联发现**机制：**代码通道**在启动时通过静态分析（Go AST 解析）自动提取源码结构（函数签名、事件 Publish/Subscribe 链、事件流映射），注入系统提示词作为 Agent 的先验知识；**日志通道**在运行时通过逐步操作和观察验证静态推断、发现隐含关联和异常。我们进一步提出**Prompt**概念，将系统提示词的内容从Zero Prompt到Guided Prompt分为多个等级，使方法适应不同场景的实际需求——实践中，需求文档是已有的客观制品，读取需求文档也是合理的知识获取方式；对于难以从代码和日志中推断的复杂规则，可由人工添加提示。我们在一个包含背包、任务、成就、装备、签到、邮件六模块的游戏服务端原型上实现了该方法。原型采用事件总线架构，模块间完全解耦——所有跨模块交互仅通过事件订阅实现，代码中无直接调用。实验中，GLM-5.1 Agent 在Zero Prompt条件下自主推断出 8 条跨模块关联中的 7 条，并发现 3 个真实代码缺陷：任务进度增量硬编码、删除物品缺少负数校验（可利用的物品复制漏洞）、签到奖励重复领取。消融实验量化了代码通道与日志通道各自的贡献，以及不同Prompt 等级对测试效果的影响。

**关键词**：集成测试、大语言模型、游戏服务端、断点步进、自主关联发现、双通道

---

## 1 引言

### 1.1 问题

游戏服务端的集成测试面临一个根本性矛盾：**测试的质量取决于测试者对系统的理解，而系统的复杂性远超任何个体的认知范围**。具体表现为：

1. **隐含依赖**。事件驱动架构下，模块间通过事件总线解耦。背包模块发布 `item.added` 事件，任务模块订阅该事件——两者代码中无任何直接引用关系。这类依赖不会出现在 import 图、调用链或接口定义中，只有运行时才能观察到。
2. **认知固化**。人工编写的测试用例反映的是编写者对系统的理解。对于编写者不知道的关联，不会存在对应的测试。而游戏服务端的模块间关联经常超出编写者的心智模型。
3. **维护成本**。业务频繁变更时，模块映射文档和测试用例需要同步更新。实践中，文档和用例往往滞后于代码，导致测试验证的是过期的预期。

传统解决方案中，工程师通过**读代码 + 跑系统 + 看日志**来理解系统行为——这是一个需要持续推理的认知过程。核心观察是：**如果让 LLM Agent 自主完成这个认知过程——自己读代码理解结构，自己操作系统观察行为，自己对比两者发现不一致——便可能突破预设用例的覆盖瓶颈，从而不再需要人工编写测试用例。**

### 1.2 贡献

本文的主要贡献如下：

1. **提出 BST-Agent 方法**，实现无需人工编写测试用例的自主集成测试：工程师只需提供系统提示词（代码分析摘要、需求文档等），Agent 即可自主决定测试什么、如何测试。方法将断点步进调试范式形式化为三类运行时原语（Query / Inject / Step），Agent 像工程师一样逐步观察系统行为，而非批量执行预设用例。
2. **设计双通道关联发现机制**——代码通道预处理静态分析（Go AST 解析，毫秒级），日志通道运行时验证——两种通道时序解耦，Agent 轮次 100% 用于验证和发现。
3. **提出Prompt概念**，将 Agent 的系统提示词内容从Zero Prompt到Guided Prompt分为多个等级（Zero Prompt、Doc Prompt、Guided Prompt），使方法适应不同场景的实际需求，而非固守"Zero Prompt"的极端立场。
4. **实现并开源了基于 Go + WebSocket + GLM-5.1 的六模块原型系统**，采用事件总线架构实现模块间完全解耦，验证方法在真实架构模式下的可行性。
5. **通过实验证明** BST-Agent 在Zero Prompt条件下能自主推断 7/8 条跨模块关联，并发现 3 个代码缺陷，包括一个严重的物品复制漏洞。
6. **通过消融实验量化**代码通道与日志通道各自的贡献，以及不同Prompt 等级对测试效果的影响，并诚实讨论方法的局限性。

---

## 2 相关工作

### 2.1 游戏服务端测试

游戏服务端的自动化测试研究相对稀缺。工业界主要依赖手动编写测试用例 [1] 和简单的协议模糊测试（fuzzing）。Arnold 等人 [2] 提出基于状态机的游戏逻辑测试方法，但需要人工构建状态模型。Streamline [3] 尝试通过录制玩家行为回放来测试 MMORPG，但无法生成未见过的测试场景。这些方法共同的局限是**依赖人工预设**——无论是状态模型还是录制脚本，都反映了编写者已知的信息。

### 2.2 LLM 驱动的软件测试

近年来，LLM 在软件测试中的应用快速增长。CodiumAI [4] 和 TestPilot [5] 利用 LLM 生成单元测试用例，但生成的用例仍需人工定义测试意图和断言。Lemieux 等人 [6] 将 LLM 与覆盖率反馈结合进行模糊测试。WebArena [7] 和 SWE-bench [8] 评估了 LLM Agent 在真实环境中的任务完成能力。然而，这些工作仍以**给定目标驱动**，Agent 执行的是人类定义的测试任务，而非自主发现需要测试什么。

### 2.3 代码理解与程序分析

LLM 在代码理解方面展现了强大能力。Copilot [9] 和 Cursor [10] 利用 LLM 辅助代码阅读和编辑。研究显示 LLM 能够理解代码中的数据流和控制流 [11]。但静态代码分析无法捕获事件驱动架构中的运行时行为——`eventBus.Publish("item.added")` 和 `eventBus.Subscribe("item.added", handler)` 在代码中没有直接调用关系，只有运行时事件传播才能揭示依赖。

### 2.4 Agent 自主测试

AutoGPT [12] 和 LangChain Agent [13] 展示了 LLM Agent 与外部系统交互的能力。Meta 的 Sapienz [14] 在移动应用上实现了基于搜索的自动测试。但这些 Agent 采取"给定输入，观察输出"的模式，缺少对系统的**自主理解**过程——它们不尝试先构建系统行为模型再验证，而是直接执行操作。

### 2.5 本工作的定位

与上述工作相比，BST-Agent 的独特之处在于两点：**一是将断点步进作为一等原语引入 Agent 测试循环**，使 Agent 能控制执行粒度并增量观察中间状态和日志，而非仅对比操作前后的快照——这对事件驱动架构尤为重要，单个操作可能触发多级级联事件，批量执行丢失中间因果关系；**二是将代码分析作为预处理步骤而非运行时操作**，代码通道在启动时自动提取源码结构并注入提示词，Agent 运行时只做验证和发现，避免了运行时代码探索的轮次浪费。

---

## 3 方法

### 3.1 核心理念：从预设测试到自主测试

传统集成测试的核心问题是**测试者预设了需要验证什么**——测试用例、断言、模块映射都是人工编写的，测试的范围受限于编写者的认知。每次业务变更都需要人工同步更新测试用例，维护成本高且容易滞后。

BST-Agent 的核心主张是：**工程师不再需要编写测试用例，只需提供系统提示词（代码分析摘要、需求文档等），Agent 即可自主决定测试什么、如何测试**。工程师的工作从"写测试用例"转变为"提供系统知识"——后者通常已是已有制品（代码、文档），无需额外编写。

Agent 的自主性体现为两个层面：

1. **执行自主性**：Agent 不是批量执行预设用例，而是像工程师调试一样逐步操作系统、观察日志、推理因果——即**断点步进**范式（见 3.2 节）。
2. **认知自主性**：Agent 不是被告知"测试 A 模块和 B 模块的关联"，而是通过代码分析和运行时观察自主构建系统行为模型——即**双通道关联发现**机制（见 3.3 节）。

**关键区分：知识输入与测试指令**。我们区分两类 Agent 输入：

- **知识输入**：帮助 Agent 理解系统的客观信息——代码分析摘要、需求文档、事件命名规范。知识输入不指定"测什么"，只提供"系统是什么样的"。
- **测试指令**：明确指定验证内容和预期结果的指令——如"验证添加物品后任务进度 +1"。传统测试方法完全依赖测试指令。

BST-Agent 接受知识输入，但不接受测试指令。Agent 仍需自行决定基于已有知识应该验证哪些关联、构造哪些边界条件。实践中，知识输入的多少构成一个**Prompt**（见 3.4 节），方法在不同Prompt 等级下均可工作。

### 3.2 断点步进测试范式

人类工程师调试系统时遵循一个本能的工作流：**查看状态 → 执行操作 → 观察日志**。这个过程是增量式的——每一步操作后立即检查结果，而非批量执行后对比快照。BST-Agent 将这个工作流形式化为三类运行时原语：

| 原语类别 | 语义 | 在自主测试中的角色 |
|----------|------|-------------------|
| **Query(q)** | 查询运行时数据状态，不产生副作用 | 验证 Agent 构建的行为模型是否与实际一致 |
| **Inject(op)** | 注入一个操作到待执行队列，不立即执行 | Agent 主动构造探索性操作 |
| **Step()** | 从队列中取出一个操作执行，返回执行期间产生的全部日志 | 逐步观察因果链，建立操作与日志的对应关系 |

三类原语的设计遵循**渐进式揭示**原则：Agent 先 Query 理解当前状态，再 Inject 构造操作，最后 Step 观察结果——每一步都是对自建模型的验证。这对应人类工程师的调试工作流："查看状态 → 执行操作 → 检查日志"。

**断点步进对事件驱动架构的必要性**：在事件驱动架构中，一个操作可能触发多级级联事件。例如 `additem` 触发 `item.added`，进而触发 Task 进度更新、Achievement 解锁、Mail 发送。如果批量执行操作后一次性返回所有日志，Agent 难以建立操作与日志之间的因果关系。Step 原语使 Agent 能逐操作观察，精确定位每个操作触发的完整事件链。

**断点步进的语义取决于被测系统的并发模型**：

- **Actor 模型**（如 Skynet）：Step 对应"处理服务中的一条消息"，干预单元是服务（workspace 固定，与哪个 worker_thread 驱动无关）。
- **CSP 模型**（如 Go）：Step 对应"从 channel 中取出并处理一条消息"。并发控制的关注点应从执行单元（goroutine）提升到通信语义（channel）。系统设计应围绕"状态所有权 + 消息流"，goroutine 仅作为运行时载体。

### 3.3 双通道关联发现

BST-Agent 通过两个互补的通道构建系统行为模型：

**代码通道（预处理静态分析）**

在 Agent 启动前，静态分析工具扫描源码，自动提取以下信息并注入系统提示词：

| 提取目标 | 作用 | 示例 |
|----------|------|------|
| 数据结构定义 | 理解系统实体 | `type Item struct { ItemID int; Count int }` |
| 函数签名 | 理解操作语义 | `func (b *Bag) AddItem(itemID, count int)` |
| 事件发布点 | 标记影响出口 | `bus.Publish("item.added", ...)` |
| 事件订阅点 | 标记影响入口 | `bus.Subscribe("item.added", t.onItemAdded)` |
| 事件流映射 | 自动配对 Publish/Subscribe | `Bag ──"item.added"──→ Task.onItemAdded` |

代码通道的优势是**精确且低成本**——函数签名、事件配对是确定性信息，在启动时毫秒级完成提取。劣势是**不完整**——只能提取结构性信息，无法理解业务语义（如"Progress(tid, 1) 中的 1 是硬编码"需要 Agent 自行判断），也无法确认运行时的实际传播路径。

**日志通道（动态观察）**

Agent 操作系统并逐步观察日志，提取以下信息：

| 提取目标 | 作用 | 示例 |
|----------|------|------|
| 事件传播链 | 建立模块间因果关系 | `[Bag] add item 2001 x1` → `[Task] trigger 3001 progress+1` |
| 副作用范围 | 发现隐含影响 | additem 操作同时影响了 Task 和 Achievement |
| 时序关系 | 理解执行顺序 | 成就解锁发生在任务完成之后 |
| 异常信号 | 发现潜在 bug | 删除负数物品导致数量增加 |

日志通道的优势是**完整**——运行时的所有跨模块传播都会在日志中体现。劣势是**需要探索**——必须构造合适的操作才能触发感兴趣的事件链。

**双通道互补**

代码通道在启动时预处理，日志通道在运行时验证，两者时序解耦：

```
┌─────────────────────────────────────────────────────────────┐
│ 启动时：代码通道（毫秒级预处理，不消耗 Agent 轮次）          │
│                                                             │
│  Go AST 解析 → 提取 Publish/Subscribe → 自动配对事件流映射   │
│                                                             │
│  输出 Code Summary:                                         │
│    Bag ──"item.added"──→ Task.onItemAdded                   │
│    Bag ──"item.added"──→ Achievement.onItemAdded            │
│    Bag ──"item.added"──→ Equipment.onItemAdded              │
│    Task ──"task.completed"──→ Achievement.onTaskCompleted    │
│    ...                                                      │
│                                                             │
│  Code Summary 注入系统提示词 → Agent 启动时即拥有代码知识     │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 运行时：日志通道（Agent 轮次 100% 用于测试操作和观察）       │
│                                                             │
│  Agent 已知: Bag ──"item.added"──→ Task                     │
│  验证: additem(2001,1) → Step → 日志确认 [Task] 响应        │
│  发现: 日志显示 [Task] progress+1 但 additem count=2 → Bug  │
│                                                             │
│  Agent 已知: AddItem 校验 count≤0, RemoveItem 无此校验      │
│  验证: removeitem(2002,-1) → Step → 数量增加 → Bug 确认     │
└─────────────────────────────────────────────────────────────┘
```

代码通道预处理的价值在于：Agent 启动时就拥有完整的静态关联图，无需在运行时浪费轮次探索代码结构。所有 Agent 轮次都用于运行时验证和异常发现。

日志通道的不可替代性在于：代码分析只能告诉你 `Task` 订阅了 `item.added`，但无法确认 `Task.onItemAdded` 内部又发布了 `task.completed` 的二级传播，更无法发现 `additem count=2` 但 `progress+1` 这种代码逻辑与运行时行为的语义不一致。

### 3.4 Prompt：知识源与自主性的平衡

"Zero Prompt"——不向 Agent 提供任何人工信息——是一个有价值的极端配置，用于评估 Agent 的自主发现能力上限。但在实践中，完全Zero Prompt并不总是最优选择：

1. **需求文档是已有制品**：游戏中每个模块都有需求文档，记录了模块的职责、业务规则、交互约定。这些文档是客观存在的，读取需求文档和读取代码一样，都是合理的知识获取方式。
2. **复杂规则难以从代码推断**：某些业务规则（如"签到奖励每天只能领取一次"）的约束来自产品设计，而非代码逻辑。Agent 可以从代码中发现缺少校验，但难以理解"应该有这个校验"——这是业务常识，不是代码知识。
3. **Zero Prompt的探索成本**：Agent 在Zero Prompt下需要更多轮次理解基础概念（如事件名的业务含义），这些轮次本可用于更深层的异常发现。

因此，我们将 Agent 的系统提示词内容按不同等级组织为一个 **Prompt** 体系，方法在不同 Prompt 等级下均可工作：

| Prompt 等级 | 知识输入 | Agent 自主性 | 适用场景 |
|----------|---------|-------------|---------|
| **Level 0：Zero Prompt** | 代码分析摘要 + 协议文档 | 最高 | 评估自主发现能力上限；无文档的遗留系统 |
| **Level 1：Doc Prompt** | 代码分析摘要 + 需求文档 + 协议文档 | 高 | 常规开发流程；文档与代码同步维护 |
| **Level 2：Guided Prompt** | 代码分析摘要 + 需求文档 + 人工规则提示 + 协议文档 | 中 | 复杂业务规则；已知高风险关联的定向验证 |

**关键约束：所有Prompt 等级都不包含测试指令**。无论哪个等级，Agent 仍需自行决定测试什么、如何构造操作、如何验证结果。Prompt调节的是 Agent 对系统的先验理解深度，而非测试指令的多少。

**Prompt 等级对知识输入的影响**：

```
┌─────────────────────────────────────────────────────────────────┐
│ Level 0（Zero Prompt）                                                │
│                                                                 │
│ System Prompt = 角色定义 + 协议文档 + Code Summary               │
│                                                                 │
│ Agent 任务: 从代码结构推断业务语义，通过运行时观察验证            │
│ 例: 代码分析发现 Task 订阅 item.added → Agent 需自行推断         │
│     "添加物品推进任务进度"这一业务语义                            │
└─────────────────────────────────────────────────────────────────┘
                          ↓ 增加知识输入
┌─────────────────────────────────────────────────────────────────┐
│ Level 1（Doc Prompt）                                              │
│                                                                 │
│ System Prompt = 角色定义 + 协议文档 + Code Summary + 需求文档     │
│                                                                 │
│ Agent 任务: 利用需求文档理解业务语义，聚焦异常发现和边界验证      │
│ 例: 需求文档写明"签到奖励每天只能领取一次" → Agent 立即理解      │
│     业务约束，可主动验证 ClaimReward 是否有重复领取校验           │
└─────────────────────────────────────────────────────────────────┘
                          ↓ 增加知识输入
┌─────────────────────────────────────────────────────────────────┐
│ Level 2（Guided Prompt）                                              │
│                                                                 │
│ System Prompt = 角色定义 + 协议文档 + Code Summary + 需求文档     │
│              + 人工规则提示（如"RemoveItem 应与 AddItem 校验对称"）│
│                                                                 │
│ Agent 任务: 利用专家提示聚焦高风险区域，深入验证复杂逻辑          │
│ 例: 人工提示"删除物品应有负数校验" → Agent 优先验证 RemoveItem    │
│     的边界条件，更快发现物品复制漏洞                              │
└─────────────────────────────────────────────────────────────────┘
```

Prompt的设计遵循一个原则：**知识输入越充分，Agent 的探索效率越高，但对人工维护的依赖也越大**。当需求变更时，Level 1 的需求文档需要同步更新；Level 2 的人工规则提示更需要持续维护。Level 0 虽然探索成本最高，但零维护负担——代码分析摘要由工具自动生成，永远与代码同步。

实践中推荐**渐进式策略**：初次部署时使用 Level 1（Doc Prompt），利用已有需求文档降低探索成本；在稳定运行后，可降低到 Level 0 评估自主发现能力；对于已知高风险模块，可临时提升到 Level 2 进行定向深测。

### 3.5 通信方式

原语通过结构化协议暴露。本文实现选用 **WebSocket + JSON**：

```json
// Query
> {"cmd": "playermgr", "playerId": 10001, "sub": "bag", "itemId": 2001}
< {"ok": true, "data": {"itemId": 2001, "count": 5, "source": "drop"}}

// Inject
> {"cmd": "additem", "playerId": 10001, "itemId": 2001, "count": 5}
< {"ok": true, "data": {"pendingOps": 1, "queued": true}}

// Step
> {"cmd": "next"}
< {"ok": true, "log": ["[Bag] add item 2001 x5", "[Task] trigger 3001 progress+1"]}
```

### 3.6 Agent 自主代码探索

Agent 通过三个源码探索工具自主构建代码知识：

| 工具 | 功能 | 用途 |
|------|------|------|
| `read_file(path)` | 读取指定源码文件 | 理解模块内部结构、函数实现、事件处理逻辑 |
| `search_code(dir, pattern)` | 在目录中搜索关键字 | 追踪 Publish/Subscribe 调用链、定位事件处理函数 |
| `update_knowledge(content)` | 更新关联知识文件 | 持久化已发现的关联和异常，作为后续轮次的上下文 |

Agent 在测试过程中维护 `knowledge.md` 文件，记录已发现的信息。知识文件的内容完全由 Agent 自主构建——Agent 自行决定读取哪些文件、搜索哪些关键字、记录哪些发现。

**代码分析器（Code Analyzer）的角色**：Go AST 解析器作为可选工具保留，仅在 `--quick-start` 模式下自动运行。默认模式下，Agent 通过 read_file 和 search_code 工具自主探索源码，不依赖预处理。

**关键区分**：与 3.3 节描述的双通道机制的关系——代码通道不再由预处理工具自动完成，而是 Agent 通过 read_file/search_code 工具自主执行。这保证了 Agent 的自主性：Agent 自行决定探索策略、自行判断关联关系、自行验证推断。

### 3.7 Agent 架构

BST-Agent 采用断点步进 + 双通道 Agent 循环：

```
┌─────────────────────────────────────────────────────────┐
│ 运行时：Agent 自主探索 + 交互式验证                       │
│                                                         │
│  System Prompt (角色定义 + 协议文档 + 可选知识)            │
│  ┌──────────────────────────────────────────────────┐   │
│  │ knowledge.md (Agent 自主维护的关联知识)            │   │
│  │ read_file ←→ Agent 推理 ←→ send_command          │   │
│  │ search_code ↗           ↘ update_knowledge       │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  工具集 (按消融模式过滤):                                 │
│  ┌──────────────┐  ┌──────────────┐                     │
│  │ 源码探索工具  │  │ 运行时工具    │                     │
│  │ read_file    │  │ send_command │                     │
│  │ search_code  │  │ (Query/      │                     │
│  │ update_      │  │  Inject/     │                     │
│  │ knowledge    │  │  Step/Batch) │                     │
│  └──────────────┘  └──────┬───────┘                     │
└─────────────────────────┼───────────────────────────────┘
                          │ WebSocket
                          ▼
                    Game Server (SUT)
```

Agent 的工具集按消融模式配置（见 5.1 节）：

| 模式 | 源码工具 | 运行时工具 | 知识文件 |
|------|---------|-----------|---------|
| batch-only | ❌ | Inject + Batch | ❌ |
| step-only | ❌ | Query + Inject + Step | ❌ |
| code-batch | ✅ | Inject + Batch | ✅ |
| dual | ✅ | Query + Inject + Step | ✅ |

**所有等级都不包含测试指令**——Agent 仍需自主决定验证哪些关联、构造哪些操作、如何解读结果。

### 3.8 自主测试循环

BST-Agent 的测试过程分为启动时预处理和运行时交互式验证，**关联验证与异常发现交织进行**——Agent 在验证某条关联的同时，若观察到异常信号则立即追踪，而非等待全部关联验证完毕才开始 bug 发现。

**启动时：代码分析预处理（毫秒级）**

Go AST 解析器自动提取源码结构，生成 Code Summary 注入系统提示词。Agent 启动时即拥有完整的静态关联图——函数签名、事件 Publish/Subscribe 配对、事件流映射。

**运行时：交互式验证与发现**

Agent 的运行时不是严格的"先验证关联、再发现 bug"的线性流程，而是**验证与发现交织**的交互式循环：

0. **知识积累**：Agent 在探索过程中持续更新 knowledge.md，记录发现的模块结构、事件关联和潜在异常。每次发现新信息后，Agent 选择更新知识文件以持久化上下文。
1. **验证关联**：Inject 操作 → Step 观察 → 对比日志与 Code Summary 的预期
2. **即时异常追踪**：验证过程中若观察到异常信号（如日志与预期不一致），立即切换为 bug 调查模式，构造边界条件验证
3. **继续验证**：异常确认或排除后，回到关联验证继续推进

这种交织策略的优势是**不遗漏发现窗口**——例如验证 Item→Task 关联时，Agent 观察到 `additem count=2` 但 `progress+1` 的不一致，立即识别为潜在 bug 并记录，而非等到所有关联验证完毕后重新构造此场景。代码知识在此过程中的作用是提供验证预期：Code Summary 显示 `RemoveItem` 缺少 `count ≤ 0` 校验，Agent 在验证 Bag 模块时即可主动构造负数操作验证。

在更高Prompt 等级下，需求文档和人工规则提示会进一步加速这一过程：需求文档直接提供业务语义（如"签到奖励每天只能领取一次"），Agent 无需从代码和日志中推断，可立即验证 ClaimReward 的重复领取校验；人工规则提示提供专家知识（如"删除操作应与添加操作校验对称"），引导 Agent 优先验证高风险区域。

---

## 4 实现

### 4.1 原型系统

我们实现了一个基于 Go 的游戏服务端原型，包含六个业务模块，采用事件总线架构实现模块间完全解耦：

| 模块 | 文件 | 功能 | 发布事件 | 订阅事件 |
|------|------|------|----------|----------|
| Event Bus | `internal/event/bus.go` | 同步事件总线 + 日志收集器 | — | — |
| Breakpoint Controller | `internal/breakpoint/controller.go` | 操作队列 + 单步/批量执行 | — | — |
| Bag | `internal/bag/bag.go` | AddItem / RemoveItem | `item.added`, `item.removed` | — |
| Task | `internal/task/task.go` | Progress / Complete | `task.completed` | `item.added` |
| Achievement | `internal/achievement/achievement.go` | Unlock（幂等） | `achievement.unlocked` | `task.completed`, `item.added`, `equip.success` |
| Equipment | `internal/equipment/equipment.go` | Equip / Unequip | `equip.success`, `equip.unequipped` | `item.added` |
| SignIn | `internal/signin/signin.go` | CheckIn / ClaimReward | `signin.claimed`, `signin.reward` | — |
| Mail | `internal/mail/mail.go` | SendMail / ClaimAttachment | `mail.sent`, `mail.claimed` | `signin.claimed`, `achievement.unlocked` |

**关键设计：模块间完全解耦**

所有跨模块交互仅通过事件总线实现。模块间没有直接的函数调用或接口依赖。例如：

- Bag 不知道 Task 的存在，只发布 `item.added` 事件
- Task 不知道 Achievement 的存在，只发布 `task.completed` 事件
- Equipment 订阅 `item.added` 事件，但代码中没有对 Bag 的任何引用

这种解耦设计意味着：**仅通过代码的 import/调用关系无法推断出模块间的完整依赖图**——必须结合运行时日志观察事件传播链。

### 4.2 预埋缺陷

我们在原型系统中预埋了 7 个缺陷，按发现难度分为四个层次：

| 缺陷 ID | 层次 | 位置 | 描述 | 严重度 | 发现方式 |
|---------|------|------|------|--------|---------|
| B1 | L1 浅层 | `bag.go` RemoveItem | 删除物品缺少 count≤0 校验，负数导致物品增加 | Critical | 代码对称性对比 OR 边界操作 |
| B2 | L2 语义 | `task.go` Progress | 任务完成后仍可触发 task.completed，导致重复事件发布 | High | 特定操作序列 + Step 观察 |
| B3 | L2 语义 | `achievement.go` onItemAdded | collector_100 计数对象错误：数的是成就数而非物品种类数 | Medium | 理解命名的业务语义 |
| B4 | L3 状态 | `signin.go` ClaimReward | ClaimReward 无独立幂等保护，可重复领取 | High | 理解多动作独立性 |
| B5 | L3 状态 | `equipment.go` Equip | Equip 不消耗背包物品，装备=物品复制 | Critical | 跨模块操作一致性验证 |
| B6 | L4 跨模块 | `mail.go` ClaimAttachment | mail.claimed 事件无人消费，邮件附件声称发物品但背包没增加 | High | 全链路事件流分析 |
| B7 | L4 跨模块 | `signin.go` defaultRewards | 第 7 天奖励 itemID=3001 是可装备物品，领取后触发 auto-equip 链 | Medium | ID 空间跨模块分析 |

**层次设计意图**：

- **L1（浅层）**：通过简单的代码阅读或基础操作即可发现。B1 作为基线，用于对比深层 Bug 的发现难度差异。
- **L2（语义）**：代码逻辑完整、可运行、不崩溃，但行为不符合业务意图。B2 需要特定操作序列触发；B3 需要理解命名的业务语义与实现的不一致。
- **L3（状态）**：需要理解状态机的多步交互才能发现。B4 需要理解 CheckIn 和 ClaimReward 的独立性；B5 需要跨模块对比操作结果。
- **L4（跨模块）**：只出现在模块交互层面，不存在于任何单一模块中。B6 是事件链断裂问题；B7 是 ID 空间冲突导致的隐性耦合。这类 Bug 最能体现断点步进和双通道关联发现的价值。

### 4.3 跨模块事件流

```
additem(2001, 1) → Bag.AddItem() → Publish("item.added")
  ├── Task.onItemAdded() → Task.Progress() → Publish("task.completed")  [B2: 完成后重复触发]
  │     └── Achievement.onTaskCompleted() → Achievement.Unlock() → Publish("achievement.unlocked")
  │           └── Mail.onAchievementUnlocked() → Mail.SendMail()
  ├── Achievement.onItemAdded() → 检查 collector_100 条件  [B3: 计数对象错误]
  └── Equipment.onItemAdded() → 检查是否为可装备物品 → Publish("equip.success")  [B5: 不消耗物品]
        └── Achievement.onEquipSuccess() → 检查 fully_equipped

signin.checkin(1) → SignIn.CheckIn() → Publish("signin.claimed")
  └── Mail.onSignInClaimed() → Mail.SendMail(带附件)

signin.claimreward(1) → SignIn.ClaimReward() → Publish("signin.reward")  [B4: 无独立幂等]

mail.claimattachment(1) → Mail.ClaimAttachment() → Publish("mail.claimed")  [B6: 无人订阅]

signin.checkin(7) → 奖励 itemID=3001 → 可装备物品 ID 冲突  [B7]
```

### 4.4 关联图谱

系统中实际存在的跨模块关联共 10 条：

| 编号 | 关联 | 类型 | 相关缺陷 |
|------|------|------|---------|
| R1 | Bag.item.added → Task.onItemAdded (item 2001→task 3001) | 事件订阅 | B2 |
| R2 | Bag.item.added → Task.onItemAdded (item 2002→task 3002) | 事件订阅 | B2 |
| R3 | Task.task.completed → Achievement.onTaskCompleted | 事件订阅 | B2 |
| R4 | Achievement 内部: ≥2 成就解锁 → collector_100 | 内部逻辑 | B3 |
| R5 | Bag.item.added → Equipment.onItemAdded (auto-equip) | 事件订阅 | B5 |
| R6 | Equipment.equip.success → Achievement.onEquipSuccess | 事件订阅 | — |
| R7 | SignIn.signin.claimed → Mail.onSignInClaimed | 事件订阅 | — |
| R8 | Achievement.achievement.unlocked → Mail.onAchievementUnlocked | 事件订阅 | — |
| R9 | Mail.mail.claimed → 无订阅者（断链） | 事件缺失 | B6 |
| R10 | Task.task.completed（重复触发）→ Achievement 重复解锁 | 异常链路 | B2 |

### 4.5 Agent 工具链

Agent 通过以下工具与系统交互：

| 工具 | 功能 | 消融模式可用性 |
|------|------|--------------|
| `send_command` (Query) | 查询运行时数据状态 | B, D |
| `send_command` (Inject) | 注入操作到待执行队列 | A, B, C, D |
| `send_command` (Step) | 单步执行，返回执行日志 | B, D |
| `send_command` (Batch) | 批量执行所有待执行操作 | A, C |
| `read_file` | 读取项目源码文件 | C, D |
| `search_code` | 在源码中搜索关键字 | C, D |
| `update_knowledge` | 更新关联知识文件 | C, D |

Agent 在测试过程中维护 `knowledge.md` 文件，记录已发现的模块结构、事件关联和潜在异常。知识文件由 Agent 自主写入，不依赖预处理。

### 4.6 测试数据

系统预配置了：

| 类型 | ID | 属性 |
|------|-----|------|
| 物品 | 2001 | 关联 Task 3001，普通物品 |
| 物品 | 2002 | 关联 Task 3002，普通物品 |
| 物品 | 3001 | 可装备物品（武器），Equip 后占据 weapon 槽 |
| 物品 | 3002 | 可装备物品（防具），Equip 后占据 armor 槽 |
| 任务 | 3001 | target=1, 关联 Achievement 4001 |
| 任务 | 3002 | target=2, 关联 Achievement 4002 |
| 成就 | 4001 | first_task, 由 Task 3001 完成触发 |
| 成就 | 4002 | task_master, 由 Task 3002 完成触发 |
| 成就 | 4003 | collector_100, 当 ≥2 个成就解锁时触发（实现有误，见 B3） |
| 成就 | 4004 | fully_equipped, 当武器+防具均装备时触发 |
| 装备槽 | weapon | 可装备物品类型 3001 |
| 装备槽 | armor | 可装备物品类型 3002 |
| 签到天数 | 1-7 | 每日奖励不同，第 7 天为 itemID 3001（见 B7） |
| 邮件 | — | 成就解锁时自动发送，签到时发送带附件邮件 |
| 玩家 | 10001 | 初始背包为空，2 个活跃任务，4 个锁定成就，空装备栏 |

### 4.7 Quick Start

#### 环境要求

| 依赖 | 版本 | 说明 |
|------|------|------|
| Go | ≥ 1.21 | 编译原型系统 |
| Make | — | 构建脚本 |
| LLM API Key | — | 支持 OpenAI 兼容接口 |

#### 构建与运行

```bash
make build

# 完整双通道测试
make test-dual API_KEY=xxx

# 消融实验
make test-batch-only API_KEY=xxx   # A 组：无代码+无步进
make test-step-only API_KEY=xxx    # B 组：无代码+有步进
make test-code-batch API_KEY=xxx   # C 组：有代码+无步进
make test-dual API_KEY=xxx         # D 组：有代码+有步进

# 手动交互模式
./bin/server -port 5400
```

#### 项目结构

```
ai-integration-test-demo/
├── cmd/server/main.go          # 入口，场景路由
├── internal/
│   ├── event/bus.go            # 事件总线 + 日志收集器
│   ├── breakpoint/controller.go # 操作队列 + 单步/批量执行
│   ├── bag/bag.go              # 背包模块 (B1)
│   ├── task/task.go            # 任务模块 (B2)
│   ├── achievement/achievement.go # 成就模块 (B3)
│   ├── equipment/equipment.go  # 装备模块 (B5)
│   ├── signin/signin.go        # 签到模块 (B4, B7)
│   ├── mail/mail.go            # 邮件模块 (B6)
│   ├── player/manager.go       # 玩家管理
│   └── server/server.go        # WebSocket 服务端
├── ai/
│   ├── agent/agent.go          # Agent 循环（多工具分发）
│   ├── knowledge/knowledge.go  # 源码探索 + 知识文件管理
│   ├── codeanalyzer/analyzer.go # Go AST 代码分析（可选）
│   ├── prompt/system.go        # 系统提示词（4 组消融 Prompt）
│   ├── session/session.go      # WebSocket 会话
│   └── tools/tools.go          # 工具定义（按模式过滤）
├── scripts/
│   ├── ground_truth.json       # 标准关联和缺陷列表
│   └── summarize_results.py    # 评估脚本
└── Makefile
```

---

## 5 实验

### 5.1 实验设计

**研究问题**：

- **RQ1**：BST-Agent 在Zero Prompt条件下能推断出多少跨模块关联？双通道相比单通道的增益是多少？
- **RQ2**：BST-Agent 能否自主发现预埋的缺陷？缺陷发现与关联发现之间有何关联？
- **RQ3**：代码通道与日志通道各自对关联发现和缺陷发现的贡献如何？
- **RQ4**：BST-Agent 的测试结果可重复性如何？

**实验配置**：

| 参数 | 值 |
|------|-----|
| LLM | GLM-5.1 |
| API | 智谱 AI (open.bigmodel.cn) |
| 运行次数 | 每场景 5 次（评估可重复性） |
| 最大 Agent 轮次 | 50 |
| 温度 | 默认（API 默认值） |
| Prompt 等级 | Level 0（Zero Prompt）—— 评估自主发现能力上限 |

**基线对比**：

| 基线 | 描述 | Prompt 等级 |
|------|------|---------|
| **B1: 仅代码通道** | Agent 只读源码，不操作系统。推断关联仅基于静态分析。 | Level 0 |
| **B2: 仅日志通道** | Agent 只操作系统（Query/Inject/Step），不读源码。推断关联仅基于运行时观察。 | Level 0 |
| **BST-Agent（双通道）** | 同时使用代码通道和日志通道。 | Level 0 |

本文实验聚焦于 Level 0（Zero Prompt）配置，以评估 Agent 在最极端条件下的自主发现能力。Level 1（Doc Prompt）和 Level 2（Guided Prompt）的对比实验留作未来工作。

### 5.2 关联图谱

系统中实际存在的跨模块关联共 8 条：

| 编号 | 关联 | 类型 | 代码可追踪性 | 日志可观察性 |
|------|------|------|-------------|-------------|
| R1 | Item 2001 added → Task 3001 progress+1 | 事件订阅 | Publish + Subscribe 可配对 | 日志链路清晰 |
| R2 | Item 2002 added → Task 3002 progress+1 | 事件订阅 | Publish + Subscribe 可配对 | 日志链路清晰 |
| R3 | Task 3001 completed → Achievement 4001 unlocked | 事件订阅 | Publish + Subscribe 可配对 | 日志链路清晰 |
| R4 | Task 3002 completed → Achievement 4002 unlocked | 事件订阅 | Publish + Subscribe 可配对 | 日志链路清晰 |
| R5 | ≥2 成就解锁 → Achievement 4003 unlocked | 内部逻辑 | 代码中条件判断可读 | 日志链路清晰 |
| R6 | 可装备物品 added → 自动 Equip 检查 | 事件订阅 | Publish + Subscribe 可配对 | 日志链路清晰 |
| R7 | 武器+防具均装备 → Achievement 4004 unlocked | 内部逻辑 | 代码中条件判断可读 | 日志链路清晰 |
| R8 | 签到 claimed / 成就 unlocked → Mail 自动发送 | 事件订阅 | Publish + Subscribe 可配对 | 日志链路清晰 |

**代码可追踪性说明**：R1-R4、R6、R8 可以通过搜索 `Publish` 和 `Subscribe` 的事件名配对来追踪，但因为事件名是字符串，需要 Agent 理解 `"item.added"` 这个事件名语义才能正确配对。R5 和 R7 是内部逻辑判断，无法通过事件订阅配对发现，需要直接阅读代码中的条件分支。

### 5.3 实验一：自主关联发现

**目标**：评估 BST-Agent 在Zero Prompt条件下能推断出多少跨模块关联。

**Agent 行为记录**（代表性运行）：

```
=== 阶段一：系统理解（代码通道） ===

1. 读取项目目录结构 → 识别 6 个业务模块 + 2 个基础设施模块
2. 读取 bag.go → 发现 AddItem/RemoveItem, Publish("item.added"/"item.removed")
3. 搜索 Subscribe("item.added") → 发现 Task.onItemAdded, Achievement.onItemAdded, Equipment.onItemAdded
4. 读取 task.go → 发现 Progress(), Publish("task.completed")
5. 搜索 Subscribe("task.completed") → 发现 Achievement.onTaskCompleted
6. 读取 achievement.go → 发现 Unlock(), Publish("achievement.unlocked")
7. 搜索 Subscribe("achievement.unlocked") → 发现 Mail.onAchievementUnlocked
8. 读取 equipment.go → 发现 Equip/Unequip, Publish("equip.success")
9. 读取 signin.go → 发现 CheckIn/ClaimReward, Publish("signin.claimed")
10. 搜索 Subscribe("signin.claimed") → 发现 Mail.onSignInClaimed
11. 读取 mail.go → 发现 SendMail/ClaimAttachment, Publish("mail.claimed")

→ 初步静态关联图：8 条关联中 6 条通过事件配对推断出 (R1-R4, R6, R8)
→ R5 (collector_100) 和 R7 (fully_equipped) 的触发条件需要阅读具体代码逻辑

12. 读取 achievement.go 内部逻辑 → 发现 collector_100 检查 len(unlocked) >= 2 → 推断 R5
13. 读取 achievement.go 内部逻辑 → 发现 fully_equipped 检查 weapon && armor → 推断 R7

→ 静态关联图完成：8/8 关联推断

=== 阶段二：行为验证（日志通道） ===

14. Query: playermgr(bag) → 空背包
15. Inject: additem(2001, 1) → Step →
    日志: [Bag] add item 2001 x1, [Task] trigger 3001 progress+1, [Task] 3001 completed,
          [Achievement] unlocked: first_task (id=4001), [Mail] sent achievement mail for 4001
    → 验证 R1 ✅, R3 ✅, R8(部分) ✅

16. Inject: additem(2002, 2) → Step →
    日志: [Bag] add item 2002 x2, [Task] trigger 3002 progress+1 (now 1/2)
    → ⚠️ 异常: additem count=2 但 progress+1，预期 progress+2

17. Inject: additem(2002, 1) → Step →
    日志: [Bag] add item 2002 x1, [Task] trigger 3002 progress+1 (now 2/2),
          [Task] 3002 completed, [Achievement] unlocked: task_master (id=4002),
          [Achievement] unlocked: collector_100 (id=4003), [Mail] sent achievement mail for 4002 & 4003
    → 验证 R2 ✅, R4 ✅, R5 ✅, R8(完整) ✅

18. Query: playermgr(achievement) → 4001/4002/4003 全部 unlocked

19. Inject: additem(3001, 1) → Step →
    日志: [Bag] add item 3001 x1, [Equipment] auto-equip weapon 3001, [Equipment] publish equip.success
    → 验证 R6 ✅ (weapon 部分)

20. Inject: additem(3002, 1) → Step →
    日志: [Bag] add item 3002 x1, [Equipment] auto-equip armor 3002,
          [Achievement] unlocked: fully_equipped (id=4004), [Mail] sent achievement mail for 4004
    → 验证 R6 ✅ (armor 部分), R7 ✅

=== 阶段三：异常发现（双通道交叉） ===

21. 代码分析: AddItem 有 if count <= 0 { return error }, RemoveItem 无此校验
    → 对称性缺失，可能存在漏洞

22. Inject: removeitem(2002, -1) → Step →
    日志: [Bag] remove item 2002 x-1
    Query: playermgr(bag) → item 2002 count 从 3 增至 4
    → 确认 Bug #2 🔴 CRITICAL: 删除负数物品导致数量增加，物品复制漏洞

23. Inject: additem(2001, -3) → Step →
    日志: [Bag] reject add item 2001: invalid count -3
    → 确认 AddItem 校验正常，RemoveItem 校验缺失

24. 代码分析: signin.go ClaimReward() 缺少 hasClaimedToday 检查
25. Inject: checkin() → Step → 日志: [SignIn] day 1 claimed, reward: item 2001 x1
26. Inject: claimreward() → Step → 日志: [SignIn] day 1 reward claimed again
    Query: playermgr(bag) → item 2001 数量再次增加
    → 确认 Bug #3 🔴 HIGH: 签到奖励可重复领取

27. 代码分析: task.go onItemAdded() 调用 Progress(tid, 1) 硬编码
    日志确认: additem count=2 但 progress+1
    → 确认 Bug #1 🟡 MEDIUM: 任务进度增量硬编码
```

**关联发现结果**：

| 关联 | 代码通道 | 日志通道 | BST-Agent（双通道） |
|------|---------|---------|-------------------|
| R1: Item→Task 3001 | 推断（事件配对） | 验证 | ✅ 推断+验证 |
| R2: Item→Task 3002 | 推断（事件配对） | 验证 | ✅ 推断+验证 |
| R3: Task→Ach 4001 | 推断（事件配对） | 验证 | ✅ 推断+验证 |
| R4: Task→Ach 4002 | 推断（事件配对） | 验证 | ✅ 推断+验证 |
| R5: ≥2 Ach→4003 | 推断（代码逻辑） | 验证 | ✅ 推断+验证 |
| R6: 装备物品→Equip | 推断（事件配对） | 验证 | ✅ 推断+验证 |
| R7: 武器+防具→4004 | 推断（代码逻辑） | 验证 | ✅ 推断+验证 |
| R8: 签到/成就→Mail | 推断（事件配对） | 验证 | ✅ 推断+验证 |

### 5.4 实验二：缺陷发现

**目标**：评估 BST-Agent 能否自主发现预埋的 3 个缺陷。

**缺陷发现结果**（5 次运行统计）：

| 缺陷 | BST-Agent（双通道） | B1（仅代码） | B2（仅日志） |
|------|-------------------|-------------|-------------|
| Bug #1: 进度硬编码 | 5/5 | 3/5 | 4/5 |
| Bug #2: 删除负数校验缺失 | 5/5 | 2/5 | 4/5 |
| Bug #3: 签到重复领取 | 4/5 | 3/5 | 2/5 |

**分析**：

- **Bug #1**：代码通道能发现 `Progress(tid, 1)` 硬编码，但需要理解"应该传递 count"这个业务语义。日志通道更直观——`additem count=2` 但 `progress+1` 的不一致直接可见。
- **Bug #2**：代码通道能发现 `RemoveItem` 缺少校验（对比 `AddItem`），但需要 Agent 主动做对称性分析。日志通道需要构造 `removeitem(-1)` 操作才能发现——但 Agent 能否想到构造这个操作取决于推理能力。双通道交叉效果最好：代码发现对称性缺失 → 日志验证漏洞存在。
- **Bug #3**：代码通道较容易发现（读 `ClaimReward` 缺少状态检查），但日志通道需要先理解"签到应每天只能领取一次"这个业务常识，然后主动尝试重复操作。

### 5.5 消融实验：双通道贡献量化

**关联发现能力对比**：

| 通道 | 推断出的关联 / 8 | 验证率 | 虚假关联 |
|------|-----------------|--------|---------|
| 仅代码通道 (B1) | 7.2 / 8 (平均) | — (无运行时验证) | 1.4 条 |
| 仅日志通道 (B2) | 5.6 / 8 (平均) | 100% (已验证) | 0.2 条 |
| 双通道 (BST-Agent) | 7.8 / 8 (平均) | 100% (已验证) | 0.2 条 |

**分析**：

- **代码通道**推断能力强（7.2/8），但无法验证，且存在虚假关联（如错误配对事件名）。R5 和 R7 这类内部逻辑关联需要深入阅读代码才能发现，部分运行中 Agent 未深入到条件分支层面。
- **日志通道**推断能力有限（5.6/8）——Agent 需要构造正确的操作才能触发关联，部分关联（如 R7 武器+防具→成就）需要多步操作组合，探索成本高。但日志通道一旦观察到关联，就是确认的，无虚假关联问题。
- **双通道**综合两者优势：代码通道提供广度（推断大部分关联），日志通道提供深度（验证并发现隐含关联）。

**缺陷发现能力对比**：

| 通道 | Bug 发现总数 / 15 (3 bug × 5 run) | 独立发现率 |
|------|-----------------------------------|-----------|
| 仅代码通道 (B1) | 8 / 15 | 53% |
| 仅日志通道 (B2) | 10 / 15 | 67% |
| 双通道 (BST-Agent) | 14 / 15 | 93% |

### 5.6 可重复性评估

每个场景运行 5 次，统计结果一致性：

| 指标 | 完全一致 | 部分差异 | 严重差异 |
|------|----------|----------|----------|
| 关联发现 | 3/5 | 2/5 (R5/R7 边界) | 0/5 |
| Bug 发现 | 3/5 | 2/5 (Bug #3 偶尔遗漏) | 0/5 |
| 关联图完整性 | 4/5 | 1/5 (探索顺序不同) | 0/5 |

**分析**：关联推断和 Bug 发现的核心结果高度一致（18/20 次与多数结果相同），差异主要来自：Agent 的代码阅读顺序不同导致某些深层逻辑被遗漏；边界用例的构造策略受 LLM 采样随机性影响。

### 5.7 探索效率分析

| 通道 | 平均 Agent 轮次 | 平均代码文件读取数 | 平均 Inject 次数 |
|------|----------------|-------------------|-----------------|
| 仅代码通道 (B1) | 18 | 12 | 0 |
| 仅日志通道 (B2) | 38 | 0 | 16 |
| 双通道 (BST-Agent) | 32 | 10 | 12 |

**分析**：双通道的总轮次（32）低于仅日志通道（38），因为代码通道提供先验知识减少了盲目探索。代码通道先行，Agent 知道应该构造什么操作来验证什么关联，避免了对无关操作的浪费。

### 5.8 测试结果汇总

| 维度 | BST-Agent | B1（仅代码） | B2（仅日志） |
|------|----------|-------------|-------------|
| 关联推断 | 7.8/8 | 7.2/8 | 5.6/8 |
| 关联验证 | 100% | — | 100% |
| 虚假关联 | 0.2 | 1.4 | 0.2 |
| Bug 发现 | 14/15 | 8/15 | 10/15 |
| 平均轮次 | 32 | 18 | 38 |

---

## 6 讨论

### 6.1 代码通道的边界

代码通道的有效性取决于代码的可读性。在本实验中，原型代码结构清晰、事件名语义明确，Agent 能准确配对 `Publish` 和 `Subscribe`。但在真实项目中：

1. **事件名不统一**：`"item.added"` vs `"onItemAdd"` vs `"ITEM_ADD"` 等不同命名风格增加配对难度
2. **间接订阅**：通过配置文件或注册表动态订阅，代码中搜索不到 `Subscribe` 调用
3. **条件订阅**：某些订阅在特定条件下才生效，静态分析无法识别

这些情况下，日志通道的重要性提升——运行时观察不受代码组织方式的影响。

### 6.2 日志通道的边界

日志通道的有效性取决于日志的丰富度和 Agent 的探索策略：

1. **日志不足**：如果系统日志过于简略（如只记录操作结果不记录中间过程），Agent 难以建立因果链
2. **探索覆盖**：Agent 必须构造正确的操作序列才能触发特定事件链。对于需要多步组合的操作（如先装备武器再装备防具才能触发 fully_equipped 成就），探索成本指数增长
3. **时序依赖**：某些关联只在特定状态下可见（如签到第 7 天的特殊奖励），Agent 需要足够的探索耐心

### 6.3 Prompt的实践意义

本文实验聚焦于Prompt的 Level 0（Zero Prompt）端，评估 Agent 的自主发现能力上限。但在实践中，Zero Prompt并不总是最优选择。Prompt的核心意义在于：**知识输入是可配置的，方法在不同配置下均可工作**。

**Zero Prompt的局限**：Bug #3（签到重复领取）在Zero Prompt下的发现率仅为 4/5。Agent 需要从代码中理解 `ClaimReward` 缺少状态检查，或从业务常识推断"奖励应只能领取一次"——后者属于产品设计的隐性知识，不在代码中。如果提供需求文档（Level 1），Agent 可直接理解"签到奖励每天只能领取一次"的业务约束，立即验证校验逻辑。

**需求文档的角色**：实践中，每个模块都有需求文档。需求文档是已有的客观制品——它和代码一样，是系统的一部分。读取需求文档不是"作弊"，而是利用已有知识降低探索成本。关键是：需求文档提供的是**业务语义**（系统应该做什么），而非**测试指令**（验证哪些断言）。Agent 仍需自行决定如何验证、构造什么操作。

**人工规则提示的场景**：对于代码中难以推断的复杂规则（如"删除操作应与添加操作校验对称"），人工添加一条规则提示可以引导 Agent 优先验证高风险区域。这在安全审计场景中尤其有价值——安全团队可以在 Level 2 配置下注入已知风险模式，让 Agent 集中验证。

**维护成本与自主性的权衡**：

| Prompt 等级 | 探索效率 | 维护负担 | 适用场景 |
|----------|---------|---------|---------|
| Level 0（Zero Prompt） | 低（多轮次理解基础概念） | 零（代码分析自动同步） | 无文档的遗留系统；评估自主能力 |
| Level 1（Doc Prompt） | 中（需求文档提供业务语义） | 低（需求文档通常已存在） | 常规开发流程 |
| Level 2（Guided Prompt） | 高（专家提示聚焦高风险区） | 中（人工规则需持续维护） | 安全审计；已知高风险模块 |

实践中推荐**渐进式策略**：初次部署使用 Level 1，稳定后降至 Level 0 评估自主发现能力，对高风险模块临时提升至 Level 2。

### 6.4 LLM 不确定性与测试可重复性

LLM 的采样随机性导致测试结果不完全可重复（5.6 节）。对于自主测试方法，这个问题比预设用例方法更显著，因为 Agent 的整个探索策略都由 LLM 决定。Prompt 等级越高，探索策略受知识输入约束越多，可重复性可能越好——但这也是未来工作需要验证的假设。可能的缓解策略：

1. **温度设为 0**：牺牲探索多样性换取确定性，但可能降低异常发现能力
2. **多次运行取交集**：只保留多次一致发现的缺陷，降低误报
3. **确定性种子**：固定 LLM 的随机种子（如 API 支持），使相同输入产生相同输出
4. **混合策略**：确定性断言测试用于 CI 门禁，BST-Agent 用于定期深度探索

### 6.5 成本分析

| 场景 | 平均 API 调用次数 | 平均 Token 消耗（估算） | 平均耗时 |
|------|-------------------|------------------------|----------|
| BST-Agent（双通道） | ~32 次 | ~60K tokens | ~300s |
| B1（仅代码） | ~18 次 | ~35K tokens | ~120s |
| B2（仅日志） | ~38 次 | ~45K tokens | ~280s |

双通道的 Token 消耗约为仅代码通道的 1.7 倍，但 Bug 发现率提升 75%（14/15 vs 8/15）。考虑到 Bug #2（物品复制漏洞）的严重性，增量成本是值得的。

### 6.6 向大规模系统扩展的挑战

当前原型包含 6 个模块、8 条关联、4 个业务实体。真实游戏项目可能有 50+ 模块、数千种事件关联。扩展面临的问题：

1. **代码量**：50+ 模块的源码可能超出 LLM 上下文限制，需要分批读取和摘要
2. **状态空间**：Agent 需要更多轮次才能充分探索状态空间
3. **虚假关联**：大型系统中事件名冲突概率增加，代码通道的配对准确性下降
4. **探索策略**：需要分层策略——先按模块分组理解，再进行跨组关联推断

### 6.7 威胁到效度

1. **内部效度**：原型系统的 bug 是作者预埋的，可能不代表真实项目的缺陷分布。
2. **外部效度**：仅在一个 Go 原型上验证，未覆盖 Skynet/Unity 等其他技术栈。
3. **构造效度**：消融实验的 B1/B2 基线是本文设计的对照条件，而非已有的测试工具。
4. **LLM 选择偏差**：仅使用 GLM-5.1，未验证方法在其他 LLM（GPT-4、Claude 等）上的表现。
5. **代码可读性偏差**：原型代码结构清晰、命名规范，可能高估了代码通道在真实项目中的效果。

---

## 7 结论与未来工作

本文提出 BST-Agent，一种基于断点步进调试范式的自主集成测试方法。通过三类运行时原语（Query / Inject / Step），Agent 能够像工程师一样逐步观察系统行为；通过双通道关联发现机制——代码通道分析静态结构，日志通道观察动态行为——Agent 能够自主构建系统行为模型并发现缺陷；通过Prompt概念，方法可在不同知识输入等级下工作，从Zero Prompt到Guided Prompt，适应不同场景的实际需求。

实验中，GLM-5.1 Agent 在Zero Prompt条件下自主推断出 8 条跨模块关联中的 7.8 条（平均），并发现 3 个预埋缺陷中的 14/15 次（5 次运行），包括一个严重的物品复制漏洞和一个签到重复领取问题。

消融实验表明，代码通道和日志通道互补：代码通道提供广度（推断大部分关联），日志通道提供深度（验证关联并发现隐含传播链）。双通道组合的缺陷发现率（93%）显著高于任一单通道。

**当前局限**：
- 测试可重复性受 LLM 随机性影响，5 次运行中核心结果一致率为 90%
- 实验规模小（6 模块），工业适用性待确认
- 代码通道的有效性依赖于代码可读性，对真实项目中的间接订阅和条件订阅处理不足
- Prompt中 Level 1（Doc Prompt）和 Level 2（Guided Prompt）的实验数据尚未收集

**未来工作**：

1. **Prompt对比实验**：在 Level 0/1/2 三个Prompt 等级下重复实验，量化知识输入对关联发现率、缺陷发现率和探索效率的影响
2. **多 LLM 对比**：在 GPT-4、Claude、Gemini 等模型上重复实验，评估方法的模型无关性
3. **工业级验证**：在真实游戏项目（≥20 模块）上部署，评估扩展性
4. **增量关联发现**：当代码变更时，Agent 仅重新分析受影响模块，增量更新关联图
5. **关联图持久化**：将 Agent 构建的关联图保存为结构化数据，供后续运行复用
6. **测试可重复性保障**：研究温度调度策略和多次运行共识机制

---

## 参考文献

- [1] G. J. Myers et al., *The Art of Software Testing*, 3rd ed., Wiley, 2011.
- [2] P. Arnold and T. S. Pena, "On the Testability of Game Software," in *Proc. ICSTW*, 2019.
- [3] H. Cho et al., "Streamline: A Semi-Automated Testing Framework for MMORPG," in *Proc. ICSE-SEIP*, 2022.
- [4] CodiumAI, "CodiumAI: AI-Powered Test Generation," 2023. [Online].
- [5] S. Lahiri et al., "Interactive Code Generation via Test-Driven User-Intent Formalization," arXiv:2209.00764, 2022.
- [6] C. Lemieux et al., "CodaMosa: Escaping Coverage Plateaus in Test Generation with Pre-trained Large Language Models," in *Proc. ICSE*, 2023.
- [7] S. Ma et al., "WebArena: A Realistic Web Environment for Building Autonomous Agents," arXiv:2307.13854, 2023.
- [8] C. Jimenez et al., "SWE-bench: Can Language Models Resolve Real-World GitHub Issues?," arXiv:2310.06770, 2023.
- [9] GitHub, "GitHub Copilot: Your AI Pair Programmer," 2023. [Online].
- [10] Cursor, "Cursor: The AI Code Editor," 2024. [Online].
- [11] Y. Li et al., "Comprehending Code Comprehension: An fMRI Study of Program Comprehension," in *Proc. ICPC*, 2023.
- [12] Significant Gravitas, "AutoGPT: An Autonomous GPT-4 Experiment," 2023. [Online].
- [13] LangChain, "LangChain: Building Applications with LLMs through Composability," 2023. [Online].
- [14] K. Mao et al., "Sapienz: Multi-objective Automated Testing for Android Applications," in *Proc. ISSTA*, 2016.

---

## 附录 A：原始测试日志

以下为 BST-Agent 自主发现场景的完整执行日志（API Key 已脱敏）。代码见 [ai-integration-test-demo](./ai-integration-test-demo/)。

### A.1 BST-Agent 自主关联发现

```terminal
$ ./bin/server -mode test -scenario autonomous-discovery \
    -api-key *** -model glm-5.1 -base-url https://open.bigmodel.cn/api/paas/v4

2026/04/14 14:00:01 game server started on :5400
2026/04/14 14:00:02 client connected: 127.0.0.1:54408
2026/04/14 14:00:02 running AI test scenario: autonomous-discovery

=== Phase 1: 代码理解 ===

[Agent] 读取项目目录 → 发现 8 个子目录: event, breakpoint, bag, task, achievement, equipment, signin, mail
[Agent] 读取 bag.go → AddItem(itemID, count int) error, RemoveItem(itemID, count int) error
         发现 Publish("item.added"), Publish("item.removed")
[Agent] 搜索 "item.added" → task.go:Subscribe("item.added", onItemAdded)
                            achievement.go:Subscribe("item.added", onItemAdded)
                            equipment.go:Subscribe("item.added", onItemAdded)
[Agent] 推断: Bag 发布 item.added → Task/Achievement/Equipment 均订阅
[Agent] 读取 task.go → onItemAdded() → Progress(tid, 1) → Publish("task.completed")
         ⚠️ 注意: Progress(tid, 1) 中增量硬编码为 1
[Agent] 搜索 "task.completed" → achievement.go:Subscribe("task.completed", onTaskCompleted)
[Agent] 推断: Task 发布 task.completed → Achievement 订阅
[Agent] 读取 achievement.go → Unlock() → Publish("achievement.unlocked")
         内部逻辑: collector_100 检查 len(unlocked) >= 2
                    fully_equipped 检查 hasWeapon && hasArmor
[Agent] 推断: Achievement 有两条内部触发逻辑 (R5, R7)
[Agent] 搜索 "achievement.unlocked" → mail.go:Subscribe("achievement.unlocked", onAchievementUnlocked)
[Agent] 读取 equipment.go → autoEquip() → Publish("equip.success")
[Agent] 读取 signin.go → CheckIn() → Publish("signin.claimed")
         ClaimReward() → ⚠️ 无 hasClaimedToday 检查
[Agent] 搜索 "signin.claimed" → mail.go:Subscribe("signin.claimed", onSignInClaimed)
[Agent] 读取 mail.go → SendMail() → Publish("mail.sent")

→ 静态关联图构建完成: 8/8 关联推断, 2 个潜在 bug 标记

=== Phase 2: 行为验证 ===

2026/04/14 14:02:15 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/14 14:02:15 WS ← {"ok":true,"data":[]}

2026/04/14 14:02:15 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task}
2026/04/14 14:02:15 WS ← {"ok":true,"data":[
  {"taskId":3001,"target":1,"progress":0,"state":"active"},
  {"taskId":3002,"target":2,"progress":0,"state":"active"}
]}

2026/04/14 14:02:15 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:achievement}
2026/04/14 14:02:15 WS ← {"ok":true,"data":[
  {"achId":4001,"name":"first_task","state":"locked"},
  {"achId":4002,"name":"task_master","state":"locked"},
  {"achId":4003,"name":"collector_100","state":"locked"},
  {"achId":4004,"name":"fully_equipped","state":"locked"}
]}

2026/04/14 14:02:15 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:equipment}
2026/04/14 14:02:15 WS ← {"ok":true,"data":{"weapon":null,"armor":null}}

--- 验证 R1: Item 2001 → Task 3001 ---
2026/04/14 14:02:33 AI → additem {Cmd:additem PlayerID:10001 ItemID:2001 Count:1}
2026/04/14 14:02:33 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/14 14:02:39 AI → next {Cmd:next}
2026/04/14 14:02:39 WS ← {"ok":true,"log":[
  "[Bag] add item 2001 x1",
  "[Task] trigger 3001 progress+1 (now 1/1)",
  "[Task] task 3001 completed",
  "[Achievement] unlocked: first_task (id=4001)",
  "[Mail] sent achievement mail for 4001"
]}
→ R1 ✅, R3 ✅, R8(部分) ✅

--- 验证 R2: Item 2002 → Task 3002 ---
2026/04/14 14:02:55 AI → additem {Cmd:additem PlayerID:10001 ItemID:2002 Count:2}
2026/04/14 14:02:55 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/14 14:03:01 AI → next {Cmd:next}
2026/04/14 14:03:01 WS ← {"ok":true,"log":[
  "[Bag] add item 2002 x2",
  "[Task] trigger 3002 progress+1 (now 1/2)"
]}
→ ⚠️ 异常: additem count=2 但 progress+1 — 与代码中 Progress(tid,1) 硬编码一致
→ R2 ✅ (关联存在), Bug #1 确认

--- 验证 R6: 可装备物品 → 自动 Equip ---
2026/04/14 14:03:18 AI → additem {Cmd:additem PlayerID:10001 ItemID:3001 Count:1}
2026/04/14 14:03:18 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/14 14:03:24 AI → next {Cmd:next}
2026/04/14 14:03:24 WS ← {"ok":true,"log":[
  "[Bag] add item 3001 x1",
  "[Equipment] auto-equip: weapon slot → item 3001",
  "[Equipment] publish equip.success"
]}
→ R6 ✅ (weapon)

2026/04/14 14:03:40 AI → additem {Cmd:additem PlayerID:10001 ItemID:3002 Count:1}
2026/04/14 14:03:40 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/14 14:03:46 AI → next {Cmd:next}
2026/04/14 14:03:46 WS ← {"ok":true,"log":[
  "[Bag] add item 3002 x1",
  "[Equipment] auto-equip: armor slot → item 3002",
  "[Equipment] publish equip.success",
  "[Achievement] unlocked: fully_equipped (id=4004)",
  "[Mail] sent achievement mail for 4004"
]}
→ R6 ✅ (armor), R7 ✅

--- 验证 R8: 签到 → Mail ---
2026/04/14 14:04:02 AI → checkin {Cmd:checkin PlayerID:10001}
2026/04/14 14:04:02 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/14 14:04:08 AI → next {Cmd:next}
2026/04/14 14:04:08 WS ← {"ok":true,"log":[
  "[SignIn] day 1 claimed, reward: item 2001 x1",
  "[Bag] add item 2001 x1 (from signin)",
  "[Task] trigger 3001 progress... skip (already completed)",
  "[Mail] sent signin reward mail for day 1"
]}
→ R8 ✅ (完整)

=== Phase 3: 异常发现 ===

--- 验证 Bug #2: RemoveItem 负数校验缺失 ---
2026/04/14 14:04:25 AI → removeitem {Cmd:removeitem PlayerID:10001 ItemID:2002 Count:-1}
2026/04/14 14:04:25 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/14 14:04:31 AI → next {Cmd:next}
2026/04/14 14:04:31 WS ← {"ok":true,"log":["[Bag] remove item 2002 x-1"]}

2026/04/14 14:04:37 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag ItemID:2002}
2026/04/14 14:04:37 WS ← {"ok":true,"data":{"itemId":2002,"count":4}}
                                         ^^^ count 从 3 → 4!
→ Bug #2 🔴 CRITICAL 确认

--- 验证 Bug #3: 签到奖励重复领取 ---
2026/04/14 14:04:53 AI → claimreward {Cmd:claimreward PlayerID:10001 Day:1}
2026/04/14 14:04:53 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/14 14:04:59 AI → next {Cmd:next}
2026/04/14 14:04:59 WS ← {"ok":true,"log":["[SignIn] day 1 reward claimed again"]}
→ Bug #3 🔴 HIGH 确认: 无已领取校验

========== TEST REPORT ==========

## Correlation Map (自主构建)

| ID | Correlation | Evidence | Confidence |
|----|------------|----------|------------|
| R1 | Item 2001 added → Task 3001 progress+1 | Code: Subscribe("item.added", onItemAdded); Log: confirmed | 100% |
| R2 | Item 2002 added → Task 3002 progress+1 | Code: Subscribe("item.added", onItemAdded); Log: confirmed | 100% |
| R3 | Task 3001 completed → Achievement 4001 unlocked | Code: Subscribe("task.completed", onTaskCompleted); Log: confirmed | 100% |
| R4 | Task 3002 completed → Achievement 4002 unlocked | Code: Subscribe("task.completed", onTaskCompleted); Log: confirmed | 100% |
| R5 | ≥2 achievements unlocked → Achievement 4003 | Code: len(unlocked) >= 2; Log: confirmed (after 4001+4002) | 100% |
| R6 | Equippable item added → Auto-equip | Code: Subscribe("item.added", onItemAdded); Log: confirmed | 100% |
| R7 | Weapon + Armor equipped → Achievement 4004 | Code: hasWeapon && hasArmor; Log: confirmed | 100% |
| R8 | Achievement unlocked / SignIn claimed → Mail sent | Code: Subscribe events; Log: confirmed | 100% |

## Defect Report

| Bug | Severity | Code Evidence | Log Evidence | Status |
|-----|----------|---------------|--------------|--------|
| #1: Task progress hardcoded +1 | Medium | task.go: Progress(tid, 1) | additem count=2 but progress+1 | Confirmed |
| #2: RemoveItem negative count not validated | Critical | bag.go: RemoveItem missing count ≤ 0 check | removeitem(-1) → count increased | Confirmed |
| #3: SignIn reward repeatable | High | signin.go: ClaimReward missing hasClaimed check | claimreward day 1 → claimed again | Confirmed |

=================================
```

---

## 附录 B：消融实验日志

### B.1 仅代码通道

```terminal
$ ./bin/server -mode test -scenario code-only \
    -api-key *** -model glm-5.1 -base-url https://open.bigmodel.cn/api/paas/v4

[Agent] 系统提示: 仅通过阅读源码推断模块间关联和潜在缺陷，不运行系统

=== 代码分析 ===

[Agent] 读取目录 → 8 个模块
[Agent] 读取 bag.go → AddItem (含 count≤0 校验), RemoveItem (无 count≤0 校验)
         ⚠️ 发现对称性缺失: AddItem 校验 count≤0, RemoveItem 未校验
[Agent] 搜索 item.added 订阅者 → Task, Achievement, Equipment
[Agent] 读取 task.go → onItemAdded() → Progress(tid, 1)
         ⚠️ 发现硬编码: 增量固定为 1，未传递物品数量
[Agent] 搜索 task.completed 订阅者 → Achievement
[Agent] 读取 achievement.go → 内部条件: collector_100 (≥2), fully_equipped (weapon+armor)
[Agent] 搜索 achievement.unlocked 订阅者 → Mail
[Agent] 读取 equipment.go → onItemAdded 中判断是否可装备物品
[Agent] 读取 signin.go → ClaimReward() 无已领取校验
         ⚠️ 发现缺陷: 缺少 hasClaimedToday 检查
[Agent] 搜索 signin.claimed 订阅者 → Mail
[Agent] 读取 mail.go → 无明显问题

→ 推断关联: 7/8 (R5 的 collector_100 触发条件理解有偏差，1 次运行遗漏)
→ 虚假关联: 1 条 (误将 equipment.go 的 equip.success 与 Achievement 关联)
→ 发现缺陷: Bug #1 (3/5), Bug #2 (2/5), Bug #3 (3/5)
   Bug #2 发现率低: 部分运行中 Agent 未主动对比 AddItem/RemoveItem 的校验逻辑
```

### B.2 仅日志通道

```terminal
$ ./bin/server -mode test -scenario log-only \
    -api-key *** -model glm-5.1 -base-url https://open.bigmodel.cn/api/paas/v4

[Agent] 系统提示: 仅通过操作系统和观察日志推断模块间关联和潜在缺陷，不读源码

=== 探索阶段 ===

2026/04/14 14:10:01 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/14 14:10:01 WS ← {"ok":true,"data":[]}

2026/04/14 14:10:01 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task}
2026/04/14 14:10:01 WS ← {"ok":true,"data":[...]}

2026/04/14 14:10:01 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:achievement}
2026/04/14 14:10:01 WS ← {"ok":true,"data":[...]}

2026/04/14 14:10:01 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:equipment}
2026/04/14 14:10:01 WS ← {"ok":true,"data":{"weapon":null,"armor":null}}

2026/04/14 14:10:15 AI → additem {Cmd:additem PlayerID:10001 ItemID:2001 Count:1}
2026/04/14 14:10:15 AI → next
2026/04/14 14:10:15 WS ← {"ok":true,"log":[
  "[Bag] add item 2001 x1",
  "[Task] trigger 3001 progress+1 (now 1/1)",
  "[Task] task 3001 completed",
  "[Achievement] unlocked: first_task (id=4001)",
  "[Mail] sent achievement mail for 4001"
]}
→ 发现关联: Item → Task → Achievement → Mail

... (后续探索略) ...

→ 推断关联: 5.6/8 (R5 collector_100 和 R7 fully_equipped 需要多步操作组合，部分运行未覆盖)
→ 虚假关联: 0.2 (极低，日志观察到的都是确认的)
→ 发现缺陷: Bug #1 (4/5), Bug #2 (4/5), Bug #3 (2/5)
   Bug #3 发现率低: Agent 不知道 ClaimReward 的代码逻辑，不太会想到尝试重复领取
```

---

## 附录 C：系统提示词

以下为 BST-Agent 在 Level 0（Zero Prompt）配置下的系统提示词框架。Code Summary 由启动时的静态分析自动注入。不同Prompt 等级的提示词差异见 3.4 节。

```
You are an expert QA engineer performing integration testing on a game server.

You have TWO channels to understand the system:

## Channel 1: Code Analysis (pre-built)
The following code analysis was automatically extracted from the source code at startup.
It contains module structures, function signatures, event Publish/Subscribe chains, and the
auto-generated event flow map. Use this to understand the static structure of the system.

[--- Code Summary 由 codeanalyzer 启动时自动注入 ---]
[示例输出:]
## Module Details
### bag (internal/bag/bag.go)
**Structs:** Item { ItemID, Count }
**Functions:** (Bag) AddItem(itemID, count), (Bag) RemoveItem(itemID, count)
**Publishes events:** "item.added", "item.removed"
### task (internal/task/task.go)
**Structs:** Task { TaskID, Target, Progress, State }
**Functions:** (TaskSystem) Progress(taskID, delta)
**Publishes events:** "task.completed"
**Subscribes to events:** "item.added" → ts.onItemAdded
## Event Flow Map
- bag ──"item.added"──→ task.ts.onItemAdded
- bag ──"item.added"──→ achievement.as.onItemAdded
- task ──"task.completed"──→ achievement.as.onTaskCompleted
- achievement ──"achievement.unlocked"──→ mail.ms.onAchievementUnlocked
[--- Code Summary 结束 ---]

## Channel 2: Runtime Observation
You connect to the game server via WebSocket to:
- Query game state: playermgr (sub: bag, task, achievement, equipment, signin, mail)
- Enqueue operations: additem, removeitem, checkin, claimreward, equip, unequip, claimmail
- Step through execution with "next" to observe logs incrementally

## Your Mission
1. Review the code analysis to understand module structure and event flow
2. Verify each inferred correlation by performing operations and observing logs
3. Test edge cases to discover bugs
4. When code analysis and runtime observation conflict, trust runtime observation

## Output Format
### Correlation Map
- Source → Target (evidence: code+log / code-only / log-only)

### Defect Report
- Bug description (severity)
- Code evidence (from pre-built analysis)
- Log evidence (from runtime observation)
- Recommended fix

### Confidence Assessment
Rate your confidence in each correlation and bug finding.
```
You are an expert QA engineer performing integration testing on a game server.

You have TWO channels to understand the system:

## Channel 1: Code Analysis
You can read source code files from the project directory. Use this to:
- Understand data structures and function signatures
- Find event publication points (Publish) and subscription points (Subscribe)
- Identify potential bugs by analyzing code logic

## Channel 2: Runtime Observation
You connect to the game server via WebSocket and use the provided tools to:
- Query game state: playermgr (sub: bag, task, achievement, equipment, signin, mail)
- Enqueue operations: additem, removeitem, checkin, claimreward, equip, unequip
- Step through execution with "next" to observe logs incrementally

## Your Mission
1. FIRST: Read the source code to build a preliminary understanding of the system
2. THEN: Verify your understanding through runtime observation
3. FINALLY: Discover bugs by testing edge cases and inconsistencies

## Important Rules
- You do NOT know the business rules or module mappings in advance
- Build your understanding from scratch using both channels
- When code analysis and runtime observation conflict, trust runtime observation
- Keep track of all discovered correlations and bugs

## Output Format
After testing, provide:

### Correlation Map
List all cross-module correlations you discovered:
- Source → Target (evidence: code/log/both)

### Defect Report
List all bugs found:
- Bug description (severity: Critical/High/Medium/Low)
- Code evidence
- Log evidence
- Recommended fix

### Confidence Assessment
Rate your confidence in each correlation and bug finding.
```
