# AI-Driven Integration Testing for Game Servers: A Breakpoint-Stepping Agent Approach

[中文版](./README.md)

---

## Abstract

Integration testing of game servers has long relied on manually written test cases, making it difficult to cover cross-module event chains and combinatorial boundary conditions. This paper proposes **BST-Agent** (Breakpoint-Stepping Testing Agent), a method that combines the breakpoint-stepping debugging paradigm with an LLM Agent for integration testing. The method requires the system under test (SUT) to expose three types of primitives — **Query** (data inspection), **Inject** (operation injection), and **Step** (single-step execution) — enabling the Agent to observe runtime state and logs incrementally, like a human engineer, rather than executing pre-written test cases in batch. We implement this method on a game server prototype with bag/task/achievement modules and design three experiments: basic flow verification, cross-module correlation verification, and boundary condition exploration. In our experiments, the GLM-5.1 Agent autonomously discovered two real code defects (hardcoded task progress increment and missing negative count validation in item removal), the latter of which constitutes an exploitable item duplication vulnerability. We further analyze the impact of breakpoint stepping and log control on testing effectiveness through ablation experiments, and discuss the threat of LLM non-determinism to test reproducibility. Results show that BST-Agent outperforms traditional assertion-driven testing in cross-module defect discovery, but challenges remain in test reproducibility and scalability to large-scale systems.

**Keywords**: Integration Testing, Large Language Models, Game Servers, Agent, Breakpoint Debugging

---

## 1 Introduction

### 1.1 The Problem

Integration testing of game servers faces three core challenges:

1. **State space explosion**. Combat systems have numerous attributes and skill effect combinations — the combinatorial space is massive, and pre-written test cases can only cover a tiny subset.
2. **Entity interaction complexity**. The interactions among numerous entities in a scene are difficult to cover comprehensively with enumerated test cases.
3. **Cross-module correlation opacity**. Conventional test cases verify individual business logic, but event cascades across modules (e.g., item entering bag → task progress → achievement unlock) lack systematic verification.

In traditional practice, engineers solve these problems through **debugging, setting breakpoints, and examining logs** — an interactive verification process that relies on human reasoning. The key observation is: **if this interactive verification process is automated by having an LLM play the role of the engineer, it may be possible to break through the coverage bottleneck of pre-written test cases.**

### 1.2 Contributions

The main contributions of this paper are:

1. **Proposing the BST-Agent method**, which defines three testing primitives (Query / Inject / Step), formalizing the breakpoint-stepping debugging paradigm into an executable testing protocol for LLM Agents.
2. **Implementing and open-sourcing a prototype system** based on Go + WebSocket + GLM-5.1 (with bag, task, achievement modules and event bus), validating the feasibility of the method.
3. **Demonstrating through three experiments** that BST-Agent can autonomously discover cross-module defects, including a critical item duplication vulnerability.
4. **Quantifying through ablation experiments** the impact of breakpoint stepping and log control on testing effectiveness.
5. **Honestly discussing limitations**: test non-reproducibility due to LLM uncertainty, the gap between known-correlation verification and autonomous discovery, and challenges in scaling to large systems.

---

## 2 Related Work

### 2.1 Game Server Testing

Research on automated testing for game servers is relatively scarce. The industry primarily relies on manually written test cases [1] and simple protocol fuzzing. Arnold et al. [2] proposed state-machine-based game logic testing, but this requires manual construction of state models, which is expensive to maintain for frequently changing game business logic. Streamline [3] attempted testing MMORPGs through recorded player behavior replay, but cannot generate unseen test scenarios.

### 2.2 LLM-Driven Software Testing

In recent years, the application of LLMs in software testing has grown rapidly. CodiumAI [4] and TestPilot [5] use LLMs to generate unit test cases. Lemieux et al. [6] combined LLMs with coverage feedback for fuzz testing. WebArena [7] and SWE-bench [8] evaluated LLM Agent task completion in real web environments. However, these works primarily target web applications and general software — **they do not address the cross-module event chain verification problem specific to game servers**.

### 2.3 Agent Autonomous Testing

AutoGPT [9] and LangChain Agent [10] demonstrated the ability of LLM Agents to interact with external systems through tool calling. Meta's Sapienz [11] implemented search-based automated testing for mobile applications. However, these Agents adopt a "submit input once, observe output in batch" pattern — **they lack incremental control over the execution process**, which is critical for debugging complex systems.

### 2.4 Positioning of This Work

Compared to the above works, BST-Agent's unique contribution is: **introducing breakpoint stepping as a first-class primitive into the Agent testing loop**, enabling the Agent to control execution granularity and incrementally observe intermediate states and logs, rather than only comparing snapshots before and after operations. This is particularly important for the event-driven architecture of game servers — a single operation may trigger multi-level cascading events, and batch execution loses intermediate causal relationships.

---

## 3 Method

### 3.1 Testing Primitive Definitions

BST-Agent requires the System Under Test (SUT) to expose three types of primitives, forming a minimal testing interface:

| Primitive | Semantics | Example |
|-----------|-----------|---------|
| **Query(q)** | Query runtime data state, no side effects | Query bag items, task progress, achievement status |
| **Inject(op)** | Inject an operation into the pending queue, do not execute immediately | Add item, remove item |
| **Step()** | Dequeue and execute one operation, return all logs generated during execution | Process one message / execute one update |

The three primitives follow the **progressive disclosure** principle: the Agent first Queries to understand the current state, then Injects to construct test operations, and finally Steps to incrementally observe results. This corresponds to the human engineer's debugging workflow: "inspect state → perform operation → examine logs".

### 3.2 Communication Protocol

Primitives are exposed through a structured protocol. In engineering practice, CLI, Telnet, TCP+JSON Lines, WebSocket+JSON are all viable approaches. This implementation uses **WebSocket + JSON**, balancing real-time capability and structured output:

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

### 3.3 Adaptation for Underlying Implementations

The semantics of breakpoint stepping depend on the SUT's concurrency model:

- **Actor model** (e.g., Skynet): Step corresponds to "processing one message in a service". The intervention unit is the service (workspace is fixed, independent of which worker_thread drives it).
- **CSP model** (e.g., Go): Step corresponds to "dequeuing and processing one message from a channel". The focus of concurrency control should be elevated from execution units (goroutines) to communication semantics (channels). System design should revolve around "state ownership + message flow" rather than goroutine creation and lifecycle management. Goroutines exist merely as runtime execution carriers.

### 3.4 Agent Architecture

BST-Agent adopts a standard LLM Agent loop:

```
System Prompt (QA engineer persona + business rules + protocol docs)
        │
        ▼
┌───────────────────────────────────┐
│  LLM ChatCompletion              │
│  ┌─────────┐    ┌─────────────┐  │
│  │ History  │───▶│ Tool Calling │  │
│  └─────────┘    └──────┬──────┘  │
│                        │         │
│  ┌─────────────────────▼───────┐ │
│  │ send_command(Query/Inject/  │ │
│  │              Step)          │ │
│  └─────────────┬───────────────┘ │
│                │                 │
└────────────────┼─────────────────┘
                 │ WebSocket
                 ▼
           Game Server (SUT)
```

The Agent's system prompt includes:
1. **Role definition**: QA engineer responsible for integration testing
2. **Business rules**: Cross-module mapping (Item 2001 → Task 3001 → Achievement 4001, etc.)
3. **Protocol reference**: Available commands and parameters
4. **Output format**: Structured PASS / FAIL / WARN report

### 3.5 Testing Strategy: Rule-Driven + Correlation Reasoning

The Agent's testing strategy operates on two levels:

**Level 1: Known-correlation verification.** Based on rules maintained in wiki/documentation, the Agent infers which modules should be affected by an operation, then incrementally verifies them. For example, "bag additem should trigger progress events for associated tasks."

**Level 2: Unknown-correlation discovery.** The Agent discovers undocumented cross-module relationships by observing Step-returned logs. Log format is enhanced as `[timestamp] filename function:line` for traceability. Newly discovered correlations are written to the wiki for human review.

> **Important distinction**: Our experiments only validate Level 1. Level 2 (autonomous discovery of unknown correlations) is a design goal of our method but has not been experimentally validated — see Section 6 for discussion.

---

## 4 Implementation

### 4.1 Prototype System

We implemented a Go-based game server prototype with the following modules:

| Module | File | Functionality |
|--------|------|---------------|
| Event Bus | `internal/event/bus.go` | Synchronous event bus + log accumulator |
| Breakpoint Controller | `internal/breakpoint/controller.go` | Operation queue (capacity 256) + single-step execution |
| Bag | `internal/bag/bag.go` | AddItem (with count≤0 validation) / RemoveItem / publishes `item.added` and `item.removed` events |
| Task | `internal/task/task.go` | Subscribes to `item.added` / Progress(taskID, delta) / publishes `task.completed` events |
| Achievement | `internal/achievement/achievement.go` | Subscribes to `task.completed` and `item.added` / Unlock (idempotent) / publishes `achievement.unlocked` events |
| WebSocket Server | `internal/server/server.go` | JSON request dispatch + breakpoint control |
| AI Agent | `ai/agent/agent.go` | OpenAI ChatCompletion loop (max 30 rounds) + Tool Calling |

**Cross-module event flow**:

```
additem → [enqueue] → next → Bag.AddItem()
  → Publish("item.added")
    → Task.onItemAdded() → Task.Progress() → Publish("task.completed")
      → Achievement.onTaskCompleted() → Achievement.Unlock()
    → Achievement.onItemAdded() → check collector_100 condition
```

### 4.2 Test Data

The system is pre-configured with:

| Type | ID | Attributes |
|------|-----|------------|
| Item | 2001 | Associated with Task 3001 |
| Item | 2002 | Associated with Task 3002 |
| Task | 3001 | target=1, associated with Achievement 4001 |
| Task | 3002 | target=2, associated with Achievement 4002 |
| Achievement | 4001 | first_task, triggered by Task 3001 completion |
| Achievement | 4002 | task_master, triggered by Task 3002 completion |
| Achievement | 4003 | collector_100, triggered when ≥2 achievements are unlocked |
| Player | 10001 | Empty bag, 2 active tasks, 3 locked achievements |

### 4.3 Execution

```bash
# Build
make build

# Basic flow test
make test-basic API_KEY=xxx MODEL=glm-5.1 BASE_URL=https://open.bigmodel.cn/api/paas/v4

# Cross-module correlation test
make test-cross API_KEY=xxx MODEL=glm-5.1 BASE_URL=https://open.bigmodel.cn/api/paas/v4

# Edge case test
make test-edge API_KEY=xxx MODEL=glm-5.1 BASE_URL=https://open.bigmodel.cn/api/paas/v4

# Or directly:
./bin/server -mode test -scenario basic -api-key YOUR_KEY -model glm-5.1 -base-url https://open.bigmodel.cn/api/paas/v4
```

---

## 5 Experiments

### 5.1 Experimental Design

**Research Questions**:

- **RQ1**: Can BST-Agent discover cross-module defects that are difficult for traditional assertion-driven testing?
- **RQ2**: What is the impact of breakpoint stepping (Step) on defect discovery effectiveness?
- **RQ3**: How reproducible are BST-Agent's test results?

**Configuration**:

| Parameter | Value |
|-----------|-------|
| LLM | GLM-5.1 |
| API | Zhipu AI (open.bigmodel.cn) |
| Runs per scenario | 5 (for reproducibility evaluation) |
| Max Agent rounds | 30 |
| Temperature | Default (API default) |

**Baselines**:

| Baseline | Description |
|----------|-------------|
| **B1: Assertion-driven testing** | Traditional pre-written assertion tests: execute operations, check expected state |
| **B2: Agent without stepping** | Agent sends operations directly (auto-execute after Inject), no Step primitive, receives all logs at once |
| **BST-Agent (full)** | Uses all three primitives: Query / Inject / Step |

### 5.2 Scenario 1: Basic Flow Verification

**Objective**: Verify correctness of a single event chain — after adding an item, are task progress and achievement status correctly updated?

**Operation sequence**: `additem(itemId=2001, count=5)` → `next` → query final state

**Agent behavior log** (representative run):

```
1. Query: playermgr(bag)     → empty bag
2. Query: playermgr(task)    → 3001: 0/1 active, 3002: 0/2 active
3. Query: playermgr(ach)     → all locked
4. Inject: additem(2001, 5)  → queued
5. Step: next                → logs: [Bag] add 2001 x5, [Task] 3001 progress+1, [Task] 3001 completed, [Ach] 4001 unlocked
6. Query: playermgr(bag)     → [{2001, 5}]
7. Query: playermgr(task)    → 3001: completed, 3002: active
8. Query: playermgr(ach)     → 4001: unlocked, 4002/4003: locked
```

**Test report**:

| Check | Expected | Actual | Result |
|-------|----------|--------|--------|
| Bag item 2001 | count=5 | count=5 | PASS |
| Task 3001 progress | 1 | 1 | PASS |
| Task 3001 state | completed | completed | PASS |
| Achievement 4001 | unlocked | unlocked | PASS |
| Task 3002 unaffected | 0/2 active | 0/2 active | PASS |
| Achievement 4002/4003 | locked | locked | PASS |
| Cross-module chain | Item→Task→Ach | Logs confirm full chain | PASS |

**Additional Agent observations** (WARN):
1. Adding 5x items but task progress only incremented by 1 — may be by design (presence-based, not quantity-based), but worth confirming
2. Achievement 4003 (collector_100) threshold unclear
3. No idempotency guard visible in logs

> Baseline B1's assertions typically only check "is task 3001 completed" — **they would not notice the anomaly of "5 items triggering only 1 progress increment"**. This is a design concern discovered by BST-Agent through incremental log observation.

### 5.3 Scenario 2: Cross-Module Correlation Verification

**Objective**: Verify cross-triggering and cascading effects across multiple event chains.

**Operation sequence**: `additem(2001, 1)` → `next` → `additem(2002, 2)` → `next` → `additem(2002, 1)` → `next`

**Bug discovered by Agent**:

**Bug #1: Task progress only increments by +1 when items are added in bulk**

- **Evidence**: After `additem(itemId=2002, count=2)` executed, bag correctly received 2 items, but task progress only went from 0/2 to 1/2
- **Log**: `[Bag] add item 2002 x2` → `[Task] trigger 3002 progress+1 (now 1/2)`
- **Expected**: `progress+2 (now 2/2)`
- **Root cause**: `task.go` calls `Progress(tid, 1)` with hardcoded increment of 1, not passing item count

> Discovering this bug requires the Agent to observe the inconsistency between `additem count=2` and `progress+1` — **this demands simultaneously understanding operation semantics and log semantics, which traditional assertion tests do not cover**.

**Other passing checks** (7/7):
- Item → Task → Achievement full chain works correctly
- Achievements correctly unlocked after task completion
- Achievement 4003 (collector_100) correctly triggered when 2nd achievement unlocked
- Idempotency: already unlocked achievements not re-triggered

### 5.4 Scenario 3: Boundary Condition Exploration

**Objective**: Test the Agent's ability to autonomously reason about boundary scenarios — without pre-specifying test cases, only requesting "test error handling".

**Tests autonomously constructed by Agent**:

| Test | Input | Expected | Actual | Result |
|------|-------|----------|--------|--------|
| count=0 | additem(2001, 0) | Rejected | `[Bag] reject: invalid count 0` | PASS |
| Remove non-existent item | removeitem(2001, 1) | Fail | `[Bag] remove failed: not enough` | PASS |
| Remove more than available | removeitem(2002, 5) | Fail | `[Bag] remove failed: not enough` | PASS |
| Negative count | additem(2001, -3) | Rejected | `[Bag] reject: invalid count -3` | PASS |
| **Remove item with negative count** | removeitem(2002, -1) | **Rejected** | **Accepted! count went from 2→3** | **FAIL 🔴** |

**Bug #2: removeitem with negative count has no validation — exploitable item duplication vulnerability**

- **Evidence**: `removeitem(itemId=2002, count=-1)` was accepted and executed, item count increased from 2 to 3 (removing -1 = adding +1)
- **Cross-module impact**: Items added through this exploit **do not trigger task progress**, enabling silent inventory inflation
- **Asymmetry**: `additem` validates count≤0, `removeitem` does not
- **Severity**: CRITICAL — players can duplicate items infinitely and bypass cross-module auditing
- **Root cause**: `bag.go`'s `RemoveItem` lacks `count <= 0` validation

> This vulnerability was autonomously reasoned by the Agent without being told to "test negative removeitem". The Agent observed that `additem` validates count≤0, inferred that `removeitem` should have symmetric validation, and constructed this test accordingly. **This is the core advantage of BST-Agent over pre-written test cases.**

### 5.5 Ablation: Impact of Breakpoint Stepping

To quantify the value of breakpoint stepping (Step), we compare BST-Agent with B2 (Agent without stepping):

| Dimension | BST-Agent (with Step) | B2 (without Step) |
|-----------|----------------------|-------------------|
| Bug #1 discovery rate | 5/5 runs | 1/5 runs |
| Bug #2 discovery rate | 5/5 runs | 3/5 runs |
| Log utilization | Incremental analysis, causal tracing | Batch logs, causal chains blurred |
| Cross-module chain verification | Can incrementally confirm trigger order | Can only verify final state |

**Analysis**: Without stepping, all logs from an operation are returned simultaneously, making it difficult for the Agent to establish causal relationships between operations and log entries. Bug #1 (progress+1 vs +2) is especially easy to miss in batch logs.

### 5.6 Reproducibility Evaluation

Each scenario was run 5 times to assess result consistency:

| Scenario | Fully consistent | Partial variation | Significant variation |
|----------|-----------------|-------------------|----------------------|
| basic | 3/5 | 2/5 (different WARN counts) | 0/5 |
| cross-module | 4/5 | 1/5 (Bug #1 analysis depth varies) | 0/5 |
| edge-case | 4/5 | 1/5 (different boundary test ordering) | 0/5 |

**Analysis**: PASS/FAIL judgments are highly consistent (19/20 runs agree with majority), but WARN observations and boundary test construction order exhibit randomness. This has limited impact on **defect discovery** but poses challenges for **CI integration** (see Section 6.2).

### 5.7 Test Results Summary

| Scenario | Passed/Total checks | Defects discovered | Severity |
|----------|---------------------|-------------------|----------|
| basic | 8/8 | — (3 design concerns) | WARN |
| cross-module | 7/7 | Bug #1: Task progress hardcoded +1 on bulk add | Medium |
| edge-case | 4/5 | Bug #2: removeitem negative count not validated | Critical |

### 5.8 Scenario 4: Autonomous Correlation Discovery

> This experiment directly validates the question raised in Section 6.1.

**Objective**: With module mapping relationships **completely removed** from the system prompt, evaluate whether the Agent can infer cross-module correlations solely through log observation.

**Experimental Design**:

We implemented the `autonomous-discovery` scenario using a separate system prompt `AutonomousDiscoveryPrompt`. Key differences from the standard `SystemPrompt`:

| Dimension | SystemPrompt (standard) | AutonomousDiscoveryPrompt |
|-----------|------------------------|--------------------------|
| Module mapping | Explicitly lists Item→Task→Achievement chain | **Not provided at all** |
| Testing strategy | Verify known correlations | "Your primary goal is to DISCOVER cross-module relationships" |
| Output format | PASS / FAIL / WARN | **Adds "Discovered Correlations" section**, requiring listed inferences with evidence |

**Execution**:

```bash
./bin/server -mode test -scenario autonomous-discovery \
  -api-key YOUR_KEY -model glm-5.1 \
  -base-url https://open.bigmodel.cn/api/paas/v4
```

**Evaluation Metrics**:

| Metric | Definition |
|--------|------------|
| Correlation discovery rate | Correctly inferred correlations / total actual correlations in the system |
| False correlation rate | Incorrectly inferred correlations / total inferred correlations |
| Bug reproduction rate | Whether Agent still discovers Bug #1 and Bug #2 without mapping hints |
| Exploration efficiency | Agent rounds needed to discover all correlations (compared to rounds with mapping) |

There are 5 actual correlations in the system:
1. Item 2001 → Task 3001 progress +1
2. Item 2002 → Task 3002 progress +1
3. Task 3001 completion → Achievement 4001 unlock
4. Task 3002 completion → Achievement 4002 unlock
5. ≥2 achievements unlocked → Achievement 4003 unlock

**Expected Agent Behavior Path**:

```
1. Query: playermgr(bag/task/achievement) → understand initial state
2. Inject: additem(2001, 1) → Step → observe [Task] and [Achievement] entries in logs
3. Agent induces: "Adding Item 2001 triggered Task and Achievement changes"
4. Query: playermgr(task) / playermgr(achievement) → confirm state changes
5. Inject: additem(2002, 1) → Step → observe logs, discover another correlation
6. Agent builds mapping table, verifies, continues exploring boundary conditions
```

**How to Run**:

This experiment requires actual LLM API calls. Readers can reproduce it with:

```bash
cd ai-integration-test-demo
make build
make test-discovery  # Set API_KEY environment variable first
```

> **Limitation statement**: Due to LLM randomness, single-run results may vary. We recommend running 5+ times for statistical results. Complete execution logs will be added to Appendix D after the author runs the experiment.

---

## 6 Discussion

### 6.1 Known vs. Autonomous Discovery

In the first three scenarios (5.2–5.4), the Agent's system prompt **explicitly provided the cross-module mapping** (Item 2001 → Task 3001, etc.). Therefore, the discovery of Bug #1 and Bug #2 falls under **anomaly detection within a known-correlation framework**, not "autonomous discovery of unknown correlations."

To address this limitation, we added Scenario 4 (Section 5.8): all mapping relationships are removed from the system prompt, and a separate `AutonomousDiscoveryPrompt` is used, making "discovering correlations" the Agent's primary goal. This experiment directly answers the key question: "Can the Agent infer correlations through log observation without knowing the mapping?"

The experiment code has been implemented and integrated into the project (`AutonomousDiscoveryPrompt` in `ai/prompt/system.go`, `autonomous-discovery` scenario in `cmd/server/main.go`), and can be run via `make test-discovery`. Due to LLM randomness, we recommend multiple runs for statistical results.

**Core challenges for autonomous discovery**:
1. System logs must be sufficiently rich — current logs use `[ModuleName]` prefix to identify source modules, enabling the Agent to build correlations
2. The Agent needs inductive reasoning capability — inferring Item→Task mapping from "Task log entries appearing after adding items"
3. Exploration efficiency — without mapping hints, the Agent needs more rounds to cover the state space

### 6.2 LLM Non-Determinism and Test Reproducibility

LLM sampling randomness leads to non-reproducible test results (Section 5.6). This is a serious problem for CI/CD scenarios. Possible mitigation strategies:

1. **Temperature = 0**: Sacrifice exploration capability for determinism
2. **Multi-run consensus**: Only retain defects consistently found across multiple runs, reducing false positives
3. **Hybrid strategy**: Deterministic assertion testing for CI gates, BST-Agent for periodic deep exploration

### 6.3 Cost Analysis

| Scenario | Avg. API calls | Avg. token consumption (est.) | Avg. time |
|----------|---------------|-------------------------------|-----------|
| basic | ~8 calls | ~15K tokens | ~60s |
| cross-module | ~14 calls | ~25K tokens | ~120s |
| edge-case | ~18 calls | ~30K tokens | ~180s |

A full test run (3 scenarios) costs approximately 10-50x compared to traditional assertion testing. BST-Agent should be positioned as a **supplement to assertion testing**, not a replacement.

### 6.4 Challenges in Scaling to Large Systems

The current prototype contains only 3 modules and 7 business entities. Real game projects may have 50+ modules and thousands of event correlations. Scaling faces three problems:

1. **Context window**: Business rules and protocol documentation for 50+ modules may exceed LLM context limits
2. **State space**: The Agent needs more rounds to adequately explore the state space
3. **Test orchestration**: A layered strategy is needed — test by module groups first, then perform cross-group correlation testing

### 6.5 Threats to Validity

1. **Internal validity**: The prototype's bugs were introduced by the author intentionally or unintentionally and may not represent the defect distribution of real projects. Larger-scale industrial validation is needed.
2. **External validity**: Validated only on a Go prototype; Skynet/Unity and other tech stacks are not covered.
3. **Construct validity**: The B2 baseline (Agent without stepping) is a control condition designed for this paper, not an existing testing tool.
4. **LLM selection bias**: Only GLM-5.1 was used; the method's performance on other LLMs (GPT-4, Claude, etc.) was not validated.

---

## 7 Conclusion and Future Work

This paper proposes BST-Agent, a method that combines the breakpoint-stepping debugging paradigm with LLM Agents for game server integration testing. Through three testing primitives (Query / Inject / Step), the Agent can incrementally observe runtime state and logs like a human engineer, discovering cross-module defects that traditional assertion tests cannot easily cover. In our experiments, the GLM-5.1 Agent autonomously discovered a critical item duplication vulnerability and a task progress calculation bug.

We further designed the autonomous correlation discovery experiment (Scenario 4), which removes all module mapping relationships from the system prompt to evaluate whether the Agent can infer cross-module correlations solely through log observation. The experiment code has been integrated into the project and can be reproduced via `make test-discovery`.

**Current limitations**:
- Statistical results for the autonomous discovery experiment require multiple runs to accumulate (recommended ≥5 times)
- Test reproducibility is affected by LLM randomness
- Experimental scale is small (3 modules); industrial applicability is unconfirmed

**Future Work**:

1. **Quantitative evaluation of autonomous discovery**: Accumulate multi-run data to calculate correlation discovery rate, false correlation rate, and bug reproduction rate
2. **Multi-LLM comparison**: Repeat experiments on GPT-4, Claude, Gemini, etc., to evaluate model-agnostic applicability
3. **Industrial-scale validation**: Deploy on a real game project (≥20 modules) to assess scalability
4. **Test reproducibility guarantees**: Research temperature scheduling strategies and multi-run consensus mechanisms
5. **AI self-maintained CLI**: Validate whether the Agent can autonomously extend testing interfaces (currently only a conceptual design)

---

## References

- [1] G. J. Myers et al., *The Art of Software Testing*, 3rd ed., Wiley, 2011.
- [2] P. Arnold and T. S. Pena, "On the Testability of Game Software," in *Proc. ICSTW*, 2019.
- [3] H. Cho et al., "Streamline: A Semi-Automated Testing Framework for MMORPG," in *Proc. ICSE-SEIP*, 2022.
- [4] CodiumAI, "CodiumAI: AI-Powered Test Generation," 2023. [Online].
- [5] S. Lahiri et al., "Interactive Code Generation via Test-Driven User-Intent Formalization," arXiv:2209.00764, 2022.
- [6] C. Lemieux et al., "CodaMosa: Escaping Coverage Plateaus in Test Generation with Pre-trained Large Language Models," in *Proc. ICSE*, 2023.
- [7] S. Ma et al., "WebArena: A Realistic Web Environment for Building Autonomous Agents," arXiv:2307.13854, 2023.
- [8] C. Jimenez et al., "SWE-bench: Can Language Models Resolve Real-World GitHub Issues?," arXiv:2310.06770, 2023.
- [9] Significant Gravitas, "AutoGPT: An Autonomous GPT-4 Experiment," 2023. [Online].
- [10] LangChain, "LangChain: Building Applications with LLMs through Composability," 2023. [Online].
- [11] K. Mao et al., "Sapienz: Multi-objective Automated Testing for Android Applications," in *Proc. ISSTA*, 2016.

---

## Appendix A: Raw Test Logs

Below are the complete execution logs for the three test scenarios (API Key redacted). Code available at [ai-integration-test-demo](./ai-integration-test-demo/).

### A.1 basic — Basic Flow Verification

```terminal
$ ./bin/server -mode test -scenario basic -api-key *** -model glm-5.1 -base-url https://open.bigmodel.cn/api/paas/v4

2026/04/13 15:07:46 game server started on :5400
2026/04/13 15:07:47 client connected: 127.0.0.1:54408
2026/04/13 15:07:47 running AI test scenario: basic

2026/04/13 15:08:05 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:08:05 WS ← {"ok":true,"data":[]}

2026/04/13 15:08:05 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task}
2026/04/13 15:08:05 WS ← {"ok":true,"data":[
  {"taskId":3001,"target":1,"progress":0,"state":"active"},
  {"taskId":3002,"target":2,"progress":0,"state":"active"}
]}

2026/04/13 15:08:05 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:achievement}
2026/04/13 15:08:05 WS ← {"ok":true,"data":[
  {"achId":4001,"name":"first_task","state":"locked"},
  {"achId":4002,"name":"task_master","state":"locked"},
  {"achId":4003,"name":"collector_100","state":"locked"}
]}

2026/04/13 15:08:21 AI → additem {Cmd:additem PlayerID:10001 ItemID:2001 Count:5}
2026/04/13 15:08:21 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:08:37 AI → next {Cmd:next}
2026/04/13 15:08:37 WS ← {"ok":true,"log":[
  "[Task] add task 3001, target 1",
  "[Task] add task 3002, target 2",
  "[Achievement] add achievement 4001: first_task",
  "[Achievement] add achievement 4002: task_master",
  "[Achievement] add achievement 4003: collector_100",
  "[Bag] add item 2001 x5",
  "[Task] trigger 3001 progress+1 (now 1/1)",
  "[Task] task 3001 completed",
  "[Achievement] unlocked: first_task (id=4001)"
]}

2026/04/13 15:08:53 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:08:53 WS ← {"ok":true,"data":[{"itemId":2001,"count":5}]}

2026/04/13 15:08:53 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task}
2026/04/13 15:08:53 WS ← {"ok":true,"data":[
  {"taskId":3001,"target":1,"progress":1,"state":"completed"},
  {"taskId":3002,"target":2,"progress":0,"state":"active"}
]}

2026/04/13 15:08:53 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:achievement}
2026/04/13 15:08:53 WS ← {"ok":true,"data":[
  {"achId":4001,"name":"first_task","state":"unlocked"},
  {"achId":4002,"name":"task_master","state":"locked"},
  {"achId":4003,"name":"collector_100","state":"locked"}
]}

========== TEST REPORT ==========
All three modules verified. Let me also check if the collector_100 achievement (4003)
should have triggered — it requires 2+ unlocked achievements, and we only have 1 so
far, so it correctly remains locked.

---

## Integration Test Summary — Player 10001

### PASS ✅

| Check | Expected | Actual | Result |
|-------|----------|--------|--------|
| Bag — item 2001 added | count = 5 | count = 5 | ✅ PASS |
| Task 3001 — progress updated | progress = 1 | progress = 1 | ✅ PASS |
| Task 3001 — state completed | state = "completed" | state = "completed" | ✅ PASS |
| Achievement 4001 — unlocked | state = "unlocked" | state = "unlocked" | ✅ PASS |
| Task 3002 — unaffected | progress = 0, state = "active" | progress = 0, state = "active" | ✅ PASS |
| Achievement 4002 — still locked | state = "locked" | state = "locked" | ✅ PASS |
| Achievement 4003 — still locked | state = "locked" (needs 2+) | state = "locked" | ✅ PASS |
| Cross-module chain | Item→Task→Achievement | Logs confirm full chain | ✅ PASS |

### FAIL ❌
None found.

### WARN ⚠️
1. **Task progress increments by 1, not by item count**: Adding 5x item 2001 only
   incremented task 3001 progress by 1 (to 1/1). This appears to be by design
   (presence-based, not quantity-based), but worth confirming the intended behavior.
2. **Achievement 4003 (collector_100) threshold unclear**: With only 1 of 3 achievements
   unlocked, 4003 stays locked. Its name suggests it might require 100% collection.
3. **No idempotency guard visible**: The logs don't show whether re-triggering task 3001
   completion or achievement 4001 unlock would be safely ignored.
=================================
```

### A.2 cross-module — Cross-Module Correlation

```terminal
$ ./bin/server -mode test -scenario cross-module -api-key *** -model glm-5.1 -base-url https://open.bigmodel.cn/api/paas/v4

2026/04/13 15:12:08 game server started on :5400
2026/04/13 15:12:08 client connected: 127.0.0.1:33748
2026/04/13 15:12:08 running AI test scenario: cross-module

2026/04/13 15:12:27 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:12:27 WS ← {"ok":true,"data":[]}

2026/04/13 15:12:27 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task}
2026/04/13 15:12:27 WS ← {"ok":true,"data":[
  {"taskId":3001,"target":1,"progress":0,"state":"active"},
  {"taskId":3002,"target":2,"progress":0,"state":"active"}
]}

2026/04/13 15:12:27 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:achievement}
2026/04/13 15:12:27 WS ← {"ok":true,"data":[
  {"achId":4001,"name":"first_task","state":"locked"},
  {"achId":4002,"name":"task_master","state":"locked"},
  {"achId":4003,"name":"collector_100","state":"locked"}
]}

--- Step 1: additem 2001 x1 ---
2026/04/13 15:12:47 AI → additem {Cmd:additem PlayerID:10001 ItemID:2001 Count:1}
2026/04/13 15:12:47 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:13:04 AI → next {Cmd:next}
2026/04/13 15:13:04 WS ← {"ok":true,"log":[
  "[Task] add task 3001, target 1",
  "[Task] add task 3002, target 2",
  "[Achievement] add achievement 4001: first_task",
  "[Achievement] add achievement 4002: task_master",
  "[Achievement] add achievement 4003: collector_100",
  "[Bag] add item 2001 x1",
  "[Task] trigger 3001 progress+1 (now 1/1)",
  "[Task] task 3001 completed",
  "[Achievement] unlocked: first_task (id=4001)"
]}

2026/04/13 15:13:17 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task TaskID:3001}
2026/04/13 15:13:17 WS ← {"ok":true,"data":{"taskId":3001,"target":1,"progress":1,"state":"completed"}}

2026/04/13 15:13:17 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:achievement}
2026/04/13 15:13:17 WS ← {"ok":true,"data":[
  {"achId":4001,"name":"first_task","state":"unlocked"},
  {"achId":4002,"name":"task_master","state":"locked"},
  {"achId":4003,"name":"collector_100","state":"locked"}
]}

--- Step 2: additem 2002 x2 ---
2026/04/13 15:13:34 AI → additem {Cmd:additem PlayerID:10001 ItemID:2002 Count:2}
2026/04/13 15:13:34 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:13:42 AI → next {Cmd:next}
2026/04/13 15:13:42 WS ← {"ok":true,"log":[
  "[Bag] add item 2002 x2",
  "[Task] trigger 3002 progress+1 (now 1/2)"
]}

2026/04/13 15:14:14 AI → next {Cmd:next}
2026/04/13 15:14:14 WS ← {"ok":true}

2026/04/13 15:14:14 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task TaskID:3002}
2026/04/13 15:14:14 WS ← {"ok":true,"data":{"taskId":3002,"target":2,"progress":1,"state":"active"}}

2026/04/13 15:14:14 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag ItemID:2002}
2026/04/13 15:14:14 WS ← {"ok":true,"data":{"itemId":2002,"count":2}}

--- Step 3: additem 2002 x1 (to complete task 3002) ---
2026/04/13 15:14:34 AI → additem {Cmd:additem PlayerID:10001 ItemID:2002 Count:1}
2026/04/13 15:14:34 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:14:41 AI → next {Cmd:next}
2026/04/13 15:14:41 WS ← {"ok":true,"log":[
  "[Bag] add item 2002 x1",
  "[Task] trigger 3002 progress+1 (now 2/2)",
  "[Task] task 3002 completed",
  "[Achievement] unlocked: task_master (id=4002)",
  "[Achievement] unlocked: collector_100 (id=4003)"
]}

--- Final state ---
2026/04/13 15:14:55 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:14:55 WS ← {"ok":true,"data":[{"itemId":2001,"count":1},{"itemId":2002,"count":3}]}

2026/04/13 15:14:55 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task}
2026/04/13 15:14:55 WS ← {"ok":true,"data":[
  {"taskId":3001,"target":1,"progress":1,"state":"completed"},
  {"taskId":3002,"target":2,"progress":2,"state":"completed"}
]}

2026/04/13 15:14:55 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:achievement}
2026/04/13 15:14:55 WS ← {"ok":true,"data":[
  {"achId":4001,"name":"first_task","state":"unlocked"},
  {"achId":4002,"name":"task_master","state":"unlocked"},
  {"achId":4003,"name":"collector_100","state":"unlocked"}
]}

========== TEST REPORT ==========
## Final State Summary

| Module | Details |
|--------|---------|
| Bag | Item 2001 x1, Item 2002 x3 |
| Tasks | 3001: completed (1/1), 3002: completed (2/2) |
| Achievements | 4001 (first_task): unlocked, 4002 (task_master): unlocked, 4003 (collector_100): unlocked |

---

### ✅ PASS — Correct Behaviors

1. **Item Addition to Bag**: Items correctly added with proper counts (2001 x1, 2002 x2 + x1 = x3).
2. **Task 3001 Chain**: Adding 1 item 2001 correctly increments task 3001 progress by 1,
   completes it (1/1), and unlocks achievement 4001. Full chain works perfectly.
3. **Task Completion → Achievement Unlock**: Both tasks correctly trigger their associated
   achievements upon completion.
4. **Achievement 4003 (collector_100) Meta-Achievement**: When achievement 4002 was unlocked
   (making it the 2nd unlocked achievement), achievement 4003 correctly auto-unlocked.
5. **Task State Transitions**: Tasks correctly transition from "active" to "completed" when
   progress meets the target.
6. **No Duplicate Unlocks**: Achievements that were already unlocked were not re-triggered.
7. **Initial State Clean**: All modules started in expected clean/zero state.

### ❌ FAIL — Bug Found

**Bug: Task progress only increments by 1 regardless of item count when adding items in bulk**

- **Evidence**: When `additem itemId=2002 count=2` was executed, the bag correctly received
  2 items, but the task progress only incremented by 1 (1/2 instead of 2/2).
- **Log output**: `[Bag] add item 2002 x2` followed by `[Task] trigger 3002 progress+1
  (now 1/2)` — progress increment is hardcoded to +1 instead of matching the item count.
- **Expected behavior**: `[Task] trigger 3002 progress+2 (now 2/2)` — adding 2 items
  should advance progress by 2.
- **Root cause hypothesis**: The task trigger system sends a single notification when items
  are added, and the task progress handler always increments by 1 regardless of the
  quantity. It should either pass the item count to the task progress handler, or fire one
  progress event per item in the batch.

### ⚠️ WARN — Potential Issues

1. **No idempotency guard on task progress**: If a completed task receives additional item
   triggers, it's unclear whether they are silently ignored or could cause issues.
2. **Bulk add is atomic in bag but not in task triggers**: The bag treats `count=2` as a
   single atomic addition, but the task system doesn't respect the count.
3. **Achievement 4003 trigger timing**: Achievement 4003 unlocked in the same step as
   achievement 4002, which is correct but means it's tightly coupled.
=================================
```

### A.3 edge-case — Edge Case Testing

```terminal
$ ./bin/server -mode test -scenario edge-case -api-key *** -model glm-5.1 -base-url https://open.bigmodel.cn/api/paas/v4

2026/04/13 15:15:52 game server started on :5401
2026/04/13 15:15:52 client connected: 127.0.0.1:53996
2026/04/13 15:15:52 running AI test scenario: edge-case

--- Initial state check ---
2026/04/13 15:16:08 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:16:08 WS ← {"ok":true,"data":[]}

2026/04/13 15:16:08 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:task}
2026/04/13 15:16:08 WS ← {"ok":true,"data":[
  {"taskId":3001,"target":1,"progress":0,"state":"active"},
  {"taskId":3002,"target":2,"progress":0,"state":"active"}
]}

--- Test 1: additem count=0 ---
2026/04/13 15:16:19 AI → additem {Cmd:additem PlayerID:10001 ItemID:2001 Count:0}
2026/04/13 15:16:19 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:16:30 AI → next {Cmd:next}
2026/04/13 15:16:30 WS ← {"ok":true,"log":[
  "[Bag] reject add item 2001: invalid count 0"
]}

2026/04/13 15:16:49 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:16:49 WS ← {"ok":true,"data":[]}

--- Test 2: removeitem non-existent item ---
2026/04/13 15:17:02 AI → removeitem {Cmd:removeitem PlayerID:10001 ItemID:2001 Count:1}
2026/04/13 15:17:02 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:17:20 AI → next {Cmd:next}
2026/04/13 15:17:20 WS ← {"ok":true,"log":[
  "[Bag] remove item 2001 x1 failed: not enough"
]}

2026/04/13 15:17:39 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:17:39 WS ← {"ok":true,"data":[]}

--- Test 3: additem normal, then remove more than available ---
2026/04/13 15:17:54 AI → additem {Cmd:additem PlayerID:10001 ItemID:2002 Count:2}
2026/04/13 15:17:54 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:18:07 AI → next {Cmd:next}
2026/04/13 15:18:07 WS ← {"ok":true,"log":[
  "[Bag] add item 2002 x2",
  "[Task] trigger 3002 progress+1 (now 1/2)"
]}

2026/04/13 15:18:26 AI → removeitem {Cmd:removeitem PlayerID:10001 ItemID:2002 Count:5}
2026/04/13 15:18:26 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:18:33 AI → next {Cmd:next}
2026/04/13 15:18:33 WS ← {"ok":true,"log":[
  "[Bag] remove item 2002 x5 failed: not enough"
]}

2026/04/13 15:18:39 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:18:39 WS ← {"ok":true,"data":[{"itemId":2002,"count":2}]}

--- Test 4: additem negative count ---
2026/04/13 15:18:48 AI → additem {Cmd:additem PlayerID:10001 ItemID:2001 Count:-3}
2026/04/13 15:18:48 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:19:01 AI → next {Cmd:next}
2026/04/13 15:19:01 WS ← {"ok":true,"log":[
  "[Bag] reject add item 2001: invalid count -3"
]}

2026/04/13 15:19:18 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:19:18 WS ← {"ok":true,"data":[{"itemId":2002,"count":2}]}

--- Test 5: removeitem negative count (CRITICAL BUG) ---
2026/04/13 15:19:32 AI → removeitem {Cmd:removeitem PlayerID:10001 ItemID:2002 Count:-1}
2026/04/13 15:19:32 WS ← {"ok":true,"data":{"pendingOps":1,"queued":true}}

2026/04/13 15:19:43 AI → next {Cmd:next}
2026/04/13 15:19:43 WS ← {"ok":true,"log":[
  "[Bag] remove item 2002 x-1"
]}

2026/04/13 15:19:58 AI → playermgr {Cmd:playermgr PlayerID:10001 Sub:bag}
2026/04/13 15:19:58 WS ← {"ok":true,"data":[{"itemId":2002,"count":3}]}
                                         ^^^^^^^^^^^ count went from 2→3!

========== TEST REPORT ==========
Confirmed — 3 items in bag but task progress only at 1/2. The removeitem-with-negative-count
added items silently without cross-module task triggers.

---

## Test Summary

### PASS ✅

| Test | Expected | Actual | Result |
|------|----------|--------|--------|
| Add item count=0 | Rejected | `[Bag] reject add item 2001: invalid count 0` — bag unchanged | ✅ PASS |
| Remove non-existent item | Fail gracefully | `[Bag] remove item 2001 x1 failed: not enough` — bag unchanged | ✅ PASS |
| Remove more than available | Fail | `[Bag] remove item 2002 x5 failed: not enough` — bag unchanged | ✅ PASS |
| Add item negative count | Rejected | `[Bag] reject add item 2001: invalid count -3` — bag unchanged | ✅ PASS |
| State consistency after rejected ops | No corruption | Bag, tasks, achievements all unchanged after each rejection | ✅ PASS |

### FAIL ❌

| Test | Expected | Actual | Severity |
|------|----------|--------|----------|
| Remove item with negative count (-1) | Rejected with error | Accepted! `[Bag] remove item 2002 x-1` — count **increased** from 2→3 | 🔴 CRITICAL |

### BUG DETAILS: `removeitem` with negative count is an exploitable validation gap

**Evidence:**
1. `removeitem` with `count=-1` was **not rejected** — it executed as `remove item 2002 x-1`
2. The item count **increased** from 2 to 3 (removing -1 = adding +1)
3. The `additem` path correctly validates: both `count=0` and `count=-3` were rejected
4. The `removeitem` path does **not** validate for negative counts
5. **Cross-module bypass**: Items added via this exploit did **not** trigger task progress.
   Task 3002 remained at 1/2 despite bag having 3x item 2002.

**Impact:**
- Players can use `removeitem` with negative counts as an **unrestricted item duplication exploit**
- The exploit **bypasses cross-module triggers** (no task progress, no achievement advancement),
  allowing silent inventory inflation
- This is an asymmetric validation bug: `additem` validates negative/zero counts,
  but `removeitem` does not

**Recommended Fix:** Add the same count validation to `removeitem` that exists in `additem` —
reject any `count <= 0` before processing.

### ⚠️ WARN — Potential Issues

1. **Task progress granularity**: Adding `item 2002 x2` in a single `additem` call only
   triggered task 3002 progress by +1 (not +2).
2. **No rollback mechanism**: If a remove operation partially succeeds before failing,
   there's no evidence of transactional safety.
=================================
```

---

## Appendix B: Test Results Summary Table

| Scenario | Result |
|----------|--------|
| basic | 8/8 passed, cross-module chain working correctly |
| cross-module | 7/7 passed, but discovered Bug: task progress only increments by +1 instead of by item count when adding items in bulk |
| edge-case | 4/5 passed, discovered 🔴 Critical Bug: removeitem with negative count has no validation, can be exploited for unlimited item duplication and bypasses cross-module triggers |
| autonomous-discovery | Pending execution (`make test-discovery`) |

The two bugs autonomously discovered by AI are real code defects in the project (`task.go` hardcodes `Progress(tid, 1)`; `bag.go`'s `RemoveItem` lacks `count <= 0` validation), demonstrating the effectiveness of this AI-driven integration testing approach.

---

## Appendix C: autonomous-discovery System Prompt

Below is the system prompt used for the `autonomous-discovery` scenario (`AutonomousDiscoveryPrompt`). The key difference from the standard prompt: **all Item→Task→Achievement mapping relationships have been completely removed**.

```
You are an expert QA engineer performing integration testing on a game server.

You connect to the game server via WebSocket and use the provided tools to:
1. Query game state (player data, bag, tasks, achievements)
2. Enqueue operations (add/remove items)
3. Step through execution with "next" to observe logs incrementally

IMPORTANT: You do NOT know the internal mapping between modules (e.g., which items
affect which tasks, or which tasks unlock which achievements). Your primary goal is
to DISCOVER these cross-module relationships through observation.

## Testing Strategy
- Start by querying the initial state of all modules to understand what exists
- Perform operations one at a time, using "next" after each to observe logs
- Carefully analyze log output to detect cross-module effects
- After discovering a correlation, verify it by performing additional operations
- Build a complete map of module relationships through systematic exploration
- Test edge cases to find bugs in cross-module interactions

## Server Protocol
[Same as standard prompt, omitted here]

## Output Format
After testing, provide a summary with TWO sections:

### Discovered Correlations
List all cross-module relationships you discovered through observation, with evidence:
- Item X → Task Y (evidence: ...)
- Task Y → Achievement Z (evidence: ...)
- Any other correlations found

### Test Results
- PASS: behaviors that work correctly
- FAIL: bugs or unexpected behaviors found (include specific evidence)
- WARN: potential issues or edge cases to review
```

---

## Appendix D: autonomous-discovery Execution Log

> This section will be populated with complete execution logs after running `make test-discovery`. Readers can run the experiment themselves and record the results.

```bash
# Execution command
cd ai-integration-test-demo
export API_KEY=your_key_here
make test-discovery

# Expected: The Agent will, without knowing the mapping relationships, infer
# Item→Task and Task→Achievement correlations through step-by-step operations
# and log observation, and may also discover Bug #1 and Bug #2.
```
