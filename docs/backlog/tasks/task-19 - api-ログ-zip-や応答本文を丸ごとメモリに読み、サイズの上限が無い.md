---
id: TASK-19
title: 'api: ログ zip や応答本文を丸ごとメモリに読み、サイズの上限が無い'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
  - checks
  - perf
dependencies: []
references:
  - internal/github/api/rest.go
  - internal/github/api/transport.go
  - internal/github/api/joblog.go
priority: low
type: enhancement
ordinal: 19000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`io.ReadAll(resp.Body)` に上限が無い（`api/rest.go:66`、`api/transport.go:69`）。zip は全体を `bytes.NewReader` に載せ（`joblog.go:80`）、各エントリも `io.ReadAll` で読む（:150）。アーカイブ・展開後のエントリ・`[]LogLine` が同時にメモリに載る。

出典: phase4-actions-followups:152-164（規模を測っていないとして見送り）
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 大きい matrix run のアーカイブサイズを測り、記録する
- [ ] #2 測った値を根拠に上限（または一時ファイル化）を入れ、超えたら分かるエラーを返す
<!-- AC:END -->
