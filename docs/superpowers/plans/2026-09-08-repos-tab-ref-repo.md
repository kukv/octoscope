# Repos タブから開いたビューでリポジトリ名が消える 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repos タブから開いた checks ビューと diff ビューのタイトルに、リポジトリ名が出るようにする。

**Architecture:** `internal/tui/repo` の `SelectedRef` が `gh.ItemRef.Repo` を空のまま返しているのが原因。ヘッダーに出しているリポジトリ名（`m.repoName`）を、そのまま ref に入れる。ビュー側の描画は変えない。

**Tech Stack:** Go 1.27.1 / Bubble Tea v2（`charm.land/*/v2`）/ `internal/golden`

**Spec:** 該当する設計記述は無い。`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.4.3 のモックアップが `┌─ kukv/octoscope #61 ─` とリポジトリ名込みのタイトルを描いており、**それが期待される見た目である**。

## 実測（2026-09-08、tmux 80x30 の本物の端末、`--lang ja`、kukv/octoscope#67）

`--repo kukv/octoscope` で起動すると Repos タブが開く。そこから `s` を押すと
checks ビューのタイトルが `kukv/octoscope #67` ではなく **` #67`**（先頭に空白 1 つ）になる。

```
 #67
失敗 0 · 実行中 0
Checks                │失敗ステップ────────────────────────────
CI #101               │
  ✓ lint  0:32        │
```

**golden では起きない。** 録っているモデルはどれも `Repo` の入った `gh.ItemRef` を
使っており、この経路を通らない。実端末で動かして初めて出た。

## Global Constraints

- **`make check` が通らない状態でコミットしない。**
- **`internal/tui` は `internal/gh/cli` を import しない**（depguard が落とす）
- **テストは状態を直接組み立てず、`Update` にメッセージやキーを渡して到達させる**
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
- コードのコメントは英語。書きすぎない。`//nolint` を新しく足さない
- **触るのは `internal/tui/repo` だけ。** 描画側（`checks/render.go`、`diff/render.go`）は
  正しい ref を渡されれば正しく描くので、直す理由が無い

---

## Decisions

### D1. `SelectedRef` を直す。描画側で空を場合分けしない

今の `SelectedRef` には「Repo stays empty: the list only ever shows the client's
repository」というコメントが付いており、**意図的に空にしている**。その前提は
バックエンドについては正しい（`internal/gh/cli` の `effectiveRepo` が空を
クライアントのリポジトリで補う）が、**ref は画面にも流れる**ことを見落としている。

描画側に `if Repo == ""` を足す案は採らない。ビューが 2 つ（checks と diff）あり、
今後増えるたびに同じ分岐が要る。**不完全な値を配って受け手全員に直させるより、
配る側が完全な値を作る。**

### D2. `repoName` が空のときは今の見た目のまま

`repoName` は `RepoName` の取得が失敗すると空になる（`repo.go` の
`fetchRepoName` が「名前はヘッダーの飾りなので失敗は報告しない」として空を返す）。
そのときは ref も空になり、**今の見た目に戻るだけ**で、今より悪くはならない。
そこに新しいエラー経路を作らない。

---

## File Structure

| ファイル | 役割 |
|---|---|
| `internal/tui/repo/repo.go`（修正） | `SelectedRef` が `Repo` を埋める |
| `internal/tui/repo/repo_test.go`（修正） | 名前が届いたあとの `SelectedRef` を見る |
| `internal/tui/app/scenario_test.go`（修正） | Repos タブから checks を開くとタイトルに名前が出る |

---

### Task 1: `SelectedRef` にリポジトリ名を入れる

**Files:**
- Modify: `internal/tui/repo/repo.go`（`SelectedRef`）
- Test: `internal/tui/repo/repo_test.go`, `internal/tui/app/scenario_test.go`

**Interfaces:**
- 変わる振る舞い: `(repo.Model).SelectedRef() (gh.ItemRef, bool)` の `Repo` が
  `m.repoName`（ヘッダーに出している名前）になる

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/repo/repo_test.go` に足す。**モデルは `Update` で作る**——
`repoNameMsg` と一覧の到着を流し込み、フィールドを手で立てない。

```go
// The ref leaves this view for the detail, diff and checks views, and those
// draw the repository in their titles. Leaving it empty put a bare " #67"
// at the top of the checks view (seen on a real terminal, 2026-09-08).
func TestTheSelectedRefCarriesTheRepositoryName(t *testing.T) {
	t.Parallel()

	m := loadedRepo(t) // 既存のヘルパーに合わせる。無ければ既存テストの作り方に倣う
	ref, ok := m.SelectedRef()
	if !ok {
		t.Fatal("SelectedRef reported nothing selected")
	}
	if ref.Repo == "" {
		t.Error("Repo is empty; the views that draw it print a bare number")
	}
}
```

`internal/tui/app/scenario_test.go` に、キー入力だけで通す 1 本を足す
（`.claude/rules/testing.md` の「画面をまたぐ操作はキー入力だけで通す」）。
既存のシナリオテストの形に合わせ、Repos タブで `s` を押したあと `View()` に
リポジトリ名が出ることを見る。既存のシナリオが Work タブから開いているなら、
Repos タブ版を 1 本足す。

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/tui/repo/ ./internal/tui/app/ -run 'TestTheSelectedRef|Scenario'`
Expected: FAIL（`Repo is empty`）

- [ ] **Step 3: 実装する**

`internal/tui/repo/repo.go`:

```go
// SelectedRef names the item under the cursor. ok is false when the tab is
// empty. The repository is the one this view is showing: the ref travels to
// the detail, diff and checks views, which draw it in their titles.
func (m Model) SelectedRef() (gh.ItemRef, bool) {
	if m.tab == tabPRs {
		if len(m.prs) == 0 {
			return gh.ItemRef{}, false
		}
		return gh.ItemRef{Kind: gh.ItemPR, Repo: m.repoName, Number: m.prs[m.cursors[tabPRs]].Number}, true
	}
	if len(m.issues) == 0 {
		return gh.ItemRef{}, false
	}
	return gh.ItemRef{Kind: gh.ItemIssue, Repo: m.repoName, Number: m.issues[m.cursors[tabIssues]].Number}, true
}
```

**古いコメントの「Repo stays empty」を残さない。** 事実でなくなる。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `make test`
Expected: PASS

**`m.ref` を比較しているコードが壊れないかを確かめる。** checks ビューも merge の
ポップアップも「別の PR 宛ての答えを捨てる」ために `msg.ref != m.ref` を見ている。
ref の中身が変わるので、**同じセッションの中で ref が途中から変わることが無いか**を
`grep -rn 'ref !=\|ref ==' internal/tui/` で洗い、テストが全部通ることで確かめる。

- [ ] **Step 5: テストが空振りでないことを確かめる**

`Repo: m.repoName` を `Repo: ""` に戻して、Step 1 の 2 本が落ちることを見る。見たら戻す。

- [ ] **Step 6: golden を確かめる**

```bash
go test ./internal/tui/...
git status --short internal/tui
```

**golden は 1 本も動かないはず**である（録っているモデルは Repos タブの
`SelectedRef` を通らない）。動いたら止めて、なぜ動いたかを調べる。

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal/tui docs
git commit -m "fix: keep the repository's name on the ref the Repos tab hands out"
```

---

## Self-Review

| 要求 | どのタスク |
|---|---|
| Repos タブから開いた checks / diff のタイトルにリポジトリ名が出る | Task 1 |
| 描画側に空の場合分けを増やさない | Task 1（D1） |
| 名前の取得が失敗したときに新しいエラー経路を作らない | Task 1（D2） |
| キー入力だけで画面をまたぐ経路が守られている | Task 1 Step 1 の scenario テスト |
