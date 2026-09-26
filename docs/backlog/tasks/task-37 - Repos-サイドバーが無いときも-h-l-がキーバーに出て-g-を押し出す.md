---
id: TASK-37
title: 'Repos: サイドバーが無いときも h/l がキーバーに出て g を押し出す'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
  - i18n
dependencies: []
references:
  - internal/app/presentation/tui/repo/render.go
priority: low
type: enhancement
ordinal: 37000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`repo/render.go:145-160` の `footerHints` は、`sidebarCols()` を見ずに `footer.list.pane` を入れる。80 桁や行が 0 件でサイドバーが畳まれていても `h/l:pane` が出る。ja の 80 桁では、末尾の `g:取り込み` が落ちる。

出典: phase4-repos-sidebar-followups #8、phase4-repos-dialog-followups #2
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 sidebarCols()==0 のとき、キーバーに h/l が出ない（golden を録り直す）
- [ ] #2 ja の 80 桁で行が 0 件のとき、g のヒントが残る
<!-- AC:END -->
