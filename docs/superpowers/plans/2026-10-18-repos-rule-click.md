# Repos: 区切り線の桁のクリックで何も起こさない（#173）

## 問題

`handleMouseClick`（`internal/app/presentation/tui/repo/mouse.go`）は `msg.X < sidebarWidth`
でサイドバーかを判定し、外れたら `msg.X -= m.sidebarCols()` で表の座標に直す。
`sidebarCols()` は `sidebarWidth + sidebarRule` なので、区切り線の桁（x = `sidebarWidth`）は
サイドバーの判定から漏れたうえで X = -1 になる。表の行の判定は Y しか見ないため、
区切り線をクリックするとその高さの行が選ばれ、フォーカスも表に移る。

## 方針

表の座標に直した直後に `msg.X < 0` なら何もせず返す。区切り線はどちらのペインでもない。
フォーカスも動かさない。

範囲外: 区切り線の上でのホイール（表がスクロールする）。害が無く、Issue の範囲でもない。

## タスク

1. テスト（赤）: サイドバーが出た状態で、x = `sidebarWidth`・表の 2 行目の y をクリックしても、
   カーソル・フォーカスが変わらず、コマンドも返らない
2. `mouse.go` に 1 行足して緑にする
3. `make check`
