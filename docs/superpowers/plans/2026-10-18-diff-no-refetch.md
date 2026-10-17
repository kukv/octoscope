# diff: 書き込みのたびに全体を取り直さない（#227）

設計: `docs/superpowers/specs/2026-10-18-diff-no-refetch-design.md`

#226（#203）の上に積む。#226 で入れた `gql.CommentReview` と、レビューが `null` のコメントを捨てる処理を使う。
#226 がマージされたら main に rebase する。

## タスク

1. **録る。** テスト用リポジトリ #2 で、設計 §3.1 の `add_thread.graphql` を実際に送った返事を
   `internal/github/gql/testdata/add_thread.json` に置く（2026-10-18 に録り済み。録ったあとレビューは破棄した）。
   testdata の `README.md` に、録った日・対象・手順を書く
2. **gql。** テスト（赤）: 録った返事を `AddReviewThread` が読むと、コメント 1 件・`PullRequestReview.State == "PENDING"`
   のスレッドを返す。`add_thread.graphql` を書き換え、`AddReviewThread` が `(ReviewThread, error)` を返すようにして緑にする。
   `schema_test.go` が選んだフィールドをスキーマと照合するので、それも通す
3. **gateway。** テスト（赤）: `AddReviewThread` が、backend の返したスレッドを変換して返す（pending review が
   無いときに `StartReview` を先に呼ぶ既存の振る舞いは変えない）。`backend` の `reviewer` と `Gateway.AddReviewThread`
   を直して緑にする
4. **domain。** テスト（赤）: `ReviewContext.Discarded` / `Submitted` の表テスト（設計 §5）。元の値が書き換わらないこと。
   実装して緑にする
5. **usecase。** `reviewer.AddReviewThread` と `PostLineComment` をスレッドも返す形にする。既存のテストを合わせる
6. **diff。** テスト（赤）: フェイクの `Source` に `PRReviewContext` の呼び出し回数を数えさせる。
   - 送信: `c` → 入力 → `Ctrl+S` のあと、回数が増えず、`View()` に送ったコメントと「not sent yet」が出る
   - 破棄: `X` → `y` のあと、回数が増えず、未送信のコメントが消え、送信済みのコメントは残る
   - 送信: `v` → 送ったあと、回数が増えず、「not sent yet」が消える

   `commentPosted` / `discarded` / `submitDone` を直して緑にする。それぞれ `fetchReview` を呼ぶように一時的に戻し、
   回数のテストが落ちることを確かめる
7. root など、`PostLineComment` のフェイクを持つテストを新しい形に合わせる
8. `make golden`（変わらない見込み）/ `make check`
9. 実際に起動して確かめる。テスト用リポジトリ #2 で、コメントを 3 件続けて付けてすぐ破棄し、消えることを見る。
   レビューを送って「未送信」が外れることも見る（#2 にコメントが公開されるので、送るのは `COMMENT` にする）。
   `--lang ja` と 80 桁でも見る
