---
id: TASK-22
title: 'api: nextLink が Link ヘッダを「,」で分割するので、URL に「,」があると壊れる'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
dependencies: []
references:
  - internal/github/api/rest.go
priority: low
type: bug
ordinal: 22000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`api/rest.go:170` は `strings.Split(h.Get("Link"), ",")` で分割している。見送った当時は到達しないとしたが、今は ListLabels も walkPages を通るので、影響を受ける読み手が増えている。

出典: phase4-rest-backend-followups:31-33
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 <...> の中の「,」を区切りとして扱わずに Link ヘッダを解析する
- [ ] #2 「,」を含む next URL のテストがある
<!-- AC:END -->
