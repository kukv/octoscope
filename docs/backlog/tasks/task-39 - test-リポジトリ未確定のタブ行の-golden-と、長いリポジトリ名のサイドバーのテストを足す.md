---
id: TASK-39
title: 'test: リポジトリ未確定のタブ行の golden と、長いリポジトリ名のサイドバーのテストを足す'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
  - root
  - test
dependencies: []
references:
  - internal/app/presentation/tui/root/golden_test.go
  - internal/app/presentation/tui/repo/golden_test.go
priority: low
type: chore
ordinal: 39000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
root の通常のタブ行は `Options{Repo: "kukv/demo"}` でしか録っていない（`root/golden_test.go:53-75`）。サイドバーの fixture は短い名前だけで、30 桁を超える名前の切り詰めを踏んでいない。

出典: phase4-repos-sidebar-followups #1 #2
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 root に Options{} の通常画面の golden を、en・ja × 80/120/160 で足す
- [ ] #2 30 桁を超える名前をサイドバーに入れ、1 行に収まることを確かめる
<!-- AC:END -->
