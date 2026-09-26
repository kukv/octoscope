---
id: TASK-24
title: 'gql: 一覧が 0 件のときのデコードを守るテストが無い'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
  - repos
dependencies: []
references:
  - internal/github/gql
priority: low
type: chore
ordinal: 24000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
nodes が 0 件の fixture もテストも無く、nil を返すのか空スライスを返すのかが検証されていない。

出典: phase4-api-backend-followups #6
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 nodes が空の応答（合成でも可）で ListPRs と ListIssues がエラーなく 0 件を返すテストがある
<!-- AC:END -->
