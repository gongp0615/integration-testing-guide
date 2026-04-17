# L1 Run 1 Report (GLM-5.1)

- Date: 2026-04-16
- Scenario: `l1`
- Model: `glm-5.1`
- Status: completed with live API-backed execution

## Summary

This run demonstrates that the current prototype can execute a real **L1** workflow with live model output. The agent inspected the code and runtime state, used the new `register_cmd` capability to create purpose-specific validation commands, and successfully crossed the interface gap for B1.

Audit note: the stored `l1-run1.log` had one post-run timestamp normalization on the `test_remove_negative` execution line to keep the log monotonic. The command sequence itself was already consistent with line order and server behavior (registration before execution).

## Custom commands registered by the agent

- `test_remove_negative`
- `test_add_zero`

These commands are important because they show the Design phase is no longer purely conceptual in the prototype.

## Strongest result: B1 was confirmed through a registered command

The built-in `removeitem` command rejected `count=-1` at the command layer.
The agent then registered `test_remove_negative`, mapped to raw `bag.RemoveItem`, and executed it with `count=-1`.

Observed result:

- prior bag count for `itemId=2001`: `1`
- after raw negative remove: `2`

This is a direct confirmation of **B1** and a concrete demonstration of the interface-gap thesis:

- understanding alone found the suspicion
- design created a new validation interface
- execution turned suspicion into defect evidence

## Defects reported in this run

1. Mail claim attachment does not add items to bag
2. ClaimReward allows unlimited repeated claims
3. B1: negative RemoveItem increases items
4. collector_100 counts achievements instead of items
5. Equip accepts invalid items / slot combinations
6. Arbitrary equipment slot names are accepted

## Caution

This is still only a **single live L1 run**.
It is enough to show that:

- live L1 execution now works
- `register_cmd` is being used by the agent in practice
- B1 can be reproduced through the new interface path

It is **not enough** to claim a finished empirical comparison across L0 / L1 / L2.
