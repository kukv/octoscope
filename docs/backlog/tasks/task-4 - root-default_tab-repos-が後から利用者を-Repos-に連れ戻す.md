---
id: TASK-4
title: 'root: default_tab: repos が後から利用者を Repos に連れ戻す'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - root
  - config
dependencies: []
references:
  - internal/app/presentation/tui/root/root.go
  - internal/app/presentation/tui/root/mouse.go
priority: medium
type: bug
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`wantRepos` は repoResolved の中でしか消えない（`root/root.go:327`）。リポジトリ名が返る前（最長 20 秒）にキーやタブ行クリックで別タブへ移っても、返ってきた時点で Repos に戻される。設計 §4「見ている画面を奪わない」に反する。テスト（root_test.go:1392）は返事の後に移る場合しか見ていない。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 解決前に利用者がタブを移したら、解決後も移動しない
- [ ] #2 そのケースのテストがある
<!-- AC:END -->
