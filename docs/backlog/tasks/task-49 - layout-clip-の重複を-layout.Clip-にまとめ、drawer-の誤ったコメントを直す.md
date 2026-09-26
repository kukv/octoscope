---
id: TASK-49
title: 'layout: clip の重複を layout.Clip にまとめ、drawer の誤ったコメントを直す'
status: To Do
assignee: []
created_date: '2026-09-26 16:51'
labels:
  - work
  - checks
  - diff
dependencies: []
references:
  - internal/app/presentation/tui/layout/columns.go
  - internal/app/presentation/tui/drawer/drawer.go
priority: low
type: chore
ordinal: 49000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`layout.Clip` と同じ実装の `clip` が 4 か所に残っている（work・checks・diff・drawer の render）。`drawer.go:183-185` のコメントは「layout.Clip は余白を 1 桁取る」と言うが、余白を取るのは `layout.Pad` で、コメントが誤っている。

出典: phase4-search-foundation-followups #4
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 4 か所の clip を layout.Clip に置き換えても golden が変わらない
- [ ] #2 drawer.go の誤ったコメントを直す
<!-- AC:END -->
