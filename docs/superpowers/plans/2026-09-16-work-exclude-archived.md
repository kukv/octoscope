# Work タブ: アーカイブ済みリポジトリを除く 実装計画

**Goal:** Work タブの 4 列から、アーカイブ済みリポジトリの Issue / PR を除く。

**Architecture:** `internal/app/adapter/gateway/gh/work.go` の `workQueries` だけで閉じる。
GitHub の Issue 検索に `archived:false` を足し、GitHub 側で絞る。
取得後のフィルタも、リポジトリの状態を引く追加リクエストも要らない。

**Tech Stack:** Go 1.25 / GitHub GraphQL search

**Spec:** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.1
（列の検索クエリの表。タスク 2 で追従させる）

**Branch:** `feat/work-exclude-archived`

## なぜ

アーカイブ済みリポジトリは読み取り専用で、そこにある PR はマージもレビューもできず、
Issue も閉じられない。Work タブは「自分が手を動かす対象」の盤面なので、動かせない
ものが混ざると滞留量が読めなくなる。

実測（2026-09-16、`gh api graphql` で `is:open author:@me`）:

| クエリ | 件数 |
|---|---|
| `is:open author:@me` | 93 |
| `is:open author:@me archived:false` | 15 |
| `is:open author:@me archived:true` | 78 |

8 割がアーカイブ済みだった。修飾子が効いていることもこの実測で確かめている
（GitHub は知らない修飾子を自由語として扱うので、綴りを間違えても静かに通る）。

## 決めたこと

**常に除外する。設定では切り替えない。** 動かせないものを盤面に出す理由が無い。

**Search タブは変えない。** あちらはユーザーが書いたクエリをそのまま送る場所で、
`archived:true` を自分で書ける。Repos タブと `RepoCounts` も対象外。

## Global Constraints

- 各タスクの末尾で `make check` が緑
- コメントは外部の事情・正しい理由・doc の 3 つだけ（`.claude/rules/go-style.md`）

## ファイル構成

| ファイル | 変更 |
|---|---|
| `gh/work.go` | `workQueries` の 4 本に `archived:false` |
| `gh/work_test.go` | 4 本の文字列と、送られるクエリ |
| 設計書 §4.1 | 列の検索クエリの表 |

## Task 1: クエリに `archived:false` を足す

**Files:** `gh/work.go`, `gh/work_test.go`

**Steps:**

- [ ] `TestWorkQueryDefinesEachColumn` の 4 本を `... archived:false` に書き換える
- [ ] `TestListWorkSectionSendsTheColumnsQuery` の期待を
      `"is:open mentions:@me archived:false"` にする
- [ ] `go test ./internal/app/adapter/gateway/gh/` が赤いことを見る
- [ ] `workQueries` を直す。なぜ除くのか（アーカイブ済みは読み取り専用で、
      盤面から手を出せない）を 1 行のコメントにする
- [ ] `make check`

**Verify:** `go test ./internal/app/adapter/gateway/gh/` が緑。赤→緑を見ている

## Task 2: 設計書の追従

**Files:** 設計書 §4.1

**Steps:**

- [ ] §4.1 の表の 4 本を新しい文字列にする
- [ ] `make check`

**Verify:** 設計書とコードの文字列が一致する

## Task 3: 実際に起動して見る

**Files:** なし

**Steps:**

- [ ] `go run ./cmd/octoscope` を動かす
- [ ] 4 列がちゃんと埋まる（クエリを壊すと空になるだけでエラーにならない）
- [ ] アーカイブ済みリポジトリのカードが消えている

**Verify:** 盤面が出て、アーカイブ済みのカードが無い
