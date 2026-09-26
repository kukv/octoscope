---
id: TASK-46
title: 'Search: 候補の取得中にカーソルが行き来すると同じ取得が積まれる'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - search
  - perf
dependencies: []
references:
  - internal/app/presentation/tui/search/search.go
priority: low
type: bug
ordinal: 46000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`maybeFetchCandidates` は取得済みのキャッシュしか見ず、取得中かどうかを覚えていない（`search/search.go:232-250`）。応答の前に label や author の行を離れて戻ると、同じリポジトリの取得がもう一度走る。

出典: phase4-search-tab-followups #7
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 同じリポジトリの候補を取得している間は、再要求を積まないことをテストで確かめる
<!-- AC:END -->
