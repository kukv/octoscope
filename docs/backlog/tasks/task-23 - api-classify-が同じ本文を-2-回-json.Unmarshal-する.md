---
id: TASK-23
title: 'api: classify が同じ本文を 2 回 json.Unmarshal する'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
dependencies: []
references:
  - internal/github/api/transport.go
priority: low
type: chore
ordinal: 23000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
GraphQL の部分応答を判定するために、`statusError`（`transport.go:108`）でデコードした本文を、`classify`（:140）でもう一度デコードしている。実害は無く、コストだけの問題。

出典: phase4-rest-backend-followups #1
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 本文のデコードが 1 回になり、既存の transport のテストが通る
<!-- AC:END -->
