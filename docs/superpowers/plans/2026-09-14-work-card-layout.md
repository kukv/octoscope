# Work タブ: 2 列の段階とタイトル 2 行 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 幅が中途半端な端末で Work のカードに載る情報を増やす。列数の段階に 2 列を足し、
カードのタイトルを 2 行にする。

**Architecture:** どちらも `internal/app/presentation/tui/work` の描画の中だけで閉じる。
列数は `visibleSections()` が返す窓の大きさで決まり、当たり判定の `columnAt()` が同じ窓を読む。
カードの高さは `cardHeight()` ひとつが決めており、そこに 1 行足す。
データ取得も usecase も domain も触らない。

**Tech Stack:** Go 1.25 / Bubble Tea v2 / lipgloss v2 / golden テスト

**Spec:** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.1 / §4.6
（この計画は両方を変える。タスク 4 で設計書を追従させる）

**Branch:** `feat/work-card-layout`（`main` から作成する）

## なぜ（実測 2026-09-14）

`testdata/work_ja_80.golden` を見ると、幅 80 桁のカードのタイトルは 7 桁しか残っていない。

```
▸ • #12 fix th…      ← 英語で 7 文字
  • #999 レン…       ← 日本語で 3 文字
```

列幅は `(80 - 3×3) / 4 = 17` 桁。そこから gutter 2 桁と `• #999 ` の 8 桁が引かれる。
タイトルが何の PR か言えていない。

この計画の後の見込み。1 行目は marker と番号に食われ、2 行目は `cardTitle` が
受け取る幅（枠なしなら列幅 − gutter、枠ありなら inner）を丸ごと使う。

| 幅 | 列数 | 列幅 | 1 行目 | 2 行目 | 合計 |
|---|---|---|---|---|---|
| 59 | 1 | 59 | 49 | 57 | 106 |
| 60 | 2 | 28 | 18 | 26 | 44 |
| 80 | 2 | 38 | 28 | 36 | 64（今は 7） |
| 99 | 2 | 48 | 38 | 46 | 84 |
| 120 | 4・枠あり | 27 | 15 | 23 | 38（今は 15） |
| 160 | 4・枠あり | 37 | 25 | 33 | 58 |

## Global Constraints

- 各タスクの末尾で `make check` が緑。緑でない状態でコミットしない
- 文字列を足すなら `en` / `ja` 両方のカタログへ。ただしこの計画では新しい文字列は足さない
  （`work.column_position` を「カーソルが 4 列中の何列目にいるか」として使い回す）
- 桁数は必ず `ansi.StringWidth` で数える（`.claude/rules/tui.md`）
- 色は `theme` から引く。16 進をビューに書かない
- コメントは外部の事情・正しい理由・doc の 3 つだけ（`.claude/rules/go-style.md`）
- **当たり判定は描画と同じ関数を読む。** `mouse.go` の冒頭のコメントが縛っている。
  レイアウトを 2 度計算しない
- テストは「主張する状態に到達している」ことを先に確かめる。カードが 1 枚しかない
  モデルでスクロールを主張しない（`tests-that-cannot-fail`）

## 決めたこと

**列数は 1 / 2 / 4。3 列は入れない。** セクションが 4 つなので、2 列ならページが
`[Review requested, Your PRs]` と `[Assigned, Mentioned]` にきれいに割れる。
3 列は割れず、最後のページが 1 列だけになる。

**しきい値は 60 と 100。既存の 2 つをそのまま使う。**

| 幅 | 列数 | 枠 | drawer |
|---|---|---|---|
| < 60 | 1（ページング） | なし | なし |
| 60 – 99 | **2（ページング）** | なし | なし |
| ≥ 100 | 4 | あり | あり |

`drawerMinColumns = 100` の「1 列が 17 桁しかない」という理由づけは 4 列のときの話なので、
2 列の段では成り立たない。それでも枠のしきい値は 100 のまま動かす理由がない
（枠は幅ではなく高さを 2 行食う。60 桁の端末は高さも狭いことが多い）。
コメントはその旨に直す。

**タイトルの 2 行目はインデントしない。** `• #999 ` の 8 桁分下げると読みやすいが、
狭いカードでは残る桁のほうが効く。2 行目はカードの左端（枠なしなら gutter の右）から始める。

**「枠なしの 4 列」という段は消える。** 60〜99 桁が 2 列になるので、4 列と枠ありが
同じ条件になる。`drawerMinColumns` のコメント（「1 列が 17 桁しかない」）と
`goldenWidths` のコメント（「3 つの段」）は、どちらも無くなる状態を説明しているので直す。

**タイトルが 1 行に収まっても 2 行目は空ける。** カードの高さが可変になると、
`cardHeight()` を割り算に使っている `visibleCards` / `cardWindow` / `cardAt` が全部崩れる。
短いタイトルが並ぶ列では空行が出るが、GitHub のタイトルは折り返す長さが大半である。

## ファイル構成

| ファイル | 変更 |
|---|---|
| `work/render.go` | `visibleSections` に 2 列の窓、`cardHeight` に +1、`cardTitle` を 2 行化 |
| `work/mouse.go` | `columnAt` が窓の中の位置を絶対の列番号に直す |
| `work/render_test.go` | 幅ごとの列数、タイトルの折り返し、桁の検証 |
| `work/mouse_test.go` | 2 列のときのクリック位置 |
| `work/golden_test.go` | `goldenWidths` に 50 を足す（1 列の段が録れていない） |
| `work/testdata/*.golden` | 録り直し |
| `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` | §4.1 / §4.6 |

## Task 1: 2 列の段階

**Files:** `work/render.go`, `work/render_test.go`

`render.go` に窓の計算を足す。

```go
// columnsFor is how many columns fit side by side. The sections divide into
// four, two and one, so every tier pages by whole pages; three columns would
// leave a page with one column in it.
func (m Model) columnsFor() int {
	switch {
	case m.width < singleColumnBelow:
		return 1
	case m.width < drawerMinColumns:
		return 2
	default:
		return m.columns()
	}
}

// visibleSections is the window of columns on screen: the page the cursor's
// column falls in. h/l move the cursor, and the page follows it.
func (m Model) visibleSections() []domain.WorkSection {
	n := m.columnsFor()
	all := domain.WorkSections()
	page := m.col / n
	return all[page*n : min((page+1)*n, len(all))]
}
```

`boardTop()` は `len(m.visibleSections()) < m.columns()` のままで、2 列でも
「2/4 列目」の行が出る。`columnWidth(n)` も引数で受けているので変更なし。

**Steps:**

- [ ] `render_test.go` に幅ごとの列数のテストを書く。幅 50 / 59 / 60 / 80 / 99 / 100 / 160 で
      `View()` を描き、縦罫線 `│` の本数が 0 / 0 / 1 / 1 / 1 / 3 / 3 であることを見る
- [ ] 2 列のときに `m.col` が 0〜3 のそれぞれで、見出しに出るセクション名が
      `[レビュー依頼, 自分の PR]` / `[担当, メンション]` に切り替わるテストを書く
      （4 つの見出しが全部違う文字列であることを先に確かめる）
- [ ] 幅 80 で「2/4 列目」の行が出ることのテストを書く（`work.column_position`）
- [ ] `columnsFor` / `visibleSections` を実装し、上記を通す
- [ ] `make check`

**Verify:** 上のテストが赤→緑。既存の `render_test.go:387` の 1 列の主張が
まだ通る（幅の指定が 60 未満であることを確かめる。60 以上なら 50 に直す）

## Task 2: 当たり判定を窓に合わせる

**Files:** `work/mouse.go`, `work/mouse_test.go`

`columnAt` は今「窓が全列でなければ `m.col` を返す」＝ 1 列前提になっている。
窓の中の位置 `i` を絶対の列番号に直す。

```go
	n := len(sections)
	page := m.col / n
	return page*n + i, true
```

`cardAt` は `domain.WorkSections()[col]` を引いているので、col が絶対番号になれば
そのまま正しくなる。

**Steps:**

- [ ] `mouse_test.go` に、幅 80・`m.col = 2` の盤面で右側の列のカードをクリックすると
      `col == 3` が選ばれるテストを書く（先に左の列で `col == 2` も見る）
- [ ] 列と列の隙間（縦罫線の上）と、最後の列の右外はこれまで通り `ok == false` であることを見る
- [ ] `columnAt` を直して通す
- [ ] ホイールも同じ `columnAt` を通るので、幅 80 で右の列にホイールを当てると
      その列にカーソルが移るテストを書く
- [ ] `make check`

**Verify:** `go test ./internal/app/presentation/tui/work/ -run Mouse -v` が緑

## Task 3: タイトル 2 行

**Files:** `work/render.go`, `work/render_test.go`

`cardHeight()` に 1 行足す。

```go
// titleLines is fixed rather than fitted to the title: visibleCards,
// cardWindow and the mouse hit-test all divide by the card height, and a card
// that changed height would put them out by however many short titles were
// above the pointer.
const titleLines = 2

func (m Model) cardHeight() int {
	if m.boxed() {
		return titleLines + 3 // the meta line and the box's two borders
	}
	return titleLines + 1
}
```

`cardTitle` は `[]string` を返すようにし、`card` がそれを並べる。折り返しは
1 行目の幅で折り、残りを 2 行目の幅に詰める。

```go
// wrapTitle folds the title over two lines. The first line is what is left
// beside the marker and the number; the second gets the whole width, so the
// wrap is computed twice rather than once.
func wrapTitle(title string, first, second int) (string, string) {
	head := strings.Split(ansi.Wrap(title, max(first, 1), ""), "\n")[0]
	rest := strings.TrimSpace(strings.TrimPrefix(title, head))
	return head, clip(rest, second)
}
```

`ansi.Wrap` は語で折り、語が入らなければ桁で切る。日本語は空白が無いので桁で折れる。

**2 行目は折り返した残りの行を連結して作らない。** 日本語は桁で切られるので、
`strings.Join(lines[1:], " ")` は元のタイトルに無い空白を挟む。1 行目を前方一致で
剥がして残りを取る。この前提（1 行目が必ずタイトルの接頭辞であること）は
下のテストで確かめる。

選択中の色は今まで通りタイトルの部分にだけ当てる（2 行とも）。

**Steps:**

- [ ] `render_test.go` に、幅 80 で長い英語のタイトルが 2 行に分かれて
      **1 行目と 2 行目のどちらにも語が載っている**ことのテストを書く
      （2 行目が空でないことを主張する。空でも通るテストにしない）
- [ ] 同じく日本語のタイトルで、各行の `ansi.StringWidth` が列幅を超えないテストを書く
- [ ] **2 行に収まる日本語のタイトルで `ansi.Strip(1 行目) + ansi.Strip(2 行目) == タイトル`**
      であることのテストを書く。幅の主張だけでは、連結で空白が混ざるバグも
      前方一致が外れて 1 行目が二重に出るバグも素通りする
- [ ] 短いタイトルのカードでも高さが `cardHeight()` ちょうどであることのテストを書く
- [ ] 幅 160（枠あり）で、枠の中の行数が `cardHeight()-2` であることのテストを書く
- [ ] `titleLines` / `wrapTitle` / `cardTitle` / `card` を実装して通す
- [ ] `visibleCards` が 1 枚は返す floor（`boardHeight` の `max`）がまだ効いていることを、
      高さ 10 の盤面で `View()` が破綻しないテストで見る
- [ ] `make check`

**Verify:** `go test ./internal/app/presentation/tui/work/` が緑。
`cardAt` の y 計算が `cardHeight()` を読んでいるので、Task 2 のマウステストも緑のまま

## Task 4: golden の録り直しと設計書の追従

**Files:** `work/golden_test.go`, `work/testdata/*.golden`,
`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`

**Steps:**

- [ ] `goldenWidths` を `{160, 120, 80, 50}` にする。80 が 2 列に変わったので、
      1 列の段を録るものが他に無い。コメントも「4 つの段」に直す。
      これは `.claude/rules/testing.md` の「en / ja × 80 / 120 / 160 桁」からの逸脱なので、
      規約側に「段が 3 つを超えるビューは段ごとに録る」の一文を足す提案も同じ PR に含める
- [ ] `make golden` で録り直す（`OCTOSCOPE_UPDATE_GOLDEN=1 go test ./...`）。
      **差分を 1 枚ずつ目で見る**。特に幅 80（4 列→2 列）と幅 50（新規）。
      `cat -v testdata/work_ja_80.golden` で桁ずれを見る
- [ ] 設計書 §4.1 の「中身は 2 行」を「タイトル 2 行 + メタ 1 行の 3 行」に直す
- [ ] 設計書 §4.6 の劣化の段階に 2 列を足す。今は「1 列ずつのページング（60 桁未満）」しか
      書かれていない
- [ ] `make check`

**Verify:** `git diff --stat` の golden の変更が想定の枚数。設計書とコードが一致する

## Task 5: 実際に起動して見る

**Files:** なし

`.claude/rules/tui.md` の「確認」と `tui-never-ship-without-rendering`。
テストが緑なだけでは完了にしない。

**Steps:**

- [ ] `go run ./cmd/octoscope` を 160 / 120 / 80 / 50 桁の端末幅で動かす
- [ ] `go run ./cmd/octoscope --lang ja` で同じ 4 つを見る。全角で桁が押されていないか
- [ ] 幅 80 で `h` / `l` を押し、ページが 2 列ずつ切り替わり「N/4 列目」が合っているか
- [ ] 幅 80 でカードをクリックし、狙った列・狙った行が選ばれるか
- [ ] タイトルが 1 行で収まるカードと折り返すカードが並んだときの見え方を確認する

**Verify:** 4 つの幅 × 2 言語で崩れがない
