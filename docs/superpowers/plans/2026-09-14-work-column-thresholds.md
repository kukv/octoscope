# Work タブ: 列が変わる幅と、枠の常時化 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 列数が変わる幅を上げ、カードの枠をどの幅でも描く。

**Architecture:** `internal/app/presentation/tui/work` の描画の中だけで閉じる。
しきい値の定数と `boxed()` を変え、枠なしの描画経路を消す。

**Tech Stack:** Go 1.25 / Bubble Tea v2 / lipgloss v2 / golden テスト

**Spec:** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.1 / §4.6
（この計画は §4.6 の劣化の段階を変える。タスク 4 で追従させる）

**Branch:** `feat/work-card-layout`（PR #93。同じカードの見た目を触るので続ける）

## なぜ（実測 2026-09-14）

カードの 1 行目 `• kukv/octoscope #12` は 20 桁を使う。4 列のときのカードの内側は
`(W - 9) / 4 - 4` 桁しかない。

| 端末幅 | 4 列のときの内側 | 20 桁は |
|---|---|---|
| 100 | 18 | 入らない |
| 120 | 23 | 入る |
| 140 | 28 | 余裕 |

4 列が実用になるのは 120 桁から。100〜119 桁は「4 列だがリポジトリ名が読めない」帯に
なっていた。1 列と 2 列の境も同じ理由で 60 → 80 に上げる（幅 60 の 2 列は
1 枚 28 桁、内側 24 桁で、上の 1 行目がぎりぎりしか入らない）。

## 決めたこと

**段は 4 つ。列数のしきい値と drawer のしきい値を分ける。**

| 幅 | 列数 | 枠 | drawer |
|---|---|---|---|
| < 80 | 1（ページング） | あり | なし |
| 80 – 99 | 2（ページング） | あり | なし |
| 100 – 119 | 2（ページング） | あり | あり |
| ≥ 120 | 4 | あり | あり |

**枠はどの幅でも描く。** 枠を外していたのは「4 列だと 1 列 17 桁しかなく、枠の 2 桁が
致命的」だったからで、列数が幅に追従するようになった今、そこまで細い列は存在しない。
いちばん細いのは幅 80 の 2 列で、1 枚 38 桁・内側 34 桁ある。

枠が常時になると次が不要になり、消す。

- `boxed()` の分岐と、`card()` の枠なしの経路
- `cardHead` / `cardTitle` の `marker` 引数と、`▸` のカーソル記号
  （選択は枠の色と背景が示す。設計 §4.1）

**drawer は 100 桁のまま。** drawer は左右 2 段組で、幅 100 なら左 58 桁・右 39 桁。
列数とは別の都合なので、列数のしきい値に付き合わせない。

## Global Constraints

- 各タスクの末尾で `make check` が緑。緑でない状態でコミットしない
- 桁数は必ず `ansi.StringWidth` で数える（`.claude/rules/tui.md`）
- 当たり判定は描画と同じ関数を読む。レイアウトを 2 度計算しない
- 自分の変更で使われなくなったものは消す（枠なしの経路、`▸` の記号、`marker` 引数）
- コメントは外部の事情・正しい理由・doc の 3 つだけ（`.claude/rules/go-style.md`）

## ファイル構成

| ファイル | 変更 |
|---|---|
| `work/render.go` | しきい値の定数、`boxed()` の削除、`card` / `cardHead` / `cardTitle` の整理 |
| `work/render_test.go` | 幅ごとの列数、枠が常にあること |
| `work/mouse_test.go` | 幅の指定（80 が 2 列になる） |
| `work/golden_test.go` | `goldenWidths` を 4 つの段に合わせる |
| `*/testdata/*.golden` | 録り直し |
| 設計書 §4.1 / §4.6 | 枠の常時化と、劣化の段階 |

## Task 1: しきい値

**Files:** `work/render.go`, `work/render_test.go`

```go
const (
	// twoColumnsBelow and singleColumnBelow are where the board stops fitting
	// four and two columns. A card's head line — the marker, "owner/name" and
	// the number — wants about twenty columns, and four columns leave a card
	// (W-9)/4-4 columns wide: eighteen at a hundred, twenty-three at a
	// hundred and twenty.
	twoColumnsBelow   = 120
	singleColumnBelow = 80

	// drawerMinColumns is the drawer's own threshold: it is two panes side by
	// side, and a hundred columns leaves them fifty-eight and thirty-nine.
	drawerMinColumns = 100
)
```

**Steps:**

- [ ] `render_test.go` の `TestHowManyColumnsFitTheWidth` の表を
      `{50,1} {79,1} {80,2} {110,2} {119,2} {120,4} {160,4}` に書き換える
- [ ] 幅 110 で drawer が出て、幅 80 では出ないことのテストを書く
      （2 列でも drawer が出る帯があることを押さえる）
- [ ] 定数を直して通す
- [ ] `mouse_test.go` の幅の前提を見直す。`TestATwoColumnBoardHitTestsTheSecondPage`
      は 80 のままで良い（今も 2 列）。`TestClickingACardSelectsIt` の 120 は
      4 列のままである
- [ ] `make check`（golden はまだ赤い）

**Verify:** 列数と drawer のテストが緑

## Task 2: 枠を常時にする

**Files:** `work/render.go`, `work/render_test.go`

**Steps:**

- [ ] `render_test.go` に、**どの幅でもカードに枠があり、高さが `cardHeight()` に
      等しい**ことのテストを書く（幅 50 / 80 / 110 / 160）。先に幅 50 で
      今は枠が無いことを確かめてから書く
- [ ] `TestANarrowCardLosesItsBox` を消す。枠を外す段が無くなる
- [ ] `boxed()` を消し、`cardHeight()` を `titleLines + 4` 一本にする
- [ ] `card()` の枠なしの経路を消す
- [ ] `cardHead` / `cardTitle` から `marker` 引数を落とす。`▸` は使われなくなる
- [ ] 呼び出し側（`card`、テスト）を直す
- [ ] `make check`（golden はまだ赤い）

**Verify:** `grep -rn '▸' internal/app/presentation/tui/work/` が `icon.Collapsed` の
経路以外に出てこない。`gutter` は見出しと読み込み中の行でまだ使われている

## Task 3: アサーションが空振りしていないことを確かめる

**Files:** なし（一時的な変更のみ）

**Steps:**

- [ ] しきい値を 100 に戻して、Task 1 の列数のテストが落ちることを見る
- [ ] `cardHeight()` を 1 減らして、Task 2 の高さのテストが落ちることを見る
- [ ] 元に戻して `make check`

**Verify:** 2 つとも落ちた

## Task 4: golden の録り直しと設計書の追従

**Files:** `work/golden_test.go`, `*/testdata/*.golden`, 設計書 §4.1 / §4.6

**Steps:**

- [ ] `goldenWidths` を `{160, 120, 110, 80, 50}` にする。4 つの段（1 列 / 2 列・
      drawer なし / 2 列・drawer あり / 4 列）と、120 の境目を 1 枚ずつ録る。
      `.claude/rules/testing.md` の「段ごとに 1 つずつ録る」に従う
- [ ] `make golden` で録り直し、差分を 1 枚ずつ目で見る。とくに幅 50 と 80
      （枠が付く）と 110（新しい帯）
- [ ] 設計書 §4.1 から「枠を外す」前提を消す
- [ ] 設計書 §4.6 の劣化の段階を 4 つに直す。枠を外す段は無くなる
- [ ] `make check`

**Verify:** `make check` が緑。設計書とコードが一致する

## Task 5: 実際に起動して見る

**Files:** なし

**Steps:**

- [ ] `go run ./cmd/octoscope --lang ja` を 160 / 120 / 110 / 80 / 50 桁で動かす
- [ ] 幅 50 の 1 列で枠が窮屈すぎないか
- [ ] 幅 120 の 4 列でリポジトリ名が読めるか（この計画の目的）
- [ ] 選択中のカードが枠の色と背景で分かるか（`▸` が無くなるため）

**Verify:** 5 つの幅 × 2 言語で崩れがない
