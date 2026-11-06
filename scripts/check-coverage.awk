# Baselines from the first CI adoption; excludes third-party code and manual smoke CLIs.
BEGIN { minimum["mcpserver"] = 86; minimum["publicapi"] = 73; minimum["simulation"] = 73 }
{
  for (pkg in minimum) {
    if (index($1, "/internal/" pkg "/")) {
      total[pkg] += $2
      if ($3 > 0) covered[pkg] += $2
    }
  }
}
END {
  for (pkg in minimum) {
    percent = total[pkg] ? 100 * covered[pkg] / total[pkg] : 0
    printf "%s: %.1f%% (minimum %d%%)\n", pkg, percent, minimum[pkg]
    if (!total[pkg] || percent < minimum[pkg]) failed = 1
  }
  exit failed
}
