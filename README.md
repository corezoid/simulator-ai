# Simulator AI

**Connect your AI assistant to Simulator.Company.**

[![CI](https://github.com/corezoid/simulator-ai/actions/workflows/check.yml/badge.svg?branch=develop)](https://github.com/corezoid/simulator-ai/actions/workflows/check.yml)
[Documentation](docs/README.md) · [Changelog](CHANGELOG.md) · [Report an issue](https://github.com/corezoid/simulator-ai/issues)

Simulator AI is a Go-based [Model Context Protocol](https://modelcontextprotocol.io/) server
with skills for **Claude Code**, **Codex** and **Kiro**. It lets your assistant work with Simulator
records, forms, graphs and applications through the platform's public API.

The default profile is **read-only**. Additional profiles enable graph creation, Smart Forms,
and broader workspace operations. Every request uses the access granted to your configured
credential; enabling a tool does not grant additional platform permissions.

> **Development preview.** Source installation is available today. Tool availability depends
> on the selected profile and a compatible Simulator API. This is a separate project from
> [the legacy Simulator plugin](https://github.com/corezoid/simulator-ai-plugin).

## What you can do

- **Explore a workspace:** find records, inspect their fields, browse form templates and read graph layers.
- **Work with graphs:** inspect relationships and placements, plan flowcharts, and create graph elements with the appropriate profile.
- **Build Smart Forms:** create applications, edit their files and manage releases with guided workflows.
- **Use workspace tools:** Console profiles add access management, chats, files, accounts, charts and simulation.

The plugin includes skills that teach the assistant how to use these tools for common tasks.
It uses the model provided by your host client; no separate model API key is needed by the plugin.
Your host client's normal model usage and billing still apply.

## Quick start

You need Go **1.27.4+**, Git, a current Claude Code, Codex or Kiro client, and access to a Simulator
instance with the public API enabled. You sign in with your own Simulator.Company account in the
browser; an API-user key from your workspace administrator is an optional alternative.

### 1. Build the server

```sh
git clone https://github.com/corezoid/simulator-ai.git
cd simulator-ai
make build
export PATH="$PWD/bin:$PATH"
simulator-ai --version
```

Keep the executable on the client process's `PATH`. For a persistent installation, run
`go install ./cmd/simulator-ai` and add your Go binary directory to `PATH`.

On **Windows x64**, use PowerShell (Go 1.27.4+ for a source build):

```powershell
git clone https://github.com/corezoid/simulator-ai.git
cd simulator-ai
go build -p=1 -o bin/simulator-ai.exe ./cmd/simulator-ai
$env:PATH = "$PWD\bin;$env:PATH"
simulator-ai --version
$env:SIMULATOR_AI_PROFILE = "read"
```

With an API key instead of a browser login, also set `SIMULATOR_AI_ORIGIN` and
`SIMULATOR_AI_WORKSPACE`, and supply `SIMULATOR_AI_API_KEY` through your secret-management
mechanism before starting the client. The client install commands below also work from PowerShell. When a release archive
is available, extract `simulator-ai-windows-amd64.zip` and add its `bin` directory to `PATH`;
it contains the executable, plugin and skills together. You do not need Go to run that archive.

### 2. Sign in

Nothing is required in the environment for production: the server starts, and in your client you
ask the assistant to **log in to Simulator**. It calls `login`, your browser opens the
Simulator.Company (SuperAdmin) sign-in page, and after you sign in there it lists your workspaces
(`workspaces_list`) and asks which one to use (`workspace_set`). The session and the chosen
workspace are kept per API origin in `~/.config/simulator-ai/credentials.json` (mode 0600, never in
the project); the token is renewed with its refresh token when SuperAdmin issues one, otherwise
you run `login` again when it expires. `whoami` shows who you are signed in as; `logout` forgets
the stored login. Requests then act with **your own** permissions.

Optional settings, in the environment that starts your client:

| Variable | Purpose |
| --- | --- |
| `SIMULATOR_AI_ORIGIN` | Your API origin, without a path or query string; with a browser login `https://mw.simulator.company` when unset (required with an API key). HTTPS is required; literal loopback HTTP is supported for local development. |
| `SIMULATOR_AI_PROFILE` | Tool profile; defaults to `read`. See the profile table below. |
| `SIMULATOR_AI_WORKSPACE` | Pins the workspace (overrides `workspace_set`). Required with an API key. |
| `SIMULATOR_AI_API_KEY` | An API-user credential instead of the browser login; it always wins over a stored login. Keep it out of prompts, source control and shared configuration. |
| `SIMULATOR_AI_ACCOUNT_URL` | SuperAdmin of a non-production stand; `https://account.corezoid.com` when unset. |
| `SIMULATOR_AI_CONFIG_DIR` | Where the login is stored instead of `~/.config/simulator-ai`. |

```sh
export SIMULATOR_AI_PROFILE="read"   # or graph-read, console, …
```

### 3. Install the plugin

Run the commands for your client **from this repository's root**, in the configured shell.
They install the skills and MCP server configuration; the executable is installed separately
in step 1.

**Codex**

```sh
codex plugin marketplace add "$PWD" --json
codex plugin add simulator-ai@simulator-ai --json
codex
```

**Claude Code**

```sh
claude plugin install simulator-ai --marketplace "$PWD" --scope user --json
claude mcp list
claude
```

The plugin's Codex configuration forwards the `SIMULATOR_AI_*` variables it uses (plus
`XDG_CONFIG_HOME`; Codex passes `HOME` itself). Claude Code inherits them from its process
environment. Restart the client after changing the origin, profile or an API key; a login and
`workspace_set` take effect at once.

**Kiro**

Open **Powers → Add Custom Power → Import power from a folder** and select
`plugins/simulator-ai` inside this checkout or extracted release archive. Keep the matching
`simulator-ai` executable on Kiro's `PATH`. The Power uses the same bundled skills and Go
server as the other hosts, with `read` selected explicitly. Approve forwarding
`SIMULATOR_AI_ORIGIN`, `SIMULATOR_AI_WORKSPACE` and `SIMULATOR_AI_API_KEY` in Kiro;
configuration contains variable references, never the secret itself.

See [Kiro setup](docs/kiro.md) for changing profiles and troubleshooting.

### 4. Try a read-only task

For example:

- “List the form templates I can access in this workspace.”
- “Find records matching ‘customer onboarding’ and show their titles.”
- “Read this actor and summarize its fields: `<actor-id>`.”

Use `graph-read` to explore graph relationships. For write workflows, select the corresponding
profile below and give the assistant the specific change you want it to make.

## Tools by profile

| Profile | Capabilities | Writes |
| --- | --- | --- |
| `read` — default | Actor search and reads, form-template discovery, and paginated layer records. | No |
| `graph-read` | Read tools plus graph relationships, nodes, edges, placements, statistics and chart (dashboard) data. | No |
| `reactions-read` | Metadata reads and reaction counts. | No |
| `graph-create` | Graph reads, flowchart planning, and graph-element creation with recovery tools. | Create operations |
| `smart-forms` | Metadata reads plus application creation, file editing, releases and recovery. | Yes |
| `console` | A focused catalog for interactive workspace operations. | Yes |
| `console-full` | The complete Console catalog, including finance, files, charts, meetings and simulation. | Yes |

See the [generated catalog](docs/tool-catalog.md) for exact profile membership and the
[tool guide](docs/tool-reference.md) for public API mappings and behavior. The MCP catalog returned by your running server describes the tools enabled
for that profile.

Smart Forms and graph-create writes have explicit recovery keys and backend contract checks.
Console writes use different retry semantics. Follow the relevant guide when integrating
these profiles; do not assume every write is safe to replay.

### AI Console profile

`console` and `console-full` are also available to applications that host the MCP server.
A host can supply `SIMULATOR_AI_AUTHORIZATION` instead of an API key, using the requesting
user's credential. The separate [delegated-read profile](docs/delegated-read.md) supports
scoped host-issued access. These are integration options, not additional desktop setup steps.

## MCP-only setup

For a client that supports local stdio MCP servers, configure the executable directly.
This installs tools only; it does not load the bundled skills. Use it as an alternative
to the plugin installation above.

```json
{
  "mcpServers": {
    "simulator-ai": {
      "command": "simulator-ai",
      "args": []
    }
  }
}
```

For direct Codex configuration in `~/.codex/config.toml`:

```toml
[mcp_servers.simulator-ai]
command = "simulator-ai"
args = []
env_vars = ["SIMULATOR_AI_ORIGIN", "SIMULATOR_AI_WORKSPACE", "SIMULATOR_AI_API_KEY", "SIMULATOR_AI_PROFILE"]
```

Supply the same environment variables as in the quick start. Other MCP clients must support
stdio and forwarding configuration to the server process; their installation flow may differ.

## Documentation

Browse the [documentation index](docs/README.md) for user and contributor guides.

| Guide | Topics |
| --- | --- |
| [Tool reference](docs/tool-reference.md) | Tool catalog, profiles, API mappings and operational limits. |
| [Graph reads](docs/graph-read.md) · [Graph creation](docs/graph-create.md) · [Planning](docs/graph-planning.md) | Graph inspection, placement, creation and recovery. |
| [Smart Forms](docs/smart-forms.md) | Application and file workflows, release checks and recovery. |
| [Reaction statistics](docs/reactions-read.md) | Reaction counts and access behavior. |
| [Architecture](docs/architecture.md) | Transport, configuration and API boundaries. |
| [Repository quality](QUALITY.md) | Required checks, release evidence and review policy. |
| [Security policy](SECURITY.md) | Credentials, vulnerability reporting and security checks. |
| [Changelog](CHANGELOG.md) | Changes to the plugin. |

## Development

```sh
make check          # format, modules, lint, tests and build
make test-race      # race detection
make security       # reachable Go vulnerability scan
make client-smoke   # isolated Codex + Claude installation checks; no model calls
```

`make check` requires golangci-lint **2.14.0**. `make client-smoke` requires both client CLIs.
CI also checks generated files, coverage and git history for secrets. Agent-assisted
contributors should read [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md); Claude-specific entry instructions are in
[CLAUDE.md](CLAUDE.md).

For bugs or feature requests, [open an issue](https://github.com/corezoid/simulator-ai/issues)
with your plugin version, client and profile, plus a credential-free reproduction. Report
security issues privately as described in [SECURITY.md](SECURITY.md).

Maintained by [Simulator.Company](https://simulator.company/). Licensed under [MIT](LICENSE).
Dependencies retain their own licenses; see [third-party notices](THIRD_PARTY_NOTICES.md).
