---
id: TASK-36
title: 'Repos: errMsg が取得・ブラウザ・保存を兼ね、o の失敗が捨てられうる'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
dependencies: []
references:
  - internal/app/presentation/tui/repo/repo.go
priority: low
type: bug
ordinal: 36000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`errMsg` は一覧の取得・`o`・保存の 3 種類を 1 つの型で運ぶ（`repo/repo.go:78-83`）。gen による破棄が `noticeOpen` にも掛かるので（:452）、`o` の結果が返る前にサイドバーが動くと、ブラウザの失敗が捨てられる。errors.md の「表示場所ごとにメッセージ型を分ける」にも合わない。

出典: phase4-repos-sidebar-followups #9、phase4-repos-dialog-followups #4
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 o の失敗は、サイドバーのカーソルが動いても捨てられない（テストで固定する）
- [ ] #2 失敗の種類ごとにメッセージ型を分けるか、分けない理由を規約に書く
<!-- AC:END -->
