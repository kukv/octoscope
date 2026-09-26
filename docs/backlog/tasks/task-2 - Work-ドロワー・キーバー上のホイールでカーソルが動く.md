---
id: TASK-2
title: 'Work: ドロワー・キーバー上のホイールでカーソルが動く'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - work
  - mouse
dependencies: []
references:
  - internal/app/presentation/tui/work/mouse.go
  - docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md
priority: high
type: bug
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`work/mouse.go` の `handleMouseWheel` は X 座標しか見ず Y を確かめない。ドロワーやキーバーの上でホイールを回すと、その X にある列のカーソルが動く。設計 §4.0「ドロワー上のホイールは何もしない」に反する。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 盤面より下（ドロワー・キーバー）でのホイールは何もしない
- [ ] #2 それを縛るテストがある
<!-- AC:END -->
