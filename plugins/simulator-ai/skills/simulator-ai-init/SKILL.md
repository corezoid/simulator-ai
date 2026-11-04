---
name: simulator-ai-init
description: Connect the public Simulator AI MCP to a Simulator stand — origin, workspace, credential and profile through the client's environment — and check the connection. Use when the user wants to set up, connect, log in to or configure Simulator, switch workspace or profile, or the simulator-ai tools are missing or refuse to start ("init", "setup", "connect to simulator", "set workspace", "підключи симулятор", "налаштуй", "подключи симулятор").
---

The `simulator-ai` server has no login, OAuth or workspace-switch tool. It reads its whole
configuration from the **process environment** of the host client when it starts, and binds
one origin, one workspace and one credential for the life of the process. Never ask for a key
or token in chat, never put one in a tool argument, a file you write or your answer.

## Variables

| Variable | Value |
| --- | --- |
| `SIMULATOR_AI_ORIGIN` | the stand's origin only, `https://<host>` (no path; plain `http` only for literal loopback) |
| `SIMULATOR_AI_WORKSPACE` | the workspace id the credential belongs to |
| `SIMULATOR_AI_API_KEY` | an API-user key of that workspace (desktop) |
| `SIMULATOR_AI_AUTHORIZATION` | instead of the key: a complete header value, `Bearer <token>`, `Simulator <jwt>` or `ApiKey <key>`; used when a host such as AI Console starts the process per request under the user's own credential |
| `SIMULATOR_AI_PROFILE` | the tool catalog, below; `read` when unset |
| `SIMULATOR_AI_ACTOR` | optional: binds `graph-read` or `reactions-read` to one layer/actor |

Set exactly one of `SIMULATOR_AI_API_KEY` and `SIMULATOR_AI_AUTHORIZATION`. Redirects are
refused, so the origin must be the one that answers the API.

## Profiles

| Profile | For |
| --- | --- |
| `read` | actor, template and layer reads |
| `graph-read` / `graph-create` | native graph reads / creating a flowchart with receipts |
| `smart-forms` | recoverable Smart Form writes (`simulator-ai-smart-forms`) |
| `reactions-read` | reaction counts |
| `console` | the AI Console core: actors, templates, flowcharts, Smart Forms, comments, access, files |
| `console-full` | `console` plus every other actor, template, graph, access, reaction, user, file and finance tool |

Pick the narrowest profile that covers the task; a profile is chosen at start, not per call.
`delegated-read` takes a scoped token from the Console manager and is never set up by hand.

## Steps

1. Ask which stand (origin) and workspace the user means; never assume a default.
2. Tell the user to put the variables in the environment the client starts MCP servers with
   (its secret manager, or the shell that launches it), then restart the client or its MCP
   server. Claude Code inherits the shell; Codex forwards only the names in the server's
   `env_vars` list, which must include the credential variable in use.
3. Check the connection with one cheap read of the active profile: `forms_list {type:
   "system", limit: 3}` (`read`, `graph-read`, `console`), `workspaces_list` in `console-full`,
   `graph_edge_types_list {limit: 1}` in `graph-create`, the single tool with `{}` when
   `SIMULATOR_AI_ACTOR` binds it, and in `smart-forms`, `reactions-read` or `delegated-read` an
   `actor_get` of an actor the user names. System templates only, and an empty custom list,
   mean the API user has no workspace role yet.
4. Report the origin, the workspace id and the profile, never the credential.

## When it fails

- No `simulator-ai` tools: the server did not start; a required variable is missing or both
  credentials are set. Check the client's MCP status.
- `accessDenied` / `notAuthorized`: the credential lacks a scope or a role in that workspace.
  Ask the workspace owner; do not retry under another identity.
- `methodNotFound`: the stand's sim-api is older than the tools need.

To work in another workspace or stand, the user changes the variables and restarts the server;
there is no switch tool.
