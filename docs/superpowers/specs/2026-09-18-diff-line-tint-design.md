# 設計: diff の追加/削除行を背景色で見分ける

Diff ビューで追加行と削除行が読み取りにくい。
行頭の `+` / `-` マーカー 1 文字だけが色を持ち、本文はどの行も同じ
シンタックスハイライトで描かれる。80 桁の行が並ぶと、左端 1 桁の色差は
視線に入らない。

この文書は「追加/削除行を、行全体の背景色で見分けられるようにする」ことを決める。

`.claude/rules/tui.md` の「色は theme にだけ書く」が上位にある。
`docs/superpowers/specs/2026-09-16-selection-highlight-design.md` が決めた
「リセットの直後に背景を張り直す」手法を、2 色目に広げる形で再利用する。
その文書の結論（この描画が出すリセットは `ansi.ResetStyle` と chroma の
`ESC[0m` の 2 形だけ）は、ここでも前提として使う。

## 1. いま何が描かれているか

`internal/app/presentation/tui/diff/render.go` の `diffTextLine` は、
行番号ガター、`markerStyle` が色を付けた `+` / `-`、そして
`theme.Highlight` に通した本文を並べる。
本文の色はファイルの言語だけで決まり、その行が追加なのか削除なのかは
本文の色に一切出ない。

`diffLine` はカーソル行だけを `theme.SelectedLine(layout.Fill(line, width))` で
塗る。追加/削除行には何も足さない。

## 2. 決めること

### 2.1 追加/削除行は行全体に薄い背景色を敷く

GitHub の web UI と同じく、追加行はうすい緑、削除行はうすい赤の背景を
ペインの右端まで敷く。シンタックスハイライトは残す。

ターミナルに透過は無いので、Primer の diff 行背景トークンが rgba で持つ値は
背景 canvas の上に合成した不透明値に直して使う。

| | light (`#ffffff` 合成) | dark (`#0d1117` 合成) |
|---|---|---|
| 追加行 | `#e6ffec` | `#12261e` — rgba(46,160,67,.15) |
| 削除行 | `#ffebe9` | `#25171c` — rgba(248,81,73,.10) |

合成の根拠を theme.go のコメントに残す。既存のパレット項目が
GitHub の値であることを書いているのと同じ扱いにする。

### 2.2 カーソル行は選択色が勝つ

カーソルが追加/削除行に乗っているときは、いままでどおり選択背景 1 色で塗る。
その行が追加か削除かは `+` / `-` マーカーの前景色で分かる。

**着色したうえで `SelectedLine` に渡してはいけない。**
着色行が持つ背景 SGR は、`SelectedLine` が張り直す背景の**後ろ**にも
そのまま残っているので、選択背景と行の背景が交互に出る行になる。
分岐は `styledLine` ではなく `diffLine` に置き、両者が重ならないようにする。

### 2.3 触らないもの

`+` / `-` マーカーの前景色、`theme.Highlight`、ハンクヘッダ・コメント行・
折りたたみ行・注記、サイドバー、i18n のカタログ。

## 3. 実装の形

### 3.1 theme

`SelectedLine` の本体を、背景色を引数に取る非公開の関数に切り出す。

```go
// fillLine は s を bg 一色で塗る。リセットのたびに bg を置き直す。
func fillLine(s string, bg color.Color) string

func SelectedLine(s string) string { return fillLine(s, selection()) }

// DiffLine は追加/削除行を背景色で塗る。LineContext は素のまま返す。
func DiffLine(k domain.DiffLineKind, s string) string
```

`SelectedLine` の既存の doc コメント（背景は文字のある桁にしか乗らない、
リセットは 2 形しかない）は `fillLine` に移す。呼び手が守るべき前提は
どちらの公開関数にも同じようにかかる。

パレットには `diffAddedBg()` / `diffRemovedBg()` を足す。
`selection()` と同じく `color.Color` を返す関数にする。

### 3.2 render

`diffLine` を 3 分岐にする。

```go
func (m Model) diffLine(r row, selected bool, width int) string {
	line := m.styledLine(r, width)
	switch {
	case selected:
		return theme.SelectedLine(layout.Fill(line, width))
	case r.kind == rowLine && r.line.Kind != domain.LineContext:
		return theme.DiffLine(r.line.Kind, layout.Fill(line, width))
	default:
		return line
	}
}
```

`layout.Fill` は着色行にも要る。背景は文字のある桁にしか乗らないので、
埋めずに渡すと本文の終わりで色が切れる。

## 4. 検証

1. **theme の単体テスト** — `fillLine` の出力が、先頭に背景、
   `ansi.ResetStyle` と chroma の `ESC[0m` それぞれの直後に背景、
   末尾にリセットを持つ。
2. **golden の再生成と目視** — `diff/testdata` を再生成し、差分を読む。
   - 追加行に追加背景が「1 + その行のリセット数」回
   - コンテキスト行に 0 回
   - カーソルが乗った追加行には選択背景（dark なら `48;2;29;39;53`）だけ、
     追加背景は 0 回
3. `make check` が通る。
4. 実際に起動して見る（`.claude/rules/tui.md` / CLAUDE.md）。
   dark と light の両方の端末、`--lang ja` も。
   シンタックスハイライトの前景色が背景の上で潰れていないことを確かめる。

## 5. やらないこと

- 単語単位の差分強調（intra-line diff）
- 行番号ガターだけを別の色にすること
- テーマの切り替え設定
