---
id: TASK-7
title: 'Search: マウス操作に対応する'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - search
  - mouse
dependencies: []
references:
  - internal/app/presentation/tui/search
  - internal/app/presentation/tui/root/mouse.go
priority: low
type: feature
ordinal: 7000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Search タブにはマウス処理が 1 つも無い（root は MouseMsg を渡しているが search 側が受け取らない）。設計 §4.0 の「行のクリックで選択・選択中の再クリックで詳細・ホイールでカーソル移動」の対象から漏れている。search-tab-followups #1 に記録済み。当たり判定は描画と同じ関数を読む。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 結果の行のクリックで選択、選択中の再クリックで詳細が開く
- [ ] #2 ホイールでカーソルが動く
<!-- AC:END -->
