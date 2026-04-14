# BST-Agent Evaluation Report

## Experiment Configuration

| Parameter | Value |
|-----------|-------|
| LLM | GLM-5.1 |
| API | 智谱 AI (open.bigmodel.cn) |
| Runs per group | 1 |
| Max Agent iterations | 80 |
| Prompt Level | Level 0 (Zero Prompt) |

## Bug Discovery Matrix (Ground Truth: B1-B7)

| Bug | Level | Severity | D (dual) | B (step-only) | C (code-batch) | A (batch-only) | code-only |
|-----|-------|----------|:--------:|:-------------:|:--------------:|:--------------:|:---------:|
| B1  | L1    | Critical | - | - | - | - | - |
| B2  | L2    | High     | - | Y | - | Y | Y |
| B3  | L2    | Medium   | Y | Y | - | - | Y |
| B4  | L3    | High     | Y | Y | Y | Y | Y |
| B5  | L3    | Critical | Y | Y | Y | - | Y |
| B6  | L4    | High     | Y | Y | Y | - | Y |
| B7  | L4    | Medium   | - | - | - | - | Y |

## Summary Metrics

| Metric | D (dual) | B (step-only) | C (code-batch) | A (batch-only) | code-only |
|--------|:--------:|:-------------:|:--------------:|:--------------:|:---------:|
| Bugs Found | 4 | 5 | 3 | 2 | 6 |
| Bug Precision | 80% | 83% | 75% | 25% | 60% |
| Bug Recall | 57% | 71% | 43% | 29% | 86% |
| Bug F1 | 0.667 | 0.769 | 0.545 | 0.267 | 0.706 |
| Corrs Found | 10 | 7 | 7 | 6 | 10 |
| Corr Precision | 100% | 100% | 100% | 75% | 77% |
| Corr Recall | 100% | 70% | 70% | 60% | 100% |
| Corr F1 | 1.000 | 0.824 | 0.824 | 0.667 | 0.870 |
| Level Score | 8.5 | 10.0 | 7.0 | 3.5 | 13.0 |
| Score Normalized | 0.61 | 0.71 | 0.50 | 0.25 | 0.93 |
| FP Rate | 20% | 17% | 25% | 75% | 40% |
| Iterations | 105 | 94 | 70 | 62 | 25 |

## Level Score Breakdown (max = 14.0)

| Level | Weight | D (dual) | B (step-only) | C (code-batch) | A (batch-only) | code-only |
|-------|--------|:--------:|:-------------:|:--------------:|:--------------:|:---------:|
| L1    | 1.0    | 0/1 | 0/1 | 0/1 | 0/1 | 0/1 |
| L2    | 1.5    | 1/2 | 2/2 | 0/2 | 1/2 | 2/2 |
| L3    | 2.0    | 2/2 | 2/2 | 2/2 | 1/2 | 2/2 |
| L4    | 3.0    | 1/2 | 1/2 | 1/2 | 0/2 | 2/2 |
| **Total** | **14.0** | **8.5** | **10.0** | **7.0** | **3.5** | **13.0** |

## 2x2 Ablation Factor Analysis

| Factor | Corr R Gain | Bug R Gain | Level Score Gain |
|--------|:-----------:|:----------:|:----------------:|
| Code main effect | +0.20 | +0.00 | +0.07 |
| Step main effect | +0.20 | +0.29 | +0.29 |
| Interaction (D-C)-(B-A) | +0.20 | -0.29 | -0.36 |

## Key Observations

1. **B (step-only) achieves best bug discovery in runtime groups**: 5/7 bugs, Level Score 0.71. Step mode enables precise causal observation even without code knowledge. D group spent ~30% iterations on code reading, leaving fewer for runtime testing.

2. **Code-only discovers most bugs (6/7) but with high FP rate (40%)**: Pure static analysis excels at structural issues (orphan events, missing guards). Cannot verify findings at runtime, leading to more false positives.

3. **B1 (RemoveItem negative count) missed by ALL groups**: Requires specifically testing removeitem with count=-1 (adversarial boundary condition). Not discoverable from code structure alone.

4. **B7 (day7 ID conflict) only found by code-only**: Requires cross-module ID space analysis.

5. **Negative interaction effect**: D (dual) underperforms B (step-only) in bug discovery, suggesting code reading and runtime testing compete for limited iteration budget.
