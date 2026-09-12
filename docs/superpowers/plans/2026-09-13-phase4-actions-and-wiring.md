# Actions とバックエンド配線 実装計画（Phase 4 スライス 4-4）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `source` interface の最後の 2 本（`JobLog` / `RerunWorkflow`）を
`internal/gh/api` に実装し、`cmd/octoscope` が `gh` の有無でバックエンドを選び、
どちらも使えないときに認証を促して終わる状態にする。あわせて `api` の HTTP に
タイムアウトを入れ、役目を終えた `parity_test.go` を消す。

**Architecture:** `JobLog` は `gh run view --job <id> --log[-failed]` が実際に叩く
順序をそのままなぞる — ジョブを 1 件引き、完了していなければ `gh` と同じ一文で断り、
run のログ zip を取って `<ジョブ名>/<番号>_*.txt` をステップごとに拾う。zip に
ステップが無ければ top-level のジョブ全体ログ、それも無ければ
`actions/jobs/{id}/logs` の平文へ落ちる。行の整形（制御文字の無害化・BOM 除去・
先頭の RFC3339 タイムスタンプ切り出し）は `cli` が `gh` の出力から取り出しているものと
同じ結果になる必要があるので、後半 2 つを `internal/gh` の共有ヘルパーに出して
両バックエンドが使う。タイムアウトは `api.Client` が持つ `*http.Client` 1 つに集約し、
本文のダウンロードではなく「サーバが応答を返さない」ことだけを縛る。

**Tech Stack:** Go 1.27.1、標準ライブラリのみ（`archive/zip` / `net/http` /
`regexp` / `unicode/utf16`）。**依存は 1 つも足さない。**

**Spec:** `docs/superpowers/specs/2026-09-08-phase4-design.md`（§6・§10・§11）と
`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §3.2。
Actions の実測値は `docs/superpowers/specs/2026-09-07-phase3-design.md` §2。
前スライスの積み残しは `docs/superpowers/2026-09-13-phase4-rest-backend-followups.md`。

---

## Global Constraints

設計と `.claude/rules/` から、このスライスの全タスクに掛かるもの。

- **ネットワークも外部プロセスも実際には叩かない**（`.claude/rules/testing.md`）。
  HTTP は `httptest.Server` に向ける。継ぎ目は `Client.baseURL` の 1 つだけで、
  これを増やさない。ヘルパーは既存の `serveREST`（`internal/gh/api/rest_test.go:17`）
- **パーステストの入力は実際に録ったレスポンス**。`internal/gh/api/testdata/` に置き、
  録り方・録った日・対象リポジトリを同ディレクトリの `README.md` に書く。
  ただし zip は録りものをバイナリで置くのではなく、**テスト内で `archive/zip` で
  組む**（中身の平文は録ったものを使う）。バイナリの fixture は読み手が中身を
  確かめられない
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
  （設計 §8）
- **実装が組み立てた引数をコピーした期待値を書かない**（`.claude/rules/testing.md`）。
  テスト名は「なぜその引数が要るのか」が読める形にする
- **`internal/gh` は `internal/tui` も `internal/usecase` も import しない**
  （`.golangci.yml` の `gh-layer`）。`api` は `cli` を import しない
- **GitHub API 固有の文字列はこの層で止める。** `"completed"` / `"failure"` /
  `"startup_failure"` を外に出さない
- **エラーは 3 種類に分ける**（`.claude/rules/errors.md`）。センチネルは `errors.Is`
  で分岐を実際に書くものだけ。この層に `internal/i18n` を入れない
- **コメントは英語。** 外部の事情・一見おかしいコードが正しい理由・doc コメントの
  3 つだけ。**実装計画や設計書への参照をコードに書かない**（`.claude/rules/go-style.md`）
- **`context.Context` は取得系の第一引数。** 書き込み系は `source` interface に
  `ctx` が無いので `cli` と同じく `context.Background()` を使う
- 各タスクの最後に `make check` が緑であること。カバレッジの基準は 80%

---

## 着手前に確かめたこと（2026-09-13）

`gh` の trunk のソースを読んで確かめた。推測ではない。

| 確かめたこと | 結果（出典） |
|---|---|
| `gh run view --job <id> --log` が叩く順序 | `GET actions/jobs/{id}` →（未完了なら断る）→ `GET actions/runs/{run_id}/logs` の zip → zip 内のファイルをジョブ・ステップに対応づけ → 1 行ずつ `ジョブ名 \t ステップ名 \t 本文` で出す（`pkg/cmd/run/view/view.go:307-343`） |
| zip の構造 | `<ジョブ名>/<ステップ番号>_<ステップ名>.txt` と、top-level の `<序数>_<ジョブ名>.txt`。古い Actions サービス由来で `-<負の数>_<ジョブ名>.txt` のこともある（`pkg/cmd/run/view/logs.go` の `getZipLogMap` の doc） |
| ステップのログが 1 つでも zip にあれば | **ジョブ全体のログは使わず、ステップ単位で出す。** 1 つも無ければジョブ全体のログ 1 件（`populateLogSegments`） |
| zip にジョブ全体のログも無いとき | `GET actions/jobs/{id}/logs` の平文へ落ちる（`apiLogFetcher`） |
| zip 内のジョブ名 | サーバ側が加工している。`gh` は `/` と `:` を除去し、**UTF-16 のコードユニットで 90 に切り詰め**、前後の空白を落とす（`getJobNameForLogFilename` / `truncateAsUTF16`） |
| ステップ名が引けないとき `gh` が出す文字列 | `UNKNOWN STEP`（`displayLogSegments`） |
| 未完了のジョブに対する文言 | `job %d is still in progress; logs will be available when it is complete`（`view.go:309`）。Phase 3 設計 §2 の実測（stderr の文言）と一致する |
| `--log-failed` が「失敗」と見なす conclusion | `action_required` / `failure` / `startup_failure` / `timed_out`（`pkg/cmd/run/shared/shared.go:318`）。`skipped` のジョブはそもそも飛ばす |
| `gh` が本文に掛けている加工 | `asciisanitizer`。C0/C1 制御文字を caret 表記（`\x1b` → `^[`）に置換する。**`\t` `\n` `\v` `\r` は素通し**（`cli/go-gh` の `pkg/asciisanitizer/sanitizer.go`）。つまり `cli` バックエンドが受け取る本文に生の ANSI エスケープは無い |
| `cli` の `gh` 実行にタイムアウトはあるか | **無い**（`internal/gh/cli/cli.go:103` は `exec.CommandContext` に呼び出し側の ctx をそのまま渡すだけ）。だから `api` に入れるタイムアウトは「`cli` と揃える」ためのものではなく、`api` だけが持つ安全策である |

---

## 利用者が決めたこと（2026-09-13）

計画を分岐させる 4 点。いずれも承認済み。

1. **`JobLog` は `gh` と同じ zip 方式**（フォールバック 2 段を含む）。平文
   エンドポイントだけで済ませる案は採らない。`Step` が全行で空になり、
   `--log-failed` がステップ単位で効かず、`cli` と画面が変わるため
2. **タイムアウトは transport 側の応答待ち**（`Dial` / `TLSHandshake` /
   `ResponseHeaderTimeout`）。`http.Client.Timeout` は本文のダウンロードまで
   縛るので、run のログ zip を途中で切る恐れがある
3. **認証エラー画面は `tea.NewProgram` の前に stderr へ出して `exit 1`。**
   alt screen に入る前なので文言がそのまま残る
4. **行のパースは `internal/gh` の共有ヘルパーに出す。** `cli` 側には
   `JobLog` の**出力**を主張するテストを残す（4-3 の「移動で守られなくなる」
   落とし穴への対策）

---

## このスライスでやらないこと（理由つき）

- **`gh` を PATH から外した手動確認は 4-5。** TTY と実在のリポジトリが要る
- **ラベルの並び順を GraphQL で厳密化する件**は繰り越しのまま。Actions とは無関係
- **`parseRemote` の明示ポート付き `ssh://`** も繰り越しのまま。`parseRemote` を
  触るタスクがこのスライスに無い
- **ラベル/担当者の編集で追加だけ通る件**も繰り越し。UI 側の再読み込みと一緒に決める
- **zip のキャッシュ**は持たない。`gh` は `run-log-<id>-<時刻>.zip` をディスクに
  キャッシュするが、octoscope は同じジョブのログを繰り返し開く道具ではなく、
  キャッシュの寿命と置き場所という新しい決め事が要る

---

## ファイル構成

| ファイル | 責務 |
|---|---|
| `internal/gh/checks.go`（修正） | `LogLine` に加え、1 行の本文から `LogLine` を組む共有ヘルパー |
| `internal/gh/checks_test.go`（修正） | そのヘルパーのテスト |
| `internal/gh/cli/checks.go`（修正） | `parseJobLog` が共有ヘルパーを呼ぶ形に |
| `internal/gh/api/checks.go`（新規） | `RerunWorkflow` と `JobLog` の本体。GitHub の JSON から `gh.LogLine` まで |
| `internal/gh/api/joblog.go`（新規） | zip をジョブ・ステップに対応づける部分と、制御文字の無害化 |
| `internal/gh/api/checks_test.go`（新規） | `RerunWorkflow` / `JobLog` のテスト |
| `internal/gh/api/joblog_test.go`（新規） | 名前の対応づけと無害化のテスト |
| `internal/gh/api/api.go`（修正） | `*http.Client` を 1 つ持つ |
| `internal/gh/api/transport.go`（修正） | `post` がその Client を使う。タイムアウトの値と理由 |
| `internal/gh/api/rest.go`（修正） | `send` がその Client を使う |
| `internal/gh/api/parity_test.go`（削除） | `usecase.New(api.New(...))` が通った時点で役目が終わる |
| `cmd/octoscope/main.go`（修正） | バックエンド選択と、どちらも無いときの終了 |
| `cmd/octoscope/backend.go`（新規） | 選択そのもの。`main` から切り出してテストできる形に |
| `cmd/octoscope/backend_test.go`（新規） | 選択のテスト |
| `internal/i18n/locales/active.en.yaml` / `active.ja.yaml`（修正） | 認証エラーの文言 |

---

## Task 1: 1 行から `LogLine` を組む共有ヘルパー

**Files:**
- Modify: `internal/gh/checks.go`
- Modify: `internal/gh/cli/checks.go:32-57`（`parseJobLog`）
- Test: `internal/gh/checks_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `func gh.NewLogLine(step, message string) LogLine`

**Interfaces（詳細）:** `message` は `gh` が `ジョブ名 \t ステップ名 \t` の後ろに
置いている部分、つまり runner が書いた 1 行そのもの。BOM の除去と、先頭の
RFC3339Nano タイムスタンプの切り出しをここで行う。

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/checks_test.go` に足す。**このファイルは `package gh`** なので
`NewLogLine` は修飾せずに呼ぶ。

```go
// The runner stamps most lines with an RFC3339 timestamp and a space, and the
// very first line of a log carries a byte order mark before it. Both belong to
// the transport, not to what the step printed, so neither reaches the screen.
func TestALogLineKeepsTheStampOutOfTheText(t *testing.T) {
	t.Parallel()

	line := NewLogLine("Run tests", "\ufeff2026-09-07T09:15:22.1234567Z ok  \tgithub.com/kukv/octoscope\t0.4s")

	if line.Step != "Run tests" {
		t.Errorf("Step = %q, want %q", line.Step, "Run tests")
	}
	want := time.Date(2026, 9, 7, 9, 15, 22, 123456700, time.UTC)
	if !line.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", line.Time, want)
	}
	if line.Text != "ok  \tgithub.com/kukv/octoscope\t0.4s" {
		t.Errorf("Text = %q", line.Text)
	}
}

// A step's output can wrap onto lines the runner did not stamp. Those keep
// their whole text: cutting at the first space would eat a word.
func TestAnUnstampedLineKeepsItsWholeText(t *testing.T) {
	t.Parallel()

	line := NewLogLine("Run tests", "    expected 3, got 4")

	if !line.Time.IsZero() {
		t.Errorf("Time = %v, want the zero time", line.Time)
	}
	if line.Text != "    expected 3, got 4" {
		t.Errorf("Text = %q", line.Text)
	}
}

// A line whose first word merely looks like it could be a stamp is not one.
// Cutting it off would drop a word the step actually printed.
func TestAFirstWordThatIsNotATimestampStays(t *testing.T) {
	t.Parallel()

	line := NewLogLine("Run tests", "2026-09-07 09:15:22 starting")

	if !line.Time.IsZero() {
		t.Errorf("Time = %v, want the zero time", line.Time)
	}
	if line.Text != "2026-09-07 09:15:22 starting" {
		t.Errorf("Text = %q", line.Text)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/ -run TestALogLine -v`
Expected: FAIL（`undefined: NewLogLine` でビルドが通らない）

- [ ] **Step 3: 実装する**

`internal/gh/checks.go` の `LogLine` の下に足す。

```go
// NewLogLine reads one line of a job's log. message is what the runner wrote:
// the stamp it prefixes most lines with, and on the very first line of a log a
// byte order mark before that. Neither is part of what the step printed.
func NewLogLine(step, message string) LogLine {
	message = strings.TrimPrefix(message, "\ufeff")
	line := LogLine{Step: step, Text: message}
	if stamp, rest, ok := strings.Cut(message, " "); ok {
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			line.Time, line.Text = at, rest
		}
	}
	return line
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/gh/ -run 'TestALogLine|TestAnUnstamped|TestAFirstWord' -v`
Expected: PASS

- [ ] **Step 5: `cli` を共有ヘルパーに載せ替える**

`internal/gh/cli/checks.go` の `parseJobLog` の本体を書き換える。doc コメントの
「BOM は行頭ではなくメッセージの先頭に付く」という説明は `NewLogLine` 側に移った
ので、ここには `gh` が出すタブ区切りの形の説明だけを残す。

```go
// parseJobLog reads the format gh prints: three tab-separated fields, the
// job's name, the step's name, and the line the runner wrote.
func parseJobLog(out string) []gh.LogLine {
	var lines []gh.LogLine
	for _, raw := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if raw == "" {
			continue
		}
		fields := strings.SplitN(raw, "\t", 3)
		if len(fields) < 3 {
			lines = append(lines, gh.LogLine{Text: fields[len(fields)-1]})
			continue
		}
		lines = append(lines, gh.NewLogLine(fields[1], fields[2]))
	}
	return lines
}
```

`time` の import が `internal/gh/cli/checks.go` で未使用になるなら消す
（自分の変更で不要になったものは消す）。

- [ ] **Step 6: `cli` 側のテストが今も出力を主張していることを確かめる**

Run: `go test ./internal/gh/cli/ -run TestJobLog -v`
Expected: PASS。

**そのうえで、`internal/gh/checks.go` の `NewLogLine` から
`strings.TrimPrefix(message, "\ufeff")` の行を一時的に消して
`go test ./internal/gh/cli/ -run TestJobLogStripsTheJobNameAndTheByteOrderMark`
を走らせ、落ちることを確かめる。** 落ちなければ `cli` のテストは移動によって
主張を失っている。落ちることを確かめたら行を戻す。

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/gh/checks.go internal/gh/checks_test.go internal/gh/cli/checks.go
git commit -m "refactor: share one log line's parsing between both backends"
```

---

## Task 2: `RerunWorkflow`

**Files:**
- Create: `internal/gh/api/checks.go`
- Test: `internal/gh/api/checks_test.go`

**Interfaces:**
- Consumes: `(*Client).write`（`internal/gh/api/rest.go`）、`(*Client).repoPath`
- Produces: `func (c *Client) RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error`

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/api/checks_test.go` を作る。

```go
package api

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// Rerunning everything and rerunning only what failed are two endpoints, not
// one endpoint with a flag. Sending the whole run to the failed-jobs path
// would start jobs that already passed.
func TestRerunScopePicksTheEndpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		scope gh.RerunScope
		want  string
	}{
		{"all", gh.RerunAll, "/repos/kukv/octoscope/actions/runs/61/rerun"},
		{"failed", gh.RerunFailed, "/repos/kukv/octoscope/actions/runs/61/rerun-failed-jobs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{}`)
			})
			if err := c.RerunWorkflow(context.Background(), "kukv/octoscope", 61, tc.scope); err != nil {
				t.Fatalf("RerunWorkflow: %v", err)
			}

			req := (*got)[0]
			if req.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", req.Method)
			}
			if req.URL.Path != tc.want {
				t.Errorf("path = %q, want %q", req.URL.Path, tc.want)
			}
		})
	}
}

// A rerun is not repeated when GitHub's front end fails to answer: a 502 says
// no answer came back, not that nothing happened, and a second POST would
// start the run twice.
func TestARerunIsNotRepeatedOnATransientFailure(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"message": "Server Error"}`)
	})
	if err := c.RerunWorkflow(context.Background(), "kukv/octoscope", 61, gh.RerunAll); err == nil {
		t.Fatal("RerunWorkflow: want an error")
	}
	if len(*got) != 1 {
		t.Errorf("requests = %d, want 1: a rerun must not be sent twice", len(*got))
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/api/ -run TestRerun -v`
Expected: FAIL（`c.RerunWorkflow` undefined）

- [ ] **Step 3: 実装する**

`internal/gh/api/checks.go` を作る。

```go
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kukv/octoscope/internal/gh"
)

// RerunWorkflow starts a workflow run again. GitHub has two endpoints rather
// than one with a flag: the failed-jobs one leaves the jobs that passed alone.
func (c *Client) RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error {
	r, err := c.repoPath(repo)
	if err != nil {
		return err
	}
	endpoint := "rerun"
	if scope == gh.RerunFailed {
		endpoint = "rerun-failed-jobs"
	}
	path := fmt.Sprintf("repos/%s/actions/runs/%d/%s", r, runID, endpoint)
	_, err = c.write(ctx, http.MethodPost, path, nil)
	return err
}
```

`write` の第 4 引数が `nil` のとき `send` は `Content-Type` を付けず本文も送らない。
GitHub の rerun はどちらも本文を要求しない。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/gh/api/ -run TestRerun -v`
Expected: PASS

- [ ] **Step 5: `make check` とコミット**

```bash
make check
git add internal/gh/api/checks.go internal/gh/api/checks_test.go
git commit -m "feat: rerun a workflow run through the REST API"
```

---

## Task 3: ジョブを 1 件引き、未完了と「失敗が無い」を先に断る

**Files:**
- Modify: `internal/gh/api/checks.go`
- Test: `internal/gh/api/checks_test.go`
- Create: `internal/gh/api/testdata/job.json`
- Modify: `internal/gh/api/testdata/README.md`

**Interfaces:**
- Consumes: `(*Client).read`、`(*Client).repoPath`
- Produces: `func (c *Client) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error)`（このタスクではログ本体をまだ取らず、
  ジョブの取得と 2 つの早期リターンだけを持つ）と、内部型
  `type job struct { ID, RunID int64; Name, Status, Conclusion string; Steps []jobStep }`、
  `type jobStep struct { Name string; Number int; Conclusion string }`

- [ ] **Step 1: fixture を録る**

`internal/gh/api/testdata/job.json` に、実在の完了済みジョブ 1 件の応答を置く。

```bash
gh api repos/kukv/octoscope/actions/jobs/<JOB_ID> > internal/gh/api/testdata/job.json
```

**私有情報を入れない。** `kukv/octoscope` は公開リポジトリなので応答をそのまま
置いてよい。`internal/gh/api/testdata/README.md` に、録ったコマンド・日付・
対象（リポジトリとジョブ id）を 1 行足す。

- [ ] **Step 2: 失敗するテストを書く**

```go
// gh refuses a job that has not finished, and the TUI shows GitHub's sentence
// as it is. A different wording here would put a different sentence on the
// screen depending on which backend is running.
func TestAnInProgressJobIsRefusedInGhsOwnWords(t *testing.T) {
	t.Parallel()

	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": 61, "run_id": 7, "name": "build", "status": "in_progress"}`)
	})

	_, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err == nil {
		t.Fatal("JobLog: want an error for a job that has not finished")
	}
	want := "job 61 is still in progress; logs will be available when it is complete"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

// A job that passed has no failed steps to show. gh prints nothing and exits
// zero, and the cli backend already treats an empty log as an answer; asking
// GitHub for the run's whole log archive here would be a download nobody reads.
func TestFailedOnlyOnAPassingJobAnswersWithNothing(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w,
			`{"id": 61, "run_id": 7, "name": "build", "status": "completed", "conclusion": "success"}`)
	})

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, true)
	if err != nil {
		t.Fatalf("JobLog: %v, want no error: an empty log is not a failure", err)
	}
	if len(lines) != 0 {
		t.Errorf("lines = %d, want none", len(lines))
	}
	if len(*got) != 1 {
		t.Errorf("requests = %d, want 1: the log archive must not be fetched", len(*got))
	}
}

// A job GitHub marks action_required or timed_out failed too. Treating only
// "failure" as a failure would show an empty log for a job whose red mark is
// exactly what the user clicked.
func TestTheOtherFailedConclusionsCountAsFailed(t *testing.T) {
	t.Parallel()

	for _, conclusion := range []string{"failure", "startup_failure", "timed_out", "action_required"} {
		t.Run(conclusion, func(t *testing.T) {
			t.Parallel()

			var calls int
			c, _ := serveREST(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.HasSuffix(r.URL.Path, "/logs") {
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"message": "Not Found"}`)
					return
				}
				_, _ = io.WriteString(w, `{"id": 61, "run_id": 7, "name": "build", "status": "completed",
					"conclusion": "`+conclusion+`"}`)
			})

			// The log fetch is expected to fail here: what this test checks is
			// that JobLog went looking for it at all.
			_, _ = c.JobLog(context.Background(), "kukv/octoscope", 61, true)
			if calls < 2 {
				t.Errorf("requests = %d, want the log to be fetched for a %s job", calls, conclusion)
			}
		})
	}
}
```

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/gh/api/ -run 'TestAnInProgress|TestFailedOnly|TestTheOtherFailed' -v`
Expected: FAIL（`c.JobLog` undefined）

- [ ] **Step 4: 実装する**

`internal/gh/api/checks.go` に足す。

```go
// job is one Actions job. Only the fields the log needs are decoded: which run
// carries the archive, what the zip's directory is named after, and which
// steps failed.
type job struct {
	ID         int64     `json:"id"`
	RunID      int64     `json:"run_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Steps      []jobStep `json:"steps"`
}

type jobStep struct {
	Name       string `json:"name"`
	Number     int    `json:"number"`
	Conclusion string `json:"conclusion"`
}

// failed is what GitHub reports for work that did not succeed. action_required
// and timed_out are failures the same way an outright failure is: each one is
// a red mark someone clicked to read.
func failed(conclusion string) bool {
	switch conclusion {
	case "failure", "startup_failure", "timed_out", "action_required":
		return true
	}
	return false
}

// JobLog reads one job's log. With failedOnly it answers with the failed steps
// alone: that is what someone opening a red check came to read.
//
// A job that has not finished has no log yet, and a passing job has no failed
// steps: neither is an error, and the second answers with no lines at all.
func (c *Client) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return nil, err
	}
	body, err := c.read(ctx, fmt.Sprintf("repos/%s/actions/jobs/%d", r, jobID), "")
	if err != nil {
		return nil, err
	}
	var j job
	if err := json.Unmarshal(body, &j); err != nil {
		return nil, fmt.Errorf("read job %d: %w", jobID, err)
	}
	if j.Status != "completed" {
		return nil, fmt.Errorf("job %d is still in progress; logs will be available when it is complete", jobID)
	}
	if failedOnly && !failed(j.Conclusion) {
		return nil, nil
	}
	return c.jobLogLines(ctx, r, j, failedOnly) // Task 4
}
```

このタスクの時点では `jobLogLines` をまだ書いていないので、Task 4 まで
コンパイルが通らない。**Task 3 と Task 4 は 1 つのコミットにまとめる**
（途中でコンパイルできない状態をコミットしない）。Task 3 の Step 5 は無く、
Task 4 の最後にまとめてコミットする。

---

## Task 4: zip からステップごとのログを取り出す

**Files:**
- Create: `internal/gh/api/joblog.go`
- Modify: `internal/gh/api/checks.go`
- Test: `internal/gh/api/joblog_test.go`, `internal/gh/api/checks_test.go`

**Interfaces:**
- Consumes: Task 3 の `job` / `jobStep` / `failed`、`(*Client).read`
- Produces:
  - `func (c *Client) jobLogLines(ctx context.Context, repo string, j job, failedOnly bool) ([]gh.LogLine, error)`
  - `func logFileName(jobName string) string`（zip の中でジョブが名乗る名前）
  - `func sanitizeControls(line string) string`

- [ ] **Step 1: 名前の対応づけの失敗するテストを書く**

`internal/gh/api/joblog_test.go` を作る。

```go
package api

import (
	"strings"
	"testing"
)

// The server that builds the zip strips characters it cannot put in a path.
// A composite action's job is named "<job> / <action>", so a job whose name
// has a slash would never match its own directory unless the slash goes.
func TestASlashInAJobNameIsNotInItsZipEntry(t *testing.T) {
	t.Parallel()

	if got := logFileName("build / test"); got != "build  test" {
		t.Errorf("logFileName = %q, want %q", got, "build  test")
	}
}

// A colon goes the same way: Windows cannot have one in a path.
func TestAColonInAJobNameIsNotInItsZipEntry(t *testing.T) {
	t.Parallel()

	if got := logFileName("build: linux"); got != "build linux" {
		t.Errorf("logFileName = %q, want %q", got, "build linux")
	}
}

// The server truncates at 90 UTF-16 code units, not 90 bytes and not 90 runes.
// A name of multi-byte characters is cut in a place neither of the other two
// counts would pick, and a name cut in the wrong place matches nothing.
func TestALongJobNameIsCutWhereTheServerCutsIt(t *testing.T) {
	t.Parallel()

	// Each of these is one rune, three bytes, and one UTF-16 code unit.
	name := strings.Repeat("あ", 100)
	got := logFileName(name)
	if got != strings.Repeat("あ", 90) {
		t.Errorf("logFileName cut to %d characters, want 90", len([]rune(got)))
	}
}

// An emoji is two UTF-16 code units, so 50 of them already exceed the limit
// even though they are 50 runes. Counting runes would leave the name uncut and
// matching nothing.
func TestAnEmojiCountsTwiceTowardTheLimit(t *testing.T) {
	t.Parallel()

	name := strings.Repeat("😅", 50)
	got := logFileName(name)
	if len([]rune(got)) != 45 {
		t.Errorf("logFileName kept %d runes, want 45", len([]rune(got)))
	}
}

// The runner writes ANSI colour codes into the log. Handing an escape
// character to the screen lets the log repaint the terminal, so every C0 and
// C1 control turns into caret notation -- which is what gh hands the cli
// backend, so both backends show the same text.
func TestAnEscapeInTheLogCannotReachTheScreen(t *testing.T) {
	t.Parallel()

	if got := sanitizeControls("\x1b[31mFAIL\x1b[0m"); got != "^[[31mFAIL^[[0m" {
		t.Errorf("sanitizeControls = %q", got)
	}
}

// The single-byte form of the same escape is a different rune with the same
// effect on a terminal. Mapping it by the arithmetic the C0 range uses would
// hand the raw byte straight back.
func TestTheSingleByteEscapeIsNeutralisedToo(t *testing.T) {
	t.Parallel()

	if got := sanitizeControls("\u009b[31mFAIL"); got != "^[[31mFAIL" {
		t.Errorf("sanitizeControls = %q", got)
	}
}

// Delete is a control character Go names as one, and gh does not rewrite it.
// Rewriting it here would put a "^" on a screen the cli backend leaves clean.
func TestDeleteIsLeftAloneTheWayGhLeavesIt(t *testing.T) {
	t.Parallel()

	if got := sanitizeControls("a\u007fb"); got != "a\u007fb" {
		t.Errorf("sanitizeControls = %q", got)
	}
}

// A tab is how the runner lines up test output. Turning it into caret notation
// would break every table in a log.
func TestATabSurvivesSanitizing(t *testing.T) {
	t.Parallel()

	if got := sanitizeControls("ok\tgithub.com/kukv/octoscope\t0.4s"); got != "ok\tgithub.com/kukv/octoscope\t0.4s" {
		t.Errorf("sanitizeControls = %q", got)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/api/ -run 'TestASlash|TestAColon|TestALongJob|TestAnEmoji|TestAnEscape|TestATab' -v`
Expected: FAIL（`logFileName` / `sanitizeControls` undefined）

- [ ] **Step 3: 名前と無害化を実装する**

`internal/gh/api/joblog.go` を作る。

```go
package api

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/kukv/octoscope/internal/gh"
)

// jobNameLimit is how many UTF-16 code units of a job's name the server keeps
// when it builds the zip. The limit is there to stay under a zip path length
// limit, and it counts the way the server's runtime counts: a character
// outside the basic plane takes two.
const jobNameLimit = 90

// logFileName is what a job is called inside the log archive. The server drops
// the characters it cannot put in a path and cuts the rest short, so a job's
// own name does not find its entry.
func logFileName(jobName string) string {
	name := strings.ReplaceAll(jobName, "/", "")
	name = strings.ReplaceAll(name, ":", "")
	if units := utf16.Encode([]rune(name)); len(units) > jobNameLimit {
		name = string(utf16.Decode(units[:jobNameLimit]))
	}
	return strings.TrimSpace(name)
}

// sanitizeControls replaces the control characters a log line may carry with
// caret notation. A log is output from somewhere else that ends up on this
// terminal, and an escape character in it would be acted on rather than shown.
// Tab, newline, vertical tab and carriage return are left alone: they are how
// the runner lays output out, and so is delete, which gh leaves alone too.
func sanitizeControls(line string) string {
	var b strings.Builder
	for _, r := range line {
		if c, ok := caret(r); ok {
			b.WriteString(c)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// caret is a control character written the way a terminal shows one: ^ and the
// letter it is made from. C1 characters (0x80-0x9f) show as the C0 characters
// they mirror. Anything else is returned untouched, delete (0x7f) included:
// it is a control character Go names as one and gh does not rewrite.
func caret(r rune) (string, bool) {
	switch {
	case r == '\t' || r == '\n' || r == '\v' || r == '\r':
		return "", false
	case r >= 0x00 && r <= 0x1f:
		return "^" + string('@'+r), true
	case r >= 0x80 && r <= 0x9f:
		return "^" + string('@'+r-0x80), true
	}
	return "", false
}
```

`caret` の検算: `\x1b`（27）→ `'@'`（64）+ 27 = 91 = `'['` なので `^[`。
`\x00` → `'@'` そのままで `^@`。`\x9b`（155）→ 155-128 = 27 → `'@'`+27 = `^[`。
`\x7f` は上の 2 つの範囲のどちらにも入らないので素通し。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/gh/api/ -run 'TestASlash|TestAColon|TestALongJob|TestAnEmoji|TestAnEscape|TestATab' -v`
Expected: PASS

- [ ] **Step 5: zip からの取り出しの失敗するテストを書く**

`internal/gh/api/checks_test.go` に足す。zip はテスト内で組む。

```go
// zipOf builds a log archive the way GitHub serves one, so the test's
// expectations are readable: a binary fixture would hide what is in it.
func zipOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := io.WriteString(f, body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// serveJobLog answers the two requests JobLog makes: the job, then the run's
// log archive.
func serveJobLog(t *testing.T, j string, archive []byte) (*Client, *[]*http.Request) {
	t.Helper()

	return serveREST(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logs") {
			_, _ = w.Write(archive)
			return
		}
		_, _ = io.WriteString(w, j)
	})
}

const twoStepJob = `{"id": 61, "run_id": 7, "name": "build", "status": "completed",
	"conclusion": "failure", "steps": [
		{"name": "Set up job", "number": 1, "conclusion": "success"},
		{"name": "Run tests", "number": 2, "conclusion": "failure"}]}`

// A step's name is not in the log's text: it is in the name of the file the
// step's output was written to. Reading the log without the entry names would
// leave every line unattributed.
func TestEachLineCarriesTheStepItCameFrom(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "2026-09-07T09:15:20.0000000Z starting\n",
		"build/2_Run tests.txt":  "2026-09-07T09:15:22.0000000Z FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if lines[0].Step != "Set up job" || lines[1].Step != "Run tests" {
		t.Errorf("steps = %q, %q", lines[0].Step, lines[1].Step)
	}
	if lines[0].Text != "starting" || lines[1].Text != "FAIL" {
		t.Errorf("text = %q, %q", lines[0].Text, lines[1].Text)
	}
}

// The steps come out in the order they ran. A zip's entries are in whatever
// order the archive was written in, and a log read out of order is unreadable.
func TestStepsComeOutInTheOrderTheyRan(t *testing.T) {
	t.Parallel()

	// Ten steps, so that a sort by name would put "10_" before "2_".
	steps := make([]string, 0, 10)
	entries := map[string]string{}
	for i := 1; i <= 10; i++ {
		steps = append(steps, fmt.Sprintf(`{"name": "step %d", "number": %d, "conclusion": "success"}`, i, i))
		entries[fmt.Sprintf("build/%d_step %d.txt", i, i)] = fmt.Sprintf("line %d\n", i)
	}
	j := fmt.Sprintf(`{"id": 61, "run_id": 7, "name": "build", "status": "completed",
		"conclusion": "failure", "steps": [%s]}`, strings.Join(steps, ","))

	c, _ := serveJobLog(t, j, zipOf(t, entries))
	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	for i, line := range lines {
		if want := fmt.Sprintf("line %d", i+1); line.Text != want {
			t.Fatalf("lines[%d] = %q, want %q", i, line.Text, want)
		}
	}
}

// failedOnly narrows to the steps that failed, not to the jobs that failed:
// a failed job's passing steps are the part nobody opened the log to read.
func TestFailedOnlyKeepsOnlyTheStepsThatFailed(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "2026-09-07T09:15:20.0000000Z starting\n",
		"build/2_Run tests.txt":  "2026-09-07T09:15:22.0000000Z FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, true)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Step != "Run tests" {
		t.Fatalf("lines = %+v, want only the failed step", lines)
	}
}

// Some runs have no per-step entries at all, only one file for the whole job.
// Without this fallback those jobs would show an empty log.
func TestAJobWithNoStepEntriesFallsBackToItsWholeLog(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"0_build.txt": "2026-09-07T09:15:20.0000000Z starting\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "starting" {
		t.Fatalf("lines = %+v", lines)
	}
}

// The older Actions service names the whole-job file after the job's id, which
// is negative. A pattern that only allowed digits would miss it and show an
// empty log.
func TestTheLegacyWholeJobEntryIsFoundToo(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"-2147483648_build.txt": "2026-09-07T09:15:20.0000000Z starting\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "starting" {
		t.Fatalf("lines = %+v", lines)
	}
}

// A run whose archive has expired, or a job whose entry the name matching
// could not find, still has its own log endpoint. Answering with nothing would
// look like a job that printed nothing.
func TestAnArchiveWithoutTheJobFallsBackToTheJobsOwnLog(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/actions/runs/7/logs"):
			_, _ = w.Write(zipOf(t, map[string]string{"0_other job.txt": "not this one\n"}))
		case strings.HasSuffix(r.URL.Path, "/actions/jobs/61/logs"):
			_, _ = io.WriteString(w, "2026-09-07T09:15:20.0000000Z starting\n")
		default:
			_, _ = io.WriteString(w, twoStepJob)
		}
	})

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "starting" {
		t.Fatalf("lines = %+v", lines)
	}
	if len(*got) != 3 {
		t.Errorf("requests = %d, want 3", len(*got))
	}
}

// A job whose name has a slash is stored under a directory without it. Looking
// for the job's own name would find nothing and fall all the way through to
// the single-job endpoint, losing every step name on the way.
func TestAJobNamedAfterACompositeActionFindsItsEntries(t *testing.T) {
	t.Parallel()

	j := `{"id": 61, "run_id": 7, "name": "build / test", "status": "completed",
		"conclusion": "failure", "steps": [{"name": "Run tests", "number": 1, "conclusion": "failure"}]}`
	c, _ := serveJobLog(t, j, zipOf(t, map[string]string{
		"build  test/1_Run tests.txt": "2026-09-07T09:15:22.0000000Z FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Step != "Run tests" {
		t.Fatalf("lines = %+v", lines)
	}
}
```

- [ ] **Step 6: 落ちることを確かめる**

Run: `go test ./internal/gh/api/ -run 'TestEachLine|TestStepsCome|TestFailedOnlyKeeps|TestAJobWith|TestTheLegacy|TestAnArchive|TestAJobNamed' -v`
Expected: FAIL（`jobLogLines` undefined でビルドが通らない）

- [ ] **Step 7: 実装する**

`internal/gh/api/joblog.go` に足す。

```go
// jobLogLines reads the job's log out of the run's archive. GitHub builds one
// zip per run, so a single job's log is a matter of finding its entries in it.
//
// The order is the one gh reads in: per-step entries when the archive has any,
// the job's whole log when it does not, and the job's own endpoint when the
// archive names neither. An archive that cannot be fetched or read at all is
// an error rather than a fourth case: gh stops there too, and answering with
// the job endpoint would hide a failure behind a log with no step names.
func (c *Client) jobLogLines(ctx context.Context, repo string, j job, failedOnly bool) ([]gh.LogLine, error) {
	archive, err := c.read(ctx, fmt.Sprintf("repos/%s/actions/runs/%d/logs", repo, j.RunID), "")
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("read the run's log archive: %w", err)
	}

	if lines, ok, err := stepLines(zr, j, failedOnly); err != nil {
		return nil, err
	} else if ok {
		return lines, nil
	}
	if f := match(zr, wholeJobPattern(j.Name)); f != nil {
		return readEntry(f, unknownStep)
	}
	return c.wholeJobLog(ctx, repo, j)
}

// unknownStep is what a line is attributed to when only the job's whole log is
// available: the file holds every step's output with no boundary between them.
// It is the words gh puts there, so both backends show the same thing.
const unknownStep = "UNKNOWN STEP"

// stepLines reads one entry per step. ok is false when the archive has no
// per-step entry for this job at all, which is the caller's signal to fall
// back rather than to show an empty log.
func stepLines(zr *zip.Reader, j job, failedOnly bool) (lines []gh.LogLine, ok bool, err error) {
	steps := slices.Clone(j.Steps)
	slices.SortFunc(steps, func(a, b jobStep) int { return a.Number - b.Number })

	dir := logFileName(j.Name)
	for _, s := range steps {
		f := match(zr, stepPattern(dir, s.Number))
		if f == nil {
			continue
		}
		ok = true
		if failedOnly && !failed(s.Conclusion) {
			continue
		}
		entry, err := readEntry(f, s.Name)
		if err != nil {
			return nil, false, err
		}
		lines = append(lines, entry...)
	}
	return lines, ok, nil
}

// wholeJobLog reads the job's own log endpoint, which answers with plain text
// rather than an archive. There are no step boundaries in it.
func (c *Client) wholeJobLog(ctx context.Context, repo string, j job) ([]gh.LogLine, error) {
	body, err := c.read(ctx, fmt.Sprintf("repos/%s/actions/jobs/%d/logs", repo, j.ID), "")
	if err != nil {
		return nil, err
	}
	return logLines(string(body), unknownStep), nil
}

func readEntry(f *zip.File, step string) ([]gh.LogLine, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("read log entry %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read log entry %s: %w", f.Name, err)
	}
	return logLines(string(body), step), nil
}

// logLines splits one entry into the lines the screen shows.
func logLines(body, step string) []gh.LogLine {
	var lines []gh.LogLine
	for _, raw := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if raw == "" {
			continue
		}
		lines = append(lines, gh.NewLogLine(step, sanitizeControls(strings.TrimSuffix(raw, "\r"))))
	}
	return lines
}

// stepPattern is where a step's output sits: under the job's directory, named
// after the step's number and the step's own name. The name after the number
// is the server's, not ours, so only the number is matched.
func stepPattern(dir string, number int) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^%s/%d_.*\.txt$`, regexp.QuoteMeta(dir), number))
}

// wholeJobPattern is the top-level entry holding one job's entire log. The
// number in front is an ordinal that says nothing about which job it is, and
// the older Actions service puts a negative job id there instead.
func wholeJobPattern(jobName string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^-?\d+_%s\.txt$`, regexp.QuoteMeta(logFileName(jobName))))
}

func match(zr *zip.Reader, re *regexp.Regexp) *zip.File {
	for _, f := range zr.File {
		if re.MatchString(f.Name) {
			return f
		}
	}
	return nil
}
```

`io` を import に足す。

- [ ] **Step 8: 通ることを確かめる**

Run: `go test ./internal/gh/api/ -v`
Expected: PASS（Task 3 で書いたテストも含めて全部）

- [ ] **Step 9: 書いたテストが本当に落ちるか確かめる**

次の 3 つを 1 つずつ一時的に壊し、対応するテストが落ちることを確かめてから戻す。

1. `stepPattern` の `%d_` を `.*_` にする → `TestStepsComeOutInTheOrderTheyRan`
   か `TestEachLineCarriesTheStepItCameFrom` が落ちること
2. `wholeJobPattern` の `-?` を消す → `TestTheLegacyWholeJobEntryIsFoundToo`
   が落ちること
3. `stepLines` の `failedOnly && !failed(s.Conclusion)` を `false` にする →
   `TestFailedOnlyKeepsOnlyTheStepsThatFailed` が落ちること

- [ ] **Step 10: `make check` とコミット**

```bash
make check
git add internal/gh/api/checks.go internal/gh/api/checks_test.go \
	internal/gh/api/joblog.go internal/gh/api/joblog_test.go \
	internal/gh/api/testdata/job.json internal/gh/api/testdata/README.md
git commit -m "feat: read one job's log out of the run's log archive"
```

---

## Task 5: HTTP にタイムアウトを入れる

**Files:**
- Modify: `internal/gh/api/api.go`
- Modify: `internal/gh/api/transport.go:62`（`http.DefaultClient.Do`）
- Modify: `internal/gh/api/rest.go:58`（`http.DefaultClient.Do`）
- Test: `internal/gh/api/transport_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `Client.http *http.Client`（`New` が組む）と、パッケージ変数
  `responseTimeout time.Duration`

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/api/transport_test.go` に足す。

```go
// A server that accepts the connection and then never answers would otherwise
// hold the screen forever: there is no deadline on the context the TUI passes,
// and gh is not here to be killed.
func TestAServerThatNeverAnswersDoesNotHangForever(t *testing.T) {
	orig := responseTimeout
	responseTimeout = 20 * time.Millisecond
	t.Cleanup(func() { responseTimeout = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	t.Cleanup(srv.Close)

	c := New("", "kukv/octoscope", "secret-token")
	c.baseURL = srv.URL

	done := make(chan error, 1)
	go func() {
		_, err := c.read(context.Background(), "repos/kukv/octoscope/labels", "")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read: want an error when the server never answers")
		}
	case <-time.After(time.Second):
		t.Fatal("read did not give up")
	}
}

// The body of a large answer is not on the clock the headers are. A run's log
// archive is megabytes: cutting it off part way would show half a log as if
// the job had printed half a log.
func TestASlowBodyIsNotCutOff(t *testing.T) {
	orig := responseTimeout
	responseTimeout = 50 * time.Millisecond
	t.Cleanup(func() { responseTimeout = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)

	c := New("", "kukv/octoscope", "secret-token")
	c.baseURL = srv.URL

	if _, err := c.read(context.Background(), "repos/kukv/octoscope/labels", ""); err != nil {
		t.Fatalf("read: %v, want the slow body to arrive", err)
	}
}
```

`TestAServerThatNeverAnswersDoesNotHangForever` は `httptest.Server.Close` が
ハンドラの `time.Sleep` を待つので 2 秒ほど掛かる。それでよい。

**この 2 本は `t.Parallel()` を呼ばない。** パッケージ変数を差し替えるため
（既存の `TestARemoteReadKilledByTheDeadlineIsRetried`
（`internal/gh/api/repo_test.go:189`）と同じ理由・同じ形）。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/api/ -run 'TestAServerThatNeverAnswers|TestASlowBody' -v`
Expected: FAIL（`responseTimeout` undefined）

- [ ] **Step 3: 実装する**

`internal/gh/api/api.go` に足す。

```go
// responseTimeout bounds how long GitHub has to start answering: the time to
// connect, to shake hands, and to send response headers. It does not bound
// reading the body, because one answer here is a run's whole log archive and
// cutting that off part way would look like a job that printed half a log.
//
// Eight seconds is the longest answer measured while this backend was built
// (thirty repository aliases in one GraphQL request, 8.13s including the
// line), so the value is that rounded up to the next power of two: long
// enough that a request which is merely slow still lands.
var responseTimeout = 16 * time.Second
```

`Client` に `http *http.Client` を足し、`New` で組む。

```go
func New(dir, repo, token string) *Client {
	c := &Client{
		dir:   dir,
		repo:  repo,
		token: token,
		http: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: responseTimeout}).DialContext,
				TLSHandshakeTimeout:   responseTimeout,
				ResponseHeaderTimeout: responseTimeout,
				ForceAttemptHTTP2:     true,
			},
		},
	}
	c.Client = &gql.Client{
		Do:       c.post,
		RepoVars: c.repoVars,
	}
	return c
}
```

`transport.go:62` と `rest.go:58` の `http.DefaultClient.Do(req)` を
`c.http.Do(req)` にする。

**`Proxy: http.ProxyFromEnvironment` を落とさないこと。** `http.DefaultTransport`
はこれを持っており、自前の `Transport` に替えると企業プロキシ環境で通らなくなる。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/gh/api/ -v`
Expected: PASS

- [ ] **Step 5: `make check` とコミット**

```bash
make check
git add internal/gh/api/api.go internal/gh/api/transport.go internal/gh/api/rest.go \
	internal/gh/api/transport_test.go
git commit -m "feat: give the API backend a deadline for GitHub to start answering"
```

---

## Task 6: バックエンドの選択と、どちらも無いときの終了

**Files:**
- Create: `cmd/octoscope/backend.go`
- Create: `cmd/octoscope/backend_test.go`
- Modify: `cmd/octoscope/main.go:52-63`
- Modify: `internal/i18n/locales/active.en.yaml`, `internal/i18n/locales/active.ja.yaml`
- Delete: `internal/gh/api/parity_test.go`

**Interfaces:**
- Consumes: `cli.New(dir, repo string) *cli.Client`、`api.New(dir, repo, token string) *api.Client`、
  `api.Token() (string, error)`、`usecase.New(src, store)`
- Produces:
  ```go
  func chooseBackend(dir, repo string, lookPath func(string) (string, error),
  	token func() (string, error)) (*cli.Client, *api.Client, error)
  ```

**返り値を interface 1 つにまとめない理由。** `usecase.source` は unexported の
ままにする（export すると `.claude/rules/architecture.md`「interface は
利用側で定義する」の逆になる）。`cmd/octoscope` 側で同じ形の interface を
宣言し直すと、`source` の写しが 2 つ目として育ち、片方だけ更新される。
だから具体型を 2 つ返し、`main` が非 nil のほうを `usecase.New` に渡す。

`main` は返ってきた非 nil のほうを `usecase.New` に渡す。**これが
`usecase.New(api.New(...))` と `usecase.New(cli.New(...))` の両方を
コンパイラに検査させる形であり、`parity_test.go` が消せる理由である。**

- [ ] **Step 1: 失敗するテストを書く**

`cmd/octoscope/backend_test.go` を作る。

```go
package main

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// gh wins when it is there: it carries the user's own login, which the
// environment variables may not have, and it is what the user already set up.
func TestGhOnThePathWins(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope",
		func(string) (string, error) { return "/usr/bin/gh", nil },
		func() (string, error) { return "a-token", nil })
	if err != nil {
		t.Fatalf("chooseBackend: %v", err)
	}
	if c == nil || a != nil {
		t.Errorf("chose the API backend with gh on the path")
	}
}

// Without gh the token is the whole reason this backend exists.
func TestATokenIsUsedWhenGhIsNotThere(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope",
		func(string) (string, error) { return "", exec.ErrNotFound },
		func() (string, error) { return "a-token", nil })
	if err != nil {
		t.Fatalf("chooseBackend: %v", err)
	}
	if a == nil || c != nil {
		t.Errorf("did not choose the API backend without gh")
	}
}

// Neither one is not a failure to report as an unknown error: it is the one
// situation the user can fix, and the caller tells them how.
func TestNeitherGhNorATokenIsAnAuthenticationFailure(t *testing.T) {
	t.Parallel()

	_, _, err := chooseBackend("/work", "kukv/octoscope",
		func(string) (string, error) { return "", exec.ErrNotFound },
		func() (string, error) { return "", gh.ErrUnauthenticated })
	if !errors.Is(err, gh.ErrUnauthenticated) {
		t.Errorf("err = %v, want gh.ErrUnauthenticated", err)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./cmd/octoscope/ -v`
Expected: FAIL（`chooseBackend` undefined）

- [ ] **Step 3: 実装する**

`cmd/octoscope/backend.go` を作る。

```go
package main

import (
	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/api"
	"github.com/kukv/octoscope/internal/gh/cli"
)

// chooseBackend picks which client talks to GitHub. gh comes first: it is
// already signed in, and it carries a login the environment variables need not
// have. Without it a token is the whole reason the API backend exists.
//
// lookPath and token are parameters rather than the functions themselves so a
// test can say what the machine has.
//
// Exactly one of the two clients is non-nil when err is nil. Two return values
// rather than one interface is deliberate: the caller passes whichever it got
// to usecase.New, and the compiler checks both clients answer everything the
// usecase layer asks for.
func chooseBackend(dir, repo string, lookPath func(string) (string, error),
	token func() (string, error)) (*cli.Client, *api.Client, error) {
	if _, err := lookPath("gh"); err == nil {
		return cli.New(dir, repo), nil, nil
	}
	t, err := token()
	if err != nil {
		return nil, nil, gh.ErrUnauthenticated
	}
	return nil, api.New(dir, repo, t), nil
}
```

`main.go` の client を組んでいる箇所を書き換える。

```go
	// Whether the current directory has a repository is settled by the UI,
	// not here: answering it costs a request before the first frame, and
	// waiting for one left the terminal blank for as long as it took.
	ghClient, apiClient, err := chooseBackend(dir, *repoFlag, exec.LookPath, api.Token)
	if err != nil {
		// Printed before the program starts: once it is in the alt screen,
		// nothing written here survives the screen being cleared.
		fmt.Fprintln(os.Stderr, i18n.T("error.no_backend"))
		os.Exit(1)
	}

	// The store is built even when config.Path failed: it reports that
	// failure when something is saved, rather than saving nothing in silence.
	store := config.NewStore(path)
	var uc *usecase.Usecase
	if ghClient != nil {
		uc = usecase.New(ghClient, store)
	} else {
		uc = usecase.New(apiClient, store)
	}
```

`os/exec` と `internal/gh/api` を import に足す。

- [ ] **Step 4: 文言を足す**

`internal/i18n/locales/active.en.yaml` の `error:` の下に:

```yaml
  no_backend:
    other: "No way to reach GitHub. Either install the gh CLI and run gh auth login, or set GH_TOKEN to a personal access token."
```

`internal/i18n/locales/active.ja.yaml` の同じ位置に:

```yaml
  no_backend:
    other: "GitHub に接続する手段がありません。gh CLI を入れて gh auth login を実行するか、GH_TOKEN に個人アクセストークンを設定してください。"
```

既存の `error.gh_not_found` は消さない（`internal/tui` が別の場面で使っている。
使われていなければ**指摘だけして消さない** — 自分の変更で不要になったものでは
ないため）。

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./cmd/octoscope/ ./internal/i18n/ -v`
Expected: PASS。`internal/i18n/i18n_test.go:117` が両ロケールのキーの一致を
見ている（`ja catalog is missing IDs` / `en catalog is missing IDs`）ので、
片方に足し忘れるとここで落ちる。

- [ ] **Step 6: `parity_test.go` を消す**

```bash
git rm internal/gh/api/parity_test.go
```

`usecase.New(apiClient, store)` がコンパイルできることが、このファイルが
していた検査そのものになる。**消したあとで `api.Client` からメソッドを 1 つ
一時的に消し、`go build ./...` が落ちることを確かめる**（コンパイラが本当に
検査になっているかの確認）。確かめたら戻す。

- [ ] **Step 7: 起動して見る**

```bash
go run ./cmd/octoscope --repo kukv/octoscope
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

`gh` がある環境なので `cli` が選ばれる。今までと同じ画面が出ること。

エラー画面は `gh` を隠して確かめる。**先にビルドする** —
`go run` 自体が PATH の `go` を要るので、PATH を空にした状態では走らない。

```bash
go build -o /tmp/octoscope-check ./cmd/octoscope
env -i /tmp/octoscope-check
env -i LANG=ja_JP.UTF-8 /tmp/octoscope-check --lang ja
rm /tmp/octoscope-check
```

`env -i` は環境を空にするので `gh` も `GH_TOKEN` も `GITHUB_TOKEN` も無い状態になる。
**en と ja の両方で文言が読めることを確かめる。** 全角で桁を 2 つ使うので
`--lang ja` でも見る。

- [ ] **Step 8: `make check` とコミット**

```bash
make check
git add cmd/octoscope/backend.go cmd/octoscope/backend_test.go cmd/octoscope/main.go \
	internal/i18n/locales/active.en.yaml internal/i18n/locales/active.ja.yaml
git commit -m "feat: pick the backend from what the machine has"
```

---

## Task 7: 実データで 2 つのバックエンドを突き合わせる

**Files:** なし（確認だけ。結果は Task 8 の積み残し文書に書く）

小さな fixture の上でしか見ないレビューでは、規模と実データの形でしか出ない
食い違いは捕まらない。zip の名前の対応づけ・制御文字の無害化・BOM は
まさにその種類である。

- [ ] **Step 1: 材料があるか確かめる**

```bash
command -v gh && echo "gh: yes"
gh auth token > /dev/null 2>&1 && echo "token: yes"
```

両方無ければこの Task は飛ばし、**Task 8 の積み残し文書に「実データの
突き合わせは未実施」と理由つきで書く。** 黙って飛ばさない。

- [ ] **Step 2: 失敗したジョブを 1 件選ぶ**

```bash
gh run list --repo kukv/octoscope --status failure --limit 5 \
	--json databaseId,displayTitle
gh run view <RUN_ID> --repo kukv/octoscope --json jobs \
	--jq '.jobs[] | select(.conclusion=="failure") | {id, name}'
```

- [ ] **Step 3: 両方で引いて差を見る**

`internal/gh/api` に一時的なテスト（`//go:build parity` のタグ付き）を書くか、
`/tmp` に小さな `main` を書いて、同じ repo・同じ job id・`failedOnly` の
true と false の両方で `cli.Client.JobLog` と `api.Client.JobLog` を呼び、
`[]gh.LogLine` を比べる。**比べるのは `Step` / `Time` / `Text` の全部。**

差が出たら、どちらが正しいかを決める前に理由を確かめる。よくある食い違いの
出どころは 3 つ: ステップ名の対応づけ（zip のディレクトリ名）、制御文字
（`^[` になっているか）、タイムスタンプの切り出し。

- [ ] **Step 4: 一時的なものを消す**

`//go:build parity` のテストも `/tmp` の `main` も残さない。
**結果（差が無かった／どこがどう違ったか）は Task 8 の文書に書く。**

- [ ] **Step 5: `RerunWorkflow` は実際には叩かない**

本物の CI を走らせることになる。**エンドポイントの選択はテストで検査済みで、
実行の確認は 4-5 の手動確認（利用者の手元）に回す。**

---

## Task 8: 設計の訂正と積み残しの記録

**Files:**
- Modify: `docs/superpowers/specs/2026-09-08-phase4-design.md`
- Create: `docs/superpowers/2026-09-13-phase4-actions-followups.md`
- Modify: `docs/superpowers/2026-09-13-phase4-rest-backend-followups.md`

- [ ] **Step 1: 設計 §6 に `run view --log` の実際の形を書く**

§6 は「`gh` は Actions のログ zip を取って整形しており、api 側はこの整形を
自分で書くことになる」までしか書いていない。**実際に読んで分かった
`gh` の手順（この計画の「着手前に確かめたこと」の表）を §6 に移す。**
設計が実装より後に育つのは 4-3 と同じ形である。

`gh` が叩く順序・zip の構造・ジョブ名のサニタイズ・`UNKNOWN STEP`・
`asciisanitizer` の 5 点を、出典（`gh` のファイルと行）つきで書く。

- [ ] **Step 2: 完了条件を照合する**

設計 §11 の 1〜9 のうち、このスライスで動くのは 7 と 8 の再確認だけ。

```bash
go list -deps ./internal/tui/... | grep -E 'internal/gh/(cli|api)$' && echo "NG" || echo "OK"
```

これが `OK` を出すこと（完了条件 7）。`cmd/octoscope` だけが両方を import する。

- [ ] **Step 3: 積み残しを書く**

`docs/superpowers/2026-09-13-phase4-actions-followups.md` に、**直さなかったものは
直さないと決めた理由つきで**書く。少なくとも次を含める。

- **zip のキャッシュを持たない。** 同じジョブのログを開き直すたびに run の
  ログ zip を取り直す。`gh` はディスクにキャッシュする。octoscope が
  同じログを繰り返し開く道具になったら見直す
- **run のログ zip は 1 ジョブ分を読むために run 全体を落とす。** ジョブが
  多い run では無駄が大きい。`actions/jobs/{id}/logs` はステップの境界を
  持たないので、ステップ名を捨てる覚悟が要る取り替えである
- **タイムアウトは本文のダウンロードを縛らない。** ログ zip を落とす途中で
  回線が細くなっても待ち続ける。値の根拠は §2 の実測（30 alias で 8.13 秒）
- **`cli` バックエンドには依然タイムアウトが無い**（`exec.CommandContext` に
  呼び出し側の ctx を渡すだけ）。`api` だけに入れたので 2 つの挙動が揃って
  いない。揃えるなら `cli` 側にも同じ値のデッドラインが要る
- **`ResponseHeaderTimeout` は `/logs` にも掛かる。** GitHub は zip を組んでから
  302 を返すので、ジョブの多い run で応答開始までどれだけ掛かるかは未計測。
  16 秒で足りるかは 4-5 の手動確認で分かる
- **トークンが設定されているが無効なとき、`api` バックエンドでも TUI は起動し、
  実行時の `error.unauthenticated` が `Run: gh auth login` と出る。** `gh` が
  入っていない機械ではこの案内は誤り。文言をバックエンドで分けるか、
  両方を並べるかを 4-5 で決める
- **ラベルの順序 / `parseRemote` の明示ポート / ラベル編集の部分適用 /
  `nextLink` の `,`** は 4-3 から引き続き繰り越し
- Task 7 の実データ突き合わせの結果（未実施ならその理由）

`docs/superpowers/2026-09-13-phase4-rest-backend-followups.md` の「繰り越し」から、
**このスライスで片付いた 3 つ**（Actions・`main.go` の配線・タイムアウト・
`parity_test.go`）に片付いた旨と新しい文書への参照を足す。**行を消さない**
（何がいつ片付いたかが読めなくなる）。

- [ ] **Step 4: コミット**

```bash
make check
git add docs/
git commit -m "docs: record how gh builds a job log, and what this slice left"
```

---

## Self-Review

**1. 設計の網羅:** §10 が 4-4 に割り当てたのは `RerunWorkflow`（Task 2）・
`JobLog`（Task 3・4）・`main.go` のバックエンド選択と認証エラー画面（Task 6）。
4-3 の積み残しからは取得のタイムアウト（Task 5）と `parity_test.go` の削除
（Task 6 Step 6）。§11 の完了条件 7 は Task 8 Step 2 で照合する。
完了条件 10（`gh` を PATH から外した全機能の手動確認）は 4-5。

**2. 型の一致:** `job` / `jobStep` / `failed` は Task 3 で定義し Task 4 が使う。
`logFileName` / `sanitizeControls` は Task 4 Step 3 で定義し Step 7 が使う。
`gh.NewLogLine` は Task 1 で定義し Task 4 の `logLines` が使う。
`responseTimeout` は Task 5。`chooseBackend` は Task 6。

**3. コンパイルできない中間状態:** Task 3 は単体ではビルドが通らないので
Task 4 と 1 コミットにまとめる、と Task 3 の末尾に明記した。ほかのタスクは
それぞれ独立してコミットできる。
