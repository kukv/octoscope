---
id: TASK-20
title: 'api: タイムアウトが本文のダウンロードには掛からない'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
  - perf
dependencies: []
references:
  - internal/github/api/api.go
priority: low
type: enhancement
ordinal: 20000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`api/api.go:80-82` にあるのは DialContext・TLSHandshakeTimeout・ResponseHeaderTimeout だけ。ヘッダが返った後に回線が細くなると、本文（特にログ zip）を待ち続ける。`http.Client.Timeout` は大きい zip が途中で切れる恐れがあるので、意図的に採らなかった。

出典: phase4-actions-followups:117-124
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 本文の読み取りが一定時間進まなければ打ち切る（全体の時間ではなく、進まない時間で判定する）
- [ ] #2 大きくても進み続けているダウンロードは切られない
<!-- AC:END -->
