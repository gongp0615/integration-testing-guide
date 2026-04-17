# Fresh L2 Attempt Notes (Updated Codebase)

## Purpose

These notes track the first attempts to rerun L2 scenarios after the `register_cmd` MVP and profile changes landed, so that the paper does not silently mix old-structure L2 data with current-code behavior.

## Attempt status

### 1. `dual-live-run1.log`
- status: **partial / interrupted**
- what was confirmed before interruption:
  - `additem(2001,1)` still triggers Task 3001 → Achievement 4001 → Mail
  - `additem(2002,1)` twice still triggers Task 3002 completion → Achievement 4002/4003 → Mail
  - `additem(3001)` / `additem(3002)` still drives auto-equip and `fully_equipped`
  - current-code runtime behavior remains broadly aligned with the old dual chain structure

### 2. `step-live-run1.log`
- status: **partial / interrupted**
- what was confirmed before interruption:
  - `checkin(day=1)` sends reward mail
  - `claimmail(mailId=1)` logs a successful claim but bag remains empty afterwards
  - this reproduces the broken `mail.claimed` delivery path under the updated codebase
  - `additem(2001,1)` still triggers Task 3001 / Achievement 4001 / Mail in step mode
  - repeated `additem(2001,2)` produces extra progress and re-completion signs on task 3001

### 3. Later fresh rerun attempts
- `dual-live-run2.log`: blocked by `429 Too Many Requests / insufficient balance`
- `batch-live-run2.log`: blocked by `429 Too Many Requests / insufficient balance`
- `code-only-live-run2.log`: blocked by `429 Too Many Requests / insufficient balance`

## Honest interpretation

At this point we have:

- full legacy L2 baselines from the old structure
- full live L0 evidence on the updated codebase
- full live L1 evidence on the updated codebase
- only **partial fresh L2 evidence** on the updated codebase

Therefore the paper can honestly claim:

- current-code L0 and L1 are live and evidenced
- current-code L2 reruns were started and yielded confirming partial traces
- full current-code L0/L1/L2 comparison is still incomplete because fresh L2 reruns were cut short and later blocked by API resource exhaustion
