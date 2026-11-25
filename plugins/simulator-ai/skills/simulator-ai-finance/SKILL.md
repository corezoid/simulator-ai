---
name: simulator-ai-finance
description: "Balances, who paid whom how much, payments: Simulator accounts, transactions, transfers, currencies, counters, formulas. Triggers: \"account balance\", \"хто заплатив\", \"переказ коштів\", \"баланс счёта\"."
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
- `incomeType` is the side. **The workspace's convention for the pair comes first:** a budget
  account may keep its spending on `credit` on every project. `account_create` answers
  `postingSide {side, actors, otherSide}` — how other actors post one-sided transactions of the
  pair — and `ontology_apply`'s dry run shows it per transaction ("convention: credit (3 actors)").
  Without either, look: `transactions_filter {accounts: [{nameId, currencyId}], from, to}` and the
  `incomeType` of rows without `transferId` on other actors. Follow the majority unless the person
  names the side; only with no history: income (what the actor receives or earns) to `credit`,
  spending to `debit`. A transfer always moves value from `debit` rows to `credit` rows.

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
- What a chart (dashboard) shows for a period, its per-bucket values and their total:
  `chart_data_get` (see the charts skill), not a rebuild from `accounts_list`.
- `transactions_list {actorId, nameId, currencyId}` explains a balance; `account_transactions_list
  {accountId}` pages one row; `transactions_filter` searches the workspace. A window needs
  both `from` and `to` (Unix ms): one alone is ignored, and without both it is this month.
  sim-api reads at most 366 days per window; the tools split a longer period (all time: `from: 0`)
  into 366-day windows, newest first, up to 5 years back from `to`, sum the `*_count` tools and
  join pages up to `limit`. `next {from, to, cursor?}` is the call that reads on; `note` names a
  part the 5-year ceiling left unread. `offset` does not page a long period: use `next`.

## Who paid whom, how much

Start from the graph, not from the ledger: every transfer leaves a money edge
`transfersAmount_<nameId>_<currencyId>` from the payer to the receiver, its `weight` the amount
and its `linkedActorId` the transfer's Transfers actor (the tools below name the transfer itself).

1. `entity_context_get {actorId}` (one call): `transfers` sums those edges per counterparty —
   `direction` `in` (it paid this actor) or `out` (this actor paid it), `amount`, `account`,
   `currency` and `transferIds` (`transferActorIds`: transfers whose actor you may not read).
   A `partial` line sums only the newest `perType` edges of its type: repeat with `perType: 50`;
   still `partial`, page `graph_get {actorId, edgeTypeIds: [edgeTypeId]}` (limit/offset) and add
   up its `transfers` — never report a partial sum as the total. The actor's balances are in
   `accounts` of the same answer.
2. Details of one payment (date, comment, legs): `transfer_get {id: <transferId>, with: ["legs"]}`.
3. **A payment "for" a contract, an invoice, a project** — something that is seldom a party to it:
   `entity_context_get {actorId: <the contract>}` answers `payments`, the transfers of the last
   366 days it is a party to or whose comment names its number ("Оплата за договором №22"), each
   with `from`, `to`, amounts and `createdAt`. That answers "which payment, from whom, to whom".
4. **A period of payment** ("in April", "this year"): `transfers_filter {from, to, dateBy:
   "originalDate", actorIds?}` (and `transfers_filter_count` with the same filter) — sim-api bounds
   and orders the transfers by their legs' business date and answers it as `originalDate`. Without
   `dateBy` the window is when the transfer was ENTERED, which misses payments entered later for
   that month. A form-field transfer has no business date: `dateBy: "originalDate"` with
   `formFields` or `kind: "formFields"` is refused (-32602 invalid params); search those by
   `createdAt`. Over 366 days the tools read windows as usual, each with `dateBy`.
`graph_get` names the money edges of its answer in `transfers` too.

## Record a transaction

`transaction_create {accountId, amount, ref, comment}` posts to one row — the row of the side the
convention above names (`credit` or `debit` id from `account_create` / `accounts_list`) — and returns the
balance after it. An amount past the account's limits is stored as `type: "declined"` and
moves nothing: check the returned `type`. Always send a `ref`: after a timeout read `transaction_by_ref_get {accountId,
ref}` before retrying, never post twice. A hold is two steps: `type: "authorized"`, then a
second call with `type` `completed`, `canceled` or `reversed` and `parentId` (or `parentRef`).
`transaction_by_actor_ref_create` posts by the actor's template and ref without looking up the row.
A blocked, formula, statistical or transfer-only account refuses plain transactions.

## Transactions vs transfers

- A **transaction** is an event on ONE account (income, spending, a plan, an adjustment).
- A **transfer** is an exchange between parties: its legs are transactions, and it is what draws
  the graph's "transaction" edges (`transfersAmount_<nameId>_<currencyId>` between the from and
  to actors, asynchronously). Never create such an edge yourself, even when asked to "draw it
  directly": `edge_create`, `edges_create`, `graph_build` and `ontology_apply` refuse the type, and
  `edge_update` / `edge_endpoints_update` refuse to relink or re-point one. An edge without a
  transfer lies about money: explain that and offer the transfer instead.
- Wrong postings are corrected by a reversing one (storno), never deleted.
- Structure and money together ("a contract with its payment"): one `ontology_apply` patch with a
  dry run (`simulator-ai-ontology`).

**Legacy ledger.** A workspace whose ledger is not yet on this API refuses every posting with
"wrong financial API … legacy API" (`ledgerWrongAPI`): transactions and transfers there are still
made in the web app. Say so plainly; do not retry with other tools. Reading balances and history
works everywhere.

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

Find transfers with `transfers_filter` (`transfers_filter_count` totals the same filter):
`actorIds` lists an actor's transfers, a leg of the actor or an owner change that moved it.
A change of a form field's value between actors (and of the owner) is a transfer too:
`kind: "formFields"` keeps only those, any field, and `formFields: [{formId, fieldId}]` narrows
to fields (`fieldId` is the template item id, `select_1` or `ownerId`). Neither combines with
`accounts` or `accountType`. The window is `from` and `to` (Unix ms), this month by default.

**A form field's value or owner** moves between actors with `transfer_form_fields_create {ref,
comment?, from: [{actorId, formId, fieldId, value: [option values]}], to: [...]}`: from legs take
the values out of a select or multiSelect field, to legs add them, together exactly the same
values. Confirm the actors, field and values first; it shows in `transfers_filter` with
`kind: "formFields"`.

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
