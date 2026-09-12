# testdata

GitHub の REST API から実際に録った応答。手書きしない。
テストを通すために編集しない（落ちたら実装を疑う）。

| ファイル | 録り方 | 録った日 | 対象 |
|---|---|---|---|
| `labels.json` | `gh api 'repos/cli/cli/labels?per_page=100'` | 2026-09-13 | `cli/cli`（83 件） |
| `assignees.json` | `gh api 'repos/cli/cli/assignees?per_page=100' --jq '[.[] | {login, id}]'` | 2026-09-13 | `cli/cli` |

`assignees.json` は `jq` で `login` と `id` に絞ってある。生の応答には
プロフィール URL とアバター URL が並び、録りものに残す理由が無い。
