---
id: TASK-28
title: 'docs: 502 に対する即時再試行の成功率を実測し、設計 §6 を確定する'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
  - perf
dependencies: []
references:
  - docs/superpowers/specs/2026-09-11-fetch-failures-design.md
priority: low
type: docs
ordinal: 28000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
30 回投げて 502 が一度も出なかったため、即時再試行が成功するかの数字が無い。遅延を入れるかどうかの根拠も無い。実装は遅延なしで読み取りを 1 回だけ引き直す（`cli/cli.go:85,117`、`api/rest.go:74`）。

出典: fetch-failures-followups #4
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 実際の 502 に対する即時再試行の結果が記録され、fetch-failures-design §6 が数字つきで書き直されている
- [ ] #2 その数字をもとに、遅延を入れるかが決まっている
<!-- AC:END -->
