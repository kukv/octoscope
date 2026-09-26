---
id: TASK-13
title: 'docs: 当初設計と実装の食い違いを設計書に反映する'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - docs
dependencies: []
references:
  - docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md
priority: low
type: docs
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
実装が正しいが設計に記録が無いもの:
- §4.5 スパークラインは見送り済み（phase1-followups）だが本文に要件として残る
- §3.2 gh もトークンも無いとき、エラー画面ではなく TUI 前に stderr へ出して exit 1（`cmd/octoscope/main.go`）
- §4.5 `--icons auto` / 未知の値は unicode ではなく次の候補（環境変数→設定）へ進む（`icon/icon.go`、テストあり）
- §4 `default_tab: repos` はカレントのリポジトリが無いと効かない（`root/root.go:321`）
- §4.4.1 diff の行に載らない同一ファイルのスレッドは末尾に出る

古い記述: `diff/render.go:43-44` のコメント「Work board drops its card borders」は §4.6（枠は外さない）と合わない。phase1-followups:160 も同様。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 上の各項目が設計書に反映されている
- [ ] #2 diff/render.go の古いコメントが直っている
<!-- AC:END -->
