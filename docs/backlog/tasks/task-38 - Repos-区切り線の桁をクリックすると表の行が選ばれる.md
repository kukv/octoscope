---
id: TASK-38
title: 'Repos: 区切り線の桁をクリックすると表の行が選ばれる'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
  - mouse
dependencies: []
references:
  - internal/app/presentation/tui/repo/mouse.go
priority: low
type: bug
ordinal: 38000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`repo/mouse.go:24` は `msg.X < sidebarWidth` で判定しているが、区切り線は x=30 にある。区切り線をクリックすると :40 で X が -1 になり、表の行の選択に落ちる。

出典: phase4-repos-sidebar-followups #4
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 区切り線の桁をクリックしても何も起きないことをテストで確かめる
<!-- AC:END -->
