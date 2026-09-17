# 自分宛てメンションの強調 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 自分宛てのメンションを含むコメントと説明を、詳細画面で一目で見つけられるようにする。
あわせて `handleKey` の 114 行・15 分岐を分ける。

**Architecture:** `viewer { login }` を GraphQL で 1 回引き、GitHub → gateway → usecase → root と
運ぶ。root は起動直後に非同期で引き、詳細画面を開くときに渡す。詳細画面は glamour に渡す**前の**
生 markdown を見て、ブロック単位で罫と見出し行の色を変える。描画済みの本文には触らない。

**Tech Stack:** Go 1.25 / Bubble Tea v2 / glamour v2 / lipgloss v2 / golden テスト

**Spec:** `docs/superpowers/specs/2026-09-17-mention-highlight-design.md`

**Branch:** `feat/detail-redesign`（続き。`a84993f` の上に積む）

---

## Global Constraints

- 各タスクの末尾で `make check`。緑でない状態でコミットしない
  （`internal/app/config` の `TestPathPutsTheFileUnderTheConfigDirectory` は
  この変更の前から落ちている環境依存。これだけは無視する）
- 桁数は `ansi.StringWidth` で数える（`.claude/rules/tui.md`）
- i18n のカタログは触らない。新しい文字列は 1 つも足さない
- `make golden` で塗り潰さない。既存のゴールデンが動いたら、まず自分の変更を疑う

## ファイル構成

| ファイル | 責務 | Task |
|---|---|---|
| `internal/app/presentation/tui/detail/keys.go` | mode ごとのキー処理（`handleKey` を分ける） | 1 |
| `internal/github/gql/viewer.graphql` + `viewer.go` | `viewer { login }` の送信と読み取り | 2 |
| `internal/app/adapter/gateway/gh/backend.go` | `backend` に `viewerFetcher` を足す（promotion で通す） | 3 |
| `internal/app/usecase/usecase.go` | ポート `viewerFetcher` と `Viewer(ctx)` | 3 |
| `internal/app/presentation/tui/root/root.go` | `resolveViewer` / `viewerResolvedMsg` / `m.viewer` / detail へ渡す | 4 |
| `internal/app/presentation/tui/detail/mention.go` | `mentionsViewer`（純関数。markdown は解析しない） | 5 |
| `internal/app/presentation/tui/icon/icon.go` | `MentionBar()` グリフ | 6 |
| `internal/app/presentation/tui/detail/body.go` | 罫と見出し行の色・グリフ | 7 |
| `internal/app/presentation/tui/detail/detail.go` | `Model.viewer` と `SetViewer`（Task 4）、`setBodyContent` が `viewer` を渡す（Task 7） | 4, 7 |

---

### Task 1: `handleKey` を分ける

振る舞いを変えない。強調より先にやる（振る舞いを変える変更と混ぜない）。

**Files:**
- Modify: `internal/app/presentation/tui/detail/keys.go`

- [ ] **Step 1: 先に現状を記録する**

Run: `go test ./internal/app/presentation/tui/detail/ && git status --short`
Expected: PASS、`git status` は空。ここが「何も変えていない」基準点になる。

- [ ] **Step 2: 本体を持つ 7 つの分岐をメソッドに切り出す**

`keys.go` の `handleKey` の `switch msg.String()` からファイル内の `handlePickerKey` の
直前までを、次で置き換える。**各 case のコメントは切り出し先へそのまま連れていく。**

```go
	switch msg.String() {
	case "q", "esc":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "o":
		if m.url == "" {
			return m, nil
		}
		return m, m.openWeb(m.ref, m.url)
	case "d":
		// An issue has no diff.
		if m.ref.Kind != domain.ItemPR {
			return m, nil
		}
		ref := m.ref
		return m, func() tea.Msg { return OpenDiffMsg{Ref: ref} }
	case "s":
		// An issue has no checks.
		if m.ref.Kind != domain.ItemPR {
			return m, nil
		}
		ref := m.ref
		return m, func() tea.Msg { return OpenChecksMsg{Ref: ref} }
	case "m":
		return m.openMerge()
	case "r":
		return m.refetch()
	case "c":
		return m.openCompose()
	case "x":
		return m.openConfirm()
	case "v":
		return m.openSubmit()
	case "l":
		return m.openPicker(pickLabels)
	case "a":
		return m.openPicker(pickAssignees)
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

// openMerge opens the merge popup. An issue has nothing to merge, and nor
// has a pull request that is already merged or closed: GitHub answers
// UNKNOWN for a merged one, so the popup would say it is still working the
// answer out, for ever.
func (m Model) openMerge() (Model, tea.Cmd) {
	if m.ref.Kind != domain.ItemPR {
		return m, nil
	}
	if m.phase == phaseLoading {
		return m.stillLoading(), nil
	}
	if !m.canMerge() {
		return m, nil
	}
	m.mode, m.phase = modeMerge, phaseIdle
	m.errText = ""
	m.merge = merge.New(m.src, m.ref)
	m.merge, _ = m.merge.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m, m.merge.Init()
}

// refetch asks for the item again. Unlike the others it is allowed while a
// fetch is already in flight: asking twice is what a reader does when the
// first one is taking too long.
func (m Model) refetch() (Model, tea.Cmd) {
	m.phase = phaseLoading
	m.declined = ""
	return m, fetch(m.src, m.ref)
}

// openCompose opens the comment composer.
func (m Model) openCompose() (Model, tea.Cmd) {
	if m.phase == phaseLoading {
		return m.stillLoading(), nil
	}
	m.mode, m.phase = modeCompose, phaseIdle
	m.errText = ""
	m.textarea.Reset()
	m.textarea.Focus()
	return m, textarea.Blink
}

// openConfirm asks before closing or reopening the item.
func (m Model) openConfirm() (Model, tea.Cmd) {
	if m.phase == phaseLoading {
		return m.stillLoading(), nil
	}
	if _, ok := m.stateAction(); !ok {
		return m, nil // merged and the like: no action
	}
	m.mode, m.phase = modeConfirm, phaseIdle
	m.errText = ""
	return m, nil
}

// openSubmit opens the review popup. An issue has no review. Unlike the diff
// view's v, this always fetches first: detail holds no review context of its
// own.
func (m Model) openSubmit() (Model, tea.Cmd) {
	if m.phase == phaseLoading {
		return m.stillLoading(), nil
	}
	if m.ref.Kind != domain.ItemPR {
		return m, nil
	}
	m.mode, m.phase = modeSubmit, phaseLoading
	m.errText = ""
	return m, fetchReviewContext(m.src, m.ref)
}

// openPicker opens the label or the assignee picker. Both wait on the
// repository's candidates, which is why the picker opens in phaseLoading
// rather than with an empty list.
func (m Model) openPicker(kind pickerKind) (Model, tea.Cmd) {
	if m.phase == phaseLoading {
		return m.stillLoading(), nil
	}
	m.mode, m.phase = modePick, phaseLoading
	m.errText = ""
	if kind == pickLabels {
		return m, fetchLabelPicker(m.src, m.ref)
	}
	return m, fetchAssigneePicker(m.src, m.ref)
}
```

`handleKey` の頭（`if m.mode != modeView && m.phase == phaseLoading` と mode の `switch`）は
触らない。

- [ ] **Step 3: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 4: ゴールデンも描画も動いていないことを確かめる**

Run: `git diff --stat -- internal/app/presentation/tui/detail/testdata/`
Expected: 出力なし（1 行も変わっていない）

- [ ] **Step 5: 行数を見る**

Run: `wc -l internal/app/presentation/tui/detail/keys.go`
Expected: ファイル全体は 260 行前後。`handleKey` 自体は 45 行以下で、
`modeView` の分岐は `q` `o` `d` `s` 以外は本体を持たない形になっている。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/app/presentation/tui/detail/keys.go
git commit -m "refactor(detail): give each key that opens something its own method

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `viewer { login }` を引く

**Files:**
- Create: `internal/github/gql/viewer.graphql`
- Create: `internal/github/gql/viewer.go`
- Create: `internal/github/gql/viewer_test.go`
- Modify: `internal/github/gql/schema_test.go`（`checkedDocs` に 1 行）

- [ ] **Step 1: 失敗するテストを書く**

`internal/github/gql/viewer_test.go`:

```go
package gql

import (
	"context"
	"strings"
	"testing"
)

// TestViewerReadsTheLogin is the whole of what this document is for: the one
// fact octoscope has never known about itself.
func TestViewerReadsTheLogin(t *testing.T) {
	t.Parallel()

	f := &fake{body: []byte(`{"data":{"viewer":{"login":"kukv"}}}`)}
	got, err := f.client().Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if got != "kukv" {
		t.Errorf("login is %q, want %q", got, "kukv")
	}
}

// TestViewerAsksForNoRepository guards the one way this document differs
// from every other query here: it takes no variables, so a backend that
// cannot resolve a repository can still send it.
func TestViewerAsksForNoRepository(t *testing.T) {
	t.Parallel()

	f := &fake{body: []byte(`{"data":{"viewer":{"login":"kukv"}}}`)}
	if _, err := f.client().Viewer(context.Background()); err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if len(f.vars[0]) != 0 {
		t.Errorf("the query was sent with %d variables, want none", len(f.vars[0]))
	}
	if strings.Contains(f.docs[0], "repository") {
		t.Error("the document names a repository")
	}
}

// TestViewerReportsABodyItCannotRead covers the answer that is not JSON at
// all: a proxy's HTML error page arriving where the API was expected.
func TestViewerReportsABodyItCannotRead(t *testing.T) {
	t.Parallel()

	f := &fake{body: []byte("<html>nope</html>")}
	if _, err := f.client().Viewer(context.Background()); err == nil {
		t.Error("a body that is not JSON was read as a login")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/github/gql/ -run TestViewer -v`
Expected: FAIL（コンパイルエラー: `f.client().Viewer undefined`）

- [ ] **Step 3: 文書と読み取りを書く**

`internal/github/gql/viewer.graphql`:

```graphql
# Who is signed in. The detail view needs the name to tell a comment
# addressed at the reader from one that is not; nothing else in octoscope has
# ever had to know it -- the Work board asks GitHub for "mentions:@me" and
# lets GitHub resolve it.
query {
  viewer {
    login
  }
}
```

`internal/github/gql/viewer.go`:

```go
package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed viewer.graphql
var viewerQuery string

// Viewer returns the login of the signed-in user. It takes no repository:
// who is signed in is a fact about the token, not about a repository, which
// is why this is the one read here that sends no variables.
func (c *Client) Viewer(ctx context.Context) (string, error) {
	out, err := c.Read(ctx, viewerQuery)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse viewer: %w", err)
	}
	return resp.Data.Viewer.Login, nil
}
```

- [ ] **Step 4: スキーマの検査に載せる**

`schema_test.go` の `checkedDocs` の `"repo_name.graphql": repoNameQuery,` の下に足す。

```go
	"viewer.graphql":             viewerQuery,
```

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/github/gql/ -run 'TestViewer|TestEveryFieldTheDocumentsSelect|TestEveryGraphQLFileIsChecked' -v`
Expected: PASS。スキーマ検査は記録済みのスキーマから `Query.viewer` → `User`、
`User.login` → `String` を引く（どちらも `testdata/schema.json` にある）。

- [ ] **Step 6: 両クライアントに生えていることを確かめる**

`cli.Client` と `api.Client` はどちらも `*gql.Client` を埋め込んでいるので、
ラッパを書かなくても `Viewer` を持つはず。それを型で確かめる一時ファイルを作って消す。

```bash
mkdir -p /tmp/viewer_probe
cat > /tmp/viewer_probe/main.go <<'GO'
package main

import (
	"context"

	"github.com/kukv/octoscope/internal/github/api"
	"github.com/kukv/octoscope/internal/github/cli"
)

type viewerFetcher interface {
	Viewer(ctx context.Context) (string, error)
}

var (
	_ viewerFetcher = (*cli.Client)(nil)
	_ viewerFetcher = (*api.Client)(nil)
)

func main() {}
GO
go vet /tmp/viewer_probe/main.go
rm -r /tmp/viewer_probe
```

Expected: `go vet` が何も言わずに終わる（両方が満たしている）。
落ちたら promotion が効いていないので、`cli/cli.go` の `RepoName` と同じ形の
ラッパを両方に書く。

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/github/gql/viewer.go internal/github/gql/viewer.graphql internal/github/gql/viewer_test.go internal/github/gql/schema_test.go
git commit -m "feat(gql): ask GitHub who is signed in

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: gateway と usecase に通す

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`
- Modify: `internal/app/usecase/usecase.go`
- Test: `internal/app/usecase/usecase_test.go`

- [ ] **Step 1: 失敗するテストを書く**

まず既存のフェイクの名前を確かめる。

Run: `grep -n 'type fake' internal/app/usecase/*_test.go`

バックエンドのフェイク（`usecase.New` の第 1 引数に渡っているもの）と、
store のフェイク（第 2 引数）の名前を控える。以下ではそれぞれ `fakeBackend` /
`fakeStore` と書くので、違っていたら読み替える。

バックエンドのフェイクに足す。

```go
	viewer    string
	viewerErr error
```

```go
func (f *fakeBackend) Viewer(context.Context) (string, error) { return f.viewer, f.viewerErr }
```

`usecase_test.go` の末尾に足す。

```go
// TestViewerPassesTheLoginThrough is the whole of what this layer does with
// it: there is nothing to decide, and a view is not allowed to reach the
// GitHub layer itself (.claude/rules/architecture.md).
func TestViewerPassesTheLoginThrough(t *testing.T) {
	u := usecase.New(&fakeBackend{viewer: "kukv"}, &fakeStore{})

	got, err := u.Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if got != "kukv" {
		t.Errorf("login is %q, want %q", got, "kukv")
	}
}
```

（`usecase_test.go` が内部テストパッケージなら `usecase.New` から `usecase.` を落とす。
ファイル先頭の `package` 行で確かめる。）

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/usecase/ -run TestViewerPassesTheLoginThrough -v`
Expected: FAIL（`u.Viewer undefined`）

- [ ] **Step 3: usecase にポートとメソッドを足す**

`usecase.go` の `lister` interface の**下**に足す。

```go
// viewerFetcher names the signed-in user. It is not part of lister: there is
// no repository involved, and nothing is being listed.
type viewerFetcher interface {
	Viewer(ctx context.Context) (string, error)
}
```

`source` interface の `lister` の下に 1 行。

```go
	viewerFetcher
```

`Usecase` 構造体の `lists lister` の下に 1 行。

```go
	viewer     viewerFetcher
```

`New` の `lists: src,` の下に 1 行。

```go
		viewer:     src,
```

`usecase.go:255` の `RepoName` の下に足す。

```go
// Viewer is the login of the signed-in user.
func (u *Usecase) Viewer(ctx context.Context) (string, error) { return u.viewer.Viewer(ctx) }
```

- [ ] **Step 4: gateway の `backend` に足す**

`internal/app/adapter/gateway/gh/backend.go` の `lister` interface の下に足す。

```go
// viewerFetcher names the signed-in user. Gateway promotes it unchanged:
// there is nothing to convert -- a login is a string in both languages.
// Promoting it is safe under the rule at the top of this file because the
// only caller (root's resolveViewer) drops the failure rather than showing
// it on the fatal-error screen.
type viewerFetcher interface {
	Viewer(ctx context.Context) (string, error)
}
```

`backend` interface の `lister` の下に 1 行。

```go
	viewerFetcher
```

`gh` パッケージにメソッドは書かない。

- [ ] **Step 5: 通ることを確かめる**

Run: `go build ./... && go test ./internal/app/usecase/ ./internal/app/adapter/... 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/app/usecase/ internal/app/adapter/gateway/gh/backend.go
git commit -m "feat(usecase): carry the signed-in login to the views

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: root が起動直後に引き、詳細画面へ渡す

**Files:**
- Modify: `internal/app/presentation/tui/detail/detail.go`
- Modify: `internal/app/presentation/tui/root/root.go`
- Test: `internal/app/presentation/tui/root/root_test.go`
- Modify: `internal/app/presentation/tui/root/scenario_test.go`

この時点では `viewer` は詳細画面に届くだけで、まだ何も描き変えない。描画は Task 7。

- [ ] **Step 1: detail に置き場所を作る**

`detail.go` の `Model` 構造体、`src` / `ref` の並びの直後に足す。

```go
	// viewer is the login of the signed-in user, or "" when it is not known
	// -- the lookup has not answered yet, or it failed. Empty draws the body
	// exactly as it was drawn before mentions were highlighted at all.
	viewer string
```

`New` の下に足す。

```go
// SetViewer names the signed-in user, so the body can tell a comment
// addressed at the reader from one that is not. The root calls it as the
// view is built; search.New(src).SetSavedQueries(...) is the same shape.
//
// It is not a parameter of New because the login is the one input that may
// not have arrived yet, and every test that builds a detail view would
// otherwise have to say it does not care.
func (m Model) SetViewer(login string) Model {
	m.viewer = login
	return m
}
```

- [ ] **Step 2: 失敗するテストを書く**

`root_test.go` のパッケージ行を確かめる。

Run: `head -3 internal/app/presentation/tui/root/root_test.go`

**内部テスト（`package root`）の場合**、末尾に足す。

```go
// TestTheViewerLookupReachesTheDetailView covers the one thing the login is
// fetched for. The lookup answers while the board is on screen, and the view
// built afterwards has to be told.
func TestTheViewerLookupReachesTheDetailView(t *testing.T) {
	m := New(&fakeSource{}, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(viewerResolvedMsg{login: "kukv"})
	m = next.(Model)

	if m.viewer != "kukv" {
		t.Errorf("the root kept %q, want %q", m.viewer, "kukv")
	}

	next, _ = m.openDetail(domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/demo", Number: 1})
	if got := next.(Model).detail.ViewerForTest(); got != "kukv" {
		t.Errorf("the detail view was told %q, want %q", got, "kukv")
	}
}

// TestAFailedViewerLookupIsNotAnError covers what happens when GitHub will
// not say who is signed in: the highlight is worth doing without, and an
// error screen over a decoration would cost the whole session.
func TestAFailedViewerLookupIsNotAnError(t *testing.T) {
	m := New(&fakeSource{viewerErr: errors.New("boom")}, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(resolveViewer(m.src)())
	m = next.(Model)

	if m.viewer != "" {
		t.Errorf("a failed lookup left %q behind", m.viewer)
	}
	if strings.Contains(m.View(), "boom") {
		t.Error("a failed viewer lookup reached the screen")
	}
}
```

**外部テスト（`package root_test`）の場合**は `viewerResolvedMsg` にも `m.viewer` にも
手が届かない。その場合は上の 2 つを書かず、代わりに次の 1 つだけを書く
（`root` が `Viewer` を呼んだことと、失敗が画面に出ないことを外から見る）。

```go
// TestAFailedViewerLookupIsNotAnError covers what happens when GitHub will
// not say who is signed in: the highlight is worth doing without, and an
// error screen over a decoration would cost the whole session.
func TestAFailedViewerLookupIsNotAnError(t *testing.T) {
	src := &fakeSource{viewerErr: errors.New("boom")}
	m := root.New(src, root.Options{})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(root.Model)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			next, _ = m.Update(msg)
			m = next.(root.Model)
		}
	}
	if !src.viewerAsked {
		t.Error("the root never asked who is signed in")
	}
	if strings.Contains(m.View(), "boom") {
		t.Error("a failed viewer lookup reached the screen")
	}
}
```

どちらの場合も `fakeSource` に足す。

```go
	viewer      string
	viewerErr   error
	viewerAsked bool
```

```go
func (f *fakeSource) Viewer(context.Context) (string, error) {
	f.viewerAsked = true
	return f.viewer, f.viewerErr
}
```

`scenario_test.go` の `scenarioSource` にも、`RepoName` の隣に足す。

```go
func (f *scenarioSource) Viewer(context.Context) (string, error) { return "kukv", nil }
```

内部テストを書いた場合のみ、`detail.go` の `SetViewer` の下に足す。

```go
// ViewerForTest is what the root told this view. It exists so that the
// root's test can see the login arrive without reaching into another
// package's fields.
func (m Model) ViewerForTest() string { return m.viewer }
```

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/root/ -run 'ViewerLookup' -v`
Expected: FAIL（`viewerResolvedMsg` / `resolveViewer` が無い）

- [ ] **Step 4: root に引く経路を足す**

`root.go` の `repoNamer` interface の下に足す。

```go
// viewerNamer names the signed-in user. Like repoNamer it is the root's
// alone: the views are told who is reading, they do not ask.
type viewerNamer interface {
	Viewer(ctx context.Context) (string, error)
}
```

`Source` interface の `repoNamer` の下に 1 行。

```go
	viewerNamer
```

`repoLookupTimeout` の定義の下に足す。

```go
// viewerLookupTimeout bounds the one call that learns who is signed in. It
// is as generous as repoLookupTimeout, and for the same reason: the call
// reaches the API, and a cold one is slow.
const viewerLookupTimeout = 20 * time.Second

// viewerResolvedMsg carries the login, or "" when there is none to be had.
// Unlike repoResolvedMsg it does not tell a timeout apart from a failure:
// nothing on the screen changes either way.
type viewerResolvedMsg struct{ login string }

// resolveViewer asks GitHub who is signed in, so the detail view can tell a
// comment addressed at the reader from one that is not.
//
// A failure is dropped. The name buys a highlight, not correctness: without
// it every view draws as it always did, and an error screen over a
// decoration would cost the user the session. That is also what keeps
// Gateway's promoted Viewer -- whose failures carry the client's own
// sentinels, not the domain's -- away from the fatal-error screen
// (adapter/gateway/gh/backend.go).
func resolveViewer(src Source) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), viewerLookupTimeout)
		defer cancel()
		login, err := src.Viewer(ctx)
		if err != nil {
			return viewerResolvedMsg{}
		}
		return viewerResolvedMsg{login: login}
	}
}
```

`Model` 構造体の `repoLookupTimedOut` の下に足す。

```go
	// viewer is the login of the signed-in user, handed to each detail view
	// as it is opened. Empty until the lookup answers, and for good if it
	// failed.
	viewer string
```

`Update` の `case repoResolvedMsg:` の下に足す。

```go
	case viewerResolvedMsg:
		m.viewer = msg.login
		return m, nil
```

`resize` の `if !m.started {` のブロック、
`cmds = append(cmds, fetch, m.repo.Init(), m.search.Init())` の下に 1 行足す。
`resolveRepo` と違い `--repo` の有無によらず常に引く（どのリポジトリを見ているかとは
関係の無い事実なので）。

```go
		cmds = append(cmds, resolveViewer(m.src))
```

`openDetail` の 1 行目を置き換える。

```go
	m.detail = detail.New(m.src, ref).SetViewer(m.viewer)
```

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/root/ -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 6: ゴールデンが動いていないことを確かめる**

Run: `git diff --stat -- internal/app/presentation/tui/root/testdata/ internal/app/presentation/tui/detail/testdata/`
Expected: 出力なし

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/app/presentation/tui/root/ internal/app/presentation/tui/detail/detail.go
git commit -m "feat(root): learn who is signed in and tell the detail view

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 自分宛てかどうかを判定する

**Files:**
- Create: `internal/app/presentation/tui/detail/mention.go`
- Create: `internal/app/presentation/tui/detail/mention_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/presentation/tui/detail/mention_test.go`:

```go
package detail

import "testing"

// TestMentionsViewer is the whole rule, case by case. The scan is not a
// markdown parser: it skips the three places an "@name" is certainly not
// addressed at a person, and takes everything else at face value.
func TestMentionsViewer(t *testing.T) {
	cases := map[string]struct {
		body string
		want bool
	}{
		"plain":                          {"@kukv ここ見てもらえますか", true},
		"mid-sentence":                   {"これは @kukv の担当です", true},
		"at the very end":                {"よろしく @kukv", true},
		"a different case":               {"@KuKv お願いします", true},
		"inside a link":                  {"[@kukv](https://github.com/kukv)", true},
		"straight onto a wide character": {"@kukvさん お願いします", true},
		"another name starting the same": {"@kukv-bot が直します", false},
		"a longer name":                  {"@kukvx が直します", false},
		"somebody else":                  {"@alice お願いします", false},
		"no mention at all":              {"LGTM です", false},
		"in a code span":                 {"`@kukv` と書くと通知が飛ぶ", false},
		"in a fenced block":              {"```\n@kukv\n```", false},
		"in a tilde fenced block":        {"~~~\n@kukv\n~~~", false},
		"in an unclosed fence":           {"```\n@kukv", false},
		"in a quoted line":               {"> @kukv と言われた", false},
		"in an indented quote":           {"  > @kukv と言われた", false},
		"after a fence closes":           {"```\ncode\n```\n@kukv 見てください", true},
		"an unmatched backtick":          {"値は 3` です @kukv", true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := mentionsViewer(c.body, "kukv"); got != c.want {
				t.Errorf("mentionsViewer(%q) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

// TestAnUnknownViewerMentionsNobody is the state the view is in until the
// lookup answers, and for good if it failed. Everything must read as "not
// addressed at me" rather than as "addressed at everybody".
func TestAnUnknownViewerMentionsNobody(t *testing.T) {
	if mentionsViewer("@kukv @alice @", "") {
		t.Error("an empty login matched")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestMentionsViewer|TestAnUnknownViewer' -v`
Expected: FAIL（`mentionsViewer` が未定義）

- [ ] **Step 3: 実装する**

`internal/app/presentation/tui/detail/mention.go`:

```go
// Whether a block of GitHub markdown is addressed at the reader. The answer
// decides which comment is drawn as the reader's own, so it is asked of the
// source GitHub sent -- not of what glamour drew, where colour and wrapping
// can split a name across two lines.

package detail

import "strings"

// mentionsViewer reports whether src names login the way a person writes to
// a person.
//
// This is not a markdown parser. Three places are skipped because an "@name"
// in them is certainly not addressed at anyone -- a fenced code block, an
// inline code span, and a quoted line -- and every other "@name" is taken at
// face value, including one inside a link or a heading. Lighting one comment
// too many costs a glance; missing the comment that names the reader costs
// the whole point of the highlight.
func mentionsViewer(src, login string) bool {
	if login == "" {
		return false
	}
	inFence := false
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		// Either marker toggles either kind of fence. Telling them apart
		// would only matter for a ``` inside a ~~~ block, where the answer
		// is the same anyway: it is code, and nobody is being addressed.
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(trimmed, ">") {
			continue
		}
		if mentionsInLine(stripCodeSpans(trimmed), login) {
			return true
		}
	}
	return false
}

// stripCodeSpans drops what sits between backticks. An odd number of them
// means the last one closes nothing -- in markdown it is a literal backtick,
// and the words after it are prose -- so such a line is left whole.
func stripCodeSpans(line string) string {
	parts := strings.Split(line, "`")
	if len(parts)%2 == 0 { // an even count of parts is an odd count of backticks
		return line
	}
	var b strings.Builder
	for i := 0; i < len(parts); i += 2 {
		b.WriteString(parts[i])
	}
	return b.String()
}

// mentionsInLine finds "@login" where the name ends. GitHub logins are
// letters, digits and hyphens and are not case-sensitive, so the "@kukv" in
// "@kukv-bot" is the start of somebody else's name.
func mentionsInLine(line, login string) bool {
	lower := strings.ToLower(line)
	name := "@" + strings.ToLower(login)
	for i := 0; i+len(name) <= len(lower); {
		j := strings.Index(lower[i:], name)
		if j < 0 {
			return false
		}
		end := i + j + len(name)
		if end == len(lower) || !isLoginByte(lower[end]) {
			return true
		}
		i = end
	}
	return false
}

// isLoginByte reports whether b could be the next byte of a login. A byte of
// a multi-byte character is not one, which is what makes "@kukvさん" a
// mention of kukv.
func isLoginByte(b byte) bool {
	return b == '-' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z'
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestMentionsViewer|TestAnUnknownViewer' -v`
Expected: PASS（18 ケース + 1 つ）

- [ ] **Step 5: 空振りしないことを確かめる**

`mentionsInLine` の `if end == len(lower) || !isLoginByte(lower[end])` を `if true` に変え、
`"@kukv-bot が直します"` のケースが落ちることを見る。見たら戻す。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/app/presentation/tui/detail/mention.go internal/app/presentation/tui/detail/mention_test.go
git commit -m "feat(detail): tell a block that names the reader from one that does not

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: 太い罫のグリフ

**Files:**
- Modify: `internal/app/presentation/tui/icon/icon.go`
- Modify: `internal/app/presentation/tui/icon/icon_test.go`

- [ ] **Step 1: テストに新しいグリフを載せる**

`icon_test.go` の `markers()` の中、次の行を

```go
	got = append(got, icon.Collapsed(), icon.CommentBar(), icon.ThreadBadge())
```

これで置き換える。

```go
	got = append(got, icon.Collapsed(), icon.CommentBar(), icon.MentionBar(), icon.ThreadBadge())
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/icon/ 2>&1 | tail -5`
Expected: FAIL（`icon.MentionBar undefined`）

- [ ] **Step 3: グリフを足す**

`icon.go` の `glyphs` 構造体の次の行を

```go
	collapsed, commentBar                            string
```

これで置き換える（フィールドの縦揃えは `make fmt` が直す）。

```go
	collapsed, commentBar, mentionBar                string
```

`sets` の 3 つのセットそれぞれで、`collapsed:` の行を置き換える。

Unicode:

```go
		collapsed: "▸", commentBar: "▌", mentionBar: "█",
```

Nerd:

```go
		collapsed: "▸", commentBar: "▌", mentionBar: "█",
```

ASCII:

```go
		collapsed: ">", commentBar: "|", mentionBar: "#",
```

`CommentBar` の下に足す。

```go
// MentionBar returns the one-column bar drawn down the left of a comment
// that names the reader. It is heavier than CommentBar rather than only a
// different colour, so that the two stay apart on a terminal that draws no
// colour at all.
func MentionBar() string { return active().mentionBar }
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/icon/ -v 2>&1 | tail -10`
Expected: PASS。`TestEveryGlyphIsOneColumn` が `█` と `#` を 1 桁と認める。

`TestEachSetDrawsItsOwnGlyphs` が落ちたら、そのテストが**セット間**の重複を見ているのか
**セット内**の重複を見ているのかを読んでから直す。セット内の重複を禁じているなら、
ASCII の `mentionBar` は `barDone` の `#` を避けて `%` にする。

- [ ] **Step 5: `make check` とコミット**

```bash
make check
git add internal/app/presentation/tui/icon/
git commit -m "feat(icon): add the heavy bar for a comment that names the reader

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: 描く

**Files:**
- Modify: `internal/app/presentation/tui/detail/body.go`
- Modify: `internal/app/presentation/tui/detail/detail.go:314`
- Modify: `internal/app/presentation/tui/detail/body_test.go`
- Modify: `internal/app/presentation/tui/detail/golden_test.go`

- [ ] **Step 1: 既存の呼び出しを 3 引数に直し、失敗するテストを足す**

`body_test.go` の既存の 6 か所を直す。

- `body_test.go:33` `commentLines(withComments().Comments[1], 40)` → `commentLines(withComments().Comments[1], 40, "")`
- `body_test.go:49` `bodyLines(withComments(), 60)` → `bodyLines(withComments(), 60, "")`
- `body_test.go:57` 同上
- `body_test.go:72` `bodyLines(it, 60)` → `bodyLines(it, 60, "")`
- `body_test.go:83` 同上
- `body_test.go:91` `bodyLines(withComments(), 40)` → `bodyLines(withComments(), 40, "")`

`body_test.go` の末尾に足す。

```go
// TestOnlyTheCommentThatNamesTheReaderGetsTheHeavyBar is what the whole
// feature comes down to: two comments, one of them addressed at the reader,
// and the reader can tell which before reading either.
func TestOnlyTheCommentThatNamesTheReaderGetsTheHeavyBar(t *testing.T) {
	it := withComments()
	it.Comments[1].Body = "@kukv ここ見てもらえますか"

	for _, l := range commentLines(it.Comments[0], 40, "kukv") {
		if !strings.HasPrefix(ansi.Strip(l), icon.CommentBar()) {
			t.Errorf("a comment that names nobody starts with %q", ansi.Strip(l))
		}
	}
	for _, l := range commentLines(it.Comments[1], 40, "kukv") {
		if !strings.HasPrefix(ansi.Strip(l), icon.MentionBar()) {
			t.Errorf("a comment that names the reader starts with %q", ansi.Strip(l))
		}
	}
}

// TestTheHeavyBarCostsTheTextNoColumns guards the pane: the bar is what the
// body is indented by, and a bar that measured two columns would push one
// comment's text out of step with the next.
func TestTheHeavyBarCostsTheTextNoColumns(t *testing.T) {
	it := withComments()
	it.Comments[1].Body = "@kukv ここ見てもらえますか"

	for _, l := range commentLines(it.Comments[1], 40, "kukv") {
		s := ansi.Strip(l)
		if w := ansi.StringWidth(string([]rune(s)[0])); w != 1 {
			t.Errorf("the heavy bar is %d columns wide, want 1", w)
		}
		if w := ansi.StringWidth(s); w > 40 {
			t.Errorf("a line is %d columns wide, want at most 40", w)
		}
	}
}

// TestTheDescriptionHeadingSaysWhenItNamesTheReader covers the other block:
// a description is not a comment and has no bar, so the heading above it is
// what carries the news.
func TestTheDescriptionHeadingSaysWhenItNamesTheReader(t *testing.T) {
	it := withComments()
	it.Body = "cc @kukv お願いします"

	plain := bodyLines(withComments(), 60, "kukv")[0]
	mine := bodyLines(it, 60, "kukv")[0]

	if ansi.Strip(plain) != ansi.Strip(mine) {
		t.Fatalf("the two headings read differently: %q and %q",
			ansi.Strip(plain), ansi.Strip(mine))
	}
	if plain == mine {
		t.Error("the heading looks the same whether or not the description names the reader")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestOnlyTheComment|TestTheHeavyBar|TestTheDescriptionHeading' -v`
Expected: FAIL（引数の数が合わない）

- [ ] **Step 3: `body.go` を直す**

`sectionHeading` を置き換える。

```go
// sectionHeading names a section and runs a rule from the end of the name to
// the edge of the pane. The name on its own was quieter than the text beneath
// it -- theme.Heading is muted, and a GitHub body is not -- which left the
// reader looking for the end of the description in a blank line.
//
// mine says the block under it names the reader. Only the name changes
// colour: the rule runs the width of the pane, and colouring that too would
// shout louder than a comment's bar does for the same piece of news.
func sectionHeading(name string, w int, mine bool) string {
	style := theme.Heading()
	if mine {
		style = theme.Accent().Bold(true)
	}
	head := style.Render(name)
	// The space is what keeps the rule from running into the last character.
	rest := w - ansi.StringWidth(name) - 1
	if rest <= 0 {
		return layout.Clip(head, w)
	}
	return head + " " + theme.Rule().Render(strings.Repeat("─", rest))
}
```

`bodyLines` を置き換える。

```go
// bodyLines is what scrolls: the description under its heading, then every
// comment behind its own bar. viewer is the reader's login, or "" when it is
// not known, which highlights nothing.
func bodyLines(it usecase.Item, w int, viewer string) []string {
	// The mention is looked for in what GitHub sent, not in the placeholder
	// that stands in for an empty description.
	lines := []string{sectionHeading(i18n.T("detail.section.description"), w,
		mentionsViewer(it.Body, viewer))}

	body := it.Body
	if strings.TrimSpace(body) == "" {
		body = i18n.T("detail.no_description")
	}
	for _, l := range markdownLines(body, max(w-bodyIndentWidth, 1)) {
		lines = append(lines, bodyIndent+l)
	}

	if len(it.Comments) == 0 {
		return fit(lines, w)
	}
	// The comments heading is never the reader's own: which of the comments
	// under it names them is said by each one's bar.
	lines = append(lines, "",
		sectionHeading(i18n.Tn("detail.section.comments", len(it.Comments)), w, false))
	for _, c := range it.Comments {
		lines = append(lines, "")
		lines = append(lines, commentLines(c, w, viewer)...)
	}
	return fit(lines, w)
}
```

`commentLines` を置き換える。

```go
// commentLines draws one comment: who wrote it and when, then the body, with
// a bar down the left of every line. The bar is what says where one comment
// ends and the next begins, so it cannot be left off a wrapped line.
//
// A comment that names the reader gets a heavier bar and the accent colour,
// down the whole of it. Only the bar and the header are coloured: the body
// has been through glamour, which has put colours of its own in it, and a
// style wrapped around that would reset them part way through a line.
func commentLines(c domain.Comment, w int, viewer string) []string {
	glyph, style := icon.CommentBar(), theme.Dim()
	if mentionsViewer(c.Body, viewer) {
		glyph, style = icon.MentionBar(), theme.Accent()
	}
	bar := style.Render(glyph + " ")
	lines := []string{bar + style.Render("@"+c.Author.Login+" · "+i18n.DateTime(c.CreatedAt))}
	for _, l := range markdownLines(c.Body, max(w-commentBarWidth, 1)) {
		lines = append(lines, bar+l)
	}
	return lines
}
```

`detail.go:314` を置き換える。

```go
	m.body.SetContentLines(bodyLines(m.item, w, m.viewer))
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -v 2>&1 | tail -30`
Expected: PASS

- [ ] **Step 5: 既存のゴールデンが動いていないことを確かめる**

Run: `git diff --stat -- internal/app/presentation/tui/detail/testdata/`
Expected: 出力なし。`goldenModel` は `SetViewer` を呼んでいないので `viewer` は ""。
動いていたら、`viewer` が "" のときに描き方が変わっている。

- [ ] **Step 6: 強調が出ている記録を足す**

`golden_test.go` の `goldenPR` の下に足す。

```go
// goldenMentionPR is the same item with the reader named in two places: the
// description and the first comment. The second comment names nobody, which
// is what makes the recording show the difference rather than only a colour.
func goldenMentionPR() domain.PR {
	pr := goldenPR()
	pr.Body = "This replaces the renderer.\n\n- one\n- two\n\ncc @kukv"
	pr.Comments[0].Body = "@kukv 見てもらえますか"
	return pr
}
```

`goldenModel` の下に足す。

```go
// goldenMentionModel is goldenModel with the reader known, so the recording
// keeps what the highlight actually draws.
func goldenMentionModel(width int) Model {
	f := &fakeSource{pr: goldenMentionPR()}
	m := New(f, goldenRef()).SetViewer("kukv")
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(fetch(f, goldenRef())())
	return m
}
```

`TestGolden` の中、最初の
`golden.Assert(t, fmt.Sprintf("detail_%s_%d", lang.name, w), m.View())` の直後に足す。

```go
				golden.Assert(t, fmt.Sprintf("detail_mention_%s_%d", lang.name, w),
					goldenMentionModel(w).View())
```

- [ ] **Step 7: 記録を作り、目で読む**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/detail/ -run TestGolden
git status --short internal/app/presentation/tui/detail/testdata/
```

Expected: `detail_mention_{en,ja}_{80,120,160}.golden` の **6 本だけ**が新規（`??`）。
既存のファイルが 1 つでも `M` になっていたら止めて、何を書き換えたのかを調べる。

続けて中身を読む。

```bash
sed -e 's/\x1b\[[0-9;]*m//g' internal/app/presentation/tui/detail/testdata/detail_mention_ja_120.golden
```

Expected: `cc @kukv` を含む説明の見出し行があり、`@kukv 見てもらえますか` のコメントの
罫が `█`、もう 1 件のコメントの罫が `▌`。どの行も右端が揃っている。

- [ ] **Step 8: `make check` とコミット**

```bash
make check
git add internal/app/presentation/tui/detail/
git commit -m "feat(detail): mark the comment and the description that name the reader

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: 設計書を追従させる

**Files:**
- Modify: `docs/superpowers/specs/2026-09-16-detail-redesign-design.md`
- Modify: `docs/superpowers/specs/2026-09-17-mention-highlight-design.md`

- [ ] **Step 1: 再設計の §7 から済んだ項を落とす**

`2026-09-16-detail-redesign-design.md` の §7「やらないこと」の第 1 項
（**自分宛てメンションの強調**、`viewer { login }` の配管に触れている段落）を丸ごと削り、
代わりに 1 行入れる。

```markdown
- **自分宛てメンションの強調**は `2026-09-17-mention-highlight-design.md` で設計し、実装した
```

- [ ] **Step 2: 再設計の §5.1 に新しいファイルを足す**

同じファイルの §5.1 の表に 1 行足し、`keys.go` と `detail.go` と `body.go` の行数を
実測に直す。

Run: `wc -l internal/app/presentation/tui/detail/*.go`

```markdown
| `mention.go` | 生 markdown が読み手を名指ししているかの判定 | （実測値） |
```

- [ ] **Step 3: 本設計の §3.3 を実装に合わせる**

`2026-09-17-mention-highlight-design.md` の §3.3 は `New` の第 3 引数として書いてあるが、
実装は `search.New(src).SetSavedQueries(...)` と同じ chainable にした。§3.3 の signature を
次に直し、理由（`New` を呼ぶテストが 12 か所あり、そのすべてが「ログイン名はどうでもよい」と
書く羽目になる）を 2 行添える。

```go
func (m Model) SetViewer(login string) Model
```

- [ ] **Step 4: コミット**

```bash
git add docs/superpowers/specs/
git commit -m "docs: record the mention highlight as built

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## 仕上げ

- [ ] `make check` が緑（既知の `internal/app/config` の 1 件を除く）
- [ ] `git diff --stat a84993f -- internal/app/presentation/tui/detail/testdata/` が
      **新規 6 本だけ**（既存の記録は 1 バイトも動いていない）
- [ ] `wc -l internal/app/presentation/tui/detail/*.go` でどのファイルも 300 行前後
- [ ] 実機で確かめる。自分がメンションされている PR を開き、コメントの罫が太く青いこと

  ```bash
  go run ./cmd/octoscope --lang ja
  ```

- [ ] 全角で桁が崩れていないことを `--lang ja` で見る（`.claude/rules/tui.md`）
- [ ] `OCTOSCOPE_ICONS=ascii go run ./cmd/octoscope` で ASCII セットも見る（`#` と `|`）
- [ ] `memory/detail-view-leftovers.md` を消し、`MEMORY.md` の該当行も消す
