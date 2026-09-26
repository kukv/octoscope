---
id: TASK-16
title: 'api: ポートを明示した ssh:// の remote を github.com ではないとして拒む'
status: To Do
assignee: []
created_date: '2026-09-26 16:50'
labels:
  - api
  - repos
dependencies: []
references:
  - internal/github/api/repo.go
  - docs/superpowers/2026-09-12-phase4-api-backend-followups.md
priority: medium
type: bug
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`ssh://git@github.com:22/owner/repo.git` は git が実際に書く形だが、`api/repo.go:91` の `CutPrefix(s, host+"/")` に一致しない。そのため api バックエンドではカレントリポジトリの解決に失敗し、Repos タブに出ない。

出典: phase4-api-backend-followups #4、phase4-rest-backend-followups:41-43
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ssh://git@github.com:22/owner/repo(.git) から owner/repo が取れる
- [ ] #2 github.com 以外のホスト（例: gitlab.com:22）は引き続き拒否される
<!-- AC:END -->
