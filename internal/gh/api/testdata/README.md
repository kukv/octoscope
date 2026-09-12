# testdata

GitHub の REST API から実際に録った応答。手書きしない。
テストを通すために編集しない（落ちたら実装を疑う）。

| ファイル | 録り方 | 録った日 | 対象 |
|---|---|---|---|
| `labels.json` | `gh api 'repos/cli/cli/labels?per_page=100'` | 2026-09-13 | `cli/cli`（83 件） |
| `assignees.json` | `gh api 'repos/cli/cli/assignees?per_page=100' --jq '[.[] | {login, id}]'` | 2026-09-13 | `cli/cli` |
| `pr_diff.txt` | `gh api repos/kukv/octoscope/pulls/66 -H 'Accept: application/vnd.github.v3.diff'` | 2026-09-13 | `kukv/octoscope#66`（8 ファイル） |
| `pr_files.json` | `gh api 'repos/kukv/octoscope/pulls/66/files?per_page=100'` | 2026-09-13 | `kukv/octoscope#66`（`--jq` で絞らない。`patch` の欠落がそのまま録られている必要がある） |
| `search_repos.json` | `gh api 'search/repositories?q=octoscope&per_page=5'` | 2026-09-13 | 公開検索結果（`--jq` で絞らない。伏せるものが無い） |
| `user_repos.json` | `gh api 'user/repos?affiliation=owner&sort=pushed&direction=desc&per_page=100' --jq '[.[] \| select(.private==false) \| {full_name, private}] \| .[0:5]'` | 2026-09-13 | `kukv` 自身の公開リポジトリ 5 件（private なリポジトリ名が残っていたため公開のみに絞り直した） |
| `user_orgs.json` | `gh api user/orgs --jq '[.[] \| {login}]'` | 2026-09-13 | `kukv` の所属 Org（1 件） |

`assignees.json` は `jq` で `login` と `id` に絞ってある。生の応答には
プロフィール URL とアバター URL が並び、録りものに残す理由が無い。
