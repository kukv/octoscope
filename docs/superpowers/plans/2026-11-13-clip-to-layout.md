# layout: clip の重複を layout.Clip にまとめる（#184）

## 問題

`layout.Clip`（`layout/columns.go`、`ansi.Truncate(s, w, "…")`）と同じ実装の非公開関数 `clip` が、
work・checks・diff・drawer の 4 パッケージに 1 つずつある。drawer の `clip` の doc コメントは
「layout.Clip は余白を 1 桁取る」と書いているが、余白を取るのは `layout.Pad` で、このコメントは誤っている
（出典: phase4-search-foundation-followups #4）。

## 方針

4 つの `clip` を消し、呼び出しを `layout.Clip` に置き換える。挙動は変わらないので、golden は変わらない。

- 置き換える呼び出しは、実装 47 か所（work 6・checks 14・diff 20・drawer 7）と、テスト 2 か所
  （`checks/checks_test.go:346`・`diff/diff_test.go:895`）
- 4 パッケージの実装はどれも既に `layout` を import している。テスト 2 ファイルは import を足す
- `clip` を消して `ansi` が使われなくなったファイルは、import から外す（コンパイラが教える）
- drawer の誤ったコメントは、`clip` ごと消える。別の場所に言い直しは書かない。`layout.Clip` と `layout.Pad` の
  違いは、それぞれの doc コメントに既に書いてある

## タスク

1. 4 パッケージで `clip` を消し、`layout.Clip` に置き換える。テスト 2 か所も同じ
2. `make golden` を走らせ、`git status` で golden に差分が出ないことを確かめる（挙動を変えていないことの検証）
3. `make check` を通し、`refactor(tui): ...`（`Closes #184`）でコミットする
