# api バックエンドのための実測（2026-09-12）

Phase 4 スライス 4 の計画を書く前に、**推測ではなく実際の API 応答で**確かめたもの。
対象は自分の公開リポジトリ `kukv/octoscope`。設計
`docs/superpowers/specs/2026-09-08-phase4-design.md` §6 の前提を更新する。

## 1. REST では `gh.PR` を埋められない

`gh api 'repos/kukv/octoscope/pulls?per_page=1&state=all'` の返すキーに、次が**無い**。

| `gh.PR` のフィールド | REST の一覧（`/pulls`） | REST の単体（`/pulls/{n}`） |
|---|---|---|
| `Additions` / `Deletions` | **無い** | ある |
| `Review`（`reviewDecision`） | **無い** | **無い** |
| `Checks`（`statusCheckRollup`） | **無い** | **無い** |
| `Comments` | **無い**（別エンドポイント） | **無い**（別エンドポイント） |

`/issues` の一覧にも `body` はあるが `comments` は件数（数値）であって本文ではない。

**結論:** 設計 §6 の表が「`gh` のサブコマンドに依存している面」に挙げた
`pr list` / `issue list` / `repo view` は、REST では置き換えられない。
`api` 側は **`gql` に新しい `.graphql` 文書を足して**埋める。そうすると
完了条件 8（文書が 1 組のまま両方から使われ、`schema_test.go` が検証する）も
これらの経路について自動的に満たされる。

`gh pr list --json` 自体が内部で GraphQL を叩いているので、これは
「`gh` と同じことをする」だけであって新しい判断ではない。

## 2. 設計 §6 の表に載っていないサブコマンド依存が 5 つある

設計を書いた 2026-09-08 以降に足されたものを含む。**パリティの正本は
`internal/usecase/usecase.go` の `source` interface である。**

- `pr diff`（失敗時に `repos/{o}/{r}/pulls/{n}/files` へフォールバック）
- `search repos`（追加ダイアログの候補）
- `repo list`（初回投入）
- `api user/orgs`（org の一覧）
- `api repos/{o}/{r}/assignees`（担当者の候補）

## 3. Actions のログ: `gh run view --log` と `--log-failed` は別のものを読んでいる

録ってある fixture（`internal/gh/cli/testdata/job_log.txt` と `job_log_failed.txt`）の
1 行目を見ると、**`--log` はステップ名が `UNKNOWN STEP` になっており、
`--log-failed` には実際のステップ名が入っている。** 実際の API を叩いて理由が分かった。

### `GET /repos/{o}/{r}/actions/runs/{run_id}/logs`（zip、gh が使っているもの）

`kukv/octoscope` の CI 1 回分で **127,849 バイト**。中身は 2 段構成。

```
0_lint.txt                              ← ジョブ全体の平坦なログ（ステップの切れ目が無い）
lint/system.txt
lint/1_Set up job.txt                   ← ステップごと。<番号>_<ステップ名>.txt
lint/2_Run actions_checkout@3d3c42e…txt ← ステップ名の "/" は "_" に置換される
...
1_test.txt
test/5_Test.txt
```

- `--log` は平坦な `0_lint.txt` を読んでいる**と思われる**。だからステップが分からず
  `UNKNOWN STEP` になる
- `--log-failed` は `lint/<n>_<step>.txt` を読んでいる**と思われる**。だからステップ名が入る

**この 2 行だけは実測ではなく推定である。** 根拠は録ってある fixture の形（`job_log.txt` の
1 行目が `Current runner version` で始まり、ステップ名が全部 `UNKNOWN STEP`。
`job_log_failed.txt` には実際のステップ名が入っている）と、上の zip の構造が
その 2 つにちょうど対応すること。**4-4 の最初のタスクで `gh` の
`pkg/cmd/run/view/view.go` を読んで確かめる。** 確かめる前に実装方針を固めない。

### `GET /repos/{o}/{r}/actions/jobs/{job_id}/logs`（1 ジョブの平坦なログ）

リダイレクト先のプレーンテキスト。`kukv/octoscope` の test ジョブで **3,079 行**。
形は `<BOM><RFC3339Nano タイムスタンプ> <本文>` で、ANSI エスケープを含む。
**zip の `0_<job>.txt` と同じ内容。**

`gh api` はこれをそのまま出そうとすると
`the response contains terminal escape sequences; pass --allow-escape-sequences`
と言って止める（`gh` 側の安全装置であって、API の挙動ではない）。

### `GET /repos/{o}/{r}/actions/jobs/{job_id}`

`run_id` / `name` / `conclusion` / `html_url` / `started_at` と、
`steps[]`（`number` / `name` / `conclusion`）を返す。
**`JobLog` は `jobID` しか受け取らないので、zip を取るための `run_id` はここから引く。**

### 4-4 の実装方針（この実測から出る結論）

| 呼ばれ方 | api 側の手段 |
|---|---|
| `JobLog(..., failedOnly=false)` | `actions/jobs/{id}/logs` の平坦なテキスト 1 本。`gh` と同じく `Step` は `UNKNOWN STEP` |
| `JobLog(..., failedOnly=true)` | `actions/jobs/{id}` で `run_id` と失敗ステップを引き、`actions/runs/{run_id}/logs` の zip から `<job 名>/<番号>_<ステップ名>.txt` をステップ順に読む |

`parseJobLog`（`internal/gh/cli/checks.go`）はタブ区切りの 3 列を読む関数なので、
api 側は**その形に組み立てるのではなく、`gh.LogLine` を直接組む**ほうが素直である。
ステップ名の `/` → `_` 置換は zip 側の綴りなので、ファイルを探すときに同じ置換をかける。

## 4. 境界として記録しておくこと

`github.com` のみを相手にする。`gh` が見る `GH_HOST` / `GH_ENTERPRISE_TOKEN` は
`api` バックエンドでは見ない。GitHub Enterprise は Phase 4 の範囲外である。
