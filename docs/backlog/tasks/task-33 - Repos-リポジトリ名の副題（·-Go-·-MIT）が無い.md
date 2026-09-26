---
id: TASK-33
title: 'Repos: リポジトリ名の副題（· Go · MIT）が無い'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - repos
dependencies: []
references:
  - internal/app/presentation/tui/repo
  - docs/superpowers/2026-09-06-phase1-followups.md
priority: low
type: feature
ordinal: 33000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
モックアップにある、リポジトリ名の横の主言語とライセンスの副題が出ていない。primaryLanguage も licenseInfo も取得していない。Phase 1 では別のサブプロセスが要るとして範囲外にしたが、今は既存の GraphQL クエリに相乗りできる可能性がある。

出典: phase1-followups:172
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Repos のヘッダか選択行に、主言語とライセンスが出る
- [ ] #2 リクエストを増やさない（既存のクエリに相乗りする）か、増やすならその理由が記録されている
<!-- AC:END -->
