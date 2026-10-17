# diff: 破棄の途中に取り直したスレッドを送信済みとして出さない（#203）

## 問題

pending review を破棄（`X` → `y`）すると、破棄したコメントが「not sent yet」の印を失って
送信済みとして残ることがある。`r` で取り直すと消える。

`deletePullRequestReview` が返ってからおよそ 1 秒のあいだ、GitHub の `review.graphql` への応答は
次の形になる（2026-10-18、`bright-room-sandbox/octoscope-test-repository` #2 で、破棄と並行して
0.6 秒おきに問い合わせて録った。破棄の完了後の問い合わせでもこの形が返った）。

- `reviews(states: [PENDING])` はもう空
- `reviewThreads` には破棄したスレッドが残り、コメントの `pullRequestReview` が `null`

`discarded`（`internal/app/presentation/tui/diff/diff.go`）は破棄が終わるとすぐに `fetchReview` を呼ぶので、
この窓に当たることがある。`gql.ThreadComment.PullRequestReview` は値の struct なので `null` が
`State: ""` になり、`toThreadComment`（`internal/app/adapter/gateway/gh/review.go`）は
`State == "PENDING"` でないコメントを送信済みとみなす。取り直しはそれ以上起きないので残り続ける。

## 方針

所属する review が無いコメントは、破棄の途中にあるものとして gateway で捨てる。これは GitHub の
返し方の事情なので、翻訳の層（ACL）に置く。

1. `gql.ThreadComment.PullRequestReview` をポインタにする。ワイヤ型は応答に忠実にし、
   `null` と実在する review を区別できるようにする
2. `toReviewThread` で `PullRequestReview == nil` のコメントを捨てる。捨てるのはコメント単位
   （送信済みのコメントに未送信の返信が付いたスレッドで、送信済みのほうを残すため）
3. コメントが 1 件も残らないスレッドは `toReviewContext` で捨てる

却下:
- `null` を未送信とみなす。もう存在しない review のコメントを「not sent yet」として出し続けることになる
- 破棄のあとで少し待ってから取り直す。窓の長さは GitHub 次第で、待つ時間を決められない

範囲外: 取り直しの応答が前後して着く問題（`reviewArrived` には世代の区別が無い）。理屈の上では
起こり得るが、今回の原因ではない。

## タスク

1. 破棄の途中に録った応答を `internal/github/gql/testdata/review_context_discarding.json` に置く。
   testdata の `README.md` に、録った日・対象・録り方（`deletePullRequestReview` と並行して
   `review.graphql` を繰り返し問い合わせ、`reviews` が空でスレッドが残る応答を選ぶ）を書く。
   README の「録る対象は `kukv/octoscope` に限る」に、公開のテスト用リポジトリ
   `bright-room-sandbox/octoscope-test-repository` を加える
2. テスト（赤）: gql のパーステストで、録った応答のコメントの `PullRequestReview` が nil になる
3. テスト（赤）: gateway で、review の無いコメントだけのスレッドが消える。review のあるコメントと
   無いコメントが混ざったスレッドでは、review のあるコメントだけが残る。gateway のテストは
   `gql.ReviewContext` を受け取るので、録った応答（gql の非公開の応答型でしか読めない）ではなく
   2 で確かめた形（`PullRequestReview == nil`）を組んで渡す
4. ポインタにして、`toReviewThread` / `toReviewContext` を直して緑にする。既存の
   `review_test.go` の `PullRequestReview.State = ...` はポインタに合わせて書き換える。
   捨てる処理を一時的に外して、3 のテストが落ちることを確かめる
5. `make check`
6. 実際に起動して確かめる。テスト用リポジトリの #2（作者は別アカウント）で、コメントを付けてから
   diff を開き直し、`X` → `y` で破棄して、コメントと `●1` が消えることを見る。タイミングに
   左右されるので数回繰り返す
