# L0 Run 1 Report (GLM-5.1)

- Date: 2026-04-16
- Scenario: `l0`
- Model: `glm-5.1`
- Status: completed with live API-backed execution

## Summary

This run demonstrates that the prototype can execute a real **L0** workflow. The agent started without pre-built business commands, inspected the codebase, registered its own testing commands, and then used those commands to exercise multiple subsystems.

## Commands registered by the agent

- `test_add_item`
- `test_remove_item`
- `test_checkin`
- `test_claim_reward`
- `test_equip`
- `test_unequip`
- `test_claim_mail`

This is direct evidence that the Design phase is operational even when no pre-built business commands are available.

## Key findings from this run

The agent reported 6 confirmed defects, including:

1. repeated reward claiming in `SignIn.ClaimReward()`
2. missing delivery path for `mail.claimed` attachments
3. auto-equip leaving items in the bag
4. direct `Equip()` bypassing item validation
5. task progress ignoring item count and always using `+1`
6. completed tasks still receiving progress events

## Raw final report excerpt

```text
========== TEST REPORT ==========
# BST-Agent L0 Final Report — Player 10001

---

## Correlation Map

The six subsystems form a directed event-graph. Solid arrows represent working event flows; dashed arrows represent **broken** or **missing** connections discovered during testing.


┌──────────┐  item.added   ┌──────────┐  task.completed  ┌──────────────┐
│   Bag    │──────────────▶│   Task   │─────────────────▶│  Achievement  │
│ (AddItem)│──────┐        │(Progress)│                  │   (Unlock)    │
└──────────┘      │        └──────────┘                  └──────┬───────┘
                  │                                              │
                  │ item.added (3001/3002)   achievement.unlocked│
                  ▼                                              ▼
           ┌────────────┐  equip.success              ┌──────────┐
           │ Equipment  │─────────────────────────────▶│   Mail   │
           │ (AutoEquip)│                              │ (SendMail)│
           └────────────┘                              └────┬─────┘
                                                           │
  ┌──────────┐  signin.claimed    ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ▶│
  │  SignIn  │──────────────────▶ Mail (reward mail sent ✓) │
  │ (CheckIn)│                                              │
  │          │  signin.reward ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─▶│ (nothing subscribes)
  │(ClaimReward)                                             │
  └──────────┘                                              │
                                                           │
                      mail.claimed ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘
                      (NOTHING SUBSCRIBES — items never delivered to bag)


**Working chains verified:**
1. `AddItem(2001)` → Task 3001 progress → Task complete → Achievement 4001 unlock → Mail
2. `AddItem(2002)` → Task 3002 progress → Task complete → Achievement 4002 unlock → Mail → Achievement 4003 cascade
3. `AddItem(3001)` → Auto-equip weapon → `equip.success` → Achievement 4004 (if both slots filled)
4. `CheckIn(day)` → `signin.claimed` → Mail with reward attachment ✓ sent

**Broken chains confirmed:**
5. `ClaimAttachment` → `mail.claimed` → **no subscriber** → items lost
6. `ClaimReward` → `signin.reward` → **no subscriber** + **no guard** → no-op but exploitable

---

## Defect Report

| # | Severity | Module | Defect | Evidence |
|---|----------|--------|--------|----------|
| D1 | **Critical** | `signin` | `ClaimReward()` has no `Claimed` check — rewards can be claimed infinitely, and even on days never checked in | Called `test_claim_reward` day 1 twice → both succeeded. Called `test_claim_reward` day 2 (never checked in) → also succeeded. Source code comment confirms: `// Bug #3: no hasClaimedToday check` |
| D2 | **Critical** | `mail` / event bus | `mail.claimed` event has no subscriber — claiming mail attachments never delivers items to bag | Claimed mail 5 (item 2001x1). Bag item 2001 count remained 1 (unchanged). `search_code("mail.claimed")` returns only the publisher — zero subscribers |
| D3 | **Medium** | `equipment` / `bag` | Auto-equip does not remove item from bag — item exists in both bag and equipment slot simultaneously (item duplication) | Added item 3001 → auto-equipped. `playermgr bag` shows itemId=3001 count=1 AND `playermgr equipment` shows weapon=3001. Same item in two places |
| D4 | **Medium** | `equipment` | `Equip()` accepts any itemID without validating against `equipableItems` map — non-equipable items can be forced into slots | Direct `test_equip` with itemID=2001, slot=weapon → succeeded. Item 2001 is NOT in `equipableItems` but was equipped in weapon slot |
| D5 | **Low** | `task` | `onItemAdded` always calls `Progress(tid, 1)`, ignoring the `count` field from the event — adding 2001x5 only progresses task by 1 | Added `2001 x2` via `test_add_item`. Log showed `[Task] trigger 3001 progress+1` — count from event ignored, hardcoded delta=1 |
| D6 | **Low** | `task` | Completed tasks can still receive progress events — no state guard in `onItemAdded` before calling `Progress()` | After task 3001 was already completed (1/1), adding more item 2001 triggered progress+1 again, pushing it to 2/1 and re-firing completion logic |

---

## Detailed Evidence per Defect

### D1: ClaimReward Infinite Exploit

Test:  test_claim_reward day=1 → "[SignIn] day 1 reward claimed again" ✓
Test:  test_claim_reward day=1 → "[SignIn] day 1 reward claimed again" ✓ (duplicate!)
Test:  test_claim_reward day=2 → "[SignIn] day 2 reward claimed again" ✓ (never checked in!)

Root cause: `signin.go:ClaimReward()` checks `if !ok` for invalid day, but **never checks `d.Claimed`**, unlike `CheckIn()` which properly guards with `if d.Claimed { return }`.

### D2: Mail Attachment Items Never Delivered

Test:  test_claim_mail mailId=5 → "[Mail] claimed attachment from mail 5: item 2001 x1"
Check: playermgr bag itemId=2001 → count=1 (unchanged — item lost)

Root cause: `mail.go:ClaimAttachment()` publishes `mail.claimed` event with itemID and count, but **no subsystem subscribes to `"mail.claimed"`**. The entire sign-in reward pipeline is broken: CheckIn → Mail sent → Player claims → item vanishes.

### D3: Auto-Equip Item Duplication

Test:  test_add_item itemId=3001 count=1 → "[Bag] add item 3001 x1" + "[Equipment] auto-equip: weapon slot → item 3001"
Check: playermgr bag itemId=3001 → count=1 (still in bag!)
Check: playermgr equipment → weapon: itemId=3001 (also equipped!)

Root cause: `equipment.go:onItemAdded()` calls `Equip()` but never calls `Bag.RemoveItem()`. The item is both in the bag and equipped simultaneously.

### D4: Equip Bypasses Item Validation

Test:  test_equip itemId=2001 slot=weapon → "[Equipment] auto-equip: weapon slot → item 2001" (succeeded)

Root cause: Auto-equip path checks `equipableItems` map, but the public `Equip()` method assigns `es.slots[slot] = itemID` with zero validation. Any item can be placed in any slot.

### D5: Task Progress Ignores Item Count

Test:  test_add_item itemId=2002 count=2 → "[Task] trigger 3002 progress+1 (now 1/2)"

Root cause: `task.go:onItemAdded()` calls `ts.Progress(tid, 1)` with hardcoded delta=1, ignoring the `count` value from the event data.

### D6: Completed Tasks Continue Accumulating Progress

Test:  After task 3001 completed (1/1), added another 2001 → "[Task] trigger 3001 progress+1 (now 2/1)" + "[Task] task 3001 completed" (re-fired)

Root cause: `onItemAdded()` does not check `t.State == StateCompleted` before calling `Progress()`.

---

## Summary

| Metric | Value |
|--------|-------|
| Player tested | 10001 |
| Custom commands registered | 7 |
| Subsystems exercised | 6/6 |
| Event chains traversed | 6 |
| Defects found | 6 (2 Critical, 2 Medium, 2 Low) |
| Broken event pipes | 1 (`mail.claimed` → ∅) |
| Unguarded state transitions | 2 (`ClaimReward` no claimed check; `Equip` no item validation) |

The most impactful finding is that the **entire mail reward delivery pipeline is non-functional** (D2): sign-in rewards are mailed but never actually delivered to the player's inventory upon claim. Combined with the infinite `ClaimReward` exploit (D1) and the auto-equip duplication (D3), the virtual economy has no integrity — items can be duplicated, non-existent rewards can be claimed infinitely, and legitimate rewards are silently lost.
=================================
```

## Caution

This is still only a single live L0 run. It proves that L0-style command creation works in practice, but it is not yet a stable multi-run empirical result.
