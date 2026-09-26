---
id: TASK-14
title: 'checks: 失敗ステップのみの絞り込みが効かないのに見出しは failed steps と出る'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - checks
  - api
dependencies: []
references:
  - internal/github/api/joblog.go
  - internal/app/presentation/tui/checks/checks.go
  - internal/app/presentation/tui/checks/render.go
  - docs/superpowers/2026-09-13-phase4-actions-followups.md
priority: medium
type: enhancement
ordinal: 14000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
GitHub が run ログ zip にステップ別エントリを入れなくなったため、api バックエンドでは `failedOnly` に関わらずジョブ全体のログが全行 UNKNOWN STEP で返る（gh も同じ）。それでもペインの見出しは既定で "failed steps" と出るので、失敗ステップだけが表示されていると誤解させる。

案: jobs API の `steps[].started_at`/`completed_at` とログ行のタイムスタンプを突き合わせてステップ境界を復元する。

出典: phase4-actions-followups #2 #3、phase4-handover
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ステップ境界を復元できないときは、見出しか文言でジョブ全体のログだと分かる
- [ ] #2 （採用するなら）steps API の時刻からステップ名を付け直し、failedOnly で失敗ステップだけに絞れる
<!-- AC:END -->
