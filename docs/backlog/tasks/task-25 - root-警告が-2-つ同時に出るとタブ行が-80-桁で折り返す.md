---
id: TASK-25
title: 'root: 警告が 2 つ同時に出るとタブ行が 80 桁で折り返す'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - root
  - i18n
dependencies: []
references:
  - internal/app/presentation/tui/root/render.go
  - docs/superpowers/2026-09-08-phase4-config-followups.md
priority: low
type: bug
ordinal: 25000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`root/render.go:57-80` の `tabRow()` は、`tab.repo_lookup_timeout` と `tab.config_unreadable` を切らずに連結する。両方が立つと ja で約 99 桁、en で約 108 桁になり、80 桁で折り返す。この組み合わせの golden も無い。

出典: phase4-config-followups:8-17（稀なこと、片方を切るとどちらの警告か分からないことから見送り）
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 両方の警告が立っても、ja・en の 80 桁でタブ行が 1 行に収まる
- [ ] #2 その状態の golden（ja・en × 80）がある
<!-- AC:END -->
