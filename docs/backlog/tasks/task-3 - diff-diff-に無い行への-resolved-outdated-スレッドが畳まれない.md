---
id: TASK-3
title: 'diff: diff に無い行への resolved/outdated スレッドが畳まれない'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - diff
dependencies: []
references:
  - internal/app/presentation/tui/diff/thread.go
  - internal/app/adapter/gateway/gh/review.go
priority: medium
type: bug
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`diff/thread.go` の `orphanRows` は `Collapsed()` を見ず key も付けないため、diff の行に載らないスレッドのうち resolved/outdated のものが全文で出て、enter でも畳めない。設計 §4.4.1「解決済みと outdated は畳んで件数だけ出す」に反する。

関連: `gateway/gh/review.go` の `toReviewThread` は `line` が null のとき `OriginalLine` を使うため、outdated スレッドが今の diff の無関係な行に付く可能性がある（未検証）。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 末尾に出るスレッドも resolved/outdated は畳まれ、enter で開閉できる
- [ ] #2 OriginalLine が別の行に重なるケースを確かめ、必要なら直す
<!-- AC:END -->
