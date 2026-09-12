# 保存クエリスライスの積み残し

Phase 4 スライス 3-3「保存クエリ」（`docs/superpowers/plans/2026-09-12-phase4-saved-queries.md`、
Task 1〜6、全部完了。実装は Task 1〜5、コミット 9 本）で見つかったが、このスライスでは
直さなかったもの。**直さないと決めた理由も書く。** これで Phase 4 スライス 3（Search
タブ）が完結する。

## 実端末での確認の依頼

TTY の無い環境では代行できないため、利用者に見てほしいものを列挙する。
`.claude/rules/tui.md` と過去のスライスと同じ受け渡しである。golden は
`internal/tui/search/testdata/search_picker_*` と `search_naming_*`（picker /
naming それぞれ en/ja × 80/120/160）。

- `Ctrl+O` のポップアップが ja の 80 桁で読めること
- `s` → 名前を打つ → `enter` で本当に `config.yaml`（`Path()` が返す先。既定は
  `~/.config/octoscope/config.yaml`）の `saved_queries` に書かれ、次回起動でも
  残っていること
- `Ctrl+O` のポップアップで `enter` を押し、呼び出したクエリで検索が実際に走ること
- `x` で消したエントリが次回起動でも消えたままであること
- `default_tab: search` で Search タブから起動すること。`--repo` を付けたときは
  Repos になること（下の「決めたこと」参照）
- ポップアップが開いている間、`q` で octoscope が終了せず、`1` / `3` を押しても
  タブが切り替わらないこと

## 決めたこと（記録する価値があるもの）

### 設計 §4 の「`dialog` を共用する」を満たしていない(前提の見直し、利用者の承認待ち)

設計（`docs/superpowers/specs/2026-09-08-phase4-design.md:101-103`）は
「`internal/tui/dialog` を新設し、Repos の追加ダイアログと Search の保存クエリ
ポップアップで共用する」と書いているが、このスライスは型を共用しなかった。
共用したのは箱の寸法（`boxWidth` / `contentWidth`）だけで、`internal/tui/layout`
に `PopupWidth` / `PopupContentWidth`（`internal/tui/layout/popup.go:12,21`）として
切り出し、`internal/tui/dialog/render.go:70-77` と Search の
`internal/tui/search/saved_render.go:18-19` の両方から呼んでいる
（コミット `e755fea`）。

**理由:** `dialog.Model` は `gh.RepoCandidate` の一覧と「打って検索→候補が絞り込
まれる」操作に固定されている。保存クエリのポップアップは「一覧から選ぶだけ」で
入力欄が要らない。無理に一般化すると、Repos 側は使わない「入力欄なし」の分岐を、
Search 側は使わない `Searching()` / `SetError` を抱えることになり、両方の呼び出し
側が歪む。

**提案:** 設計 §4 の 101〜103 行目を「`internal/tui/dialog` の入力欄・候補一覧の
形は Repos の追加ダイアログに固有とし、ポップアップの箱の寸法だけ
`internal/tui/layout` で共有する」に直す。これは利用者の承認事項であり、この
スライスは提案するところまでしかできない。

### 境界の型は `internal/usecase` に置いた

`usecase.SavedQuery`（`internal/usecase/usecase.go` に定義、`internal/tui/search`
と `internal/config` の両方から参照される）が、設定ファイルの `config.SavedQuery`
と TUI 側の往復に使う型になっている。`internal/tui` は `internal/config` を
import できず、`internal/usecase` は `internal/tui` を import できない
（`.claude/rules/architecture.md` の依存の向き、`depguard` で強制）。
`internal/tui` は `usecase.Item` で既に `internal/usecase` を import しているので、
**両方から見える場所はそこしかない。**

### `default_tab: search` は待たせない

`internal/tui/app/app.go:178-181` の `wantRepos` は、設定ファイルの開始タブ
（Repos）をリポジトリ解決が返るまで保留する。これは Repos タブがカレント
ディレクトリのリポジトリを必要とするためで、Search にはその依存が無いので
`app.New` の時点で直接タブを合わせる（`app.go:199`
`m.wantRepos = opts.DefaultTab == "repos"` と、Search が絡む分岐は別に持つ）。

### `--repo` と `default_tab` が両方あるときは `--repo` が勝つ

`app.go:201` の `case opts.Repo != "":` が `wantRepos` の判定より先に評価される。
フラグはその場限りの上書き、設定ファイルは恒久的な好みという扱い。

### キーバーのヒントは列の末尾に足す

`ctrl+o` のヒントを最初 `refresh` / `quit` より前に置いたところ、`FitKeyBar` が
末尾から落とす仕様と噛み合わず、ja 80 桁で `r:再検索` と `q:終了` が golden
6 枚中 3 枚（`search_candidates_ja_80.golden` ほか）から消えた。コミット
`e7f33d1` で末尾に置き直して直した。「most important first」の列に後から
足すときは末尾に置くこと。

## 実装中に見つけた「緑のまま何も守っていなかったテスト」3 件（全部直した）

前スライスの followups（`docs/superpowers/2026-09-12-phase4-search-tab-followups.md`
の「教訓」）にも同じ形の欠陥が記録されており、**それでも今回また 3 件出た。**

1. **`TestEscapeClosesThePopup`**（`internal/tui/search/search_test.go:699`）—
   `newTestModel` が `WindowSizeMsg` を送らず幅 0 のままだったため、`ctrl+o` の
   分岐を丸ごと潰しても、esc の前後で `View()` が同じ（空のままの）出力を返し
   続けて緑だった。コミット `b2020b6` で幅を持たせ、esc の前にポップアップの
   中身（保存したクエリ名 `"mine"`）が画面に出ていることをアサートするよう直した。
2. **`TestRemovingTheLastRowKeepsTheCursorInRange`**（`search_test.go:632`、実装
   計画が指定したテスト）— `pickerView` が enter の成否に関わらず全行を描画する
   ため、`strings.Contains` だけではカーソルの収め直しを外しても通った。
   `m.mode != modePicker` を合わせて見る形に直した（enter が弾かれたらポップアップ
   は閉じないので、mode がそのまま `modePicker` に残ることを確かめられる）。
3. **`make check` 緑という誤報** — 前段のタスクが「lint 0 issues」と報告したが、
   実際は `ineffassign` と `unparam` で赤だった。タスクのレビューが対象テストしか
   回しておらず見逃していた。コミット `8eea14a` で `internal/tui/app/app_test.go`
   の `started(t, width)` から未使用の `width` 引数を落とし、
   `internal/tui/search/golden_test.go` の `saveQuery` が捨てていた `tea.Cmd` を
   コメント付きで明示的に受け取る形に直した。

**共通する形:** 「画面に出ているはずのものを、出ていない状態でも通る形で確かめて
いた」。`View()` を見るテストは幅を持たせてから見る。

## 見つかったが直さなかったこと（理由つき）

### 1. ポップアップの行が桁で揃っていない

`internal/tui/search/saved_render.go` の行は「名前 + 空白 + クエリ」の単純な
連結なので、名前の長さが違うとクエリの開始位置が揃わない（全角の名前だと
なおさらずれる）。

**直さなかった理由:** ブリーフが要求しているのは「重ならないこと」だけで、
一覧としての桁揃えはスコープ外とした。

### 2. `openField` にも同種の off-by-one がある疑い

コミット `95cf974` で `openName` / `openRaw`（`internal/tui/search/search.go:501,515`）
の `textinput.SetWidth` に `cursorCol` 分の余白を足したが、フィルタの入力欄を開く
`openField`（`search.go:490-495`）は対象外のままで、`cursorCol` を引いていない。

**直さなかった理由:** `95cf974` はこのスライスで見つけた別バグ（ポップアップの
命名欄と生クエリ編集欄の幅）の修正であり、`openField` に同じ問題が実在するかは
未確認。次にこの行を触る機会に、まず再現するテストを書いて確かめること。

### 3. 前スライスから引き継いだままのもの

- 生クエリの「未設定」と「空文字」を区別しない（前スライスの積み残し 11 番）。
  保存するのは組み上がった文字列（`m.query()` の戻り値）なので、このスライスでは
  決着させる必要が無かった。
- マウス非対応（前スライスの積み残し 1 番）。
- `first: 50` のページングが無く `50+` と描くだけ（前スライスの積み残し 2 番）と、
  それが `internal/gh/cli/work.graphql` の `first: 50` と黙って結合している件
  （積み残し 3 番）。このスライスは検索の実行経路に触れていないため対象外。
- en のキーバーだけコロンの後に空白がある件（前スライスの積み残し 9 番）。

### 4. 保存クエリの並べ替え・名前の変更が無い

ブリーフにも仕様にも無い操作。消して付け直せば同じ結果になる。

**直さなかった理由:** スコープ外。

### 5. `upsert` が呼び出し側のスライスを破壊的に更新する

`internal/tui/search/saved.go:31` の `upsert` は、同名エントリを置き換えるとき
`qs[i] = q` で元のスライスをその場で書き換える。起動時に渡した
`opts.SavedQueries` が参照する配列が書き換わりうる。

**直さなかった理由:** 今は `app.Options` が起動時に 1 度だけ使い捨てで渡される
ため、書き換えても実害が無い。`Options` を使い回す変更が入るときに合わせて
コピーを取る形に直すのが筋が良い。

### 6. 保存失敗の通知経路（`saveErrMsg`）にテストが無い

`internal/tui/search/saved.go:20` の `saveErrMsg` と、`search.go:302` のその
受け口はあるが、`search_test.go` の `fakeStore.err`（同ファイル `fakeStore` 型）
に非 nil を渡して失敗時の表示行を確かめるテストが無い。

**直さなかった理由:** ブリーフの完了条件に無く、実装中に見つけた副産物。
`fakeStore` は既にこの用途向けの `err` フィールドを持っているので、次に
このファイルを触るときに軽く足せる。

### 7. `mode` の doc コメントは古いまま

`internal/tui/search/search.go:92-94` の `mode` の doc コメントは
「none, a typed filter's field, or the raw query editor」とだけ書いており、
このスライスで足した `modeName` / `modePicker`（`search.go:100-101`）に触れて
いない。

**直さなかった理由:** 確認した結果、実際に古いままだった。動作に影響は無く、
コメント 1 行の更新だけなので、次にこのファイルを触る機会にまとめて直す。
このスライス単独でコミットを割くほどの変更ではないと判断した。

### 8. Repos の追加ダイアログの積み残し「候補のスクロール」は決着しなかった

前スライスの `docs/superpowers/2026-09-12-phase4-search-tab-followups.md` は
触れていないが、`internal/tui/dialog` 自体の積み残し（Repos の追加ダイアログで
候補が一覧に収まらないときのスクロール）は、保存クエリのポップアップが解決する
機会にはならなかった。

**直さなかった理由:** 保存クエリの一覧はブリーフの想定件数が少なく、スクロールを
必要とする場面が無い。加えて、上の「決めたこと」で書いたとおり `dialog.Model` と
ポップアップの型を共用しなかったため、`dialog` 側のスクロール未対応はそのまま
残っている。

## 前スライスの積み残しのうち、このスライスで解消したもの

- **積み残し 4 番「`s` は未割当」**（`search-tab-followups.md`）— `s` は名前を
  付けて保存する操作に割り当てた（コミット `864f9cd`）。
- **`config.Store` に `SaveQueries` を足す**という申し送り — `internal/config/config.go:109`
  の `Store.SaveQueries` として入った（コミット `6736334`）。
- **`default_tab: search`** という申し送り — `config.Config.DefaultTabName()`
  （`internal/config/config.go:43`）が `"repos"` / `"search"` の両方を返す形に
  なり、`config.Config.WantsRepos()` と `app.Options.DefaultRepos` は廃止された
  （コミット `6736334`）。
