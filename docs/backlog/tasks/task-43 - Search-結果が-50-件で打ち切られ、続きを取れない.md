---
id: TASK-43
title: 'Search: 結果が 50 件で打ち切られ、続きを取れない'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - search
  - api
dependencies: []
references:
  - internal/github/gql/work.graphql
  - internal/app/presentation/tui/search/render.go
priority: medium
type: enhancement
ordinal: 43000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`gql/work.graphql:11` は `first: 50` で、Search は `50+` と描くだけ（`search/render.go:48-50`）。任意のクエリを投げられるので 50 件で切れやすい。表示側の `searchCap` と GraphQL の `first` が一致している前提も、テストで守られていない。

出典: phase4-search-foundation-followups #1、phase4-search-tab-followups #2 #3
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 51 件目以降を取れる（ページング、またはもっと読む）
- [ ] #2 表示の上限と取得件数が同じ定義から来るか、食い違うとテストが落ちる
<!-- AC:END -->
