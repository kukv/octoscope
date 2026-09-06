#!/usr/bin/env bash
# InstructionsLoaded: どの CLAUDE.md / rules がいつ・なぜ読み込まれたかを記録する。
# 出力はログだけで、context には何も足さない。
set -euo pipefail

log="${CLAUDE_PROJECT_DIR:-.}/.claude/instructions-loaded.log"
jq -c --arg ts "$(date -Iseconds)" '{ts: $ts} + del(.transcript_path)' >> "$log"
