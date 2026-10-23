# 承認と merge を、見ていた head のコミットに固定する

## 問題

セキュリティ監査 run-1 の要検証リード `gql/pr-writes-unbound-head-oid`。

- diff ビューの diff と、レビューのコンテキスト（`review.graphql`）は、開いたとき（と `r` を押したとき）にしか
  取り直さない。その間に PR の作成者が push しても、画面は古い diff のまま
- `review_at_once.graphql`（`addPullRequestReview`）と `start_review.graphql`（pending review の作成）は
  `commitOID` を渡さない。GitHub はその時点の head に対するレビューとして記録するので、見ていないコミットを
  承認したことになりうる
- `merge_pr.graphql`（`mergePullRequest`）と `enable_auto_merge.graphql`（`enablePullRequestAutoMerge`）は
  `expectedHeadOid` を渡さない。merge のポップアップを開いたあとに push されたコミットも merge される
- 「古い承認を取り消す」設定がどう効くかなど、GitHub 側の結果は未確認。ただし固定して困ることは無い。
  固定すれば、push 後の承認は古いコミットへのものとして記録され、merge は GitHub が拒否する

## 方針

- **head を読むクエリに `headRefOid` を足す。** `review.graphql` と `merge.graphql` の `pullRequest` に足す
- **ドメインに不透明なハンドルとして持たせる。** `domain.CommitHandle`（`string`）を作る（`.claude/rules/architecture.md`
  の「識別子は不透明な `string` ハンドル」）。`ReviewContext` と `ReviewTarget` に `Head CommitHandle` は使えない
  （`ReviewContext.Head` は既にブランチ名）ので `Commit CommitHandle` とする。`MergeContext` にも `Commit` を足す
- **書き込みに渡す。**
  - `addPullRequestReview` の 2 か所（一括のレビューと pending の作成）に `commitOID: $commitOID`
  - `mergePullRequest` と `enablePullRequestAutoMerge` に `expectedHeadOid: $expectedHeadOid`
  - `submitPullRequestReview` には該当する入力が無い。pending review は作成時のコミットに紐づく
- **どこから渡すか。** ビューは受け取った `ReviewContext` / `MergeContext` の `Commit` を、書き込みの
  `ReviewTarget` / merge の引数にそのまま載せる。merge の port の引数は、今の `PullRequestHandle` と同じ並びに
  `CommitHandle` を足す（引数が 4 つ以上になるなら構造体にまとめる。`.claude/rules/go-style.md`）
- **ずれたときの表示。** GitHub が head の不一致で拒否したら、そのエラーの原文を今までどおり出す
  （`.claude/rules/errors.md`。GitHub 側の失敗なので翻訳しない）。`r` で取り直せば新しい head で送れる
- **既知の限界。** diff（`gh pr diff` / REST の diff）とレビューのコンテキストは別のリクエストで取るので、
  その 2 つの間に push されると、diff は新しく、`Commit` は古い（またはその逆）ことがありうる。
  間隔は 1 回の往復ぶんで、閲覧と操作の間（分単位になりうる）を固定できる効果のほうが大きい。
  完全にするには diff をコミット範囲で取る必要があり、範囲外とする

却下: 書き込みの直前に head を取り直して比べる。取り直した時点の head は、利用者が見た diff の head ではない。

## タスク

各タスクは、まず失敗するテストを書き、実装して緑にし、検証している行を一時的に壊してテストが落ちることを
確かめてから戻す（`.claude/rules/testing.md`）。

1. **gql。** `review.graphql` と `merge.graphql` に `headRefOid` を足し、パース結果に入ることをテストする。
   testdata の録り直しが要るなら `bright-room-sandbox/octoscope-test-repository` で録り、`README.md` に日付を残す
   （`.claude/rules/testing.md`「外部レスポンスは実物を録る」。手で JSON を書き足さない）
2. **ミューテーション。** 一括レビュー・pending の作成・merge・auto-merge が、渡したコミットを変数として送ることを、
   fake transport で送られた変数を見て確かめる（監査の `headbind_harness` と同じ形）。
   テスト名は「なぜ要るか」で書く（例: `TestApprovalNamesTheCommitTheDiffShowed`）
3. **domain / gateway / usecase / tui。** `CommitHandle` を通す。domain のタグ検査と網羅テストが通ること。
   diff ビューの承認と、merge のポップアップの merge・admin merge・auto-merge が、コンテキストの `Commit` を
   渡すことを、フェイクの `Source` で確かめる
4. `make check`（lint の出力が `0 issues.` であることも見る）
5. **動かして確かめる。** テスト用リポジトリの PR #1 で、diff を開いたまま別の端末からブランチに空コミットを push し、
   `r` を押さずにコメントのレビュー（承認ではなく COMMENT）を送る。そのあと `reviews(last:1){commit{oid}}` が
   push 前のコミットであることを `gh api graphql` で確かめる。merge は実行しない
