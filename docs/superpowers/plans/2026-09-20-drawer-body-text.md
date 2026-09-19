# ドロワーの本文を両タブで平文に揃える 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repos タブのドロワーの本文プレビューを、Work と同じ「マークダウン記法を
除去した平文」にする。

**Problem:** #111 で Repos の一覧クエリに `body`（マークダウン原文）を足した。
一方 Work の盤面クエリは `bodyText` を取っている。同じ `drawer.Render` に渡る
文字列の種類が違うので、Repos だけ `## 見出し` や `<!-- PR テンプレートのコメント -->`
がそのままプレビューの 3 行を占める。見た目を揃えるのが #111 の目的だったので、
ここが揃っていないのは積み残しである。

**Architecture:** `WorkItem.Body` は「プレビュー用の平文」という意味で
Work と Search が既に使っている。Repos もそれに合わせる。
`domain.PR.Body` / `domain.Issue.Body` はマークダウン原文であり、詳細ビューが
`markdownLines` で描いている。**この 2 つを 1 つのフィールドで兼ねない。**
`BodyText` を別に持ち、一覧が詰め、ドロワーが読む。

名前は GitHub のものを借りる（`.claude/rules/architecture.md`
「名前は借りてよい、形は借りない」）。`bodyText` は GraphQL のフィールド名である。

**Tech Stack:** Go, GraphQL（既存）

---

## ファイル構成

- 変更: `internal/github/gql/repo_prs.graphql`, `repo_issues.graphql` — `body` → `bodyText`
- 変更: `internal/github/gql/items.go` — `BodyText`
- 変更: `internal/github/gql/items_test.go` — 数える語を変える
- 変更: `internal/app/domain/domain.go` — `PR.BodyText` / `Issue.BodyText`
- 変更: `internal/app/adapter/gateway/gh/items.go` — 変換
- 変更: `internal/app/presentation/tui/repo/render.go` — `selectedItem`
- 変更: `internal/app/presentation/tui/repo/table_test.go`

golden は動かない見込み。fixture の本文にマークダウンが無いため。
**動いたら差分を読んでから受け入れる。**

---

### Task 1: 一覧クエリを bodyText に替える

**Files:**
- Modify: `internal/github/gql/repo_prs.graphql`, `repo_issues.graphql`
- Modify: `internal/github/gql/items.go`
- Test: `internal/github/gql/items_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`TestListDocumentsSelectTheBody` を書き換える。`\bbody\b` は `bodyText` に
一致しないので（`\b` は語境界）、数える語を変えるだけで落ちる。

```go
// The drawer previews the body as plain text, the way the board's does: a
// list that asked for "body" would put raw markdown -- headings, comment
// markers from a pull request template -- into the preview on one tab and
// clean prose on the other.
func TestListDocumentsSelectTheBodyAsText(t *testing.T) {
	// repoPRsQuery / repoIssuesQuery に \bbodyText\b が 1 つ、\bbody\b が 0 個
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/github/gql/ -run TestListDocumentsSelectTheBody`
Expected: FAIL — どちらも `bodyText` を 0 回選んでいる

- [ ] **Step 3: 通す最小の実装**

2 つの `.graphql` の `body` を `bodyText` に替え、コメントを直す。
`gql.PullRequest` と `gql.Issue` に `BodyText string \`json:"bodyText"\`` を足す。
**`Body` は消さない。** 単一取得のクエリが使っている。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/github/...`
Expected: PASS

- [ ] **Step 5: コミット**

メッセージ: `feat(gql): ask the list queries for the body as text`

---

### Task 2: domain と gateway に通す

**Files:**
- Modify: `internal/app/domain/domain.go`
- Modify: `internal/app/adapter/gateway/gh/items.go`
- Test: `internal/app/adapter/gateway/gh/items_test.go`

- [ ] **Step 1: 失敗するテストを書く**

```go
// TestAListedPullRequestCarriesItsBodyAsText is what the Repos drawer draws
// its preview from. The list asks for bodyText and not body, so the markdown
// field stays empty until the item is opened.
func TestAListedPullRequestCarriesItsBodyAsText(t *testing.T) {
	// 録った一覧レスポンスを通し、BodyText が詰まっていること
}
```

録画に `bodyText` が無ければ、**録り直す**（`.claude/rules/testing.md`:
テストを通すために録ったファイルを編集しない）。録り方は
`internal/github/gql/testdata/README.md` にある。

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/adapter/gateway/gh/ -run TestAListedPullRequest`
Expected: FAIL

- [ ] **Step 3: 通す最小の実装**

`domain.PR` と `domain.Issue` に `BodyText` を足す。**何が違うのかをコメントに書く**
（`Body` はマークダウン原文で詳細ビューが描く、`BodyText` は一覧が持つプレビュー用の
平文で、単一取得では空）。`toPR` / `toIssue` で写す。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/...`
Expected: PASS。domain の struct タグ検査（reflect と go/ast の 2 本）も通ること。

- [ ] **Step 5: コミット**

メッセージ: `feat(domain): carry a listed item's body as plain text`

---

### Task 3: ドロワーに平文を渡す

**Files:**
- Modify: `internal/app/presentation/tui/repo/render.go`
- Test: `internal/app/presentation/tui/repo/table_test.go`

- [ ] **Step 1: 失敗するテストを書く**

```go
// TestTheDrawerPreviewsTheBodyAsText is the last piece of "the same drawer on
// both tabs": the board feeds it plain text, so this tab must too, or a pull
// request template's comment markers would fill the preview here and not
// there.
func TestTheDrawerPreviewsTheBodyAsText(t *testing.T) {
	// BodyText を持ち Body にマークダウンを持つ PR を選び、
	// ドロワーに BodyText が出て Body の記法が出ないこと
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/repo/ -run TestTheDrawerPreviews`
Expected: FAIL — マークダウンのほうが出ている

- [ ] **Step 3: 通す最小の実装**

`selectedItem()` の `Body:` を `pr.BodyText` / `issue.BodyText` に替える。

- [ ] **Step 4: 通ることを確かめる**

Run: `make check`
Expected: tidy / lint / fmt / test すべて通る。golden が動いたら差分を読む。

- [ ] **Step 5: コミット**

メッセージ: `fix(repos): preview the body as text, as the board does`

---

### Task 4: 実際に起動して見る

**Files:** なし

- [ ] **Step 1: 見る**

Run: `go run ./cmd/octoscope --repo kukv/koto`

テンプレートのコメントや見出しを持つ PR を選び、プレビューが平文になっていること、
Work タブの同じ PR と見た目が一致することを見る。`--lang ja` でも見る。
