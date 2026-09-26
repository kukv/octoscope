---
id: TASK-9
title: 'review: 未提出コメント 0 件の pending review がヘッダに出ない'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - diff
  - review
dependencies: []
references:
  - internal/app/presentation/tui/diff/render.go
priority: low
type: bug
ordinal: 9000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
ヘッダの `pending · N` は未提出コメントが 1 件以上のときだけ出る（`diff/render.go:271`）。コメント 0 件の pending review だけがある場合は書きかけの存在が見えない。設計 §4.4.2「書きかけが存在することを隠さない」。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 コメント 0 件の pending review でもヘッダに pending と出る
<!-- AC:END -->
