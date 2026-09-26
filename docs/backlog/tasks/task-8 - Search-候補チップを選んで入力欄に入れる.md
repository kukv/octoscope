---
id: TASK-8
title: 'Search: 候補チップを選んで入力欄に入れる'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - search
dependencies: []
references:
  - internal/app/presentation/tui/search/render.go
  - internal/app/presentation/tui/search/search.go
priority: low
type: feature
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
label / author の候補チップは表示されるが、選ぶ操作が無く表示だけになっている（search-tab-followups #6、brief 範囲外として見送り）。設計 §4.3「label や author は候補をチップとして提示する」。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 チップを選ぶと該当フィルタに値が入る
<!-- AC:END -->
