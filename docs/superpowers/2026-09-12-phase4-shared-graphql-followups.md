# 共通 GraphQL 層スライスの積み残し

Phase 4 スライス 4-1「共通 GraphQL 層」（`docs/superpowers/plans/2026-09-12-phase4-shared-graphql.md`、
Task 1〜5、全部完了）で見つかったが、このスライスでは直さなかったもの。
**直さないと決めた理由も書く。**

このスライスは `.graphql` 文書とその応答のデコードを `internal/gh/cli` から
`internal/gh/gql` へ動かしただけで、振る舞いは変えていない（4 コミット、
`43017c7`〜`07057f1`）。実バグは出ておらず、各タスクのレビューも
Critical/Important 0 件で通っている。

## 見つかったが直さなかったこと（理由つき）

### 1. 設計 §6 の表が実態と合わない（2 点、利用者の承認待ち）

`docs/superpowers/specs/2026-09-08-phase4-design.md` §6 の表は、`gh` の
サブコマンドに依存している面を `go-github` の REST で `api` 側に実装する、
と書いている。実際に GitHub API を叩いて確かめると、この前提が 2 か所で崩れる
（`docs/superpowers/2026-09-12-phase4-api-measurements.md` に実測がある）。

- **`pr list` / `issue list` / `repo view` は REST に置き換えられない。**
  `/pulls` の一覧には `additions` / `deletions` / `reviewDecision` /
  `statusCheckRollup` / `comments` が無く、単体の `/pulls/{n}` にも
  `reviewDecision` と `statusCheckRollup` が無い。この 3 つは
  **新しい `.graphql` 文書にして `gql` に足す**ほうが筋が良い
  （`gh pr list --json` 自体が内部で GraphQL を叩いているため、
  「`gh` と同じことをする」だけであって新しい判断ではない）
- **設計 §6 の表に載っていないサブコマンド依存が 5 つある。**
  `pr diff`、`search repos`、`repo list`、`api user/orgs`、
  `api repos/.../assignees`。設計を書いた 2026-09-08 以降に足されたもので、
  **パリティの正本は設計 §6 の表ではなく `internal/usecase/usecase.go` の
  `source` interface（100 行目）である**

**直さなかった理由:** どちらも設計文書そのものの書き換えが要る（§6 の表の
訂正と、§10 に「4 は 5 本に割った」の 1 行を足す）。文書の訂正は利用者の
承認事項であり、このスライスはコードを 1 行も変えない Task 5 の範囲では
提案するところまでしかできない。次のスライスに入る前に、この 2 点を
反映した §6 を承認してもらうこと。

### 2. `gql.Client.repoVars` がエラーに `name repository: ` という文脈を足す（意図して受け入れた差分）

移設前の `cli.repoArgs` は `repo %q has no owner/name separator` を素で
返していた。移設後の `gql.Client.repoVars`（`internal/gh/gql/gql.go`）は
`SplitRepoVars` / `RepoVars` の結果を `fmt.Errorf("name repository: %w", err)`
で包んでおり、エラー文言が変わっている。

**直さなかった理由:** この文言は `--repo` に不正な値を渡したときだけ見える
もので、`errors.Is` で分岐している箇所は無い。文脈が増えるぶん元より
分かりやすくなっており、戻す価値が無いと判断した。

## `internal/gh/cli` に残ったサブコマンド依存（4-3 / 4-4 が引き取るもの）

`internal/gh/gql` に移ったのは GraphQL の文書とデコードだけで、`cli` には
まだ `gh` のサブコマンドを直接呼ぶ処理が残っている。実際にコードを読んで
数えたもの（`internal/gh/cli/cli.go` / `diff.go` / `own_repos.go` /
`checks.go` / `search.go`）:

- **一覧・単体取得:** `pr list`（`ListPRs`）、`issue list`（`ListIssues`）、
  `pr view`（`GetPR`）、`issue view`（`GetIssue`）、`repo view`（`RepoName`）、
  `label list`（`ListLabels`）— 上の積み残し 1 番により、4-2 でこれらを裏で
  支える `.graphql` 文書が要る
- **書き込み系（`gh` のサブコマンドのまま）:** `pr comment` /
  `issue comment`（`AddPRComment` / `AddIssueComment`）、`pr close` /
  `reopen`、`issue close` / `reopen`、`pr edit` / `issue edit` の
  `--add-label` / `--remove-label` / `--add-assignee` /
  `--remove-assignee`（`editItems` 経由の `EditPRLabels` /
  `EditIssueLabels` / `EditPRAssignees` / `EditIssueAssignees`）
- **設計 §6 の表に無かった 5 つ**（積み残し 1 番で列挙したもの）:
  `pr diff`（`PRDiff`、失敗時に `api repos/.../pulls/{n}/files` へ
  フォールバックする `prFiles`）、`search repos`（`SearchRepos`）、
  `repo list`（`ListOwnRepos`）、`api user/orgs`（`ListOrgs`）、
  `api repos/.../assignees`（`ListAssignees`）
- **Actions:** `run view --job`（`JobLog`）、`run rerun`（`RerunWorkflow`）。
  ログの整形（`parseJobLog`）が 4-4 の一番重い部分になる見込み
  （下の「次のスライスに渡すもの」参照）
- **`gh` を経由しないもの:** `OpenWeb` は `internal/browser` を直接呼んでおり、
  `api` バックエンドでもそのまま使える

## 次のスライス（4-2 以降）に渡すもの

- **`gh.PR` / `gh.Issue` を埋める `.graphql` 文書はまだ無い。** 4-2 で
  `pr list` / `issue list` / `pr view` / `issue view` / `repo view` を
  置き換える文書を `gql` に足すことになる（積み残し 1 番）
- **`gql.Client.RepoVars` が `nil` のときは `SplitRepoVars`
  （`internal/gh/gql/gql.go`）に落ちる。** つまり「リポジトリ名を渡さずに
  済ませる」のは `cli` だけの特権であり（`gh` がカレントディレクトリの
  remote から `{owner}`/`{repo}` を埋めるため）、4-2 の `api` は必ず自分で
  owner/name を解決してから `RepoVars` に渡すこと
- **実測資料は必ず参照すること:**
  `docs/superpowers/2026-09-12-phase4-api-measurements.md`。上の 2 点に
  加えて、Actions のログの取り方に推定が 2 行だけ残っている
  （`gh run view --log` は zip の平坦なログ、`--log-failed` はステップごとの
  ログを読んでいる**と思われる**、というもの）。**4-4 の最初のタスクで
  `gh` の `pkg/cmd/run/view/view.go` を読んで確かめてから実装方針を固める**こと
- **設計 §6 の訂正と §10 への「4 は 5 本に割った」の 1 行は、利用者の承認待ちで
  まだ入れていない。** 4-2 に着手する前に承認を取ること（積み残し 1 番）
