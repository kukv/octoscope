# Phase 3 checks ビュー 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** PR の checks を一覧し、失敗したジョブのログを読み、ワークフローを再実行できる 2 ペインのビューを足す。

**Architecture:** `internal/gh` の `CheckRun` にジョブ id・run id・ワークフロー名・時刻を足し、`internal/gh/cli` に 1 PR 分の checks を引く GraphQL（`contexts` をページングする）と `gh run view` / `gh run rerun` の呼び出しを置く。`internal/tui/checks` は diff ビューと同じ骨格（左に一覧、右にログ、`mode`/`phase` の enum、`declined` の 1 行）で、`internal/usecase` 越しにだけバックエンドを触る。

**Tech Stack:** Go 1.27.1 / Bubble Tea v2（`charm.land/*/v2`）/ `internal/golden`（`OCTOSCOPE_UPDATE_GOLDEN=1`）/ `gh` CLI 2.100.0

**Spec:** `docs/superpowers/specs/2026-09-07-phase3-design.md`（§2 実測値 / §3 バックエンド / §4 境界 / §5 ページング / §8 完了条件 1・2・3・6・7・8）と `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.4.3（画面はこちらが正）

## Global Constraints

- **`make check` が通らない状態でコミットしない。** 各タスクの最後は必ず `make check`
- **`internal/tui` は `internal/gh/cli` を import しない**（depguard が落とす）。TUI が要る操作はサブモデルが interface で宣言し、`internal/usecase` が実装する
- **テストでネットワークもサブプロセスも叩かない**（`.claude/rules/testing.md`）。`gh` の出力は `testdata` に実物を録って使う
- **テストは状態を直接組み立てず、`Update` にメッセージやキーを渡して到達させる**（`.claude/rules/testing.md`）
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
- **`//nolint` を新しく足さない**（`.claude/rules/go-style.md`）
- **コードのコメントは英語。** 画面に出す文字列は `internal/i18n` から引き、`active.en.yaml` と `active.ja.yaml` の**両方**に足す
- **コメントは書きすぎない。** 外部の事情・そう書いた理由・doc コメントの 3 つだけ残す（`.claude/rules/go-style.md`）
- **`View` は副作用を持たず、時計も読まない**（`.claude/rules/tui.md`）。所要時間の計算に必要な時刻は `Update` が持つ
- **グリフを直接書かない。** `internal/tui/icon` から引く（Nerd Font の私用領域を直書きすると桁がずれる）
- **色を直接書かない。** `internal/tui/theme` の役割名で引く
- Phase 3 の残り（merge、各スレッドの `comments` のページング）をこの計画に入れない

---

## Decisions（実装者はここを勝手に読み替えない。変えたくなったら止めて相談する）

### D1. checks ビューは自分の GraphQL を持つ。`work.graphql` の `contexts` は 100 件のまま

spec §5 は `statusCheckRollup.contexts` のページングを求めている。**それを直すのは
checks ビューの新しいクエリ（`checks.graphql`）で**、Work 板が使う `work.graphql` は
`first: 100` のままにする。

理由: Work 板は 4 列 × 最大 50 件のカードを 1 リクエストで引いており、各カードの
rollup を追加でページングすると**カード 1 枚ごとに往復が増える**。板が要るのは
件数の丸めだけで、101 件目以降の 1 件 1 件ではない。詳細を要るのは checks ビューだけで、
そちらは 1 PR しか見ない。

### D2. `gh.CheckRun` を伸ばす。新しい型を作らない

`CheckRun` は既に「roll-up の後ろにある 1 件の check」であり、ジョブ id も
ワークフロー名も同じものの属性である。別の型を足すと、同じ概念に 2 つの名前が付く。
`work.graphql` はそれらのフィールドを選ばないのでゼロ値のままになるが、
それを読むのは checks ビューだけである。

### D3. ログの整形は `internal/gh/cli` に閉じる

タブ区切りを剥がすのも、BOM を落とすのも、タイムスタンプを切り出すのも cli 側。
TUI が受け取るのは `[]gh.LogLine` で、そこに `gh` の出力の形は残らない（spec §4）。

### D4. 進行中の run のログは、実測してから文言を決める

spec §2 のとおり、進行中の run に `gh run view --log` を投げたときの挙動は測っていない。
**Task 4 で実物を測り**、その結果に合わせて文言を決める。測る前に決め打ちしない。

---

## File Structure

| ファイル | 役割 |
|---|---|
| `internal/gh/gh.go`（修正） | `CheckRun` に `Kind` / `Workflow` / `RunNumber` / `JobID` / `RunID` / `URL` / `StartedAt` / `CompletedAt` を足し、`Duration()` を持たせる |
| `internal/gh/checks.go`（新規） | `CheckKind` / `LogLine` / `RerunScope` |
| `internal/gh/cli/checks.graphql`（新規） | 1 PR の `statusCheckRollup.contexts` を `after` でページングして引く |
| `internal/gh/cli/checks.go`（新規） | `PRChecks` / `JobLog` / `RerunWorkflow` と、`gh run view --log` のパーサ |
| `internal/gh/cli/testdata/`（追加） | `pr_checks.json`（1 ページ目）/ `pr_checks_page2.json` / `job_log.txt` / `job_log_failed.txt`、`schema.json` の録り直し |
| `internal/usecase/usecase.go`（修正） | `checksFetcher` interface と 3 つの委譲メソッド |
| `internal/tui/checks/checks.go`（新規） | `Source` / `Model` / `Update` / キー処理 |
| `internal/tui/checks/render.go`（新規） | 一覧ペイン、ログペイン、ヘッダ、キーバー |
| `internal/tui/checks/rerun.go`（新規) | 再実行のポップアップ |
| `internal/tui/app/app.go`（修正） | `overlayChecks`、`OpenChecksMsg` の受け口、`checks.Source` を `Source` に足す |
| `internal/tui/work/work.go` ほか（修正） | `s` キーで `OpenChecksMsg` を出す（work / repo / detail） |
| `internal/i18n/locales/active.{en,ja}.yaml`（修正） | `checks.*` の文字列 |

---

### Task 1: ドメイン型

**Files:**
- Modify: `internal/gh/gh.go`（`CheckRun` の定義）
- Create: `internal/gh/checks.go`
- Test: `internal/gh/checks_test.go`

**Interfaces:**
- Produces: `gh.CheckKind`（`CheckKindRun` / `CheckKindStatus`）、`gh.CheckRun`（既存 2 フィールド + `Kind CheckKind` / `Workflow string` / `RunNumber int` / `JobID int64` / `RunID int64` / `URL string` / `StartedAt time.Time` / `CompletedAt time.Time`）、`(gh.CheckRun).Duration() time.Duration`、`gh.LogLine{Step string; Time time.Time; Text string}`、`gh.RerunScope`（`RerunFailed` / `RerunAll`）

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/checks_test.go`:

```go
package gh

import (
	"testing"
	"time"
)

func TestDurationIsTheTimeBetweenStartAndFinish(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 7, 5, 44, 0, 0, time.UTC)
	c := CheckRun{StartedAt: start, CompletedAt: start.Add(2*time.Minute + 14*time.Second)}
	if got, want := c.Duration(), 2*time.Minute+14*time.Second; got != want {
		t.Errorf("Duration() = %v, want %v", got, want)
	}
}

func TestARunningCheckHasNoDuration(t *testing.T) {
	t.Parallel()

	c := CheckRun{StartedAt: time.Date(2026, 9, 7, 5, 44, 0, 0, time.UTC)}
	if got := c.Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0 while the check is still running", got)
	}
}

func TestACheckThatNeverStartedHasNoDuration(t *testing.T) {
	t.Parallel()

	if got := (CheckRun{}).Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0", got)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/ -run Duration -v`
Expected: FAIL（`c.Duration undefined`）

- [ ] **Step 3: 型を足す**

`internal/gh/gh.go` の `CheckRun` を差し替える:

```go
// CheckRun is one check behind the roll-up, named as GitHub names it.
//
// Everything past Name and State is filled only by the per-pull-request
// query the checks view runs: the Work board's search selects the roll-up to
// count it, not to act on it.
type CheckRun struct {
	Name  string
	State CheckState
	Kind  CheckKind
	// Workflow and RunNumber name the Actions workflow this check belongs to.
	// A StatusContext belongs to none.
	Workflow  string
	RunNumber int
	// JobID addresses the job's log, RunID the run to rerun. Both are zero
	// for a StatusContext, which has neither.
	JobID int64
	RunID int64
	// URL is where the check reports itself: detailsUrl for a check run,
	// targetUrl for a StatusContext.
	URL         string
	StartedAt   time.Time
	CompletedAt time.Time
}

// Duration is how long the check took, and zero while it is still running.
func (c CheckRun) Duration() time.Duration {
	if c.StartedAt.IsZero() || c.CompletedAt.IsZero() {
		return 0
	}
	return c.CompletedAt.Sub(c.StartedAt)
}
```

`internal/gh/checks.go` を作る:

```go
package gh

import "time"

// CheckKind separates the two shapes GitHub reports a check in. Only a check
// run has a log to read and a workflow to rerun; a StatusContext is an
// external service reporting a state and a link.
type CheckKind int

const (
	CheckKindRun CheckKind = iota
	CheckKindStatus
)

// LogLine is one line of a job's log. Time is zero on a continuation line:
// a step's output can wrap onto lines the runner did not stamp.
type LogLine struct {
	Step string
	Time time.Time
	Text string
}

// RerunScope is how much of a workflow run to start again.
type RerunScope int

const (
	RerunFailed RerunScope = iota
	RerunAll
)
```

（`internal/gh/checks.go` に `time` の import が要る。`gh.go` は既に `time` を import している。）

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/gh/ -run Duration -v`
Expected: PASS（3 本）

- [ ] **Step 5: 空振りしないことを確かめる**

`Duration()` の `if` を消して `go test ./internal/gh/ -run Duration` を走らせ、
2 本目と 3 本目が落ちることを見る。見たら戻す。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/gh/gh.go internal/gh/checks.go internal/gh/checks_test.go
git commit -m "feat: give a check run the ids the checks view acts on"
```

---

### Task 2: 1 PR の checks を引く（`contexts` のページングつき）

**Files:**
- Create: `internal/gh/cli/checks.graphql`, `internal/gh/cli/checks.go`, `internal/gh/cli/checks_test.go`
- Create: `internal/gh/cli/testdata/pr_checks.json`, `internal/gh/cli/testdata/pr_checks_page2.json`
- Modify: `internal/gh/cli/testdata/schema.json`（`CheckSuite` / `WorkflowRun` / `Workflow` を足して録り直す）
- Modify: `internal/gh/cli/schema_test.go`（`checks.graphql` を docs に足す）
- Modify: `internal/gh/cli/testdata/README.md`（録り方を書く）

**Interfaces:**
- Consumes: Task 1 の `gh.CheckRun` / `gh.CheckKind`
- Produces: `func (c *Client) PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error)`

- [ ] **Step 1: クエリを書く**

`internal/gh/cli/checks.graphql`:

```graphql
query ($owner: String!, $name: String!, $number: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              # first is capped at 100 by GitHub, so pageInfo/after page
              # beyond it. The Work board's copy of this selection stays at
              # one page on purpose: it counts the rollup, it does not act
              # on it.
              contexts(first: 100, after: $after) {
                pageInfo {
                  hasNextPage
                  endCursor
                }
                nodes {
                  __typename
                  ... on CheckRun {
                    name
                    status
                    conclusion
                    databaseId
                    detailsUrl
                    startedAt
                    completedAt
                    checkSuite {
                      workflowRun {
                        databaseId
                        runNumber
                        workflow {
                          name
                        }
                      }
                    }
                  }
                  ... on StatusContext {
                    context
                    state
                    targetUrl
                    createdAt
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
```

- [ ] **Step 2: schema.json を録り直し、`checks.graphql` を検証に足す**

`testdata/README.md` の手順の `--argjson types` に `"CheckSuite"`, `"WorkflowRun"`,
`"Workflow"`, `"URI"`, `"DateTime"` を足して録り直す（現在の `schema.json` に
`CheckSuite` / `WorkflowRun` / `Workflow` が無いことを確認済み）。README のリストも
同じに直す。

`schema_test.go` の `docs` マップに 1 行足す:

```go
		"checks.graphql":         checksQuery,
```

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run Schema -v`
Expected: FAIL（`checksQuery` が未定義でコンパイルが通らない）

- [ ] **Step 4: 実レスポンスを録る**

```bash
D=internal/gh/cli/testdata
gh api graphql -F query=@internal/gh/cli/checks.graphql \
  -f owner=kukv -f name=octoscope -F number=61 | jq . > $D/pr_checks.json
```

2 ページ目の fixture（`pr_checks_page2.json`）は、**録った 1 ページ目を写して
`nodes` を 1 件だけ残し、`hasNextPage` を `false` に、`endCursor` を `null` に
書き換えて作る**。1 ページ目のほうは `hasNextPage` を `true`、`endCursor` を
`"CURSOR"` に書き換える。README にそう作ったことを書く（実測を写した加工物であり、
手書きの JSON ではない）。

- [ ] **Step 5: パースとページングのテストを書く**

`internal/gh/cli/checks_test.go`:

```go
package cli

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// twoPages answers the first request with page one and the second with page
// two, and records the arguments of each call.
func twoPages(t *testing.T, calls *[][]string) runFunc {
	t.Helper()

	page1, err := os.ReadFile("testdata/pr_checks.json")
	if err != nil {
		t.Fatal(err)
	}
	page2, err := os.ReadFile("testdata/pr_checks_page2.json")
	if err != nil {
		t.Fatal(err)
	}
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		*calls = append(*calls, args)
		if len(*calls) == 1 {
			return page1, nil
		}
		return page2, nil
	}
}

func TestPRChecksWalksEveryPageOfContexts(t *testing.T) {
	t.Parallel()

	var calls [][]string
	c := &Client{repo: "kukv/octoscope", run: twoPages(t, &calls)}
	checks, err := c.PRChecks(context.Background(), "", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("made %d requests, want 2 (the first page says there is another)", len(calls))
	}
	if !slices.Contains(calls[1], "after=CURSOR") {
		t.Errorf("second request did not carry the cursor: %v", calls[1])
	}
	if checks.Total != 4 {
		t.Errorf("Total = %d, want 4 (three on page one, one on page two)", checks.Total)
	}
}

func TestPRChecksReadsTheIdsTheViewActsOn(t *testing.T) {
	t.Parallel()

	var calls [][]string
	c := &Client{repo: "kukv/octoscope", run: twoPages(t, &calls)}
	checks, err := c.PRChecks(context.Background(), "", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	i := slices.IndexFunc(checks.Runs, func(r gh.CheckRun) bool { return r.Name == "lint" })
	if i < 0 {
		t.Fatalf("no check named lint in %v", checks.Runs)
	}
	got := checks.Runs[i]
	if got.Kind != gh.CheckKindRun {
		t.Errorf("Kind = %v, want CheckKindRun", got.Kind)
	}
	if got.JobID != 101635448466 {
		t.Errorf("JobID = %d, want 101635448466", got.JobID)
	}
	if got.RunID != 34087925535 {
		t.Errorf("RunID = %d, want 34087925535", got.RunID)
	}
	if got.Workflow != "CI" {
		t.Errorf("Workflow = %q, want %q", got.Workflow, "CI")
	}
	if got.Duration() == 0 {
		t.Error("Duration() = 0, want the time between startedAt and completedAt")
	}
}
```

（`lint` の id と所要時間は Step 4 で録った `pr_checks.json` の実値に合わせる。
録った値が上と違ったら**テストのほうを実値に直す**。fixture を書き換えない。）

- [ ] **Step 6: 実装する**

`internal/gh/cli/checks.go`:

```go
package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed checks.graphql
var checksQuery string

type checksResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								Contexts struct {
									PageInfo struct {
										HasNextPage bool   `json:"hasNextPage"`
										EndCursor   string `json:"endCursor"`
									} `json:"pageInfo"`
									Nodes []checkDetailNode `json:"nodes"`
								} `json:"contexts"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// checkDetailNode is the rollup context with the fields the checks view acts
// on. checkNode (graphql.go) stays as it is: the board selects fewer fields.
type checkDetailNode struct {
	checkNode
	DatabaseID  int64     `json:"databaseId"`
	DetailsURL  string    `json:"detailsUrl"`
	TargetURL   string    `json:"targetUrl"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	CreatedAt   time.Time `json:"createdAt"`
	CheckSuite  struct {
		WorkflowRun *struct {
			DatabaseID int64 `json:"databaseId"`
			RunNumber  int   `json:"runNumber"`
			Workflow   struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
}

// PRChecks fetches every check on the pull request's head commit.
func (c *Client) PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error) {
	repoFields, err := repoArgs(c.effectiveRepo(repo))
	if err != nil {
		return gh.Checks{}, err
	}
	var nodes []checkDetailNode
	cursor := ""
	for {
		args := append([]string{"api", "graphql", "-f", "query=" + checksQuery}, repoFields...)
		args = append(args, "-F", "number="+strconv.Itoa(number))
		if cursor != "" {
			args = append(args, "-f", "after="+cursor)
		}
		out, err := c.run(ctx, c.dir, args...)
		if err != nil {
			return gh.Checks{}, err
		}
		var resp checksResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return gh.Checks{}, fmt.Errorf("parse checks: %w", err)
		}
		commits := resp.Data.Repository.PullRequest.Commits.Nodes
		if len(commits) == 0 || commits[0].Commit.StatusCheckRollup == nil {
			return gh.Checks{}, nil
		}
		contexts := commits[0].Commit.StatusCheckRollup.Contexts
		nodes = append(nodes, contexts.Nodes...)
		if !contexts.PageInfo.HasNextPage || contexts.PageInfo.EndCursor == "" {
			break
		}
		cursor = contexts.PageInfo.EndCursor
	}
	return detailRollup(nodes), nil
}

// detailRollup counts the contexts the same way rollup does, and keeps the
// ids the checks view needs alongside each one.
func detailRollup(nodes []checkDetailNode) gh.Checks {
	plain := make([]checkNode, 0, len(nodes))
	for _, n := range nodes {
		plain = append(plain, n.checkNode)
	}
	checks := rollup(plain)
	for i, n := range nodes {
		checks.Runs[i] = n.toRun(checks.Runs[i].State)
	}
	return checks
}

func (n checkDetailNode) toRun(state gh.CheckState) gh.CheckRun {
	run := gh.CheckRun{Name: n.name(), State: state, Kind: gh.CheckKindRun}
	if n.Typename == "StatusContext" {
		run.Kind = gh.CheckKindStatus
		run.URL = n.TargetURL
		run.StartedAt = n.CreatedAt
		return run
	}
	run.URL = n.DetailsURL
	run.JobID = n.DatabaseID
	run.StartedAt = n.StartedAt
	run.CompletedAt = n.CompletedAt
	if wr := n.CheckSuite.WorkflowRun; wr != nil {
		run.RunID = wr.DatabaseID
		run.RunNumber = wr.RunNumber
		run.Workflow = wr.Workflow.Name
	}
	return run
}
```

- [ ] **Step 7: 通ることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'PRChecks|Schema' -v`
Expected: PASS

- [ ] **Step 8: 空振りしないことを確かめる**

`PRChecks` のページングのループを `break` 固定にして
`TestPRChecksWalksEveryPageOfContexts` が落ちること、`toRun` の `run.JobID` の
代入を消して `TestPRChecksReadsTheIdsTheViewActsOn` が落ちることを、それぞれ見る。

- [ ] **Step 9: `make check` とコミット**

```bash
make check
git add internal/gh/cli/ 
git commit -m "feat: fetch one pull request's checks, every page of them"
```

---

### Task 3: ジョブのログを読む

**Files:**
- Modify: `internal/gh/cli/checks.go`
- Create: `internal/gh/cli/testdata/job_log.txt`, `internal/gh/cli/testdata/job_log_failed.txt`
- Modify: `internal/gh/cli/checks_test.go`, `internal/gh/cli/testdata/README.md`

**Interfaces:**
- Produces: `func (c *Client) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error)`

- [ ] **Step 1: 実物を録る**

```bash
D=internal/gh/cli/testdata
gh run view -R kukv/octoscope --job 88970766114 --log-failed > $D/job_log_failed.txt
gh run view -R kukv/octoscope --job 101635448466 --log | head -40 > $D/job_log.txt
```

`job_log.txt` を 40 行に切るのは、パーサのテストに 251 行は要らないから。README に
切ったことを書く。**中身は書き換えない**（先頭行の BOM を含む。それがあることが
このテストの意味の半分である）。

- [ ] **Step 2: 失敗するテストを書く**

```go
func TestJobLogStripsTheJobNameAndTheByteOrderMark(t *testing.T) {
	t.Parallel()

	c := &Client{repo: "kukv/octoscope", run: fileRun(t, "testdata/job_log.txt")}
	lines, err := c.JobLog(context.Background(), "", 101635448466, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	first := lines[0]
	if strings.HasPrefix(first.Text, "﻿") {
		t.Errorf("Text keeps the byte order mark: %q", first.Text)
	}
	if strings.Contains(first.Text, "\t") {
		t.Errorf("Text keeps a tab, so a field was not stripped: %q", first.Text)
	}
	if first.Step == "" {
		t.Error("Step is empty, want the step name the log line carries")
	}
	if first.Time.IsZero() {
		t.Error("Time is zero, want the timestamp the line starts with")
	}
	if !strings.HasPrefix(first.Text, "Current runner version") {
		t.Errorf("Text = %q, want the message with the timestamp taken off", first.Text)
	}
}

func TestALineWithoutATimestampKeepsItsText(t *testing.T) {
	t.Parallel()

	c := &Client{repo: "kukv/octoscope", run: fileRun(t, "testdata/job_log_failed.txt")}
	lines, err := c.JobLog(context.Background(), "", 88970766114, true)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	i := slices.IndexFunc(lines, func(l gh.LogLine) bool { return l.Time.IsZero() })
	if i < 0 {
		t.Skip("the recording has no continuation line")
	}
	if lines[i].Text == "" {
		t.Error("a continuation line lost its text")
	}
}

func TestJobLogAsksForOnlyTheFailedStepsWhenToldTo(t *testing.T) {
	t.Parallel()

	var got []string
	c := &Client{repo: "kukv/octoscope", run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}}
	if _, err := c.JobLog(context.Background(), "", 42, true); err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if !slices.Contains(got, "--log-failed") {
		t.Errorf("args = %v, want --log-failed", got)
	}
	if slices.Contains(got, "--log") {
		t.Errorf("args = %v, want no --log alongside --log-failed", got)
	}
}

func TestAJobThatDidNotFailReturnsNoLines(t *testing.T) {
	t.Parallel()

	c := &Client{repo: "kukv/octoscope", run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, nil // gh exits 0 with no output when no step failed
	}}
	lines, err := c.JobLog(context.Background(), "", 42, true)
	if err != nil {
		t.Fatalf("JobLog: %v, want no error: an empty log is not a failure", err)
	}
	if len(lines) != 0 {
		t.Errorf("lines = %v, want none", lines)
	}
}
```

`fileRun` はこのファイルに足すヘルパー:

```go
// fileRun answers every call with the contents of one recording.
func fileRun(t *testing.T, path string) runFunc {
	t.Helper()

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context, string, ...string) ([]byte, error) { return out, nil }
}
```

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run JobLog -v`
Expected: FAIL（`c.JobLog undefined`）

- [ ] **Step 4: 実装する**

`internal/gh/cli/checks.go` に足す:

```go
// JobLog reads one job's log. With failedOnly it asks for the failed steps
// alone: that is what someone opening a red check came to read, and a whole
// job's log has been measured at nine times the length.
//
// gh exits zero with no output at all when a successful job is asked for its
// failed steps, so an empty result is an answer, not an error.
func (c *Client) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error) {
	args := []string{"run", "view", "--job", strconv.FormatInt(jobID, 10)}
	if failedOnly {
		args = append(args, "--log-failed")
	} else {
		args = append(args, "--log")
	}
	out, err := c.run(ctx, c.dir, appendRepo(args, c.effectiveRepo(repo))...)
	if err != nil {
		return nil, err
	}
	return parseJobLog(string(out)), nil
}

// parseJobLog reads the format gh prints: three tab-separated fields, the
// job's name, the step's name, and the message, whose first word is an
// RFC3339 timestamp on every line the runner stamped. The very first byte of
// the output is a byte order mark.
func parseJobLog(out string) []gh.LogLine {
	var lines []gh.LogLine
	for _, raw := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if raw == "" {
			continue
		}
		fields := strings.SplitN(strings.TrimPrefix(raw, "﻿"), "\t", 3)
		if len(fields) < 3 {
			lines = append(lines, gh.LogLine{Text: fields[len(fields)-1]})
			continue
		}
		line := gh.LogLine{Step: fields[1], Text: fields[2]}
		if stamp, rest, ok := strings.Cut(fields[2], " "); ok {
			if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
				line.Time, line.Text = at, rest
			}
		}
		lines = append(lines, line)
	}
	return lines
}
```

（`strings` の import を足す。）

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'JobLog|Timestamp|DidNotFail' -v`
Expected: PASS

- [ ] **Step 6: 空振りしないことを確かめる**

`strings.TrimPrefix(raw, "﻿")` を `raw` に戻して BOM のテストが落ちること、
`failedOnly` の分岐を潰して引数のテストが落ちることを見る。

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/gh/cli/
git commit -m "feat: read a job's log, failed steps first"
```

---

### Task 4: 再実行と、進行中の run の実測

**Files:**
- Modify: `internal/gh/cli/checks.go`, `internal/gh/cli/checks_test.go`
- Modify: `docs/superpowers/specs/2026-09-07-phase3-design.md`（§2 の「まだ測っていない」行を実測に置き換える）

**Interfaces:**
- Produces: `func (c *Client) RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error`

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestRerunAsksForOnlyTheFailedJobsWhenScopedThatWay(t *testing.T) {
	t.Parallel()

	var got []string
	c := &Client{repo: "kukv/octoscope", run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}}
	if err := c.RerunWorkflow(context.Background(), "", 34087925535, gh.RerunFailed); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	want := []string{"run", "rerun", "34087925535", "--failed", "--repo", "kukv/octoscope"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestRerunOfTheWholeRunPassesNoFailedFlag(t *testing.T) {
	t.Parallel()

	var got []string
	c := &Client{repo: "kukv/octoscope", run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}}
	if err := c.RerunWorkflow(context.Background(), "", 34087925535, gh.RerunAll); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	if slices.Contains(got, "--failed") {
		t.Errorf("args = %v, want no --failed", got)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run Rerun -v`
Expected: FAIL（`c.RerunWorkflow undefined`）

- [ ] **Step 3: 実装する**

```go
// RerunWorkflow starts a workflow run again. GraphQL has no mutation for
// this, which is why it goes through the subcommand.
func (c *Client) RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error {
	args := []string{"run", "rerun", strconv.FormatInt(runID, 10)}
	if scope == gh.RerunFailed {
		args = append(args, "--failed")
	}
	_, err := c.run(ctx, c.dir, appendRepo(args, c.effectiveRepo(repo))...)
	return err
}
```

- [ ] **Step 4: 通ることと空振りしないことを確かめる**

Run: `go test ./internal/gh/cli/ -run Rerun -v` → PASS。
`--failed` の分岐を潰して 1 本目が落ちることを見る。

- [ ] **Step 5: 進行中の run のログを実測する**

**これは実物への 1 回の観測であり、コードではない。** 進行中の run を 1 つ作り
（このリポジトリに空コミットを push するのが最も安い）、走っている間に:

```bash
gh run view -R kukv/octoscope --job <走っているジョブの id> --log-failed; echo "exit=$?"
gh run view -R kukv/octoscope --job <同じ id> --log; echo "exit=$?"
```

見るもの: 終了コード、stderr の**正確な文言**、標準出力の有無。

- [ ] **Step 6: 測った結果を spec に書き、必要なら扱いを足す**

`docs/superpowers/specs/2026-09-07-phase3-design.md` §2 の
「進行中の run に `gh run view --log` を投げたときの挙動は**まだ測っていない**」の段落を、
測った事実に置き換える（日付つき）。

- **エラーで返るなら**: `JobLog` はそのエラーをそのまま返す。TUI 側（Task 7）が
  ログ欄にその文言を出す。stderr の文言を `testdata/job_log_in_progress.txt` に録り、
  「そのエラーが素通しで返る」ことのテストを 1 本足す
- **空で返るなら**: 追加のコードは要らない。§2 にそう書くだけにする

**測る前にどちらかを決め打ちして実装しない。**

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/gh/cli/ docs/superpowers/specs/2026-09-07-phase3-design.md
git commit -m "feat: rerun a workflow, all of it or only what failed"
```

---

### Task 5: usecase の口

**Files:**
- Modify: `internal/usecase/usecase.go`
- Test: `internal/usecase/usecase_test.go`

**Interfaces:**
- Consumes: Task 2〜4 の `PRChecks` / `JobLog` / `RerunWorkflow`
- Produces: `(*usecase.Usecase).PRChecks` / `.JobLog` / `.RerunWorkflow`（cli と同じシグネチャ）

- [ ] **Step 1: 失敗するテストを書く**

`internal/usecase/usecase_test.go` の既存の fake に 3 メソッドを足し、委譲のテストを書く。
既存のテストがどう fake を組んでいるかをまず読む（`usecase_test.go`）。書くテストは:

```go
func TestPRChecksReachesTheBackend(t *testing.T) {
	t.Parallel()

	src := &fakeSource{checks: gh.Checks{Total: 3}}
	u := New(src)
	got, err := u.PRChecks(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	if got.Total != 3 {
		t.Errorf("Total = %d, want 3", got.Total)
	}
	if src.checksRepo != "kukv/octoscope" || src.checksNumber != 61 {
		t.Errorf("backend was asked for %s#%d, want kukv/octoscope#61", src.checksRepo, src.checksNumber)
	}
}
```

`JobLog` と `RerunWorkflow` にも同じ形で 1 本ずつ（`RerunWorkflow` は
`gh.RerunAll` を渡し、fake が受け取った scope を確かめる）。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/usecase/ -run 'PRChecks|JobLog|Rerun' -v`
Expected: FAIL（コンパイルが通らない）

- [ ] **Step 3: 実装する**

`internal/usecase/usecase.go` に足す:

```go
type checksFetcher interface {
	PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error)
	JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error)
	RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error
}
```

`source` interface に `checksFetcher` を足し、`Usecase` に `checks checksFetcher`
フィールドと `New` の `checks: src` を足す。委譲メソッド:

```go
func (u *Usecase) PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error) {
	return u.checks.PRChecks(ctx, repo, number)
}

func (u *Usecase) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error) {
	return u.checks.JobLog(ctx, repo, jobID, failedOnly)
}

func (u *Usecase) RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error {
	return u.checks.RerunWorkflow(ctx, repo, runID, scope)
}
```

- [ ] **Step 4: 通ることを確かめ、空振りしないことを見る**

Run: `go test ./internal/usecase/ -run 'PRChecks|JobLog|Rerun' -v` → PASS。
`PRChecks` の委譲で `repo` を捨てて `""` を渡すようにし、テストが落ちることを見る。

- [ ] **Step 5: `make check` とコミット**

```bash
make check
git add internal/usecase/
git commit -m "feat: let a view ask for checks, logs, and reruns"
```

---

### Task 6: checks ビューの一覧ペイン

**Files:**
- Create: `internal/tui/checks/checks.go`, `internal/tui/checks/render.go`, `internal/tui/checks/checks_test.go`
- Modify: `internal/i18n/locales/active.en.yaml`, `internal/i18n/locales/active.ja.yaml`

**Interfaces:**
- Consumes: Task 5 の 3 メソッド
- Produces: `checks.Source`（`PRChecks` / `JobLog` / `RerunWorkflow` / `OpenWeb`）、`checks.New(src Source, ref gh.ItemRef) Model`、`checks.Model`（`Init` / `Update(tea.Msg) (Model, tea.Cmd)` / `View() string`）、`checks.ClosedMsg`、`checks.ErrorMsg`

- [ ] **Step 1: 文字列をカタログに足す**

`active.en.yaml`:

```yaml
checks:
  title:
    other: "Checks"
  none:
    other: "no checks on this pull request"
  loading:
    other: "loading checks..."
  summary:
    other: "{{.Failing}} failing · {{.Running}} running"
  decline_status_context:
    other: "this check reports from outside GitHub Actions; press o to open it"
  decline_loading:
    other: "still loading the checks; try again in a moment"
```

`active.ja.yaml` に同じキーで:

```yaml
checks:
  title:
    other: "Checks"
  none:
    other: "この PR には check がありません"
  loading:
    other: "checks を読み込み中..."
  summary:
    other: "失敗 {{.Failing}} · 実行中 {{.Running}}"
  decline_status_context:
    other: "この check は GitHub Actions の外から報告されています。o で開けます"
  decline_loading:
    other: "checks を読み込み中です。少し待ってからもう一度"
```

- [ ] **Step 2: 失敗するテストを書く**

`internal/tui/checks/checks_test.go`:

```go
package checks

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	checks gh.Checks
	log    []gh.LogLine
}

func (f *fakeSource) PRChecks(context.Context, string, int) (gh.Checks, error) {
	return f.checks, nil
}

func (f *fakeSource) JobLog(context.Context, string, int64, bool) ([]gh.LogLine, error) {
	return f.log, nil
}

func (f *fakeSource) RerunWorkflow(context.Context, string, int64, gh.RerunScope) error { return nil }

func (f *fakeSource) OpenWeb(string) error { return nil }

// fixture is two workflows, the failing one recorded second on purpose: the
// view has to move it to the top.
func fixture() gh.Checks {
	return gh.Checks{
		Total: 3, Passed: 1, Failed: 1, Running: 1, State: gh.CheckFailure,
		Runs: []gh.CheckRun{
			{Name: "lint", State: gh.CheckSuccess, Kind: gh.CheckKindRun, Workflow: "CI", JobID: 1, RunID: 10},
			{Name: "sca", State: gh.CheckFailure, Kind: gh.CheckKindRun, Workflow: "security", JobID: 2, RunID: 20},
			{Name: "ci/circleci", State: gh.CheckRunning, Kind: gh.CheckKindStatus, URL: "https://circleci.example/1"},
		},
	}
}

func open(t *testing.T, width int) Model {
	t.Helper()

	m := New(&fakeSource{checks: fixture()}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	return m
}

func TestTheFailingWorkflowComesFirst(t *testing.T) {
	t.Parallel()

	view := open(t, 120).View()
	failing, passing := strings.Index(view, "sca"), strings.Index(view, "lint")
	if failing < 0 || passing < 0 {
		t.Fatalf("both checks should be drawn:\n%s", view)
	}
	if failing > passing {
		t.Errorf("the failing check is drawn below the passing one:\n%s", view)
	}
}

func TestTheHeaderCountsWhatIsWrong(t *testing.T) {
	t.Parallel()

	if view := open(t, 120).View(); !strings.Contains(view, "1 failing") {
		t.Errorf("header does not count the failures:\n%s", view)
	}
}

func TestEscLeavesTheView(t *testing.T) {
	t.Parallel()

	_, cmd := open(t, 120).Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc produced no command, want ClosedMsg")
	}
	if _, ok := cmd().(ClosedMsg); !ok {
		t.Errorf("esc sent %T, want ClosedMsg", cmd())
	}
}
```

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/tui/checks/ -v`
Expected: FAIL（パッケージが無い）

- [ ] **Step 4: 実装する**

`internal/tui/checks/checks.go` を書く。骨格は `internal/tui/diff/diff.go` に倣う:

```go
// Package checks shows one pull request's checks: what ran down the left,
// the log of the selected one on the right.
package checks

import (
	"context"
	"sort"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// Source is what the checks view needs. repo is "owner/repo"; the empty
// string targets the workspace repository.
type Source interface {
	PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error)
	JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error)
	RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error
	OpenWeb(url string) error
}

// ClosedMsg tells the parent the user left the checks view.
type ClosedMsg struct{}

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type checksMsg struct {
	ref    gh.ItemRef
	checks gh.Checks
}

type errMsg struct {
	ref gh.ItemRef
	err error
}

type Model struct {
	src Source
	ref gh.ItemRef

	width, height int

	loading bool
	spin    spinner.Model

	checks gh.Checks
	// order is the checks as drawn: failing workflows first, and the checks
	// of one workflow together. It is rebuilt when the checks arrive rather
	// than on every draw, because View may do no work of its own.
	order []gh.CheckRun
	row   int
	top   int

	declined string
	errText  string
}
```

`New` / `Init` / `fetch` / `Update` / `handleKey` / `checksArrived` は diff の同名の
ものと同じ形にする。`checksArrived` が `order` を組む:

```go
// arrange puts the failing workflows at the top and keeps each workflow's
// checks together. A StatusContext belongs to no workflow and sorts last: it
// is the one the view can do the least with.
func arrange(runs []gh.CheckRun) []gh.CheckRun {
	out := append([]gh.CheckRun(nil), runs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Kind == gh.CheckKindStatus) != (b.Kind == gh.CheckKindStatus) {
			return b.Kind == gh.CheckKindStatus
		}
		if a.Workflow != b.Workflow {
			return worstOf(runs, a.Workflow) > worstOf(runs, b.Workflow)
		}
		return false
	})
	return out
}

// worstOf ranks a workflow by the worst state any of its checks is in, so a
// workflow with one red check outranks a workflow that is merely running.
func worstOf(runs []gh.CheckRun, workflow string) int {
	worst := 0
	for _, r := range runs {
		if r.Workflow != workflow {
			continue
		}
		worst = max(worst, rank(r.State))
	}
	return worst
}

func rank(s gh.CheckState) int {
	switch s {
	case gh.CheckFailure:
		return 3
	case gh.CheckRunning, gh.CheckPending:
		return 2
	default:
		return 1
	}
}
```

`render.go` は左ペイン（ワークフロー名の見出し + 各 check の行）とヘッダだけをまず描く。
右ペインは Task 7。グリフは `icon.Check(state)` から引き、色は `theme` から引く
（`internal/tui/work/render.go` の check の描き方をそのまま参考にする）。

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/tui/checks/ -v`
Expected: PASS（3 本）

- [ ] **Step 6: 空振りしないことを確かめる**

`checksArrived` の `arrange` を素通しにして 1 本目が落ちること、ヘッダの
`summary` を消して 2 本目が落ちることを見る。

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/tui/checks/ internal/i18n/
git commit -m "feat: list a pull request's checks, the failing ones first"
```

---

### Task 7: ログペイン

**Files:**
- Modify: `internal/tui/checks/checks.go`, `internal/tui/checks/render.go`, `internal/tui/checks/checks_test.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`

**Interfaces:**
- Produces: `enter` / `L` / `h` / `l` / `o` のキー処理と、`Model.log []gh.LogLine`

- [ ] **Step 1: 文字列を足す**

両カタログに:

```yaml
  log_loading:
    other: "loading the log..."          # ja: "ログを読み込み中..."
  log_empty_failed:
    other: "no step of this job failed"  # ja: "このジョブに失敗したステップはありません"
  log_full:
    other: "full log"                    # ja: "全ログ"
  log_failed_only:
    other: "failed steps"                # ja: "失敗ステップ"
```

- [ ] **Step 2: 失敗するテストを書く**

```go
func TestEnterFetchesTheLogOfTheSelectedCheck(t *testing.T) {
	t.Parallel()

	src := &fakeSource{checks: fixture(), log: []gh.LogLine{{Step: "Run tests", Text: "FAIL ./internal/gh"}}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("enter started no fetch")
	}
	m, _ = m.Update(cmd())
	if view := m.View(); !strings.Contains(view, "FAIL ./internal/gh") {
		t.Errorf("the log is not on screen:\n%s", view)
	}
}

func TestASucceededJobSaysNoStepFailed(t *testing.T) {
	t.Parallel()

	src := &fakeSource{checks: fixture()} // JobLog answers with no lines
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	if view := m.View(); !strings.Contains(view, i18n.T("checks.log_empty_failed")) {
		t.Errorf("an empty failed-step log said nothing:\n%s", view)
	}
}

func TestAStatusContextSaysWhyItHasNoLog(t *testing.T) {
	t.Parallel()

	m := open(t, 120)
	m = moveTo(t, m, "ci/circleci")
	m, _ = m.Update(keyPress("enter"))
	if view := m.View(); !strings.Contains(view, i18n.T("checks.decline_status_context")) {
		t.Errorf("enter on a StatusContext did nothing and said nothing:\n%s", view)
	}
}

func TestTheLogDoesNotWrap(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 400)
	src := &fakeSource{checks: fixture(), log: []gh.LogLine{{Step: "s", Text: long}}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 80 {
			t.Fatalf("line %d is %d columns wide, want at most 80:\n%s", i, w, line)
		}
	}
}
```

`moveTo` はこのファイルに足すヘルパー（`j` を押しながら目的の行に着くまで進む。
着かなければ `t.Fatalf`）。**「押しても着かないヘルパー」は空振りテストの元なので、
着けなかったら必ず失敗させる**（`.claude/rules/testing.md`）。

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/tui/checks/ -run 'Log|StatusContext' -v`
Expected: FAIL

- [ ] **Step 4: 実装する**

- `Model` に `log []gh.LogLine` / `logRow int` / `hscroll int` / `failedOnly bool`（既定 `true`）/ `logPhase`（`phaseIdle` / `phaseLoading`）を足す
- `enter`: 選択中が `CheckKindStatus` なら `m.declined = i18n.T("checks.decline_status_context")`、
  そうでなければ `fetchLog(jobID, m.failedOnly)` を返す。checks がまだ届いていないときは
  `checks.decline_loading`
- `L`: `failedOnly` を反転して取り直す。`CheckKindStatus` では `enter` と同じ理由を出す
- `h` / `l`: `hscroll` を動かす。左端で `h` を押したら一覧ペインへ戻る（diff の `sidebar` と同じ）
- `o`: `OpenWeb(選択中の URL)`
- ログの描画は**折り返さない**。`hscroll` から幅ぶんを切り出す。切り出しは
  `internal/tui/layout` の既存の関数を使う（`diff` の行の切り方を読んで同じにする）
- 取得したログが 0 行で `failedOnly` なら `checks.log_empty_failed` を出す

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/tui/checks/ -v` → PASS

- [ ] **Step 6: 空振りしないことを確かめる**

`decline_status_context` の代入を消して 3 本目が落ちること、ログの切り出しを
やめて 4 本目が落ちることを見る。

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/tui/checks/ internal/i18n/
git commit -m "feat: read the failed steps of a check without leaving the terminal"
```

---

### Task 8: 再実行のポップアップ

**Files:**
- Create: `internal/tui/checks/rerun.go`, `internal/tui/checks/rerun_test.go`
- Modify: `internal/tui/checks/checks.go`, `internal/tui/checks/render.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`

**Interfaces:**
- Produces: `mode`（`modeView` / `modeRerun`）、`R` キーの処理、`rerunDoneMsg` / `rerunErrMsg`

- [ ] **Step 1: 文字列を足す**

両カタログに:

```yaml
  rerun_title:
    other: "Rerun {{.Workflow}}"         # ja: "{{.Workflow}} を再実行"
  rerun_failed_only:
    other: "only the failed jobs"        # ja: "失敗したジョブのみ"
  rerun_all:
    other: "the whole workflow"          # ja: "ワークフロー全体"
  rerun_started:
    other: "rerun requested; press r to see it"  # ja: "再実行を要求しました。r で取り直せます"
  decline_rerun_status_context:
    other: "only GitHub Actions checks can be rerun from here"
    # ja: "ここから再実行できるのは GitHub Actions の check だけです"
```

- [ ] **Step 2: 失敗するテストを書く**

```go
func TestRerunAsksWhichScope(t *testing.T) {
	t.Parallel()

	m := press(open(t, 120), "R")
	if view := m.View(); !strings.Contains(view, i18n.T("checks.rerun_failed_only")) ||
		!strings.Contains(view, i18n.T("checks.rerun_all")) {
		t.Errorf("R did not offer both scopes:\n%s", view)
	}
}

func TestChoosingAScopeSendsIt(t *testing.T) {
	t.Parallel()

	src := &recordingSource{fakeSource: fakeSource{checks: fixture()}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m = press(m, "R")
	m = press(m, "j") // move from "only the failed jobs" to "the whole workflow"
	_, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}
	cmd()
	if src.scope != gh.RerunAll {
		t.Errorf("scope = %v, want RerunAll", src.scope)
	}
	if src.runID != 10 {
		t.Errorf("runID = %d, want the run of the selected check", src.runID)
	}
}

func TestEscLeavesTheRerunPopupWithoutSending(t *testing.T) {
	t.Parallel()

	src := &recordingSource{fakeSource: fakeSource{checks: fixture()}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m = press(press(m, "R"), "esc")
	if src.reruns != 0 {
		t.Errorf("esc sent %d reruns, want 0", src.reruns)
	}
	if view := m.View(); strings.Contains(view, i18n.T("checks.rerun_all")) {
		t.Errorf("the popup is still up after esc:\n%s", view)
	}
}

func TestRerunOnAStatusContextSaysWhyItCannot(t *testing.T) {
	t.Parallel()

	m := press(moveTo(t, open(t, 120), "ci/circleci"), "R")
	if view := m.View(); !strings.Contains(view, i18n.T("checks.decline_rerun_status_context")) {
		t.Errorf("R on a StatusContext said nothing:\n%s", view)
	}
}
```

`recordingSource` は `fakeSource` を埋め込み、`RerunWorkflow` の引数を控える。

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/tui/checks/ -run Rerun -v`
Expected: FAIL

- [ ] **Step 4: 実装する**

`internal/tui/diff` の `mode` / `phase` の書き方に揃える（`String()` も持たせる。
失敗メッセージが `mode = 1` にならないため）。ポップアップは 2 行の選択リストで、
`j` / `k` で選び、`enter` で送り、`esc` で閉じる。送信中は `phaseWorking`。
成功したら `checks.rerun_started` を 1 行出し、ポップアップを閉じる。
**再実行しても一覧は自動で取り直さない**（`r` は利用者が押す。spec §4.4.3 の
「更新は手動のみ」に従う）。

- [ ] **Step 5: 通ることを確かめ、空振りしないことを見る**

Run: `go test ./internal/tui/checks/ -v` → PASS。
`enter` の送信を潰して 2 本目が落ちること、`esc` の分岐を潰して 3 本目が落ちることを見る。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/tui/checks/ internal/i18n/
git commit -m "feat: rerun a workflow from the checks view"
```

---

### Task 9: `s` で開く配線

**Files:**
- Modify: `internal/tui/app/app.go`, `internal/tui/app/app_test.go`
- Modify: `internal/tui/work/work.go`, `internal/tui/repo/repo.go`, `internal/tui/detail/detail.go`（+ 各 `_test.go`）
- Modify: `README.md`, `README.ja.md`（キー表があれば追随）

**Interfaces:**
- Produces: `work.OpenChecksMsg` / `repo.OpenChecksMsg` / `detail.OpenChecksMsg`（いずれも `struct{ Ref gh.ItemRef }`）、`app` の `overlayChecks`

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/work/work_test.go` に、`d` の既存テストと同じ形で:

```go
func TestSOpensTheChecksOfTheSelectedPullRequest(t *testing.T) {
	t.Parallel()

	m := loadedBoard(t) // the board's existing helper
	_, cmd := m.Update(keyPress("s"))
	if cmd == nil {
		t.Fatal("s produced no command")
	}
	msg, ok := cmd().(OpenChecksMsg)
	if !ok {
		t.Fatalf("s sent %T, want OpenChecksMsg", cmd())
	}
	if msg.Ref.Kind != gh.ItemPR {
		t.Errorf("Ref.Kind = %v, want ItemPR", msg.Ref.Kind)
	}
}

func TestSDoesNothingOnAnIssue(t *testing.T) {
	t.Parallel()

	m := moveToIssue(t, loadedBoard(t))
	if _, cmd := m.Update(keyPress("s")); cmd != nil {
		t.Errorf("s on an issue sent %T, want nothing: an issue has no checks", cmd())
	}
}
```

`repo` と `detail` にも同じ形で 1 本ずつ。`app_test.go` には
「`work.OpenChecksMsg` を受けると checks ビューが画面に出る」「`esc` で
タブに戻る」の 2 本。既存の `OpenDiffMsg` のテストを読んで同じ形にする。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/... -run Checks -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

- `work` / `repo` / `detail` に `OpenChecksMsg` を足し、`s` のキー処理を
  既存の `d` の隣に置く（PR 以外では何もしない、も `d` と同じ）
- `app.Source` に `checks.Source` を足す
- `overlay` に `overlayChecks` を足し、`openChecks` / `startChecks` を
  `openDiff` / `startDiff` と同じ形で書く。詳細ビューから開いたときは
  `openDiffOverDetail` と同じくスタックに積む
- `broadcast` に checks モデルを足す（他のビューと同じく、離れたあとに
  届く応答を捨てないため）
- `checks.ErrorMsg` は `diffFailed` と同じ形で、スタックに無ければ捨てる

- [ ] **Step 4: 通ることを確かめ、空振りしないことを見る**

Run: `make test` → PASS。
`s` の分岐を消して work のテストが落ちること、`app` の
`case work.OpenChecksMsg` を消して app のテストが落ちることを見る。

- [ ] **Step 5: README のキー表に `s` を足す**

`README.md` と `README.ja.md` に diff の `d` を載せた表があれば、同じ場所に
`s`（checks）を足す。無ければ何もしない。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/tui/ README.md README.ja.md
git commit -m "feat: open the checks of a pull request with s"
```

---

### Task 10: golden と、実端末での確認

**Files:**
- Create: `internal/tui/checks/golden_test.go`, `internal/tui/checks/testdata/*.golden`
- Create: `docs/superpowers/2026-09-07-phase3-checks-handoff.md`

- [ ] **Step 1: golden テストを書く**

`internal/tui/diff/golden_test.go` をそのまま参考にする。録る状態は 5 つ:

| 名前 | 何の状態か |
|---|---|
| `checks_<lang>_<w>` | 一覧だけ（ログ未取得） |
| `checks_log_<lang>_<w>` | ログが出ている |
| `checks_rerun_<lang>_<w>` | 再実行のポップアップ |
| `checks_loading_<lang>_<w>` | 取得中（スピナー） |
| `checks_none_<lang>_<w>` | check が 1 つも無い PR |

fixture には**日本語のログ行を 1 本入れる**（全角で桁が 2 つ要る。それが
golden で桁ずれを捕まえる唯一の手段である）。`goldenWidths` は `{160, 120, 80}`、
言語は en / ja。

- [ ] **Step 2: 録って、目で見る**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/checks/
cat -v internal/tui/checks/testdata/checks_ja_80.golden
cat -v internal/tui/checks/testdata/checks_log_ja_80.golden
```

**80 桁の ja で、キーバーと本文が 80 桁を超えていないことを目で確かめる。**
超えていたら詰める。録っただけで済ませない。

- [ ] **Step 3: 幅のテストを 1 本足す**

```go
func TestNothingOverrunsTheTerminal(t *testing.T) {
	t.Parallel()

	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			for i, line := range strings.Split(goldenModel(w).View(), "\n") {
				if got := ansi.StringWidth(line); got > w {
					t.Errorf("%s %d: line %d is %d columns:\n%s", lang.name, w, i, got, line)
				}
			}
		}
	}
}
```

- [ ] **Step 4: `make check` とコミット**

```bash
make check
git add internal/tui/checks/
git commit -m "test: record what the checks view draws"
```

- [ ] **Step 5: 受け渡しの文書を書く**

`docs/superpowers/2026-09-07-phase3-checks-handoff.md` に、
`2026-09-06-phase2-handoff.md` と同じ形で書く:

- **この環境には TTY が無いので、実端末での確認は代行できない**
- 手順: 失敗した check のある PR を開き、`s` → `enter` でログ、`L` で全ログ、
  `h` / `l` で横スクロール、`R` → 失敗のみ で再実行、GitHub の Web UI で
  再実行が始まっていることを確かめる
- 外部 CI（StatusContext）のある PR で `enter` / `R` を押し、理由が出ることを確かめる
- `--lang ja` で 80 桁のときに桁がずれないこと
- 進行中の run で `enter` を押したときの見え方（Task 4 Step 5 で測った挙動が
  実際にそう見えるか）
- 報告してほしいこと: 桁がずれたら端末とフォントと `--icons`、
  ログが読めない形で出たらその PR と check の名前

- [ ] **Step 6: コミットと PR**

```bash
make check
git add docs/superpowers/
git commit -m "docs: hand off what only a real terminal can check"
git log --oneline main..HEAD
```

PR の説明には、D1（Work 板の `contexts` は 100 件のままにした理由）と
Task 4 Step 5 で測った進行中の run の挙動を書く。末尾に:

```
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## Self-Review

**spec のカバレッジ:**

| spec | この計画 |
|---|---|
| §4.4.3 一覧・並べ替え・ヘッダ | Task 6 |
| §4.4.3 ログ（既定は失敗ステップ、`L` で全体、折り返さない、空出力の扱い） | Task 3 / Task 7 |
| §4.4.3 StatusContext は `o` だけ、`L` / `R` は理由を出す | Task 7 / Task 8 |
| §4.4.3 再実行（失敗のみ / 全体） | Task 4 / Task 8 |
| §4.4.3 更新は `r` の手動のみ | Task 6（`r`）/ Task 8（再実行後に自動で取り直さない） |
| §4.4.3 入口は `s` | Task 9 |
| Phase 3 spec §2 の実測値 | Task 2 / Task 3 の fixture、Task 4 Step 5 で残る 1 件を測る |
| Phase 3 spec §3 ログと再実行は `gh run` | Task 3 / Task 4 |
| Phase 3 spec §4 境界 | Task 5（usecase 越し）/ Task 6（`Source` は tui 側で宣言） |
| Phase 3 spec §5 `contexts` のページング | Task 2（+ D1 で Work 板は対象外と決めた） |
| Phase 3 spec §6 テスト | 各タスクの「空振りしないことを確かめる」/ Task 10 |
| Phase 3 spec §8 完了条件 1・2・3・6・7・8 | Task 6 / 7 / 8 / 2 / 5 / 10 |

**この計画が扱わないもの:** merge（§4.4.4）、各スレッドの `comments` の
ページング、Work 板の search `first: 50`、マウス操作（spec §4.4.3 に
マウスの記述が無い。要ると分かってから足す）。
