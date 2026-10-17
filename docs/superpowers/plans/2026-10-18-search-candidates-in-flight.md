# Search: 取得中の候補をもう一度要求しない（#181）

## 問題

`maybeFetchCandidates`（`internal/app/presentation/tui/search/search.go`）は、取得を終えて覚えた
`labelCandidatesRepo` / `authorCandidatesRepo` しか見ない。応答が届く前に label / author の行を離れて
戻ると、同じリポジトリの取得をもう一度投げる。

## 方針

種類ごとに取得中のリポジトリ（`labelFetching` / `authorFetching`）を持つ。

- 要求: 取得済みか取得中が同じリポジトリなら何もしない。そうでなければ取得中に記録して投げる
- 応答: `msg.repo` が取得中と同じなら取得中を空に戻す。そのあと今までどおり古い応答を捨て、結果を覚える
- 失敗: 今は `nil` を返して捨てているので、取得中の印が残ってそのリポジトリを二度と取りに行かなくなる。
  `labelCandidatesMsg` / `authorCandidatesMsg` に `err` を持たせ、失敗なら取得中を空にするだけにする。
  失敗は今までどおり画面に出さない。次にその行に来たとき、今までどおりもう一度取りに行く

却下: 要求した時点で `labelCandidatesRepo` に記録する案。欄は増えないが、失敗したリポジトリを二度と取りに行かなくなる。

## タスク

1. テスト（赤）:
   - 応答の前に label → 別の行 → label と動いても、2 度目は取得を投げない
   - 取得中に別のリポジトリへの古い応答が届いても、取得中の印は消えない
2. テスト: 失敗の応答のあと label に戻ると、もう一度取りに行く（今も通るが、修正で壊さないことを固定する）
3. 実装して緑にする。`resolveCandidates` は着地で投げた取得を捨てて `maybeFetchCandidates` を
   もう一度呼んでいた（2 度目を投げなくなると成り立たない）。着地する行を引数に取り、
   着地が返した取得をそのまま解決する形に直す
4. `make check`
