# Kiro setup

Simulator AI is a local stdio MCP Power for Kiro. It uses the same Go executable and
portable skills as Claude Code and Codex. It does not start a server-side agent or require
another model provider key.

## Install

1. Build the server or extract the matching release archive as described in the
   [quick start](../README.md#quick-start). Add its `bin` directory to the environment `PATH`
   inherited by Kiro. On Windows, the executable is `simulator-ai.exe`.
2. Set `SIMULATOR_AI_ORIGIN`, `SIMULATOR_AI_WORKSPACE` and `SIMULATOR_AI_API_KEY` in the
   environment that starts Kiro. Restart Kiro after changing them. Never paste a key into chat.
3. In **Powers → Add Custom Power → Import power from a folder**, choose
   `plugins/simulator-ai` from that same checkout/archive. Its `plugin.json`, `mcp.json`
   and `skills/` form one Power. Install either the Power or a manual MCP entry, not both.
4. Approve the three environment variables when Kiro requests access. Enable MCP if disabled.
   Check the server connection and available tools, then try a read task from the README.

Kiro namespaces Power server names. Choose the tools belonging to the installed Simulator AI
Power; the visible server name need not be exactly `simulator-ai`.

## Profiles and permissions

The Power explicitly selects `read`, even if the shell has another profile set. To use
`graph-read` or a write profile, edit this Power's MCP configuration in Kiro and set
`SIMULATOR_AI_PROFILE` to that exact profile. Reconnect and inspect its catalog before use.
Read the [tool reference](tool-reference.md) for differences in write/recovery behavior.
No wildcard tool auto-approval is included. Kiro approval and backend permissions are
separate: approving a tool never grants platform access.

This follows Kiro's [Power format](https://kiro.dev/docs/powers/create/),
[MCP configuration](https://kiro.dev/docs/mcp/configuration/) and
[Agent Skills](https://kiro.dev/docs/skills/) support. It requires a Kiro version supporting
`plugin.json` Powers. It does not use the legacy plugin's shell launcher, OAuth login,
`$CLAUDE_PLUGIN_ROOT` substitution or `.kiro-plugin` metadata. Skills and their relative
reference files are already portable, so no generated copies or extra installer are needed.

## Update and troubleshoot

Update the executable and Power from the same release, then reload the Power and restart its
MCP server. Keep the previous archive for rollback. Do not update credentials or API origin
as a side effect of a plugin update.

If the executable is not found, check Kiro's process `PATH` or set an absolute executable path
in its MCP configuration. A desktop icon launch may not inherit your shell environment.
For missing configuration, check the approved variables and restart the client; do not embed
credentials into a shared configuration file. Access errors require checking the API user's
permissions on the configured instance/workspace, not switching identity automatically.

Repository tests check the Power's version, credential references and safe default against
other host packages. Windows CI executes the packaged binary over stdio. These checks do
not constitute a real Kiro installation or model-driven acceptance; verify connection,
skill discovery and an allowed read in the installed client before rolling it out.
