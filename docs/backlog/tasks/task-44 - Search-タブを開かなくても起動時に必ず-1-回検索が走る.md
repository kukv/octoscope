---
id: TASK-44
title: 'Search: タブを開かなくても起動時に必ず 1 回検索が走る'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - search
  - root
  - perf
dependencies: []
references:
  - internal/app/presentation/tui/root/root.go
  - internal/app/presentation/tui/search/search.go
priority: low
type: enhancement
ordinal: 44000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
root の Init が `m.search.Init()` を呼び、それが即座に `runSearch` を投げる（`root/root.go:477`、`search/search.go:167-168`）。タブを開いた瞬間に結果が出るほうが速く感じられるとして許容したが、起動直後に GitHub 全体への検索が 1 本走る。

出典: phase4-search-tab-followups #10
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Search タブを初めて開いたとき（または default_tab: search のとき）だけ初回の検索が走る
- [ ] #2 起動直後に Search の取得が走らないことを root のテストで確かめる
<!-- AC:END -->
