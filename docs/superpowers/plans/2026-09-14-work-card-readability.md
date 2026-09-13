# Work タブ: カードの並べ替えと記号 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** カードを読みやすくする。リポジトリ名を owner ごと 1 行目へ出し、タイトルを
その下に独立させる。未レビューの `•` を黄色にし、Issue の記号を `⦿` にする。

**Architecture:** `internal/app/presentation/tui/work` の描画と、`icon` / `theme` の
語彙だけを変える。カードの高さは今までどおり `cardHeight()` ひとつが決める。
データ取得も usecase も domain も触らない。

**Tech Stack:** Go 1.25 / Bubble Tea v2 / lipgloss v2 / golden テスト

**Spec:** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.1 / §4.5
（この計画は §4.1 のカードの構成を変える。タスク 5 で追従させる）

**Branch:** `feat/work-card-layout`（PR #93。未マージで、同じカードの見た目を触るので続ける）

## 今のカードと、変えたあとのカード

今（幅 80・2 列・枠なし）:

```
▸ • #12 fix the thing

  octoscope ▰▰▱▱▱▱▱  bug  3 時間前
```

変えたあと:

```
▸ • kukv/octoscope #12
  fix the thing

  ▰▰▱▱▱▱▱ 3 時間前  bug   ci
```

1 行目に「何の状態か・どのリポジトリか・何番か」、2〜3 行目にタイトル、
4 行目に CI と経過時間とラベル。リポジトリ名は owner を含む全体を出す
（今は owner を落としていた）。

## 決めたこと

**カードは 4 行（枠あり 6 行）。** 見出し 1 行 + タイトル 2 行 + メタ 1 行。
ラベルはメタ行に残す。ラベルを独立した行にすると 5 行（枠あり 7 行）になり、
高さ 40 の端末で 1 列に見えるカードが 6 枚から 4 枚に落ちる。列の長さが滞留量を表すのが
この盤面の要点なので、そこは 5 枚で止める。

| | 今 | 変えたあと |
|---|---|---|
| 枠なしのカード | 3 行 | 4 行 |
| 枠ありのカード | 5 行 | 6 行 |
| 高さ 40・幅 120 で見えるカード | 6 枚 | 5 枚 |

**1 行目が入りきらないときはリポジトリ名を切り詰め、番号は必ず残す。**
番号がそのカードの識別子で、切れた番号は何の役にも立たない。
幅 120 の 4 列（中身 23 桁）なら `• kukv/octosc… #12` になる。

**`•` はどちらの未レビュー状態でも黄色にする。** `domain.ReviewRequired`
（レビュー必須でまだ承認なし）はすでに黄色だが、`domain.ReviewNone`
（GitHub がレビュー判定を持っていない）は灰色だった。利用者から見ればどちらも
「まだ誰も見ていない」で、記号も同じ `•` なのに色だけ違うのは読めない。

**Issue の記号は `⦿`。** `ansi.StringWidth("⦿")` は 1（2026-09-14 に実測）。
ASCII セットは draft と Issue がどちらも `o` で衝突しているので、Issue を `@` にする。
この計画は「Issue が一目で分かること」を目的にしているので、衝突はその範囲に入る。
Nerd Font セットは専用のグリフを持っているのでそのまま。

**メタ行の並びは CI バー → 経過時間 → ラベル。** 今はリポジトリ名 → バー → ラベル →
経過時間だった。リポジトリ名が 1 行目へ移り、経過時間が前に出る。

## Global Constraints

- 各タスクの末尾で `make check` が緑。緑でない状態でコミットしない
- 桁数は必ず `ansi.StringWidth` で数える（`.claude/rules/tui.md`）
- 色は `theme` にだけ書く。16 進をビューに書かない
- 記号は `icon` にだけ書く。3 セットすべてに足し、どれも 1 桁に保つ
  （`TestEveryGlyphIsOneColumn` が縛っている）
- 当たり判定は描画と同じ関数を読む。レイアウトを 2 度計算しない
- コメントは外部の事情・正しい理由・doc の 3 つだけ（`.claude/rules/go-style.md`）
- 自分の変更で使われなくなったものは消す。`shortRepo` はこの計画で不要になる

## ファイル構成

| ファイル | 変更 |
|---|---|
| `icon/icon.go` | Unicode の `issue` を `⦿`、ASCII の `issue` を `@` |
| `theme/theme.go` | `Review` が `ReviewNone` にも `attention()`（黄）を返す |
| `work/render.go` | `cardHead` を足し、`cardTitle` からヘッダを外し、`cardMeta` から
リポジトリ名を外す。`cardHeight` に +1。`shortRepo` を削除 |
| `work/render_test.go` | 1 行目・タイトル・メタ行の中身と桁 |
| `icon/icon_test.go`, `theme/theme_test.go` | 記号と色 |
| `work/testdata/*.golden`, `repo` / `search` / `root` の golden | 録り直し |
| 設計書 §4.1 | カードの構成 |

## Task 1: 記号

**Files:** `icon/icon.go`, `icon/icon_test.go`

**Steps:**

- [ ] `icon_test.go` に、**どのセットでも draft と Issue の記号が違う**ことのテストを書く
      （今 ASCII はどちらも `o` なので赤くなる）
- [ ] Unicode の Issue が `⦿` であることのテストを書く
- [ ] `sets` の Unicode `issue` を `"⦿"`、ASCII `issue` を `"@"` にする
- [ ] `make check`。`TestEveryGlyphIsOneColumn` が新しい記号も 1 桁だと言うこと

**Verify:** `go test ./internal/app/presentation/tui/icon/ -v` が緑

## Task 2: 未レビューの色

**Files:** `theme/theme.go`, `theme/theme_test.go`

**Steps:**

- [ ] `theme_test.go` に、`Review(domain.ReviewNone, false)` が `Dim()` と**違う**こと、
      かつ `Review(domain.ReviewRequired, false)` と**同じ**であることのテストを書く
- [ ] draft は今までどおり `Dim()` と同じであることも見る（未レビューと draft を
      同じ色にしてしまう実装を落とすため）
- [ ] `Review` の `ReviewNone` の枝を `attention()` にする
- [ ] `make check`

**Verify:** Repos タブ・Search タブの一覧も同じ `theme.Review` を引いているので、
そちらの golden にも同じ色の変化が出る。差分を目で見て、変わっているのが
未レビューの記号だけであることを確かめる

## Task 3: カードを 4 行にする

**Files:** `work/render.go`, `work/render_test.go`

1 行目を作る関数を足し、タイトルとメタからそれぞれの持ち物を外す。

```go
// cardHead is the first line: what state the item is in, where it lives and
// which number it is. The repository is clipped before the number is, because
// the number is what identifies the card and half a number identifies nothing.
func (m Model) cardHead(it domain.WorkItem, w int, selected bool, marker string) string
```

`cardHeight` は見出し 1 行ぶん増える。

```go
func (m Model) cardHeight() int {
	if m.boxed() {
		return titleLines + 4 // the head, the meta line and the box's two borders
	}
	return titleLines + 2
}
```

**Steps:**

- [ ] `render_test.go` に、1 行目が `kukv/octoscope` と `#12` の**両方**を載せることの
      テストを書く（owner が落ちていないことを主張する）
- [ ] 1 行目が入りきらない幅で、**番号が丸ごと残り、リポジトリ名が切れる**ことの
      テストを書く。先に「その幅では全部は入らない」ことを確かめてから主張する
- [ ] タイトルの行にリポジトリ名も番号も載らないことのテストを書く
- [ ] メタ行がリポジトリ名を載せず、CI バー → 経過時間 → ラベルの順であることの
      テストを書く（順序は `strings.Index` の比較で見る）
- [ ] 短いタイトルのカードでも高さが `cardHeight()` ちょうどであることを、
      枠あり・枠なしの両方で見る
- [ ] `cardHead` / `cardTitle` / `cardMeta` / `cardHeight` を実装して通す
- [ ] `shortRepo` を消す（呼び出しが無くなる）
- [ ] `make check`

**Verify:** `go test ./internal/app/presentation/tui/work/` の golden 以外が緑。
マウスの当たり判定は `cardHeight()` を読んでいるので、クリックのテストも緑のまま

## Task 4: アサーションが空振りしていないことを確かめる

**Files:** なし（一時的な変更のみ）

`.claude/rules/testing.md` の「アサーションを空振りさせない」。

**Steps:**

- [ ] 1 行目から owner を落とす（`shortRepo` を戻す）変更を一時的に入れ、
      Task 3 の 1 つ目のテストが落ちることを見る
- [ ] 切り詰めの順序を逆（番号から切る）にして、2 つ目のテストが落ちることを見る
- [ ] `ReviewNone` を `muted()` に戻して、Task 2 のテストが落ちることを見る
- [ ] すべて元に戻し、`make check`

**Verify:** 3 つとも落ちた。落ちないテストがあれば、それは何も守っていないので書き直す

## Task 5: golden の録り直しと設計書の追従

**Files:** `*/testdata/*.golden`, 設計書 §4.1

**Steps:**

- [ ] `make golden` で録り直す。work だけでなく repo / search / root も変わる
      （`theme.Review` と `icon.Issue` を共有しているため）
- [ ] 差分を 1 枚ずつ目で見る。work は 4 幅 × 2 言語、他のタブは記号と色だけが
      変わっていること。`cat -v` で桁ずれを見る
- [ ] 設計書 §4.1 のカードの説明を、1 行目 / タイトル 2 行 / メタ行の 4 行に直す
- [ ] `make check`

**Verify:** work 以外の golden の差分が、記号と色だけであること

## Task 6: 実際に起動して見る

**Files:** なし

**Steps:**

- [ ] `go run ./cmd/octoscope --lang ja` を 160 / 120 / 80 / 50 桁で動かす
- [ ] 未レビューの `•` が黄色で、draft の `◌` と見分けがつくか
- [ ] Issue の `⦿` が使っている端末のフォントで描けるか（描けないと豆腐になる）
- [ ] 幅 120 の 4 列でリポジトリ名が切れたとき、番号が読めるか
- [ ] Repos タブと Search タブの記号も見て、色が揃っているか

**Verify:** 4 つの幅 × 2 言語で崩れがない
