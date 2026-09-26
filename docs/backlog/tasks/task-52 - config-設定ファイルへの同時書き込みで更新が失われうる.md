---
id: TASK-52
title: 'config: 設定ファイルへの同時書き込みで更新が失われうる'
status: To Do
assignee: []
created_date: '2026-09-26 16:51'
labels:
  - config
dependencies: []
references:
  - internal/app/adapter/datasource/datasource.go
priority: medium
type: bug
ordinal: 52000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`SaveRepositories` と `SaveQueries` は load → 差し替え → save の read-modify-write で、Store に排他が無い（`datasource/datasource.go:57-79`）。picker で `x` を 2 回続けて押すなど、保存が並行すると片方の書き込みが失われる。rename が atomic なので、ファイル自体は壊れない。

出典: phase4-saved-queries-followups #5
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 続けて 2 回保存しても、最後の状態がファイルに残ることをテストで確かめる（直列化など）
<!-- AC:END -->
