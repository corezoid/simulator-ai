---
name: simulator-ai-chat
description: Message a Simulator user and open or reuse a p2p or group chat with the public Simulator AI console tools. Use when the user wants to write to, message or DM someone, open a chat or reply in a chat ("напиши повідомлення користувачу", "надішли повідомлення", "відкрий чат з", "напиши сообщение пользователю", "открой чат с").
---

Use the installed `simulator-ai` catalog (profile `console`). A chat is an actor of the Events
system template with `data.chatType` `p2p` or `group`; its participants are the actor's access
rules; its messages are `comment` reactions. `chat_open` finds or creates the chat; the message
is `reaction_create` in it.

## Message user N

1. **Resolve N.** `users_search {query: "N"}` → N's `id`. Several matches: ask which one.
2. **Open the chat.** `chat_open {type: "p2p", userIds: [<N>], selfUserId: <you>}`. You are the
   user the run's credential names; `selfUserId`, the userId the request names ("userId … — the
   user you act for"), is checked against it and is needed where the credential cannot tell (an API
   key, or a member without the viewUsers permission: a reused chat must then hold its rule). It reuses the 1:1
   chat of exactly the two of you, as the web UI does (`ref` `p2pConversation_<min id>_<max id>`),
   else creates it titled `<nick>:p2p:<nick>` and shares it with N (view, modify, remove; N is
   notified once you are known to be its creator) in one atomic call. You own a chat you create: do not add yourself.
3. **Post the message.** `reaction_create {rootActorId: <chatId>, type: "comment",
   description: "<the text>"}`. A message notifies N: confirm the text with the user first
   when it speaks for them.

Read the conversation back with `reactions_list {rootActorId: <chat id>, view: "flat",
orderValue: "ASC"}`; `readTime` is when the other side last read it.

## Variations

- **Group chat:** `chat_open {type: "group", title, userIds: [...]}` always creates a new one
  (the UI does not reuse group chats) and shares it with every participant.
- **Reply in a thread:** `parentId` of the reaction replied to.
- **A file in a message:** `attachment_upload` first, then `attachmentIds: [id]` in
  `reaction_create` (see `simulator-ai-attachments`).
- **Quietly:** `notify: false` on `reaction_create`.

Participants are access rules, never `data` fields. An Events actor without `chatType` is an
event or a task (`simulator-ai-tasks`), not a chat.
