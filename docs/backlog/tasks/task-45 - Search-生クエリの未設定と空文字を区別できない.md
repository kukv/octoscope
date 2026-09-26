---
id: TASK-45
title: 'Search: 生クエリの未設定と空文字を区別できない'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - search
dependencies: []
references:
  - internal/app/presentation/tui/search/search.go
priority: low
type: bug
ordinal: 45000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`e` で生クエリを全部消して確定すると `raw` が空になり、フィルタから組み立て直した結果に戻る（`search/search.go:100, 161-162`）。型が string なので、2 つを区別できない。

出典: phase4-search-tab-followups #11
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 空で確定したときの意味を仕様で決め、型（*string など）で表す
- [ ] #2 その挙動をテストで固定する
<!-- AC:END -->
