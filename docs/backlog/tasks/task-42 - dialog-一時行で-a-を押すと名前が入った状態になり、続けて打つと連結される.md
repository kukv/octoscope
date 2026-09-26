---
id: TASK-42
title: 'dialog: 一時行で a を押すと名前が入った状態になり、続けて打つと連結される'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
dependencies: []
references:
  - internal/app/presentation/tui/repo/add.go
priority: low
type: enhancement
ordinal: 42000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
カレントのリポジトリ（一時行）にカーソルがあるとき、`a` は入力欄をその名前で埋める。別のリポジトリを足したい利用者は、先に消す必要がある。

出典: phase4-repos-dialog-followups #3（次に触るときに決める）
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 名前が入った後の最初の打鍵で置き換えるかを決め、実装する
- [ ] #2 その挙動をテストで固定する
<!-- AC:END -->
