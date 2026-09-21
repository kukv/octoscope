# 識別子が 2 系統であることをモデルに書く実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 違和感 F-05（「PR の同一性を表す識別子が 2 系統ある」）を、
**2 系統であることを明示的にモデルに書く**ことで閉じる。コード変更は無く、doc だけ。

**Architecture:** `ItemRef`（読み取り側）と `PullRequestHandle`（書き込み側）の doc に、
互いの関係——**どちらも他方から導出されない**——を書く。F-10（`SavedQuery` の doc）と同じ形。

**Tech Stack:** Go、golangci-lint、gotestsum

**Spec:** 違和感 F-05（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）。
俯瞰図は 2 案を挙げていた——(a) 識別子を 1 つの型にまとめる、
(b) 2 系統であることを明示的にモデルに書く。**(b) を採る**（利用者が承認、2026-09-21）。

---

## なぜ (b) か。(a) は何を待っているか

**(a) は F-12（識別子にサービスの次元が無い）待ちである。** 識別子を 1 つの型に畳むなら、
その型はサービス次元を持つかどうかを同時に決めることになる。F-12 は保留中なので、
今 (a) を採ると F-12 のときに識別子を二度設計する。これは F-10 で
`domain.SearchQuery` を作らなかったのと同じ判断である。

**(b) は F-12 を待たない。** 「2 系統ある」という事実は、サービスが 1 つでも
2 つでも変わらない。#125（F-15）が「F-12 待ちにならない形」を選んで先に進めたのと同じ。

## 着手前に調べた事実（2026-09-21）

### A. handle が作られるのは gateway の取得の戻り値の中だけ

`grep -rn 'PullRequestHandle' internal cmd --include='*.go' | grep -v _test.go` の
結果 20 行のうち、**`domain.PullRequestHandle(...)` という変換は 2 箇所しかない**。

| 場所 | 中身 |
|---|---|
| `gateway/gh/review.go:154` | `PullRequest: domain.PullRequestHandle(rc.ID)`（`toReviewContext`） |
| `gateway/gh/merge.go:47` | `PullRequest: domain.PullRequestHandle(c.PullRequestID)`（`toMergeContext`） |

どちらも **`PRReviewContext` / `PRMergeContext` の戻り値を組み立てている最中**である。
残り 18 行は型として通しているだけ（port の署名、構造体のフィールド、`string(pr)` への戻し）。

### B. `ItemRef` ⇄ handle の変換はどこにも無い

- **`ItemRef` から handle を作る経路は無い。** handle を得るには
  `PRReviewContext` か `PRMergeContext` を呼ぶしかない
- **handle から `ItemRef` を作る経路も無い。** handle は不透明文字列で、
  repo も number も入っていない
- だからオブジェクト図の ③（レビュー）と ④（マージ）は、同じ PR を指しながら
  **互いを参照する手段を型として持っていない**——これは事故ではなく、
  上の 2 つの取得が別々に handle を配っている結果である

### C. 既存の doc は「片側」しか言っていない

`domain/handle.go:3-6` のパッケージ側 doc:

> A handle addresses something on the service the application is talking to.
> The application never reads one: it passes back what it was given.
> The service's own shape for an id -- a node id, a database id -- stops at the gateway.

**不透明であることは書いてある。** 書いていないのは
「では `ItemRef` との関係は何か」「どこで手に入るのか」である。

`domain/item.go:38-45` の `ItemRef` の doc も、自分が何であるかは書いているが、
**書き込みが別の識別子を使うことに触れていない。**

### D. 対象外

- **`ItemRef` と handle を 1 つの型に畳むこと**（案 a）。F-12 待ち
- **`ReviewHandle` / `JobHandle` / `RunHandle`。** これらは「PR の同一性」の話ではない。
  ただし `handle.go` のパッケージ doc に手を入れるので、**4 つに等しく当てはまる書き方**にする
- **守るテストを足すこと。** 検査したいのは「変換が**存在しない**こと」で、
  Go のテストで書けるものではない。F-10 も同じ理由でテストを足していない
- **`.claude/rules/` の更新。** 規約を変える話ではない

## Global Constraints

- **コードを 1 行も変えない。** doc コメントだけ
- **golden 354 枚を 1 バイトも変えない**
- `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

---

### Task 1: `ItemRef` の doc に読み取り側であることを書く

**Files:**
- Modify: `internal/app/domain/item.go:38-45`

- [ ] 既存の doc の後ろに段落を足す。書く内容:
      - `ItemRef` は**読み取りの識別子**である
      - レビューとマージの**書き込みは `PullRequestHandle` を使う**（別の識別子）
      - **`ItemRef` から handle は作れない。** handle は
        `PRReviewContext` / `PRMergeContext` の戻り値に入ってくる
- [ ] 既存の 2 段落（`Repo` が "owner/name" であること、`Kind` が identity 側にある理由）は
      **触らない**

### Task 2: `handle.go` に 2 系統の関係を書く

**Files:**
- Modify: `internal/app/domain/handle.go:3-6`（パッケージ側の doc）と
  `:8-9`（`PullRequestHandle` の doc）

- [ ] 型ごとの doc の前にある共通 doc に、**手に入る場所**を書く——
      handle は gateway が取得の戻り値を組み立てるときに配る。
      4 つの handle すべてに当てはまる言い方にする（事実 D）
- [ ] `PullRequestHandle` の doc に、`ItemRef` との関係を書く:
      - 同じ pull request を指す識別子が 2 つあり、**一方から他方は導出されない**
      - handle は `PRReviewContext` と `PRMergeContext` が**それぞれ別に**配る。
        だからレビュー中の handle とマージ中の handle は、同じ PR のものでも
        型の上では繋がっていない
      - **なぜ畳まないのか**を一行で: GitHub が書き込みに node id を要求するからで、
        `(repo, number)` から node id を求めるには問い合わせが 1 回要る。
        それを隠すと port を読んで呼び出し回数が分からなくなる
        （`.claude/rules/architecture.md` の (ii)）
- [ ] **Verification:** `make check`

**Commit:** `docs: say that the read side and the write side name a PR differently`

---

## 完了条件

- [ ] `make check` が通る
- [ ] `git diff` に `.go` のコード行が 1 行も無い（コメントのみ）
- [ ] golden 354 枚が 1 枚も変わっていない
- [ ] `ItemRef` の doc と `PullRequestHandle` の doc が**互いを名指ししている**
- [ ] PR 本文に「(a) を採らなかった理由は F-12 待ちだから」と書く
- [ ] Artifact の F-05 を「完了」に更新し、記憶も更新する
