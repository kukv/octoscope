---
id: TASK-29
title: 'root: 詳細・diff・checks の取得失敗が全画面エラーになる（設計 §4 と食い違い）'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - root
dependencies: []
references:
  - internal/app/presentation/tui/root/root.go
  - docs/superpowers/specs/2026-09-11-fetch-failures-design.md
priority: low
type: enhancement
ordinal: 29000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
fetch-failures-design §4 は「ErrGhNotFound と ErrUnauthenticated 以外は 1 行の通知に出す」とするが、オーバーレイは `failOverlay` → `showError` で全画面のエラーになる（`root/root.go:336-351, 556-560`）。esc で戻れるので、価値が低いとして見送った。

出典: fetch-failures-followups:17-26
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 オーバーレイの取得失敗が 1 行の通知で出る、またはオーバーレイは例外だと設計 §4 に明記されている
<!-- AC:END -->
