# api: 応答の本文を、エラーの判定のために 2 回デコードしない（#158）

## 問題

`internal/github/api/transport.go` の `statusError` は、ステータスを見る前に、本文を必ず `errorsBody` に
`json.Unmarshal` する。GraphQL の `classify` は、`statusError` が nil を返した（status < 400 の）あとで、
部分応答の `errors` を見るために同じ本文をもう一度デコードする。

そのため、GraphQL の成功した応答は、毎回本文を丸ごと 2 回パースしている。REST の成功した応答も
`statusError` で 1 回パースしているが、status < 400 では結果を使わないので、このパースはまるごと無駄になっている
（出典: phase4-rest-backend-followups #1）。

## 方針

**`statusError` の先頭で、status < 400 なら本文を読まずに nil を返す。**

`statusError` が本文を使うのは status >= 400 の分岐だけなので、挙動は変わらない。その結果、次のようになる。

- GraphQL の成功（200）: `classify` の 1 回だけ
- GraphQL・REST の失敗（>= 400）: `statusError` の 1 回だけ
- REST の成功: 0 回

署名は変えない（`rest.go:64` と `transport.go:71` の呼び出しはそのまま）。

却下: `classify` で 1 回デコードして、`statusError` にメッセージを渡す形に変える。REST の成功時の無駄なパースが
残るうえ、署名が変わって `rest.go` 側も触ることになる。

**新しいテストは足さない。** 「デコードが 1 回であること」は `json.Unmarshal` を差し替えないと観測できず、
そのための差し替え点を足すのは割に合わない。挙動が変わっていないことは、既存のテストで確かめる。
502 / 503 / 504（`TestAGatewayFailureIsTransient`）、401（`TestABadCredentialIsUnauthenticated`）、
200 の部分応答（`TestAPartialAnswerComesBackWithItsBody`）、その他の 4xx（`TestAnOrdinaryFailureKeepsWhatGitHubSaid`）、
REST の再試行（`rest_test.go`）が、`statusError` と `classify` の分岐を一通り通っている。

## タスク

1. `statusError` の先頭に `if status < http.StatusBadRequest { return nil }` を置き、末尾の `return nil` と
   `case status >= 400:` を整理する（後者は `default:` になる。全分岐が `status` の値だけで決まるので、
   staticcheck QF1002 に従い `switch status` にする）。`classify` は触らない
2. 空振りの確認: 早期 return を一時的に `status < 600` に変えて、上の 4xx / 5xx のテストが落ちることを見てから戻す
   （早期 return の位置と条件が、既存のテストで守られていることの確認）
3. `make check` を通し、`perf(api): ...`（`Closes #158`）でコミットする
