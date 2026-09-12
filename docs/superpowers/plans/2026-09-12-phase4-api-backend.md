# api バックエンド本体 実装計画（Phase 4 スライス 4-2）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `gh` を PATH に持たない環境でも GraphQL 系の取得が全部動くよう、
`internal/gh/api` を新設し、`pr list` / `issue list` / `pr view` / `issue view` /
`repo view` を `internal/gh/gql` の `.graphql` 文書に置き換える。

**Architecture:** 4-1 で切り出した `gql.Client` は `Transport`（1 文書を運ぶ関数）と
`RepoVars`（リポジトリ名を GraphQL の変数に直す関数）の 2 つだけで backend の差を
吸収する。`api.Client` はその 2 つを埋めるだけの薄い型で、GraphQL 系のメソッドは
`*gql.Client` の埋め込みからそのまま生える。REST でしか取れない面（4-3）と Actions
（4-4）はこのスライスでは実装しない。

**Tech Stack:** Go 1.27.1、標準ライブラリのみ（`net/http` / `os/exec` / `encoding/json`）。
**`go-github` はこのスライスでは足さない**（4-3）。`githubv4` は Phase 4 全体で足さない
（設計 §6）。

**Spec:** `docs/superpowers/specs/2026-09-08-phase4-design.md`（特に §6・§7・§8）と
`docs/superpowers/2026-09-12-phase4-api-measurements.md`（実測）。
前スライスの積み残しは `docs/superpowers/2026-09-12-phase4-shared-graphql-followups.md`。

---

## Global Constraints

設計と `.claude/rules/` から、このスライスの全タスクに掛かるもの。

- **ネットワークも外部プロセスも実際には叩かない**（`.claude/rules/testing.md`）。
  HTTP は `httptest.Server`、`git` は差し替え可能な関数フィールドで検証する
- **パーステストの入力は実際に録ったレスポンス**。`internal/gh/gql/testdata/` に置き、
  録り方・録った日・対象リポジトリを同ディレクトリの `README.md` に書く
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
  （設計 §8。`docs/superpowers/2026-09-12-phase4-saved-queries-followups.md` に、
  この確認を飛ばして空振りテストが 3 件出た記録がある）
- **`internal/gh` は `internal/tui` も `internal/usecase` も import しない**
  （`.golangci.yml` の `gh-layer`。`**/internal/gh/**` なので `api` も自動で掛かる）
- **GitHub API 固有の文字列はこの層で止める。** `"OPEN"` / `"APPROVED"` を外に出さない
  （`.claude/rules/architecture.md`）
- **エラーは 3 種類に分ける**（`.claude/rules/errors.md`）。環境の不備＝
  `gh.ErrUnauthenticated`、GitHub 側の失敗＝原文のまま、`errors.Is` で分岐するものだけ
  センチネルにする。この層に `internal/i18n` を入れない
- **コメントは英語。** 外部の事情・一見おかしいコードが正しい理由・doc コメントの
  3 つだけ書く。**実装計画や設計書への参照をコードに書かない**（`.claude/rules/go-style.md`）
- **`context.Context` は取得系の第一引数**。構造体のフィールドに持たせない
- 各タスクの最後に `make check` が緑であること。カバレッジの基準は 80%

---

## このスライスでやらないこと（理由つき）

- **`cmd/octoscope` のバックエンド選択と認証エラー画面。** `usecase.New(src source, ...)`
  の `source`（`internal/usecase/usecase.go:113`）は 4-2 の完了時点でもまだ 18 メソッド
  足りない（REST 系 16 と Actions 2）ので、`api.Client` を渡すコードはコンパイルできない。
  **設計 §10 の 4-2 の行にある「`main.go` のバックエンド選択と認証エラー画面」は 4-4 に送る**
  （2026-09-12 に利用者の承認を得た。§10 の訂正は Task 8 で同じ PR に入れる）
- **REST 系**（`label list` / assignees / コメント / close・reopen / ラベルと担当者の編集 /
  `pulls/{n}/files` / `search repos` / `repo list` / `user/orgs`）は 4-3
- **Actions**（`RerunWorkflow` / `JobLog`）は 4-4。`gh` の `pkg/cmd/run/view/view.go` を
  読んでからでないと実装方針を決めないこと（実測資料 §3 に推定が 2 行残っている）
- **GitHub Enterprise。** `github.com` のみを相手にし、`GH_HOST` /
  `GH_ENTERPRISE_TOKEN` は見ない（設計 §6 の「境界」）
- **一覧のページング。** `first: 100` は GraphQL の 1 ページの上限であり、今の
  `gh pr list --limit 100` と同じ天井である。**100 件を超える分が落ちるのは今と同じ**で、
  このスライスは振る舞いを変えない

---

## 着手前に読んで確かめたこと（推測ではない）

実装者はここを疑ってよいが、疑うなら同じ手段で確かめ直すこと。

### 1. `gh pr list` の並び順は `CREATED_AT` の降順である（読んで、実測で確かめた）

`cli/cli` の `pkg/cmd/pr/shared/lister.go` と `pkg/cmd/issue/list/http.go` は
どちらも `orderBy: {field: CREATED_AT, direction: DESC}` を渡し、state の既定は `OPEN`。

**読むだけでは前提にしない。2026-09-12 に突き合わせた。** 下の文書と
`gh pr list` を同じリポジトリに当て、返る番号の列を比べた。

```bash
gh pr list --repo cli/cli --limit 100 --json number | jq -c '[.[].number]' > /tmp/a.json
gh api graphql -F query=@/tmp/repo_prs.graphql -f owner=cli -f name=cli \
  | jq -c '[.data.repository.pullRequests.nodes[].number]' > /tmp/b.json
diff /tmp/a.json /tmp/b.json
```

**56 件、完全に一致した**（`diff` に差分なし）。`cli/cli` の open な PR が
56 件で 100 の天井には届いていないので、**天井の一致はこの実測では確かめていない**
（`first: 100` が GraphQL の 1 ページの上限であり、`--limit 100` と同じ数字である、
というところまで）。

**並び順は飾りではない。** `internal/usecase` にも `internal/tui/repo` にも一覧を
並べ替えるコードは無い（`grep -rn 'sort\.\|slices.Sort' internal/usecase internal/tui/repo`
は `drawer.go` のチェック一覧しか返さない）ので、**GitHub が返した順がそのまま画面の順**
である。`orderBy` を書き忘れると、Repos タブの並びが黙って変わる。

### 2. `schema.json` に足りない型が 4 つある

`internal/gh/gql/testdata/schema.json` には 39 型が録ってある。今回の文書が新しく
触るのは次の 4 つで、**録り直さないと `schema_test.go` が
`type X is not in testdata/schema.json` で落ちる。**

| 型 | 要る理由 |
|---|---|
| `IssueComment` | `pullRequest.comments.nodes` / `issue.comments.nodes` |
| `IssueCommentConnection` | 同上の接続 |
| `User` | `assignees.nodes`（`login`） |
| `UserConnection` | 同上の接続 |

一覧の文書（Task 1）が使う型は全部録ってある（`Repository.pullRequests` →
`PullRequestConnection`、`Repository.issues` → `IssueConnection`、`PullRequest` /
`Issue` / `Label` / `Actor` / `Commit` / `StatusCheckRollup` 系）。
**録り直しが要るのは Task 2 からである。**

### 3. `RepoName` の失敗は致命的ではない

`internal/tui/app/app.go:109` の `resolved` は、`RepoName` がエラーを返したら
「カレントのリポジトリは無い」と読む（タイムアウトだけ別扱い）。**`api` が
「ここは git リポジトリではない」を普通のエラーで返してよい**のはこのため。
`gh.IsFatal` が真になるのは `ErrGhNotFound` と `ErrUnauthenticated` だけ。

### 4. `gql.Client.RepoVars` は `ctx` を取らない

4-1 が決めた signature が `func(repo string) ([]Var, error)` なので、`api` が
`git remote get-url` を実行する場所に呼び出し側の context は届かない。
**Task 3 は自前の短いタイムアウトを持つ**（`exec.CommandContext` に
`context.WithTimeout` を渡す）。ローカルのプロセス 1 本なので 5 秒で足りる。
4-1 の signature を広げる案は採らない: 使う側が `api` 1 つで、`cli` 側は
placeholder を返すだけの関数であり、context を通す先が無い。

---

## File Structure

| ファイル | 責務 |
|---|---|
| `internal/gh/gql/repo_prs.graphql` | 1 リポジトリの open な PR 一覧（`gh pr list` の代替） |
| `internal/gh/gql/repo_issues.graphql` | 同 Issue 一覧（`gh issue list` の代替） |
| `internal/gh/gql/pr.graphql` | PR 1 件（`gh pr view` の代替。body / comments / assignees つき） |
| `internal/gh/gql/issue.graphql` | Issue 1 件（`gh issue view` の代替） |
| `internal/gh/gql/repo_name.graphql` | `nameWithOwner`（`gh repo view` の代替） |
| `internal/gh/gql/items.go` | 上 5 本の埋め込み・デコード・`gh.PR` / `gh.Issue` への変換・5 メソッド |
| `internal/gh/api/api.go` | トークン検出（Task 4）、`Client` と `New`（Task 5・6） |
| `internal/gh/api/transport.go` | `https://api.github.com/graphql` への POST とエラー分類 |
| `internal/gh/api/repo.go` | git remote からカレントリポジトリを解決し `gql.Var` に直す |

`internal/gh/cli/item.go` は Task 7 で消える（`prJSON` / `issueJSON` は
`gh ... --json` の形であり、GraphQL に切り替えると読み手がいなくなる）。

**`items.go` は Task 3 を終えた時点で 300 行台に乗る見込みである**
（`.claude/rules/architecture.md` の「300 行を超えたら責務が増えていないか疑う」）。
責務は「PR と Issue を引く」1 つなので分けない。**Task 3 の終わりに実際の行数を数え、
400 行を超えていたら一覧（`repo_prs` / `repo_issues`）と単体（`pr` / `issue` /
`repo_name`）で 2 ファイルに割ること。** 数える前に割らない。

---

## Task 1: 一覧の 2 文書と `ListPRs` / `ListIssues`

**Files:**
- Create: `internal/gh/gql/repo_prs.graphql`
- Create: `internal/gh/gql/repo_issues.graphql`
- Create: `internal/gh/gql/items.go`
- Create: `internal/gh/gql/items_test.go`
- Create: `internal/gh/gql/testdata/repo_prs.json`（録る）
- Create: `internal/gh/gql/testdata/repo_issues.json`（録る）
- Modify: `internal/gh/gql/schema_test.go`（`checkedDocs` に 2 本足す）
- Modify: `internal/gh/gql/testdata/README.md`（録り方を書く）

**Interfaces:**
- Consumes: `gql.Client.Read`、`gql.Client.repoVars`、`gql.RollupContexts`、
  `gql.CheckContext`（すべて既存）
- Produces:
  - `func (c *Client) ListPRs(ctx context.Context, repo string) ([]gh.PR, error)`
  - `func (c *Client) ListIssues(ctx context.Context, repo string) ([]gh.Issue, error)`
  - 非公開の `prNode` / `issueNode` と `func (n prNode) toPR() gh.PR` /
    `func (n issueNode) toIssue() gh.Issue`（Task 2 が body / comments / assignees を
    足して使い回す）

- [ ] **Step 1: 一覧の PR 文書を書く**

`internal/gh/gql/repo_prs.graphql`:

```graphql
# One repository's open pull requests: what the Repos tab lists.
#
# REST cannot answer this. The /pulls listing carries neither reviewDecision
# nor statusCheckRollup nor additions/deletions, and gh pr list --json sends
# a GraphQL query of its own for the same reason.
#
# orderBy is the same one gh pr list asks for. Nothing between here and the
# screen reorders the list, so a different order here is a different screen.
#
# first: 100 is GraphQL's cap for one page, and the same ceiling gh was asked
# for with --limit 100. A repository with more open pull requests loses the
# rest, exactly as it did before.
query ($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(
      states: OPEN
      first: 100
      orderBy: { field: CREATED_AT, direction: DESC }
    ) {
      nodes { ...PullRequestListFields }
    }
  }
}

fragment PullRequestListFields on PullRequest {
  number
  title
  state
  url
  isDraft
  updatedAt
  reviewDecision
  headRefName
  baseRefName
  additions
  deletions
  author {
    login
  }
  # 100 is GitHub's cap for a connection, and an item can carry any number
  # of labels.
  labels(first: 100) {
    nodes { ...LabelFields }
  }
  commits(last: 1) {
    nodes {
      commit {
        statusCheckRollup {
          contexts(first: 100) {
            nodes { ...CheckContext }
          }
        }
      }
    }
  }
}

fragment LabelFields on Label {
  name
  color
}

# CheckRun reports status and conclusion and calls its name "name"; the older
# StatusContext reports a single state and calls its name "context". Both
# shapes have to be asked for.
fragment CheckContext on StatusCheckRollupContext {
  __typename
  ... on CheckRun {
    name
    status
    conclusion
  }
  ... on StatusContext {
    context
    state
  }
}
```

- [ ] **Step 2: 一覧の Issue 文書を書く**

`internal/gh/gql/repo_issues.graphql`:

```graphql
# One repository's open issues: what the Repos tab lists beside the pull
# requests. Same ordering and the same 100-item ceiling as repo_prs.graphql.
query ($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    issues(
      states: OPEN
      first: 100
      orderBy: { field: CREATED_AT, direction: DESC }
    ) {
      nodes { ...IssueListFields }
    }
  }
}

fragment IssueListFields on Issue {
  number
  title
  state
  url
  updatedAt
  author {
    login
  }
  labels(first: 100) {
    nodes { ...LabelFields }
  }
}

fragment LabelFields on Label {
  name
  color
}
```

- [ ] **Step 3: 失敗するテストを書く**

`internal/gh/gql/items_test.go`（`package gql`。非公開の `prNode` を読むため内部テスト）:

```go
package gql

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// fixedTransport answers every document with one recorded body.
func fixedTransport(t *testing.T, path string) (*Client, *[]Var) {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var got []Var
	c := &Client{Do: func(_ context.Context, _ string, vars []Var) ([]byte, error) {
		got = vars
		return body, nil
	}}
	return c, &got
}

func TestListPRsReadsTheRecordedAnswer(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_prs.json")
	prs, err := c.ListPRs(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) == 0 {
		t.Fatal("no pull requests decoded")
	}
	first := prs[0]
	if first.Number == 0 || first.Title == "" || first.URL == "" {
		t.Errorf("number/title/url not filled: %+v", first)
	}
	if first.State != gh.StateOpen {
		t.Errorf("state = %v, want open", first.State)
	}
	if first.Author.Login == "" {
		t.Error("author not filled")
	}
}

// The Repos tab shows the review decision and the check roll-up, and those
// two are exactly what REST could not answer. A document that stops selecting
// them decodes into a screen with empty columns and no error.
func TestListPRsFillsTheFieldsRESTCannotAnswer(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_prs.json")
	prs, err := c.ListPRs(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	var sawReview, sawChecks, sawSize bool
	for _, pr := range prs {
		if pr.Review != gh.ReviewNone {
			sawReview = true
		}
		if pr.Checks.Total > 0 {
			sawChecks = true
		}
		if pr.Additions > 0 || pr.Deletions > 0 {
			sawSize = true
		}
	}
	if !sawReview {
		t.Error("no review decision in the whole answer")
	}
	if !sawChecks {
		t.Error("no check roll-up in the whole answer")
	}
	if !sawSize {
		t.Error("no additions/deletions in the whole answer")
	}
}

// Nothing between the document and the screen reorders the list, so the
// document has to ask for the order gh asked for.
func TestTheListDocumentsAskForTheOrderGhAskedFor(t *testing.T) {
	t.Parallel()

	for name, doc := range map[string]string{
		"repo_prs.graphql":    repoPRsQuery,
		"repo_issues.graphql": repoIssuesQuery,
	} {
		if !strings.Contains(doc, "field: CREATED_AT") ||
			!strings.Contains(doc, "direction: DESC") {
			t.Errorf("%s does not order by CREATED_AT DESC", name)
		}
	}
}

// gh pr list was asked for --limit 100; the documents have to ask for as
// many, or a busy repository silently loses rows.
func TestTheListDocumentsAskForAHundred(t *testing.T) {
	t.Parallel()

	for name, doc := range map[string]string{
		"repo_prs.graphql":    repoPRsQuery,
		"repo_issues.graphql": repoIssuesQuery,
	} {
		if !strings.Contains(doc, "first: 100") {
			t.Errorf("%s does not ask for 100 items", name)
		}
	}
}

func TestListIssuesReadsTheRecordedAnswer(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_issues.json")
	issues, err := c.ListIssues(context.Background(), "kukv/octoscope")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("no issues decoded")
	}
	if issues[0].Number == 0 || issues[0].Title == "" {
		t.Errorf("number/title not filled: %+v", issues[0])
	}
}

// The repository is named by two variables, not by one "owner/name" string:
// GraphQL's repository() takes the halves separately.
func TestTheListCallsNameTheRepositoryByItsTwoHalves(t *testing.T) {
	t.Parallel()

	c, got := fixedTransport(t, "testdata/repo_prs.json")
	if _, err := c.ListPRs(context.Background(), "cli/cli"); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	want := map[string]string{"owner": "cli", "name": "cli"}
	for _, v := range *got {
		if w, ok := want[v.Name]; ok && v.Str == w {
			delete(want, v.Name)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing variables: %v (got %+v)", want, *got)
	}
}

func TestListPRsRejectsARepositoryWithoutASeparator(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_prs.json")
	if _, err := c.ListPRs(context.Background(), "octoscope"); err == nil {
		t.Fatal("want an error for a repository with no owner/name separator")
	}
}
```

- [ ] **Step 4: テストが落ちることを確認する**

Run: `go test ./internal/gh/gql/ -run 'TestList|TestTheList' -v`
Expected: FAIL（`repoPRsQuery` も `ListPRs` も未定義でコンパイルが通らない）

- [ ] **Step 5: 実装を書く**

`internal/gh/gql/items.go`:

```go
package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed repo_prs.graphql
var repoPRsQuery string

//go:embed repo_issues.graphql
var repoIssuesQuery string

// prNode is one pull request as the documents in this package select it.
// The list document leaves Body, Comments and Assignees unselected and they
// decode as zero values; the single-item document fills them.
type prNode struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	UpdatedAt      time.Time `json:"updatedAt"`
	ReviewDecision string    `json:"reviewDecision"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	Body           string    `json:"body"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels struct {
		Nodes []gh.Label `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []gh.Author `json:"nodes"`
	} `json:"assignees"`
	Comments struct {
		Nodes []commentNode `json:"nodes"`
	} `json:"comments"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []CheckContext `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// commentNode is one comment. GraphQL nests the author under an object,
// which gh.Comment already spells the same way.
type commentNode struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

func toComments(in []commentNode) []gh.Comment {
	if len(in) == 0 {
		return nil
	}
	out := make([]gh.Comment, len(in))
	for i, c := range in {
		out[i] = gh.Comment{
			Author:    gh.Author{Login: c.Author.Login},
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
		}
	}
	return out
}

func (n prNode) toPR() gh.PR {
	return gh.PR{
		Number:    n.Number,
		Title:     n.Title,
		Author:    gh.Author{Login: n.Author.Login},
		State:     gh.ParseItemState(n.State),
		IsDraft:   n.IsDraft,
		UpdatedAt: n.UpdatedAt,
		Review:    gh.ParseReviewDecision(n.ReviewDecision),
		URL:       n.URL,
		Body:      n.Body,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    n.Labels.Nodes,
		Assignees: n.Assignees.Nodes,
		Checks:    n.checks(),
		Head:      n.HeadRefName,
		Base:      n.BaseRefName,
		Additions: n.Additions,
		Deletions: n.Deletions,
	}
}

// checks reads the roll-up off the last commit, which is where GitHub hangs
// it: a pull request has no roll-up of its own.
func (n prNode) checks() gh.Checks {
	var nodes []CheckContext
	for _, commit := range n.Commits.Nodes {
		if rollup := commit.Commit.StatusCheckRollup; rollup != nil {
			nodes = append(nodes, rollup.Contexts.Nodes...)
		}
	}
	return RollupContexts(nodes)
}

type issueNode struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
	Body      string    `json:"body"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels struct {
		Nodes []gh.Label `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []gh.Author `json:"nodes"`
	} `json:"assignees"`
	Comments struct {
		Nodes []commentNode `json:"nodes"`
	} `json:"comments"`
}

func (n issueNode) toIssue() gh.Issue {
	return gh.Issue{
		Number:    n.Number,
		Title:     n.Title,
		Author:    gh.Author{Login: n.Author.Login},
		State:     gh.ParseItemState(n.State),
		UpdatedAt: n.UpdatedAt,
		URL:       n.URL,
		Body:      n.Body,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    n.Labels.Nodes,
		Assignees: n.Assignees.Nodes,
	}
}

type prListResponse struct {
	Data struct {
		Repository struct {
			PullRequests struct {
				Nodes []prNode `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	} `json:"data"`
}

type issueListResponse struct {
	Data struct {
		Repository struct {
			Issues struct {
				Nodes []issueNode `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	} `json:"data"`
}

// ListPRs returns the open pull requests of one repository. An empty repo
// means "wherever we are", which only a transport that can answer that
// accepts.
func (c *Client) ListPRs(ctx context.Context, repo string) ([]gh.PR, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.Read(ctx, repoPRsQuery, vars...)
	if err != nil {
		return nil, err
	}
	var resp prListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse pull request list: %w", err)
	}
	nodes := resp.Data.Repository.PullRequests.Nodes
	prs := make([]gh.PR, len(nodes))
	for i, n := range nodes {
		prs[i] = n.toPR()
	}
	return prs, nil
}

// ListIssues returns the open issues of one repository.
func (c *Client) ListIssues(ctx context.Context, repo string) ([]gh.Issue, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.Read(ctx, repoIssuesQuery, vars...)
	if err != nil {
		return nil, err
	}
	var resp issueListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse issue list: %w", err)
	}
	nodes := resp.Data.Repository.Issues.Nodes
	issues := make([]gh.Issue, len(nodes))
	for i, n := range nodes {
		issues[i] = n.toIssue()
	}
	return issues, nil
}
```

- [ ] **Step 6: fixture を録る**

**手で書かない。実際のレスポンスを録る**（`.claude/rules/testing.md`）。

`kukv/octoscope` には open な PR が無いことがある（`testdata/README.md` の
`merge_context.json` の項に同じ事情が書いてある）。その場合は `cli/cli` に対して録る。

**これは `testdata/README.md` の「録る対象は自分の公開リポジトリ（`kukv/octoscope`）に
限る」から外れる。** `repo_counts.json` が既に `cli/cli` を対象にしている前例は
あるが、あちらが録ったのは名前と件数だけで、今回は他人の PR のタイトルまで録る。
**黙って逸脱せず、README のその一文を先に直す**（`.claude/rules/architecture.md`
「規約そのものを変える」）。直す内容:

> 録る対象は自分の公開リポジトリ（`kukv/octoscope`）を第一とする。そこに録りたい
> 状態（open な PR など）が無いときに限り、他の**公開**リポジトリを使ってよい。
> 守るべきは「秘密情報が入らないこと」であって対象そのものではない。他人の
> リポジトリを対象にしたときは、**なぜそこにしたかを節に書く**。

```bash
D=internal/gh/gql/testdata
# PR 一覧。kukv/octoscope に open な PR があればそちらを優先する。
gh api graphql -F query=@internal/gh/gql/repo_prs.graphql \
  -f owner=cli -f name=cli | jq '.data.repository.pullRequests.nodes |= .[0:5]' > $D/repo_prs.json

# Issue 一覧。kukv/octoscope に open な Issue（#50）がある。
gh api graphql -F query=@internal/gh/gql/repo_issues.graphql \
  -f owner=kukv -f name=octoscope | jq '.data.repository.issues.nodes |= .[0:5]' > $D/repo_issues.json
```

**残す前に全ノードの `title` を人間が読む**（README の `work_section.json` の項と
同じ手順。一覧の文書は `body` を選んでいないので、読む対象はタイトルと
ブランチ名である）。インフラ・ホスト名・IP レンジ・認証情報・私有リポジトリ名が
出てくるノードは外し、次の候補に差し替える。

`TestListPRsFillsTheFieldsRESTCannotAnswer` は「答え全体のどこかに review decision と
check の roll-up と additions がある」ことを主張する。5 件の中にそれが 1 つも無いときは
**テストを緩めず、それらが入っている 5 件を選び直す。**

- [ ] **Step 7: `README.md` に録り方を書く**

`internal/gh/gql/testdata/README.md` に `## repo_prs.json` / `## repo_issues.json`
の節を足す。既存の節と同じく、**録った日・対象リポジトリ・なぜその対象なのか・
上のコマンド**を書く。`cli/cli` を選んだ場合は「`kukv/octoscope` に open な PR が
無かったため」を書く。

- [ ] **Step 8: `checkedDocs` に足す**

`internal/gh/gql/schema_test.go` の `checkedDocs` に 2 行足す。

```go
	"repo_prs.graphql":           repoPRsQuery,
	"repo_issues.graphql":        repoIssuesQuery,
```

足し忘れると `TestEveryGraphQLFileIsCheckedAgainstTheSchema` が落ちる（それが
そのテストの目的である）。

- [ ] **Step 9: テストが通ることを確認する**

Run: `go test ./internal/gh/gql/ -v`
Expected: PASS（`TestEveryFieldTheDocumentsSelectExistsInTheSchema` の新しい 2 本を含む）

- [ ] **Step 10: 空振りしていないことを確認する**

1 つずつ壊して、**落ちることを目で見る**。

- `repo_prs.graphql` から `orderBy` の行を消す → `TestTheListDocumentsAskForTheOrderGhAskedFor` が落ちる
- `first: 100` を `first: 30` にする → `TestTheListDocumentsAskForAHundred` が落ちる
- `toPR` の `Review:` の行を消す → `TestListPRsFillsTheFieldsRESTCannotAnswer` が落ちる
- `toPR` の `Checks:` の行を消す → 同上
- `repo_prs.graphql` から `reviewDecision` を消す → `schema_test` は通るが
  `TestListPRsFillsTheFieldsRESTCannotAnswer` が落ちる
- `ListPRs` の `c.repoVars(repo)` を `SplitRepoVars(repo)` の素通しに変える →
  `TestTheListCallsNameTheRepositoryByItsTwoHalves` は通ってしまう。**これは
  期待どおり**（この時点では両者が同じ結果を返す）。Task 6 でこの差が出る

全部戻してから次へ。

- [ ] **Step 11: コミット**

```bash
git add internal/gh/gql
git commit -m "feat: list a repository's pull requests and issues over GraphQL

REST cannot answer this listing: /pulls carries neither reviewDecision nor
statusCheckRollup nor additions and deletions, which is what the Repos tab
shows. gh pr list --json sends a GraphQL query for the same reason, so this
is what gh already does, not a new decision.

The order and the 100-item ceiling are the ones gh was asked for. Nothing
between the document and the screen reorders the list.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: 単体の 2 文書と `GetPR` / `GetIssue`

**Files:**
- Create: `internal/gh/gql/pr.graphql`
- Create: `internal/gh/gql/issue.graphql`
- Modify: `internal/gh/gql/items.go`（`GetPR` / `GetIssue` と embed 2 本）
- Modify: `internal/gh/gql/items_test.go`
- Create: `internal/gh/gql/testdata/pr.json`（録る）
- Create: `internal/gh/gql/testdata/issue.json`（録る）
- Modify: `internal/gh/gql/testdata/schema.json`（4 型を足して録り直す）
- Modify: `internal/gh/gql/testdata/README.md`
- Modify: `internal/gh/gql/schema_test.go`

**Interfaces:**
- Consumes: Task 1 の `prNode` / `issueNode` / `toPR` / `toIssue` / `commentNode`
- Produces:
  - `func (c *Client) GetPR(ctx context.Context, repo string, number int) (gh.PR, error)`
  - `func (c *Client) GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error)`

- [ ] **Step 1: `schema.json` を録り直す**

`testdata/README.md` の `## schema.json` にあるコマンドの `--argjson types` の配列に
**`"IssueComment"`、`"IssueCommentConnection"`、`"User"`、`"UserConnection"` の 4 つを
足して**録り直す。README の型一覧も同じ 4 つを足して更新し、「2026-09-12 に
`pr.graphql` / `issue.graphql`（`comments` と `assignees`）のために追加した」と
1 行書く（`PullRequestConnection` の行が前例）。

確認:

```bash
jq 'keys | length' internal/gh/gql/testdata/schema.json   # 39 -> 43
jq '.IssueComment.fields | keys' internal/gh/gql/testdata/schema.json
```

- [ ] **Step 2: 単体の PR 文書を書く**

`internal/gh/gql/pr.graphql`:

```graphql
# One pull request, with everything the detail view shows. The single-item
# REST endpoint has additions and deletions but still neither reviewDecision
# nor statusCheckRollup, and it carries no comments at all.
query ($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      number
      title
      state
      url
      isDraft
      updatedAt
      reviewDecision
      headRefName
      baseRefName
      additions
      deletions
      body
      author {
        login
      }
      labels(first: 100) {
        nodes { ...LabelFields }
      }
      assignees(first: 100) {
        nodes { login }
      }
      # The detail view shows the conversation in order and does not page it;
      # 100 is GitHub's cap for one connection.
      comments(first: 100) {
        nodes {
          author { login }
          body
          createdAt
        }
      }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              contexts(first: 100) {
                nodes { ...CheckContext }
              }
            }
          }
        }
      }
    }
  }
}

fragment LabelFields on Label {
  name
  color
}

fragment CheckContext on StatusCheckRollupContext {
  __typename
  ... on CheckRun {
    name
    status
    conclusion
  }
  ... on StatusContext {
    context
    state
  }
}
```

- [ ] **Step 3: 単体の Issue 文書を書く**

`internal/gh/gql/issue.graphql`:

```graphql
# One issue, with everything the detail view shows.
query ($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      number
      title
      state
      url
      updatedAt
      body
      author {
        login
      }
      labels(first: 100) {
        nodes { ...LabelFields }
      }
      assignees(first: 100) {
        nodes { login }
      }
      comments(first: 100) {
        nodes {
          author { login }
          body
          createdAt
        }
      }
    }
  }
}

fragment LabelFields on Label {
  name
  color
}
```

- [ ] **Step 4: 失敗するテストを書く**

`internal/gh/gql/items_test.go` に足す:

```go
func TestGetPRFillsTheBodyAndTheConversation(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/pr.json")
	pr, err := c.GetPR(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.Number == 0 || pr.Title == "" {
		t.Errorf("number/title not filled: %+v", pr)
	}
	if pr.Body == "" {
		t.Error("body not filled; the list document leaves it empty, the single one must not")
	}
	if len(pr.Comments) == 0 {
		t.Fatal("no comments decoded")
	}
	if pr.Comments[0].Author.Login == "" || pr.Comments[0].Body == "" {
		t.Errorf("comment not filled: %+v", pr.Comments[0])
	}
	if pr.Comments[0].CreatedAt.IsZero() {
		t.Error("comment has no timestamp")
	}
}

// The number is a GraphQL Int. A transport that spells it as a string gets
// the whole document rejected before any of it runs.
func TestGetPRSendsTheNumberAsANumber(t *testing.T) {
	t.Parallel()

	c, got := fixedTransport(t, "testdata/pr.json")
	if _, err := c.GetPR(context.Background(), "kukv/octoscope", 61); err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	for _, v := range *got {
		if v.Name == "number" {
			if v.Kind != VarInt {
				t.Errorf("number is %v, want VarInt", v.Kind)
			}
			if v.Int != 61 {
				t.Errorf("number = %d, want 61", v.Int)
			}
			return
		}
	}
	t.Errorf("no number variable in %+v", *got)
}

func TestGetIssueFillsTheBodyAndTheConversation(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/issue.json")
	issue, err := c.GetIssue(context.Background(), "kukv/octoscope", 50)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Number == 0 || issue.Title == "" {
		t.Errorf("number/title not filled: %+v", issue)
	}
	if issue.Body == "" {
		t.Error("body not filled")
	}
}
```

- [ ] **Step 5: テストが落ちることを確認する**

Run: `go test ./internal/gh/gql/ -run 'TestGet' -v`
Expected: FAIL（`GetPR` / `GetIssue` が未定義）

- [ ] **Step 6: 実装を書く**

`internal/gh/gql/items.go` に足す:

```go
//go:embed pr.graphql
var prQuery string

//go:embed issue.graphql
var issueQuery string

type prResponse struct {
	Data struct {
		Repository struct {
			PullRequest prNode `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type issueResponse struct {
	Data struct {
		Repository struct {
			Issue issueNode `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
}

// GetPR returns one pull request with its body and conversation.
func (c *Client) GetPR(ctx context.Context, repo string, number int) (gh.PR, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return gh.PR{}, err
	}
	out, err := c.Read(ctx, prQuery, append(vars, N("number", number))...)
	if err != nil {
		return gh.PR{}, err
	}
	var resp prResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.PR{}, fmt.Errorf("parse pull request: %w", err)
	}
	return resp.Data.Repository.PullRequest.toPR(), nil
}

// GetIssue returns one issue with its body and conversation.
func (c *Client) GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return gh.Issue{}, err
	}
	out, err := c.Read(ctx, issueQuery, append(vars, N("number", number))...)
	if err != nil {
		return gh.Issue{}, err
	}
	var resp issueResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.Issue{}, fmt.Errorf("parse issue: %w", err)
	}
	return resp.Data.Repository.Issue.toIssue(), nil
}
```

`append(vars, ...)` は `repoVars` が毎回新しいスライスを返すので呼び出し側を
壊さない（`SplitRepoVars` も `cli.repoVars` もリテラルを返している）。

- [ ] **Step 7: fixture を録る**

```bash
D=internal/gh/gql/testdata
gh api graphql -F query=@internal/gh/gql/pr.graphql \
  -f owner=kukv -f name=octoscope -F number=61 | jq . > $D/pr.json
gh api graphql -F query=@internal/gh/gql/issue.graphql \
  -f owner=kukv -f name=octoscope -F number=50 | jq . > $D/issue.json
```

`#61` は `pr_checks.json` / `merge_context.json` が既に対象にしている PR で、
マージ済みだが body・comments・labels・assignees は返る。**コメントが 1 件も
無い PR を選ぶと `TestGetPRFillsTheBodyAndTheConversation` が落ちる。** 落ちたら
テストを緩めず、**コメントのある PR を選び直す**（`gh pr list --state all --json
number,comments` で探す）。

`README.md` に `## pr.json` / `## issue.json` の節を足す。録った日・対象・
なぜその番号なのかを書く。

- [ ] **Step 8: `checkedDocs` に足す**

```go
	"pr.graphql":                 prQuery,
	"issue.graphql":              issueQuery,
```

- [ ] **Step 9: テストが通ることを確認する**

Run: `go test ./internal/gh/gql/ -v`
Expected: PASS

- [ ] **Step 10: 空振りしていないことを確認する**

- `pr.graphql` から `body` を消す → `TestGetPRFillsTheBodyAndTheConversation` が落ちる
- `pr.graphql` から `comments` のブロックを消す → 同上
- `GetPR` の `N("number", number)` を `S("number", strconv.Itoa(number))` にする →
  `TestGetPRSendsTheNumberAsANumber` が落ちる
- `schema.json` から `IssueComment` を消す → `TestEveryFieldTheDocumentsSelectExistsInTheSchema`
  が落ちる

全部戻す。

- [ ] **Step 11: コミット**

```bash
git add internal/gh/gql
git commit -m "feat: fetch one pull request or issue over GraphQL

The single-item REST endpoint has additions and deletions but still neither
reviewDecision nor statusCheckRollup, and no comments at all. Recording the
schema needed four more types: IssueComment, IssueCommentConnection, User and
UserConnection.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: `repo_name.graphql` と `RepoName`

**Files:**
- Create: `internal/gh/gql/repo_name.graphql`
- Modify: `internal/gh/gql/items.go`
- Modify: `internal/gh/gql/items_test.go`
- Create: `internal/gh/gql/testdata/repo_name.json`（録る）
- Modify: `internal/gh/gql/testdata/README.md`
- Modify: `internal/gh/gql/schema_test.go`

**Interfaces:**
- Produces: `func (c *Client) RepoName(ctx context.Context, repo string) (string, error)`

**`cli.RepoName` と signature が違う。** `cli` のほうは `RepoName(ctx)` で、
リポジトリはクライアントが握っているものを使う（`internal/gh/cli/cli.go` の
`c.repo`）。`gql` 側は他の 4 本と同じく `repo` を取る形にし、`cli` / `api` それぞれの
`RepoName(ctx)` がクライアントのリポジトリを渡して呼ぶ。`internal/tui/app/app.go:30` の
`RepoName(ctx context.Context) (string, error)` は**変えない**。

- [ ] **Step 1: 文書を書く**

`internal/gh/gql/repo_name.graphql`:

```graphql
# The repository's canonical "owner/name". The Repos tab asks for it to find
# out whether the working directory has a repository at all, and to follow a
# rename: GitHub answers with the current name, not the one that was asked
# for.
query ($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
  }
}
```

- [ ] **Step 2: 失敗するテストを書く**

```go
func TestRepoNameReadsTheCanonicalName(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_name.json")
	name, err := c.RepoName(context.Background(), "kukv/octoscope")
	if err != nil {
		t.Fatalf("RepoName: %v", err)
	}
	if name != "kukv/octoscope" {
		t.Errorf("name = %q, want kukv/octoscope", name)
	}
}

// A repository nobody can see comes back as a null node beside an errors
// array. "" with no error would read as "this directory has no repository",
// which is a different thing from "GitHub refused".
func TestRepoNameReportsAFailureRatherThanAnEmptyName(t *testing.T) {
	t.Parallel()

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		return []byte(`{"data":{"repository":null}}`), errors.New("Could not resolve to a Repository")
	}}
	if _, err := c.RepoName(context.Background(), "kukv/nope"); err == nil {
		t.Fatal("want the transport's error back")
	}
}
```

（`errors` を import に足す。）

- [ ] **Step 3: テストが落ちることを確認する**

Run: `go test ./internal/gh/gql/ -run TestRepoName -v`
Expected: FAIL（`RepoName` が未定義）

- [ ] **Step 4: 実装を書く**

```go
//go:embed repo_name.graphql
var repoNameQuery string

// RepoName returns the repository's canonical "owner/name".
func (c *Client) RepoName(ctx context.Context, repo string) (string, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return "", err
	}
	out, err := c.Read(ctx, repoNameQuery, vars...)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Repository struct {
				NameWithOwner string `json:"nameWithOwner"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse repository name: %w", err)
	}
	return resp.Data.Repository.NameWithOwner, nil
}
```

- [ ] **Step 5: fixture を録り、`checkedDocs` に足す**

```bash
gh api graphql -F query=@internal/gh/gql/repo_name.graphql \
  -f owner=kukv -f name=octoscope | jq . > internal/gh/gql/testdata/repo_name.json
```

`checkedDocs` に `"repo_name.graphql": repoNameQuery,` を足し、`README.md` に節を足す。

- [ ] **Step 6: テストが通ることを確認し、空振りを確認する**

Run: `go test ./internal/gh/gql/ -v` → PASS

壊して確かめる: `RepoName` の `return resp.Data.Repository.NameWithOwner, nil` を
`return "", nil` にする → `TestRepoNameReadsTheCanonicalName` が落ちる。戻す。

- [ ] **Step 7: コミット**

```bash
git add internal/gh/gql
git commit -m "feat: name a repository over GraphQL

GitHub answers with the current name, which is how a rename reaches the
Repos tab.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: `internal/gh/api` パッケージとトークン検出

**Files:**
- Create: `internal/gh/api/api.go`
- Create: `internal/gh/api/api_test.go`
- Modify: `.golangci.yml`（`tui-layer` に `internal/gh/api` を足す）

**Interfaces:**
- Produces: `func Token() (string, error)` — `GH_TOKEN` → `GITHUB_TOKEN`、
  どちらも無ければ `gh.ErrUnauthenticated`

**`Client` はこのタスクでは作らない。** transport（Task 5）と `RepoVars`（Task 6）が
揃うまで `gql.Client` を組み立てられず、空のフィールドを並べると `staticcheck` が
未使用として落とす。**各タスクはそれ単体で `make check` が緑になる形に切ってある。**

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/api/api_test.go`（`package api`）:

```go
package api

import (
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

func TestTokenPrefersGHTokenOverGitHubToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "from-gh")
	t.Setenv("GITHUB_TOKEN", "from-github")

	got, err := Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "from-gh" {
		t.Errorf("token = %q, want from-gh", got)
	}
}

func TestTokenFallsBackToGitHubToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "from-github")

	got, err := Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "from-github" {
		t.Errorf("token = %q, want from-github", got)
	}
}

// Without a token the user has to act, and the UI tells them so by asking
// gh.IsFatal. A plain error would be reported above the key bar and retried
// forever.
func TestTokenWithoutOneIsUnauthenticated(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	_, err := Token()
	if !errors.Is(err, gh.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
	if !gh.IsFatal(err) {
		t.Error("the UI would not show the error screen for this")
	}
}

// Whitespace around a token pasted into a shell profile would be sent in the
// Authorization header and rejected as a bad credential.
func TestTokenIsTrimmed(t *testing.T) {
	t.Setenv("GH_TOKEN", "  padded\n")

	got, err := Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "padded" {
		t.Errorf("token = %q, want padded", got)
	}
}
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/gh/api/ -v`
Expected: FAIL（パッケージが無い）

- [ ] **Step 3: 実装を書く**

`internal/gh/api/api.go`:

```go
// Package api fetches GitHub data by talking to the API itself, for machines
// that have no gh CLI. It answers the same domain types internal/gh/cli does
// and sends the same GraphQL documents, through a different transport.
package api

import (
	"os"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
)

// Token reads the token this backend authenticates with. The order is gh's
// own: GH_TOKEN wins so that a machine with both can point octoscope at the
// same credential gh uses.
func Token() (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, nil
		}
	}
	return "", gh.ErrUnauthenticated
}
```

これでこのファイルは閉じている。`Client` は Task 5 が入れる。

- [ ] **Step 4: depguard に足す**

`.golangci.yml` の `tui-layer` の `deny` に足す。**`internal/gh/cli` と
`internal/gh/gql` が既に並んでおり、`api` だけ抜けていると、views が
新しいバックエンドだけ直接 import できてしまう。**

```yaml
            - pkg: github.com/kukv/octoscope/internal/gh/api
              desc: views go through internal/usecase; only cmd/octoscope names a backend
```

`gh-layer` は `**/internal/gh/**` なので `api` も自動で掛かる。足す必要は無い。

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./internal/gh/api/ -v` → PASS
Run: `make lint` → 0 issues

- [ ] **Step 6: 空振りしていないことを確認する**

- `Token` の 2 つの環境変数の順を入れ替える → `TestTokenPrefersGHTokenOverGitHubToken` が落ちる
- `strings.TrimSpace` を外す → `TestTokenIsTrimmed` が落ちる
- `gh.ErrUnauthenticated` を `errors.New("no token")` にする →
  `TestTokenWithoutOneIsUnauthenticated` が落ちる

- [ ] **Step 7: コミット**

```bash
git add internal/gh/api .golangci.yml
git commit -m "feat: read the api backend's token from the environment

GH_TOKEN wins over GITHUB_TOKEN so a machine with both points octoscope at
the credential gh uses. Missing means the user has to act, so it is
ErrUnauthenticated and gh.IsFatal says so.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: `Client` と HTTP transport

**Files:**
- Create: `internal/gh/api/transport.go`
- Create: `internal/gh/api/transport_test.go`
- Modify: `internal/gh/api/api.go`（`Client` と `New` と `OpenWeb`）

**Interfaces:**
- Consumes: `gql.Transport`（`func(ctx, doc string, vars []gql.Var) ([]byte, error)`）、
  `gql.VarString` / `gql.VarInt` / `gql.VarPlaceholder`、`gh.ErrTransient` /
  `gh.ErrUnauthenticated` / `gh.Classify`
- Produces:
  - `type Client struct { *gql.Client; ... }`
  - `func New(repo, token string) *Client`（**Task 6 で `New(dir, repo, token)` に
    変わる**。同じ型の引数が並ぶので、Task 6 で 3 つ目を足す時点で並び順を
    doc コメントに書く）
  - `func (c *Client) post(ctx context.Context, doc string, vars []gql.Var) ([]byte, error)`
  - `func (c *Client) OpenWeb(url string) error`

**この時点の `New` は `RepoVars` を埋めない。** `gql.Client.repoVars` は `nil` の
とき `SplitRepoVars` に落ちる（`internal/gh/gql/gql.go`）ので、「owner/name を
明示すれば動く」状態になる。カレントディレクトリからの解決は Task 6。

**transport の契約**（`internal/gh/gql/gql.go` の `Transport` の doc コメント）:
**err が非 nil でも body を返すこと。** GitHub は部分的に解決できたクエリに対し、
解決できた `data` と並べて top-level の `errors` を返す。`RepoCounts` はそれを
拾って「この 1 つだけバッジを『—』にする」を実現している。

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/api/transport_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

// recorded is what the stand-in GitHub was sent. The handler fills it while
// the call under test is still running, so the fields are read only after
// that call returns -- a value captured at serve's return would always be
// the zero one.
type recorded struct {
	req  *http.Request
	body []byte
}

// serve stands in for GitHub. It records the one request it was sent.
func serve(t *testing.T, status int, body string) (*Client, *recorded) {
	t.Helper()

	var got recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.req = r.Clone(r.Context())
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	c := New("kukv/octoscope", "secret-token")
	c.endpoint = srv.URL
	return c, &got
}

func TestThePostCarriesTheDocumentAndTheVariables(t *testing.T) {
	t.Parallel()

	c, got := serve(t, http.StatusOK, `{"data":{}}`)
	if _, err := c.post(context.Background(), "query { viewer { login } }",
		[]gql.Var{gql.S("owner", "kukv"), gql.N("number", 61)}); err != nil {
		t.Fatalf("post: %v", err)
	}

	var sent struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(got.body, &sent); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if !strings.Contains(sent.Query, "viewer") {
		t.Errorf("query = %q", sent.Query)
	}
	if sent.Variables["owner"] != "kukv" {
		t.Errorf("owner = %v, want kukv", sent.Variables["owner"])
	}
	// GraphQL rejects "61" where it wants 61: the number has to travel as a
	// JSON number, not as a string.
	if n, ok := sent.Variables["number"].(float64); !ok || n != 61 {
		t.Errorf("number = %#v, want the JSON number 61", sent.Variables["number"])
	}
}

func TestThePostAuthenticatesWithTheToken(t *testing.T) {
	t.Parallel()

	c, got := serve(t, http.StatusOK, `{"data":{}}`)
	if _, err := c.post(context.Background(), "query {}", nil); err != nil {
		t.Fatalf("post: %v", err)
	}
	if h := got.req.Header.Get("Authorization"); h != "bearer secret-token" {
		t.Errorf("Authorization = %q, want bearer secret-token", h)
	}
	// GitHub asks every client to name itself and may refuse an unnamed one.
	if got.req.Header.Get("User-Agent") == "" {
		t.Error("no User-Agent")
	}
}

// A placeholder is a value only gh can fill in, from the working directory's
// remote. Sending "{owner}" to the API would ask GitHub for a repository
// with that literal name.
func TestThePostRefusesAPlaceholder(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusOK, `{"data":{}}`)
	_, err := c.post(context.Background(), "query {}",
		[]gql.Var{gql.Placeholder("owner", "{owner}")})
	if err == nil {
		t.Fatal("want an error for a placeholder variable")
	}
}

func TestAGatewayFailureIsTransient(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		c, _ := serve(t, status, "")
		_, err := c.post(context.Background(), "query {}", nil)
		if !errors.Is(err, gh.ErrTransient) {
			t.Errorf("status %d: err = %v, want ErrTransient", status, err)
		}
	}
}

func TestABadCredentialIsUnauthenticated(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusUnauthorized, `{"message":"Bad credentials"}`)
	_, err := c.post(context.Background(), "query {}", nil)
	if !errors.Is(err, gh.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
	if !gh.IsFatal(err) {
		t.Error("the UI would not show the error screen for this")
	}
}

// GitHub answers a partially resolvable query with the data it could resolve
// beside a top-level errors array. RepoCounts salvages that body to draw the
// badges it does have, so the transport must hand it back with the error.
func TestAPartialAnswerComesBackWithItsBody(t *testing.T) {
	t.Parallel()

	const partial = `{"data":{"r0":{"nameWithOwner":"kukv/octoscope"},"r1":null},` +
		`"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a Repository"}]}`
	c, _ := serve(t, http.StatusOK, partial)

	out, err := c.post(context.Background(), "query {}", nil)
	if err == nil {
		t.Fatal("want an error beside the body")
	}
	if !strings.Contains(string(out), "kukv/octoscope") {
		t.Errorf("body was dropped: %q", out)
	}
	if !strings.Contains(err.Error(), "Could not resolve to a Repository") {
		t.Errorf("err = %q, want GitHub's own words", err)
	}
}

// What GitHub said is the most informative thing a user gets, and the rules
// leave it untranslated. A wrapper that replaces it loses the only actionable
// part.
func TestAnOrdinaryFailureKeepsWhatGitHubSaid(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusForbidden, `{"message":"API rate limit exceeded"}`)
	_, err := c.post(context.Background(), "query {}", nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "API rate limit exceeded") {
		t.Errorf("err = %q, want GitHub's own words", err)
	}
	if gh.IsFatal(err) {
		t.Error("a rate limit is not something the user has to fix before anything works")
	}
}
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/gh/api/ -run 'TestThePost|TestAGateway|TestABad|TestAPartial|TestAnOrdinary' -v`
Expected: FAIL（`post` も `endpoint` も無い）

- [ ] **Step 3: 実装を書く**

`internal/gh/api/transport.go`:

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

// endpoint is github.com's GraphQL endpoint. GitHub Enterprise is out of
// scope for this phase, so GH_HOST is not read.
const defaultEndpoint = "https://api.github.com/graphql"

// errorsBody is the part of an answer that says what failed. GitHub puts a
// top-level "errors" array beside whatever "data" it could resolve; a request
// that failed outright carries "message" instead.
type errorsBody struct {
	Message string `json:"message"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// text is the single sentence to show, GitHub's own words and nothing else.
func (b errorsBody) text() string {
	msgs := make([]string, 0, len(b.Errors)+1)
	for _, e := range b.Errors {
		if e.Message != "" {
			msgs = append(msgs, e.Message)
		}
	}
	if len(msgs) == 0 && b.Message != "" {
		msgs = append(msgs, b.Message)
	}
	return strings.Join(msgs, "; ")
}

// post sends one document to GitHub.
//
// It returns the body even when err is non-nil: a partially resolvable query
// answers with the data it could resolve beside a top-level errors array, and
// RepoCounts draws the badges it does have out of that body.
func (c *Client) post(ctx context.Context, doc string, vars []gql.Var) ([]byte, error) {
	payload, err := requestBody(doc, vars)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointURL(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	// GitHub asks every client to name itself; an unnamed one may be refused.
	req.Header.Set("User-Agent", "octoscope")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach GitHub: %w", err)
	}
	// Closing a body that was only read has nothing to report.
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read answer: %w", err)
	}
	return out, classify(resp.StatusCode, out)
}

// requestBody spells the document and its variables the way GraphQL takes
// them. A number has to travel as a JSON number: GraphQL rejects "61" where
// it wants 61.
func requestBody(doc string, vars []gql.Var) ([]byte, error) {
	values := make(map[string]any, len(vars))
	for _, v := range vars {
		switch v.Kind {
		case gql.VarInt:
			values[v.Name] = v.Int
		case gql.VarPlaceholder:
			// Only gh fills these in, from the working directory's remote.
			// Sending one would ask GitHub for a repository literally named
			// "{repo}".
			return nil, fmt.Errorf("variable %q is a placeholder only gh can fill in", v.Name)
		default:
			values[v.Name] = v.Str
		}
	}
	payload, err := json.Marshal(map[string]any{"query": doc, "variables": values})
	if err != nil {
		return nil, fmt.Errorf("build request body: %w", err)
	}
	return payload, nil
}

// classify names the failures a caller acts on differently: one worth asking
// again for, and one only the user can fix. Everything else keeps what
// GitHub said and no type at all.
func classify(status int, body []byte) error {
	var b errorsBody
	// A body that is not JSON leaves b zero, which reads as "no words from
	// GitHub" -- the status alone then describes the failure.
	_ = json.Unmarshal(body, &b)
	msg := b.text()

	switch {
	case status == http.StatusBadGateway,
		status == http.StatusServiceUnavailable,
		status == http.StatusGatewayTimeout:
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return gh.Classify(gh.ErrTransient, msg)
	case status == http.StatusUnauthorized:
		if msg == "" {
			msg = "HTTP 401"
		}
		return gh.Classify(gh.ErrUnauthenticated, msg)
	case status >= 400:
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return gh.Classify(nil, msg)
	case msg != "":
		// 200 with a top-level errors array: a partially resolvable query.
		return gh.Classify(nil, msg)
	}
	return nil
}
```

`gh.Classify(nil, msg)` は `errors.Is` でどのセンチネルにも当たらない、GitHub の
文言だけを持つエラーになる。**この計画を書いた時点で実際に走らせて確かめた**:
`Error()` は渡した文言そのもの、`errors.Is` は `ErrTransient` にも
`ErrUnauthenticated` にも当たらず、`gh.IsFatal` は偽。`Unwrap` が `nil` を返すのは
`errors.Is` の停止条件であって異常ではない（`internal/gh/gh.go` の `classified`）。

`internal/gh/api/api.go` に `Client` と `New` と `OpenWeb` を足す
（import に `net/http`、`github.com/kukv/octoscope/internal/browser`、
`github.com/kukv/octoscope/internal/gh/gql` が増える）:

```go
// Client talks to GitHub over HTTPS. The GraphQL calls come from the
// embedded gql.Client, which this type gives a transport.
type Client struct {
	*gql.Client
	repo  string
	token string
	// endpoint and client are the seams the tests use: a local server, and
	// a client with a shorter patience than the default.
	endpoint string
	client   *http.Client
}

// New returns a client for the repository named by repo ("owner/name").
func New(repo, token string) *Client {
	c := &Client{repo: repo, token: token}
	c.Client = &gql.Client{Do: c.post}
	return c
}

// endpointURL is where the documents go. Tests point it at a local server.
func (c *Client) endpointURL() string {
	if c.endpoint != "" {
		return c.endpoint
	}
	return defaultEndpoint
}

// httpClient is the client to send with. Callers do not set one.
func (c *Client) httpClient() *http.Client {
	if c.client != nil {
		return c.client
	}
	return http.DefaultClient
}

// OpenWeb shows the item in a browser. It is the same call the cli backend
// makes: GitHub gives every item its URL, and opening one needs no backend.
func (c *Client) OpenWeb(url string) error {
	return browser.Open(url)
}
```

`gql.Client.RepoVars` は `nil` のまま置く。その場合 `SplitRepoVars` に落ちるので、
`repoVars("kukv/octoscope")` は動き、`repoVars("")` は
`repo "" has no owner/name separator` で失敗する。**Task 6 がそこを埋める。**

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/gh/api/ -v` → PASS

- [ ] **Step 5: 空振りしていないことを確認する**

- `requestBody` の `case gql.VarInt:` を消して `default` に流す →
  `TestThePostCarriesTheDocumentAndTheVariables` が落ちる
- `VarPlaceholder` の分岐を消す → `TestThePostRefusesAPlaceholder` が落ちる
- `classify` の 502/503/504 の分岐を消す → `TestAGatewayFailureIsTransient` が落ちる
- `post` の `return out, classify(...)` を `return nil, classify(...)` にする →
  `TestAPartialAnswerComesBackWithItsBody` が落ちる（**この 1 本が
  `RepoCounts` の部分失敗を守っている唯一のテストである**）
- `classify` の `msg` を `"request failed"` の固定文言にする →
  `TestAnOrdinaryFailureKeepsWhatGitHubSaid` が落ちる

- [ ] **Step 6: コミット**

```bash
git add internal/gh/api
git commit -m "feat: send the GraphQL documents over HTTPS

The transport hands the body back with the error: GitHub answers a partially
resolvable query with the data it could resolve beside a top-level errors
array, and RepoCounts draws the badges it does have out of that body.

A placeholder variable is refused rather than sent. Only gh fills {owner} and
{repo} in, from the working directory's remote.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: カレントリポジトリの解決

**Files:**
- Create: `internal/gh/api/repo.go`
- Create: `internal/gh/api/repo_test.go`
- Create: `internal/gh/api/parity_test.go`（`package api_test`）
- Modify: `internal/gh/api/api.go`（`New` が `dir` を取り、`RepoVars` を埋める）
- Modify: `internal/gh/api/transport_test.go`（`serve` の `New` の呼び出しに
  `dir` の引数が 1 つ増える）

**Interfaces:**
- Produces:
  - `func (c *Client) repoVars(repo string) ([]gql.Var, error)`
  - `func (c *Client) RepoName(ctx context.Context) (string, error)`（`app.Source` の形）
  - 非公開の `func parseRemote(url string) (string, error)`

**なぜ必要か:** `gh` は working directory の remote から `{owner}` / `{repo}` を
埋める。`api` にその代行者はいないので、自分で remote を読む
（`docs/superpowers/2026-09-12-phase4-shared-graphql-followups.md` の
「次のスライスに渡すもの」）。

**`.git` はファイルのことがある。** この worktree がまさにそうで、
`.git/config` を直接読む実装は worktree で壊れる。**`git remote get-url origin` を
実行する。**

**context が無い。** `gql.Client.RepoVars` は `func(repo string) ([]gql.Var, error)`
なので、呼び出し側の context は届かない。自前の短いタイムアウトを持つ。

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/api/repo_test.go`:

```go
package api

import (
	"context"
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/gh/gql"
)

func TestParseRemoteReadsEveryShapeGitWritesTheURLIn(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		url  string
		want string
	}{
		"scp-like ssh":      {"git@github.com:kukv/octoscope.git", "kukv/octoscope"},
		"ssh url":           {"ssh://git@github.com/kukv/octoscope.git", "kukv/octoscope"},
		"ssh url no user":   {"ssh://github.com/kukv/octoscope.git", "kukv/octoscope"},
		"https with suffix": {"https://github.com/kukv/octoscope.git", "kukv/octoscope"},
		"https bare":        {"https://github.com/kukv/octoscope", "kukv/octoscope"},
		"https with user":   {"https://kukv@github.com/kukv/octoscope.git", "kukv/octoscope"},
		"trailing slash":    {"https://github.com/kukv/octoscope/", "kukv/octoscope"},
		"trailing newline":  {"https://github.com/kukv/octoscope.git\n", "kukv/octoscope"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRemote(tc.url)
			if err != nil {
				t.Fatalf("parseRemote(%q): %v", tc.url, err)
			}
			if got != tc.want {
				t.Errorf("parseRemote(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// GitHub Enterprise is out of scope for this phase. A remote pointing
// somewhere else has to say so rather than be read as a github.com
// repository that does not exist.
func TestParseRemoteRefusesAnythingButGitHubCom(t *testing.T) {
	t.Parallel()

	for _, url := range []string{
		"git@gitlab.com:kukv/octoscope.git",
		"https://github.example.com/kukv/octoscope.git",
		"https://github.com/kukv",
		"",
	} {
		if got, err := parseRemote(url); err == nil {
			t.Errorf("parseRemote(%q) = %q, want an error", url, got)
		}
	}
}

// --repo is an override for the moment; the remote is what the directory is.
func TestAnExplicitRepositoryWinsOverTheRemote(t *testing.T) {
	t.Parallel()

	c := New("", "other/repo", "token")
	c.runGit = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("git must not run when the repository was named")
		return nil, nil
	}
	vars, err := c.repoVars("")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	assertRepoVars(t, vars, "other", "repo")
}

// The per-call repository wins over the client's: the Repos sidebar fetches
// several repositories through one client.
func TestAPerCallRepositoryWinsOverTheClients(t *testing.T) {
	t.Parallel()

	c := New("", "other/repo", "token")
	vars, err := c.repoVars("kukv/octoscope")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	assertRepoVars(t, vars, "kukv", "octoscope")
}

func TestTheRemoteFillsInTheRepositoryWhenNoneWasNamed(t *testing.T) {
	t.Parallel()

	c := New("/somewhere", "", "token")
	var gotDir string
	var gotArgs []string
	c.runGit = func(_ context.Context, dir string, args ...string) ([]byte, error) {
		gotDir, gotArgs = dir, args
		return []byte("git@github.com:kukv/octoscope.git\n"), nil
	}
	vars, err := c.repoVars("")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	assertRepoVars(t, vars, "kukv", "octoscope")
	if gotDir != "/somewhere" {
		t.Errorf("dir = %q, want /somewhere", gotDir)
	}
	if len(gotArgs) == 0 || gotArgs[0] != "remote" {
		t.Errorf("args = %v, want a git remote lookup", gotArgs)
	}
}

// A directory that is no repository leaves the Repos tab without a current
// one, which the app reads as "there is none". It must not be fatal.
func TestADirectoryThatIsNoRepositoryIsAnOrdinaryFailure(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "", "token")
	c.runGit = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("fatal: not a git repository")
	}
	if _, err := c.repoVars(""); err == nil {
		t.Fatal("want an error")
	}
}

// The remote is read once: the sidebar asks for several repositories at
// once, and a subprocess per badge would be paid for every refresh.
func TestTheRemoteIsReadOnlyOnce(t *testing.T) {
	t.Parallel()

	c := New("", "", "token")
	calls := 0
	c.runGit = func(context.Context, string, ...string) ([]byte, error) {
		calls++
		return []byte("https://github.com/kukv/octoscope.git"), nil
	}
	for range 3 {
		if _, err := c.repoVars(""); err != nil {
			t.Fatalf("repoVars: %v", err)
		}
	}
	if calls != 1 {
		t.Errorf("git ran %d times, want 1", calls)
	}
}

func assertRepoVars(t *testing.T, vars []gql.Var, owner, name string) {
	t.Helper()

	got := map[string]string{}
	for _, v := range vars {
		if v.Kind != gql.VarString {
			t.Errorf("%s is %v, want VarString", v.Name, v.Kind)
		}
		got[v.Name] = v.Str
	}
	if got["owner"] != owner || got["name"] != name {
		t.Errorf("vars = %v, want owner=%s name=%s", got, owner, name)
	}
}
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/gh/api/ -run 'TestParseRemote|TestTheRemote|TestAnExplicit|TestAPerCall|TestADirectory' -v`
Expected: FAIL（`parseRemote` も `runGit` も無い）

- [ ] **Step 3: 実装を書く**

`internal/gh/api/repo.go`:

```go
package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

// remoteTimeout bounds the one local subprocess this package runs. gh's own
// lookup is allowed twenty seconds because it reaches the API; reading a
// remote out of a git config does not.
const remoteTimeout = 5 * time.Second

// repoVars names the repository of a call. An explicit repository -- the
// --repo flag, or the sidebar asking for one of its rows -- wins; otherwise
// the working directory's remote says where we are, the way gh resolves it.
func (c *Client) repoVars(repo string) ([]gql.Var, error) {
	if repo == "" {
		repo = c.repo
	}
	if repo == "" {
		var err error
		if repo, err = c.currentRepo(); err != nil {
			return nil, err
		}
	}
	return gql.SplitRepoVars(repo)
}

// currentRepo reads the working directory's origin remote, once. The sidebar
// asks for several repositories at a time and every refresh would otherwise
// pay for a subprocess per row.
//
// gql.Client.RepoVars takes no context, so the deadline is this package's own.
func (c *Client) currentRepo() (string, error) {
	c.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
		defer cancel()
		out, err := c.git()(ctx, c.dir, "remote", "get-url", "origin")
		if err != nil {
			c.currentErr = fmt.Errorf("read the origin remote: %w", err)
			return
		}
		c.current, c.currentErr = parseRemote(string(out))
	})
	return c.current, c.currentErr
}

// parseRemote reads "owner/name" out of a remote URL. git writes the same
// remote in several shapes -- scp-like ssh, an ssh:// URL, https with or
// without the .git suffix -- and all of them reach this.
//
// Anything but github.com is refused: GitHub Enterprise is out of scope, and
// reading a gitlab remote as a github repository would ask GitHub for one
// that does not exist.
func parseRemote(url string) (string, error) {
	s := strings.TrimSpace(url)
	const host = "github.com"

	switch {
	case strings.HasPrefix(s, "git@"+host+":"):
		s = strings.TrimPrefix(s, "git@"+host+":")
	default:
		// Strip the scheme and any user@ in front of the host.
		if _, rest, ok := strings.Cut(s, "://"); ok {
			s = rest
		}
		if _, rest, ok := strings.Cut(s, "@"); ok {
			s = rest
		}
		rest, ok := strings.CutPrefix(s, host+"/")
		if !ok {
			return "", fmt.Errorf("remote %q is not on %s", url, host)
		}
		s = rest
	}
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(s), "/"), ".git")
	if _, _, ok := gh.SplitRepo(s); !ok {
		return "", fmt.Errorf("remote %q has no owner/name", url)
	}
	return s, nil
}

// RepoName returns the repository the client works against, as GitHub spells
// it. A failure means the directory has no repository to speak of, which the
// Repos tab reads as "there is no current repository" rather than as a fault.
func (c *Client) RepoName(ctx context.Context) (string, error) {
	return c.Client.RepoName(ctx, c.repo)
}

// git is the subprocess runner. Tests replace it.
func (c *Client) git() gitFunc {
	if c.runGit != nil {
		return c.runGit
	}
	return runGit
}

type gitFunc func(ctx context.Context, dir string, args ...string) ([]byte, error)

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	// The arguments are this package's own literals, never external input.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return nil, errors.New(string(msg))
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}
```

`api.go` の `Client` に足し、`New` を書き換える:

```go
type Client struct {
	*gql.Client
	dir   string
	repo  string
	token string

	endpoint string
	client   *http.Client

	runGit     gitFunc
	once       sync.Once
	current    string
	currentErr error
}

// New returns a client for the repository named by repo ("owner/name"), whose
// working directory is dir. An empty repo is resolved from dir's git remote,
// the way gh resolves it from the working directory.
//
// The three strings are dir, repo, token, in that order: all of them are
// strings, so a swapped pair still compiles.
func New(dir, repo, token string) *Client {
	c := &Client{dir: dir, repo: repo, token: token}
	c.Client = &gql.Client{
		Do:       c.post,
		RepoVars: c.repoVars,
	}
	return c
}
```

`transport_test.go` の `serve` の `New("kukv/octoscope", "secret-token")` を
`New("", "kukv/octoscope", "secret-token")` に直す。

**`sync.Once` を持つ以上 `Client` はコピーできない。** `New` がポインタを返し、
どのメソッドもポインタレシーバなので問題ないが、値でコピーする使い方を足さないこと。

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/gh/api/ -v` → PASS

- [ ] **Step 5: 空振りしていないことを確認する**

- `parseRemote` の `host` の判定を消す → `TestParseRemoteRefusesAnythingButGitHubCom` が落ちる
- `strings.TrimSuffix(..., ".git")` を消す → `TestParseRemoteReadsEveryShapeGitWritesTheURLIn` が落ちる
- `repoVars` の `if repo == "" { repo = c.repo }` を消す →
  `TestAnExplicitRepositoryWinsOverTheRemote` が落ちる
- `currentRepo` の `c.once.Do` を外して毎回実行する → `TestTheRemoteIsReadOnlyOnce` が落ちる

- [ ] **Step 6: `api.Client` が GraphQL 系を全部持っていることを確認する**

**コンパイルで確かめる。** `internal/gh/api/api_test.go` に足す:

```go
// graphQLSource is every GraphQL-backed operation the usecase layer's source
// interface asks for. The rest of that interface -- the REST calls and the
// Actions calls -- arrives in later slices; this is the part this backend is
// finished for.
type graphQLSource interface {
	ListPRs(ctx context.Context, repo string) ([]gh.PR, error)
	ListIssues(ctx context.Context, repo string) ([]gh.Issue, error)
	GetPR(ctx context.Context, repo string, number int) (gh.PR, error)
	GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error)
	RepoName(ctx context.Context) (string, error)
	ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error)
	SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error)
	RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error)
	PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
	PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error)
	OpenWeb(url string) error
}

// Both backends answer the same operations with the same domain types. A
// method that only one of them has would leave the other's screens empty.
var (
	_ graphQLSource = (*Client)(nil)
	_ graphQLSource = (*cli.Client)(nil)
)
```

**`cli.Client` を import するので、このファイルは `package api_test`（外部テスト
パッケージ）に分ける** — `api` が `cli` を import すると、バックエンドどうしが
依存し合う。`internal/gh/api/parity_test.go` として `package api_test` で置く。

この時点で `cli.Client` は `RepoName(ctx)` を持ち（既存）、`api.Client` も持つ。
`ListWorkSection` などは両方 `*gql.Client` の埋め込みから生えている。

- [ ] **Step 7: コミット**

```bash
git add internal/gh/api
git commit -m "feat: resolve the current repository from the git remote

gh fills {owner} and {repo} from the working directory's remote; this backend
has no such stand-in, so it reads the remote itself. It runs git rather than
parsing .git/config: in a worktree .git is a file, not a directory.

The lookup is cached because the sidebar asks for several repositories at a
time, and gql.Client.RepoVars takes no context, so the deadline is this
package's own.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: `cli` を新しい文書に切り替える

**Files:**
- Modify: `internal/gh/cli/cli.go`（5 メソッドを消す）
- Delete: `internal/gh/cli/item.go`
- Modify: `internal/gh/cli/cli_test.go`（`gh` の引数を主張していたテストを外す）
- Modify: `internal/gh/cli/graphql_test.go`（文書を通したテストに寄せる）

**Interfaces:**
- Consumes: Task 1〜3 の `gql.Client.ListPRs` / `ListIssues` / `GetPR` / `GetIssue` /
  `RepoName(ctx, repo)`
- Produces: `cli.Client.RepoName(ctx) (string, error)`（`app.Source` の形のまま）

**なぜやるか:** 完了条件 8「`.graphql` 文書が 1 組のまま両バックエンドから使われ、
`schema_test.go` がその 1 組を検証している」を、この 5 経路について満たすため。
切り替えないと、`gh ... --json` の綴りとこの `.graphql` の 2 組が並び、
片方だけがスキーマ検証を受ける状態が続く。

**振る舞いを変えないこと。** 並び順（`CREATED_AT` の降順）と 100 件の天井は
Task 1 で合わせてある。`gh` が中で投げているのと同じクエリになる。

- [ ] **Step 1: 落ちるテストを先に確かめる**

今の `cli_test.go` は `gh pr list` の引数を主張している。**まずそれらを探す。**

Run: `go test ./internal/gh/cli/ -run 'TestPRList|TestIssueList|TestGetPR|TestGetIssue|TestRepoName' -v`

どのテストが `pr list` / `issue list` / `pr view` / `issue view` / `repo view` の
引数を見ているかを一覧にしてから次に進む。

- [ ] **Step 2: 切り替え後の姿を主張するテストを書く**

`internal/gh/cli/graphql_test.go` に足す（このファイルは既に `gh api graphql` の
引数を見る形になっている）:

```go
// The five listing calls go through the shared documents now, not through
// gh's own subcommands. A backend that still shells out to `gh pr list`
// would be selecting a second, unchecked copy of the same fields.
func TestTheListingCallsSendAGraphQLDocument(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*Client) error{
		"ListPRs":    func(c *Client) error { _, err := c.ListPRs(context.Background(), "kukv/octoscope"); return err },
		"ListIssues": func(c *Client) error { _, err := c.ListIssues(context.Background(), "kukv/octoscope"); return err },
		"GetPR":      func(c *Client) error { _, err := c.GetPR(context.Background(), "kukv/octoscope", 1); return err },
		"GetIssue":   func(c *Client) error { _, err := c.GetIssue(context.Background(), "kukv/octoscope", 1); return err },
		"RepoName":   func(c *Client) error { _, err := c.RepoName(context.Background()); return err },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := New("", "kukv/octoscope")
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(`{"data":{}}`), nil
			}
			if err := call(c); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if len(got) < 2 || got[0] != "api" || got[1] != "graphql" {
				t.Errorf("%s ran gh %v, want gh api graphql", name, got)
			}
		})
	}
}

// Inside a checkout with no --repo, gh is the one that knows where we are:
// it fills {owner} and {repo} from the remote. The api backend cannot, which
// is why the two spell the same variables differently.
func TestWithoutARepositoryTheDocumentCarriesGhsPlaceholders(t *testing.T) {
	t.Parallel()

	c := New("", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{}}`), nil
	}
	if _, err := c.ListPRs(context.Background(), ""); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "owner={owner}") || !strings.Contains(joined, "name={repo}") {
		t.Errorf("args = %v, want gh's own placeholders", got)
	}
}
```

- [ ] **Step 3: テストが落ちることを確認する**

Run: `go test ./internal/gh/cli/ -run 'TestTheListingCalls|TestWithoutARepository' -v`
Expected: FAIL（まだ `gh pr list` を走らせている）

- [ ] **Step 4: `cli.go` から 5 メソッドを消す**

`ListPRs` / `ListIssues` / `GetPR` / `GetIssue` を丸ごと削除する（埋め込んだ
`*gql.Client` のものが見えるようになる）。`RepoName` は signature が違うので
残し、中身を委譲に変える:

```go
// RepoName returns the repository the client works against, as GitHub spells
// it. An empty repo means the working directory's, which gh fills in.
func (c *Client) RepoName(ctx context.Context) (string, error) {
	return c.Client.RepoName(ctx, c.repo)
}
```

**使われなくなるものを落とす**（`.claude/rules` の「自分の変更によって不要に
なったもの」）:

- `internal/gh/cli/item.go` を削除（`prJSON` / `issueJSON` / `toPRs` / `toIssues`）
- `prListFields` / `prViewFields` / `issueListFields` / `issueViewFields` の定数
- `listLimit` は `label list` がまだ使うので**残す**。`pr list` / `issue list` が
  消えたぶん doc コメントを直す

- [ ] **Step 5: 消えた経路のテストを外す**

Step 1 で一覧にした「`gh pr list` の引数を主張するテスト」を消す。
**主張ごと消さない** — 同じ主張のうち文書に移ったもの（100 件の天井、並び順）は
Task 1 の `TestTheListDocumentsAskForAHundred` /
`TestTheListDocumentsAskForTheOrderGhAskedFor` が引き取っている。
**引き取り手の無い主張が残っていないかを、消す前に 1 本ずつ確かめる。**

`item.go` を対象にしたテスト（`prJSON.toDomain` など）があれば、それも消す。
同じ変換は `internal/gh/gql/items_test.go` が録りもので確かめている。

**読み手のいなくなる fixture も消す。** `internal/gh/cli/testdata/` の
`pr_list.json` / `pr_view.json` / `issue_list.json` / `issue_view.json` は
`gh ... --json` の応答を録ったもので、消える 4 メソッドのテストしか読んでいない。
**消す前に `grep -rn '<ファイル名>' internal/` で読み手が本当にいないことを
確かめ**、同ディレクトリの `README.md` からもその 4 つの節を消す。
`job_log*.txt` / `pr_files.json` / `own_repos.json` / `search_repos.json` /
`sample.diff` は 4-3 / 4-4 が引き取る経路のものなので**残す**。

- [ ] **Step 6: テストが通ることを確認する**

Run: `make check`
Expected: 全部 PASS、lint 0 issues

**カバレッジが 80% を割っていないこと**（`cli` の行が減るので上がるはず）。

- [ ] **Step 7: 一覧の中身が変わっていないことを実測で確かめる**

**golden では確かめられない。** golden はフェイクを相手に描いた画面なので、
`cli.ListPRs` の中身が `gh pr list` から GraphQL に変わっても 1 ドットも動かない。
確かめるには**実際に両方叩いて突き合わせる**しかない（TTY は要らない）。

```bash
gh pr list --repo cli/cli --limit 100 --json number | jq -c '[.[].number]' > /tmp/a.json
gh api graphql -F query=@internal/gh/gql/repo_prs.graphql \
  -f owner=cli -f name=cli | jq -c '[.data.repository.pullRequests.nodes[].number]' > /tmp/b.json
diff /tmp/a.json /tmp/b.json

gh issue list --repo kukv/octoscope --limit 100 --json number | jq -c '[.[].number]' > /tmp/c.json
gh api graphql -F query=@internal/gh/gql/repo_issues.graphql \
  -f owner=kukv -f name=octoscope | jq -c '[.data.repository.issues.nodes[].number]' > /tmp/d.json
diff /tmp/c.json /tmp/d.json
```

**差分が出たら切り替えを止めて原因を突き止める。** 計画を書いた時点では PR 側を
実測して 56 件が完全一致している（前提 1）。Issue 側はこの Step が初回である。
**結果（件数と一致・不一致）を Task 8 の積み残しに記録すること。**

- [ ] **Step 8: 空振りしていないことを確認する**

- `cli.go` の `RepoName` の委譲を `return "", nil` にする →
  `TestTheListingCallsSendAGraphQLDocument/RepoName` が落ちる
- `cli.repoVars` の placeholder の分岐を消す →
  `TestWithoutARepositoryTheDocumentCarriesGhsPlaceholders` が落ちる

- [ ] **Step 9: コミット**

```bash
git add internal/gh
git commit -m "refactor: list and view items through the shared documents

Both backends now send one set of .graphql documents for these five calls,
which is what schema_test.go checks. gh pr list --json was a second copy of
the same field selection that nothing validated.

The order and the 100-item ceiling are the ones gh asked for, so the Repos
tab shows what it showed before.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: 積み残し文書と設計 §10 の訂正

**Files:**
- Create: `docs/superpowers/2026-09-12-phase4-api-backend-followups.md`
- Modify: `docs/superpowers/specs/2026-09-08-phase4-design.md`（§10 の 4-2 / 4-4 の行）

- [ ] **Step 1: 設計 §10 を直す**

§10 の 4-2 の行から「`main.go` のバックエンド選択と認証エラー画面」を外し、4-4 に
移す。**理由を 1 行添える**（`source` interface が 4-4 まで満たされないため）。
訂正の形は §6 と §4 の前例（2026-09-12 の 2 本の PR）に合わせる。

- [ ] **Step 2: 積み残しを書く**

前 3 本（`2026-09-12-phase4-*-followups.md`）と同じ形で書く。**直さないと決めた
理由も書く。** 最低限、次を含める。

- **繰り越し**: REST 系（4-3）、Actions（4-4）、`main.go` の配線と認証エラー画面（4-4）
- **このスライスで決めたこと**: `git remote get-url origin` を実行する理由
  （worktree では `.git` がファイル）、remote の解決を 1 度だけにした理由、
  `parseRemote` が github.com 以外を拒む理由
- **`cli` から消えたもの**: `item.go`、`prListFields` ほかの定数、それらを見ていたテスト
- **実端末での確認の依頼**（TTY が要るので代行できない）:
  - `gh` を PATH から外し `GH_TOKEN` だけで Work 板・Repos タブ・Search タブが引けること
  - `--repo` を付けない状態で、カレントのリポジトリが Repos タブに出ること
  - git リポジトリでないディレクトリで起動しても落ちず、Repos タブが空で出ること
  - `GH_TOKEN` を空にしたときの挙動（**4-4 まで認証エラー画面は無い**ので、
    この時点では「エラーが出る」までしか確認できない。それも記録する）
- **見つかったが直さなかったこと**（あれば、理由つき）

- [ ] **Step 3: `make check` が緑であることを確認してコミット**

```bash
git add docs
git commit -m "docs: record what the api backend slice left behind

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review（この計画を書いたあとに確かめたこと）

**1. spec の網羅**

| 設計の要求 | 引き取ったタスク |
|---|---|
| §6「GraphQL は文書を共通に置き transport だけ差し替える」 | Task 5（transport）、Task 7（`cli` 側も同じ文書に） |
| §6 訂正 1「`pr list` / `issue list` / `repo view` は新しい `.graphql` 文書」 | Task 1・2・3 |
| §6「認証は `GH_TOKEN` → `GITHUB_TOKEN`」 | Task 4 |
| §6「`OpenWeb` は `internal/browser`」 | Task 4 |
| §6 境界「`github.com` のみ。`GH_HOST` を見ない」 | Task 5（`defaultEndpoint`）、Task 6（`parseRemote`） |
| §7「`cli` と `api` はどちらも同じドメイン型を返す具体型」 | Task 6 Step 6（`parity_test.go`） |
| §8「ネットワークもサブプロセスも叩かない」 | Task 5（`httptest`）、Task 6（`runGit` 差し替え） |
| §8「fixture は実物を録る」 | Task 1・2・3 |
| §8「壊して落ちることを確認してからコミット」 | 各タスクの「空振りしていないことを確認する」 |
| §8「`api` と `cli` に同じ検証を当てる」 | Task 6 Step 6 |
| 完了条件 8「文書が 1 組のまま両方から使われる」 | Task 7 |
| §6「起動時に `gh` を探してバックエンドを選ぶ」「認証エラー画面」 | **4-4 に送った**（承認済み。Task 8 で §10 を直す） |
| §6「REST 系」 | 4-3 |
| §6「`run rerun` / `run view --log`」 | 4-4 |

**2. placeholder の走査**

「適切なエラー処理を足す」「同様に」「TBD」の類が無いことを確認した。
Task 7 Step 1・5 は「既存テストを一覧にしてから消す」という**作業の指示**であり、
中身を書けないのは、今の `cli_test.go` の 795 行のどれが該当するかが
着手時点のコードに依存するため。**一覧を作る手順は具体的に書いてある。**

**3. 型の一貫性**

- `gql.Client.RepoName` は `(ctx, repo)`、`cli.Client` と `api.Client` の
  `RepoName` は `(ctx)`。`internal/tui/app/app.go:30` の `Source` は後者。
  Task 3 で明示し、Task 6・7 の両方で委譲を書いた
- `prNode` / `issueNode` / `toPR` / `toIssue` / `commentNode` は Task 1 で定義し、
  Task 2 がそのまま使う。フィールド名は `items.go` の 1 箇所にしかない
- `gitFunc` は Task 6 で定義し、`api.Client.runGit` の型として Task 6 の中だけで閉じる
- `gh.Classify(nil, msg)` を Task 5 で使う。**実際に走らせて確かめた**（上）

---

## 実行の選択

**Plan complete and saved to `docs/superpowers/plans/2026-09-12-phase4-api-backend.md`.**

1. **Subagent-Driven（推奨）** — タスクごとに新しいサブエージェントを出し、間でレビューする
2. **Inline Execution** — このセッションで `superpowers:executing-plans` に沿って通す
