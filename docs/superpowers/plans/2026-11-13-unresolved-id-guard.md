# checks / merge / search / dialog / drawer に未解決 ID のガードを掛ける（#145）

## 問題

`i18n.AssertNoUnresolvedIDs` を呼んでいるのは work / detail / review / diff / repo / root だけ。
checks / merge / search は golden を録っているが、golden はカタログに無い ID を `!id` のまま記録するので、
ID を打ち間違えても `make golden` で録り直すと通ってしまう。dialog / drawer は golden を持たず、ガードも無い。

## 方針

**golden を録っている 3 パッケージは、`TestGolden` の中で、録った画面にガードを掛ける。**
golden が `!id` を記録してしまう場所でそのまま検査するので、状態を作るコードを二重に持たずに済み、
en / ja × 80 / 120 / 160 × 全状態を網羅できる。

- checks / merge: `golden.Assert(t, name, state.build(...).View())` を、`view` を変数に取ってから
  `golden.Assert` と `i18n.AssertNoUnresolvedIDs(t, view)` の 2 つに渡す形にする
- search: `TestGolden` は 6 つの状態を `golden.Assert` で個別に録っている。ループの中に
  `record := func(name, view string)`（`golden.Assert` とガードの両方を呼ぶ）を置き、6 か所をそれに置き換える

却下: diff / review のように別の `TestNoUnresolvedIDs...` を立てる。golden の状態一覧をもう一度並べることになり、
状態を足したときに片方だけ更新される。

**golden を持たない 2 パッケージは、ID を引く分岐を通る画面を en / ja で描いて検査するテストを足す。**
言語を切り替えるので `t.Parallel()` にせず、`t.Cleanup` で英語に戻す。

- dialog（`dialog_test.go`、`package dialog_test`）: `TestNoUnresolvedIDsInTheDialog`。描くのは
  検索中（`Searching()`、`dialog.searching`）、候補 0 件（`SetCandidates(nil)`、`dialog.no_candidates`）、候補あり、の 3 つ
- drawer（`drawer_test.go`、`package drawer_test`）: `TestNoUnresolvedIDsInTheDrawer`。描くのは
  check の無い PR（`work.no_checks`）、表示件数を超える check を持つ PR（`work.checks_more` と `work.checks_summary`）、
  Issue、の 3 つ。幅は 120。drawer は文言を幅で切り詰めるので、狭いと `!id` が途中で切れて検査をすり抜ける

## タスク

1. **checks / merge / search。** 上のとおり `TestGolden` にガードを足す。golden は変わらない（`make golden` を走らせても diff が出ない）
2. **dialog / drawer。** 上の 2 つのテストを足す
3. **空振りの確認。** パッケージごとに、そのパッケージが引く ID を 1 つ、コード側で一時的に打ち間違えて
   （例: `"dialog.searching"` → `"dialog.searchingx"`）、ガードが落ちることを見てから戻す。
   checks / merge / search は golden 自体も落ちるが、見るのはガードの `unresolved message ID` が出ること
4. `make check` を通し、`test(i18n): ...`（`Closes #145`）でコミットする
