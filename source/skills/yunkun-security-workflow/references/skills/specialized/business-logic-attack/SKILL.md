---
name: business-logic-attack
description: >
  Authorized business-flow integrity lane for payment, order, quota, coupon,
  identity, approval, workflow, and state-transition review.
---

# Business Logic Lane

Business logic issues are invariant failures. The goal is to demonstrate one
safe, reproducible violation of an expected rule, not to generate a large
volume of unusual requests.

## Phase 0: Model the Invariant

Write the normal flow and the rule that must remain true:

- price and total are server-derived;
- quantity, amount, quota, and balance stay within valid ranges;
- an order follows an allowed state transition;
- a coupon, reward, or verification code has one defined lifecycle;
- an identity can change only its own objects;
- an approval requires the correct role and predecessor state;
- a retry or concurrent request cannot create duplicate value.

Record the actor, object, tenant, state, precondition, transition, and expected
postcondition.

## Phase 1: Select One Boundary

Choose the strongest signal from the observed flow:

| Boundary | Safe comparison |
| --- | --- |
| Value | server total versus client-provided total |
| Quantity | zero, negative, boundary, and normal value |
| Object | same operation on owned and peer object |
| State | allowed predecessor versus skipped predecessor |
| Identity | same token versus a separately authorized role |
| Replay | first request versus a controlled duplicate |
| Concurrency | two bounded requests against a disposable object |

Never combine value, identity, state, and concurrency mutations in one test.

## Phase 2: Baseline and Differential Check

1. Capture a normal request and the complete relevant response.
2. Confirm the object and actor are synthetic or explicitly authorized.
3. Change one field or one ordering constraint.
4. Compare status, response body, returned object, audit event, and final
   state.
5. Restore or delete test state and confirm the final state.

Client-side validation bypass alone is not an issue. The server must accept an
invalid transition or produce unauthorized value.

## Phase 3: High-Value Families

- payment and refund: price source, total recomputation, callback integrity,
  duplicate settlement, and refund ownership;
- orders and inventory: quantity, state transitions, cancellation, shipment,
  and duplicate fulfillment;
- coupons and rewards: reuse, scope, stacking, expiry, and tenant binding;
- account recovery: token binding, role binding, replay, and contact change;
- approval workflows: role, predecessor state, alternate endpoint, and
  asynchronous job ownership;
- concurrency: one-time redemption, balance mutation, quota decrement, and
  idempotency.

Use `references/playbooks/logic-flaws/11-business-logic.md` for bounded
examples and `skills/skill/knowledge-base/race-condition-test.md` when the
invariant depends on concurrency.

## Evidence Gate

The result is reportable only when it includes:

- authorized scope and test identity;
- normal and mutated request pair;
- precise object or business state;
- observable unauthorized effect;
- cleanup or rollback confirmation;
- redacted, reproducible evidence.

Return `incomplete` when the response changes but no meaningful state or
security impact is demonstrated.
