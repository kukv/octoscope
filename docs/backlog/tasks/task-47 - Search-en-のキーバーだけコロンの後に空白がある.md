---
id: TASK-47
title: 'Search: en のキーバーだけコロンの後に空白がある'
status: To Do
assignee: []
created_date: '2026-09-26 16:51'
labels:
  - search
  - i18n
dependencies: []
references:
  - internal/i18n/locales/active.en.yaml
priority: low
type: chore
ordinal: 47000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
en の `footer.search.*` は `j/k: field` の形で、他のタブ（`j/k:move`）や ja と揃っていない。ヒント 1 つにつき 1 桁多く使い、FitKeyBar が早くヒントを落とす。

出典: phase4-search-tab-followups #9
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 en の Search のキーバーを key:label の形に揃え、golden を録り直す
<!-- AC:END -->
