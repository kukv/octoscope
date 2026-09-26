---
id: TASK-17
title: 'cli: gh サブプロセスにタイムアウトが無い'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - perf
  - root
dependencies: []
references:
  - internal/github/cli/cli.go
  - internal/app/presentation/tui/root/root.go
  - docs/superpowers/2026-09-13-phase4-actions-followups.md
priority: medium
type: enhancement
ordinal: 17000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
api バックエンドには 16 秒の responseTimeout があるが、cli バックエンドは呼び出し側の ctx を `exec.CommandContext` に渡すだけで、呼び出し側も大半が `context.Background()`。gh がハングすると取得がいつまでも終わらない。

出典: phase4-actions-followups:125-130（gh のハングを想定した設計判断が要るとして見送り）
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 cli バックエンドの取得に上限時間があり、超えたらサブプロセスが終わり、一時的なエラーとして画面に出る
- [ ] #2 ログ取得など、長くかかる正当な操作が上限で切られない
<!-- AC:END -->
