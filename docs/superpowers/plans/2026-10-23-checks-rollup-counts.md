# Work・詳細・Repos: check の集計を 100 件で切らない（#166）

## 問題

Work 板・詳細・Repos 一覧の check の集計は、`statusCheckRollup.contexts(first: 100)` の nodes を数えて作る
（`internal/github/gql/work.graphql`・`pr.graphql`・`repo_prs.graphql`、`gateway/gh/checks.go` の
`toChecksFromContexts`）。contexts が 100 件を超える PR では、101 件目以降が `Total`・`Passed`・`Failed`・
`Running`・`State` のどれにも入らない。101 件目が失敗していても、カードは緑になりうる
（`docs/superpowers/2026-09-07-phase3-checks-handoff.md` D1）。

## 方針（利用者が決めたこと: 往復を増やさず、件数のフィールドで集計する）

GitHub の `StatusCheckRollupContextConnection` は、ページの外も含めた件数を返す。
2026-10-23 に `microsoft/vscode` の PR で実測し、`totalCount`・`checkRunCountsByState { state count }`・
`statusContextCountsByState { state count }` が全状態ぶん（0 件の状態も含めて）返ることを確かめた。

- 3 つの文書の `contexts(first: 100)` に `totalCount`・`checkRunCountsByState { state count }`・
  `statusContextCountsByState { state count }` を足す。nodes はそのまま（ドロワーに名前を出すため）
- `gql` に roll-up の型を 1 つ置き、`PullRequest` と `SearchItem` の `StatusCheckContexts()` を、
  nodes と件数を一緒に返す `StatusCheckRollup()` に置き換える。今は 2 つの型が同じ匿名構造体を別々に持っている
- `gateway/gh` の `toChecksFromContexts` は roll-up を受け取り、次のように組み立てる
  - `Runs` は今までどおり nodes から作る（最大 100 件）
  - `Passed`・`Failed`・`Running` は状態ごとの件数を足して作り、`Total` はその和にする。
    `totalCount` は使わない。`Total == Passed + Failed + Running` を保つためで、バー（`icon.ChecksBar`）と
    「N 件中」の文言はこの関係を前提にしている。`totalCount` は、和と一致することをテストで確かめるのに使う
  - 状態の振り分けは `toCheckState` と同じ規則にする。CheckRun の状態名（`CheckRunState`）は、
    完了していれば conclusion、完了していなければ status になる。そこで、conclusion を振り分ける部分を
    関数に切り出し、`toCheckState` と件数の両方から使う。成功（`SUCCESS`・`NEUTRAL`・`SKIPPED`）と
    失敗（`FAILURE`・`TIMED_OUT`・`CANCELLED`・`ACTION_REQUIRED`・`STARTUP_FAILURE`）以外は、
    今と同じく `Running` に数える。`COMPLETED`（conclusion の無い完了）と `STALE` もここに入る。
    今の `toCheckState` が default でそう扱っているためで、直感には反するが、挙動を変えない。
    StatusContext は `SUCCESS`、`FAILURE`/`ERROR`、それ以外の 3 つに分ける
  - `State` は今と同じく、件数から決める（失敗 > 実行中 > 成功、0 件なら `CheckNone`）
- ドロワーの「他 N 件」（`drawer.go` の `checksPane`）を `len(runs) - 表示件数` から
  `c.Total - 表示件数` に変える。100 件を超えた分もここに入るので、新しい文言は要らない
- checks ビュー（`checks.graphql`）は自分でページングしていて全件を持つので、変えない

限界: 名前の一覧は先頭 100 件しか持たないので、101 件目以降の失敗は、ドロワーの名前の欄には出ない。
件数（「N 件中 M 件成功・F 件失敗」）とカードの色には反映される。名前まで見たいときは checks ビュー（`c`）で見る。

却下: `totalCount` だけ取り、「100+」の印を出す。集計がずれたまま残る。

却下: PR ごとに残りのページを取りに行く。カード 1 枚ごとに往復が増える。これが D1 で見送った理由で、今もそのまま当てはまる。

## タスク

各タスクは、まず失敗するテストを書き、実装して緑にし、検証している行を一時的に壊してテストが落ちることを
確かめてから戻す（`.claude/rules/testing.md`）。

1. **文書と型。** 3 つの `.graphql` に件数を足す（`schema_test.go` が schema に照らして検査する）。`gql` に roll-up の型と
   `StatusCheckRollup()` を置き、`StatusCheckContexts()` を置き換える。`items_test.go`・`search_test.go` を直し、
   件数が読めることを確かめるテストを足す
2. **集計。** `checks_test.go` に:
   - nodes が 100 件で、件数が 137 件（うち失敗 1 件はページの外）のとき、`Total` 137・`Failed` 1・`State` 失敗、
     `Runs` は 100 件
   - `CheckRunState` の 14 値（`ACTION_REQUIRED`・`CANCELLED`・`COMPLETED`・`FAILURE`・`IN_PROGRESS`・`NEUTRAL`・
     `PENDING`・`QUEUED`・`SKIPPED`・`STALE`・`STARTUP_FAILURE`・`SUCCESS`・`TIMED_OUT`・`WAITING`。2026-10-23 の実測で
     返ってきた集合。`schema.json` には enum の値が載っていない）と、StatusContext の 5 値（`EXPECTED`・`ERROR`・
     `FAILURE`・`PENDING`・`SUCCESS`）が、それぞれ `toCheckState` と同じ側に数えられる
   - 既存の `TestToChecksFromContextsRollsUpTheRawRollup` を新しい入力の形に直す
3. **testdata を録り直す。** 古い testdata には件数が無く、`Total` が 0 に読まれる。タスク 2 の後にテストを流し、
   落ちた検査が読んでいる testdata（`work_section.json`・`pr.json`・`repo_prs.json` のうち該当するもの）を、
   `testdata/README.md` の手順（公開リポジトリだけを残す絞り込みも含む）で録り直す。README の録った日と理由を
   更新する。手で書き足さない。録り直したものでは、件数の和が `totalCount` と一致することも確かめる
4. **ドロワー。** `drawer_test.go` に、`Total` が `Runs` より多いとき「他 N 件」が `Total - 3` になるテストを足す。
   golden に差分が出たら、diff を目で見て理由を確かめてから録り直す
5. **ドキュメント。** handoff の D1 に、#166 で件数を全件にしたことと、名前は 100 件までという限界を 1 行で追記する
6. `make check`（lint の出力が `0 issues.` であることも見る）
7. 実際に起動して確かめる。contexts が 100 件を超える公開 PR を探し、`--repo` で Repos タブを開いて、
   一覧の check のバーが `gh api` で数えた件数と合うことを見る。その PR の詳細（件数の表示）も見る。
   Work 板は自分に関係する PR しか出ないので、そこに出せなければ集計のテストで代える。`--lang ja` の 80 桁でも見る。
   100 件を超える PR が見つからなければ、手元だけで文書を `contexts(first: 3)` に変えて起動する（コミットしない）。
   ページの大きさと件数が別々になる点は同じなので、check が 4 件以上ある任意の PR で、バーと
   「他 N 件」が `gh api` で数えた件数と合うことを確かめられる
