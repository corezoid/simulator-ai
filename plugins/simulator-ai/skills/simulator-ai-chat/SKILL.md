---
name: simulator-ai-chat
description: Message a Simulator user and open or reuse a p2p or group chat with the public Simulator AI console tools. Use when the user wants to write to, message or DM someone, open a chat or reply in a chat ("напиши повідомлення користувачу", "надішли повідомлення", "відкрий чат з", "напиши сообщение пользователю", "открой чат с").
---

Use the installed `simulator-ai` catalog (profile `console`). A chat is an actor of the Events
system template with `data.chatType` `p2p` or `group`; its participants are the actor's access
rules; its messages are `comment` reactions. There is no "send message" tool: compose it.

## Message user N

1. **Resolve N.** `users_search {query: "N"}` → N's `id`. Several matches: ask which one.
2. **Resolve the Events template** (its id differs per workspace):
   `forms_list {type: "system", query: "Events"}` → the row titled `Events`.
3. **Build the reuse key.** It needs both userIds: N's and the sender's (ask the user or match
   them with `users_search`; there is no "current user" tool). Sort them ascending:
   - `ref = "p2pConversation_<min>_<max>"`, the key the web UI reuses a chat by;
   - `title = "<nick of min>:p2p:<nick of max>"`.
4. **Reuse before creating.** `actor_by_ref_get {formId: <Events>, ref}`. Found: use its id and
   go to step 6.
5. **Create the chat** only when none exists:
   `actor_create {formId: <Events>, title, ref, data: {chatType: "p2p", startDate: <now>,
   endDate: <now>, scheduleMeeting: false}}`. `startDate`/`endDate` are plain Unix seconds here.
   Then add N: `access_rules_save {objType: "actor", objId: <chat id>, rules: [{userId: <N>,
   privs: {view: true, modify: true, remove: true}}]}`. The creator is the owner already: do
   not add yourself.
6. **Post the message.** `reaction_create {rootActorId: <chat id>, type: "comment",
   description: "<the text>"}`. A message notifies N: confirm the text with the user first
   when it speaks for them.

Read the conversation back with `reactions_list {rootActorId: <chat id>, view: "flat",
orderValue: "ASC"}`; `readTime` is when the other side last read it.

## Variations

- **Group chat:** `data.chatType: "group"` and no `ref` (the UI does not reuse group chats),
  then one `access_rules_save` with every participant in `rules`.
- **Reply in a thread:** `parentId` of the reaction replied to.
- **A file in a message:** `attachment_upload` first, then `attachmentIds: [id]` in
  `reaction_create` (see `simulator-ai-attachments`).
- **Quietly:** `notify: false` on `reaction_create`.

Participants are access rules, never `data` fields. An Events actor without `chatType` is an
event or a task (`simulator-ai-tasks`), not a chat.
