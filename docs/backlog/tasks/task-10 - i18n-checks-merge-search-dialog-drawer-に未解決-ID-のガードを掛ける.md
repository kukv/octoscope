---
id: TASK-10
title: 'i18n: checks / merge / search / dialog / drawer に未解決 ID のガードを掛ける'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - i18n
  - test
dependencies: []
references:
  - internal/i18n/unresolved.go
priority: low
type: chore
ordinal: 10000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`i18n.AssertNoUnresolvedIDs` は work / detail / review / diff / repo / root でしか呼ばれていない。golden は `!id` をそのまま記録するので、録り直すと ID の打ち間違いが通り得る。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 5 パッケージのテストが en / ja で AssertNoUnresolvedIDs を呼ぶ
<!-- AC:END -->
