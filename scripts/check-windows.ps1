$ErrorActionPreference = 'Stop'
$archive = 'dist/simulator-ai-windows-amd64.zip'
$expected = Get-Content dist/checksums.txt | Where-Object { $_ -match '  simulator-ai-windows-amd64\.zip$' }
if (@($expected).Count -ne 1) { throw 'Missing or duplicate Windows checksum' }
if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne ($expected -split ' ')[0]) { throw 'ZIP checksum mismatch' }
$destination = Join-Path $env:RUNNER_TEMP 'simulator-ai-install'
Expand-Archive $archive $destination
$binary = Join-Path $destination 'bin/simulator-ai.exe'
$version = & $binary --version
if ($LASTEXITCODE -ne 0 -or $version -notmatch [regex]::Escape("($env:GITHUB_SHA)")) { throw 'Wrong executable revision' }
foreach ($file in @('LICENSE', 'licenses/pdf/LICENSE', 'licenses/go/LICENSE', 'plugins/simulator-ai/plugin.json', 'plugins/simulator-ai/mcp.json', 'plugins/simulator-ai/skills/simulator-ai-read/SKILL.md')) {
  if (!(Test-Path (Join-Path $destination $file))) { throw "Missing package file: $file" }
}
$start = [System.Diagnostics.ProcessStartInfo]::new()
$start.FileName = $binary
$start.UseShellExecute = $false
$start.RedirectStandardInput = $true
$start.RedirectStandardOutput = $true
$start.RedirectStandardError = $true
$start.Environment['SIMULATOR_AI_ORIGIN'] = 'http://127.0.0.1:1'
$start.Environment['SIMULATOR_AI_WORKSPACE'] = 'windows-fixture'
$start.Environment['SIMULATOR_AI_API_KEY'] = 'windows-synthetic-key'
$start.Environment['SIMULATOR_AI_PROFILE'] = 'read'
$start.Environment['SIMULATOR_AI_CONFIG_DIR'] = Join-Path $env:RUNNER_TEMP 'simulator-ai-config'
$process = [System.Diagnostics.Process]::new()
$process.StartInfo = $start
function Read-Response {
  $pending = $process.StandardOutput.ReadLineAsync()
  if (!$pending.Wait(15000)) { throw 'MCP response timed out' }
  if (!$pending.Result) { throw 'MCP exited without a response' }
  return $pending.Result | ConvertFrom-Json
}
try {
  if (!$process.Start()) { throw 'Cannot start MCP' }
  $process.StandardInput.WriteLine('{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"windows-ci","version":"1"}}}')
  $initialized = Read-Response
  if ($initialized.error -or $initialized.id -ne 1 -or !$initialized.result.serverInfo.name) { throw 'MCP initialization failed' }
  $process.StandardInput.WriteLine('{"jsonrpc":"2.0","method":"notifications/initialized"}')
  $process.StandardInput.WriteLine('{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}')
  $catalog = Read-Response
  if ($catalog.error -or $catalog.id -ne 2 -or 'actor_get' -notin $catalog.result.tools.name) { throw 'Missing read catalog' }
  # Sign-in tools change only the local login state: not read-only, never destructive.
  $account = @('login', 'logout', 'workspace_set')
  if ('whoami' -notin $catalog.result.tools.name) { throw 'Missing account tools' }
  foreach ($tool in $catalog.result.tools) {
    if ($tool.name -in $account) {
      if ($tool.annotations.destructiveHint -ne $false) { throw "Destructive account tool: $($tool.name)" }
      continue
    }
    if ($tool.annotations.readOnlyHint -ne $true) { throw "Unexpected write tool: $($tool.name)" }
  }
  Write-Output "$version; native Windows stdio and read catalog verified"
} finally {
  if ($process.Id) {
    $process.StandardInput.Close()
    if (!$process.WaitForExit(3000)) { $process.Kill($true); $process.WaitForExit() }
  }
  $process.Dispose()
}
