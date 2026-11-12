---
name: simulator-ai-init
description: Connect the public Simulator AI MCP to a Simulator stand — origin, workspace, credential and profile through the client's environment — and check the connection. Use when the user wants to set up, connect, log in to or configure Simulator, switch workspace or profile, or the simulator-ai tools are missing or refuse to start ("init", "setup", "connect to simulator", "set workspace", "підключи симулятор", "налаштуй", "подключи симулятор").
---

The `simulator-ai` server signs in one of two ways:

- **Browser login (desktop default).** With no credential in the environment the server starts
  anyway: `login` opens the SuperAdmin sign-in page in the browser, the person signs in there,
  and the server keeps the session for that origin in the user's config directory
  (`~/.config/simulator-ai/credentials.json`, never the project). Then `workspaces_list` and
  `workspace_set` pick the workspace. Requests act with the person's own rights.
- **Host credential.** `SIMULATOR_AI_API_KEY` (or `SIMULATOR_AI_AUTHORIZATION` from a host such
  as AI Console) in the process environment. It always wins over a stored login; the workspace
  then comes from `SIMULATOR_AI_WORKSPACE` and `login` only explains that it is disabled.

Never ask for a key, password or token in chat, never put one in a tool argument, a file you
write or your answer. The person types their password only on the SuperAdmin page.

## Variables

| Variable | Value |
| --- | --- |
| `SIMULATOR_AI_ORIGIN` | the stand's origin only, `https://<host>` (no path; plain `http` only for literal loopback); production `https://mw.simulator.company` when unset |
| `SIMULATOR_AI_WORKSPACE` | the workspace id; required with an API key, optional with a login (it then overrides `workspace_set`) |
| `SIMULATOR_AI_API_KEY` | optional: an API-user key of that workspace instead of a browser login |
| `SIMULATOR_AI_AUTHORIZATION` | instead of the key: a complete header value, `Bearer <token>`, `Simulator <jwt>` or `ApiKey <key>`; used when a host such as AI Console starts the process per request under the user's own credential |
| `SIMULATOR_AI_PROFILE` | the tool catalog, below; `read` when unset |
| `SIMULATOR_AI_ACTOR` | optional: binds `graph-read` or `reactions-read` to one layer/actor |
| `SIMULATOR_AI_ACCOUNT_URL` | optional: the SuperAdmin of a non-production stand (`https://account.corezoid.com` when unset) |
| `SIMULATOR_AI_CONFIG_DIR` | optional: where the login is stored instead of `~/.config/simulator-ai` |

Set at most one of `SIMULATOR_AI_API_KEY` and `SIMULATOR_AI_AUTHORIZATION`. Redirects are
refused, so the origin must be the one that answers the API.

## Profiles

| Profile | For |
| --- | --- |
| `read` | actor, template and layer reads |
| `graph-read` / `graph-create` | native graph reads / creating a flowchart with receipts |
| `smart-forms` | recoverable Smart Form writes (`simulator-ai-smart-forms`) |
| `reactions-read` | reaction counts |
| `console` | the AI Console core: actors, templates, flowcharts, Smart Forms, comments, access, files |
| `console-full` | broader Console workflows; inspect the actual catalog for availability |

Pick the narrowest profile that covers the task; a profile is chosen at start, not per call.
`delegated-read` takes a scoped token from the Console manager and is never set up by hand.

## Steps

1. Call `whoami`: it shows the origin, the credential kind (`login` or `apiKey`), the signed-in
   user and the workspace in use, with `next` when something is missing. No `whoami` tool means
   the server did not start or a host Authorization runs it (see below).
2. **Login:** if `whoami` says `call login`, call `login`. The browser opens; ask the person to
   sign in there. A `pending` answer means they have not finished yet: wait for them, then call
   `whoami` again (if `authorizationUrl` is returned, give them that link to open). Never sign in
   for them.
3. **Workspace:** call `workspaces_list`, show the workspaces to the person and let them choose;
   then `workspace_set` with that id (or exact name). Never pick one yourself. The choice is kept
   for this origin until changed; `SIMULATOR_AI_WORKSPACE` in the environment overrides it.
4. **Other stand or profile:** origin and profile are environment settings — the user changes
   `SIMULATOR_AI_ORIGIN` / `SIMULATOR_AI_PROFILE` where the client starts MCP servers and restarts
   it. Claude Code inherits the shell; Codex forwards only the names in the server's `env_vars`
   list. Kiro uses the Power’s MCP environment references and explicitly selects `read`.
   An API key goes in the same place, through the client's secret manager.
5. Check with one cheap read of the active profile: `forms_list {type: "system", limit: 3}`
   (`read`, `graph-read`, `console`), `graph_edge_types_list {limit: 1}` in `graph-create`, the
   single tool with `{}` when `SIMULATOR_AI_ACTOR` binds it, and in `smart-forms`,
   `reactions-read` or `delegated-read` an `actor_get` of an actor the user names. With an API key,
   system templates only and an empty custom list mean the API user has no workspace role yet.
6. Report the origin, the user or credential kind, the workspace and the profile — never a token.

## When it fails

- No `simulator-ai` tools: the server did not start (an invalid variable, both credentials set,
  or no home directory for a login). Check the client's MCP status.
- `not signed in` / `sign-in … expired`: call `login` again. `logout` forgets the stored login.
- `no workspace selected`: steps 3 above.
- `login is disabled`: an API key is configured; to sign in as yourself the user removes
  `SIMULATOR_AI_API_KEY` from the environment and restarts the server.
- `accessDenied` / `notAuthorized`: the credential lacks a scope or a role in that workspace.
  Ask the workspace owner; do not retry under another identity.
- `methodNotFound`: the stand's sim-api is older than the tools need.
