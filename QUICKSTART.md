# BST-Agent Quick Start

> 这份文档只负责说明**当前原型怎么运行**。方法论与论文主叙事请看 [`README.md`](./README.md)。

---

## 1. 环境要求

- Go >= 1.21
- Make
- 可用的 OpenAI-compatible LLM API
- `API_KEY`

原型目录：

- `ai-integration-test-demo/`

---

## 2. 构建

```bash
cd ai-integration-test-demo
make build
```

---

## 3. 手动启动服务端

```bash
./bin/server -host 127.0.0.1 -port 5400
```

WebSocket 地址：

- `ws://127.0.0.1:5400/ws`

安全说明：

- 当前 demo 按**本地可信环境**使用
- 默认绑定到 `127.0.0.1`
- WebSocket 升级仅接受本地来源（`localhost` / `127.0.0.1`）

---

## 4. 当前可运行的场景

### 4.1 L2 运行时基线

这些场景都属于当前已经打通的 **L2 / 预构建 CMD** 体系：

```bash
# 有代码 + 有步进
make test-dual API_KEY=xxx

# 无代码 + 有步进
make test-step-only API_KEY=xxx

# 有代码 + 无步进
make test-code-batch API_KEY=xxx

# 无代码 + 无步进
make test-batch-only API_KEY=xxx
```

### 4.2 分析基线（非标准运行时 L2）

`code-only` 更适合被理解为：

- 静态分析基线
- 消融场景
- 非运行时执行形态

它不应该和标准的运行时 L2 场景完全混在一起看。

```bash
make test-code-only API_KEY=xxx
```

### 4.3 L0 / L1 脚手架

当前已经补上了基础入口：

```bash
make test-l0 API_KEY=xxx
make test-l1 API_KEY=xxx
```

但要注意：

- 这表示脚手架和命令配置已经接入
- **不表示正式实验已经完成**
- 当前环境如果没有 `API_KEY`，仍然无法跑出真实 L0 / L1 结果

---

## 5. 主要参数

当前 `cmd/server/main.go` 支持：

| 参数 | 说明 |
|---|---|
| `-host` | 服务端绑定地址，默认 `127.0.0.1` |
| `-port` | 服务端端口，默认 `5400` |
| `-mode` | `server` / `test` |
| `-scenario` | 场景名 |
| `-api-key` | LLM API Key |
| `-model` | 模型名 |
| `-base-url` | OpenAI-compatible API 地址 |
| `-project-dir` | 代码分析根目录 |
| `-quick-start` | 是否预注入代码摘要 |
| `-doc-file` | 需求文档文件（已接线，会注入 prompt） |
| `-rules-file` | 专家规则文件（已接线，会注入 prompt） |

---

## 6. 需要特别注意的事实

当前代码里：

- 已有最小可用的 `register_cmd` MVP
- 已有 `test-l0` / `test-l1` 入口
- `doc-file` / `rules-file` 已接线，但还缺真实实验数据

所以当前原型更准确的状态是：

> 已经有可运行的 L2 基线，也补上了 L0 / L1 的基础脚手架；但由于当前环境缺少 `API_KEY`，正式的三阶段实验闭环仍未完成。

---

## 7. 目录结构

```text
integration-testing-guide/
├── README.md                  # 方法主文档
├── QUICKSTART.md              # 当前原型运行说明
└── ai-integration-test-demo/
    ├── cmd/server/main.go
    ├── internal/
    ├── ai/
    ├── results/
    ├── scripts/
    └── Makefile
```

---

## 8. 相关文档

- 方法主文档：[`README.md`](./README.md)
- 原型目录：[`ai-integration-test-demo/`](./ai-integration-test-demo/)
