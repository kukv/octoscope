# 取得の失敗から立ち直る 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Work 板の 502 を減らし、出たときも板を残したまま `r` で引き直せるようにする。

**Architecture:** 3 つを順に積む。まず `internal/gh` が一時的な失敗を種別として返せるようにし（画面は変わらない）、次に読み取りだけ 1 回再試行し、最後に Work 板を列ごとの 4 リクエストに分けて、失敗をタブの 1 行に落とす。

**Tech Stack:** Go 1.25、charm.land/bubbletea/v2、`internal/golden`、`internal/i18n`

**Spec:** `docs/superpowers/specs/2026-09-11-fetch-failures-design.md`。画面は `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.1、エラーの扱いは `.claude/rules/errors.md` が正。

**このブランチは PR #71（Repos サイドバー）の先頭から切ってある。** #71 が先にマージされる前提で、`internal/tui/repo` の現在の形（`notice` を足す先、`selectRow`、世代カウンタ）を土台にする。

## 設計からのスコープの縮小（承認が要る）

設計 §4 は「`ErrGhNotFound` / `ErrUnauthenticated` 以外はすべてそのタブの 1 行」と
書いている。**この計画では `notice` を Work タブと Repos タブの 2 つに限り、
オーバーレイ（詳細・diff・checks）は今の全画面エラーのままにする。**

理由: オーバーレイの失敗は既に `esc` で下の画面に戻れる（`app.failOverlay` と
`handleKey` のエラー分岐）。利用者が報告した「終了しか選べない」はタブの失敗のときだけで、
オーバーレイには当てはまらない。3 つのビューを変えるぶんのリスクに対して得るものが薄い。

**オーバーレイも 1 行にすべきなら、この計画を承認する前に言うこと。** タスクが 2 本増える。

## Global Constraints

- **依存の向き**（`.claude/rules/architecture.md`）: `internal/tui` は `internal/gh/cli` も
  `internal/config` も import しない。**interface は利用側で定義する**
  （1 宣言に直接並べるメソッドは 6 個まで。embed は数に含めない）
- **GitHub API 固有の文字列を層の外に出さない。** `internal/gh` のドメイン型に直して返す
- **`internal/gh/cli` に i18n を入れない。** 案内文への差し替えは TUI 側で `errors.Is` して行う
- **分岐に使わないエラーをセンチネルにしない**（`.claude/rules/errors.md`）
- 取得は `tea.Cmd` の中。**`View` は副作用を持たない・時計を読まない**（`.claude/rules/tui.md`）
- ネットワークを待つ処理には `context.Context` を通す
- 色は `internal/tui/theme` から。文字列は `internal/i18n` から。
  **新しい文言は `active.en.yaml` と `active.ja.yaml` の両方に足す**
- **表示幅は `github.com/charmbracelet/x/ansi` で数える。** 日本語は 1 文字 2 桁
- **テスト**（`.claude/rules/testing.md`）: ネットワークもサブプロセスも叩かない。
  **状態は `Update` にメッセージとキーを渡して到達させる。フィールドを直接組み立てない。**
  実装が組み立てた引数をコピーした期待値を書かない。
  **書いたテストは検証対象を一時的に壊して落ちることを確かめてから進む**
- golden は en / ja × 80 / 120 / 160。ANSI エスケープを落とさない
- **コメント**（`.claude/rules/go-style.md`）: 基本は書かない。書くのは外部の事情・
  一見おかしいコードが正しい理由・エクスポートした識別子の doc の 3 つだけ。
  **コード中に "Task N" や実装計画への参照を書かない。** コメントは英語。
  **嘘のコメントを残さない**
- このリポジトリの golangci-lint は**未使用のフィールド・関数・定数を必ず赤にする**。
  読む側と同じコミットで生まれる必要がある
- **`make check` が緑でないコミットを作らない**

## File Structure

| ファイル | 責務 |
|---|---|
| `internal/gh/gh.go`（変更） | `ErrTransient` / `ErrUnauthenticated` |
| `internal/gh/cli/cli.go`（変更） | `runGh` が種別を判定。`c.read`（1 回再試行）。読み取りの呼び出しを `c.read` に移す |
| `internal/gh/cli/cli_test.go`（変更） | 分類と再試行。**書き込みが再試行されないこと** |
| `internal/gh/cli/work.graphql`（変更） | 4 つの alias を 1 つのパラメータ付き search に |
| `internal/gh/cli/graphql.go`（変更） | `ListWorkSection`。`ListWork` を消す。検索式を Go 側へ |
| `internal/gh/cli/testdata/`（新規） | 502 の実物（nginx の HTML） |
| `internal/usecase/usecase.go`（変更） | `ListWorkSection` を通す |
| `internal/tui/work/work.go`（変更） | 列ごとの取得・`loading`・`notice`、`Summary` の意味 |
| `internal/tui/work/render.go`（変更） | 列の loading と通知の行 |
| `internal/tui/repo/repo.go`（変更） | `notice`。`errMsg` の行き先を分ける |
| `internal/tui/repo/render.go`（変更） | 通知の行 |
| `internal/tui/layout/layout.go`（変更） | `Notice(text, width)` |
| `internal/tui/app/app.go`（変更） | 全画面に行くのは環境の不備だけ。`ErrorMsg` の改名 |
| `internal/i18n/locales/active.{en,ja}.yaml`（変更） | 通知の文言、認証切れの案内 |
| `docs/superpowers/specs/2026-09-11-fetch-failures-design.md`（変更） | §6 に即時再試行の実測 |
| `docs/superpowers/2026-09-11-fetch-failures-followups.md`（新規） | 積み残しと実端末での確認 |

---

### Task 1: 一時的な失敗と認証切れを種別として返す

**このタスクで画面は変わらない。** `internal/gh` が種別を返せるようになるだけで、
TUI はまだ分岐しない。

**Files:**
- Modify: `internal/gh/gh.go`, `internal/gh/cli/cli.go`
- Modify: `internal/gh/cli/cli_test.go`

**Interfaces:**
- Produces: `gh.ErrTransient`, `gh.ErrUnauthenticated`。`runGh` がこれらで包んだ error を返す
- Consumes: 既存の `gh.ErrGhNotFound`

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/cli/cli_test.go`:

```go
// gh reports GitHub's front end giving up as an HTTP status on stderr; the
// body is nginx's HTML, not a GraphQL error. Callers retry this and nothing
// else, so it has to be told apart from a query GitHub actually answered.
func TestFrontEndFailuresAreTransient(t *testing.T) {
	for _, status := range []string{"502", "503", "504"} {
		t.Run(status, func(t *testing.T) {
			err := classify(fmt.Errorf("gh api: gh: HTTP %s", status))
			if !errors.Is(err, gh.ErrTransient) {
				t.Errorf("HTTP %s did not classify as transient: %v", status, err)
			}
		})
	}
}

func TestAnsweredFailuresAreNotTransient(t *testing.T) {
	for _, msg := range []string{
		"gh api: gh: Not Found (HTTP 404)",
		"gh api: gh: API rate limit exceeded",
		"gh pr list: no pull requests match",
	} {
		if err := classify(errors.New(msg)); errors.Is(err, gh.ErrTransient) {
			t.Errorf("%q classified as transient", msg)
		}
	}
}

func TestMissingCredentialsAreTold(t *testing.T) {
	err := classify(errors.New("gh api: gh auth login required"))
	if !errors.Is(err, gh.ErrUnauthenticated) {
		t.Errorf("did not classify as unauthenticated: %v", err)
	}
}

// The original text is what GitHub said, and the UI shows it as it was said
// (.claude/rules/errors.md). Classifying must not replace it.
func TestClassifyKeepsTheOriginalText(t *testing.T) {
	err := classify(errors.New("gh api: gh: HTTP 502"))
	if !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("the original text was lost: %v", err)
	}
}
```

**`gh auth login required` の実際の文言は推測である。** 実装の前に
`gh api graphql -f query='{viewer{login}}'` を `GH_TOKEN=invalid` で走らせ、
実際の stderr を見てからこのテストの文字列を直すこと。合わなければ
`ErrUnauthenticated` の判定を落として `ErrTransient` だけを入れ、その旨を報告に書く
（推測でパターンを書くよりよい）。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'Transient|Answered|Credentials|OriginalText' -v`
Expected: FAIL（`undefined: classify`、`undefined: gh.ErrTransient`）

- [ ] **Step 3: センチネルを足す**

`internal/gh/gh.go`（`ErrGhNotFound` の隣）:

```go
// ErrTransient wraps a failure GitHub's front end produced rather than
// answered -- 502, 503, 504. The request was well-formed, so asking again
// is the right response.
var ErrTransient = errors.New("GitHub did not answer")

// ErrUnauthenticated is returned when gh has no usable credentials.
var ErrUnauthenticated = errors.New("not authenticated; run: gh auth login")
```

- [ ] **Step 4: `runGh` が種別を付ける**

`internal/gh/cli/cli.go`。`classify` を分けて書き、`runGh` が包んだ error に通す:

```go
// classify names the failures a caller acts on differently: one worth
// asking again for, and one only the user can fix. Everything else keeps
// the text gh printed and no type at all.
func classify(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, status := range []string{"HTTP 502", "HTTP 503", "HTTP 504"} {
		if strings.Contains(msg, status) {
			return fmt.Errorf("%w: %s", gh.ErrTransient, msg)
		}
	}
	// (unauthenticated: the pattern comes from the real stderr, see Step 1)
	return err
}
```

`runGh` の 2 つの `return` を `classify(...)` に通す。**stdout は今までどおり返す**
（`runFunc` の doc が理由を書いている）。

- [ ] **Step 5: 通ることを確かめ、壊して落ちることも確かめる**

Run: `go test ./internal/gh/cli/ -v`
`classify` の `HTTP 502` を `HTTP 999` に変えて `TestFrontEndFailuresAreTransient` が
落ちること、`%w` を `%v` に変えて同じテストが落ちることを見てから戻す。

- [ ] **Step 6: `make check` してコミット**

```bash
git add internal/gh
git commit -m "feat: tell a failure GitHub did not answer from one it did

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: 読み取りだけ 1 回再試行する

**Files:**
- Modify: `internal/gh/cli/cli.go`（`read` と読み取りの呼び出し）
- Modify: `internal/gh/cli/checks.go`, `diff.go`, `graphql.go`, `merge.go`,
  `repo_counts.go`, `review.go`（読み取りの呼び出し）
- Modify: `internal/gh/cli/cli_test.go`

**Interfaces:**
- Produces: `func (c *Client) read(ctx context.Context, dir string, args ...string) ([]byte, error)`
- Consumes: `gh.ErrTransient`（Task 1）

- [ ] **Step 1: 失敗するテストを書く**

```go
// A read is safe to ask for twice. A transient failure is exactly the case
// where asking again is likely to work, so the caller never sees the first one.
func TestAReadIsAskedAgainAfterATransientFailure(t *testing.T) {
	c := New("", "kukv/demo")
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
		}
		return []byte("[]"), nil
	}
	if _, err := c.ListPRs(context.Background(), ""); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if calls != 2 {
		t.Errorf("gh ran %d times, want 2", calls)
	}
}

func TestAReadIsAskedAgainOnlyOnce(t *testing.T) {
	c := New("", "kukv/demo")
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
	}
	if _, err := c.ListPRs(context.Background(), ""); err == nil {
		t.Fatal("ListPRs succeeded on a failing gh")
	}
	if calls != 2 {
		t.Errorf("gh ran %d times, want 2", calls)
	}
}

// A failure GitHub answered will answer the same way again.
func TestAnAnsweredFailureIsNotAskedAgain(t *testing.T) {
	c := New("", "kukv/demo")
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		return nil, errors.New("gh pr list: gh: Not Found (HTTP 404)")
	}
	_, _ = c.ListPRs(context.Background(), "")
	if calls != 1 {
		t.Errorf("gh ran %d times, want 1", calls)
	}
}

// A cancelled fetch must not be asked again: the user left, refreshed, or quit.
func TestACancelledReadIsNotAskedAgain(t *testing.T) {
	c := New("", "kukv/demo")
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		cancel()
		return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
	}
	_, _ = c.ListPRs(ctx, "")
	if calls != 1 {
		t.Errorf("gh ran %d times, want 1", calls)
	}
}

// 502 means "no answer came back", not "it never arrived": GitHub may have
// applied the change. Asking again could apply it twice.
func TestWritesAreNeverAskedAgain(t *testing.T) {
	writes := map[string]func(*Client) error{
		"AddPRComment":     func(c *Client) error { return c.AddPRComment("kukv/demo", 1, "hi") },
		"ClosePR":          func(c *Client) error { return c.ClosePR("kukv/demo", 1) },
		"ReopenPR":         func(c *Client) error { return c.ReopenPR("kukv/demo", 1) },
		"CloseIssue":       func(c *Client) error { return c.CloseIssue("kukv/demo", 1) },
		"ReopenIssue":      func(c *Client) error { return c.ReopenIssue("kukv/demo", 1) },
		"EditPRLabels":     func(c *Client) error { return c.EditPRLabels("kukv/demo", 1, []string{"bug"}, nil) },
		"EditPRAssignees":  func(c *Client) error { return c.EditPRAssignees("kukv/demo", 1, []string{"kukv"}, nil) },
		"MergePR":          func(c *Client) error { return c.MergePR("id", gh.MergeSquash) },
		"EnableAutoMerge":  func(c *Client) error { return c.EnableAutoMerge("id", gh.MergeSquash) },
		"DisableAutoMerge": func(c *Client) error { return c.DisableAutoMerge("id") },
		"AddReviewThread":  func(c *Client) error { return c.AddReviewThread("id", gh.PendingComment{}) },
		"SubmitReview":     func(c *Client) error { return c.SubmitReview("id", gh.ReviewApprove, "") },
		"SubmitNewReview":  func(c *Client) error { return c.SubmitNewReview("id", gh.ReviewApprove, "") },
		"DiscardReview":    func(c *Client) error { return c.DiscardReview("id") },
		"RerunWorkflow":    func(c *Client) error { return c.RerunWorkflow(context.Background(), "kukv/demo", 1, false) },
	}
	for name, call := range writes {
		t.Run(name, func(t *testing.T) {
			c := New("", "kukv/demo")
			calls := 0
			c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				calls++
				return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
			}
			_ = call(c)
			if calls != 1 {
				t.Errorf("%s ran gh %d times, want 1: a write must never be retried", name, calls)
			}
		})
	}
}
```

**引数の型と `StartReview` の扱い**: `StartReview` は `(string, error)` を返すので
上の map に入らない。`t.Run` を 1 つ足して同じことを主張すること。
`gh.MergeSquash` / `gh.ReviewApprove` / `gh.PendingComment` の実際の名前は
`internal/gh` を読んで合わせること。**合わない名前をそのまま書かない。**

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'AskedAgain|Writes' -v`
Expected: FAIL（再試行がまだ無いので `calls` が 1 のまま）

- [ ] **Step 3: `read` を書く**

`internal/gh/cli/cli.go`:

```go
// read runs a gh call that only reads, asking again once when GitHub's front
// end did not answer. Only reads take this path: a 502 says no answer came
// back, not that nothing arrived, so a repeated write could apply twice.
func (c *Client) read(ctx context.Context, dir string, args ...string) ([]byte, error) {
	out, err := c.run(ctx, dir, args...)
	if err == nil || ctx.Err() != nil || !errors.Is(err, gh.ErrTransient) {
		return out, err
	}
	return c.run(ctx, dir, args...)
}
```

**遅延は入れない**（設計 §6。Task 6 で実測して裏づける）。

- [ ] **Step 4: 読み取りの呼び出しを `c.read` に移す**

**この 16 個だけ**。ほかは `c.run` のまま。

| ファイル | 関数 |
|---|---|
| `cli.go` | `ListPRs` / `ListIssues` / `GetPR` / `GetIssue` / `RepoName` / `ListLabels` / `ListAssignees` |
| `checks.go` | `PRChecks` / `JobLog` |
| `diff.go` | `PRDiff` / `prFiles` |
| `merge.go` | `PRMergeContext` |
| `repo_counts.go` | `RepoCounts` |
| `review.go` | `PRReviewContext` / `threadComments` |
| `graphql.go` | `ListWork`（Task 3 で `ListWorkSection` になる） |

**`RerunWorkflow` は書き込みなので移さない。** `checks.go` の 3 つのうち 1 つだけ
残る形になる。

- [ ] **Step 5: 通ることを確かめ、壊して落ちることも確かめる**

Run: `go test ./internal/gh/cli/ -v`
`AddPRComment` を `c.read` に変えて `TestWritesAreNeverAskedAgain` が落ちること、
`ctx.Err() != nil` の判定を外して `TestACancelledReadIsNotAskedAgain` が落ちることを
見てから戻す。

- [ ] **Step 6: `make check` してコミット**

```bash
git add internal/gh
git commit -m "feat: ask again once when GitHub did not answer a read

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Work 板を列ごとに引けるようにする（バックエンド）

**このタスクで画面は変わらない。** `work.Model` は 4 本を 1 つの `tea.Cmd` の中で
順に呼び、今までと同じ「全部揃ってから描く」振る舞いを保つ。部分的な描画は Task 4。

**Files:**
- Modify: `internal/gh/cli/work.graphql`, `internal/gh/cli/graphql.go`
- Modify: `internal/gh/cli/graphql_test.go`, `internal/gh/cli/schema_test.go`
- Modify: `internal/usecase/usecase.go`
- Modify: `internal/tui/work/work.go`
- Modify: `internal/tui/app/app_test.go`, `internal/tui/app/scenario_test.go`,
  `internal/tui/work/work_test.go`（fake）

**Interfaces:**
- Produces: `func (c *Client) ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error)`、
  同名の `usecase` メソッド、`work.Source` の同名メソッド
- 消える: `ListWork`（`internal/gh/cli`・`internal/usecase`・`work.Source` の 3 か所）

- [ ] **Step 1: 失敗するテストを書く**

```go
// One request per column, each carrying only its own search string. The
// board's cost is what four searches cost in one request; splitting them is
// what keeps GitHub answering (design §1).
func TestListWorkSectionSendsOneSearch(t *testing.T) {
	c := New("", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"results":{"nodes":[]}}}`), nil
	}
	if _, err := c.ListWorkSection(context.Background(), gh.SectionAssigned); err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	joined := strings.Join(got, " ")
	if strings.Count(joined, "search(") != 1 {
		t.Errorf("the document holds %d searches, want 1:\n%s", strings.Count(joined, "search("), joined)
	}
	if !strings.Contains(joined, "assignee:@me") {
		t.Errorf("the assigned column's search string is missing:\n%s", joined)
	}
	if strings.Contains(joined, "review-requested:@me") {
		t.Errorf("another column's search string came along:\n%s", joined)
	}
}

// Every column has to be reachable, and each has to send its own search.
func TestEverySectionHasItsOwnSearch(t *testing.T) {
	seen := map[string]gh.WorkSection{}
	for _, s := range gh.WorkSections() {
		c := New("", "")
		var query string
		c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			for _, a := range args {
				if strings.HasPrefix(a, "query=") {
					query = a
				}
			}
			return []byte(`{"data":{"results":{"nodes":[]}}}`), nil
		}
		if _, err := c.ListWorkSection(context.Background(), s); err != nil {
			t.Fatalf("section %d: %v", s, err)
		}
		if prev, dup := seen[query]; dup {
			t.Errorf("sections %d and %d send the same query", prev, s)
		}
		seen[query] = s
	}
}
```

**`ListWorkSection` が返すドメイン型のテストは既存の `TestListWorkTranslatesToDomainValues` を
書き直して使う。** 録ってある fixture の形（`data.reviewRequested.nodes` など）が
`data.results.nodes` に変わるので、**fixture を録り直すのではなく、
1 列ぶんを取り出した新しい fixture を録る**こと。録り方と録った日を
`internal/gh/cli/testdata/README.md` に書く（`.claude/rules/testing.md`）。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run ListWorkSection -v`
Expected: FAIL（`undefined: ListWorkSection`）

- [ ] **Step 3: `work.graphql` を書き換える**

```graphql
# One column of the Work board. The search string is the caller's: the four
# columns differ only in that, and sending them together is what made GitHub
# stop answering.
#
# reviewDecision and isDraft are GraphQL-only: REST does not expose them.
#
# search is not paged: GitHub's search connection returns at most 1000
# results however it is paged, well past what a 50-card column shows anyway.
query ($search: String!) {
  results: search(type: ISSUE, first: 50, query: $search) {
    nodes { ...WorkItem }
  }
}
```

フラグメント（`WorkItem` / `LabelFields` / `CheckContext`）はそのまま残す。

**変数名は `$query` ではなく `$search`。** `gh api graphql` は文書自体を
`-f query=` で受け取るので、変数まで `query` にすると衝突する。

- [ ] **Step 4: `graphql.go` を書き換える**

`workSearches` に検索式を持たせ、`alias` を落とす:

```go
// workSearches is each column's GitHub search. The strings are fixed text,
// not user input: they are the definition of what the column means.
var workSearches = [gh.WorkSectionCount]string{
	gh.SectionReviewRequested: "is:open is:pr review-requested:@me",
	gh.SectionYourPRs:         "is:open is:pr author:@me",
	gh.SectionAssigned:        "is:open assignee:@me",
	gh.SectionMentioned:       "is:open mentions:@me",
}
```

```go
// ListWorkSection fetches one column of the Work board.
func (c *Client) ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error) {
	if s < 0 || int(s) >= len(workSearches) {
		return nil, fmt.Errorf("unknown work section %d", s)
	}
	out, err := c.read(ctx, c.dir, "api", "graphql",
		"-f", "query="+workQuery, "-f", "search="+workSearches[s])
	...
}
```

**この呼び出しの形は 2026-09-11 に実際に叩いて確かめてある**（`-f query=<文書>` と
`-f search=<検索式>` を並べ、`$search` を宣言した文書が通ることを確認済み）。

`workResponse` の `Data` は `map[string]...` から `results` 1 つを持つ struct にする。
`ListWork` は消す。

- [ ] **Step 5: `schema_test.go` を通す**

`work.graphql` は変数を取るようになった。`schema_test.go` が文書をスキーマに
当てている形を見て、変数宣言つきの文書でも通るようにする。**通らないなら
スキーマ検証を弱めるのではなく、文書の書き方を直す。**

- [ ] **Step 6: `usecase` と `work.Model` を追従させる**

`usecase` は `ListWork` を `ListWorkSection` に差し替える。
`work.Source` も同じ。`work.Model.Refresh` は**このタスクでは**
1 つの `tea.Cmd` の中で 4 列を順に呼び、`gh.Work` を組んで今までどおり
`workMsg` を 1 回返す。**画面の振る舞いは変えない。**

```go
return m, tea.Batch(m.spin.Tick, func() tea.Msg {
	var w gh.Work
	for _, s := range gh.WorkSections() {
		items, err := src.ListWorkSection(ctx, s)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return errMsg{err}
		}
		w[s] = items
	}
	return workMsg(w)
})
```

**`.claude/rules/architecture.md` は「`tea.Cmd` のクロージャの中に 2 つ以上の
API 呼び出しを並べない」と言っている。この形は明確にそれに反する。**
Task 4 が列ごとの `tea.Cmd` に分けてこれを解消するので、**この中間状態を
コミットに残さない選択肢もある**（Task 3 と Task 4 を 1 本にする）。
実装者の判断でよい。**分けたなら、Task 3 のコミットメッセージにその旨を書くこと。**

- [ ] **Step 7: fake を追従させる**

`internal/tui/work/work_test.go`、`internal/tui/app/app_test.go`、
`internal/tui/app/scenario_test.go` の fake が `ListWork` を持っている。
`ListWorkSection` に変え、**受け取った `section` を記録させる**
（記録するだけで主張が無いテストはこのリポジトリで 9 本見つかっている。
Task 4 がそれを主張する）。

- [ ] **Step 8: `make check` してコミット**

golden は変わらないはず。変わったら**何が変わったか目で見てから**受け入れる。

```bash
git add internal/gh internal/usecase internal/tui
git commit -m "feat: fetch one column of the Work board at a time

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: 届いた列から埋める

**Files:**
- Modify: `internal/tui/work/work.go`, `internal/tui/work/render.go`
- Modify: `internal/tui/work/work_test.go`, `internal/tui/work/golden_test.go`
- Update: `internal/tui/work/testdata/*.golden`

**Interfaces:**
- Produces: `workMsg struct { section gh.WorkSection; items []gh.WorkItem }`、
  `errMsg struct { section gh.WorkSection; err error }`、
  `m.loading [gh.WorkSectionCount]bool`
- Consumes: `ListWorkSection`（Task 3）

- [ ] **Step 1: 失敗するテストを書く**

```go
// Each column is its own request, so each has to be asked for.
func TestRefreshAsksForEveryColumn(t *testing.T) {
	f := &fakeSource{}
	m, cmd := New(f).Refresh()
	drain(t, cmd)
	if len(f.sections) != gh.WorkSectionCount {
		t.Fatalf("asked for %d columns, want %d", len(f.sections), gh.WorkSectionCount)
	}
	seen := map[gh.WorkSection]bool{}
	for _, s := range f.sections {
		seen[s] = true
	}
	for _, s := range gh.WorkSections() {
		if !seen[s] {
			t.Errorf("column %d was never asked for", s)
		}
	}
}

// The columns answer at very different speeds (design §1: 1.8s to 13s), so
// what has arrived is drawn rather than held back for the slowest.
func TestAColumnIsDrawnBeforeTheOthersArrive(t *testing.T) {
	m := sized(New(&fakeSource{}), 120)
	m, _ = m.Update(workMsg{section: gh.SectionAssigned, items: sampleItems()})
	if !strings.Contains(m.View(), sampleItems()[0].Title) {
		t.Errorf("the column that answered is not on screen:\n%s", m.View())
	}
}

// One column failing must not cost the other three.
func TestAFailedColumnLeavesTheOthersAlone(t *testing.T) {
	m := sized(New(&fakeSource{}), 120)
	m, _ = m.Update(workMsg{section: gh.SectionAssigned, items: sampleItems()})
	m, _ = m.Update(errMsg{section: gh.SectionYourPRs, err: errors.New("gh: HTTP 502")})
	if !strings.Contains(m.View(), sampleItems()[0].Title) {
		t.Errorf("a failure in one column emptied another:\n%s", m.View())
	}
}

// The tab row reports the board's age. Until every column has answered the
// number would describe part of a board, so it is not shown at all.
func TestTheSummaryWaitsForEveryColumn(t *testing.T) {
	m := sized(New(&fakeSource{}), 120)
	for _, s := range gh.WorkSections()[:gh.WorkSectionCount-1] {
		m, _ = m.Update(workMsg{section: s, items: nil})
	}
	if m.Summary().Ready {
		t.Error("the summary was ready before the last column answered")
	}
	m, _ = m.Update(workMsg{section: gh.WorkSections()[gh.WorkSectionCount-1], items: nil})
	if !m.Summary().Ready {
		t.Error("the summary never became ready")
	}
}

// The age on screen is the age of the oldest thing on screen.
func TestTheSummaryReportsTheOldestColumn(t *testing.T) {
	m := sized(New(&fakeSource{}), 120)
	for _, s := range gh.WorkSections() {
		m, _ = m.Update(workMsg{section: s, items: nil})
	}
	oldest := m.Summary().FetchedAt
	for _, s := range gh.WorkSections() {
		if got := m.fetchedAt[s]; got.Before(oldest) {
			t.Errorf("column %d is older (%v) than the summary says (%v)", s, got, oldest)
		}
	}
}
```

**`TestTheSummaryReportsTheOldestColumn` は `m.fetchedAt` を直接読んでいる。**
これは実装の写しになりやすい形なので、書いたあと `Summary` を
「一番新しい」に変えて落ちることを必ず確かめること。落ちないなら書き直す。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/work/ -v`
Expected: FAIL（`workMsg` がまだ struct でない）

- [ ] **Step 3: 実装する**

- `workMsg` / `errMsg` に `section` を載せる
- `m.loading` を `[gh.WorkSectionCount]bool`、`m.fetchedAt` を
  `[gh.WorkSectionCount]time.Time` にする
- `Refresh` は列ごとに `tea.Cmd` を 1 本ずつ返す（`tea.Batch` で 4 本）。
  **クロージャの中の API 呼び出しは 1 つ**（`.claude/rules/architecture.md`）
- `Cancel` は 1 つの context を 4 本で共有し、今までどおり 1 回で全部止める。
  `releaseFetch` は**全列が答えてから**呼ぶ（1 列の答えで context を捨てると
  残り 3 本が孤児になる）
- `Summary().Ready` は全列が一度答えたとき。`FetchedAt` は成功した答えのうち
  一番古いもの
- `errMsg` は Task 5 まで今までどおり `ErrorMsg` を返す（行き先の変更は Task 5）

- [ ] **Step 4: 列の loading を描く**

`render.go`。まだ答えていない列だけスピナーを出し、答えた列は中身を描く。
**列の枠の中に収める** — 板全体を覆わない。

- [ ] **Step 5: 通ることを確かめ、壊して落ちることも確かめる**

Step 1 で書いた 5 本それぞれについて、検証対象を壊して落ちることを確かめる。
とくに `TestTheSummaryReportsTheOldestColumn`。

- [ ] **Step 6: golden を録り直して目で見る**

Run: `OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/work/ ./internal/tui/app/`
**部分的に埋まった板の golden を 1 つ足す**（1 列だけ答えた状態）。
`cat -v` で ja の 80 桁を見る。

- [ ] **Step 7: `make check` してコミット**

```bash
git add internal/tui
git commit -m "feat: draw each Work column as its answer arrives

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 失敗をタブの 1 行に落とす

**Files:**
- Modify: `internal/tui/layout/layout.go`, `internal/tui/layout/layout_test.go`
- Modify: `internal/tui/work/work.go`, `internal/tui/work/render.go`
- Modify: `internal/tui/repo/repo.go`, `internal/tui/repo/render.go`
- Modify: `internal/tui/app/app.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`
- Modify: 各 `*_test.go`、golden

**Interfaces:**
- Produces: `layout.Notice(text string, width int) string`、
  `work.Model.notice [gh.WorkSectionCount]string`、`repo.Model.notice string`、
  `work.FatalMsg` / `repo.FatalMsg`（`ErrorMsg` の改名）
- Consumes: `gh.ErrGhNotFound` / `gh.ErrUnauthenticated`（Task 1）

- [ ] **Step 1: 失敗するテストを書く**

```go
// A failure the user can do nothing about must not cost them the board they
// already have. It says what happened above the key bar and r asks again.
func TestATransientFailureKeepsTheBoard(t *testing.T) {
	m := sized(New(&fakeSource{}), 120)
	m, _ = m.Update(workMsg{section: gh.SectionAssigned, items: sampleItems()})
	m, cmd := m.Update(errMsg{section: gh.SectionYourPRs, err: errors.New("gh: HTTP 502")})
	if cmd != nil {
		if _, fatal := cmd().(FatalMsg); fatal {
			t.Error("a transient failure reached the full-screen error")
		}
	}
	view := m.View()
	if !strings.Contains(view, sampleItems()[0].Title) {
		t.Errorf("the board was lost:\n%s", view)
	}
	if !strings.Contains(view, "HTTP 502") {
		t.Errorf("the notice does not say what GitHub said:\n%s", view)
	}
}

// Only what the user must act on takes the whole screen.
func TestAMissingGhIsFatal(t *testing.T) {
	m := sized(New(&fakeSource{}), 120)
	_, cmd := m.Update(errMsg{section: gh.SectionYourPRs, err: gh.ErrGhNotFound})
	if cmd == nil {
		t.Fatal("a missing gh produced no message")
	}
	if _, fatal := cmd().(FatalMsg); !fatal {
		t.Error("a missing gh did not reach the full-screen error")
	}
}

// A complaint that outlives what it described is a lie.
func TestASuccessfulRefetchClearsTheNotice(t *testing.T) {
	m := sized(New(&fakeSource{}), 120)
	m, _ = m.Update(errMsg{section: gh.SectionYourPRs, err: errors.New("gh: HTTP 502")})
	m, _ = m.Update(workMsg{section: gh.SectionYourPRs, items: sampleItems()})
	if strings.Contains(m.View(), "HTTP 502") {
		t.Errorf("the notice outlived the failure:\n%s", m.View())
	}
}
```

`internal/tui/repo` にも同じ 3 本を、その形に合わせて書く。
**`o`（ブラウザ）の失敗が全画面に行かないことも 1 本主張する** — 積み残しの 9 番目。

`internal/tui/layout/layout_test.go`:

```go
// The notice sits on its own line: the ja key bar already fills 80 columns.
func TestNoticeFitsTheWidth(t *testing.T) {
	long := "gh api: gh: " + strings.Repeat("very long GitHub message ", 20)
	for _, w := range []int{80, 120, 160} {
		if got := ansi.StringWidth(Notice(long, w)); got > w {
			t.Errorf("at %d columns the notice is %d wide", w, got)
		}
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/... -v`
Expected: FAIL（`FatalMsg` / `Notice` が未定義）

- [ ] **Step 3: `layout.Notice` を書く**

```go
// Notice renders one line of what went wrong, cut to the terminal's width.
// It is a line of its own rather than part of the key bar: the Japanese key
// bar already fills eighty columns.
func Notice(text string, width int) string {
	return ClipLines(text, width)
}
```

**装飾（`⚠` や色）はビュー側で `theme` を通して付ける** — `layout` は色を知らない。

- [ ] **Step 4: サブモデルに `notice` を持たせる**

`work.Model`（列ごと）と `repo.Model`（1 つ）。

```go
// notice is a failure the view can carry on despite: the previous answer is
// still on screen and r asks again. A successful fetch clears it, so a stale
// complaint never outlives what it described.
notice string
```

`errMsg` の受け口:

```go
case errMsg:
	m.loading[msg.section] = false
	if fatal(msg.err) {
		err := msg.err
		return m, func() tea.Msg { return FatalMsg{err} }
	}
	m.notice[msg.section] = msg.err.Error()
	return m, nil
```

```go
// fatal reports whether the user has to act before anything can work.
// Everything else is worth a line and a retry.
func fatal(err error) bool {
	return errors.Is(err, gh.ErrGhNotFound) || errors.Is(err, gh.ErrUnauthenticated)
}
```

`fatal` は 2 パッケージで要る。**片方に書いて import させず、
それぞれのパッケージに置くか、`internal/gh` に `gh.IsFatal(err) bool` を置くか
を実装者が決めること。** 後者なら doc に理由を書く。

- [ ] **Step 5: 描く**

`work` と `repo` の `View` が、キーバーの真上に 1 行入れる。
**板・表の高さをそのぶん 1 行減らす**（`boardTop` / `footerHeight` 周り）。
通知が無いときは行を消費しない。

- [ ] **Step 6: `app` を直す**

- `work.ErrorMsg` / `repo.ErrorMsg` を `FatalMsg` に改名し、受け口も直す
- `showError` に `gh.ErrUnauthenticated` の分岐を足す（`error.unauthenticated`）
- **`Options.HasRepo` のときと同じ後始末**: 嘘になったコメントを直す。
  `fail` の doc は「板や Repos 一覧の失敗」と書いてあるが、それらはもう来ない

- [ ] **Step 7: 文言を足す**

| キー | en | ja |
|---|---|---|
| `notice.fetch_failed` | `Could not fetch` | `取得できませんでした` |
| `notice.retry` | `r to try again` | `r で再取得` |
| `error.unauthenticated` | `Not signed in to GitHub. Run: gh auth login` | `GitHub にサインインしていません。次を実行してください: gh auth login` |

通知の 1 行は `notice.fetch_failed` + GitHub の原文 + `notice.retry` を
`·` でつないだ形にする（原文は翻訳しない — `.claude/rules/errors.md`）。

- [ ] **Step 8: 通ることを確かめ、壊して落ちることも確かめる**

とくに `TestASuccessfulRefetchClearsTheNotice`（クリアを消して落ちるか）と
`TestAMissingGhIsFatal`（`fatal` から `ErrGhNotFound` を外して落ちるか）。

- [ ] **Step 9: golden を録り直して目で見る**

**通知が出ている板と Repos タブを録る**（en / ja × 80 / 120 / 160）。
`cat -v` で **ja の 80 桁で通知とキーバーが両方収まっていること**を確かめる。

- [ ] **Step 10: `make check` してコミット**

```bash
git add internal/tui internal/i18n
git commit -m "fix: keep the tab on screen when a fetch fails

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: 実測と受け渡し

**Files:**
- Modify: `docs/superpowers/specs/2026-09-11-fetch-failures-design.md`
- Create: `docs/superpowers/2026-09-11-fetch-failures-followups.md`

- [ ] **Step 1: 即時再試行の成功率を実測する**

設計 §6 は「遅延を入れない」と書いているが、**これは推測である。**
`gh auth status` がこの環境で通るので実測できる。

分割前の `work.graphql`（4 つの search を 1 リクエストに詰めた形。
git 履歴から取れる）を繰り返し投げ、502 が出たら**即座に**投げ直して
成功するかを数える。10 回以上 502 を引くまで回す。

一時ファイルはスクラッチパッドに置く（`/tmp` に直接書かない）。

- [ ] **Step 2: 設計 §6 に数字を足す**

即時再試行が何回中何回成功したかを書く。**成功率が低ければ「遅延を入れない」を
撤回し、遅延を入れるか再試行そのものを見直す。** 分割によって 502 自体がほぼ
出なくなったなら、そのことも書く（再試行の価値が下がる）。

**実測できなければ、できなかったと書いて利用者に回す。推測の数字を書かない。**

- [ ] **Step 3: 分割後の 502 率も実測する**

`ListWorkSection` を 4 列ぶん、10 周ほど回して 502 が出るかを見る。
**分割が実際に効いたかどうかが、この変更の主目的の答えである。**
設計 §1 の表に行を足す。

- [ ] **Step 4: 積み残しを書く**

`docs/superpowers/2026-09-11-fetch-failures-followups.md` に、
前のスライスと同じ形で:

- **実端末での確認の依頼**（TTY が無い環境では代行できない）: 通知が出ている板、
  `--lang ja` の 80 桁、1 列だけ失敗した状態、`r` で通知が消えること、
  `gh` を PATH から外したときに全画面に行くこと
- **オーバーレイ（詳細・diff・checks）は全画面のまま**であること。
  設計 §4 は「それ以外すべて 1 行」と書いているが、この計画で範囲を狭めた。
  理由（`esc` で戻れるので既に立ち直れる）を書く
- 見つかったが直さなかったこと、およびその理由

- [ ] **Step 5: コミット**

```bash
git add docs
git commit -m "docs: record what the fetch-failure slice measured and left behind

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## 完了条件

設計 §9 のとおり:

1. Work 板の取得が 502 で失敗しても板は残り、キーバーの上に 1 行出る
2. `r` で引き直せ、成功したら通知が消える
3. 1 列が失敗しても他の 3 列は描かれる
4. 列は届いた順に埋まる
5. `gh` が無い・認証していないときだけ全画面のエラー画面に行く
6. 読み取りは 502 で 1 回再試行し、書き込みは再試行しない
7. `internal/tui` が `internal/gh/cli` を import していない
8. `make check` が緑

**このスライスに無いもの**: オーバーレイの 1 行化、`first: 50` の切り詰め、
`internal/gh/api` バックエンド、Search タブ、自動再取得。
