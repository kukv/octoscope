---
id: TASK-34
title: 'Repos: サイドバーを連打すると一覧の取得が行の数だけ並行に走る'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
  - perf
dependencies: []
references:
  - internal/app/presentation/tui/repo/repo.go
  - internal/app/presentation/tui/repo/mouse.go
priority: medium
type: bug
ordinal: 34000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`selectRow` は 1 行動くたびに `fetchList` を返し（`repo/repo.go:287-298`）、`fetchList` は `context.Background()` を使うので、前の取得がキャンセルされない（:331-333）。ホイールも同じ経路を通る。古い応答は gen で捨てるが、取得そのものは最後まで走る。go-style.md の context の規約（再取得では前の context をキャンセルする）にも反する。

出典: phase4-repos-sidebar-followups #7、phase4-repos-dialog-followups #5
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 カーソルが動いたら、前の行の取得の context がキャンセルされる（または間引かれる）ことをテストで確かめる
- [ ] #2 N 行を続けて移動しても、並行する取得が一定数以下に収まる
<!-- AC:END -->
