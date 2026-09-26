---
id: TASK-12
title: 'diff: 今の diff に無いファイルへのスレッドを見せる'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - diff
dependencies: []
references:
  - internal/app/presentation/tui/diff/thread.go
  - docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md
priority: low
type: feature
ordinal: 12000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
設計 §4.4.1 の既知の限界。フォースプッシュ後などでスレッドの対象ファイルが今の diff に無いと、そのスレッドはどこにも描かれない（`diff/thread.go:64-66`）。同じファイルで行が動いたものだけは末尾に出るようになった。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 diff に無いファイルのスレッドがどこかで見える
<!-- AC:END -->
