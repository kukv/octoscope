# Phase 2 立て直し Part 1 実装計画（作業順 2 / 5 / 3+4）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** GraphQL の無音の打ち切りを潰し、`internal/gh/cli` のパーステストを実物の testdata に載せ替え、`internal/usecase` を導入して API 呼び出し順序と種別振り分けを TUI から追い出す。

**Architecture:** `internal/gh/cli`（ACL）は構造を変えない。その上に `internal/usecase` を新設し、`internal/tui` は `cli` を直接見ない。各ビューの `Source` interface は今までどおり利用側で宣言し、`*usecase.Usecase` がそれを満たす。合成ルートは `cmd/octoscope`。

**Tech Stack:** Go 1.27.1 / Bubble Tea v2 (`charm.land/*/v2`) / `gh` CLI（GraphQL・REST）/ golangci-lint (depguard)

**Spec:** `docs/superpowers/specs/2026-09-06-phase2-remediation-design.md`

**前提:** 設計書の作業順 0 / 1 / 1.5 は PR #56 で完了済み（`b670048`）。この計画は作業順 **2 / 5 / 3 / 4** を扱う。**6〜9 は Part 2**（`detail.go` / `diff.go` を作業順 4 が作り替えたあとでないと、正確なコードを書けないため）。

---

## Global Constraints

設計書と `.claude/rules/` から、この計画の全タスクに等しくかかる制約。

- **`make check` が通らない状態でコミットしない。** 各タスクの最後は必ず `make check`
- **`internal/gh` / `internal/gh/cli` の構造は変えない**（設計書 §7）。ACL として成立している（§2.1）。この計画で `cli` に足すのは、ページングのループと録り直した testdata だけ
- **DI コンテナを作らない。Input/Output DTO を作らない。`app.Model` をリネームしない**（設計書 §7）
- **Phase 3 以降の機能（checks / merge / Search タブ）を入れない**（設計書 §7）
- **golden は変わらない。** 表示を変える変更はこの計画に無い。検証は `git diff --stat -- '*.golden'` が空であること
- **テストでネットワークも外部プロセスも叩かない**（`.claude/rules/testing.md`）。`gh` を実際に走らせるのは testdata の**録り直し手順**だけで、それはテストではない
- **`//nolint` を新しく足さない**（`.claude/rules/go-style.md`）
- **コードのコメントは英語**。画面に出す文字列は `internal/i18n` のカタログから引き、`en` と `ja` の両方に足す
- **GraphQL の connection は `first` が最大 100。** 101 は `EXCESSIVE_PAGINATION` で拒否される（2026-09-07 に `reviewThreads` で実測）
- **`gh api --paginate` は使わない。** GraphQL では単一の `pageInfo` しか追えず、ネストした 2 つの `pageInfo` を扱えない。ページごとに別 JSON を吐くのでパースも面倒になる。Go 側で `-f after=<cursor>` のループを書く（フェイク経由でテストできる）
- **`.graphql` に `pageInfo` を選ぶと `schema_test.go` が落ちる。** `internal/gh/cli/testdata/README.md` の `--argjson types` に `PageInfo` を足して `schema.json` を録り直す

---

## Decisions（この計画で確定させたこと）

設計書が「実装時に決める」としていた点と、設計書の記述をそのまま実装できなかった点。**実装者はここを勝手に読み替えない。** 変えたくなったら止めて相談する。

### D1. interface のメソッド数の数え方（設計書 §5 項目 3、§8 条件 3）

設計書は「`detail.Source` が 6 メソッド以下」を完了条件にしているが、`detail.Source` は
`prSource` / `issueSource` / `candidateSource` / `webOpener` / `review.Source` を embed した
**合成 interface** であり、`detail.New` と `app.Source` が要求するのはこの合成である。
作業順 4 のあとも、合成の合計は 6 + 2 + 1 + 1 + 1 = 11 になる。

**確定:** 上限は「**1 つの interface 宣言に直接並べるメソッドが 6 個まで**」とする。
embed した interface のメソッドは合計に数えない（embed 先が同じ上限を独立に負う）。
`architecture.md` に足す文言もこの数え方で書く（Task 8）。

設計書 §8 条件 3 は「`detail` が宣言する最大の interface が 6 メソッド以下」と読み替える。
**設計書からの逸脱であり、2026-09-07 に利用者の承認を得た。**
Task 8 Step 10 で設計書 §8 の条件 3 も同じ文言に直す。

（選ばなかった案: 6 メソッドの interface を `Source` と名乗らせ、合成のほうを
`Deps` などに改名する。設計書を触らずに済むが、`New` が取る型の名前が
`repo` / `diff` / `work` と揃わなくなる。）

### D2. `usecase` のミューテーションは `ctx` を取らない

設計書 §4.2(b) は `PostLineComment(ctx, ...)` / `SubmitReview(ctx, ...)` と書いているが、
`cli.StartReview` / `AddReviewThread` / `SubmitReview` / `SubmitNewReview` / `DiscardReview` は
**意図的に `ctx` を取らない**（`internal/gh/cli/review.go:164-167` のコメント。送ってしまった
コメントに中断は無い）。設計書 §7 は `cli` の構造を変えないと定めている。

**確定:** `usecase` も同じ線を引く。**取得系は `ctx` を取り、変更系は取らない。**
`.claude/rules/go-style.md` の「将来のために context だけ通しておくことはしない」に従う。

### D3. `usecase.ReviewTarget`

設計書 §4.2(b) が名前だけ挙げて中身を書いていない型。`usecase` は `internal/tui/review` を
import できない（依存の向き）ので、`review.Target` は流用できない。

```go
// ReviewTarget names the pull request a review submission acts on, and the
// unsubmitted review already on it if there is one.
type ReviewTarget struct {
    PullRequestID string
    PendingID     string
}
```

`review.Target` は `PendingComments int` を持ち続ける（画面に件数を出すためのもので、
送信先の決定には使わない）。

### D4. 作業順 3（rules）は**完了済み**。実装より先に入れた

**2026-09-07 に利用者の指示で覆した判断。** 当初は「`architecture.md` に
『interface は 6 メソッドまで』と先に書くと、`detail` がそれを破っている状態が
作業順 4 の完了まで残る」という理由で、コードを直してから規約を書く順にしていた。

利用者の判断は逆で、**規約が spec に沿っていない状態で実装を始めるほうが危険**
というもの。規約を読み込むセッションが誤った方向に進みかねない。

そこで作業順 3 を先に済ませ、**規約が現状のコードを追い越している箇所には
「今これを破っている / どのタスクで直す / 新しいコードでこれを言い訳にしない」を
明記した**。該当は 3 箇所（interface 6 メソッド / UI の mode は enum /
コードに spec 参照を書かない）。

**この計画の Task 8 は実施済み。** 残りは Task 1〜7 と Task 9。
Task 9 の完了条件チェックから「条件 8（rules と depguard）」は落としてよい。

### D5. 作業順 2 — connection ごとの結論

設計書 §4.6 の「実装前に確認する」は 2026-09-07 に実測・確認済み。その結果、
`first:` 8 箇所それぞれをどうするかをここで確定させる。

| 箇所 | 結論 | 理由 |
|---|---|---|
| `review.graphql` `reviewThreads(first:100)` | **ページングする**（Go 側ループ） | 100 件超のスレッドを持つ PR は現実にある。設計書 §3.2 が名指ししている唯一の connection |
| `review.graphql` `comments(first:50)`（スレッド内） | **50 のまま。理由をコメントで残す** | ネストした connection で、ページングにはスレッドごとに追加クエリが要る。1 スレッド 50 コメントは実運用で起きない |
| `review.graphql` `reviews(states:[PENDING], first:1)` + `Nodes[0]` | **変更しない。潜在バグではない** | pending review は 1 ユーザー 1 PR につき 1 つだけ（GraphQL / REST 共通の制約）。設計書 §3.2 の指摘は誤り。**その旨を設計書 §3.2 に追記する**（Task 8） |
| `work.graphql` `labels(first:10)` ×2 | **100 に上げる。ページングしない** | 1 item のラベル数の公式な上限は未記載。`first` の最大が 100 で、それを超える item は現実に無い |
| `work.graphql` `search(type: ISSUE, first: 50)` ×4 | **50 のまま。理由をコメントで残す** | Work ボードは「今見るべきもの」の板であって全件一覧ではない。`search` は合計 1000 件までしか返せず、50 を超える列は板として既に読めない |
| `work.graphql` `contexts(first: 100)`（checks） | **変更しない** | 既に最大値 |
| REST `repos/{owner}/{repo}/assignees?per_page=100` | **変更しない。理由をコメントで残す** | assignable users が 100 を超えるリポジトリはピッカーとして既に使えない。ページングしても選べない |

### D6. 設計書 §3.2 の残りの潜在バグ

設計書 §3.2 は 5 件挙げているが、作業順に枠が無い。**Task 3 でまとめて片づける。**

| 場所 | 対応 |
|---|---|
| `internal/gh/gh.go:207` `type Work [4][]WorkItem` | 配列長を `WorkSections()` と結びつける |
| `cli/diff.go:145,163` `64*1024` / `8*1024*1024` | 名前つき定数にして 1 箇所にする |
| `cli/diff.go:270` `fields[:min(3, len(fields))]` | 名前つき定数にする |
| `cli/review.go:141` `pr.Reviews.Nodes[0]` | D5 のとおり変更しない |

---

## ファイル構成

**新規**

| ファイル | 責務 |
|---|---|
| `internal/usecase/usecase.go` | `Usecase` 型、`New`、素通しの委譲、`Item` 型と `GetItem` |
| `internal/usecase/review.go` | `PostLineComment` / `SubmitReview` のオーケストレーションと `ReviewTarget` |
| `internal/usecase/usecase_test.go` | フェイク `source` に対する `GetItem` の振り分けテスト |
| `internal/usecase/review_test.go` | pending の有無による分岐のテスト |
| `internal/gh/cli/testdata/pr_list.json` 他 | 実物を録ったレスポンス |

**変更**

| ファイル | 変更 |
|---|---|
| `internal/gh/cli/review.graphql` | `reviewThreads` に `pageInfo`、`$after` 変数 |
| `internal/gh/cli/review.go` | `PRReviewContext` にページングのループ |
| `internal/gh/cli/work.graphql` | `labels(first:10)` → `100`、据え置きの理由コメント |
| `internal/gh/cli/diff.go` | 裸の定数の命名 |
| `internal/gh/gh.go` | `Work` の配列長 |
| `internal/gh/cli/cli_test.go` / `review_test.go` / `graphql_test.go` / `diff_test.go` | 実物の testdata を読む形へ |
| `internal/gh/cli/testdata/README.md` | `PageInfo` 追加、新しい testdata の録り方 |
| `internal/tui/detail/detail.go` | `Source` を usecase 版へ。`fetch` / `postComment` / `setState` / `applyPicker` の種別振り分けを削除 |
| `internal/tui/diff/diff.go` / `comment.go` | `Source` を usecase 版へ。`post()` のオーケストレーションを usecase に移す |
| `internal/tui/review/review.go` | `Source` を 1 メソッドへ。`submit()` の分岐を usecase に移す |
| `internal/tui/repo/repo.go` / `work/work.go` / `app/app.go` | 型が変わる範囲だけ追随 |
| `cmd/octoscope/main.go` | `cli.New → usecase.New → app.New` |
| `.golangci.yml` | depguard に `usecase` の出入り |
| `.claude/rules/architecture.md` / `tui.md` / `go-style.md` / `testing.md` | 規約 10 項目 |
| `docs/superpowers/specs/2026-09-06-phase2-remediation-design.md` | D1 / D5 の訂正 |

---

## Task 1: `reviewThreads` をページングする

**Files:**
- Modify: `internal/gh/cli/review.graphql`
- Modify: `internal/gh/cli/review.go:100-135`（`PRReviewContext`）
- Modify: `internal/gh/cli/testdata/README.md`
- Modify: `internal/gh/cli/testdata/schema.json`（`gh` で録り直す）
- Test: `internal/gh/cli/review_test.go`

**Interfaces:**
- Consumes: 既存の `runFunc`、`reviewContextResponse`
- Produces: `PRReviewContext(ctx, repo, number) (gh.ReviewContext, error)` — シグネチャは変わらない。呼び出し側から見た変化は「101 本目以降のスレッドも入る」だけ

- [ ] **Step 1: 既存のフェイクを複数レスポンス対応にする**

`fakeRun` は `out` を 1 つしか持たず、何度呼ばれても同じ JSON を返す。ページングは
「2 回呼んで別々の答えが返る」ことで初めて検証できるので、順に返すフェイクを足す。
既存の `fakeRun` は他のテストが使っているので**消さず、隣に足す**。

`internal/gh/cli/review_test.go` の import に `"fmt"` `"slices"` `"strings"` を足し、
末尾に:

```go
// fakeSeq answers each call with the next canned output, so a test can watch
// a paging loop walk more than one page. It records every argument list.
type fakeSeq struct {
	outs  []string
	calls [][]string
}

func (f *fakeSeq) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	i := len(f.calls) - 1
	if i >= len(f.outs) {
		return nil, fmt.Errorf("unexpected call %d: %v", i, args)
	}
	return []byte(f.outs[i]), nil
}
```

- [ ] **Step 2: 落ちるテストを書く**

同じファイルに:

```go
// GraphQL connections cap first at 100 (101 is refused with
// EXCESSIVE_PAGINATION), so a pull request with more review threads than
// that needs a second request. Without one the extra threads vanish with no
// error, which is the failure mode this whole remediation is about.
func TestPRReviewContextWalksEveryPageOfThreads(t *testing.T) {
	page1 := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":true,"endCursor":"CUR1"},` +
		`"nodes":[{"path":"a.go","originalLine":1,"diffSide":"RIGHT"}]}}}}}`
	page2 := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"CUR2"},` +
		`"nodes":[{"path":"b.go","originalLine":2,"diffSide":"RIGHT"}]}}}}}`

	f := &fakeSeq{outs: []string{page1, page2}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

	rc, err := c.PRReviewContext(t.Context(), "", 55)
	if err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}
	if len(rc.Threads) != 2 {
		t.Fatalf("threads = %d, want 2 (both pages)", len(rc.Threads))
	}
	if rc.Threads[0].Path != "a.go" || rc.Threads[1].Path != "b.go" {
		t.Errorf("threads = %+v, want a.go then b.go in order", rc.Threads)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(f.calls))
	}
	// The second request has to carry the first page's cursor, or it just
	// asks for page one again and the loop never ends.
	if !slices.Contains(f.calls[1], "after=CUR1") {
		t.Errorf("second call = %v, want it to carry after=CUR1", f.calls[1])
	}
	// The first request must not: a null cursor is what "from the start"
	// means to GraphQL.
	if slices.ContainsFunc(f.calls[0], func(s string) bool {
		return strings.HasPrefix(s, "after=")
	}) {
		t.Errorf("first call = %v, want no cursor", f.calls[0])
	}
}
```

- [ ] **Step 3: テストが落ちることを確認**

```bash
go test ./internal/gh/cli/ -run TestPRReviewContextWalksEveryPageOfThreads -v
```

Expected: FAIL — `threads = 1, want 2 (both pages)`（ループが無いので 1 回しか呼ばない）

- [ ] **Step 4: クエリに `pageInfo` と `$after` を足す**

`internal/gh/cli/review.graphql` の `reviewThreads` 行を置き換える。変数宣言も変える。

```graphql
query ($owner: String!, $name: String!, $number: Int!, $after: String) {
```

```graphql
      # first is capped at 100 by GitHub (101 is refused with
      # EXCESSIVE_PAGINATION), so a pull request with more threads than that
      # needs pageInfo and a second request. See PRReviewContext.
      reviewThreads(first: 100, after: $after) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
```

`comments(first: 50)` の上に据え置きの理由を足す。

```graphql
          # Not paged: this is a nested connection, so following it would
          # need one more request per thread. A review thread with more than
          # 50 comments does not happen in practice.
          comments(first: 50) {
```

- [ ] **Step 5: レスポンス構造体に `pageInfo` を足す**

`internal/gh/cli/review.go` の `reviewContextResponse` の `ReviewThreads` を:

```go
				ReviewThreads struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []threadNode `json:"nodes"`
				} `json:"reviewThreads"`
```

- [ ] **Step 6: `PRReviewContext` をループにする**

`internal/gh/cli/review.go` の `PRReviewContext` を丸ごと置き換える。

```go
// PRReviewContext fetches everything the diff view needs to draw and change
// a review. It takes one request per page of review threads: GitHub caps a
// connection at 100, and `gh api --paginate` cannot follow a GraphQL cursor
// that sits below the top level, so the walk is here.
func (c *Client) PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error) {
	repoFields, err := repoArgs(c.effectiveRepo(repo))
	if err != nil {
		return gh.ReviewContext{}, err
	}

	var rc gh.ReviewContext
	cursor := ""
	for {
		args := append([]string{"api", "graphql", "-f", "query=" + reviewContextQuery}, repoFields...)
		args = append(args, "-F", "number="+strconv.Itoa(number))
		if cursor != "" {
			args = append(args, "-f", "after="+cursor)
		}
		out, err := c.run(ctx, c.dir, args...)
		if err != nil {
			return gh.ReviewContext{}, err
		}
		var resp reviewContextResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return gh.ReviewContext{}, fmt.Errorf("parse review context: %w", err)
		}
		pr := resp.Data.Repository.PullRequest

		// The pull request's own fields repeat on every page; taking them
		// from the first is enough, and taking them again is harmless.
		rc.PullRequestID = pr.ID
		rc.Title = pr.Title
		rc.Head = pr.HeadRefName
		rc.Base = pr.BaseRefName
		rc.Additions = pr.Additions
		rc.Deletions = pr.Deletions
		if len(pr.Reviews.Nodes) > 0 {
			rc.PendingID = pr.Reviews.Nodes[0].ID
		}
		for _, n := range pr.ReviewThreads.Nodes {
			rc.Threads = append(rc.Threads, n.toDomain())
		}

		if !pr.ReviewThreads.PageInfo.HasNextPage || pr.ReviewThreads.PageInfo.EndCursor == "" {
			return rc, nil
		}
		cursor = pr.ReviewThreads.PageInfo.EndCursor
	}
}
```

`reviews(states:[PENDING], first:1)` の隣のコメントに、これが打ち切りでない理由を足す。

```graphql
      # first: 1 is not a truncation: GitHub allows one unsubmitted review per
      # user per pull request, so there is never a second one to miss.
      reviews(states: [PENDING], first: 1) {
```

- [ ] **Step 7: テストが通ることを確認**

```bash
go test ./internal/gh/cli/ -run TestPRReviewContext -v
```

Expected: PASS（新テストと既存の `PRReviewContext` テストの両方）

- [ ] **Step 8: `schema.json` に `PageInfo` を足して録り直す**

`internal/gh/cli/testdata/README.md` の `--argjson types` 配列の末尾に `"PageInfo"` を足す。
そのうえで README のコマンドをそのまま実行する（**ネットワークが要る。テストではない**）。

```bash
gh api graphql -f query='
{ __schema { types { name kind
  possibleTypes { name }
  fields(includeDeprecated:true) {
    name type { kind name ofType { kind name ofType { kind name ofType { kind name } } } }
  }
} } }' > /tmp/schema-full.json
# README の /tmp/trim.jq を作り、--argjson types に "PageInfo" を足した配列で実行する
```

- [ ] **Step 9: スキーマ突き合わせが通ることを確認**

```bash
go test ./internal/gh/cli/ -run TestEveryFieldTheDocumentsSelectExistsInTheSchema -v
```

Expected: PASS。落ちる場合は `type PageInfo is not in testdata/schema.json` なので Step 8 に戻る

- [ ] **Step 10: 空振りの確認**

`review.graphql` から `after: $after` だけを一時的に消して Step 7 を走らせる。
`TestPRReviewContextWalksEveryPageOfThreads` の `after=CUR1` の主張が落ちることを見る。
確認したら戻す。

- [ ] **Step 11: `make check` とコミット**

```bash
make check
git add internal/gh/cli/review.graphql internal/gh/cli/review.go \
        internal/gh/cli/review_test.go internal/gh/cli/testdata/README.md \
        internal/gh/cli/testdata/schema.json
git commit -m "fix: walk every page of review threads instead of stopping at 100"
```

---

## Task 2: 残りの connection の上限を確定させる

**Files:**
- Modify: `internal/gh/cli/work.graphql`
- Modify: `internal/gh/cli/cli.go:196-200`（`ListAssignees` のコメント）
- Test: `internal/gh/cli/graphql_test.go`

**Interfaces:**
- Consumes: なし
- Produces: なし（クエリ文字列の変更のみ。ドメイン型は変わらない）

D5 の表がこのタスクの仕様。**据え置くものにも理由をコメントで残す。**
「なぜ 50 のままなのか」が書いていないと、次のセッションがまた同じ調査をやり直す。

- [ ] **Step 1: 落ちるテストを書く**

`internal/gh/cli/graphql_test.go` の import に `"regexp"` `"strconv"` `"strings"` を足し、
末尾に:

```go
// A label connection asked for 10 silently drops the eleventh label, and a
// Work card is where labels are read. GitHub caps first at 100, so there is
// no reason to ask for less.
func TestWorkQueryAsksForAsManyLabelsAsGitHubAllows(t *testing.T) {
	t.Parallel()

	if strings.Contains(workQuery, "labels(first: 10)") {
		t.Error("work.graphql still asks for 10 labels; GitHub allows 100")
	}
	if n := strings.Count(workQuery, "labels(first: 100)"); n != 2 {
		t.Errorf("labels(first: 100) appears %d times, want 2 (PullRequest and Issue)", n)
	}
}

// Every connection GitHub caps at 100 must ask for no more: 101 is refused
// outright with EXCESSIVE_PAGINATION, which fails the whole document.
func TestNoConnectionAsksForMoreThanGitHubAllows(t *testing.T) {
	t.Parallel()

	docs := map[string]string{
		"work.graphql":   workQuery,
		"review.graphql": reviewContextQuery,
	}
	re := regexp.MustCompile(`first:\s*(\d+)`)
	for name, doc := range docs {
		for _, m := range re.FindAllStringSubmatch(doc, -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if n > 100 {
				t.Errorf("%s: %s exceeds GitHub's cap of 100", name, m[0])
			}
		}
	}
}
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
go test ./internal/gh/cli/ -run 'TestWorkQueryAsksForAsManyLabels|TestNoConnectionAsksForMore' -v
```

Expected: `TestWorkQueryAsksForAsManyLabelsAsGitHubAllows` が FAIL
（`work.graphql still asks for 10 labels`）。もう一方は PASS

- [ ] **Step 3: `work.graphql` を直す**

`labels(first: 10)` を 2 箇所とも `labels(first: 100)` にする。
`LabelFields` fragment の上のコメントの直前に理由を足す。

```graphql
# labels asks for 100, GitHub's cap for a connection. There is no documented
# limit on how many labels one item may carry, and a card that silently drops
# labels past the tenth is the same failure as the Repos tab stopping at 30.
```

`search(...)` の 4 行の上、既存のクエリ冒頭コメントに据え置きの理由を足す。

```graphql
# search asks for 50 per column and is not paged. The Work board is what to
# look at now, not a full listing: GitHub's search returns at most 1000
# results however it is paged, and a column past 50 cards is already past
# what the board can be read as.
```

- [ ] **Step 4: テストが通ることを確認**

```bash
go test ./internal/gh/cli/ -v
```

Expected: PASS（スキーマ突き合わせを含む全件）

- [ ] **Step 5: `ListAssignees` の据え置き理由を書く**

`internal/gh/cli/cli.go` の `ListAssignees` の doc コメントに 1 文足す。

```go
// ListAssignees returns the logins of users assignable on the repository.
// gh api substitutes {owner}/{repo} from the current directory's repo; for an
// override we build the explicit path (gh api takes no --repo).
//
// per_page=100 is REST's maximum and the request is not paged. A repository
// with more than 100 assignable users gives the picker a list nobody can pick
// from, so paging would add requests without adding an answer.
```

- [ ] **Step 6: 空振りの確認**

`work.graphql` の `labels(first: 100)` を片方だけ `10` に戻して Step 4 を走らせ、
`TestWorkQueryAsksForAsManyLabelsAsGitHubAllows` が落ちることを見る。確認したら戻す。

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/gh/cli/work.graphql internal/gh/cli/cli.go internal/gh/cli/graphql_test.go
git commit -m "fix: ask for every label a card can carry, and say why the rest stay put"
```

---

## Task 3: 裸の定数と `Work` の配列長（設計書 §3.2）

**Files:**
- Modify: `internal/gh/gh.go:207`
- Modify: `internal/gh/cli/diff.go:145,163,270`
- Test: `internal/gh/gh_test.go`（無ければ作成）

**Interfaces:**
- Consumes: `gh.WorkSections() []WorkSection`
- Produces: `gh.WorkSectionCount` — `Work` の列数。`Work` は `[WorkSectionCount][]WorkItem` になる

- [ ] **Step 1: 落ちるテストを書く**

`internal/gh/gh_test.go`（`package gh_test`）に:

```go
// Work is indexed by WorkSection. A fifth column added without widening Work
// panics at run time on the first item that lands in it -- there is no
// compiler error to catch it.
//
// The section list here is written out by hand on purpose. Taking it from
// WorkSections() would give both sides of the comparison the same source and
// the test could not fail, which is the shape .claude/rules/testing.md warns
// about: a column added to the enum and not to this list is what makes it go
// red, and that red is the reminder.
func TestEverySectionConstantIsASlotInWork(t *testing.T) {
	t.Parallel()

	sections := []gh.WorkSection{
		gh.SectionReviewRequested,
		gh.SectionYourPRs,
		gh.SectionAssigned,
		gh.SectionMentioned,
	}

	var w gh.Work
	if len(w) != len(sections) {
		t.Fatalf("Work has %d slots, %d sections are declared", len(w), len(sections))
	}
	if got := len(gh.WorkSections()); got != len(sections) {
		t.Errorf("WorkSections() returns %d, %d sections are declared", got, len(sections))
	}
	for _, s := range sections {
		if int(s) < 0 || int(s) >= len(w) {
			t.Errorf("section %d is not an index into Work (len %d)", s, len(w))
		}
	}
}
```

- [ ] **Step 2: テストが通ることを確認（現状は 4 と 4 なので通る）**

```bash
go test ./internal/gh/ -run TestEverySectionConstantIsASlotInWork -v
```

Expected: PASS。**このタスクの主眼はテストではなく Step 3 の構造変更**である。
テストは「次に列を足した人が気づく」ための見張りで、今この瞬間に落ちるものではない。

- [ ] **Step 3: 列数を定数にする**

`internal/gh/gh.go` の末尾を置き換える。

```go
// WorkSection is one column of the Work board.
type WorkSection int

const (
	SectionReviewRequested WorkSection = iota
	SectionYourPRs
	SectionAssigned
	SectionMentioned

	// WorkSectionCount is how many columns there are. It has to be the last
	// constant in this block: Work is an array indexed by WorkSection, and
	// iota is the only thing that keeps its length and this list from
	// drifting apart.
	WorkSectionCount = iota
)

// WorkSections returns the columns in display order, left to right.
func WorkSections() []WorkSection {
	sections := make([]WorkSection, WorkSectionCount)
	for i := range sections {
		sections[i] = WorkSection(i)
	}
	return sections
}

// Work holds the items of each column, indexed by WorkSection.
type Work [WorkSectionCount][]WorkItem
```

- [ ] **Step 4: 空振りでないことを確認**

`SectionMentioned` の下（`WorkSectionCount` の上）に `SectionArchived` を一時的に足す。

```bash
go test ./internal/gh/ -run TestEverySectionConstantIsASlotInWork -v
```

Expected: **FAIL** — `Work has 5 slots, 4 sections are declared`。
手書きの一覧と実装の定数の出所が違うので、片方だけ増えたら赤くなる。
これが確認できたら `SectionArchived` を消す。

そのうえで全体を走らせる。

```bash
go test ./internal/gh/... ./internal/tui/... 2>&1 | tail -20
```

Expected: 全件 PASS。表示順が変わらないので golden も動かない

- [ ] **Step 5: `diff.go` の裸の数を名前にする**

`internal/gh/cli/diff.go` のパッケージ定数に足す（ファイル先頭の `const` ブロック、
無ければ import の下に作る）。

```go
const (
	// scanBufInit and scanBufMax size the diff scanner. A single diff line
	// can be far longer than bufio's default 64KiB limit -- a minified
	// bundle is one line -- and a scanner that gives up mid-file drops every
	// file after it.
	scanBufInit = 64 * 1024
	scanBufMax  = 8 * 1024 * 1024

	// hunkHeaderFields is how many whitespace-separated fields of an
	// `@@ -12,7 +12,9 @@ func Walk(...)` header carry the ranges. Anything
	// after the second @@ is git's enclosing-function heuristic, and a
	// one-line function body puts a + or - token there too.
	hunkHeaderFields = 3
)
```

`parseBarePatch` と `parseDiff` の `s.Buffer(...)` を両方:

```go
	s.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
```

`hunkStarts` を:

```go
	for _, f := range fields[:min(hunkHeaderFields, len(fields))] {
```

`parseDiff` の中にあった「A single diff line can be far longer than…」のコメントは
定数側に移したので削除する（同じ説明を 2 箇所に置かない）。

- [ ] **Step 6: テストが通ることを確認**

```bash
go test ./internal/gh/... -v 2>&1 | tail -20
```

Expected: PASS（既存の diff パーステストがそのまま通る。挙動は変えていない）

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/gh/gh.go internal/gh/gh_test.go internal/gh/cli/diff.go
git commit -m "refactor: name the diff scanner's sizes and tie Work's length to its sections"
```

---

## Task 4: testdata を実物で録り直す（作業順 5）

**Files:**
- Create: `internal/gh/cli/testdata/pr_list.json`
- Create: `internal/gh/cli/testdata/issue_list.json`
- Create: `internal/gh/cli/testdata/pr_view.json`
- Create: `internal/gh/cli/testdata/issue_view.json`
- Create: `internal/gh/cli/testdata/work.json`
- Create: `internal/gh/cli/testdata/pr_files.json`
- Modify: `internal/gh/cli/testdata/README.md`
- Modify: `internal/gh/cli/cli_test.go` / `graphql_test.go` / `diff_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `readTestdata(t *testing.T, name string) string` — `internal/gh/cli` のテストヘルパー

このタスクは**ネットワークが要る**（録る手順のみ。テスト自体は録ったファイルを読むだけ）。

- [ ] **Step 1: 実物を録る**

`kukv/octoscope` に対して実行する。秘密情報が入らないよう、公開リポジトリだけを使う。

```bash
cd /path/to/octoscope   # 実リポジトリのチェックアウト
D=internal/gh/cli/testdata

gh pr list --repo kukv/octoscope --state all --json \
  'number,title,author,state,isDraft,updatedAt,reviewDecision,url,labels,headRefName,baseRefName,additions,deletions,statusCheckRollup' \
  --limit 100 | jq . > $D/pr_list.json

gh issue list --repo kukv/octoscope --state all --json \
  'number,title,author,state,updatedAt,labels,url' --limit 100 | jq . > $D/issue_list.json

gh pr view 55 --repo kukv/octoscope --json \
  'number,title,author,state,isDraft,updatedAt,reviewDecision,url,labels,headRefName,baseRefName,additions,deletions,statusCheckRollup,body,comments,assignees' \
  | jq . > $D/pr_view.json

gh issue view 50 --repo kukv/octoscope --json \
  'number,title,author,state,updatedAt,labels,url,body,comments,assignees' | jq . > $D/issue_view.json

# work.graphql の検索は review-requested:@me などで、自分が見えるリポジトリを
# 全部なめる。bodyText まで選んでいるので、そのまま録ると私有リポジトリの
# タイトルと本文が公開リポジトリにコミットされる。kukv/octoscope の分だけ残す。
# これは「テストを通すための編集」ではなく秘密情報の除去であり、README でも
# その区別を書く。
gh api graphql -F query=@internal/gh/cli/work.graphql \
  | jq '.data |= with_entries(
          .value.nodes |= map(select(.repository.nameWithOwner == "kukv/octoscope")))' \
  > $D/work.json

# prFiles は --paginate 付きで叩く。録るときも同じにする。
# gh は REST の配列レスポンスを 1 つの配列にマージして吐くので、prFiles の
# json.Unmarshal(out, &entries) がそのまま通る形になる。
gh api 'repos/kukv/octoscope/pulls/55/files?per_page=100' --paginate | jq . > $D/pr_files.json
```

- [ ] **Step 2: 録ったものを目で見る**

```bash
jq 'length' internal/gh/cli/testdata/pr_list.json
jq '.data | keys' internal/gh/cli/testdata/work.json
jq '.[0] | keys' internal/gh/cli/testdata/pr_files.json
```

`work.json` の 4 つのエイリアス（`reviewRequested` / `yourPRs` / `assigned` / `mentioned`）が
揃っていることと、`pr_list.json` が 1 件以上あることを確認する。
**メールアドレスなど秘密情報が入っていないことも見る。**

- [ ] **Step 3: README に録り方を書く**

`internal/gh/cli/testdata/README.md` に、`schema.json` の節の前に節を足す。

````markdown
## レスポンスの録りもの

`pr_list.json` / `issue_list.json` / `pr_view.json` / `issue_view.json` /
`work.json` / `pr_files.json` は、`gh` の実出力をそのまま録ったもの。
録った日: 2026-09-07、対象: `kukv/octoscope`（PR #55、Issue #50）。

パースのテストは手書きの JSON ではなくこれを読む。手書きだと「GitHub が実際には
そう返さない形」でも通ってしまい、返し方が変わったときに気づけない。

```bash
D=internal/gh/cli/testdata
gh pr list --repo kukv/octoscope --state all --json \
  'number,title,author,state,isDraft,updatedAt,reviewDecision,url,labels,headRefName,baseRefName,additions,deletions,statusCheckRollup' \
  --limit 100 | jq . > $D/pr_list.json
# 以下同様（Task 4 Step 1 のコマンド一式）
```

`work.json` は `@me` を含む検索なので、**録った人が見えるリポジトリを全部なめる**。
録るときに `jq` で `kukv/octoscope` の分だけに絞る（上のコマンド参照）。
**これは秘密情報の除去であって、「テストを通すための編集」ではない。**
前者は必須、後者は禁止。

件数はテストの主張に使わない（録り直すたびに変わる）。使うのは
「4 つのエイリアスが揃っていること」と「`__typename` ごとの変換結果」だけ。

`pr_list.json` は `--state all` で録っている（`ListPRs` 自身は open だけを取る）。
open / closed / merged の 3 状態が 1 ファイルに入るほうが、`ParseItemState` の
変換をまとめて確かめられるため。
````

- [ ] **Step 4: テストヘルパーを足す**

`internal/gh/cli/cli_test.go` の import に `"os"` `"path/filepath"` を足し、その下に:

```go
// readTestdata returns a recorded gh response. The recordings are what the
// parse tests answer with: hand-written JSON only proves the parser handles
// the shape the test author imagined (see testdata/README.md).
func readTestdata(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return string(b)
}
```

- [ ] **Step 5: 落ちるテストを書く**

`internal/gh/cli/cli_test.go` に:

```go
// The recorded response is what gh actually prints. A parser that only ever
// sees hand-written JSON passes on a shape GitHub may not send.
func TestListPRsParsesARecordedResponse(t *testing.T) {
	c, _ := newTestClient(readTestdata(t, "pr_list.json"), nil)

	prs, err := c.ListPRs(t.Context())
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) == 0 {
		t.Fatal("no pull requests parsed out of the recording")
	}
	for _, pr := range prs {
		if pr.Number == 0 {
			t.Errorf("pr %q has no number", pr.Title)
		}
		if pr.Title == "" {
			t.Errorf("pr #%d has no title", pr.Number)
		}
		if pr.Author.Login == "" {
			t.Errorf("pr #%d has no author", pr.Number)
		}
		if pr.URL == "" {
			t.Errorf("pr #%d has no url; o has nothing to open", pr.Number)
		}
		if pr.UpdatedAt.IsZero() {
			t.Errorf("pr #%d has no updatedAt; the board sorts on it", pr.Number)
		}
	}
}

func TestGetPRParsesARecordedResponse(t *testing.T) {
	c, _ := newTestClient(readTestdata(t, "pr_view.json"), nil)

	pr, err := c.GetPR(t.Context(), "", 55)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.Number != 55 {
		t.Errorf("number = %d, want 55", pr.Number)
	}
	if pr.Body == "" {
		t.Error("body is empty; the detail view has nothing to draw")
	}
	if pr.Head == "" || pr.Base == "" {
		t.Errorf("head/base = %q/%q, want both", pr.Head, pr.Base)
	}
}
```

`internal/gh/cli/graphql_test.go` に:

```go
// ListWork reads four aliased searches out of one response. A recording is
// the only way to know the aliases the query declares and the keys the answer
// carries are still the same four.
func TestListWorkParsesARecordedResponse(t *testing.T) {
	c, _ := newTestClient(readTestdata(t, "work.json"), nil)

	w, err := c.ListWork(t.Context())
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	for _, section := range gh.WorkSections() {
		for _, item := range w[section] {
			if item.Ref.Repo == "" {
				t.Errorf("section %d: %q has no repo; the card cannot be opened", section, item.Title)
			}
			if item.Ref.Number == 0 {
				t.Errorf("section %d: %q has no number", section, item.Title)
			}
			if item.URL == "" {
				t.Errorf("section %d: %q has no url", section, item.Title)
			}
		}
	}
}
```

`internal/gh/cli/diff_test.go` に:

```go
// The files API returns a patch per file, not a whole diff, and prFiles is
// what PRDiff falls back to when `gh pr diff` fails. The recording is what
// proves parseBarePatch is fed the shape GitHub actually sends.
func TestPRFilesParsesARecordedResponse(t *testing.T) {
	c, _ := newTestClient(readTestdata(t, "pr_files.json"), nil)

	files, err := c.prFiles(t.Context(), "", 55)
	if err != nil {
		t.Fatalf("prFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files parsed out of the recording")
	}
	for _, f := range files {
		if f.Path == "" {
			t.Error("a file has no path")
		}
	}
}
```

> `prFiles` は非公開。`diff_test.go` は `package cli`（内部テストパッケージ）なので
> そのまま呼べる。実装側の名前は変えない（設計書 §7）。
> 録るときも実装と同じ `--paginate` を付ける（Step 1 のコマンド参照）。

- [ ] **Step 6: テストを走らせる**

```bash
go test ./internal/gh/cli/ -run 'ParsesARecordedResponse' -v
```

Expected: 最初は FAIL の可能性がある（録ったフィールドと実装の期待がずれている箇所が
あればそこが本物のバグ）。**落ちたら実装を直す。testdata は書き換えない**（README の方針）

- [ ] **Step 7: 空振りの確認**

`internal/gh/cli/item.go`（または `prJSON.toDomain`）の `URL` の代入を一時的に消して
Step 6 を走らせ、`has no url` が落ちることを見る。確認したら戻す。

- [ ] **Step 8: `make check` とコミット**

```bash
make check
git add internal/gh/cli/testdata internal/gh/cli/cli_test.go \
        internal/gh/cli/graphql_test.go internal/gh/cli/diff_test.go
git commit -m "test: parse recorded gh responses instead of hand-written JSON"
```

---

## Task 5: `internal/usecase` を作る — 種別の振り分けと素通し

**Files:**
- Create: `internal/usecase/usecase.go`
- Create: `internal/usecase/usecase_test.go`

**Interfaces:**
- Consumes: `*cli.Client` のメソッド一式（下の `source` interface が要求する分）
- Produces:
  - `type Usecase struct{ ... }`
  - `func New(src source) *Usecase`
  - `type Item struct { Kind gh.ItemKind; Number int; Title string; State gh.ItemState; Body string; Labels []gh.Label; Assignees []gh.Author; Comments []gh.Comment; UpdatedAt time.Time; URL string; PR *gh.PR }`
  - `func (u *Usecase) GetItem(ctx context.Context, ref gh.ItemRef) (Item, error)`
  - `func (u *Usecase) AddComment(ref gh.ItemRef, body string) error`
  - `func (u *Usecase) SetState(ref gh.ItemRef, closing bool) error`
  - `func (u *Usecase) EditLabels(ref gh.ItemRef, add, remove []string) error`
  - `func (u *Usecase) EditAssignees(ref gh.ItemRef, add, remove []string) error`
  - `func (u *Usecase) OpenWeb(url string) error`
  - 素通し: `ListWork` / `ListPRs` / `ListIssues` / `RepoName` / `PRDiff` / `PRReviewContext` / `ListLabels` / `ListAssignees` / `DiscardReview`

**設計書 §4.2(a) の `Item` に `URL` と `Author` を足している。** 設計書の列挙には
無いが、両方とも画面が読む。

- `URL`: `detail.Model.url` が `o` の対象として使う。無いと呼び出し側が `Item.PR` から
  引くか `GetPR` をもう一度呼ぶことになり、振り分けを消す目的に反する
- `Author`: `internal/tui/detail/render.go:186` の `issueMarkdown` が
  `@%s` として出す。無いと golden が変わる

`prMarkdown` / `issueMarkdown` が読むフィールドを実測で数え上げた結果、
`Item` が持つべきなのは上の 11 個で足りる（`prMarkdown` は `gh.PR` を
そのまま受け取り続けるので、`IsDraft` / `Review` は `Item.PR` 経由で読める）。

| 関数 | 読むフィールド |
|---|---|
| `prMarkdown(gh.PR)` | `Number` `Title` `Author.Login` `State` `IsDraft` `Review` `Labels` `UpdatedAt` `Body` `Comments` |
| `issueMarkdown(usecase.Item)` | `Number` `Title` `Author.Login` `State` `Labels` `UpdatedAt` `Body` `Comments` |

**`Item` を今後太らせない規則（`architecture.md` にも入れる。Task 8 Step 5.5）**

`Item` は「UI の都合が下の層に漏れる唯一の穴」である。ここを塞がないと、
画面が増えるたびにフィールドが増え、設計書 §7 が禁じている DTO になる。

- **`Item` に共通フィールドを足してよいのは、PR と Issue の両方に GitHub 側の
  対応物があるときだけ。**
- PR にしか無いものは `Item.PR` から読む。`Item` に写さない。
- 現時点で `gh.Issue` の公開フィールドは 10 個すべて `Item` に入っている
  （`Number` `Title` `Author` `State` `UpdatedAt` `URL` `Body` `Comments`
  `Labels` `Assignees`）。**つまり「画面に出したいものが `Item` に無い」は、
  当面は起きない。** 起きたときは、まず `gh` 側に無いのではないかを疑う。

この規則があると、UI だけの修正（色・桁・文言・キー・状態遷移・描画するものの
選び方）は `internal/usecase` に一切波及しない。波及するのは
**GitHub への新しい操作を足すとき**だけで、それは元から UI だけの修正ではない
（触るファイルが `cli` + `detail` の 2 つから `cli` + `usecase` + `detail` の
3 つに増える。これが usecase を入れることの唯一の実コストである）。

- [ ] **Step 1: 落ちるテストを書く**

`internal/usecase/usecase_test.go` は **`package usecase`**（内部テストパッケージ）。
`Usecase` の非公開フィールドに直接フェイクを差し込むためで、
`.claude/rules/testing.md` の「非公開フィールドを読むテストは内部テストパッケージ」に従う。

```go
package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

// fakeSource answers the calls GetItem makes and records which it made. The
// point of GetItem is that the view stops choosing between the PR and the
// Issue call, so what this fake proves is which one the usecase chose.
type fakeSource struct {
	pr     gh.PR
	issue  gh.Issue
	err    error
	called []string
}

func (f *fakeSource) GetPR(_ context.Context, _ string, _ int) (gh.PR, error) {
	f.called = append(f.called, "GetPR")
	return f.pr, f.err
}

func (f *fakeSource) GetIssue(_ context.Context, _ string, _ int) (gh.Issue, error) {
	f.called = append(f.called, "GetIssue")
	return f.issue, f.err
}

func TestGetItemFetchesAPullRequestForAPRRef(t *testing.T) {
	t.Parallel()

	f := &fakeSource{pr: gh.PR{
		Number: 55, Title: "feat: x", State: gh.StateOpen,
		Body: "body", URL: "https://example.test/pull/55",
		UpdatedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	}}
	u := &Usecase{items: f}

	item, err := u.GetItem(t.Context(), gh.ItemRef{Kind: gh.ItemPR, Number: 55})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if len(f.called) != 1 || f.called[0] != "GetPR" {
		t.Fatalf("calls = %v, want exactly [GetPR]", f.called)
	}
	if item.Kind != gh.ItemPR || item.Number != 55 || item.Title != "feat: x" {
		t.Errorf("item = %+v, want the pull request's own fields", item)
	}
	if item.URL != "https://example.test/pull/55" {
		t.Errorf("url = %q; o would have nothing to open", item.URL)
	}
	// PR is what the detail view reads for branches, checks and review state.
	// An issue has none of it, so it is a pointer and only a PR ref fills it.
	if item.PR == nil {
		t.Fatal("PR is nil on a pull request; the detail view loses head/base and checks")
	}
	if item.PR.Number != 55 {
		t.Errorf("PR.Number = %d, want 55", item.PR.Number)
	}
}

func TestGetItemFetchesAnIssueForAnIssueRef(t *testing.T) {
	t.Parallel()

	f := &fakeSource{issue: gh.Issue{
		Number: 50, Title: "bug: y", State: gh.StateClosed,
		Body: "body", URL: "https://example.test/issues/50",
	}}
	u := &Usecase{items: f}

	item, err := u.GetItem(t.Context(), gh.ItemRef{Kind: gh.ItemIssue, Number: 50})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if len(f.called) != 1 || f.called[0] != "GetIssue" {
		t.Fatalf("calls = %v, want exactly [GetIssue]", f.called)
	}
	if item.Kind != gh.ItemIssue || item.Number != 50 || item.State != gh.StateClosed {
		t.Errorf("item = %+v, want the issue's own fields", item)
	}
	if item.PR != nil {
		t.Error("PR is set on an issue; there is no pull request behind it")
	}
}

func TestGetItemPassesTheFetchFailureThrough(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	u := &Usecase{items: &fakeSource{err: want}}

	if _, err := u.GetItem(t.Context(), gh.ItemRef{Kind: gh.ItemPR}); !errors.Is(err, want) {
		t.Errorf("err = %v, want it to wrap %v", err, want)
	}
}
```

> `fakeSource` は `GetPR` / `GetIssue` しか持たない。`usecase.New` が要求する
> `source` interface が大きすぎると、このフェイクが書けない。**書けなかったら
> `source` を分割する合図である**（`.claude/rules/architecture.md`）。
> Step 3 では `New` を「必要な分だけの小さな interface の合成」で受ける。

- [ ] **Step 2: テストが落ちることを確認**

```bash
go test ./internal/usecase/ -v
```

Expected: FAIL — `no required module provides package .../internal/usecase`（まだ無い）

- [ ] **Step 3: `internal/usecase/usecase.go` を書く**

```go
// Package usecase is where an operation that takes more than one call to the
// GitHub layer, or that picks a call by the kind of item, is decided. The
// views above it say what they want done; which requests that takes, and in
// what order, is not their business.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

// The source interfaces are split by what each operation needs, so a test can
// build a fake with only the methods that operation calls
// (.claude/rules/architecture.md).

type itemFetcher interface {
	GetPR(ctx context.Context, repo string, number int) (gh.PR, error)
	GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error)
}

// The write side is split four ways rather than kept as one itemWriter: ten
// methods on one declaration is over the limit the rules set, and GitHub
// having a separate endpoint per kind is not a reason for this package to
// have one big interface.

type commenter interface {
	AddPRComment(repo string, number int, body string) error
	AddIssueComment(repo string, number int, body string) error
}

type stateChanger interface {
	ClosePR(repo string, number int) error
	ReopenPR(repo string, number int) error
	CloseIssue(repo string, number int) error
	ReopenIssue(repo string, number int) error
}

type labelEditor interface {
	EditPRLabels(repo string, number int, add, remove []string) error
	EditIssueLabels(repo string, number int, add, remove []string) error
}

type assigneeEditor interface {
	EditPRAssignees(repo string, number int, add, remove []string) error
	EditIssueAssignees(repo string, number int, add, remove []string) error
}

type lister interface {
	ListWork(ctx context.Context) (gh.Work, error)
	ListPRs(ctx context.Context) ([]gh.PR, error)
	ListIssues(ctx context.Context) ([]gh.Issue, error)
	RepoName(ctx context.Context) (string, error)
	ListLabels(ctx context.Context, repo string) ([]gh.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

type reviewer interface {
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
	StartReview(pullRequestID string) (string, error)
	AddReviewThread(reviewID string, c gh.PendingComment) error
	SubmitReview(reviewID string, event gh.ReviewEvent, body string) error
	SubmitNewReview(pullRequestID string, event gh.ReviewEvent, body string) error
	DiscardReview(reviewID string) error
}

type opener interface {
	OpenWeb(url string) error
}

// source is the whole of what a backend has to provide. It is a composition
// of the interfaces above rather than one long list, so that a test can build
// a fake for just the half it exercises (see usecase_test.go, which is in
// this package for exactly that reason).
type source interface {
	itemFetcher
	commenter
	stateChanger
	labelEditor
	assigneeEditor
	lister
	reviewer
	opener
}

// Usecase holds one backend, split across the fields by what each operation
// needs. The split is what keeps any single interface declaration small.
type Usecase struct {
	items     itemFetcher
	comments  commenter
	states    stateChanger
	labels    labelEditor
	assignees assigneeEditor
	lists     lister
	reviews   reviewer
	web       opener
}

// New wires a Usecase to one backend. cmd/octoscope passes a *cli.Client;
// the compiler checks there that it still has every method, which is the
// whole point of naming the union.
func New(src source) *Usecase {
	return &Usecase{
		items:     src,
		comments:  src,
		states:    src,
		labels:    src,
		assignees: src,
		lists:     src,
		reviews:   src,
		web:       src,
	}
}

// Item is where a pull request and an issue meet. The detail view draws one
// screen for both, and the two GitHub types do not unify -- so this is the
// shape that screen reads. It is not a DTO: nothing here is a second spelling
// of a domain field, and PR is the pull request itself, not a copy of it.
//
// A field belongs on Item only when GitHub gives both a pull request and an
// issue their own. Anything only a pull request has is read through PR, not
// copied up to here. Adding a field because one screen wants to draw it is
// how this turns into a DTO, and a DTO is what a screen's needs leaking a
// layer down looks like.
type Item struct {
	Kind      gh.ItemKind
	Number    int
	Title     string
	Author    gh.Author
	State     gh.ItemState
	Body      string
	URL       string
	Labels    []gh.Label
	Assignees []gh.Author
	Comments  []gh.Comment
	UpdatedAt time.Time

	// PR is set only when Kind is ItemPR. It carries what an issue has no
	// equivalent of: the branches, the size of the change, the checks and
	// the review decision.
	PR *gh.PR
}

// GetItem fetches whichever of the two the reference names. Choosing between
// them is what ItemRef.Kind is for, and no view has to do it.
func (u *Usecase) GetItem(ctx context.Context, ref gh.ItemRef) (Item, error) {
	if ref.Kind == gh.ItemPR {
		pr, err := u.items.GetPR(ctx, ref.Repo, ref.Number)
		if err != nil {
			return Item{}, fmt.Errorf("get pr: %w", err)
		}
		return Item{
			Kind: gh.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
			State: pr.State, Body: pr.Body, URL: pr.URL, Labels: pr.Labels,
			Assignees: pr.Assignees, Comments: pr.Comments, UpdatedAt: pr.UpdatedAt,
			PR: &pr,
		}, nil
	}
	issue, err := u.items.GetIssue(ctx, ref.Repo, ref.Number)
	if err != nil {
		return Item{}, fmt.Errorf("get issue: %w", err)
	}
	return Item{
		Kind: gh.ItemIssue, Number: issue.Number, Title: issue.Title, Author: issue.Author,
		State: issue.State, Body: issue.Body, URL: issue.URL, Labels: issue.Labels,
		Assignees: issue.Assignees, Comments: issue.Comments, UpdatedAt: issue.UpdatedAt,
	}, nil
}

func (u *Usecase) AddComment(ref gh.ItemRef, body string) error {
	if ref.Kind == gh.ItemPR {
		return u.comments.AddPRComment(ref.Repo, ref.Number, body)
	}
	return u.comments.AddIssueComment(ref.Repo, ref.Number, body)
}

// SetState closes the item when closing is true and reopens it otherwise.
// Whether either is possible is the view's question -- a merged pull request
// is neither -- and it asks before calling.
func (u *Usecase) SetState(ref gh.ItemRef, closing bool) error {
	switch {
	case ref.Kind == gh.ItemPR && closing:
		return u.states.ClosePR(ref.Repo, ref.Number)
	case ref.Kind == gh.ItemPR:
		return u.states.ReopenPR(ref.Repo, ref.Number)
	case closing:
		return u.states.CloseIssue(ref.Repo, ref.Number)
	default:
		return u.states.ReopenIssue(ref.Repo, ref.Number)
	}
}

func (u *Usecase) EditLabels(ref gh.ItemRef, add, remove []string) error {
	if ref.Kind == gh.ItemPR {
		return u.labels.EditPRLabels(ref.Repo, ref.Number, add, remove)
	}
	return u.labels.EditIssueLabels(ref.Repo, ref.Number, add, remove)
}

func (u *Usecase) EditAssignees(ref gh.ItemRef, add, remove []string) error {
	if ref.Kind == gh.ItemPR {
		return u.assignees.EditPRAssignees(ref.Repo, ref.Number, add, remove)
	}
	return u.assignees.EditIssueAssignees(ref.Repo, ref.Number, add, remove)
}

// The rest delegate one for one. They are here so that every view asks the
// same package for everything: an operation that grows a second call later
// grows it in one place, and no view has to change which package it talks to
// (see the spec's 4.2(c)).

func (u *Usecase) ListWork(ctx context.Context) (gh.Work, error) { return u.lists.ListWork(ctx) }

func (u *Usecase) ListPRs(ctx context.Context) ([]gh.PR, error) { return u.lists.ListPRs(ctx) }

func (u *Usecase) ListIssues(ctx context.Context) ([]gh.Issue, error) {
	return u.lists.ListIssues(ctx)
}

func (u *Usecase) RepoName(ctx context.Context) (string, error) { return u.lists.RepoName(ctx) }

func (u *Usecase) ListLabels(ctx context.Context, repo string) ([]gh.Label, error) {
	return u.lists.ListLabels(ctx, repo)
}

func (u *Usecase) ListAssignees(ctx context.Context, repo string) ([]string, error) {
	return u.lists.ListAssignees(ctx, repo)
}

func (u *Usecase) PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	return u.reviews.PRDiff(ctx, repo, number)
}

func (u *Usecase) PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error) {
	return u.reviews.PRReviewContext(ctx, repo, number)
}

func (u *Usecase) DiscardReview(reviewID string) error {
	return u.reviews.DiscardReview(reviewID)
}

func (u *Usecase) OpenWeb(url string) error { return u.web.OpenWeb(url) }
```

> **なぜフィールドを 8 つに割るのか。** `New` は `source`（全部の合成）を要求するので、
> `*cli.Client` からメソッドが 1 つ欠けたらコンパイルエラーになる。一方
> フィールドを小さい interface で持つことで、テストは `&Usecase{items: f}` と
> 必要な分だけ差し込める。**`any` を取って型アサーションする形にはしない** —
> それはコンパイル時の保証を捨てて実行時 nil panic に変えるだけである。

- [ ] **Step 4: テストが通ることを確認**

```bash
go test ./internal/usecase/ -v
```

Expected: PASS（3 本とも）

- [ ] **Step 5: 空振りの確認**

`GetItem` の `ref.Kind == gh.ItemPR` を `false` に一時的に固定して Step 4 を走らせ、
`TestGetItemFetchesAPullRequestForAPRRef` が `calls = [GetIssue], want exactly [GetPR]` で
落ちることを見る。確認したら戻す。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/usecase
git commit -m "feat: add internal/usecase and move the PR/Issue split out of the views"
```

---

## Task 6: `usecase` にオーケストレーションを移す

**Files:**
- Create: `internal/usecase/review.go`
- Create: `internal/usecase/review_test.go`

**Interfaces:**
- Consumes: Task 5 の `reviewer` interface
- Produces:
  - `type ReviewTarget struct{ PullRequestID, PendingID string }`
  - `func (u *Usecase) PostLineComment(t ReviewTarget, c gh.PendingComment) (reviewID string, err error)`
  - `func (u *Usecase) SubmitReview(t ReviewTarget, event gh.ReviewEvent, body string) error`

D2 のとおり、どちらも `ctx` を取らない。

- [ ] **Step 1: 落ちるテストを書く**

`internal/usecase/review_test.go` も **`package usecase`**（Task 5 と同じ理由）:

```go
// fakeReviewer records the review calls in order. What these tests are about
// is the order and the choice, not the payloads.
type fakeReviewer struct {
	calls     []string
	newID     string
	startErr  error
	threadErr error
	submitErr error
}

func (f *fakeReviewer) StartReview(_ string) (string, error) {
	f.calls = append(f.calls, "StartReview")
	return f.newID, f.startErr
}

func (f *fakeReviewer) AddReviewThread(reviewID string, _ gh.PendingComment) error {
	f.calls = append(f.calls, "AddReviewThread("+reviewID+")")
	return f.threadErr
}

func (f *fakeReviewer) SubmitReview(_ string, _ gh.ReviewEvent, _ string) error {
	f.calls = append(f.calls, "SubmitReview")
	return f.submitErr
}

func (f *fakeReviewer) SubmitNewReview(_ string, _ gh.ReviewEvent, _ string) error {
	f.calls = append(f.calls, "SubmitNewReview")
	return f.submitErr
}

// A line comment needs a review to hang off. GitHub allows one unsubmitted
// review per user per pull request, so a second StartReview on a PR that
// already has one leaves a duplicate the user cannot see.
func TestPostLineCommentStartsAReviewOnlyWhenThereIsNone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pendingID string
		newID     string
		wantCalls []string
		wantID    string
	}{
		{
			name:      "no pending review: start one first",
			pendingID: "",
			newID:     "REV_new",
			wantCalls: []string{"StartReview", "AddReviewThread(REV_new)"},
			wantID:    "REV_new",
		},
		{
			name:      "pending review already open: add straight to it",
			pendingID: "REV_open",
			wantCalls: []string{"AddReviewThread(REV_open)"},
			wantID:    "REV_open",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeReviewer{newID: tc.newID}
			u := &Usecase{reviews: f}

			id, err := u.PostLineComment(
				ReviewTarget{PullRequestID: "PR_1", PendingID: tc.pendingID},
				gh.PendingComment{Path: "a.go", Line: 1, Body: "nit"},
			)
			if err != nil {
				t.Fatalf("%s: PostLineComment: %v", tc.name, err)
			}
			if !slices.Equal(f.calls, tc.wantCalls) {
				t.Errorf("%s: calls = %v, want %v", tc.name, f.calls, tc.wantCalls)
			}
			// The caller reuses the id for the next comment, so a wrong one
			// means every later comment starts another review.
			if id != tc.wantID {
				t.Errorf("%s: reviewID = %q, want %q", tc.name, id, tc.wantID)
			}
		})
	}
}

// A failed StartReview must not be followed by an AddReviewThread against an
// empty id: that call succeeds against nothing and the comment disappears.
func TestPostLineCommentStopsWhenTheReviewCannotBeStarted(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	f := &fakeReviewer{startErr: boom}
	u := &Usecase{reviews: f}

	if _, err := u.PostLineComment(ReviewTarget{PullRequestID: "PR_1"}, gh.PendingComment{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
	if !slices.Equal(f.calls, []string{"StartReview"}) {
		t.Errorf("calls = %v, want the walk to stop at StartReview", f.calls)
	}
}

// Approving a diff the viewer had nothing to say about is the commonest
// review there is. Going through StartReview first would leave an empty
// pending review behind if the submission then failed.
func TestSubmitReviewPicksTheOneCallThatFitsTheTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pendingID string
		wantCalls []string
	}{
		{"comments waiting: submit the pending review", "REV_open", []string{"SubmitReview"}},
		{"nothing waiting: create and submit in one call", "", []string{"SubmitNewReview"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeReviewer{}
			u := &Usecase{reviews: f}

			err := u.SubmitReview(
				ReviewTarget{PullRequestID: "PR_1", PendingID: tc.pendingID},
				gh.EventApprove, "lgtm",
			)
			if err != nil {
				t.Fatalf("%s: SubmitReview: %v", tc.name, err)
			}
			if !slices.Equal(f.calls, tc.wantCalls) {
				t.Errorf("%s: calls = %v, want %v", tc.name, f.calls, tc.wantCalls)
			}
		})
	}
}
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
go test ./internal/usecase/ -run 'PostLineComment|SubmitReview' -v
```

Expected: FAIL — `u.PostLineComment undefined` / `usecase.ReviewTarget undefined`

- [ ] **Step 3: `internal/usecase/review.go` を書く**

```go
package usecase

import (
	"fmt"

	"github.com/kukv/octoscope/internal/gh"
)

// ReviewTarget names the pull request a review acts on, and the unsubmitted
// review already open on it if there is one. GitHub allows one unsubmitted
// review per user per pull request, so PendingID is a single id, not a list.
type ReviewTarget struct {
	PullRequestID string
	PendingID     string
}

// PostLineComment attaches one line comment to the pull request's unsubmitted
// review, starting that review first if there is none.
//
// The order is GitHub's, not the diff view's: a line comment has to hang off
// a review, and a review the reader never submits must not be created just
// because they opened the diff. The returned id is the review the comment
// went onto; the caller keeps it so the next comment does not start a second
// one before the refetch lands.
func (u *Usecase) PostLineComment(t ReviewTarget, c gh.PendingComment) (string, error) {
	reviewID := t.PendingID
	if reviewID == "" {
		id, err := u.reviews.StartReview(t.PullRequestID)
		if err != nil {
			return "", fmt.Errorf("start review: %w", err)
		}
		reviewID = id
	}
	if err := u.reviews.AddReviewThread(reviewID, c); err != nil {
		return "", fmt.Errorf("add review thread: %w", err)
	}
	return reviewID, nil
}

// SubmitReview sends the review out. When line comments are already waiting
// it submits the review they are on; when nothing is waiting it creates and
// submits in one call, so a plain approval leaves nothing behind if it fails.
func (u *Usecase) SubmitReview(t ReviewTarget, event gh.ReviewEvent, body string) error {
	if t.PendingID != "" {
		if err := u.reviews.SubmitReview(t.PendingID, event, body); err != nil {
			return fmt.Errorf("submit review: %w", err)
		}
		return nil
	}
	if err := u.reviews.SubmitNewReview(t.PullRequestID, event, body); err != nil {
		return fmt.Errorf("submit new review: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: テストが通ることを確認**

```bash
go test ./internal/usecase/ -v
```

Expected: PASS（Task 5 のものを含めて全件）

- [ ] **Step 5: 空振りの確認**

`PostLineComment` の `if reviewID == ""` を `if true` に一時的に変えて Step 4 を走らせ、
`pending review already open` のケースが `calls = [StartReview AddReviewThread(REV_new)]` で
落ちることを見る。確認したら戻す。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/usecase/review.go internal/usecase/review_test.go
git commit -m "feat: move the review API call order into usecase"
```

---

## Task 7: TUI を `usecase` に載せ替える

**Files:**
- Modify: `internal/tui/detail/detail.go`
- Modify: `internal/tui/detail/render.go`（`gh.PR` / `gh.Issue` を読んでいる箇所）
- Modify: `internal/tui/diff/diff.go` / `internal/tui/diff/comment.go`
- Modify: `internal/tui/review/review.go`
- Modify: `internal/tui/app/app.go`
- Modify: `cmd/octoscope/main.go`
- Test: 各パッケージの既存 `*_test.go` のフェイク

**Interfaces:**
- Consumes: Task 5 / 6 の `*usecase.Usecase`
- Produces:
  - `detail.Source` — `itemSource`(6) + `candidateSource`(2) + `webOpener`(1) + `reviewOpener`(1) + `review.Source`(1) の合成
  - `review.Source` — `SubmitReview(t usecase.ReviewTarget, event gh.ReviewEvent, body string) error` の 1 メソッド
  - `diff.Source` — `PRDiff` / `PRReviewContext` / `PostLineComment` / `DiscardReview` + `review.Source`

**この計画で最も広く触るタスク。golden は 1 バイトも変わってはいけない。**

- [ ] **Step 1: 変更前の golden を固定する**

```bash
make test
git status --short -- '*.golden'
```

Expected: 出力なし（作業前の状態で golden が最新であることの確認）

- [ ] **Step 2: `review.Source` を 1 メソッドにする**

`internal/tui/review/review.go`:

```go
// Source is what submitting needs. Which GitHub call that takes -- the
// pending review's submit or a create-and-submit in one -- is decided in
// internal/usecase, not here: it is GitHub's rule about unsubmitted reviews,
// not a question about this popup.
type Source interface {
	SubmitReview(t usecase.ReviewTarget, event gh.ReviewEvent, body string) error
}
```

`Target` はそのまま（`PullRequestID` / `PendingID` / `PendingComments`）。
`submit()` を置き換える:

```go
// submit sends the review. Which call that takes is usecase's decision; this
// hands over the target and waits for the answer.
func (m Model) submit() (Model, tea.Cmd) {
	src := m.src
	target := usecase.ReviewTarget{
		PullRequestID: m.target.PullRequestID,
		PendingID:     m.target.PendingID,
	}
	event, body := m.event, m.textarea.Value()
	m.sending = true
	return m, func() tea.Msg {
		if err := src.SubmitReview(target, event, body); err != nil {
			return ErrorMsg{Err: err}
		}
		return SubmittedMsg{}
	}
}
```

`import` に `"github.com/kukv/octoscope/internal/usecase"` を足す。

- [ ] **Step 3: `review` のテストを直して走らせる**

`internal/tui/review/review_test.go` のフェイクを 1 メソッドにする。
`SubmitReview` / `SubmitNewReview` の 2 本を出し分けていたテストは、
**その分岐が `usecase` に移った以上ここでは検証しない**（Task 6 で検証済み）。
残すのは「popup が `SubmitReview` を呼ぶこと」「エラーが `ErrorMsg` になること」。

```bash
go test ./internal/tui/review/ -v
```

Expected: PASS

- [ ] **Step 4: `diff.Source` を載せ替える**

`internal/tui/diff/diff.go`:

```go
// Source is what the diff view needs. The order of calls a line comment takes
// is not here: PostLineComment is one operation, whatever GitHub needs behind
// it (.claude/rules/architecture.md).
type Source interface {
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
	PostLineComment(t usecase.ReviewTarget, c gh.PendingComment) (string, error)
	DiscardReview(reviewID string) error
	review.Source
}
```

`internal/tui/diff/comment.go` の `post()`:

```go
func (m Model) post() (Model, tea.Cmd) {
	body := m.textarea.Value()
	if body == "" {
		return m, nil
	}
	comment := m.target
	comment.Body = body

	src, ref := m.src, m.ref
	target := usecase.ReviewTarget{
		PullRequestID: m.review.PullRequestID,
		PendingID:     m.review.PendingID,
	}
	m.composing = false
	m.posting = true
	m.postErr = ""
	return m, func() tea.Msg {
		id, err := src.PostLineComment(target, comment)
		if err != nil {
			return commentErrorMsg{ref: ref, err: err}
		}
		return commentPostedMsg{ref: ref, reviewID: id}
	}
}
```

`post()` の doc コメントから「The review is started here rather than when the view
opens」の段落を削る（その事実は `usecase.PostLineComment` に移った）。残すのは
「捕まえた target に対して送る、カーソルの現在位置ではない」という理由。

- [ ] **Step 5: `diff` のテストを直して走らせる**

`internal/tui/diff/comment_test.go` の `StartReview` / `AddReviewThread` を持つフェイクを
`PostLineComment` の 1 本にする。**「pending が無ければ StartReview を先に呼ぶ」を
検証していたテストは削除する**（Task 6 に移った。二重に持たない）。

```bash
go test ./internal/tui/diff/ -v
git status --short -- 'internal/tui/diff/testdata/*.golden'
```

Expected: PASS、golden の変更なし

- [ ] **Step 6: `detail.Source` を載せ替える**

`internal/tui/detail/detail.go` の interface 群を置き換える。

```go
// itemSource is what the detail view does to the item it shows. A pull
// request and an issue take different GitHub calls for every one of these;
// choosing between them is internal/usecase's job, not this view's, which is
// why there is one method per operation rather than two.
type itemSource interface {
	GetItem(ctx context.Context, ref gh.ItemRef) (usecase.Item, error)
	AddComment(ref gh.ItemRef, body string) error
	SetState(ref gh.ItemRef, closing bool) error
	EditLabels(ref gh.ItemRef, add, remove []string) error
	EditAssignees(ref gh.ItemRef, add, remove []string) error
	OpenWeb(url string) error
}

// candidateSource lists what a picker offers. Labels and assignees belong to
// the repository, not to a PR or an issue.
type candidateSource interface {
	ListLabels(ctx context.Context, repo string) ([]gh.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

// reviewOpener is what v needs before the review popup can open: the pull
// request's node id and the unsubmitted review, if any.
type reviewOpener interface {
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
}

// Source is the whole of it. review.Source is embedded rather than repeated:
// the submission popup this view holds declares exactly what it needs.
type Source interface {
	itemSource
	candidateSource
	reviewOpener
	review.Source
}
```

`prMsg` / `issueMsg` を 1 つにする:

```go
	// itemMsg carries a fetch's answer along with the item it is about. The
	// detail view is rebuilt for each item the user opens, but the request
	// for the last one is still running: without the ref, its answer would
	// land here and show the wrong item for as long as the current fetch
	// takes.
	itemMsg struct {
		ref  gh.ItemRef
		item usecase.Item
	}
```

`fetch` / `postComment` / `setState` / `applyPicker` から種別の `if` / `switch` を消す:

```go
func fetch(src Source, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		item, err := src.GetItem(context.Background(), ref)
		if err != nil {
			return errMsg{ref, err}
		}
		return itemMsg{ref, item}
	}
}

func postComment(src Source, ref gh.ItemRef, body string) tea.Cmd {
	return func() tea.Msg {
		if err := src.AddComment(ref, body); err != nil {
			return commentErrorMsg{err}
		}
		return commentPostedMsg{}
	}
}

func setState(src Source, ref gh.ItemRef, closing bool) tea.Cmd {
	return func() tea.Msg {
		if err := src.SetState(ref, closing); err != nil {
			return stateErrorMsg{err}
		}
		return stateChangedMsg{}
	}
}

func applyPicker(src Source, ref gh.ItemRef, kind pickerKind, add, remove []string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if kind == pickLabels {
			err = src.EditLabels(ref, add, remove)
		} else {
			err = src.EditAssignees(ref, add, remove)
		}
		if err != nil {
			return pickErrorMsg{err}
		}
		return pickerAppliedMsg{}
	}
}
```

`Update` の `prMsg` / `issueMsg` の 2 case を 1 つに:

```go
	case itemMsg:
		if msg.ref != m.ref {
			return m, nil // an answer for an item the user has already left
		}
		it := msg.item
		m.loading = false
		m.state = it.State
		m.actionErr = ""
		m.labels = labelNames(it.Labels)
		m.assignees = authorLogins(it.Assignees)
		m.url = it.URL
		if it.Kind == gh.ItemPR {
			m.title = i18n.Tf("detail.pr_title", map[string]any{"Number": it.Number, "Title": it.Title})
			m.setContent(prMarkdown(*it.PR))
		} else {
			m.title = i18n.Tf("detail.issue_title", map[string]any{"Number": it.Number, "Title": it.Title})
			m.setContent(issueMarkdown(it))
		}
		return m, nil
```

> `prMarkdown(gh.PR)` はそのまま使える（`it.PR` が非 nil）。
> `issueMarkdown` は `gh.Issue` を取っているので、`usecase.Item` を取る形に変える。
> `internal/tui/detail/render.go` の `issueMarkdown` のシグネチャを
> `func issueMarkdown(it usecase.Item) string` にし、`issue.Body` → `it.Body`、
> `issue.Comments` → `it.Comments` のように読み替える。**出力する文字列は変えない。**

`openWeb` はそのまま（`src.OpenWeb(url)` は `itemSource` にある）。

- [ ] **Step 7: `detail` のテストを直して走らせる**

`internal/tui/detail/detail_test.go` / `picker_test.go` / `render_test.go` のフェイクを
新しい interface に合わせる。`GetPR` / `GetIssue` の 2 本 → `GetItem` 1 本、
`ClosePR` / `CloseIssue` → `SetState` など。

**「PR ref なら GetPR、Issue ref なら GetIssue」を検証していたテストは削除する**
（Task 5 に移った）。残すのは「`itemMsg` が来たら title と body が変わる」など、
このビューの状態遷移そのもの。

```bash
go test ./internal/tui/detail/ -v
git status --short -- 'internal/tui/detail/testdata/*.golden'
```

Expected: PASS、golden の変更なし。**golden が変わったら描画を変えてしまっている。
golden を録り直さず、コードを戻す。**

- [ ] **Step 8: `app` と `cmd` を配線し直す**

`internal/tui/app/app.go` の `Source` はそのまま（4 つのビューの合成）。
型が変わったので、`app_test.go` のフェイクを直す。

`cmd/octoscope/main.go`:

```go
	client := cli.New(dir, *repoFlag)
	uc := usecase.New(client)
	p := tea.NewProgram(app.New(uc, app.Options{HasRepo: *repoFlag != ""}))
```

import に `"github.com/kukv/octoscope/internal/usecase"` を足す。
`"github.com/kukv/octoscope/internal/gh/cli"` は残る（`cli.New` を呼ぶ唯一の場所）。

- [ ] **Step 9: 全部通ることを確認**

```bash
make check
git status --short -- '*.golden'
```

Expected: `make check` が PASS、golden の変更が 1 件も無い

- [ ] **Step 10: 実機で確認**

`.claude/rules/tui.md`「TUI の変更は、テストが通っただけで完了にしない」。

```bash
go run ./cmd/octoscope
go run ./cmd/octoscope --lang ja
go run ./cmd/octoscope --repo kukv/octoscope
```

以下を 1 つずつ押して、動くことを目で見る。

- Work タブ: `j` / `k` / `enter` → 詳細
- 詳細: `o`（ブラウザ）/ `c`（コメント）/ `x` → `y`（close/reopen）/ `l`（ラベル）/ `a`（アサイニー）/ `v`（レビュー提出）/ `d`（diff）
- diff: `j` / `k` / `c`（行コメント）→ `ctrl+s` / `v` / `X`（破棄）
- Repos タブ: `tab` → `j` → `enter` → 詳細 → `o`

**80 桁でも崩れないことと、`--lang ja` で桁がずれないことを見る。**

- [ ] **Step 11: 空振りの確認**

`usecase.GetItem` の `Item.URL` の代入を一時的に消して `go test ./internal/tui/detail/` を
走らせる。`o` に関するテストが落ちなければ、そのテストは何も守っていない。
落ちるテストが無ければ 1 本足す:

```go
// o opens the item's own address. Without a URL there is nothing to open and
// the key silently does nothing -- the shape of the bug this remediation
// started from.
func TestDetailKeepsTheItemsURLForOpeningInABrowser(t *testing.T) {
	...itemMsg を Update に渡し、o を押して webOpener が呼ばれることを確認...
}
```

確認したら戻す。

- [ ] **Step 12: コミット**

```bash
make check
git add internal/tui cmd/octoscope
git commit -m "refactor: put the views on internal/usecase and drop the PR/Issue split"
```

---

## Task 8: 規約 10 項目と depguard（作業順 3）— **完了済み。実施しない**

> **2026-09-07 に実装より先に済ませた**（D4 参照）。このタスクの内容は
> `.claude/rules/*.md`、`.golangci.yml`、設計書に反映済みで、depguard の 5 ルール
> （既存 2 + 新規 3）が実際に落とすことをプローブで確認してある。
>
> **以下は実施済みの記録として残す。もう一度やらない。**

**Files:**
- Modify: `.golangci.yml`
- Modify: `.claude/rules/architecture.md`
- Modify: `.claude/rules/tui.md`
- Modify: `.claude/rules/go-style.md`
- Modify: `.claude/rules/testing.md`
- Modify: `docs/superpowers/specs/2026-09-06-phase2-remediation-design.md`（D1 / D5 の訂正）

**Interfaces:**
- Consumes: なし
- Produces: なし

設計書 §5 の 10 項目。**D4 のとおり、コードを直したあとに書く。**

- [ ] **Step 1: depguard に `usecase` の出入りを足す（設計書 §5 項目 10）**

`.golangci.yml` の `depguard.rules` に足す。既存の `gh-layer` / `i18n-layer` /
`browser-layer` の deny にも `usecase` を足す（`architecture.md` は「leaf は
我々のものを何も import しない」と言っている）。

```yaml
        # The views talk to internal/usecase, not to the backend behind it.
        # cmd/octoscope is the one place that knows which backend there is.
        tui-layer:
          files:
            - "**/internal/tui/**"
          deny:
            - pkg: github.com/kukv/octoscope/internal/gh/cli
              desc: views go through internal/usecase; only cmd/octoscope names a backend
        # usecase sits below the UI: it decides which GitHub calls an
        # operation takes, and knows nothing of screens or messages.
        usecase-layer:
          files:
            - "**/internal/usecase/**"
          deny:
            - pkg: github.com/kukv/octoscope/internal/tui
              desc: internal/usecase must not depend on the UI
            - pkg: github.com/kukv/octoscope/internal/i18n
              desc: internal/usecase must not translate; the UI does
```

`gh-layer` の deny に足す:

```yaml
            - pkg: github.com/kukv/octoscope/internal/usecase
              desc: the GitHub layer must not depend on the layer above it
```

`i18n-layer` と `browser-layer` の deny にも同じ 1 項目を足す
（desc は `internal/i18n must not depend on other internal packages` /
`internal/browser must not depend on other internal packages`）。

- [ ] **Step 2: depguard が実際に落とすことを確認**

`internal/usecase/usecase.go` に一時的に
`import _ "github.com/kukv/octoscope/internal/i18n"` を足す。

**`internal/tui/detail` を使わないこと。** Task 7 のあと `detail` は `usecase` を
import するので、逆向きの import は **import cycle** になり、depguard が走る前に
コンパイラが落とす。それでは depguard を検証したことにならない。
`i18n` なら cycle にならず、`usecase-layer` の 2 つ目の deny が実際に効く。

```bash
make lint
```

Expected: FAIL — `internal/usecase must not translate; the UI does`。
確認したら import を消して再実行し PASS

- [ ] **Step 3: `architecture.md` の依存図を書き直す（項目 1）**

```
cmd/octoscope                          （合成ルート: cli.New → usecase.New → app.New）
     ↓
internal/tui  ──→  internal/usecase  ──→  internal/gh/cli  ──→ internal/browser
     │                    │                     │
     └────────────────────┴─────────────────────┴──→  internal/gh   （ドメイン型）
     ↓
internal/i18n                                              （誰にも依存しない）
```

**下の層は上の層を知らない。** `internal/tui` は `internal/gh/cli` を import しない。
どのバックエンドが動いているかを知っているのは `cmd/octoscope` だけである。
`internal/usecase` は `internal/tui` と `internal/i18n` を import しない。
`internal/i18n` と `internal/browser` は他の internal パッケージを import しない。

- [ ] **Step 4: 「層を足す前に」を書き直す（項目 2）**

`architecture.md` の当該節を丸ごと次に置き換える。

```markdown
## 層を足す前に

このプロジェクトは TUI の単一バイナリであり、Web サービスではない。
DI コンテナ、ドメインモデルとインフラモデルの二重定義、Input/Output DTO は
**入れていない**。

`internal/usecase` は 2026-09-07 に入れた。それまで「Web サービス向けの構造だから
入れない」という一般論で退けていたが、その判断は `internal/tui` が 1 行も存在しない
時点（Phase 0、`67ba0de`）に書かれ、以後一度も再検証されていなかった。
再検証の実測は次のとおりである。

- API 呼び出し順序が `tea.Cmd` のクロージャに漏れていた（2 箇所）
- 種別の振り分けが View に漏れ、`detail.Source` が 19 メソッドに膨らんでいた
- そのどちらも Bubble Tea を起動しないとテストできなかった

**規約は書いた時点のコードに対する判断である。コードが育ったら再検証が要る。**

層を足したくなったときは、次の 2 つを書けるか確かめる。

1. **この層が無いと何が壊れるか。** 実際に起きた、または確実に起きる問題を、
   一般論ではなく**このコードの実測値**で挙げる
2. **足したあと何が減るか。** 触る場所、重複、テストの手間のどれが減るか

両方書けるなら提案する価値がある。書けないなら足さない。
上の `internal/usecase` の記述が、書けたときの見本である。
```

- [ ] **Step 5: interface の上限を書く（項目 3、D1 の数え方で）**

`architecture.md` の「interface は小さく保つ」に足す。

```markdown
**1 つの interface 宣言に直接並べるメソッドは 6 個まで。**
embed した interface のメソッドは数に含めない — embed 先が同じ上限を独立に負う。

判定は簡単で、テスト用のフェイクを書いたときに、そのテストが呼ばないメソッドの
スタブをいくつ書かされたかを見る。6 を超えたら、その interface は
「1 つの画面が使う分」より大きい。

2026-09-07 時点で、この上限を超えている宣言は 1 つも無い。
`work` 1 / `repo` 各 1 / `diff` 5 / `detail` 6 / `review` 1 /
`usecase` の各 source 2〜6。**例外を作らずに済んでいる状態を保つ。**
```

- [ ] **Step 5.5: `Item` を太らせない規則を書く（設計書に無い追加項目）**

`architecture.md` の「複数の API 呼び出し」の節の直後に足す。
設計書 §5 の 10 項目には無い、この計画で足す 11 項目目である。
理由は Task 5 の `Item` の節に書いたとおりで、**UI だけの修正が下の層に
波及しないことを保つための唯一の歯止め**だからである。

```markdown
## `usecase.Item` を画面の写しにしない

`usecase.Item` は PR と Issue の合流点であって DTO ではない。
ここは UI の都合が下の層に漏れる唯一の穴なので、太らせない。

- **`Item` に共通フィールドを足してよいのは、PR と Issue の両方に GitHub 側の
  対応物があるときだけ。**
- PR にしか無いものは `Item.PR`（`*gh.PR`）から読む。`Item` に写さない。
- 「画面に出したいものが `Item` に無い」と思ったら、まず `internal/gh` の
  ドメイン型に無いのではないかを疑う。2026-09-07 時点で `gh.Issue` の
  公開フィールドは 10 個すべて `Item` に入っている。

この規則があるかぎり、UI だけの修正（色・桁・文言・キー・状態遷移・
何を描くかの選び方）は `internal/usecase` に波及しない。波及するのは
GitHub への**新しい操作**を足すときだけで、それは元から UI だけの修正ではない。
```

- [ ] **Step 6: 呼び出し順序の置き場所を書く（項目 4）**

`architecture.md` に節を足す。

```markdown
## 複数の API 呼び出しは `internal/usecase` に置く

**`tea.Cmd` のクロージャの中に、2 つ以上の API 呼び出しを並べない。**
「pending review が無ければ先に作る」は GitHub のレビュー API の仕様であって
TUI の都合ではない。ビューが知るべきなのは「行コメントを送る」という 1 操作だけで、
それが何回のリクエストになるかではない。

置き場所を分けると、順序のテストに Bubble Tea が要る。
`internal/usecase` に置けば、フェイクを 1 つ渡すだけで順序を検証できる。
```

- [ ] **Step 7: `tui.md` に mode の規約を足す（項目 5）**

```markdown
## UI の状態は enum

**並行する bool でモードを表現しない。** `composing` / `confirming` / `picking` /
`submitting` を並べると、名目上の状態数が 2^n になり、そのうち正しいのは数個しかない。
`Update` と `View` と `handleKey` がそれぞれ違う組み合わせを暗黙に前提にしてしまう。

今どのオーバーレイが出ているか（mode）と、その mode での通信状態（phase）の
2 つの enum に畳む。

```go
type mode uint8
const (
    modeView mode = iota
    modeCompose
    modeConfirm
    modePick
    modeSubmit
)

type phase uint8
const (
    phaseIdle phase = iota
    phaseLoading
    phaseWorking
)
```

エラー文字列も mode ごとに分けて持たない。どこに描くかは mode が決める。
```

- [ ] **Step 8: `go-style.md` にコメントの規約を足す（項目 6）**

「コメント」の節の「書くのは次の場合だけ」の下に足す。

```markdown
**実装計画や設計書への参照をコードに書かない。** `Task 10`、`spec 4.4.1` のような
参照は、コードを読む人には辿れず（`docs/` の中身は文脈ごと変わる）、
実装計画が完了したあとは意味を失う。書くべきなのは「なぜそうなっているか」であって
「どの計画の何番か」ではない。

`.claude/rules/*.md` への参照は別で、これは今も有効な規約を指しているので残してよい。
```

- [ ] **Step 9: `testing.md` に 3 項目を足す（項目 7 / 8 / 9）**

「外部に触らない」の下に足す。

```markdown
## 外部レスポンスは実物を録る

**パーステストの入力は手書きの JSON ではなく、実際に録ったレスポンスを使う。**
手書きだと「GitHub が実際にはそう返さない形」でも通ってしまい、
返し方が変わったときに気づけない。

録り方と、録った日・対象リポジトリを testdata の `README.md` に残す。
秘密情報は含めない（自分の公開リポジトリを使う）。

**テストを通すために録ったファイルを編集しない。** 落ちたら、まず実装を疑う。

## 外部 API に渡す引数は「仕様」を検証する

**実装が組み立てた引数をコピーした期待値を書かない。**

```go
// これは実装の鏡でしかなく、実装が間違っていても落ちない
wantArgs := []string{"pr", "list", "--json", prListFields}
```

「なぜその引数が要るのか」がテスト名から分かる形にする。

```go
// gh pr list defaults to 30 items. The Repos tab must not silently truncate
// a repository with more open pull requests than that.
func TestPRListAsksForMoreThanTheDefaultThirty(t *testing.T) {
    // args に --limit が含まれ、値が 30 より大きいことを確認する
}
```

## 画面をまたぐ操作はキー入力だけで通す

複数のビューにまたがる操作は、`internal/tui/app` にキー入力だけのシナリオテストを
置いて担保する。tty は要らない。`Update` に `tea.KeyPressMsg` を順に渡し、
`View()` の出力を見る。
```

- [ ] **Step 10: 設計書を訂正する（D1 / D5）**

`docs/superpowers/specs/2026-09-06-phase2-remediation-design.md` に手を入れる。

1. §3.2 の `cli/review.go:141` の行を消し、代わりに次を書く。

```markdown
`cli/review.go:141` の `pr.Reviews.Nodes[0]` は**潜在バグではなかった**。
GitHub は 1 ユーザー 1 PR につき unsubmitted review を 1 つしか持てない
（GraphQL / REST 共通、2026-09-07 に確認）。`first: 1` は打ち切りではない。
```

2. §8 の完了条件 3 を書き換える。

```markdown
3. `detail` が宣言する interface が、1 つあたり 6 メソッド以下
   （embed した interface のメソッドは数えない）
```

- [ ] **Step 11: 全部通ることを確認**

```bash
make check
git status --short -- '*.golden'
```

Expected: PASS、golden の変更なし

- [ ] **Step 12: コミット**

```bash
git add .golangci.yml .claude/rules docs/superpowers/specs
git commit -m "docs: record the usecase decision in the rules and guard it with depguard"
```

---

## Task 8.5: rules の暫定注記（`TRANSIENT`）を消す

**Files:**
- Modify: `.claude/rules/architecture.md`
- Modify: `.golangci.yml`

**Interfaces:** なし

規約を実装より先に入れたため、`.claude/rules/` と `.golangci.yml` に
「今これを破っている / いつ直す」という**暫定の注記**が残っている
（2026-09-07 に数えて 9 箇所。以後 rules を触れば増減するので、**件数は
数えてから使う** — Step 1）。
コードが追いついた時点で消さないと、規約が現状と食い違ったまま残り、
次のセッションがどちらを信じればいいか分からなくなる。

見つけ方は機械的にしてある。

```bash
grep -rn 'TRANSIENT' .claude/rules .golangci.yml
```

各マーカーは `TRANSIENT(Part1 Task N)` / `TRANSIENT(Part2 作業順 N)` の形で、
**どの作業が完了したら消せるか**を持っている。

**Part 1 で消すもの（7 箇所 = `Task 5` が 4、`Task 7` が 3）**

| マーカー | 場所 | 消す条件 | 消し方 |
|---|---|---|---|
| `Part1 Task 5` | `architecture.md`「`usecase.Item` を画面の写しにしない」 | `usecase.Item` がある | 注記のブロックごと削除 |
| `Part1 Task 5` | `architecture.md` 同節の箇条書き | 同上 | 「その全部を持つ**ように作る**」→「その全部を**持っている**」に直し、マーカーを削除 |
| `Part1 Task 5` | `architecture.md`「層を足す前に」 | 同上 | 「（パッケージ自体はまだ無い）」だけ削除。**2026-09-07 の日付は判断した日の記録なので残す** |
| `Part1 Task 5` | `.golangci.yml` `usecase-layer` | 同上 | `TRANSIENT` 行と続く 3 行のコメントを削除。ルール本体は残す |
| `Part1 Task 7` | `architecture.md`「依存の向き」 | `internal/usecase` があり TUI が載っている | 注記のブロックごと削除 |
| `Part1 Task 7` | `architecture.md`「interface は小さく保つ」 | `detail` の全宣言が 6 メソッド以下 | 注記のブロックごと削除 |
| `Part1 Task 7` | `architecture.md`「複数の API 呼び出しは〜」 | 順序と振り分けが `usecase` にある | 注記のブロックごと削除 |

**Part 2 に持ち越すもの（2 箇所）— このタスクでは消さない**

| マーカー | 場所 | 消す条件 |
|---|---|---|
| `Part2 作業順 6` | `tui.md`「UI の状態は enum」 | `detail` / `diff` の bool が enum に畳まれている |
| `Part2 作業順 8` | `go-style.md`「コメント」 | 非テストコードの `spec N` / `Task N` 参照が 0 箇所 |

- [ ] **Step 1: 残っているマーカーを数える**

**この計画に書いた件数を信じない。数えてから始める。**
下は 2026-09-07 に数えた値で、その後 rules を触れば変わる。

```bash
grep -rho 'TRANSIENT([^)]*)' .claude/rules .golangci.yml | sort | uniq -c
```

Expected（2026-09-07 時点）:

```
      4 TRANSIENT(Part1 Task 5)
      3 TRANSIENT(Part1 Task 7)
      1 TRANSIENT(Part2 作業順 6)
      1 TRANSIENT(Part2 作業順 8)
```

数が違ったら、上の表と突き合わせて**表のほうを直してから**先に進む。

- [ ] **Step 2: Task 5 の 4 箇所を消す**

上の表の `Part1 Task 5` の行を、「消し方」のとおりに処理する。
**注記だけを消し、規約本文には触れない。**

- [ ] **Step 3: Task 7 の 3 箇所を消す**

`Part1 Task 7` の 3 行。いずれも `<!-- TRANSIENT ... -->` の行と、
続く `>` で始まるブロック全体を削除する。

- [ ] **Step 4: 消した注記が嘘になっていないか、実際に確かめる**

注記を消すということは「もう破っていない」と言うことなので、根拠を見る。

```bash
# detail の宣言が 6 メソッド以下か
sed -n '/^type .*interface {/,/^}/p' internal/tui/detail/detail.go

# tea.Cmd の中に 2 つ以上の API 呼び出しが残っていないか
grep -n 'StartReview\|SubmitNewReview' internal/tui/ -r

# 種別の振り分けが View に残っていないか
grep -rn 'ItemPR' internal/tui/ | grep -v '_test.go'
```

`ItemPR` は「PR のときだけ `d` を出す」のような**表示の判断**には残ってよい。
残ってはいけないのは「PR なら `GetPR`、Issue なら `GetIssue`」という
**呼び分け**である。区別がつかない行が出てきたら止めて相談する。

- [ ] **Step 5: `Part1` のマーカーが 1 つも残っていないことを確認**

```bash
grep -rn 'TRANSIENT(Part1' .claude/rules .golangci.yml
```

Expected: 出力なし（`Part2` のものだけが残る）

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add .claude/rules .golangci.yml
git commit -m "docs: drop the rules' notes about what the code had not caught up to yet"
```

---

## Task 9: チェックポイント — 実機で全機能を確認して承認を取る

**Files:** なし（確認のみ）

`.claude/rules/tui.md` と設計書 §8 条件 1。**ここで一度止まり、承認を得てから Part 2 に入る。**

- [ ] **Step 1: `make check` と `make release-check`**

```bash
make check
make release-check
```

Expected: 両方 PASS

- [ ] **Step 2: 完了条件のうち Part 1 の分を機械的に確認**

```bash
# 条件 2: make check
make check

# 条件 3: detail の interface が 1 宣言あたり 6 メソッド以下
sed -n '/^type .*interface {/,/^}/p' internal/tui/detail/detail.go

# 条件 6: cli のパーステストが実物の testdata を使っている
grep -c 'readTestdata' internal/gh/cli/*_test.go

# 条件 8: depguard が usecase に対応している
grep -n 'usecase' .golangci.yml
```

- [ ] **Step 3: 実機で全機能を 1 つずつ**

Task 7 Step 10 の一覧を、`--lang en` / `--lang ja` の両方、80 桁と 160 桁で走らせる。
**動かないものが 1 つでもあれば Part 2 に進まない。**

未解決として残っているものは、動かなくてもここでは止めない（設計書 §3.4）。

- 詳細画面の `c` でコメント入力欄が開かない（再現せず。原因未特定）
- `--repo owner/name` を渡しても Repos タブでなく Work タブが出る（設計書に無い）

この 2 つは Part 2 の課題として持ち越す。**Task 9 の報告に明記する。**

- [ ] **Step 4: 承認を取る**

Part 1 の diff を要約して報告し、Part 2（作業順 6〜9）に進んでよいか確認する。
**承認なしに Part 2 の計画を書き始めない。**

Part 2 の計画には次を必ず入れる。**書き忘れると規約が現状と食い違ったまま残る。**

```bash
grep -rn 'TRANSIENT' .claude/rules .golangci.yml
```

- `tui.md`「UI の状態は enum」の注記 — 作業順 6（bool → enum）の完了時に消す
- `go-style.md`「コメント」の注記 — 作業順 8（spec 参照の削除）の完了時に消す

Part 2 の完了条件に「`grep -rn 'TRANSIENT' .claude/rules .golangci.yml` が 0 件」を入れる。

---

## Self-Review

**設計書 §5 の 10 項目の対応:**

| # | 内容 | Task |
|---|---|---|
| 1 | 依存図に `usecase` | Task 8 Step 3 |
| 2 | 「層を足す前に」書き直し | Task 8 Step 4 |
| 3 | interface 6 メソッド上限 | Task 8 Step 5（D1 で数え方を確定） |
| 4 | 呼び出し順序は usecase | Task 8 Step 6 |
| 5 | `tui.md` mode は enum | Task 8 Step 7 |
| 6 | `go-style.md` spec 参照禁止 | Task 8 Step 8 |
| 7 | `testing.md` 実物 testdata | Task 8 Step 9 |
| 8 | `testing.md` 引数は仕様 | Task 8 Step 9 |
| 9 | `testing.md` シナリオテスト | Task 8 Step 9（規約のみ。**実装は Part 2 の作業順 9**） |
| 10 | depguard | Task 8 Step 1 |
| **11** | **（設計書に無い追加）`usecase.Item` を画面の写しにしない** | **Task 8 Step 5.5** |
| **12** | **（設計書に無い追加）規約に残した暫定注記を、コードが追いついたら消す** | **Task 8.5 / Part 2** |

11 は設計書 §5 に無い。UI だけの修正が `internal/usecase` に波及しない状態を
保つための歯止めで、`Item` が唯一その経路になり得るため足した。

**設計書 §6 の作業順の対応:**

| # | Task |
|---|---|
| 2（ページング） | Task 1 / 2 / 3 |
| 3（規約） | Task 8 |
| 4（usecase） | Task 5 / 6 / 7 |
| 5（testdata） | Task 4 |
| 6〜9 | **Part 2** |

**設計書 §8 の完了条件:**

| # | Part 1 で満たすか |
|---|---|
| 1 実機で全機能 | Task 9（Part 1 の範囲で） |
| 2 `make check` | 全 Task |
| 3 `detail.Source` 6 以下 | Task 7（D1 の読み替えで） |
| 4 bool の mode が無い | **Part 2**（作業順 6） |
| 5 spec 参照 0 箇所 | **Part 2**（作業順 8。規約は Task 8 で先に書く） |
| 6 実物 testdata | Task 4 |
| 7 シナリオテスト 3 本 | **Part 2**（作業順 9） |
| 8 rules 10 項目 + depguard | Task 8（**完了済み**） |
| **9（追加）`TRANSIENT` の注記が 0 件** | `Part1` のものは Task 8.5、`Part2` のものは **Part 2** |

**未確定として実装者に判断を委ねた箇所: 無い。** TBD / 後で決める、は 1 つも残していない。
