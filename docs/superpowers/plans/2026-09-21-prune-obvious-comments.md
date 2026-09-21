# 自明なコメントを消し、規約に基準を書く実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 3 つ。

1. `internal/` と `cmd/` の全 Go ファイルから**自明なコメントを消す**
2. その基準を `.claude/rules/go-style.md` に書く
3. 今回の整備（2026-09-20 / 09-21）で rules に足した**「○○時点では」的な文言を消す**

**Architecture:** コードは 1 行も変えない。コメントと rules だけ。

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** 利用者の指示（2026-09-21）。設計書は書かない。

---

## 着手前に調べた事実（2026-09-21）

### A. コメントの総量

| | ファイル | コメント行 | 全行 | 比率 |
|---|---|---|---|---|
| 非テスト | 119 | 3,378 | — | — |
| テスト | 121 | 3,073 | — | — |
| 合計 | 240 | 6,451 | 45,089 | 14% |

**コメントブロック（連続する `//` の塊）は 2,280 個**（非テスト 1,156 / テスト 1,124）。
判断の単位はファイルではなくブロックなので、全ブロックを 1 つのダンプに出して見る。

### B. revive の `exported` は全体除外されている

`.golangci.yml:303-307` が `^(exported|unused-parameter):` を除外している。
**公開識別子から doc コメントを消しても lint は落ちない。** これが成り立たなければ
この作業の大半はできなかった。

### C. このコードベースのコメントは、大半が「なぜ」を書いている

機械的に「名前を言い換えただけの 1 行 doc」を探すと 55 件出たが、**その多くは情報を持つ**。

- `// WorkItem is one card on the Work board.` — 型を画面上の何に対応づけるかを言っている
- `// S names a string variable.` — `S` という 1 文字の名前が何かを言っている
- `// fillLine draws s filled with bg the whole way across.` — この後に 20 行の理由が続く

**だから「全部消す」ではなく、線を引く必要がある。**

### D. 実際に自明なものの例（消す側）

```go
// Error styles a failure.
func Error() lipgloss.Style { return danger() }

// SelectedLine draws s as the selected row.
func SelectedLine(s string) string { return fillLine(s, selection()) }

// Init starts the fetch.
func (m Model) Init() tea.Cmd { ... }

// Render draws the selected item.
func (m Model) Render(...) string { ... }

// ActiveTab and InactiveTab style the tab row.
func ActiveTab() lipgloss.Style   { ... }

// markerStyle colours the +/- marker.
func markerStyle(...) lipgloss.Style { ... }

// DisableAutoMerge turns auto-merge back off.
func (g *Gateway) DisableAutoMerge(ctx context.Context, pr domain.PullRequestHandle) error
```

テスト側には**次の 1 行を読み上げているだけ**のものがある。

```go
// Assert the cursor is now on a rowLine.
// Move the cursor onto the second comment's row before closing it.
// move to wip and toggle on (add wip)
// next is the model after one key press.
```

### E. 残す側の例

```go
// WorkItem is one card on the Work board.          ← 型と画面の対応
// S names a string variable.                        ← 1 文字の名前の説明
// clamp keeps v within [0, hi].                     ← 範囲という仕様
// footerHeight is the blank line and the key bar.   ← 定数が何を数えているか
// GitHub asks every client to name itself; an unnamed one may be refused. ← 外部の事情
// 規約ファイル（.claude/rules/*.md）を引いているもの                     ← 根拠
// domain/handle.go, theme.go の fillLine のような理由の段落
```

### F. rules の日付つき文言

今回の整備（2026-09-20 / 09-21）で足したもの。**2 種類あって、腐り方が違う。**

| 種類 | 例 | 扱い |
|---|---|---|
| **その時点の状態** | `今日ある adapter 2 つ`、`今は薄い`、`実害はまだ無い`、`7 個ある（2026-09-20 に数えた）` | **コードが変われば嘘になる。これが「○○時点では」** |
| **決定の日付** | `2026-09-21 に移した`、`判断を 2026-09-07 にした` | 嘘にはならないが、履歴は git と計画にある |

**今回直すのは 2026-09-20 / 09-21 に足した分だけ。**

| ファイル | 行 | 何を |
|---|---|---|
| `architecture.md` | 46-52 | #128 で足した段落。**全部が「今の状態」の記述** |
| `architecture.md` | 116 | 「2026-09-21 に移した。それ以前は `domain.ParseDiff` だった」 |
| `architecture.md` | 143 | 「2026-09-20 に移る先が usecase から gateway になった」 |
| `architecture.md` | 150 | 「この層は今は薄い」 |
| `architecture.md` | 172 | 「`Change` は 7 個ある（2026-09-20 に数えた）」 |
| `architecture.md` | 268 | 「リポジトリ名の形を gateway に移したのは 2026-09-20 である」 |
| `architecture.md` | 279 | 「対 14 本を 6 本に畳んだのは 2026-09-20 である」 |
| `go-style.md` | 97 | 「2026-09-21 にレビュー・マージの書き込みへ通したのはこの理由で」 |
| `tui.md` | 40 | 「2026-09-21 に畳んだ。それ以前は…」 |
| `tui.md` | 44-50 | detail の 2 型を残した理由（日付は無いが「2026-09-21」を含む段落と地続き） |

**範囲外として残し、利用者に報告するもの**（今回の整備より前のもの。CLAUDE.md の
「触るべき箇所だけに触れる」に従う）:

- `architecture.md:56` 「実害はまだ無い」（over-match の話）
- `architecture.md:191 / 216 / 231` 「〜を入れる判断を 2026-09-07 / 09-13 にした」の 3 節
- `testing.md:98` 「1 列の段を録るために 50 桁を足している（2026-09-14）」

## Global Constraints

- **コードを 1 行も変えない。** 消すのはコメント、直すのは rules だけ
- **golden 354 枚を 1 バイトも変えない**
- **`//go:embed` などのディレクティブを消さない**
- **`.claude/rules/*.md` を引いているコメントを消さない。** それは根拠であって自明ではない
- **`//nolint` を消さない**
- 各タスクの終わりに `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

---

### Task 1: rules の日付つき文言を消す

**Files:**
- Modify: `.claude/rules/architecture.md`（事実 F の 7 箇所）
- Modify: `.claude/rules/go-style.md:97`
- Modify: `.claude/rules/tui.md:40`

- [ ] 「その時点の状態」は**時制の無い規則 + 理由**に書き換える。
      例: architecture.md:46-52 の段落は
      「domain を import する adapter は import cycle が先に止める。
      この deny が効くのは domain 非依存の adapter サブパッケージに対してである」になる
- [ ] 「決定の日付」は**落とす**。何が正しいかだけを残す。
      例: 「2026-09-21 に移した。それ以前は `domain.ParseDiff` だった」→ 削除
      （どこにあるかは同じ段落が既に書いている）
- [ ] 数を数えた記述（`Change` は 7 個）は**数を落とす**か、数えなくても言えることに置き換える
- [ ] **Verification:** `make check`。rules は CI の対象外だが、
      `.golangci.yml` を引いている記述と食い違っていないことを目で確かめる

**Commit:** `docs: take the dates and the counts out of the rules`

---

### Task 2: 自明なコメントを消す

ダンプを作って 2,280 ブロックを見る。ファイルを 1 つずつ開くのではなく、
**コメントを判断の単位にする。**

**Files:** `internal/**/*.go`、`cmd/**/*.go` のうち該当するもの

- [ ] 非テスト 1,156 ブロックを見て、消す候補に印を付ける
- [ ] テスト 1,124 ブロックを同じ基準で見る
- [ ] 消す。**1 コミットにまとめず、非テストとテストで分ける**——
      レビューする人がコードの doc とテストの意図を別々に見られるようにする
- [ ] **Verification:** 各コミットの前に `make check`。
      **`git diff` にコード行が 1 行も出ないこと**
      （`git diff -U0 -- '*.go' | grep '^[+-]' | grep -v '^[+-][+-]' | grep -v '^[+-]\s*//'` が空）

**Commit:** `refactor: drop the comments that only re-spell the name` /
`test: drop the comments that only read out the next line`

---

### Task 3: 基準を go-style.md に書く

**Files:**
- Modify: `.claude/rules/go-style.md`（「コードのコメントは英語で書く」の隣、77 行目付近）
- Modify: `.claude/rules/architecture.md`（「規約そのものを変える」の節に、
  規約に日付と「今の状態」を書かないことを足す）

- [ ] コメントの基準を書く。**判定の形にする**——
      「そのコメントを消しても、名前と署名だけで同じことが分かるなら消す」
- [ ] 消さないものも挙げる（理由、外部の事情、規約の引用、1 文字の名前の説明）
- [ ] 「規約に『今の状態』と日付を書かない」を `architecture.md` に足す。
      理由は「規約は今の形を規定するもので、いつそうなったかは git にある」
- [ ] **Verification:** `make check`

**Commit:** `docs: say which comments to write and which to leave out`

---

## 完了条件

- [ ] `make check` が通る
- [ ] golden 354 枚が 1 枚も変わっていない
- [ ] `git diff` にコード行が 1 行も無い
- [ ] `//go:embed` / `//nolint` / 規約を引いているコメントが残っている
- [ ] `.claude/rules/` に 2026-09-20 / 09-21 の日付が残っていない
- [ ] 範囲外にした古い日付つき記述を PR 本文に列挙した
