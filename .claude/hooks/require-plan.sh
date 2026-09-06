#!/usr/bin/env bash
# PreToolUse(Edit|Write): internal/ と cmd/ の編集を、設計か実装計画を読んでいない
# セッションでは止める。設計を無視した実装が過去に何度も起きたための歯止め。
set -euo pipefail

payload=$(cat)
file=$(jq -r '.tool_input.file_path // ""' <<<"$payload")
transcript=$(jq -r '.transcript_path // ""' <<<"$payload")
cwd=$(jq -r '.cwd // ""' <<<"$payload")

rel=${file#"$cwd"/}
case "$rel" in
  internal/*|cmd/*) ;;
  *) exit 0 ;;
esac

# The pattern has to name a file, not a directory. CLAUDE.md points at
# docs/superpowers/specs/ and docs/superpowers/plans/ as places to look, and
# the whole of CLAUDE.md arrives as one JSONL line -- so a looser pattern
# matched that directory here and some unrelated .md later on the same line,
# and the gate passed in every session. Only [A-Za-z0-9._-] may follow the
# directory, which stops at the backtick CLAUDE.md puts there.
if [ -f "$transcript" ] &&
  grep -qE 'docs/superpowers/(specs|plans)/[A-Za-z0-9._-]+\.md' "$transcript"; then
  exit 0
fi

jq -n '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "deny",
    permissionDecisionReason: "設計と実装計画を読まずに internal/ または cmd/ を編集しようとしている。docs/superpowers/specs/ の該当設計と docs/superpowers/plans/ の該当計画を読んでから続けること。計画が無ければ先に書いて承認を得る。"
  }
}'
exit 2
