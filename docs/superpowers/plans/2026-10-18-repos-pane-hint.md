# Repos: サイドバーが無いときはキーバーに h/l を出さない（#172）

## 問題

`footerHints`（`internal/app/presentation/tui/repo/render.go`）は `footer.list.pane` を常に入れる。
ところが `h` は `sidebarCols() > 0` のときしか効かない（`repo.go` の `case "h", "left"`）。
100 桁未満、またはリポジトリが 0 件でサイドバーが畳まれていると、押しても何も起きないヒントが
1 つ分の幅を取り、ja の 80 桁では末尾の `g:取り込み` を押し出す。

## 方針

`sidebarCols() == 0` のときは `pane` のヒントを入れない。他のヒントの順は変えない。
`h/l:ペイン` と `g:取り込み` は区切りを含めて同じ 12 桁なので、ja の 80 桁で `g` が残るはず。

## タスク

1. テスト（赤）: ja・80 桁・リポジトリ 0 件のキーバーに `g:` があり、`h/l` が無い。
   既存の `TestKeyBarKeepsTheEssentialKeysInJapaneseAt80` は 80 桁（サイドバー無し）で `pane` が
   残ることを求めているので、その要求を外す
2. `footerHints` を直して緑にする
3. `make golden`。変わるのは Repos の 80 桁の全 golden、120 / 160 桁の 0 件の golden、
   Repos を描く root の 80 桁の golden のキーバー行だけであることを確かめる
4. `make check`
