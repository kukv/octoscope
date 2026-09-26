---
id: TASK-30
title: 'detail: ja の 80 桁でキーバーから x と r のヒントが落ちる'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - i18n
  - detail
dependencies: []
references:
  - internal/app/presentation/tui/detail
  - docs/superpowers/2026-09-08-phase3-merge-handoff.md
priority: low
type: enhancement
ordinal: 30000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
80 桁の日本語では、`m:マージ` の場所を空けるために `x:クローズ` と `r:更新` のヒントが落ちる（`detail/testdata/detail_ja_80.golden`）。キー自体は効く。

出典: phase3-merge-handoff:209-212（分かっていて直さない、とされた）
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ja の 80 桁の詳細ビューで、x と r を含む主要キーのヒントが見える
- [ ] #2 golden が更新されている
<!-- AC:END -->
