# diff: 行に載らないスレッドも畳む（#138）

## 問題

1. `diff/thread.go` の `orphanRows` は `Collapsed()` を見ず `key` も付けない。
   diff の行に載らないスレッドのうち resolved / outdated のものが全文で出て、
   `enter` でも畳めない。設計 §4.4.1「解決済みと outdated は畳んで件数だけ出す」に反する
2. `gateway/gh/review.go` の `toReviewThread` は `line` が null のとき `OriginalLine` を使う。
   `line` が null になるのは、スレッドが書かれた行が今の diff に無くなったとき。
   `originalLine` は**書かれた当時のコミットでの行番号**なので、今の diff の同じ番号の
   **無関係な行**にスレッドが付く

2 は導入コミット（97f51de）にも理由が書かれておらず、意図した挙動ではないと判断した。

## 方針

- **gateway:** `line` が null なら `Line` は 0（「今の diff に行が無い」）。diff の行番号は
  1 から始まるので、どの行にも一致せず末尾の「行が見つからない」節に落ちる。
  `domain.ReviewThread.Line` の doc にこの意味を書く
- **diff:** `orphanRows` は、置かれなかったスレッドの位置（path, line, side）ごとに
  既存の `threadRows(-1, ...)` を呼ぶ。畳み・件数・`key`・`enter` の開閉が
  行に載るスレッドと同じ経路になる。見出し「diff に無い行へのコメント」は今のまま先頭に 1 行

## タスク

1. gateway: `line: null` / `originalLine: 12` のスレッドが `Line == 0` になるテストを
   書いて赤 → 直して緑
2. diff: 行に載らない resolved スレッドが件数 1 行に畳まれ、`enter` で開き、もう一度で
   閉じるテストを書いて赤 → `orphanRows` を直して緑
3. `make check`。golden が変わったら理由を確かめる（末尾スレッドの golden があれば
   畳まれた形に変わるのは意図どおり）
