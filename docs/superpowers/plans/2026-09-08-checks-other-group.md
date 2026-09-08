# checks ビュー「その他」群 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 再実行できるワークフローの run を持たない check（GitHub App が作った check run と外部 CI の StatusContext）を「その他」の見出しの下にまとめ、直上の群の続きに見えないようにする。

**Architecture:** `internal/tui/checks/render.go` の `hasHeading` と `workflowTitle` だけを変える。`arrange` は既に run を持たない check を末尾へ寄せているので、並びは変えない。ドメイン型もバックエンドも触らない。

**Tech Stack:** Go 1.27.1 / Bubble Tea v2（`charm.land/*/v2`）/ `internal/golden`（`OCTOSCOPE_UPDATE_GOLDEN=1`）

**Spec:** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.4.3（この計画の Task 1 で直す）と `docs/superpowers/2026-09-08-phase3-checks-followups.md` の「設計の判断が要るもの」

## Global Constraints

- **`make check` が通らない状態でコミットしない。**
- **`internal/tui` は `internal/gh/cli` を import しない**（depguard が落とす）
- **グリフを直接書かない**（`internal/tui/icon`）。**色を直接書かない**（`internal/tui/theme` の役割名）
- **`View` は副作用を持たず、時計も読まない**（`.claude/rules/tui.md`）
- 画面に出す文字列は `internal/i18n` から引き、`active.en.yaml` と `active.ja.yaml` の**両方**に足す
- **テストは状態を直接組み立てず、`Update` にメッセージやキーを渡して到達させる**
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
- コードのコメントは英語。書きすぎない。`//nolint` を新しく足さない

---

## Decisions

### D1. 見出しの意味を「再実行できる run がある」から「群の名前」に変える

これまで spec §4.4.3 は、見出しをワークフロー名（`checkSuite.workflowRun.workflow.name`）と
実行番号で描くものとして書いており、**見出しがあること自体が「`R` で再実行できる」の印**に
なっていた。「その他」の見出しはその約束を破る。

**約束のほうを変える。** 見出しは群の名前であって、再実行できるかどうかは check ごとの
性質である。`R` は run を持たない check に対して既に `checks.decline_rerun_status_context` で
断っており（`internal/tui/checks/rerun.go`）、その振る舞いは変わらない。

代わりの案（右端に印を出す / インデントを外す）は採らない。ユーザーの判断（2026-09-08）で、
**見出しでまとめる形が選ばれている**。

### D2. 「その他」は 1 つの群にまとめる。App の check と外部 CI を分けない

どちらも「run が無いので再実行もログも無い」という同じ理由で同じ扱いを受ける。
利用者から見た違いは無く、2 つに割ると見出しが増えるだけである。

### D3. 並び順は変えない

`arrange` は既に `hasWorkflow` で run を持つ check を先に、その中で失敗したワークフローを
先頭に置いている。**この計画で `arrange` に触らない。** 見出しを 1 本足すだけで、
行の並びは今のままである。

---

## File Structure

| ファイル | 役割 |
|---|---|
| `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`（修正） | §4.4.3 のモックアップと箇条書き |
| `internal/tui/checks/render.go`（修正） | `hasHeading` と `workflowTitle` |
| `internal/tui/checks/checks_test.go`（修正） | 「その他」の見出しが出ることと、カーソル位置がずれないこと |
| `internal/tui/checks/testdata/checks_mixed_*.golden`（録り直し） | en / ja × 80 / 120 / 160 |
| `internal/i18n/locales/active.{en,ja}.yaml`（修正） | `checks.other` |
| `docs/superpowers/2026-09-08-phase3-checks-followups.md`（修正） | この判断が済んだことを書く |

---

### Task 1: spec を先に直す

**Files:**
- Modify: `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.4.3

**コードより先に spec を直す。** 実装が spec を追い越すと、次に読む人がどちらが正か
分からなくなる（`.claude/rules/` の「規約そのものを変える」と同じ理由）。

- [ ] **Step 1: モックアップを直す**

`● ci/circleci` を「その他」の見出しの下に移す。今のモックアップはこう:

```
│ CI                 │
│  ✗ test      2m14s │
│  ✓ lint      1m02s │
│ security           │
│  ✗ sca       0m48s │
│ ● ci/circleci      │
```

直したあと:

```
│ CI                 │
│  ✗ test      2m14s │
│  ✓ lint      1m02s │
│ security           │
│  ✗ sca       0m48s │
│ その他             │
│  ● ci/circleci     │
```

- [ ] **Step 2: 箇条書きを直す**

「所属するワークフローの無い StatusContext はグループを作らず末尾に並べる」を、
次の趣旨に書き換える。

- 再実行できるワークフローの run を持たない check は、**「その他」の見出しでまとめて末尾に置く**
- そこに入るのは 2 種類。外部 CI の StatusContext と、**GitHub App が作った check run**
  （`checkSuite.workflowRun` が `null` で返る。Codecov や Sonar がこれ）
- **見出しは群の名前であって、「再実行できる」の印ではない。** 再実行とログは
  check ごとの性質で、run を持たない check では `R` と `enter` が理由を出して断る

- [ ] **Step 3: コミット**

```bash
git add docs/superpowers/specs
git commit -m "docs: group the checks with no run of their own under a heading"
```

---

### Task 2: 「その他」の見出しを描く

**Files:**
- Modify: `internal/tui/checks/render.go`, `internal/i18n/locales/active.en.yaml`, `internal/i18n/locales/active.ja.yaml`
- Test: `internal/tui/checks/checks_test.go`

**Interfaces:**
- 変わる振る舞い: `(Model).hasHeading(i int) bool` が run を持たない群の先頭でも真を返す。
  `(Model).workflowTitle(r gh.CheckRun) string` がその群に `i18n.T("checks.other")` を返す

**足す i18n キー:** `checks.other` — en `Other`、ja `その他`

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/checks/checks_test.go` に足す。**モデルは既存のヘルパー（`fixture()` と
`Update`）を通して作る。**

```go
// The checks with no workflow run behind them -- a StatusContext, and a
// check run an App created -- used to read as the continuation of whatever
// group happened to sit above them.
func TestTheChecksWithNoRunOfTheirOwnGetTheirOwnHeading(t *testing.T) {
	t.Parallel()

	m := loadedMixed(t) // 既存のヘルパー。無ければ mixed の fixture を Update で流し込む
	view := m.View()
	if !strings.Contains(view, i18n.T("checks.other")) {
		t.Errorf("the heading for the checks with no run is missing:\n%s", view)
	}
}

// The cursor walks lines, and a heading is a line. One more heading above
// the last two checks moves them down by one.
func TestTheCursorReachesTheLastCheckPastTheNewHeading(t *testing.T) {
	t.Parallel()

	m := loadedMixed(t)
	for range len(m.order) - 1 {
		m, _ = m.Update(keyPress("j"))
	}
	if got := m.row; got != len(m.order)-1 {
		t.Errorf("row = %d, want %d: j must still reach the last check", got, len(m.order)-1)
	}
	if !strings.Contains(m.View(), i18n.T("checks.other")) {
		t.Error("the heading scrolled out from under the cursor")
	}
}
```

`loadedMixed` の作り方は `internal/tui/checks/golden_test.go` の `checksMixedModel`
（`checks_mixed_*.golden` を録っているモデル）に倣う。ヘルパーが golden 側にしか
無ければ、テストから呼べる場所に移す。

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/tui/checks/ -run 'TestTheChecks|TestTheCursorReaches'`
Expected: FAIL（見出しが無い）

- [ ] **Step 3: 実装する**

`internal/tui/checks/render.go`:

```go
// hasHeading reports whether the list draws a heading above order[i].
// arrange keeps the checks of one workflow together and the checks with no
// run of their own last, so the row before is enough to tell a new group
// from a continuing one.
func (m Model) hasHeading(i int) bool {
	r := m.order[i]
	if i == 0 {
		return true
	}
	prev := m.order[i-1]
	if !hasWorkflow(r) {
		// The first of them opens the group; the rest continue it.
		return hasWorkflow(prev)
	}
	return prev.Workflow != r.Workflow
}
```

**`prev.Kind != gh.CheckKindRun` の条件が消えることに注意。** `hasWorkflow` は
`Kind` と `RunID` の両方を見るので、`Kind` の比較は `hasWorkflow(prev)` に含まれている。

```go
// workflowTitle names the group a check belongs to. A check with no
// workflow run behind it has no name to take, so they share one heading.
func (m Model) workflowTitle(r gh.CheckRun) string {
	if !hasWorkflow(r) {
		return i18n.T("checks.other")
	}
	if r.RunNumber > 0 {
		return fmt.Sprintf("%s #%d", r.Workflow, r.RunNumber)
	}
	return r.Workflow
}
```

`internal/i18n/locales/active.en.yaml` と `active.ja.yaml` の `checks:` の下に
`other` を足す（en `Other` / ja `その他`）。

**`rerun.go` が `workflowTitle` を呼んでいる箇所を確認する。** 再実行のポップアップは
run を持つ check にしか開かないので「その他」を表示することは無いはずだが、
`startRerun` の断り方（`r.Kind == gh.CheckKindStatus || r.RunID == 0`）を読んで
それが本当かを確かめてから進む。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `go test ./internal/tui/checks/ ./internal/i18n/`
Expected: PASS

- [ ] **Step 5: テストが空振りでないことを確かめる**

`workflowTitle` の `if !hasWorkflow(r)` の枝を消して
`TestTheChecksWithNoRunOfTheirOwnGetTheirOwnHeading` が落ちること、
`hasHeading` の `return hasWorkflow(prev)` を `return false` に戻して
同じテストが落ちることを見る。見たら戻す。

- [ ] **Step 6: golden を録り直して目で見る**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/checks/
git diff --stat internal/tui/checks/testdata
cat internal/tui/checks/testdata/checks_mixed_ja_80.golden
```

差分が `checks_mixed_*` の 6 本だけであることを確かめる。**ja の 80 桁を目で見て**、
「その他」の見出しが群の先頭に 1 本だけ出ていること、枠が崩れていないこと、
キーバーが折り返していないことを確認する。他の golden が動いていたら止めて調べる。

- [ ] **Step 7: `make check` とコミット**

```bash
make check
git add internal docs
git commit -m "feat: give the checks with no run of their own a heading"
```

---

### Task 3: 積み残しを閉じる

**Files:**
- Modify: `docs/superpowers/2026-09-08-phase3-checks-followups.md`

- [ ] **Step 1: 「設計の判断が要るもの」の節を閉じる**

判断が済んだので、節を「決まったこと」に書き換える。**何をどう決めたかと、
その理由**を残す（この文書は「直さないと決めた理由も書く」という約束で書かれている）。

- 決めたこと: 「その他」の見出しでまとめる
- 見出しの意味が「再実行できる run がある」から「群の名前」に変わったこと
- 採らなかった案（右端に印 / インデントを外す）

書き終えたあと、この文書に**未決の項目が残っていないなら**、その旨を冒頭に書く。

- [ ] **Step 2: コミット**

```bash
make check
git add docs
git commit -m "docs: record how the checks with no run are told apart"
```

---

## Self-Review

| 要求 | どのタスク |
|---|---|
| run を持たない check が直上の群の続きに見えない | Task 2 |
| App が作った check run と外部 CI が同じ扱いを受ける | Task 2（`hasWorkflow` が両方を拾う） |
| 見出しの意味の変更が spec に書かれている | Task 1 |
| golden が en / ja × 80 / 120 / 160 で録り直されている | Task 2 Step 6 |
| 積み残しの記録が閉じている | Task 3 |
