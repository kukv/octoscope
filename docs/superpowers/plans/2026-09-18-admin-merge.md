# admin マージ実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** merge ポップアップで、保護ルールに止められた PR を `a` キーで押し切れるようにする。

**Architecture:** 新しい API 経路は作らない。`gh pr merge --admin` と同じく、送るのは既存の
GraphQL `mergePullRequest` ミューテーションのままで、変えるのはクライアント側のゲートだけである。
`repository.viewerPermission` を merge context に読み足し、`domain.MergeContext.CanMergeAsAdmin()` が
`BlockProtected` / `BlockBehind` のときだけ true を返す。TUI はそのとき `a` を受け付ける。
`enter` の挙動は一切変えない。

**Tech Stack:** Go / Bubble Tea v2 (`charm.land/bubbletea/v2`) / GitHub GraphQL API /
`internal/golden` によるゴールデンテスト / `internal/i18n`（en・ja 2 カタログ）

**設計:** `docs/superpowers/specs/2026-09-18-admin-merge-design.md`

---

## コメントの規約

`.claude/rules/go-style.md` は **実装計画や設計書への参照をコードに書くことを禁じている**（`Task 10`、`design 1.4` のような参照）。理由は、コードを読む人がそれを辿れず、その計画が終わったあとは意味を失うからである。**なぜそうなっているか**は書く。**どの文書の何番か**は書かない。このファイル内のコード片はその形で書いてある。

## ファイル構成

| ファイル | 役割 | 変更 |
|---|---|---|
| `internal/github/gql/merge.graphql` | merge context のクエリ | `viewerPermission` を 1 行追加 |
| `internal/github/gql/merge.go` | レスポンスのパース | 構造体に string を 2 箇所追加 |
| `internal/app/domain/merge.go` | ブロック判定の置き場 | フィールドと `CanMergeAsAdmin()` を追加 |
| `internal/app/adapter/gateway/gh/merge.go` | gql → domain の変換 | `toMergeContext` で権限文字列を bool にする |
| `internal/i18n/locales/active.en.yaml` | 英語カタログ | 2 キー追加 |
| `internal/i18n/locales/active.ja.yaml` | 日本語カタログ | 2 キー追加 |
| `internal/app/presentation/tui/merge/merge.go` | ポップアップのキー処理 | `handleKey` に `a` を追加 |
| `internal/app/presentation/tui/merge/render.go` | ポップアップの描画 | `reason()` と `hints()` に 1 分岐ずつ |
| `internal/app/presentation/tui/merge/testdata/` | ゴールデン | `merge_admin_*` を 6 本追加 |

`Source` interface、`MergePR` / `EnableAutoMerge` / `DisableAutoMerge`、`send()` は**変更しない**。

---

### Task 0: `viewerCanMergeAsAdmin` が ruleset 下で機能するか実測する — **済（2026-09-18）**

PR #100 で測った。9 本の status check が全て通り、残る要件がレビュー 1 件だけの状態で:

```
{"repository":{"viewerPermission":"ADMIN","pullRequest":{
  "mergeStateStatus":"BLOCKED","reviewDecision":"REVIEW_REQUIRED",
  "viewerCanMergeAsAdmin":false}}}
```

**`false`。** ruleset は admin ロールを `bypass_mode: always` で持っているので実際には
押し切れる。このフィールドは classic branch protection しか見ていない。

**よって判定は `repository.viewerPermission == "ADMIN"` を採る。** 結果と限界は
設計 §1.4 に記録済み（commit ac15ed5）。Task 1 以降はその前提で書き直してある。

- [x] 実測した
- [x] 設計文書 §1.4 に追記した

---

### Task 1: GraphQL から `viewerPermission` を読む

**Files:**
- Modify: `internal/github/gql/merge.graphql`
- Modify: `internal/github/gql/merge.go`
- Test: `internal/github/gql/merge_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/github/gql/merge_test.go` の `TestPRMergeContextReadsEveryField` を、
新しいフィールドを含む形に差し替える。**既存のテストを 1 つ増やすのではなく
書き換える**のは、このテストが「全フィールドが読めること」を名乗っているからである。

```go
func TestPRMergeContextReadsEveryField(t *testing.T) {
	t.Parallel()

	body := `{"data":{"repository":{"squashMergeAllowed":true,"mergeCommitAllowed":false,` +
		`"rebaseMergeAllowed":false,"deleteBranchOnMerge":true,"autoMergeAllowed":true,` +
		`"viewerPermission":"ADMIN",` +
		`"pullRequest":{"id":"PR_1","isDraft":false,"mergeable":"MERGEABLE",` +
		`"mergeStateStatus":"CLEAN","reviewDecision":"APPROVED",` +
		`"viewerCanEnableAutoMerge":true,"autoMergeRequest":null}}}}`
	f := &fakeSeq{outs: []string{body}}
	c := &Client{Do: f.do}

	got, err := c.PRMergeContext(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRMergeContext: %v", err)
	}
	want := MergeContext{
		PullRequestID:            "PR_1",
		IsDraft:                  false,
		Mergeable:                "MERGEABLE",
		MergeStateStatus:         "CLEAN",
		ReviewDecision:           "APPROVED",
		SquashMergeAllowed:       true,
		MergeCommitAllowed:       false,
		RebaseMergeAllowed:       false,
		DeleteBranchOnMerge:      true,
		AutoMergeAllowed:         true,
		ViewerCanEnableAutoMerge: true,
		ViewerPermission:         "ADMIN",
		AutoMergeEnabled:         false,
	}
	if got != want {
		t.Errorf("PRMergeContext() = %+v, want %+v", got, want)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/github/gql/ -run TestPRMergeContextReadsEveryField
```

期待: コンパイルエラー `unknown field ViewerPermission in struct literal of type MergeContext`

- [ ] **Step 3: クエリにフィールドを足す**

`internal/github/gql/merge.graphql` の `repository` の直下、`autoMergeAllowed` の直後
（**`pullRequest` の中ではない**）:

```graphql
    autoMergeAllowed
    # What says whether offering the admin merge is honest.
    # viewerCanMergeAsAdmin is the field named for it, but it reads classic
    # branch protection only: on a repository guarded by a ruleset it answers
    # false even to a viewer the ruleset lists as an always-bypass actor
    # (measured 2026-09-18). The permission is the closest honest signal left.
    viewerPermission
```

- [ ] **Step 4: 構造体に足す**

`internal/github/gql/merge.go` の `MergeContext`、`ViewerCanEnableAutoMerge` の直後:

```go
	ViewerCanEnableAutoMerge bool
	ViewerPermission         string
```

`mergeContextResponse` の `Repository` の中、`AutoMergeAllowed` の直後:

```go
			AutoMergeAllowed    bool   `json:"autoMergeAllowed"`
			ViewerPermission    string `json:"viewerPermission"`
```

`PRMergeContext` の戻り値の組み立て、`ViewerCanEnableAutoMerge` の直後。**`r.` であって `pr.` ではない**:

```go
		ViewerCanEnableAutoMerge: pr.ViewerCanEnableAutoMerge,
		ViewerPermission:         r.ViewerPermission,
```

- [ ] **Step 5: 通ることを確かめる**

```bash
go test ./internal/github/gql/
```

期待: PASS。`testdata/merge_context.json` の記録には新しいフィールドが無いが、
JSON に無いキーは false になるだけなので `TestPRMergeContextReadsWhatTheRepositoryAllows` は
そのまま通る。

- [ ] **Step 6: コミット**

```bash
git add internal/github/gql/merge.graphql internal/github/gql/merge.go internal/github/gql/merge_test.go
git commit -m "feat(merge): read viewerPermission from the merge context query"
```

---

### Task 2: `CanMergeAsAdmin` をドメインに置く

**Files:**
- Modify: `internal/app/domain/merge.go`
- Test: `internal/app/domain/merge_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/domain/merge_test.go` の末尾に足す。

```go
func TestOnlyTwoBlocksGiveWayToAnAdmin(t *testing.T) {
	t.Parallel()

	// admin makes a context in the given state with the permission granted:
	// every row below turns on the permission, so what the table measures is
	// the block, not the flag. The one row that turns it off is last.
	admin := func(c MergeContext) MergeContext {
		c.ViewerIsAdmin = true
		return c
	}

	tests := []struct {
		name string
		ctx  MergeContext
		want bool
	}{
		{"protected: this is what the key is for", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateBlocked,
		}), true},
		{"behind: gh's --admin bypasses this one too", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateBehind,
		}), true},
		{"clean: nothing to push past", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateClean,
		}), false},
		{"unstable: GitHub allows the merge already", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateUnstable,
		}), false},
		{"draft: not a rule, unfinished work", admin(MergeContext{
			IsDraft: true, Mergeable: MergeableYes, State: MergeStateBlocked,
		}), false},
		{"conflicting: no permission resolves a conflict", admin(MergeContext{
			Mergeable: MergeableConflicting, State: MergeStateDirty,
		}), false},
		{"computing: the answer is not in yet", admin(MergeContext{
			Mergeable: MergeableUnknown, State: MergeStateUnknown,
		}), false},
		{"dirty: GitHub will not merge it at all", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateDirty,
		}), false},
		{"blocked, but the viewer is no admin", MergeContext{
			Mergeable: MergeableYes, State: MergeStateBlocked,
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ctx.CanMergeAsAdmin(); got != tt.want {
				t.Errorf("CanMergeAsAdmin() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/domain/ -run TestOnlyTwoBlocksGiveWayToAnAdmin
```

期待: コンパイルエラー `ctx.CanMergeAsAdmin undefined` と
`unknown field ViewerIsAdmin`

- [ ] **Step 3: フィールドとメソッドを足す**

`internal/app/domain/merge.go` の `MergeContext`、`ViewerCanEnableAutoMerge` の直後:

```go
	AutoMergeAllowed         bool
	ViewerCanEnableAutoMerge bool
	AutoMergeEnabled         bool

	// ViewerIsAdmin is what keeps the popup from offering a key that fails.
	// The mutation that merges takes no admin input -- gh pr merge --admin
	// sends the same one -- so nothing in the answer to the merge itself
	// says whether this viewer may push past a rule. GitHub's own
	// viewerCanMergeAsAdmin reads classic branch protection only and answers
	// false under a ruleset, so the gateway fills this from the repository
	// permission instead.
	ViewerIsAdmin bool
```

`CanAutoMerge` の直後:

```go
// CanMergeAsAdmin reports whether the viewer can push the merge through what
// is holding it. Only two blocks give way, the same two gh's --admin silences.
// A draft or a conflict is not a rule to bypass: it is work that is not
// finished, and no permission finishes it.
func (c MergeContext) CanMergeAsAdmin() bool {
	if !c.ViewerIsAdmin {
		return false
	}
	switch c.Block() {
	case BlockProtected, BlockBehind:
		return true
	}
	return false
}
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/domain/
```

期待: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/app/domain/merge.go internal/app/domain/merge_test.go
git commit -m "feat(merge): name the two blocks that give way to an admin"
```

---

### Task 3: ゲートウェイで素通しする

**Files:**
- Modify: `internal/app/adapter/gateway/gh/merge.go:36-50`
- Test: `internal/app/adapter/gateway/gh/merge_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/adapter/gateway/gh/merge_test.go` の末尾に足す。

```go
func TestTheAdminPermissionSurvivesTheTranslation(t *testing.T) {
	t.Parallel()

	got := toMergeContext(gql.MergeContext{
		PullRequestID:         "PR_1",
		Mergeable:             "MERGEABLE",
		MergeStateStatus: "BLOCKED",
		ViewerPermission: "ADMIN",
	})
	if !got.ViewerIsAdmin {
		t.Error("ViewerIsAdmin = false, want true: the popup has no other way to know")
	}
	if !got.CanMergeAsAdmin() {
		t.Error("CanMergeAsAdmin() = false, want true for a blocked pull request")
	}
}
```

`gql` が既に import されていなければ `"github.com/kukv/octoscope/internal/github/gql"` を足す。

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/adapter/gateway/gh/ -run TestTheAdminPermissionSurvivesTheTranslation
```

期待: FAIL `ViewerIsAdmin = false, want true`

- [ ] **Step 3: 変換に足す**

`internal/app/adapter/gateway/gh/merge.go` の `toMergeContext`、
`AutoMergeEnabled` の行の直後:

```go
		AutoMergeEnabled:         c.AutoMergeEnabled,
		ViewerIsAdmin:            c.ViewerPermission == "ADMIN",
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/adapter/gateway/gh/
```

期待: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/app/adapter/gateway/gh/merge.go internal/app/adapter/gateway/gh/merge_test.go
git commit -m "feat(merge): carry the admin permission through the gateway"
```

---

### Task 4: 文言を 2 つのカタログに足す

**Files:**
- Modify: `internal/i18n/locales/active.en.yaml`
- Modify: `internal/i18n/locales/active.ja.yaml`

Task 5 のコードが参照するので先に足す。片方だけだと
`TestCatalogsHaveTheSameIDs` が落ちる。

- [ ] **Step 1: 英語に足す**

`active.en.yaml` の `merge:` セクション、`review_changes` の直後:

```yaml
  review_changes:
    other: "changes have been requested"
  admin_offer:
    other: "you can merge it anyway as an admin"
```

同セクションの `key_merge` の直後:

```yaml
  key_merge:
    other: "enter:merge"
  key_admin:
    other: "a:admin merge"
```

- [ ] **Step 2: 日本語に足す**

`active.ja.yaml` の同じ位置に、同じ順序で:

```yaml
  review_changes:
    other: "変更が要求されています"
  admin_offer:
    other: "admin 権限で押し切れます"
```

```yaml
  key_merge:
    other: "enter:マージ"
  key_admin:
    other: "a:adminでマージ"
```

- [ ] **Step 3: カタログ整合を確かめる**

```bash
go test ./internal/i18n/ -run TestCatalogsHaveTheSameIDs
```

期待: PASS

- [ ] **Step 4: コミット**

```bash
git add internal/i18n/locales/active.en.yaml internal/i18n/locales/active.ja.yaml
git commit -m "feat(merge): word the admin merge offer in both catalogs"
```

---

### Task 5: `a` で押し切る

**Files:**
- Modify: `internal/app/presentation/tui/merge/merge.go:136-170`
- Test: `internal/app/presentation/tui/merge/merge_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/presentation/tui/merge/merge_test.go` の末尾に足す。
ヘルパー `blocked` は Task 6 のゴールデンからも使うので、ここで定義する。

```go
// blocked is a pull request a branch rule is holding, with the viewer able
// to push past it.
func blocked() domain.MergeContext {
	c := mergeable()
	c.State = domain.MergeStateBlocked
	c.ViewerIsAdmin = true
	return c
}

func TestAPressedOnABlockedPullRequestMergesIt(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: blocked()}
	m := loaded(t, f)
	_, cmd := press(m, "a")
	if cmd == nil {
		t.Fatal("a returned no command: nothing was sent")
	}
	if msg := cmd(); msg != (MergedMsg{Merged: true}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{Merged: true}", msg)
	}
	if len(f.merged) != 1 || f.merged[0] != domain.MergeSquash {
		t.Errorf("merged = %v, want one MergeSquash (the method on the cursor)", f.merged)
	}
}

// The key that breaks a protection must never be the key an ordinary merge
// is sent with: a mistake has to be a wrong key, not the usual one.
func TestEnterStillSendsNothingOnABlockedPullRequest(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: blocked()}
	m := loaded(t, f)
	_, cmd := enter(m)
	if cmd != nil {
		t.Fatal("enter returned a command: a blocked merge must still be refused")
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing", f.merged)
	}
}

func TestAIsRefusedWhereNoPermissionWouldHelp(t *testing.T) {
	t.Parallel()

	draft := blocked()
	draft.IsDraft = true

	conflicting := blocked()
	conflicting.Mergeable = domain.MergeableConflicting
	conflicting.State = domain.MergeStateDirty

	computing := blocked()
	computing.Mergeable = domain.MergeableUnknown
	computing.State = domain.MergeStateUnknown

	notAdmin := blocked()
	notAdmin.ViewerIsAdmin = false

	tests := []struct {
		name string
		ctx  domain.MergeContext
	}{
		{"draft", draft},
		{"conflicting", conflicting},
		{"computing", computing},
		{"the viewer is no admin", notAdmin},
		{"nothing is holding it: enter is the key for that", mergeable()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeSource{ctx: tt.ctx}
			m := loaded(t, f)
			_, cmd := press(m, "a")
			if cmd != nil {
				t.Error("a returned a command, want nothing sent")
			}
			if len(f.merged) != 0 {
				t.Errorf("merged = %v, want nothing", f.merged)
			}
		})
	}
}

func TestAIsIgnoredWhileAMergeIsInFlight(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: blocked()}
	m := loaded(t, f)
	m, cmd := press(m, "a") // the command is deliberately left unrun
	if cmd == nil {
		t.Fatal("a returned no command: the test needs the popup mid-send")
	}
	_, cmd = press(m, "a")
	if cmd != nil {
		t.Error("a returned a second command while the first was in flight")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/merge/ -run 'TestAPressed|TestEnterStill|TestAIsRefused|TestAIsIgnored'
```

期待: `TestAPressedOnABlockedPullRequestMergesIt` が
`a returned no command: nothing was sent` で FAIL。他の 3 つは通る
（`a` がまだ何もしないので）。

- [ ] **Step 3: キーを足す**

`internal/app/presentation/tui/merge/merge.go` の `handleKey`、
`case "enter":` の直前:

```go
	case "a":
		return m.mergeAsAdmin()
	case "enter":
		return m.send()
	}
	return m, nil
}

// mergeAsAdmin pushes the merge past what is holding it. It is a key of its
// own rather than a meaning enter takes on when merging is blocked: breaking
// a protection must not share a keystroke with an ordinary merge, or a
// mistake looks exactly like the thing the user does every day.
func (m Model) mergeAsAdmin() (Model, tea.Cmd) {
	if !m.ctx.CanMergeAsAdmin() {
		return m, nil
	}
	method := m.method()
	return m.sendCmd(true, func() error { return m.src.MergePR(m.ctx.PullRequest, method) })
}
```

`m.sending` を見るガードは `handleKey` の先頭に既にあり、`a` にもそのまま効く。

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/presentation/tui/merge/
```

期待: PASS（ゴールデンはまだ変わらない。`a` は描画に出ていない）

- [ ] **Step 5: コミット**

```bash
git add internal/app/presentation/tui/merge/merge.go internal/app/presentation/tui/merge/merge_test.go
git commit -m "feat(merge): let a push a blocked merge through, and leave enter refusing it"
```

---

### Task 6: `a` を画面に出す

**Files:**
- Modify: `internal/app/presentation/tui/merge/render.go:108-120`（`reason`）
- Modify: `internal/app/presentation/tui/merge/render.go:143-168`（`hints`）
- Test: `internal/app/presentation/tui/merge/merge_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`merge_test.go` の末尾に足す。このファイルは今 `context` / `errors` / `testing` /
`tea` / `domain` しか import していないので、3 つ足りない:

```go
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/kukv/octoscope/internal/i18n"
```

```go
// A key that works but is not on the bar is a key nobody finds.
func TestTheBlockedPopupOffersTheAdminKey(t *testing.T) {
	t.Parallel()

	m := loaded(t, &fakeSource{ctx: blocked()})
	view := ansi.Strip(m.View())
	if want := i18n.T("merge.key_admin"); !strings.Contains(view, want) {
		t.Errorf("the key bar has no %q:\n%s", want, view)
	}
	if want := i18n.T("merge.admin_offer"); !strings.Contains(view, want) {
		t.Errorf("the popup does not say %q:\n%s", want, view)
	}
	if notWant := i18n.T("merge.key_merge"); strings.Contains(view, notWant) {
		t.Errorf("the key bar offers %q on a blocked pull request:\n%s", notWant, view)
	}
}

func TestAPopupWithNoAdminOfferSaysNothingAboutIt(t *testing.T) {
	t.Parallel()

	c := blocked()
	c.ViewerIsAdmin = false
	m := loaded(t, &fakeSource{ctx: c})
	view := ansi.Strip(m.View())
	if notWant := i18n.T("merge.key_admin"); strings.Contains(view, notWant) {
		t.Errorf("the key bar offers %q to a viewer who may not:\n%s", notWant, view)
	}
	if notWant := i18n.T("merge.admin_offer"); strings.Contains(view, notWant) {
		t.Errorf("the popup says %q to a viewer who may not:\n%s", notWant, view)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/merge/ -run 'TestTheBlockedPopupOffersTheAdminKey|TestAPopupWithNoAdminOffer'
```

期待: `TestTheBlockedPopupOffersTheAdminKey` が
`the key bar has no "a:admin merge"` で FAIL

- [ ] **Step 3: `reason` に 1 行足す**

`render.go` の `reason()` を差し替える。ブロックの行の下に、押し切れるときだけ
提案を足す形である。

```go
// reason is the line under the options: why merging is refused, or, when
// nothing refuses it, what the review still wants. A block an admin can push
// past carries a second line saying so: the key bar alone names the key but
// not that it is a protection being broken.
func (m Model) reason() string {
	if text := blockText(m.ctx.Block()); text != "" {
		line := theme.Error().Render(icon.Warning() + " " + text)
		if m.ctx.CanMergeAsAdmin() {
			line += "\n" + theme.Dim().Render("  "+i18n.T("merge.admin_offer"))
		}
		return line
	}
	switch m.ctx.Review {
	case domain.ReviewRequired:
		return theme.Review(domain.ReviewRequired, false).Render(icon.Warning() + " " + i18n.T("merge.review_required"))
	case domain.ReviewChangesRequested:
		return theme.Review(domain.ReviewChangesRequested, false).Render(icon.Warning() + " " + i18n.T("merge.review_changes"))
	default:
		return ""
	}
}
```

- [ ] **Step 4: `hints` に 1 行足す**

`render.go` の `hints()` の中、`case !m.answered() || m.ctx.Block() != domain.BlockNone:`
の本体を差し替える。

```go
	case !m.answered() || m.ctx.Block() != domain.BlockNone:
		// enter sends nothing: there is no answer, or something refuses it.
		// a does, where the viewer may push past what refuses it.
		if m.ctx.CanMergeAsAdmin() {
			hints = append(hints, i18n.T("merge.key_admin"))
		}
```

- [ ] **Step 5: 通ることを確かめる**

```bash
go test ./internal/app/presentation/tui/merge/ -run 'TestTheBlockedPopupOffersTheAdminKey|TestAPopupWithNoAdminOffer'
```

期待: PASS

- [ ] **Step 6: 既存のゴールデンが動いていないことを確かめる**

```bash
go test ./internal/app/presentation/tui/merge/
```

期待: PASS。既存のゴールデン状態はどれも `ViewerIsAdmin` が false のままなので、
録画は 1 本も変わらない。**ここでゴールデンが落ちたら、変更が
`CanMergeAsAdmin()` の外に漏れている。** 差分を読んでから進む。

- [ ] **Step 7: コミット**

```bash
git add internal/app/presentation/tui/merge/render.go internal/app/presentation/tui/merge/merge_test.go
git commit -m "feat(merge): say on screen that a blocked merge can be pushed through"
```

---

### Task 7: ゴールデンに新しい状態を足す

**Files:**
- Modify: `internal/app/presentation/tui/merge/golden_test.go:72-133`
- Create: `internal/app/presentation/tui/merge/testdata/merge_admin_{en,ja}_{80,120,160}.golden`

- [ ] **Step 1: 状態を足す**

`golden_test.go` の `mergeBlockedModel` の直後に足す。

```go
// mergeAdminModel is held by a branch rule, with the viewer able to push past
// it: the one state where the popup offers a key while merging is blocked.
func mergeAdminModel(t *testing.T, width int) Model {
	return sized(t, &fakeSource{ctx: blocked()}, width)
}
```

`goldenStates` の `{"merge_blocked", mergeBlockedModel},` の直後:

```go
	{"merge_blocked", mergeBlockedModel},
	{"merge_admin", mergeAdminModel},
```

- [ ] **Step 2: 録画が無くて落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/merge/ -run TestGolden
```

期待: FAIL。`merge_admin_en_160` などの録画が無いという趣旨のエラーが 6 件

- [ ] **Step 3: 録る**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/merge/
```

- [ ] **Step 4: 録れたものを読む**

```bash
cat internal/app/presentation/tui/merge/testdata/merge_admin_ja_80.golden
```

確かめること（**目で読む。テストは形を見ていない**）:
- 「保護ルールに止められています」の下に「admin 権限で押し切れます」がある
- キーバーが `a:adminでマージ` と `esc:中止` の両方を含む
- `enter:マージ` が**無い**
- どの行も 80 桁に収まっている（`TestNothingOverrunsTheTerminal` が自動で見るが、
  折り返しの有無は目でも確かめる）

- [ ] **Step 5: 全部通ることを確かめる**

```bash
go test ./internal/app/presentation/tui/merge/
```

期待: PASS。`TestNothingOverrunsTheTerminal` と
`TestTheKeyBarNeverDropsTheWayOut` も新しい状態を自動で見る

- [ ] **Step 6: コミット**

```bash
git add internal/app/presentation/tui/merge/golden_test.go internal/app/presentation/tui/merge/testdata/
git commit -m "test(merge): record the popup that offers the admin key"
```

---

### Task 8: 仕上げ

**Files:** なし（確認のみ）

- [ ] **Step 1: CI と同じ検査を全部走らせる**

```bash
make check
```

期待: tidy / lint / fmt / test が全て通る。落ちたら直してから進む。

- [ ] **Step 2: 実機で見る（日本語）**

```bash
go run ./cmd/octoscope --lang ja --repo kukv/octoscope
```

blocked な PR を開いて `m` を押す。確かめること:
- キーバーの行が折り返していない（日本語は 1 文字 2 桁。ボックスは 50 桁）
- 「admin 権限で押し切れます」の行が理由の下に出ている

blocked な PR が無ければ、Task 0 と同じ手順で一時的に 1 つ作って確かめ、
確認後に閉じる。

- [ ] **Step 3: 実機で見る（英語）**

```bash
go run ./cmd/octoscope --lang en --repo kukv/octoscope
```

同じ画面で、`a:admin merge` が読めることを確かめる。

- [ ] **Step 4: 押し切ってみる**

Step 2 で作った一時 PR がまだ開いていれば、実際に `a` を押してマージされることを
確かめる。`required_signatures` に弾かれた場合は、フッターに GitHub の
メッセージがそのまま出ることを確かめる（設計 §4）。**どちらの結果でも
この計画としては正しい。** 弾かれたなら、それが出るべき場所に出ている。

---

## 範囲外（設計 §7）

- merge queue の迂回
- commit message の編集
- ポップアップ以外からの admin マージ
