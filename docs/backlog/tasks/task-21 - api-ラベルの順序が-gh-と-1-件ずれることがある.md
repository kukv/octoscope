---
id: TASK-21
title: 'api: ラベルの順序が gh と 1 件ずれることがある'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
  - detail
dependencies: []
references:
  - internal/github/api/lists.go
  - internal/github/cli/cli.go
priority: low
type: enhancement
ordinal: 21000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
REST は `created_at` を返さないので、`api/lists.go:60-61` は ID の昇順で作成順を近似している。cli は `gh label list`（GraphQL の CREATED_AT ASC）。microsoft/vscode の実測では、集合は一致したが順序が 1 件ずれた。

出典: phase4-rest-backend-followups:34-40, :70-100
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ラベル一覧を共通の GraphQL 文書（CREATED_AT ASC, first 100）で取り、両バックエンドで同じ順になる
<!-- AC:END -->
