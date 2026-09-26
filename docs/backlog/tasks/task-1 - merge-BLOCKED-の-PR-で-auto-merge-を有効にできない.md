---
id: TASK-1
title: 'merge: BLOCKED の PR で auto-merge を有効にできない'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - merge
dependencies: []
references:
  - internal/app/presentation/tui/merge/merge.go
  - docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md
priority: high
type: bug
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`merge/merge.go` の `send()` は `m.ctx.Block != BlockNone` の判定を `m.auto` より先に行うため、`mergeStateStatus: BLOCKED`（必須 check 待ちの典型）の PR で auto-merge にチェックしても enter が何もしない。CLEAN も対象外なので、auto-merge が使えるのは UNSTABLE など一部の状態に限られる。

設計 §4.4.4 の字面には沿っているが、auto-merge の用途（checks 通過を待ってマージ）と噛み合わない。どの Block で auto-merge を許すかを設計で決めてから直す。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 BLOCKED の PR で auto-merge を選んで enter すると enablePullRequestAutoMerge が送られる
- [ ] #2 直接マージは従来どおり BLOCKED で塞がれる
- [ ] #3 設計 §4.4.4 に auto-merge を許す状態が明記されている
<!-- AC:END -->
