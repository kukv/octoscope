---
id: TASK-31
title: 'Work: 一覧側の statusCheckRollup の contexts をページングしていない'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - work
  - checks
  - api
dependencies: []
references:
  - internal/github/gql/work.graphql
  - docs/superpowers/2026-09-07-phase3-checks-handoff.md
priority: low
type: enhancement
ordinal: 31000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Work 板・詳細・Repos 一覧の check の集計は、先頭 100 件の contexts しか見ない（`gql/work.graphql:46`、`pr.graphql:46`、`repo_prs.graphql:55`）。100 件を超える PR では、カードやドロワーの集計が欠ける。カード 1 枚ごとの往復を増やさないことを優先して見送った。

出典: phase3-checks-handoff D1
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 contexts が 100 件を超える PR で、集計が全件を反映する、または上限を超えたことが画面に出る
<!-- AC:END -->
