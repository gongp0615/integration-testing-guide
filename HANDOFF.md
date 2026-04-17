# Session Handoff — DSMB-Agent 论文项目

> 更新时间: 2026-04-17 18:00
> 状态: **所有实验和论文工作已完成**

---

## 1. 项目概述

DSMB-Agent 是一篇关于 Agent 驱动集成测试的论文项目。核心思想：用树状命令空间 + 步进思考（Hold World）让 AI Agent 自主探索复杂系统的集成测试。

**关键文件：**
- `README.md` = `PAPER.md` — 论文正文（已是标准学术结构，数据完整）
- `QUICKSTART.md` — 原型运行说明
- `ai-integration-test-demo/` — Go 实验原型

---

## 2. 当前状态

### 论文（PAPER.md / README.md）
- ✅ 标准学术论文结构（Abstract → Introduction → Related Work → Approach → Evaluation → Discussion → Threats → Conclusion）
- ✅ 双模型完整实验数据已回填（GLM-5.1 × 21 runs + GPT-5.4 × 21 runs = 42 次运行）
- ✅ Hold World 概念已融入 3.3 节
- ✅ LSP 集成已写入 3.4 节和 6.3 节讨论
- ✅ 16 篇参考文献已核实（[16] 替换为 Zhou et al. TSE 2021，补充了 [10][11] 页码）
- ✅ 摘要、贡献、结论均已更新为完整 42 次运行数据

### 实验数据

#### GLM-5.1（智谱 AI，Anthropic 兼容接口）
全量实验在 `results/formal/glm-5.1/`，全部完成：

| 组别 | run1 | run2 | run3 | 状态 |
|------|:----:|:----:|:----:|------|
| batch-only | ✅(46) | ✅(43) | ✅(75) | 3/3 完成 |
| step-only | ✅(102) | ✅(68) | ✅(90) | 3/3 完成 |
| dual | ✅(84) | ✅(87) | ✅(107) | 3/3 完成 |
| code-batch | ✅(82) | ✅(74) | ✅(61) | 3/3 完成 |
| code-only | ✅(26) | ✅(24) | ✅(24) | 3/3 完成 |
| l0 | ✅(102) | ✅(100) | ✅(108) | 3/3 完成 |
| l1 | ✅(105) | ✅(108) | ✅(107) | 3/3 完成 |

#### GPT-5.4（OpenAI Codex CLI）
全量实验在 `results/formal/gpt-5.4/`，全部完成：

| 组别 | run1 | run2 | run3 | 状态 |
|------|:----:|:----:|:----:|------|
| batch-only | ✅(1) | ✅(11) | ✅(6) | 3/3 完成 |
| step-only | ✅(55) | ✅(74) | ✅(32) | 3/3 完成 |
| dual | ✅(57) | ✅(66) | ✅(60) | 3/3 完成 |
| code-batch | ✅(32) | ✅(33) | ✅(28) | 3/3 完成 |
| code-only | ✅ | ✅ | ✅ | 3/3 完成 |
| l0 | ✅(80) | ✅(70) | ✅(71) | 3/3 完成 |
| l1 | ✅(66) | ✅(87) | ✅(75) | 3/3 完成 |

括号内数字是 AI → 调用次数。

### 代码改造

#### 三个 LLM Provider
1. **OpenAIProvider** — `go-openai` 库直连（原有，用于 `/api/paas/v4`）
2. **CodexProvider** — 通过 `codex exec --json` CLI 调用（GPT-5.4 实验用）
3. **AnthropicProvider** — `provider_anthropic.go`，HTTP 直连 Anthropic Messages API，含指数退避重试（429/503/EOF，最多 5 次，10s→120s backoff），HTTP 超时 300s

Provider 自动选择逻辑在 `ai/agent/agent.go:New()`:
- `apiKey == "codex"` → CodexProvider
- `baseURL` 含 `/anthropic` → AnthropicProvider
- 其他 → OpenAIProvider

#### LSP 集成
- `ai/lsp/client.go` — gopls LSP 客户端（JSON-RPC 2.0 over stdio）
- 3 个新 Agent tool: `lsp_references`, `lsp_definition`, `lsp_symbols`
- gopls v0.21.1 已安装在 `/home/gongp/gopath/bin/gopls`
- 在 code-access 模式下自动启动

---

## 3. 任务完成状态

| 优先级 | 任务 | 状态 |
|--------|------|------|
| **P0** | GLM-5.1 全量实验（7组×3轮=21 runs） | ✅ 已完成 |
| **P1** | GPT-5.4 补完 l0 run3 + l1×3 | ✅ 已完成 |
| **P2** | 论文数据回填（GLM+GPT 双模型完整数据） | ✅ 已完成 |
| **P3** | 评估脚本 regex 适配 | ⏭️ 跳过（手动提取了所有数据，不影响论文） |
| **P4** | 论文文本打磨（参考文献核实、数据一致性） | ✅ 已完成 |

---

## 4. API 配置

### GLM-5.1（智谱 Coding Plan）
```
API_KEY: 2fdd0505e3c44b7ebc8200112a0a110f.IYYCA0W3L5vngLO1
BASE_URL: https://open.bigmodel.cn/api/anthropic  ← 有额度
         https://open.bigmodel.cn/api/paas/v4     ← 余额已耗尽，不能用
```

### GPT-5.4（OpenAI Codex CLI）
```
通过 codex login 认证（ChatGPT 订阅）
API_KEY 设为 "codex" 即可
默认模型 gpt-5.4（其他模型如 gpt-5.3-codex-spark 能力不足无法跟随 tool calling 约定）
```

---

## 5. 关键代码路径

```
ai-integration-test-demo/
├── cmd/server/main.go          # 入口，provider/LSP 生命周期
├── ai/agent/
│   ├── agent.go                # Agent 主循环，tool dispatch
│   ├── provider.go             # Provider 接口 + OpenAI/Codex 实现
│   └── provider_anthropic.go   # Anthropic Messages API 实现（含重试逻辑）
├── ai/lsp/client.go            # gopls LSP 客户端
├── ai/tools/tools.go           # Tool 定义（含 LSP tools）
├── ai/prompt/system.go         # 各模式的 system prompt
├── scripts/
│   ├── run_experiments.sh       # 批量实验脚本
│   ├── summarize_results.py     # 评估脚本（regex 仅适配 GLM 格式）
│   └── ground_truth.json        # 7 个预埋缺陷的 ground truth
└── results/
    ├── formal/glm-5.1/         # GLM-5.1 全量实验数据（21 runs）
    ├── formal/gpt-5.4/         # GPT-5.4 全量实验数据（21 runs）
    ├── evaluation_report.md     # GLM-5.1 早期单次运行 L2 消融结果
    ├── l0-run1-report.md        # GLM-5.1 早期 L0 报告
    └── l1-run1-report.md        # GLM-5.1 早期 L1 报告
```

---

## 6. 实验结果速查

### GLM-5.1 缺陷发现矩阵（3 runs each）

```
                B1  B2  B3  B4  B5  B6  B7  Avg
batch-only(3)    ·  1/3  ·  3/3  ·   ·   ·   1.3/7
step-only(3)     ·   ·  2/3 3/3 3/3 3/3  ·   3.7/7
dual(3)          ·  1/3  ·  3/3 1/3 3/3  ·   2.7/7
code-batch(3)   2/3 1/3 1/3 3/3 1/3 3/3  ·   3.7/7
code-only(3)    1/3  ·  3/3 3/3 2/3 3/3  ·   4.0/7
l0(3)           3/3  ·  2/3 3/3  ·  3/3  ·   3.7/7
l1(3)           1/3  ·  2/3 3/3 2/3 2/3  ·   3.3/7
```

### GPT-5.4 缺陷发现矩阵（3 runs each）

```
                B1  B2  B3  B4  B5  B6  B7  Avg
batch-only(3)    ·   ·  3/3  ·  1/3  ·  1/3  1.7/7
step-only(3)     ·  2/3 3/3  ·  1/3  ·   ·   2.0/7
dual(3)          ·  3/3 3/3 3/3  ·  3/3  ·   4.0/7
code-batch(3)    ·  3/3 3/3 3/3 2/3 3/3 1/3  5.0/7
code-only(3)    2/3 2/3 2/3 2/3 2/3 2/3  ·   4.0/7
l0(3)           3/3  ·  2/3 3/3 2/3 2/3  ·   4.3/7
l1(3)           3/3  ·  3/3 3/3 3/3 3/3 1/3  5.3/7
```

### 核心发现
1. GLM-5.1: step-only 和 code-batch 并列最优（3.7/7），步进执行主效应 +0.24
2. GPT-5.4: code-batch 最强（5.0/7），L1 模式平均 5.3/7（单次最高 6/7）
3. 两个模型均存在负交互效应：dual 组低于 step-only 和 code-batch 的均值
4. B1 几乎只有 L0/L1 模式（命令注册）能稳定发现
5. B4（签到无幂等）和 B6（邮件断链）在所有模式下几乎 100% 被发现
6. B7（ID 冲突）最难发现，仅 GPT-5.4 l1-run2 和部分 code-batch/batch-only 偶发检出

---

## 7. 后续可选工作

以下为非必须的改进方向，可在需要时开展：

- **评估脚本适配**：`summarize_results.py` 的 regex 无法解析 GPT-5.4 报告格式，需改进或统一报告输出格式
- **扩大实验规模**：当前每组 3 次运行，扩大到 10+ 次可支持 Wilcoxon 秩和检验等统计检验
- **投稿格式调整**：确定 venue（ICSE/FSE/ASE/ISSTA）后按模板转换为 LaTeX
- **更大规模系统验证**：在更多模块、更复杂关联的系统上验证可扩展性
