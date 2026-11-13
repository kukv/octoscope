# repo: setup の検査が Fatalf のために cmd() を実行しない（#162）

## 問題

`internal/app/presentation/tui/repo/repo_test.go:429`（`TestANoticeDoesNotFollowTheUserToTheOtherTab`）は、
タブの移動で fetch が始まってしまったこと（setup の崩れ）を報告するためだけに、`Fatalf` の引数で `cmd()` を実行している。
`%T` でメッセージの型を出すためだが、報告のためにコマンドを走らせるのは、テストが検査対象に副作用を起こすことになる。
今は fakeSource しか呼ばないので実害は無い（出典: fetch-failures-followups #3）。

同じファイルの 597・620・645・680 行目にも `Fatalf(..., cmd())` があるが、これらは「cmd が返したメッセージが期待した型か」
を検査していて、`cmd()` を実行すること自体が検査の中身なので、対象外。

## 方針

429 行目の `Fatalf` から `cmd()` と `%T` を外し、文言だけで報告する。

```go
t.Fatalf("setup: the move started a fetch, which would clear the notice by itself")
```

型を出さなくても、ここで知りたいのは「cmd が nil でなかった」ことだけなので、情報は失われない。

却下: `cmd()` の代わりに `fmt.Sprintf("%p", cmd)` などで cmd 自体を出す。関数値の型は常に `tea.Cmd` で、何も伝えない。

## タスク

1. 429 行目を上のとおり直す。
2. 検証: setup の検査が今も効くことを確かめるため、一時的に `if cmd != nil` を `if cmd == nil` に変えて
   `go test ./internal/app/presentation/tui/repo -run TestANoticeDoesNotFollowTheUserToTheOtherTab` が
   新しい文言で落ちることを見てから戻す。
3. `make check` が通ることを確かめてコミットする（`test(repo): ...`、`Closes #162`）。
