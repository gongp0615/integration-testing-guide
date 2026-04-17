# Formal Experiment Status

## Current state

The codebase now contains:

- a minimal `register_cmd` MVP
- L0 / L1 scenario wiring
- Makefile targets for `test-l0` and `test-l1`
- offline regression tests for the interface-gap path around B1
- one completed **real** L0 run on GLM-5.1
- one completed **real** L1 run on GLM-5.1

## What has been completed

### Offline verification

- `go test ./...` passes
- `go test -race ./internal/server` passes
- `make build` succeeds
- server-side command profiles now distinguish L0 / L1 / L2
- built-in `removeitem` rejects non-positive counts at the command layer
- registered raw commands can still call `bag.RemoveItem` and reproduce B1

### Live model evidence

A first real **L0** run has been completed with GLM-5.1.

What this run demonstrates:

- the agent can start without pre-built business commands
- the agent can design and register its own test command surface
- the agent can exercise multiple subsystems through registered commands
- the current prototype can now support at least one honest L0-style autonomous interface-construction run

See:

- `l0-run1-report.md`
- `l0-live-run1.log`
- `l0-run1-metadata.json`
- `l0-run1-registry.json`
- `l0-run1-prompt.md`

A first real **L1** run has been completed with GLM-5.1.

What this run demonstrates:

- the agent can understand the codebase and runtime state
- the agent can use `register_cmd` to create purpose-specific validation commands
- the agent can reproduce **B1** by registering and executing `test_remove_negative`
- the current prototype can now support at least one honest L1-style interface-gap validation run

See:

- `l1-run1-report.md`
- `l1-run1.log`
- `l1-run1-metadata.json`
- `l1-run1-registry.json`
- `l1-run1-prompt.md`

### Fresh updated-code L2 evidence

Fresh L2 reruns were started under the updated codebase, but they did not all complete to final report artifacts.

What we have:

- partial fresh `dual` evidence
- partial fresh `step-only` evidence
- later rerun attempts blocked by `429 Too Many Requests / insufficient balance`

So the practical blocker has now changed from “missing credentials” to “credentials present but current API resources exhausted for additional runs”.

See:

- `fresh-l2-attempts.md`

## What is still incomplete

Formal experimental completion still requires:

- at least one completed fresh **L2** comparison run under the updated codebase
- ideally repeated runs for stability / variance reporting
- final defect/correlation comparison tables written back into the paper

## Partial fresh L2 evidence

Fresh reruns under the updated codebase have been started for:

- `dual`
- `step-only`

Current status:

- both have produced new runtime/code-observation evidence under the updated implementation
- neither has yet been normalized into a final comparison report artifact
- therefore they should currently be treated as **partial fresh evidence**, not final experiment rows

## Honest conclusion

At this point, the project has completed:

- implementation
- offline validation
- first live L0 evidence
- first live L1 evidence
- partial fresh updated-code L2 evidence

But it has **not yet completed the full L0 / L1 / L2 empirical comparison** needed for a finished paper-level experiment section.
