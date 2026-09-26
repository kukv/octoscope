---
id: TASK-50
title: 'Search: 保存クエリのポップアップで名前とクエリの桁が揃っていない'
status: To Do
assignee: []
created_date: '2026-09-26 16:51'
labels:
  - search
dependencies: []
references:
  - internal/app/presentation/tui/search/saved_render.go
priority: low
type: enhancement
ordinal: 50000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`savedRow` は `q.Name + " " + query` の単純な連結（`search/saved_render.go:70-73`）。名前の長さが違うとクエリの開始位置がずれ、全角の名前ではさらにずれる。

出典: phase4-saved-queries-followups #1
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 名前の列を最大幅（上限つき）で揃え、en・ja の golden でクエリの開始桁が揃う
<!-- AC:END -->
