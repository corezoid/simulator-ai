---
name: simulator-ai-finance
description: Manage Simulator accounts, balances, transactions, transfers, currencies, account names and counters with the public Simulator AI tools (profile console-full). Use when the user wants to open an account on an actor, read a balance or its history, record a transaction, move value between actors, add a currency or category, track a counter, set a balance, write a formula, or tag accounts and bind triggers ("баланс рахунку", "запиши транзакцію", "переказ коштів", "лічильник", "баланс счёта", "перевод средств").
---

Use the installed `simulator-ai` catalog. Plain `console` (AI Console) has `account_create`,
`accounts_list`, `transaction_create`, `transfer_create` and `form_account_create`; the rest
below is `console-full`. Every tool works in the host workspace.

## The model

- An **account name** (`nameId`, e.g. Revenue, Mileage) says what a balance means; a
  **currency** (`currencyId`, e.g. USD, km) is its unit. Together they are a **pair**, an
  access-controlled object of the workspace.
- An **account** is that pair opened on an actor, typed by `accountType` (`fact` by default,
  or `plan`, `min`, `max`, `avg`): two rows, `credit` and `debit`, each with its own `id`, the
  `accountId` of the transaction tools.
- Amounts are real values: `500` on a USD account is 500 USD; `precision` is display only.
- `incomeType` is the side: post income (what the actor receives or earns) to its `credit`
  row and spending to its `debit` row; a transfer moves value from `debit` rows to `credit`
  rows.

## Set up

**Open an account on an actor by its names** ("відкрий рахунок Default USD"):
`account_create {actorId, accountName: "Default", currencyName: "USD"}`, as the actor's
Accounts tab does: the name and the currency are found (a missing one is created), the pair is
registered, and the `fact` account is opened in one atomic call; `plan: true` also opens its
plan twin. It returns `accounts [{accountType, credit, debit}]`, the row ids the transaction
tools take, and answers an account the actor already has as it is (`existed`). A pair nobody was
given access to is refused: repeat with `takeOwnership: true` only when the user agrees (see 1).

Every actor of an **Account Template** gets the account with `form_account_create {formId,
accountName, currencyName}`: actors created later open it, and the template's existing actors
get it now (500 per call; repeat with `offset: nextOffset`). `skipExisting: true` leaves them.

With ids, step by step:

1. `account_pair_create {accountName, currencyName, symbol?, precision?, type?}`, by NAME. It
   finds the name and the currency, creates only the missing ones, and returns `{nameId,
   currencyId, registered}`:
   - a new pair is registered with you as its owner (`registered: true`);
   - an existing shared pair stays as it is: you can post only if its rules give you `modify`,
     and one you may not see is refused; an owner shares it with `access_rules_save {objType:
     "account", objId: <nameId>, currencyId}` (`simulator-ai-access`);
   - an existing pair that nobody was given access to is refused: its balances can only be
     read through each actor (`account_get`, `accounts_list`) and nothing can be posted to it.
     Registering it makes you its only subject and hides it from everyone else, so repeat with
     `takeOwnership: true` only when the user agrees.
2. `account_create {actorId, nameId, currencyId, accountType?, minLimit?, maxLimit?}` opens it
   on an actor (or `account_by_ref_create` by template and ref, `console-full`).

`currencies_list` / `account_names_list` (`query`) find existing ones; `currency_create`,
`account_name_create` add one alone; `currency_update` replaces every field.

## Read

- `account_get {actorId, nameId, currencyId}`: one pair with balances (`amount`, `holdAmount`).
- `accounts_list {actorId, ...filters}`: an actor's `fact` rows unless `accountType` or
  `accountTypes` asks for others, 20 per page; `from` and `to` (Unix ms) together give the
  turnover of a window; `accounts_count` counts.
- `transactions_list {actorId, nameId, currencyId}` explains a balance; `account_transactions_list
  {accountId}` pages one row; `transactions_filter` searches the workspace. A window needs
  both `from` and `to` (Unix ms, at most about 12 months): one alone is ignored, and without
  both it is this month.

## Record a transaction

`transaction_create {accountId, amount, ref, comment}` posts to one row and returns the
balance after it. An amount past the account's limits is stored as `type: "declined"` and
moves nothing: check the returned `type`. Always send a `ref`: after a timeout read `transaction_by_ref_get {accountId,
ref}` before retrying, never post twice. A hold is two steps: `type: "authorized"`, then a
second call with `type` `completed`, `canceled` or `reversed` and `parentId` (or `parentRef`).
`transaction_by_actor_ref_create` posts by the actor's template and ref without looking up the row.
A blocked, formula, statistical or transfer-only account refuses plain transactions.

## Move value

"Проведи транзакцію між акторами" (a transaction between actors, a payment from A to B) is a
**transfer**: one `transfer_create` from A's `debit` row to B's `credit` row of the same pair
(`accounts_list {actorId, nameId}` for each, or the ids `account_create` returned). A single
`transaction_create` is only for when the user names ONE account (income to it, a charge to
it). Neither actor has the pair yet: open it on both with `account_create` first.

`transfer_create {ref, from: [{accountId, amount}], to: [{accountId, amount}]}`: `from` legs are
debit rows, `to` legs credit rows, one currency, balanced, up to 20 per side. Confirm the
amounts and the parties with the user first; a completed transfer is not undone. With
`type: "authorized"` it holds the value. To settle it, read `transfer_get {id, with:
["legs"]}`, then call `transfer_create` again with `type` `completed` or `canceled`, the
hold's `transferId`, and the `from` and `to` legs again, each with `parentId` (its step-1 leg
transaction id) or `parentRef`. After a timeout read `transfer_by_ref_get {ref}` before
retrying. Between people, find each person (`users_search`) and their twin actor
(`actor_system_get {objType: "user", objId: <userId>}`), then their account rows.

## Counters

`counter_increment`, `counter_set`, `counter_get`, `counter_by_period_get` address a counter by
strings: the actor's template (`formId`) and `actorRef`, `accountName` and `currency` by name,
`incomeType`. Counter writes are never retried and may be buffered: after an error read
`counter_get` before repeating.

## Change or remove

`account_update` replaces every setting (send the limits and URLs to keep);
`account_status_update {id, status: "blocked"|"active"}` freezes one row; `account_delete {actorId,
id}` deletes a row with its transactions and `account_pair_unregister` drops the pair's access
rules. Confirm every delete with the user.

## Counter accounts

`account_create` with `counterType` `counter`, `uniqCounter` or `systemCounter` opens an account
whose balance is read from that counter (fed by `counter_increment`); a transaction posted on it
does not show in its balance. `ignoreIfExist: true` answers a pair the actor already has
(`existed: true`) instead of failing.

## By account id

`account_by_id_get {accountId}` reads one row when you hold its id (from a transaction, a formula
source, a trigger); `from`/`to` (Unix ms) as in `accounts_list`.

## Set a balance

`account_amount_set {id, amount}` sets a row's balance by recording the difference as a
transaction: permanent and in its history. Confirm with the user first; undo by setting the old
value back. Formula and statistical accounts are refused. A target past the account's limits is
stored as declined and a counter account's balance does not move, so read the row back with
`account_by_id_get` before reporting the new balance.

## Formulas

A formula computes an account's balance from other accounts, named by their row ids (bare
lowercase UUIDs), with numbers, `+ - * /`, parentheses and `sum`, `mean`, `min`, `max`, `abs`,
`round(x[, n])`, e.g. `<accountId> * 2`. Always
`account_formula_preview {id, formula}` first (`ok`, the value, or the first error), then
`account_formula_set`; an empty formula clears it. An account with transactions cannot take a
formula, and one with a formula takes no transactions. `account_formula_get` shows the text,
sources and current value.

## Several transactions at once

`atom_transaction_create {transactions: [...]}` posts 1..20 transactions all-or-nothing (each as
in `transaction_create`, with its own `ref`): any refusal stores none. Confirm with the user
first.

## Tags and triggers

- Tags file a pair: `account_tag_add {nameId, currencyId, title}` adds one by title;
  `account_tags_list` reads them; `account_tags_set` replaces the whole set. An actor's folders:
  `account_actor_tags_list` (a folder's `actorId` is the `tag` filter of `accounts_list`).
- Triggers are actors of the AccountTriggers template: pair-level (`account_triggers_list` /
  `_set`), one actor's own account (`account_actor_triggers_list` / `_set`), or a data field of
  an actor (`actor_field_triggers_list` / `_set`, data kind).
- Every `_set` REPLACES the whole set, others' bindings included: read the list first and send
  every id to keep.

## Not available

Child actors' accounts (legacy getChildAccounts) are not carried over: account trees were removed
from the platform. To total a subtree, walk it (`graph_list` with type children) and read each
child's accounts (`accounts_list`). Triggers by template field (for every actor of a template)
are out of scope.
