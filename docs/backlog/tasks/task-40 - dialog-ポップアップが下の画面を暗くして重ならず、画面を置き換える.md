---
id: TASK-40
title: 'dialog: ポップアップが下の画面を暗くして重ならず、画面を置き換える'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
  - search
dependencies: []
references:
  - internal/app/presentation/tui/repo/render.go
  - internal/app/presentation/tui/dialog
priority: low
type: enhancement
ordinal: 40000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
モックアップは下の画面を暗くして中央にモーダルを重ねるが、実装は画面を丸ごと置き換える（`repo/render.go:33-36`。review・merge も同じ）。合成の仕組みが無く、入れるならポップアップ全部の描き方を変えることになる。

出典: phase4-repos-dialog-followups #1
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ダイアログを出している間も下の画面が暗く残り、中央に箱が重なる（golden で確かめる）
- [ ] #2 review・merge・保存クエリのポップアップも同じ仕組みで描く
<!-- AC:END -->
