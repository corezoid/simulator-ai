---
name: simulator-ai-tasks
description: Create a Simulator task or an order addressed to someone and assign its executor, approvers and signers with the public Simulator AI console tools. Use when the user wants to create or set a task, assign it, add approvers, require a signature, or issue an order or directive to a person ("створи задачу", "признач виконавця", "додай погоджувача", "потрібен підпис", "наказ на", "создай задачу", "приказ").
---

Use the installed `simulator-ai` catalog (profile `console`). A task is an actor of the Events
system template with no `data.chatType`. Its roles are access privileges on that actor, not
data fields:

| Role | Privilege | Acts with the reaction |
| --- | --- | --- |
| executor (виконавець) | `execute` | `done` |
| approver (погоджувач) | `sign` | `sign`, or `reject` with a reason |
| legal signer | `ds` | `ds` (a qualified signature) |
| watcher | `view` | none |

An order or directive "to X" is a task whose addressee X is the executor: the name in the title
alone assigns nobody.

## Create and assign

1. **Resolve the Events template:** `forms_list {type: "system", query: "Events"}`.
2. **Resolve every person** with `users_search` → `id`; never guess an id.
3. **Create the task:** `actor_create {formId: <Events>, title: "<short label>", description:
   "<the full task text>", data: {startDate: <now>, endDate: <deadline>}}`. Dates are plain
   Unix seconds; leave `chatType` out.
4. **Assign the roles in one call:** `access_rules_save {objType: "actor", objId: <task id>,
   rules: [{userId: <executor>, privs: {view: true, execute: true}}, {userId: <approver>,
   privs: {view: true, sign: true}}]}`. One person may hold several flags in one rule. Order
   approvals with `reactionOrders: {sign: 1}`, `{sign: 2}` (the same number signs in
   parallel). Add `isNotify: true` to tell them, after the user agrees.
5. **Report** the task and who holds which role.

If the workspace has its own order template with example records, read one (`actor_get`,
`access_rules_get`) and assign the person the same way it does, including any responsible
field in its `data`, and always grant the addressee access.

## Follow the lifecycle

The assignees act; you read: `reactions_list {rootActorId: <task id>, view: "flat"}` and count
`done`, `sign`, `ds` and `reject` against the people assigned. A follow-up note is a `comment`
reaction (`reaction_create`); the task body stays in the actor's `description`.

A task with `chatType` set is a chat (`simulator-ai-chat`). Roles are never stored in `data`.
