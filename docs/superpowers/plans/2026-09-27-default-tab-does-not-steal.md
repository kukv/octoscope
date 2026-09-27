# default_tab: repos が解決前に移ったタブを奪わない（#139）

## 問題

`wantRepos` は `repoResolved` の中でしか消えない。カレントリポジトリの解決（最長 20 秒）が
返る前に `1` / `3` やタブ行クリックで別タブへ移っても、返ってきた時点で Repos に戻される。
既存テスト `TestDefaultReposDoesNotPullTheUserBackAfterTheyMove` は解決の**後**に
移る場合しか見ていない。

## 方針

利用者がタブを選んだ時点で `wantRepos` を捨てる。タブを変える経路は 2 つ
（`handleKey` の `1` / `2` / `3`、`mouse.go` のタブ行クリック）なので、
どちらも `m.tab` に直接代入する代わりに `selectTab(t)`（`m.tab = t` と
`m.wantRepos = false`）を通す。

`2` で Repos を自分で選んだ場合も捨ててよい——既に Repos にいる。

範囲外: タブを変えずに Work の中でカーソルを動かす・詳細を開く、は「移った」に数えない
（Issue の受け入れ条件はタブの移動だけ）。

## タスク

1. `root_test.go` に「解決前に `1` で Work に移る → 解決 → Work のまま」
   「解決前にタブ行の Work をクリック → 解決 → Work のまま」を書いて赤を確認
   （default_tab: repos の起動直後は Work にいるので、`3` → 解決 → Search のまま、の形にする）
2. `selectTab` を足して 4 箇所を置き換え、緑に
3. `make check`。golden は不変
