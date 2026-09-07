# Phase 3 スレッドのコメントのページング 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 1 本のレビュースレッドに 51 件目以降のコメントが付いていても、diff ビューが全部見せる。

**Architecture:** `review.graphql` の `comments(first: 50)` に `pageInfo` を足し、スレッド自身の `id` も選ぶ。**50 件を超えたスレッドだけ**、`node(id:)` でそのスレッドを名指しして残りを引く新しいクエリ（`thread_comments.graphql`）を追加で投げる。超えていないスレッドには 1 リクエストも足さない。ドメイン型（`gh.ReviewThread`）は変えない。

**Tech Stack:** Go 1.27.1 / `gh` CLI 2.100.0

**Spec:** `docs/superpowers/specs/2026-09-07-phase3-design.md` §5（ページング）と §6（テスト）、§8 完了条件 6

## Global Constraints

- **`make check` が通らない状態でコミットしない。** 各タスクの最後は必ず `make check`
- **テストでネットワークもサブプロセスも叩かない**（`.claude/rules/testing.md`）
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
- **`//nolint` を新しく足さない**（`.claude/rules/go-style.md`）
- **コードのコメントは英語。** 書きすぎない（外部の事情・そう書いた理由・doc の 3 つだけ）
- **上限（何ページまで）は置かない**（spec §5）。`reviewThreads` が `cfdd925` で上限なしに全ページ取る形になっており、ここだけ別の流儀にする理由が無い
- **`internal/tui` は触らない。** これはバックエンドが黙って落としていた行を拾う変更で、画面の形は変わらない

---

## Decisions（実装者はここを勝手に読み替えない。変えたくなったら止めて相談する）

### D1. `gh.ReviewThread` にスレッドの id を足さない

追加のリクエストに要る `id` を使うのは `internal/gh/cli` の中だけである。ドメイン型に
持たせると、画面が一度も読まない値がドメインに増える。`threadNode` に足して、
`toDomain()` には渡さない。

### D2. `review.graphql` の 1 ページ目は `first: 50` のままにする

100 に上げれば追加リクエストの要るスレッドは減るが、**1 ページ目は全スレッド分まとめて
返ってくる**（`reviewThreads(first: 100)` の各ノードに 50 件ずつ）ので、上げると
「コメントが多いスレッドが 1 本ある PR」ではなく**全部の PR**の応答が重くなる。
追加が要るのは 50 件を超えたスレッドだけで、それは滅多に無い。

追加のクエリのほうは `first: 100`（GitHub の上限）にする。そちらは既に「多い」と
分かっているスレッドにしか投げない。

### D3. 検証はインライン JSON で行う。録りものは足さない

`testdata/README.md` にあるとおり、ページングの仕組みは `checks_test.go` でも
インライン JSON で確かめている。**51 件のコメントが付いたスレッドを実物に作るのは
副作用が大きすぎる**（誰かの PR にコメントが 51 件並ぶ）。フィールドの読み取りは
既存の `review_context.json` が見ている。

---

## File Structure

| ファイル | 役割 |
|---|---|
| `internal/gh/cli/review.graphql`（修正） | スレッドの `id` と `comments` の `pageInfo` を選ぶ |
| `internal/gh/cli/thread_comments.graphql`（新規） | 1 本のスレッドの 51 件目以降を引く |
| `internal/gh/cli/review.go`（修正） | `threadNode` に `id` と `pageInfo`、コメント変換の切り出し、`threadComments` と `PRReviewContext` の追い足し |
| `internal/gh/cli/review_test.go`（修正） | 2 ページ目が結果に入ること、1 ページで済むスレッドに往復を足さないこと |
| `internal/gh/cli/schema_test.go`（修正） | `thread_comments.graphql` を `docs` に足す |
| `internal/gh/cli/testdata/schema.json`（録り直し） | `Node` インターフェース |
| `internal/gh/cli/testdata/README.md`（修正） | `--argjson types` に `Node` を足す |
| `docs/superpowers/2026-09-08-phase3-checks-followups.md`（修正） | Phase 3 の残りを空にする |

---

### Task 1: スレッドの id と `pageInfo` を選ぶ

**Files:**
- Modify: `internal/gh/cli/review.graphql`, `internal/gh/cli/review.go`
- Test: `internal/gh/cli/review_test.go`

**Interfaces:**
- Produces: `threadNode` に `ID string` と `Comments.PageInfo`（`hasNextPage` / `endCursor`）、`(threadCommentNode).toDomain() gh.ThreadComment`

このタスクだけでは挙動は変わらない。**51 件目を取りに行くのは Task 2。**
ここで切るのは、クエリの変更（schema のテストが見る）と実装の変更を別のコミットにするため。

- [ ] **Step 1: `review.graphql` を直す**

`reviewThreads` の `nodes` に `id` を足し、`comments` に `pageInfo` を足す。
**コメントも直す**——今そこには「Not paged: it is a nested connection, so following it would
need one more request per thread」と書いてあり、それは事実でなくなる:

```graphql
        nodes {
          id
          isResolved
          isOutdated
          path
          line
          originalLine
          diffSide
          # A nested connection: following it costs one more request per
          # thread, so only a thread that says hasNextPage gets one
          # (thread_comments.graphql).
          comments(first: 50) {
            pageInfo {
              hasNextPage
              endCursor
            }
            nodes {
              body
              createdAt
              author {
                login
              }
              pullRequestReview {
                state
              }
            }
          }
        }
```

- [ ] **Step 2: `threadNode` を合わせる**

`internal/gh/cli/review.go`:

```go
type threadNode struct {
	// ID is not put into the domain type: only thread_comments.graphql
	// uses it, and it never reaches the screen.
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	// Line is null once the code a thread was written against has moved, so
	// it has to be a pointer to tell "no line" from "line 0".
	Line         *int   `json:"line"`
	OriginalLine int    `json:"originalLine"`
	DiffSide     string `json:"diffSide"`
	Comments     struct {
		PageInfo pageInfo            `json:"pageInfo"`
		Nodes    []threadCommentNode `json:"nodes"`
	} `json:"comments"`
}
```

`pageInfo` は `internal/gh/cli/checks.go` にある既存の型をそのまま使う（同じパッケージ）。

- [ ] **Step 3: コメントの変換を切り出す**

`toDomain()` の中のコメントを組む部分を、Task 2 から呼べるように切り出す:

```go
func (c threadCommentNode) toDomain() gh.ThreadComment {
	return gh.ThreadComment{
		Author:    gh.Author{Login: c.Author.Login},
		Body:      c.Body,
		CreatedAt: c.CreatedAt,
		// PENDING is the only review state that means "written but not
		// sent"; every other one means the comment is already public.
		Pending: c.PullRequestReview.State == "PENDING",
	}
}
```

`(threadNode).toDomain()` のループを `t.Comments = append(t.Comments, c.toDomain())` に置き換える。
**ここに書かれていたコメント（PENDING の説明）は移すだけで、増やさない。**

- [ ] **Step 4: 何も壊れていないことを確かめる**

Run: `go test ./internal/gh/cli/`
Expected: PASS（既存の `TestPRReviewContext*` がそのまま通る）

- [ ] **Step 5: `make check` とコミット**

```bash
make check
git add internal/gh/cli/review.graphql internal/gh/cli/review.go
git commit -m "refactor: select what following a thread's comments will need"
```

---

### Task 2: 51 件目以降を取りに行く

**Files:**
- Create: `internal/gh/cli/thread_comments.graphql`
- Modify: `internal/gh/cli/review.go`, `internal/gh/cli/schema_test.go`, `internal/gh/cli/testdata/README.md`, `internal/gh/cli/testdata/schema.json`
- Test: `internal/gh/cli/review_test.go`

**Interfaces:**
- Produces: `(*cli.Client).threadComments(ctx context.Context, threadID, after string) ([]gh.ThreadComment, error)`（非公開）

- [ ] **Step 1: schema.json に `Node` を足して録り直す**

`node(id:)` は `Query.node` で、返るのは `Node` インターフェースである。schema の walker は
`... on PullRequestReviewThread` を辿るのに `Node` の `possibleTypes` を要る。
`internal/gh/cli/testdata/README.md` の `--argjson types` に `"Node"` を足し、
README のコマンドをそのまま実行して `schema.json` を録り直す。

- [ ] **Step 2: 失敗するテストを書く**

`internal/gh/cli/review_test.go` に足す:

```go
// GitHub caps a thread's comments at what the first page asked for; without
// a second request the rest of a long conversation vanishes with no error.
func TestPRReviewContextWalksEveryPageOfAThreadsComments(t *testing.T) {
	t.Parallel()

	threads := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"T1"},` +
		`"nodes":[{"id":"THREAD_1","path":"a.go","originalLine":1,"diffSide":"RIGHT",` +
		`"comments":{"pageInfo":{"hasNextPage":true,"endCursor":"C50"},` +
		`"nodes":[{"body":"first","author":{"login":"kukv"}}]}}]}}}}}`
	rest := `{"data":{"node":{"comments":{"pageInfo":{"hasNextPage":false,"endCursor":"C99"},` +
		`"nodes":[{"body":"fifty-first","author":{"login":"kukv"}}]}}}}`

	f := &fakeSeq{outs: []string{threads, rest}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

	rc, err := c.PRReviewContext(t.Context(), "", 55)
	if err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (the thread says it has more comments)", len(f.calls))
	}
	if !slices.Contains(f.calls[1], "threadId=THREAD_1") {
		t.Errorf("second call = %v, want it to name the thread", f.calls[1])
	}
	// Without the cursor the second request asks for the first fifty again
	// and the loop never ends.
	if !slices.Contains(f.calls[1], "after=C50") {
		t.Errorf("second call = %v, want it to carry after=C50", f.calls[1])
	}
	if len(rc.Threads) != 1 {
		t.Fatalf("Threads = %d, want 1", len(rc.Threads))
	}
	got := rc.Threads[0].Comments
	if len(got) != 2 {
		t.Fatalf("Comments = %d, want 2 (one from each page)", len(got))
	}
	if got[0].Body != "first" || got[1].Body != "fifty-first" {
		t.Errorf("Comments = %q / %q, want the second page appended after the first",
			got[0].Body, got[1].Body)
	}
}

func TestAThreadThatFitsInOnePageCostsNoExtraRequest(t *testing.T) {
	t.Parallel()

	threads := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"T1"},` +
		`"nodes":[{"id":"THREAD_1","path":"a.go","originalLine":1,"diffSide":"RIGHT",` +
		`"comments":{"pageInfo":{"hasNextPage":false,"endCursor":"C1"},` +
		`"nodes":[{"body":"only","author":{"login":"kukv"}}]}}]}}}}}`

	f := &fakeSeq{outs: []string{threads}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

	if _, err := c.PRReviewContext(t.Context(), "", 55); err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("calls = %d, want 1: a thread with nothing more must not cost a request", len(f.calls))
	}
}

func TestAThreadWithThreePagesOfCommentsIsFollowedToTheEnd(t *testing.T) {
	t.Parallel()

	threads := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"T1"},` +
		`"nodes":[{"id":"THREAD_1","path":"a.go","originalLine":1,"diffSide":"RIGHT",` +
		`"comments":{"pageInfo":{"hasNextPage":true,"endCursor":"C50"},` +
		`"nodes":[{"body":"one","author":{"login":"kukv"}}]}}]}}}}}`
	page2 := `{"data":{"node":{"comments":{"pageInfo":{"hasNextPage":true,"endCursor":"C150"},` +
		`"nodes":[{"body":"two","author":{"login":"kukv"}}]}}}}`
	page3 := `{"data":{"node":{"comments":{"pageInfo":{"hasNextPage":false,"endCursor":"C250"},` +
		`"nodes":[{"body":"three","author":{"login":"kukv"}}]}}}}`

	f := &fakeSeq{outs: []string{threads, page2, page3}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

	rc, err := c.PRReviewContext(t.Context(), "", 55)
	if err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("calls = %d, want 3: no cap is placed on the number of pages", len(f.calls))
	}
	if len(rc.Threads[0].Comments) != 3 {
		t.Errorf("Comments = %d, want 3", len(rc.Threads[0].Comments))
	}
}
```

`review_test.go` の import に `"slices"` を足す（無ければ）。

- [ ] **Step 3: テストが落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'TestPRReviewContextWalksEveryPageOfAThreads|TestAThread'`
Expected: FAIL（`unexpected call 1` ——追加のリクエストが投げられず、2 ページ目が結果に入らない）

- [ ] **Step 4: クエリを書く**

`internal/gh/cli/thread_comments.graphql`:

```graphql
# The rest of one thread's comments. reviewThreads cannot page a nested
# connection, so a thread with more than the first page asks by node id.
query ($threadId: ID!, $after: String) {
  node(id: $threadId) {
    ... on PullRequestReviewThread {
      # first is capped at 100 by GitHub. This query only runs on a thread
      # already known to be long, so it asks for the cap.
      comments(first: 100, after: $after) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          body
          createdAt
          author {
            login
          }
          pullRequestReview {
            state
          }
        }
      }
    }
  }
}
```

- [ ] **Step 5: 実装を書く**

`internal/gh/cli/review.go`:

```go
//go:embed thread_comments.graphql
var threadCommentsQuery string

type threadCommentsResponse struct {
	Data struct {
		Node struct {
			Comments struct {
				PageInfo pageInfo            `json:"pageInfo"`
				Nodes    []threadCommentNode `json:"nodes"`
			} `json:"comments"`
		} `json:"node"`
	} `json:"data"`
}

// threadComments reads what did not fit in the page PRReviewContext already
// has, starting after the cursor that page ended on.
func (c *Client) threadComments(ctx context.Context, threadID, after string) ([]gh.ThreadComment, error) {
	var rest []gh.ThreadComment
	cursor := after
	for {
		args := []string{"api", "graphql", "-f", "query=" + threadCommentsQuery,
			"-f", "threadId=" + threadID, "-f", "after=" + cursor}
		out, err := c.run(ctx, c.dir, args...)
		if err != nil {
			return nil, err
		}
		var resp threadCommentsResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, fmt.Errorf("parse thread comments: %w", err)
		}
		page := resp.Data.Node.Comments
		for _, n := range page.Nodes {
			rest = append(rest, n.toDomain())
		}
		if !page.PageInfo.HasNextPage || page.PageInfo.EndCursor == "" {
			return rest, nil
		}
		cursor = page.PageInfo.EndCursor
	}
}
```

`PRReviewContext` のスレッドを積むところを、**全ページを集めてから**変換する形に直す。
今は 1 ページ読むごとに `rc.Threads` に足しているので、そこを `nodes` に貯める形にして、
ループを抜けたあとで変換する:

```go
	var nodes []threadNode
	cursor := ""
	for {
		// ... 今のまま。pr の各フィールドを rc に写すところも今のまま ...
		nodes = append(nodes, pr.ReviewThreads.Nodes...)

		if !pr.ReviewThreads.PageInfo.HasNextPage || pr.ReviewThreads.PageInfo.EndCursor == "" {
			break
		}
		cursor = pr.ReviewThreads.PageInfo.EndCursor
	}

	for _, n := range nodes {
		t := n.toDomain()
		if n.Comments.PageInfo.HasNextPage {
			rest, err := c.threadComments(ctx, n.ID, n.Comments.PageInfo.EndCursor)
			if err != nil {
				return gh.ReviewContext{}, err
			}
			t.Comments = append(t.Comments, rest...)
		}
		rc.Threads = append(rc.Threads, t)
	}
	return rc, nil
```

**スレッドのページを全部読み終えてから追加のリクエストを投げる**のは、順序のためではなく、
`return` が 1 か所で済むからである。スレッドの並びは `nodes` の順のままで変わらない。

`PRReviewContext` の doc コメントに 1 文足す:

```go
// PRReviewContext fetches everything the diff view needs to draw and change
// a review. It walks review threads one page at a time itself, since
// `gh api --paginate` cannot follow a GraphQL cursor below the top level,
// and follows a thread's own comments only when that thread says it has
// more.
```

- [ ] **Step 6: document を schema のテストに足す**

`internal/gh/cli/schema_test.go` の `docs` map に
`"thread_comments.graphql": threadCommentsQuery,` を足す。

- [ ] **Step 7: テストが通ることを確かめる**

Run: `go test ./internal/gh/cli/`
Expected: PASS（既存の `TestPRReviewContextWalksEveryPageOfThreads` と
`TestPRReviewContext*` も通ること。既存の fixture のスレッドは `hasNextPage` を
持たないので追加のリクエストは投げられない）

- [ ] **Step 8: テストが空振りでないことを確かめる**

3 つ壊して、それぞれ落ちることを見る。見たら戻す。

1. `if n.Comments.PageInfo.HasNextPage {` を消して常に取りに行く →
   `TestAThreadThatFitsInOnePageCostsNoExtraRequest` が落ちる
2. `threadComments` の `"-f", "after=" + cursor` を落とす →
   `TestPRReviewContextWalksEveryPageOfAThreadsComments` が落ちる
3. `threadComments` のループを 1 ページで `return` させる →
   `TestAThreadWithThreePagesOfCommentsIsFollowedToTheEnd` が落ちる
4. `thread_comments.graphql` の `pullRequestReview` を `pullRequestReviews` に打ち間違える →
   `TestEveryFieldTheDocumentsSelectExistsInTheSchema` が落ちる

- [ ] **Step 9: `make check` とコミット**

```bash
make check
git add internal/gh/cli
git commit -m "fix: stop dropping a thread's fifty-first comment"
```

---

### Task 3: 実物で 1 度だけ確かめて、記録を閉じる

**Files:**
- Modify: `docs/superpowers/2026-09-08-phase3-checks-followups.md`

このタスクにコードの変更は無い。**追加のリクエストが実際に GitHub に通ることは、
インライン JSON では確かめられない**（`node(id:)` が本当にスレッドを返すか、
`... on PullRequestReviewThread` の綴りが通るかは schema のテストが見るが、
実行時の応答は見ていない）ので、1 度だけ手で叩いて確かめる。

- [ ] **Step 1: クエリを実物に投げる**

コメントの付いているスレッドの id を取る:

```bash
gh api graphql -f query='
query { repository(owner:"kukv", name:"octoscope") {
  pullRequest(number: 55) { reviewThreads(first: 5) { nodes { id comments(first: 1) { nodes { body } } } } } } }'
```

出てきた id で新しいクエリを叩く:

```bash
gh api graphql -F query=@internal/gh/cli/thread_comments.graphql \
  -f threadId=<上で出た id>
```

`after` は渡さない（nullable なので省ける。空文字を渡すと、無効なカーソルとして
断られる可能性があり、それは確かめたいこととは別の話になる）。
**`data.node.comments.nodes` が返ること**を確かめる。落ちたら止めて、エラーの文言を持って相談する。

- [ ] **Step 2: 55 番が使えなければ別の PR で行う**

コメントの付いたスレッドがある PR ならどれでもよい。使った PR 番号を Step 3 に書く。

- [ ] **Step 3: followups を閉じる**

`docs/superpowers/2026-09-08-phase3-checks-followups.md` の「## Phase 3 の残り」の節を、
**残っていない**ことを書いた形に直す。merge は前の PR で消えているはずなので、
節そのものを消して、代わりに「Phase 3 の 3 本は全部入った。残っているのは上の設計判断
（App が作った check run の見分け）だけである」と書く。

Step 1 で確かめたこと（叩いた PR とスレッド、返ってきた形）も 1 行残す。

- [ ] **Step 4: コミットして PR を出す**

```bash
make check
git add docs
git commit -m "docs: close out Phase 3"
```

---

## Self-Review

**spec の網羅**

| spec の要求 | どのタスク |
|---|---|
| §5 各スレッドの `comments` が 1 ページを超えて取れる | Task 2 |
| §5 50 件を超えたスレッドだけ追加で引く | Task 2（`if n.Comments.PageInfo.HasNextPage`）と `TestAThreadThatFitsInOnePage...` |
| §5 ページ数の上限を置かない | Task 2（`threadComments` のループ）と `TestAThreadWithThreePages...` |
| §5 `contexts` のページング | **この計画の対象外。** checks の PR で済んでいる（`internal/gh/cli/checks.go` の `PRChecks`） |
| §5 `reviewThreads` は既に全ページ取れている | 変更しない |
| §5 Work 板の `work.graphql` は `first: 100` のまま | 触らない |
| §6 ネットワークを叩かない / 2 ページ目の中身が結果に入ることを確かめる | Task 2 Step 2 |
| §6 壊して落ちることを確認してからコミット | Task 2 Step 8 |
| §8-6 | Task 2 |

**この計画が触らないもの**（spec §5 が「別の機会にする」と書いたもの）:
Work の search の `first: 50`、`labels` の `first: 100`、pending review の `comments`
（`review.graphql` は id しか選ばないので穴ではない）。
