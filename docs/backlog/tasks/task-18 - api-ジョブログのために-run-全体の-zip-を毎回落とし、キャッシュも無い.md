---
id: TASK-18
title: 'api: ジョブログのために run 全体の zip を毎回落とし、キャッシュも無い'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - checks
  - api
  - perf
dependencies: []
references:
  - internal/github/api/joblog.go
  - docs/superpowers/2026-09-13-phase4-actions-followups.md
priority: low
type: enhancement
ordinal: 18000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
1 ジョブのログを見るのに run 全体のログ zip を落とし、開き直すたびに取り直す（gh は `~/.cache/gh` にキャッシュする）。今の zip にはステップ別のエントリが無いので、run の zip を経由する利点もほぼ無い。

出典: phase4-actions-followups:106-116
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ステップ別のエントリが無いときは actions/jobs/{id}/logs だけで済ませる、またはキャッシュで開き直しの再取得をなくす
- [ ] #2 変更の前後で、同じジョブのログ表示が同じ内容になる
<!-- AC:END -->
