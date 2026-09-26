---
id: TASK-35
title: 'Repos: RepoCounts が丸ごと失敗しても理由が画面に出ない'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
  - api
dependencies: []
references:
  - internal/app/presentation/tui/repo/repo.go
priority: low
type: enhancement
ordinal: 35000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`repo/repo.go:353-357` は、失敗時にエラーを捨てて `repoCountsMsg(nil)` を返す。認証切れやレート制限でも、全行のバッジが「—」になるだけで理由が出ない。

出典: phase4-repos-sidebar-followups #6、phase4-repos-dialog-followups #6
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 RepoCounts が失敗したとき、理由（原文）が通知の行などに出る
- [ ] #2 失敗とまだ取っていない状態を区別できることをテストで確かめる
<!-- AC:END -->
