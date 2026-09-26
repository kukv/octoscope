---
id: TASK-41
title: 'dialog: 初回投入の候補にスクロールも上限も無く、端末の高さを超える'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
dependencies: []
references:
  - internal/app/presentation/tui/dialog/render.go
  - internal/app/usecase/repos.go
priority: medium
type: bug
ordinal: 41000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`g` の初回投入は、自分と所属 Org ごとに最大 100 件の候補を全部縦に並べる（`dialog/render.go:47`、`usecase/repos.go:26`）。箱が端末より高くなり、上辺やキーバーが画面の外に出うる。保存クエリの picker には窓切りが入っているが、dialog には無い。

出典: phase4-repos-dialog-followups #8、phase4-search-foundation-followups、phase4-saved-queries-followups #4
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 候補が多くてもダイアログが端末の高さに収まり、カーソルに合わせてスクロールする
- [ ] #2 窓の計算を search の picker と共通にするか決める
<!-- AC:END -->
