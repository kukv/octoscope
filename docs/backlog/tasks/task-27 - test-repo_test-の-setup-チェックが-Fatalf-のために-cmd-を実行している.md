---
id: TASK-27
title: 'test: repo_test の setup チェックが Fatalf のために cmd() を実行している'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
  - test
dependencies: []
references:
  - internal/app/presentation/tui/repo/repo_test.go
priority: low
type: chore
ordinal: 27000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`repo/repo_test.go:427` は、setup が崩れたことを報告するためだけに、Fatalf の引数で `cmd()` を実行している。fakeSource しか呼ばないので実害は無い。

出典: fetch-failures-followups #3
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Fatalf が cmd を実行せずに報告する
<!-- AC:END -->
