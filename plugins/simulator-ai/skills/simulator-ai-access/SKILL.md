---
name: simulator-ai-access
description: Share or unshare a Simulator object and read who has access with the public Simulator AI console tools. Use when the user wants to share with someone, grant or revoke view/modify/remove/sign/execute, make something read-only, or asks who can see or edit an actor, a template, a template's actors or an account ("поділись з", "надай доступ", "забери доступ", "хто має доступ", "дай доступ", "открой доступ").
---

Use the installed `simulator-ai` catalog (profile `console` or `console-full`). Access rules
say who (a user, a group or an API user, each by its `userId`) may do what (`view`, `modify`,
`remove`, `sign`, `ds`, `execute`) to one object.

## The object

`access_rules_get` and `access_rules_save` take the same object:

| objType | objId | What it covers |
| --- | --- | --- |
| `actor` | the actor id | one actor (a chat, a task, a layer, any record) |
| `formTemplate` | the template id | the template itself (who may edit it) |
| `templateActors` | the template id | the default access to every actor of the template |
| `account` | the account name id, plus `currencyId` | one account of an actor |

## Steps

1. Resolve each person first, never guess: `users_search` with the name or login → `id`.
   Share with that `userId`, never with the person's twin actor.
2. Read the current rules when the user asks who has access, or before narrowing access:
   `access_rules_get {objType, objId}`.
3. Change them with one `access_rules_save` call:
   - `rules: [{userId, privs: {view, modify, remove, sign?, ds?, execute?}}]` grants or replaces
     each subject's privileges. The tool adds `view` to any other privilege; all false removes
     the rule.
   - `remove: [userId]` revokes.
   - `mode: "merge"` (default) leaves everyone not named as is. `mode: "replace"` also revokes
     everyone not named who only views, signs or carries the object out; whoever may modify it
     (you among them), the system owner and public-link guests stay unless `remove` names them.
   - `reactionOrders: {sign, ds, execute}` orders approval stages (1 first; the same number
     acts in parallel). Send it only to set a sequence.
   - `isNotify: true` tells the subjects (default quiet). An actor change cascades to its layers
     unless `isRecursive: false`.
4. Report what changed from the result, by name.

Sharing notifies people only with `isNotify`; still confirm with the user before giving
someone modify or remove, and before a `replace` that revokes others.

## Templates of system forms

A `templateActors` rule on a system form (Events, Layers, ...) is restricted by the backend:
only owners/admins may change it, only API users can be added, and only `view`/`modify` apply
(send `view: true` explicitly). Share individual actors instead.

## When you cannot see an object

A read that fails with an access error means you lack `view`. Say so; do not retry under
another identity or guess the data. Requesting access from the owner needs the
`access_request_*` tools, which arrive with a later release of this catalog.
