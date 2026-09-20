# 書き込みメソッドに `ctx` を通す実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `internal/github` の書き込みメソッド 10 本に `ctx` を通し、
`gateway/gh/writes.go` の `_ = ctx` 4 箇所を消す。

**Architecture:** 署名を通すだけ。`ctx` は今まで `context.Background()` として
**葉（`c.run` / `c.write`）で作られていた**。それを呼び出し元から渡すように変える。
振る舞いは変わらない——キャンセルが効くようになるのは、呼び出し元がキャンセル可能な
`ctx` を渡すようになったときである（今は detail ビューが `context.Background()` を渡す）。

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** `docs/superpowers/specs/2026-09-20-item-unification-design.md` §5
（「書き込み 4 本は `ctx` を受け取るだけで使わない……**期限付きの逸脱である**」）

---

## なぜ単独の PR か

Item 統合（#115〜#121）とは独立している。触るのは `internal/github` の 2 クライアントで、
Item の型にも port の形にも関係しない。#117（`wrap` の整形）を単独 PR にしたのと同じ判断。

## Global Constraints

- **`context.Background()` を `internal/github` の中で新しく作らない。** 消すのが目的である
- **`gateway/gh` の port の形を変えない。** 新ポート 6 本の署名は PR 2 で決めたまま
- **レビュー・マージの書き込み（`MergePR` `SubmitReview` など）は触らない。**
  それらの gateway メソッドは `ctx` を取っておらず `_ = ctx` も無い。別の話である
- **golden 354 枚と `testdata/` 全体を 1 バイトも変えない**
- 各タスクの最後に `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 着手前に調べた事実（2026-09-20）

### A. 通す先は 10 本 + ヘルパー 5 本

| クライアント | 公開メソッド | 内部ヘルパー |
|---|---|---|
| `cli` (`cli.go`) | `AddPRComment` `AddIssueComment` `ClosePR` `ReopenPR` `CloseIssue` `ReopenIssue` `EditPRLabels` `EditIssueLabels` `EditPRAssignees` `EditIssueAssignees` | `editItems` |
| `api` (`items.go`) | 同じ 10 本 | `comment` `setState` `editLabels` `editAssignees` |

### B. `context.Background()` は葉で作られている

- `cli`: 各メソッドが `c.run(context.Background(), c.dir, ...)` を呼ぶ
- `api`: 各ヘルパーが `c.write(context.Background(), ...)` を呼ぶ

**`c.run` と `c.write` は既に `ctx` を第 1 引数に取る。** 通し先は用意されている。
`api/items.go` の `editLabels` は remove 1 件につき 1 リクエストを送るループを持つので、
**ループの中の `write` にも同じ `ctx` を渡す**（途中でキャンセルされたら残りは送られない）。

### C. `backend` インターフェースの 5 つのグループ

`gateway/gh/backend.go` の `commenter` / `stateChanger` / `labelEditor` / `assigneeEditor` が
この 10 本を宣言している。**4 つの interface すべてに `ctx` を足す。**

### D. 触るテスト

`internal/github/cli/cli_test.go`、`internal/github/api/items_test.go`、
`gateway/gh/writes_test.go`、`gateway/gh/items_test.go` に呼び出しが 35 箇所。
**フェイクの関数フィールドの型も変わる**（`run` / `write` を差し替えている箇所）。

### E. 呼び出し元は今のところ `context.Background()` を渡す

`detail/commands.go` の書き込み 4 箇所（PR 3 で入れた形）。
**この PR ではそこを変えない**——`tea.Cmd` はキャンセル可能な文脈を持たないので、
今取れる形はこれである。**この PR が作るのは「通り道」であって、キャンセルそのものではない。**
通り道が無ければ、後でキャンセルを入れたいときに 2 つの層を同時に触ることになる。

---

### Task 1: `internal/github` の 2 クライアントに `ctx` を通す

**Files:**
- `internal/github/cli/cli.go`（変更）
- `internal/github/api/items.go`（変更）
- `internal/github/cli/cli_test.go`（変更）
- `internal/github/api/items_test.go`（変更）

- [ ] **Step 1: `cli` の 10 本 + `editItems`**

各メソッドの第 1 引数に `ctx context.Context` を足し、
`c.run(context.Background(), ...)` を `c.run(ctx, ...)` にする。
`editItems` も同じ（呼ぶ側 4 本から `ctx` を渡す）。

- [ ] **Step 2: `api` の 10 本 + ヘルパー 4 つ**

同じ。`comment` / `setState` / `editLabels` / `editAssignees` が `ctx` を取り、
`c.write(context.Background(), ...)` を `c.write(ctx, ...)` にする。
**`editLabels` の remove ループの中も忘れない**（事実 B）。

- [ ] **Step 3: この 2 パッケージのテストを直す**

呼び出しに `context.Background()`（テストなら `t.Context()` が使える箇所は `t.Context()`）を
足す。**既存のテストの検証内容は変えない。**

- [ ] **Step 4: 検査**

```bash
go build ./internal/github/...
go test ./internal/github/...
```

`gateway` はまだ通らない（次のタスク）。

---

### Task 2: `gateway/gh` の `backend` と `writes.go`

**Files:**
- `internal/app/adapter/gateway/gh/backend.go`（変更）
- `internal/app/adapter/gateway/gh/writes.go`（変更）
- `internal/app/adapter/gateway/gh/writes_test.go`（変更）
- `internal/app/adapter/gateway/gh/items_test.go`（変更）
- `internal/app/adapter/gateway/gh/errors_test.go`（変更、必要なら）

- [ ] **Step 1: `backend.go` の 4 つの interface**

`commenter` / `stateChanger` / `labelEditor` / `assigneeEditor` の 10 メソッドに
`ctx context.Context` を足す。

- [ ] **Step 2: `writes.go` から `_ = ctx` を消す**

4 箇所の `_ = ctx` を削除し、`g.backend.X(ctx, ...)` に変える。

**ファイル冒頭の doc コメントから、次の段落を消す:**

```go
// ctx is taken and not used: the client's write methods do not accept one
// yet. The port takes it so that threading it through later changes this
// file and not every caller.
```

期限付きの逸脱が終わったので、その説明も終わる。**他の段落は残す。**

- [ ] **Step 3: テストのフェイクを直す**

`fakeBackend` の関数フィールドの型と、呼び出し。

- [ ] **Step 4: 検査**

```bash
make fmt
make check
git status --porcelain | grep -c testdata    # 0 であること
```

- [ ] **Step 5: `_ = ctx` が消えたことを確認する**

```bash
grep -rn "_ = ctx" internal/    # 0 件
grep -rn "context.Background()" internal/github/    # 0 件であること
```

2 つ目が 0 でなければ、通し残しがある。**ただしレビュー・マージ側
（`review.go` `merge.go` など）にもし残っているなら、それはこの PR の範囲外**なので、
残っている場所を確かめて報告する。

- [ ] **Step 6: コミット**

```bash
git add internal
git commit -m "refactor: thread ctx into the write methods" \
  -m "The clients' write methods made their own context.Background() at the
leaf. They take one now, which is what lets the _ = ctx in the gateway go:
it was a deviation from go-style.md with an expiry on it, and this is the
expiry." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 設計書の申し送りを閉じる

**Files:**
- `docs/superpowers/specs/2026-09-20-item-unification-design.md`（変更）

- [ ] **Step 1: §5 の末尾を書き換える**

「書き込み 4 本は `ctx` を受け取るだけで使わない……**独立した PR で片付ける**」を、
**片付いたことを書く形**に直す。逸脱があったこと自体は消さない——
なぜ一時的に許したかが読めなくなる。

- [ ] **Step 2: 全体検査**

```bash
make check
make release-check
```

- [ ] **Step 3: 実機確認を利用者に依頼する**

書き込み経路が変わるので、**実際にコメント投稿・クローズ・ラベル編集を叩いてもらう。**
ここは golden では担保できない（golden はフェイクの上で描画だけを見ている）。

- [ ] **Step 4: コミット**

```bash
git add docs/superpowers/specs/2026-09-20-item-unification-design.md
git commit -m "docs: close the ctx hand-off the Item unification left open" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## この PR の完了条件

- `internal/` に `_ = ctx` が 0 件
- `internal/github` の**書き込み経路**に `context.Background()` が 0 件
- `backend` の書き込み 10 本すべてが `ctx` を第 1 引数に取る
- `api` の `editLabels` の remove ループが同じ `ctx` を使っている
- レビュー・マージの書き込みは無変更
- golden 354 枚と `testdata/` 全体が無変更
- `make check` と `make release-check` が通る
- **利用者が実機でコメント投稿・クローズ・ラベル編集を確認した**
- 設計書 §5 の申し送りが閉じている
