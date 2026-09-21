# レビュー・マージの書き込みに `ctx` を通す実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** #122 が範囲外に置いたレビュー・マージの書き込み 8 本に `ctx` を通し、
`internal/github/gql` の `context.Background()` 8 箇所を消す。
これで F-07（「ポートの `ctx` の扱いが読み書きで割れている」）が完全に閉じる。

**Architecture:** 署名を通すだけ。`ctx` は今 `gql/review.go` と `gql/merge.go` の
**葉で `context.Background()` として作られている**。それを呼び出し元から渡すように変える。
**キャンセルが効くようになるわけではない**——呼び出し元（`tui/merge`・`tui/diff`・`tui/review`）は
`context.Background()` を渡すので、振る舞いは 1 ミリも変わらない。
変わるのは「渡せる形になる」ことだけである。

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** 違和感 F-07（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）の残り。
先行 PR は #122（計画 `docs/superpowers/plans/2026-09-20-ctx-into-the-write-methods.md`）。

---

## これは機械的な変更ではない。規約の判断を含む

**着手前に見つけたこと:** この 8 本が `ctx` を取らないのは書き忘れではなく、
**明示的に書かれた判断**である。

`internal/github/gql/review.go:200`:

> The five mutations take no context. They are changes, not fetches: a comment that has
> been sent has been sent, so there is nothing to abandon half-way. **The existing
> AddPRComment and ClosePR take none for the same reason** (.claude/rules/go-style.md).

`internal/github/gql/merge.go:110`:

> The three mutations take no context, for the same reason review.go's do:
> a merge that has happened has happened.

`.claude/rules/go-style.md:85,91` も同じ側に立っている:

- 「**取得系の**関数は第一引数に `ctx context.Context` を取る」（書き込みとは言っていない）
- 「同期的な 1 回の呼び出ししかない箇所に、将来のために context だけ通しておくことはしない。
  並行に走らせる、あるいは途中でやめる必要が出た時点で通す」

### この理由づけは既に成り立っていない

- **前例が消えた。** `AddPRComment` / `ClosePR` は #122 で `ctx` を取るようになった。
  コメントが根拠に挙げている「既存の 2 本も取らない」は**もう事実ではない**。
  どちらの案を採るにせよ、このコメントは直さなければならない
- **#122 はこの言い分に反論していない。** 計画は「レビュー・マージの書き込みは触らない。
  それらの gateway メソッドは `ctx` を取っておらず `_ = ctx` も無い。**別の話である**」と
  書いて範囲外に置いただけで、「変更に ctx は要らない」に賛成も反対もしていない。
  #122 の動機は gateway の `_ = ctx`（期限付き逸脱）を消すことであって、政策の決定ではなかった

### 実利は今のところ無い

書き込みの呼び出し元はすべて `context.Background()` を直に渡している。
`WithCancel` を使っているのは `tui/work/work.go:95` だけ、`WithTimeout` は
`root.go:106,144` の 2 箇所だけで、**いずれも取得系**である。
だから今回通しても、キャンセルできるようになる書き込みは 1 本も無い。

### それでも通す（利用者が案 A を承認、2026-09-21）

理由は 2 つ。

1. **F-07 が指摘したのは「割れていること」そのもの。** 10 本が取り 8 本が取らない状態は、
   port を読む人に理由を伝えない。`go-style.md` の「将来のために通さない」は
   **まだ誰も通していない場合**の話であって、半分だけ通した状態を正当化しない
2. **葉の `context.Background()` を消すのが目的。** #122 と同じ形——
   `internal/github` の中で context を作らない、という一本の線に揃う

**代償として `go-style.md` の context 節を改める**（`.claude/rules/architecture.md`
「規約そのものを変える」の手順。変更は Task 6 で、同じ PR の中で行う）。

---

## 着手前に調べた事実（2026-09-21）

### A. 通す先は 4 層 ×（3〜8 本）

| 層 | ファイル | メソッド |
|---|---|---|
| `internal/github/gql` | `review.go:208,249,261,274,284` | `StartReview` `AddReviewThread` `SubmitReview` `SubmitNewReview` `DiscardReview` |
| 〃 | `merge.go:113,123,132` | `MergePR` `EnableAutoMerge` `DisableAutoMerge` |
| `adapter/gateway/gh` の `backend` | `backend.go:85-91`(`reviewer`), `:99-102`(`merger`) | 同じ 8 本 |
| `adapter/gateway/gh` | `review.go:40,58,72` / `merge.go:21,30,38` | `AddReviewThread` `SubmitReview` `DiscardReview` `MergePR` `EnableAutoMerge` `DisableAutoMerge` |
| `usecase` | `review.go:16-18,29,36,45` / `merge.go:11-13,20,24,28` | port 6 本 + 公開 6 本（`PostLineComment` `SubmitReview` `DiscardReview` `MergePR` `EnableAutoMerge` `DisableAutoMerge`） |
| `presentation` | `diff/diff.go:24,25` / `review/review.go:19` / `merge/merge.go:29-31` | 同じ 6 本（ビュー側 `Source`） |

**`StartReview` と `SubmitNewReview` は usecase に上がっていない**（`backend.go` の
パッケージ doc が「review.go が `g.backend` 経由で呼ぶ」と書いている）。
gql と backend インターフェースだけで止まる。

### B. `context.Background()` は葉にある

`gql/review.go` の 5 箇所と `gql/merge.go` の 3 箇所が
`c.Write(context.Background(), ...)` を呼んでいる。**通し先は元からある**（#122 と同じ）。

`cli` / `api` にラッパーは無い——この 8 本は `gql.Client` のメソッドで、
両クライアントが埋め込みで持っている（`grep` 済み。`internal/github/cli` にも
`internal/github/api` にも同名メソッドの定義は無い）。**#122 より通す層が 1 つ少ない。**

### C. 呼び出し元は 6 箇所

| ファイル | 呼び出し |
|---|---|
| `tui/merge/merge.go:185,194,199,202` | `m.src.MergePR` ×2 / `DisableAutoMerge` / `EnableAutoMerge`。いずれも `m.sendCmd(_, func() error {...})` のクロージャの中 |
| `tui/diff/comment.go:90` | `src.PostLineComment(target, comment)` |
| `tui/diff/review.go:101` | `src.DiscardReview(pending)` |
| `tui/review/review.go:137` | `src.SubmitReview(target, event, body)` |

`tea.Cmd` を組み立てる場所なので、**`context.Background()` を作ってよい**のはここである
（`go-style.md:87`）。`tui/detail/commands.go:51,60` が既にその形。

**`merge.go` の 4 箇所はクロージャの中にある。** `sendCmd` の中で `ctx` を作るか、
クロージャを組み立てる側で作るかは実装時に決める——`sendCmd` の署名を変えずに
クロージャの中で `context.Background()` を作るのが最小の差分になるはずだが、
`sendCmd` が `tea.Cmd` を返すのか同期実行なのかを見てから決める。

### D. フェイクは 12 ファイル

`grep -rln` の結果（`_test.go` のみ）:
`gateway/gh/{merge,review,errors,items}_test.go` /
`usecase/{merge,review}_test.go` /
`tui/{merge,diff,root,detail}` の各テスト / `gql/{merge,review}_test.go` /
`cli/cli_test.go`。**署名変更に追従するだけ**で、アサーションは変えない。

### E. gateway に足すテストの形は #122 が決めている

`gateway/gh/writes_test.go:26` の `TestTheWritesHandTheirContextToTheBackend` が
テーブル駆動で「渡した `ctx` が backend に届く」を検査している。
**同じ形を review / merge の 6 本にも書く。** 置き場所は `review_test.go` と
`merge_test.go`（`writes_test.go` は item の書き込み用なので混ぜない）。

これが要る理由も #122 と同じ——署名を通しただけでは、途中で
`context.Background()` に差し替わっても誰も気づかない。

### F. 対象外

- **`domain` の型**。`ctx` は port の話で、ドメインの値は変わらない
- **`internal/github` の取得系**。既に `ctx` を取っている
- **`cmd/octoscope`**。この 8 本を直接呼ぶ箇所は無い（`grep` 済み）
- **キャンセルを実際に効かせること。** 呼び出し元は `context.Background()` のまま。
  「途中でやめる必要が出た時点で通す」のは別の変更である

## Global Constraints

- **`context.Background()` を `internal/github` の中で新しく作らない。** 消すのが目的である
- **golden 354 枚（`internal/app/presentation/**/testdata/*.golden`）を 1 バイトも変えない。**
  `OCTOSCOPE_UPDATE_GOLDEN` を使わない
- **振る舞いを変えない。** 署名だけの変更で、渡される `ctx` は今日と同じ `Background()`
- **`_ = ctx` を書かない。** #122 が消したものを復活させない。各層で受けた `ctx` は
  必ず下へ渡す
- 各タスクの終わりに `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

---

### Task 1: `gql` の 8 本に `ctx` を通す

下から通す。この層だけならコンパイルが通らないので、**Task 2 とセットで 1 コミット**にする。

**Files:**
- Modify: `internal/github/gql/review.go`（5 本 + 先頭のコメント）
- Modify: `internal/github/gql/merge.go`（3 本 + 先頭のコメント）
- Modify: `internal/github/gql/review_test.go`, `internal/github/gql/merge_test.go`

- [ ] 8 本の第一引数に `ctx context.Context` を足し、`c.Write(ctx, ...)` に変える
- [ ] `review.go:200` の「The five mutations take no context...」段落を**消す**。
      代わりに書くことがあるかは Task 6 で規約を決めてから判断する——
      ここでは**消すだけ**にして、規約の言葉を先にコードへ書かない
- [ ] `merge.go:110` の「The three mutations take no context...」段落を消す
- [ ] テストの呼び出しに `context.Background()` を渡す

### Task 2: `backend` インターフェースと gateway の 6 本

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`（`reviewer` 5 本、`merger` の書き込み 3 本）
- Modify: `internal/app/adapter/gateway/gh/review.go:40,58,72`
- Modify: `internal/app/adapter/gateway/gh/merge.go:21,30,38`
- Modify: `internal/app/adapter/gateway/gh/{review,merge,errors,items}_test.go` のフェイク

- [ ] `reviewer` / `merger` の書き込みメソッドに `ctx` を足す
- [ ] gateway の 6 本に `ctx` を足し、`g.backend.*` へ渡す。
      `AddReviewThread` は内部で `StartReview` も呼ぶので**両方に同じ `ctx` を渡す**
- [ ] フェイクの署名を合わせる
- [ ] **Verification:** `make check`

**Commit:** `refactor: thread ctx into the review and merge mutations`

コミット本文に「葉の `context.Background()` 8 箇所が消えたこと」と
「#122 が範囲外にした残りであること」を書く。

---

### Task 3: gateway に「渡した ctx が backend に届く」テストを足す

**Files:**
- Modify: `internal/app/adapter/gateway/gh/review_test.go`
- Modify: `internal/app/adapter/gateway/gh/merge_test.go`

- [ ] `writes_test.go:26` と同じテーブル駆動の形で、6 本それぞれに 1 ケース書く
- [ ] `AddReviewThread` は**2 ケース**書く——`Pending` が空のとき（`StartReview` を
      先に呼ぶ経路）と、空でないとき。前者は `StartReview` にも同じ `ctx` が
      届くことを見る。**ここが一番落としやすい**
- [ ] **Verification:** `go test ./internal/app/adapter/gateway/gh/...`、続いて `make check`

**Commit:** `test: check the review and merge writes hand their context down`

---

### Task 4: usecase の port と公開メソッド

**Files:**
- Modify: `internal/app/usecase/review.go`, `internal/app/usecase/merge.go`
- Modify: `internal/app/usecase/{review,merge}_test.go`

- [ ] `reviewer` / `merger` の port 6 本と、公開メソッド 6 本に `ctx` を足す
- [ ] **Verification:** `make check`

**Commit:** `refactor: take a context on the review and merge use cases`

---

### Task 5: ビュー側の `Source` と呼び出し

**Files:**
- Modify: `internal/app/presentation/tui/diff/diff.go:24,25`, `diff/comment.go:90`, `diff/review.go:101`
- Modify: `internal/app/presentation/tui/review/review.go:19,137`
- Modify: `internal/app/presentation/tui/merge/merge.go:29-31,185,194,199,202`
- Modify: 各ビューのテストのフェイク

- [ ] `Source` インターフェースの 6 本に `ctx` を足す
- [ ] 呼び出し 6 箇所で `context.Background()` を作って渡す。
      `tui/detail/commands.go:51` の形に揃える
- [ ] `merge.go` の 4 箇所はクロージャの中（事実 C）。`sendCmd` の署名は変えず、
      クロージャの中で `context.Background()` を作る形を**まず試す**。
      それで読みづらければ `sendCmd` 側に移す
- [ ] **Verification:** `make check`。**golden が 1 枚も変わらないこと**

**Commit:** `refactor: hand a context to the review and merge writes from the views`

---

### Task 6: `go-style.md` の context 節を改める

**Files:**
- Modify: `.claude/rules/go-style.md:79-92`

規約を変えたその場で rules を更新する（`architecture.md`「規約そのものを変える」）。

- [ ] 「**取得系の**関数は第一引数に `ctx` を取る」を「ネットワークやプロセスを待つ関数は
      取得・書き込みを問わず第一引数に `ctx` を取る」に改める
- [ ] 「同期的な 1 回の呼び出ししかない箇所に、将来のために context だけ通しておくことはしない」は
      **残す**。これは「まだ誰も通していないとき」の話である。
      ただし「**層をまたぐ port は、その層の他のメソッドと揃える**」を足す——
      半分だけ通した状態を作らないことが、今回の判断の実質である
- [ ] **この変更で守れなくなるもの**を規約に併記する:
      「`ctx` を取っているからといってキャンセルが効くとは限らない。
      今日の書き込みの呼び出し元はすべて `context.Background()` を渡す。
      署名は『渡せる』ことしか言わない」
- [ ] **Verification:** `make check`

**Commit:** `docs: say that writes take a context too, and what that does not promise`

---

## 完了条件

- [ ] `make check` が通る
- [ ] golden 354 枚が 1 枚も変わっていない
- [ ] `internal/github` に `context.Background()` が残っていない
      （`grep -rn "context.Background()" internal/github --include='*.go' | grep -v _test.go` が
      `api/repo.go:54` の `WithTimeout` だけを返す）
- [ ] `_ = ctx` がどこにも無い
- [ ] gateway の 6 本すべてに「渡した `ctx` が backend に届く」テストがある
      （`AddReviewThread` は 2 ケース）
- [ ] `go-style.md` の context 節が新しい線と、それが約束しないことを書いている
- [ ] PR 本文に「キャンセルは今のところ効かない。署名が揃っただけである」と書く
- [ ] PR 本文に実機確認の状況を書く。**マージポップアップとレビュー提出を触るので、
      利用者に手元での確認を勧める**（`go run ./cmd/octoscope` と `--lang ja`）
