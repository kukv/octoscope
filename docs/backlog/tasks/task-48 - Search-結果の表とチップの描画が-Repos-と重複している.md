---
id: TASK-48
title: 'Search: 結果の表とチップの描画が Repos と重複している'
status: To Do
assignee: []
created_date: '2026-09-26 16:51'
labels:
  - search
  - repos
dependencies: []
references:
  - internal/app/presentation/tui/search/render.go
  - internal/app/presentation/tui/repo/render.go
priority: low
type: chore
ordinal: 48000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
設計は結果の表を Repos の右ペインと共有するとしているが、列の定数・行の窓・行の描画が 2 つのパッケージに複製されている（`search/render.go`、`repo/render.go`）。`labelChips` は `theme.Badges` と全く同じ実装のまま残っている。

出典: phase4-search-tab-followups #8
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 labelChips を theme.Badges に置き換える
- [ ] #2 列の定数と行の窓を共通にするか、共有しない理由を設計に書く
<!-- AC:END -->
