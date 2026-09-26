---
id: TASK-5
title: 'Work: タブ行の「CI 失敗」が同じ PR を二重に数えうる'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - work
dependencies: []
references:
  - internal/app/presentation/tui/work/work.go
priority: low
type: bug
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`work/work.go` の `Summary` は全列の item を単純に数える。同じ PR が複数の列に並ぶ（自分の PR で assignee も自分、など）と重複して数える。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 CI 失敗の件数は PR を一意に数える
- [ ] #2 重複を含むテストがある
<!-- AC:END -->
