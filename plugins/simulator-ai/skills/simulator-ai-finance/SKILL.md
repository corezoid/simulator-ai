---
name: simulator-ai-finance
description: Manage Simulator accounts, balances, transactions, transfers, currencies, account names and counters with the public Simulator AI tools (profile console-full). Use when the user wants to open an account on an actor, read a balance or its history, record a transaction, move value between actors, add a currency or category, or track a counter ("баланс рахунку", "запиши транзакцію", "переказ коштів", "лічильник", "баланс счёта", "перевод средств").
---

Use the installed `simulator-ai` catalog with profile `console-full`; plain `console` has no
finance tools. Every tool works in the host workspace.

## The model

- An **account name** (`nameId`, e.g. Revenue, Mileage) says what a balance means; a
  **currency** (`currencyId`, e.g. USD, km) is its unit. Together they are a **pair**, an
  access-controlled object of the workspace.
- An **account** is that pair opened on an actor, typed by `accountType` (`fact` by default,
  or `plan`, `min`, `max`, `avg`): two rows, `credit` and `debit`, each with its own `id`, the
  `accountId` of the transaction tools.
- Amounts are real values: `500` on a USD account is 500 USD; `precision` is display only.

## Set up

1. `account_pair_create {accountName, currencyName, symbol?, precision?, type?}`, by NAME. It
   creates the name and the currency when missing and registers the pair with you as owner.
   Run it before opening the pair on any actor; it is safe to repeat. Refused means the pair
   is shared and you are not among its subjects: an owner shares it with `access_rules_save
   {objType: "account", objId: <nameId>, currencyId}` (`simulator-ai-access`).
2. `account_create {actorId, nameId, currencyId, accountType?, minLimit?, maxLimit?}` opens it
   on an actor (or `account_by_ref_create` by template and ref).

`currencies_list` / `account_names_list` (`query`) find existing ones; `currency_create`,
`account_name_create` add one alone; `currency_update` replaces every field.

## Read

- `account_get {actorId, nameId, currencyId}`: one pair with balances (`amount`, `holdAmount`).
- `accounts_list {actorId, ...filters}`: every row of an actor; `from`/`to` (Unix ms) give
  the turnover of a window; `accounts_count` counts.
- `transactions_list {actorId, nameId, currencyId}` explains a balance; `account_transactions_list
  {accountId}` pages one row; `transactions_filter` searches the workspace. Windows are Unix
  ms, at most about 12 months, this month by default.

## Record a transaction

`transaction_create {accountId, amount, ref, comment}` posts to one row and returns the
balance after it. Always send a `ref`: after a timeout read `transaction_by_ref_get {accountId,
ref}` before retrying, never post twice. A hold is two steps: `type: "authorized"`, then a
second call with `type` `completed`, `canceled` or `reversed` and `parentId` (or `parentRef`).
`transaction_by_ref_create` posts by the actor's template and ref without looking up the row.
A blocked, formula, statistical or transfer-only account refuses plain transactions.

## Move value

`transfer_create {ref, from: [{accountId, amount}], to: [{accountId, amount}]}`: `from` legs are
debit rows, `to` legs credit rows, one currency, balanced, up to 20 per side. Confirm the
amounts and the parties with the user first; a completed transfer is not undone. With
`type: "authorized"` it holds the value, and a second call with `transferId` and `type`
`completed` or `canceled` settles it. After a timeout read `transfer_by_ref_get {ref}` before
retrying. Between people, find each person (`users_search`) and their twin actor
(`actor_system_get {objType: "user", objId: <userId>}`), then their account rows.

## Counters

`counter_increment`, `counter_set`, `counter_get`, `counter_period_get` address a counter by
strings: the actor's template (`formId`) and `actorRef`, `accountName` and `currency` by name,
`incomeType`. Counter writes are never retried and may be buffered: after an error read
`counter_get` before repeating.

## Change or remove

`account_update` replaces every setting (send the limits and URLs to keep);
`account_status_update {id, status: "blocked"|"active"}` freezes one row; `account_delete {actorId,
id}` deletes a row with its transactions and `account_pair_unregister` drops the pair's access
rules. Confirm every delete with the user.

## Not available yet

Reading an account by its id alone, child actors' accounts, counter-type accounts, direct balance
adjustment, formulas, atomic multi-account transactions, and account tags and triggers wait for
public sim-api methods. Say so rather than composing them from other tools.
