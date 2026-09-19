# Repos のサマリを Work のドロワーに揃える 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repos タブの表の下にある要約を、Work タブのドロワーと同じ部品にする。
見た目も高さも畳み方も揃える。

**これは設計の積み残しである。** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`
§「端末幅が足りない場合」は劣化順序の 3 番目に
「ドロワー・詳細ペインを畳む（100 桁未満。**Work / Repos**）」と書いている。
Repos にもドロワーがある前提の記述で、今の 4 行サマリはその簡易版。
同 §4.2 の「リストの下に選択中アイテムの要約（作成者、ブランチ、変更量、checks 一覧）を
固定の高さで出す」も、この計画で「Work と同じドロワー」と読み替える（Task 6 で追記する）。

**Architecture:** `work/drawer.go` を `internal/app/presentation/tui/drawer` パッケージに
出し、Work と Repos の両方がそれを呼ぶ。Repos は選択中の `domain.PR` / `domain.Issue` を
`domain.WorkItem` に組み替えて渡す。

**決めたこと（利用者に確認済み）:**

1. **幅は画面全幅。** サイドバーも跨いで下端に敷く。罫線も全幅。
   Work と同じ見た目になり、2 段組に幅が取れる。
   前回の「JoinPanes の前に埋める」は、表をサイドバーと結合したうえで
   ドロワーをその下に足す形に変わる。
2. **一覧クエリに `body` を足す。** Work の盤面クエリは既に取っており、
   本文プレビュー 3 行はドロワーの見た目の大半を占める。
3. **共通メタ行に `@作成者` を足す。** Work のドロワーも 1 要素増える。
   設計 §4.2 が作成者を挙げており、両タブが本当に同じ部品になる。

**Scope 外:** Search タブには要約が無い。足さない。

**Tech Stack:** Go, charm.land/bubbletea/v2, GraphQL（既存）

---

## ファイル構成

- 新規: `internal/app/presentation/tui/drawer/drawer.go` — `work/drawer.go` の移設
- 新規: `internal/app/presentation/tui/drawer/drawer_test.go`
- 変更: `internal/app/presentation/tui/theme/theme.go` — `Badges`
- 削除: `internal/app/presentation/tui/work/drawer.go`
- 変更: `internal/app/presentation/tui/work/render.go` — `badges` を落とし `drawer` を呼ぶ
- 変更: `internal/app/presentation/tui/repo/render.go` — `summary` / `fill` を落とし `drawer` を呼ぶ
- 変更: `internal/github/gql/repo_prs.graphql`, `repo_issues.graphql` — `body`
- 変更: `internal/github/gql/items_test.go` — `body` を数えるテスト
- 再生成: `internal/app/presentation/tui/{work,repo,root}/testdata/*.golden`
- 変更: `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.2

---

### Task 1: ラベルの列を theme に移す

`badges` は `work` と `repo` に同じものが 2 つある。ドロワーを別パッケージに
出すと 3 つ目が要るので、その前に 1 つにする。

**Files:**
- Modify: `internal/app/presentation/tui/theme/theme.go`
- Test: `internal/app/presentation/tui/theme/theme_test.go`
- Modify: `internal/app/presentation/tui/{work,repo}/render.go`

- [ ] **Step 1: 失敗するテストを書く**

```go
// TestBadgesDropsOneThatWouldBeCutInHalf is the rule both tabs drew by: a
// label that does not fit is left out rather than shown as a fragment.
func TestBadgesDropsOneThatWouldBeCutInHalf(t *testing.T) {
	// room をラベル 1 つぶんだけにして 2 つ渡し、1 つしか出ないことを見る
}

// TestBadgesFitsTheRoomItWasGiven guards the width the caller budgeted.
func TestBadgesFitsTheRoomItWasGiven(t *testing.T) {
	// ansi.StringWidth が room を超えないことを、全角のラベルでも見る
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/theme/ -run TestBadges`
Expected: FAIL — `undefined: theme.Badges`

- [ ] **Step 3: 通す最小の実装**

`work/render.go` の `badges` をそのまま `theme.Badges` として移す（doc コメントごと）。
`work` と `repo` の `badges` を消し、呼び出しを `theme.Badges` に差し替える。
2 つの実装は同一（`repo` 側は doc コメントの末尾が短いだけ）なので、
どちらを残すかで描画は変わらない。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/...`
Expected: PASS（golden も含めて描画は変わらない）

- [ ] **Step 5: コミット**

メッセージ: `refactor(theme): keep one row of label badges, not two`

---

### Task 2: ドロワーをパッケージに出す

**Files:**
- Create: `internal/app/presentation/tui/drawer/drawer.go`
- Create: `internal/app/presentation/tui/drawer/drawer_test.go`
- Delete: `internal/app/presentation/tui/work/drawer.go`
- Modify: `internal/app/presentation/tui/work/render.go`

この Task では**見た目を変えない**。移すだけ。作成者は Task 3 で足す。

- [ ] **Step 1: 失敗するテストを書く**

`drawer_test.go` に、移したあとの公開 API を呼ぶテストを書く。

```go
// TestRenderIsTheFixedHeight is what the caller budgets against: the drawer
// sits under a board whose length depends on its contents, and one that
// changed height would move the key bar under the user's eyes.
func TestRenderIsTheFixedHeight(t *testing.T) {
	// 本文が空のとき・長いとき・checks が多いときの 3 通りで
	// len(drawer.Render(it, 120)) == drawer.Height
}

// TestEmptyIsTheSameHeight covers the no-selection state.
func TestEmptyIsTheSameHeight(t *testing.T) {
	// len(drawer.Empty(120)) == drawer.Height
}

// TestRenderFitsTheWidth guards the two-pane split at every width the board
// is drawn at, in Japanese too: a full-width character takes two columns.
func TestRenderFitsTheWidth(t *testing.T) {
	// 100 / 120 / 160 で全行が width 以下
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/drawer/`
Expected: FAIL — パッケージが無い

- [ ] **Step 3: 通す最小の実装**

`work/drawer.go` を `drawer/drawer.go` に移し、`work` から切り離す。

- `func (m Model) drawer()` → `func Render(it domain.WorkItem, width int) []string`
  （`m.width` と選択中の item をもらう形にする）
- 選択が無いときの枝は `func Empty(width int) []string` に分ける
- `drawerHeight` → `Height`、`drawerMinColumns` → `MinColumns` として公開する
  （`work/render.go` の高さ予算と `drawerShown()` が読む）
- `summaryPane` / `metaLine` / `checksPane` / `bodyLines` / `blankLines` は非公開のまま移す
- `clip` は `work/render.go` にも要るので、`drawer` 側にも同じ 1 行の実装を置く
  （`layout.Clip` は 1 桁の余白を取る別物。混ぜない）

`work/render.go` 側は `drawer.Render` / `drawer.Empty` を呼ぶだけにする。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/...`
Expected: PASS。**golden が 1 つも動かないこと**が、移設だけであることの証拠。

- [ ] **Step 5: コミット**

メッセージ: `refactor(tui): move the drawer out of the work tab`

---

### Task 3: メタ行に作成者を足す

**Files:**
- Modify: `internal/app/presentation/tui/drawer/drawer.go`
- Test: `internal/app/presentation/tui/drawer/drawer_test.go`
- 再生成: `internal/app/presentation/tui/{work,root}/testdata/*.golden`

- [ ] **Step 1: 失敗するテストを書く**

```go
// TestTheMetaLineNamesTheAuthor is what spec §4.2 asks of the Repos block and
// what the Work drawer did not say: who wrote it is the first thing asked of
// an item someone else opened.
func TestTheMetaLineNamesTheAuthor(t *testing.T) {
	// Render の出力に "@kukv" が含まれること
}

// TestTheMetaLineLeavesOutAnAuthorItDoesNotHave keeps the separator from
// opening the line: a ghost account has no login.
func TestTheMetaLineLeavesOutAnAuthorItDoesNotHave(t *testing.T) {
	// Author が空のとき "@" も " · " も増えないこと
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/drawer/ -run TestTheMetaLine`
Expected: FAIL

- [ ] **Step 3: 通す最小の実装**

`metaLine` に `@login` を足す。順序は
`owner/name #番号 · @作成者 · head → base · +追加 −削除 · ラベル`。
参照が先頭なのは設計 §4.1 の並びを崩さないため。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/drawer/`
Expected: PASS

- [ ] **Step 5: golden を更新して読む**

Run: `OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/... -run TestGolden`

差分はメタ行に `@名前` が 1 つ増えるだけであること、
ja の 80 桁でメタ行が溢れていないことを見る。

- [ ] **Step 6: コミット**

メッセージ: `feat(drawer): name the author on the meta line`

---

### Task 4: 一覧クエリに本文を足す

**Files:**
- Modify: `internal/github/gql/repo_prs.graphql`, `internal/github/gql/repo_issues.graphql`
- Test: `internal/github/gql/items_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`items_test.go` には既に `wordBody` で `body` の出現数を数えるテストがある
（詳細クエリのもの）。同じやり方で一覧の 2 本にも足す。

```go
// TestTheListQueriesAskForTheBody is what the Repos drawer draws its preview
// from. A fixture-based test cannot notice the field being dropped, so the
// document text is what is checked.
func TestTheListQueriesAskForTheBody(t *testing.T) {
	// repoPRsQuery / repoIssuesQuery に \bbody\b が 1 つあること
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/github/gql/ -run TestTheListQueriesAskForTheBody`
Expected: FAIL — どちらの文書にも `body` が無い

- [ ] **Step 3: 通す最小の実装**

`PullRequestListFields` と Issue 側の対応する fragment に `body` を足す。
**なぜ取るのかをクエリのコメントに書く**（ドロワーの本文プレビュー）。

パース側（`gql.PullRequest` / `gql.Issue`）と `domain` への変換は
詳細取得が既に `Body` を運んでいるので手を入れない。**確かめてから進む。**

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/github/...`
Expected: PASS

- [ ] **Step 5: コミット**

メッセージ: `feat(gql): ask the list queries for the body`

---

### Task 5: Repos をドロワーに差し替える

**Files:**
- Modify: `internal/app/presentation/tui/repo/render.go`
- Test: `internal/app/presentation/tui/repo/table_test.go`

- [ ] **Step 1: 失敗するテストを書く**

既存の `TestTheSummaryBlockIsAlwaysTheSameHeight` と
`TestTheSummaryBlockSitsAboveTheKeyBar` は `summary()` / `summaryHeight` を
見ている。**消さずに** ドロワーを見る形に書き換える。そのうえで足す:

```go
// TestTheDrawerIsTheOneTheBoardDraws is the point of the change: the same
// lines, in the same colours, under both tabs.
func TestTheDrawerIsTheOneTheBoardDraws(t *testing.T) {
	// 選択中の PR から組んだ WorkItem を drawer.Render に渡した結果が、
	// View() の末尾（キーバーと空行を除いた drawer.Height 行）と一致すること
}

// TestTheDrawerSpansTheWholeWidth is what "the same as Work" means: the rule
// runs under the sidebar too, rather than starting at the table's edge.
func TestTheDrawerSpansTheWholeWidth(t *testing.T) {
	// 罫線の行が width 桁ぶんあり、行頭がサイドバーの空白でないこと
}

// TestTheDrawerFoldsAwayWhenNarrow is the spec's third degradation step
// (100 桁未満): the block goes and the table gets its rows back.
func TestTheDrawerFoldsAwayWhenNarrow(t *testing.T) {
	// 80 桁で drawer.Height ぶんの行が無く、visibleRows がその分増えること
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/repo/`
Expected: FAIL

- [ ] **Step 3: 通す最小の実装**

- `summary()` を消す。`summaryHeight` も消し、`drawer.Height` を使う
- `fill()` は `summary()` 専用だったので、これで使われなくなる。消す
- `drawerShown()` を足す: `m.width >= drawer.MinColumns`
- `selectedItem() (domain.WorkItem, bool)` を足す。選択中の `PR` / `Issue` を
  `WorkItem` に組み替える。`Ref.Repo` は `m.selectedRepo()`
- `View()`:

```go
	lines := append(m.header(), layout.PadLines(m.body(), m.visibleRows())...)
	if m.sidebarCols() > 0 {
		lines = layout.JoinPanes(m.sidebar(), lines, sidebarWidth)
	}
	if m.drawerShown() {
		// 全幅なので、サイドバーと結合したあとに足す
		if it, ok := m.selectedItem(); ok {
			lines = append(lines, drawer.Render(it, m.width)...)
		} else {
			lines = append(lines, drawer.Empty(m.width)...)
		}
	}
```

- 高さの予算を 2 つとも直す。どちらも**ドロワーが出ているときだけ**引く:
  - `visibleRows()`: `m.height - listTop - footerHeight - ドロワー - 通知`
  - `sidebarRows()`: `m.height - sidebarTop - addButtonHeight - footerHeight - ドロワー`

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/repo/`
Expected: golden 以外 PASS

`mouse_test.go` は 24 行の端末で当たり判定を見ている。`listTop` は動かないが
`visibleRows()` は縮むので、落ちたら**当たり判定ではなく予算のほうを疑う**。

- [ ] **Step 5: コミット**

メッセージ: `feat(repos): draw the board's drawer under the list`

---

### Task 6: golden と設計を合わせる

**Files:**
- Modify: `internal/app/presentation/tui/{repo,root}/testdata/*.golden`
- Modify: `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`

- [ ] **Step 1: 再生成する**

Run: `OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/... -run TestGolden`

- [ ] **Step 2: 差分を目視で確かめる**

**受け入れる前に読む。**
- 120 / 160 桁: 4 行のサマリが 6 行のドロワーになり、罫線が全幅になっている
- **80 桁: ブロックが丸ごと消え、表の行が 6 行増えている**（設計どおりの劣化。
  今まで実装されていなかった挙動なので、ここは目に見えて変わる）
- ja でドロワーの 2 段組が溢れていない

- [ ] **Step 3: 設計 §4.2 を直す**

「リストの下に選択中アイテムの要約（作成者、ブランチ、変更量、checks 一覧）を
固定の高さで出す」の一文を、Work と同じドロワーである旨に書き換える。
コードと設計を食い違わせたまま終わらせない。

- [ ] **Step 4: 全部通ることを確かめる**

Run: `make check`
Expected: tidy / lint / fmt / test すべて通る

- [ ] **Step 5: コミット**

`test(tui): record the Repos drawer` と
`docs(spec): the Repos block is the board's drawer` の 2 本に分ける。

---

### Task 7: 実際に起動して見る

**Files:** なし（CLAUDE.md の「TUI の変更は起動して見る」）

- [ ] **Step 1: 英語で見る**

Run: `go run ./cmd/octoscope --repo kukv/koto`

確かめること:
- 1 と 2 を往復して、下端のドロワーが同じ形・同じ高さに見える
- 本文プレビューが出ている（出ていなければ Task 4 が効いていない）
- Issue を選んだとき checks 側が空になり、崩れない
- 端末を 100 桁未満に縮めるとドロワーが消え、表が伸びる

- [ ] **Step 2: 日本語で見る**

Run: `go run ./cmd/octoscope --repo kukv/koto --lang ja`

全角の本文とラベルで 2 段組が溢れないことを見る。

- [ ] **Step 3: 直すところがあれば戻る**

見た目が揃っていなければ、`drawer` パッケージの中だけで直す。
片方のタブにだけ効く分岐を足したくなったら、それは設計に戻る合図。
