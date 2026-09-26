---
id: TASK-26
title: 'Repos: o の失敗通知を利用者が消す手段が無い'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
dependencies: []
references:
  - internal/app/presentation/tui/repo/repo.go
  - docs/superpowers/2026-09-11-fetch-failures-followups.md
priority: low
type: enhancement
ordinal: 26000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
ブラウザを開けなかったときの通知（URL を載せた 1 行）は、`r` や再取得では意図的に消さない。消えるのはサイドバーで別の行へ移ったときだけ（`repo/repo.go:292`）なので、リポジトリが 1 つだけだと消す手段が無い。

出典: fetch-failures-followups #2、phase4-repos-dialog-followups #6
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 専用の操作（例: esc）で o の失敗通知を消せる、または消さないという判断が設計に記録されている
- [ ] #2 取得の成功では引き続き消えない。リポジトリ 1 件でも消せることをテストで確かめる
<!-- AC:END -->
