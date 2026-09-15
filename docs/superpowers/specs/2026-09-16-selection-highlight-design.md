# 設計: 選択行の塗りを最後まで届かせる

Repo と Search の一覧で `j` / `k` を押しても、カーソルがどこにいるか見えない。
Diff のカーソル行には色がなく、シンタックスハイライトが消える。
どちらも原因はひとつで、**行を囲む背景色が、行の中の色が終わった時点で一緒に消えている。**

この文書はその 1 つの原因と、それを二度と踏まない形での直し方を決める。

`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` の §4.1 が
「選択中のカードは枠の色と背景で示す」と決めており、この文書はそれを覆さない。
決めるのは「背景で示す」を実際に成立させる方法である。
`.claude/rules/tui.md` の「色は theme にだけ書く」が上位にある。

## 1. 実測して確かめたこと（2026-09-16）

**推測ではなく、リポジトリの golden ファイルと依存ライブラリのソースで確認した。**

`theme.Selected()` は `lipgloss.NewStyle().Background(...)` を返す。
lipgloss の `Render` は `背景色 + 本文 + リセット` を出すだけなので、
本文の中に色付きの span があると、その span 自身の末尾のリセットが
**前景色だけでなく背景色も**消す。以降の桁は素のまま描かれる。

`internal/app/presentation/tui/repo/testdata/repo_prs_en_80.golden` の
カーソル行を `cat -v` で見た実際の出力（`^[` が ESC）:

```
^[[48;2;29;39;53m^[[38;2;63;185;80m✓^[[m ^[[38;2;139;148;158m#1^[[m    first pr ...
 └ 背景 ON         └ 緑のマーカー   └ ここで全部リセット。以降 76 桁は素のまま
```

**塗られているのは先頭のマーカー 1 文字だけ。** 行の残りには何の印もない。
これが「カーソル位置が見えづらい」の正体である。

### 1.1 どこが壊れているか

`theme.Selected()` を呼ぶ 12 箇所を、渡している文字列が色を持つかで分けた。

| 箇所 | 中の色 | 状態 |
|---|---|---|
| `repo/render.go:223` PR / Issue 行 | state / number / badge / checks / age | **壊れ** |
| `search/render.go:378` 検索結果行 | state / number / age | **壊れ** |
| `search/render.go:292` フィルタ行 | ラベルが `Dim` | **壊れ** |
| `repo/sidebar.go:98` リポジトリ行 | バッジが `Dim` | **壊れ** |
| `search/saved_render.go:77` 保存済みクエリ | クエリが `Dim` | **壊れ** |
| `dialog/render.go:50` リポジトリ候補行 | star が `Dim` | **壊れ** |
| `checks/render.go:154` rerun の選択肢 | なし | 無事 |
| `checks/render.go:284` check 行 | なし | 無事 |
| `review/render.go:43` event の選択 | なし | 無事 |
| `diff/render.go:371` サイドバーのファイル名 | なし（§1.2 のため意図的に外してある） | 無事 |
| `diff/render.go:372` サイドバーの変更量 | 同上 | 無事 |
| `diff/render.go:425` diff のカーソル行 | 同上 | 無事 |

**無事な 6 箇所は、いずれも色を持たない文字列を渡しているから無事なだけである。**
仕組みが守っているのではない。うち diff の 3 箇所は、色を捨てることで回避している。

### 1.2 Diff は色を捨てることで回避していた

`diff/render.go:418` は同じ罠を踏んだうえで、カーソル行だけ
`plainText` で色なしに組み直して `Selected()` に渡している。
サイドバーの選択ファイルも同じで、`+12 −3` を色付きで組む経路を丸ごと飛ばし、
`plainSize` / `plainRow` という別の文字列を作って渡している。
塗りは全幅に届くが、代償としてカーソル行のシンタックスハイライト、
`+` / `−` の色、スレッドバッジの色が消える。**利用者はこれを不便だと報告している。**
`diff_en_120.golden` の背景色 `48;2;29;39;53` はちょうど 1 回しか出てこない。

### 1.3 Work のカードも同じ病気である

`work/render.go` のカードは `theme.Card(selected)` の `Background` で塗る。
`work_en_120.golden` のカーソルカードを見ると:

```
^[[38;2;91;180;245m│^[[m^[[48;2;29;39;53m ^[[m^[[48;2;29;39;53m^[[38;2;210;153;34m⚠^[[m ^[[38;2;139;148;158mkukv/octoscope^[[m #12^[[m^[[48;2;29;39;53m ^[[m
                      └ 左 padding は塗れている      └ マーカーのリセットで切れる。以降は素のまま
```

**塗れているのは lipgloss が自分で敷く左右の padding だけ**で、中身は塗れていない。
`cardHead` のコメントはこの罠そのものを書いている
（「色付きマーカーの上にスタイルを掛けると、そのマーカーのリセットで終わる」）。
つまり問題は既知だが、対症的に避けられてきただけで、根が残っている。

### 1.4 扱うべきリセットは 2 形しかない

背景を張り直す方式が現実的かどうかは、「リセットが何通りの形で出てくるか」で決まる。
数えた。

- **lipgloss**: リセットを自前で組まない。`ansi.Style.Styled` を通り、
  末尾は必ず **`ansi.ResetStyle`（公開定数、`"\x1b[m"`）**。他の形は出せない。
- **chroma**: トークンごとに自前で **`"\x1b[0m"`** を出す。

`diff_en_120.golden` の SGR 列を全部数えた結果:

| 列 | 個数 |
|---|---|
| `ESC[0m` | 55 |
| `ESC[m` | 53 |
| `0;…` の合成形 | **0** |
| `ESC[49m`（背景だけ既定に戻す） | **0** |

**2 形だけ**であり、どちらも「どのライブラリが、どの定数から出しているか」まで
辿れている。当てずっぽうの文字列置換ではなく、テストで固定できる契約である。

## 2. 範囲

| 入れる | 入れない |
|---|---|
| `theme.Selected()` を `theme.SelectedLine` に置き換える | 選択色そのものの変更（`#e8eef5` / `#1d2735` は据え置き） |
| `Selected()` を呼ぶ 12 箇所すべての移行 | 行頭マーカー（`▸`）の追加 |
| Diff のカーソル行で色を保つ | 一覧のレイアウト・列幅・キー操作 |
| Work のカーソルカードの中身を塗る | `theme.Card` の枠の色 |
| 包む前に幅いっぱいまで padding されているかの確認 | 一覧以外の描画の見直し |

行頭マーカーを入れないのは、全行のタイトル幅が 2 桁縮むうえ、
塗りが全幅に届けば位置は十分に読めるからである。塗りを直して
なお足りなければ、そのとき別に決める。

## 3. 決めたこと

### 3.1 `theme.Selected()` は消し、`theme.SelectedLine(s string) string` にする

```go
// SelectedLine draws s as the selected row: the selection's background,
// carried the whole way across the line.
func SelectedLine(s string) string
```

やることは 3 つだけである。

1. 先頭に選択色の背景を置く
2. `s` の中の 2 形のリセットそれぞれの**直後**に、同じ背景を置き直す
3. 末尾でリセットする

中の前景色は生き残る。ラベルチップのように自前の背景を持つ span も、
その span のリセットの直後に行の背景が戻るので、前後が繋がる。

背景の SGR 列は `ansi.Style{}.BackgroundColor(pick(...)).String()` から得る。
lipgloss に空文字を描かせて切り出すような組み立て方はしない。

**戻り値を `lipgloss.Style` ではなく `string` にするのが要点である。**
`Selected()` が `lipgloss.Style` を返している限り、いつか誰かが色付きの文字列を
`.Render()` に渡し、同じ罠に落ちる。関数の形で不可能にする。

### 3.2 今無事な 3 箇所も移行する

`checks/render.go` の 2 箇所と `review/render.go` の 1 箇所は、
今たまたま色を持たない文字列を渡しているだけである。見た目は変わらないが、
`SelectedLine` に移す。**選択の描き方をひとつにする**ことが目的であり、
「ここは色がないから今のままでよい」という判断を将来に残さない。

### 3.3 Diff は色を捨てる経路をやめる

カーソル行も他の行と同じ経路で色をつけ、`SelectedLine` に通す。
シンタックスハイライトも `+` / `−` の色もカーソル行で生き残る。
`plainText` の呼び手は `render.go:425` の 1 つだけなので、移行と同時に消す。

サイドバーも同じで、選択ファイルのために作っている `plainRow` の分岐をやめ、
色付きで組んだ `size` をそのまま `SelectedLine` に通す。
`plainSize` は幅の判定にだけ使う（色付きの文字列で桁を数えても
`ansi.StringWidth` は同じ答えを返すが、判定と描画で別の文字列を見ないほうが読める）。

`render.go:418` の「色を捨てる」理由を書いたコメントは、逆の理由に差し替える。

### 3.4 Work のカードは箱に渡す前に塗る

`theme.Card(selected)` の `Background` はそのまま残す。
カーソルカードのときだけ、箱に渡す本文の各行を `SelectedLine` に通す。
中身は各リセットのあとに背景が戻り、左右の padding は今までどおり lipgloss が塗る。
`cardTitle` が選択時に `theme.Cursor()` を掛けているのはそのままでよい
（その span のリセットのあと背景が戻る）。

### 3.5 包む前に幅を埋める

背景は文字のある桁にしか乗らない。**`SelectedLine` に渡す時点で、
行が描きたい幅まで padding されていなければならない。**

確かめた結果は次のとおり。

| 箇所 | 幅を埋めているか |
|---|---|
| `repo/render.go` の行 | `layout.Pad` / `layout.Right` の合計がちょうど幅。**埋まる** |
| `search/render.go` の結果行 | 同上。**埋まる** |
| `repo/sidebar.go` `dialog/render.go` | `layout.Pad` + `layout.Right`。**埋まる** |
| `diff` `checks` の `fit` | clip したうえで幅まで空白を足す。**埋まる** |
| `search/saved_render.go:77` | `layout.Clip` だけ。**埋まらない** |
| `search/render.go:292` フィルタ行 | `Clip` すらしていない。**埋まらない** |
| `checks/render.go:154` rerun の選択肢 | 語だけを塗る意図。**埋めない（現状のまま）** |
| `review/render.go:43` event の選択 | 同上。**埋めない（現状のまま）** |

埋まらない 2 箇所（保存済みクエリ行・フィルタ行）は、移行と同じ作業で
`layout.Pad` を通し、行として塗られるようにする。
rerun と event の 2 箇所は選択肢の語を並べた中の 1 語を指すものなので、
全幅に広げるのはむしろ誤りである。現状の見た目を保つ。

## 4. テスト

**`SelectedLine` の単体テスト**（`theme` パッケージ）。主張するのは
「リセットのあとに行の背景が戻っていること」の一点で、3 つの入口を押さえる。

| ケース | 入力 | 主張 |
|---|---|---|
| lipgloss のリセット | `Dim().Render("x") + "y"` | `ESC[m` の直後に背景の SGR 列がある |
| chroma のリセット | `"a" + "\x1b[0m" + "b"` | `ESC[0m` の直後に背景の SGR 列がある |
| 自前の背景を持つ span | `Badge("d73a4a").Render(" bug ")` を含む行 | チップの背景は残り、そのリセットの直後に行の背景が戻る |

色そのものは主張しない（`theme` の値が変わればテストが落ちるだけで、
何も守らない）。主張するのは構造である。

**golden** は repo / search / diff / dialog / checks / review / work が動く。
差分が広いのは、選択の描き方を 9 箇所でまとめて変えるからである。
再生成したあと、Repo のカーソル行を `cat -v` で 1 本目視し、
背景の SGR 列が行の中で繰り返し現れることを確かめる。

**実機確認**（`.claude/rules/tui.md`）。`go run ./cmd/octoscope` と
`--lang ja` で、Repo・Search・Diff・Work の 4 画面のカーソルを動かして見る。
80 桁でも確かめる。テストが通っただけでは完了にしない。

## 5. 見込まれる差分

| ファイル | 変更 |
|---|---|
| `theme/theme.go` | `Selected()` を消し `SelectedLine()` を足す |
| `theme/theme_test.go` | `SelectedLine` の単体テスト |
| `repo/render.go` `repo/sidebar.go` | 呼び出しの移行 |
| `search/render.go` `search/saved_render.go` | 呼び出しの移行、フィルタ行と保存済みクエリ行の padding |
| `dialog/render.go` `checks/render.go` `review/render.go` | 呼び出しの移行 |
| `diff/render.go` | `plainText` と `plainRow` の廃止、コメントの書き換え |
| `work/render.go` | カーソルカードの本文を `SelectedLine` に通す |
| 各 `testdata/*.golden` | 再生成 |

`internal/app/domain`・`usecase`・`github` は触らない。データ取得も変わらない。
