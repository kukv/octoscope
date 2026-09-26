---
id: TASK-6
title: 'Repos: どこからでも a でリポジトリ追加ダイアログを開く'
status: To Do
assignee: []
created_date: '2026-09-26 16:40'
labels:
  - repos
dependencies: []
references:
  - internal/app/presentation/tui/repo/repo.go
  - internal/app/presentation/tui/root/root.go
  - docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md
priority: medium
type: feature
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
設計 §4.2 は「どこからでも `a` で追加ダイアログをポップアップ」とするが、`a` は Repos タブ内でしか効かない（`repo/repo.go:481`）。root は q/1/2/3 以外を現在のタブへ回すだけ。見送った記録も無い。

詳細の `a`（担当者編集）など、他の画面の `a` と衝突しないかを先に確かめる。実装しないならその判断を設計に書く。
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Work / Search から a で追加ダイアログが開く、または設計に見送りが記録されている
<!-- AC:END -->
