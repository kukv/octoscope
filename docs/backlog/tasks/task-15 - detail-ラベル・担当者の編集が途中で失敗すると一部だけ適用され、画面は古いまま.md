---
id: TASK-15
title: 'detail: ラベル・担当者の編集が途中で失敗すると一部だけ適用され、画面は古いまま'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
  - detail
dependencies: []
references:
  - internal/github/api/items.go
  - internal/app/presentation/tui/detail/update.go
  - docs/superpowers/2026-09-13-phase4-rest-backend-followups.md
priority: medium
type: bug
ordinal: 15000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
api バックエンドは POST で追加してから、ラベルごとに DELETE する（`api/items.go:73-93`）。追加の後に削除が失敗すると、GitHub には追加だけが残る。失敗時は `pickFailed` がエラーを出すだけで再取得しないため、画面は編集前のまま。

出典: phase4-rest-backend-followups:28-30、phase4-handover
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 適用に失敗したときも詳細を再取得し、GitHub 上の実際のラベル・担当者が表示される
- [ ] #2 エラー表示から、一部だけ適用された可能性が読み取れる
<!-- AC:END -->
