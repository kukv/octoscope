# detail: ja の 80 桁でも x と r のヒントを残す（#165）

## 問題

詳細ビューのキーバー（`internal/app/presentation/tui/detail/render.go` の `footerHints`）は、ja の 80 桁の open PR で
`esc:戻る  j/k:移動  c:コメント  v:レビュー  d:diff  s:checks  m:マージ`（70 桁）までしか出ない。
`x:クローズ` を足すと 82 桁、`r:更新` まで足すと 90 桁になり、`layout.FitKeyBar` が x と r を落とす。

## 方針

`j/k:移動` の優先度を `r` の後ろに下げる。詳細ビューの j/k は本文をスクロールするだけで、画面を見れば分かる。
ja 80 桁の open PR は `esc c v d s m x r` でちょうど 80 桁に収まる。en は 76 桁。

利用者と決めたこと:
- ja のラベルは縮めない（`x:閉じる` 案は、状態表示の「クローズ」と言葉がずれるので採らない）
- 2 段のキーバーや `?` ヘルプは、全ビューの高さ計算に及ぶ機能追加なので別 Issue にする

## タスク

1. テスト（赤）: ja・80 桁の open PR のキーバーに `x:クローズ` と `r:更新` がある
2. `footerHints` の順を変えて緑にする
3. `make golden`。変わるのが detail（と detail を描く画面）のキーバー行だけであることを確かめる
4. `make check`
