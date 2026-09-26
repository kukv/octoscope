---
id: TASK-32
title: 'drawer: check の所要時間を出していない'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - work
  - checks
dependencies: []
references:
  - internal/app/presentation/tui/drawer/drawer.go
  - internal/github/gql/work.graphql
priority: low
type: feature
ordinal: 32000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
モックアップのドロワーにある check の所要時間が出ていない。Work 板のクエリが startedAt と completedAt を取っていない（取っているのは `checks.graphql` だけ）。

出典: phase1-followups:173
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Work 板のクエリが check の開始・完了時刻を取り、ドロワーの各 check 行に所要時間が出る
- [ ] #2 80 桁の golden で桁が崩れない
<!-- AC:END -->
