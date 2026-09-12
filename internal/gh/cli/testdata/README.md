# testdata

ここのファイルは**実際の `gh` の出力を録ったもの**。手で書き足さない。
GitHub の返し方が変わったときに気づけることがこの testdata の目的なので、
テストを通すために中身を編集すると意味がなくなる。

録る対象は自分の公開リポジトリ（`kukv/octoscope`）に限る。秘密情報を入れない。

## `pr_files.json`

`gh` の実出力をそのまま録ったもの。録った日: 2026-09-07、対象: `kukv/octoscope`（PR #55）。

パースのテストは手書きの JSON ではなくこれを読む。手書きだと「GitHub が実際には
そう返さない形」でも通ってしまい、返し方が変わったときに気づけない。

```bash
D=internal/gh/cli/testdata
gh api 'repos/kukv/octoscope/pulls/55/files?per_page=100' --paginate | jq . > $D/pr_files.json
```

## `job_log.txt` / `job_log_failed.txt`

`JobLog` に対する実出力。`job_log.txt` は成功したジョブ全体のログを先頭 40 行に
切ったもの（パーサのテストに 251 行は要らないため）。それ以外は録ったままで、
先頭行の BOM も含めて手を入れていない。

```bash
D=internal/gh/cli/testdata
gh run view -R kukv/octoscope --job 88970766114 --log-failed > $D/job_log_failed.txt
gh run view -R kukv/octoscope --job 101635448466 --log | head -40 > $D/job_log.txt
```

## `job_log_in_progress.txt`

stderr of `gh run view --job <id> --log`（`--log-failed` も同じ文言）を、実際に
進行中のジョブに対して録ったもの。録った日: 2026-09-07、対象: `kukv/octoscope`
のジョブ 101759970990。標準出力には何も出ず、終了コードは 1。

```bash
gh run view -R kukv/octoscope --job <実行中のジョブ id> --log 2> \
  internal/gh/cli/testdata/job_log_in_progress.txt
```

## `search_repos.json`

`SearchRepos` に対する実出力。録った日: 2026-09-11。公開リポジトリの検索なので
伏せるものは無い。スター数は録った時点の値であり、テストは「どれかが 0 より大きい」
としか見ていないので、増えても落ちない。

```bash
gh search repos lipgloss --limit 5 --json fullName,stargazersCount,isPrivate \
  > internal/gh/cli/testdata/search_repos.json
```

## `own_repos.json`

`ListOwnRepos` に対する実出力。録った日: 2026-09-11。**`gh repo list` は private
リポジトリの名前も返すので、`jq` で公開ぶんだけに絞って先頭 5 件を残した。**
これは秘密情報の除去であって、テストを通すための編集ではない。

```bash
gh repo list --limit 100 --json nameWithOwner,isPrivate \
  | jq '[.[] | select(.isPrivate == false)][:5]' \
  > internal/gh/cli/testdata/own_repos.json
```

`ListOrgs` の録画は置いていない。所属 Org 名そのものが伏せる対象であり、
`--jq '[.[].login]'` が返すのは文字列の配列だけなので、テストはその形の
リテラルを `c.run` から返せば足りる。

`sample.diff` は `internal/gh/testdata` にある（README も同じ場所）。パース
そのものを見るテストが `internal/gh` に移ったので、録りものはそちらにしか
置かない。
