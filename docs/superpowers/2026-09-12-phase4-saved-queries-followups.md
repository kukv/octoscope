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

`usecase.SavedQuery`（`internal/usecase/search.go:18` に定義）が、設定ファイルの
`config.SavedQuery` と TUI 側の往復に使う型になっている。`internal/tui` は
`internal/config` を import できず（`.golangci.yml` の `tui-layer`）、
`internal/config` は他の internal パッケージを一切 import できない
（同 `config-layer`、54-62 行目）。`internal/tui` は `usecase.Item` で既に
`internal/usecase` を import しているので、**`internal/tui` と `internal/config`
の両方に手が届く場所は `internal/usecase` しかない。** 変換の両方向
（`config.SavedQuery` → `usecase.SavedQuery` と、その逆）も `internal/usecase`
が持つ: `Usecase.SaveQueries`（`search.go:24`）と `SavedQueriesFrom`
（`search.go:34`）。`internal/config` 自身は `usecase.SavedQuery` を一切
参照しない。

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
一覧としての桁揃えはスコープ外とした。全体レビューで `pickerView`
自体を高さの窓切りのために書き直す機会があり（下の「全体レビューで見つかり、この波で直したもの」参照）、
そのついでに直すのが安いという指摘を受けたが、桁揃えは見た目だけの変更で
ブリーフにも入っていないため、この波でも見送った。次に `pickerView` を
触るときに揃えること。

### 2. 前スライスから引き継いだままのもの

- 生クエリの「未設定」と「空文字」を区別しない（前スライスの積み残し 11 番）。
  保存するのは組み上がった文字列（`m.query()` の戻り値）なので、このスライスでは
  決着させる必要が無かった。
- マウス非対応（前スライスの積み残し 1 番）。
- `first: 50` のページングが無く `50+` と描くだけ（前スライスの積み残し 2 番）と、
  それが `internal/gh/cli/work.graphql` の `first: 50` と黙って結合している件
  （積み残し 3 番）。このスライスは検索の実行経路に触れていないため対象外。
- en のキーバーだけコロンの後に空白がある件（前スライスの積み残し 9 番）。

### 3. 保存クエリの並べ替え・名前の変更が無い

ブリーフにも仕様にも無い操作。消して付け直せば同じ結果になる。

**直さなかった理由:** スコープ外。

### 4. Repos の追加ダイアログの積み残し「候補のスクロール」は決着しなかった

前スライスの `docs/superpowers/2026-09-12-phase4-search-tab-followups.md` は
触れていないが、`internal/tui/dialog` 自体の積み残し（Repos の追加ダイアログで
候補が一覧に収まらないときのスクロール）は、保存クエリのポップアップが解決する
機会にはならなかった。

全体レビューで `pickerView`（`internal/tui/search/saved_render.go`）に高さの
窓切りが入り（下の「全体レビューで見つかり、この波で直したもの」参照）、この窓とカーソル追従の計算式は
`dialog` 側の同じ穴にもそのまま使える形をしている。ただし今回は
`internal/tui/search` の中に留め、`internal/tui/layout` には出さなかった。

**直さなかった理由:** ブリーフの範囲が保存クエリのポップアップに閉じており、
`dialog` 側を直すのは別の作業になる。窓の計算式を `internal/tui/layout` に
出す一般化は、実際に `dialog` 側を直す番になってから、2 箇所目の使用例を見た
うえでやるのが安全（1 箇所しか使っていない抽象化は先走りになりやすい）。

### 5. 設定ファイルへの同時書き込みでロストアップデートが起こりうる

`internal/config/config.go` の `SaveQueries` / `SaveRepositories` はどちらも
`Load`（読む）→ 差し替え → `save`（temp+rename）の read-modify-write で、
排他が無い。ピッカーで `x` を連続で 2 回押すと、2 つの goroutine が同時に
これをやる。ファイルが壊れることは無いが（`os.Rename` は atomic）、片方の
書き込みがもう片方の読み込みより先に走ると、他キーの同時更新が失われたり、
消したはずのクエリが復活したりしうる。

**直さなかった理由:** このスライスが持ち込んだものではなく、
`SaveRepositories` から拡張された既存の形。`Store` に `sync.Mutex` を足せば
read-modify-write の交錯は消えるが、順序の逆転までは消えない
（負けた goroutine が古い全量を残す）。アーキテクチャの話であり、この波の
スコープ（保存クエリの全体レビュー指摘への対処）を超えるため、別の機会に
残す。

### 6. picker の golden 6 枚が 80/120/160 で同一

`layout.PopupWidth` は `max(min(termCols-4, 60), 20)` なので 80 桁以上では
常に 60 に張り付き、picker のキーバーも 3 ヒントで 80 桁に収まるため、幅 3
種の golden が完全に同じ内容になっている。実害は無い（表示幅は測り直して
全部収まっていることを確認済み）。

**直さなかった理由:** golden の内容が同一なこと自体は不具合ではなく、
「3 幅で録った」ことが幅の網羅を意味しないという認識の記録が目的。
録り直しても同じ内容になるだけなので、直す対象がない。

## 全体レビュー（2026-09-12）で見つかり、この波で直したもの

このスライスが完結したあとの全体レビュー（`final-review.md`）が Important 6 件・
Minor の一部を挙げ、そのうち次を直した。上の「見つかったが直さなかったこと」の
旧 2 番・5 番・6 番・7 番は、この波でそれぞれ解消したのでここへ差し替える。

- **`TestXOnAnEmptyListDoesNothing` の空振り**（`search_test.go`）— モデルと
  別インスタンスの `fakeStore` を見ていて何があっても通っていた。
  `newTestModel(t, store)` に直した。
- **`openField` の off-by-one**（`search.go`）— `cursorCol` を引いておらず、
  幅 120 で空の `org` フィールドに何も打っていないのに `…` が出ていた
  （旧 2 番の「疑い」は実在した）。`-cursorCol` を足し、
  `TestATypedFilterIsNotTruncatedBeforeAnythingIsTyped` で再現・固定した。
- **ピッカーが端末の高さを溢れる**（`saved_render.go`）— `pickerView` が
  `m.height` を一切見ず、保存クエリが多いと箱の上辺とキーバーが画面外に
  押し出されて操作不能になっていた。`pickerRows` / `pickerWindow` で
  `internal/tui/search/render.go` の `resultRows` / `resultWindow` と同じ
  形の窓を切り、カーソルが窓に入るよう追従させた
  （`TestThePickerFitsTheHeight` / `TestThePickerWindowFollowsTheCursor`）。
- **起動時の配線が未検証**（`app.go`）— `search.New(src).SetSavedQueries(opts.SavedQueries)`
  の `SetSavedQueries` を丸ごと落としても `go test ./...` が全部緑だった。
  `app_test.go` に `TestTheSettingsFileSavedQueriesReachThePicker` を足した。
- **README の設定表**（`README.md` / `README.ja.md`）— `default_tab: search` と
  `saved_queries` が表に無かった。両方の表と YAML の例を足した。
- **`upsert` が呼び出し側のスライスを破壊的に更新する**（`saved.go`）— 旧 5 番の
  「`app.Options` が起動時 1 度きりだから無害」という理由は誤りだった。
  `handleNameKey` は `m.saved` を書き換えたあと `saveQueries(m.src, m.saved)` を
  `tea.Cmd` として起動するため、同名で 2 回連続保存すると、書き換え中の
  スライスを goroutine が読む競合になる。すぐ下の `removeSaved` は
  `slices.Clone` していて非対称だった。`upsert` も同じく非破壊にし、
  `TestUpsertDoesNotMutateTheCallersSlice` で固定した。
- **ピッカーが開いている間 `notice` が描かれない**（`render.go`）— `x` の
  書き込み失敗が `esc` するまで画面に出なかった。picker の `View()` にも
  notice の行を足し、`pickerRows` にその分の高さも引かせた
  （`TestTheNoticeShowsWhileThePickerIsOpen`）。
- **`saveErrMsg` の通知経路にテストが無い**（旧 6 番）— `TestAFailedSaveIsReported`
  を足して固定した。
- **`mode` の doc コメントが古い**（旧 7 番）— `modeName` / `modePicker` を
  追加した文に直した。
- **`openPicker` の `m.pick = 0` が未検証** — `TestReopeningThePickerResetsTheCursor`
  を足して固定した。
- **`TestSavingTheSameNameReplacesIt` が置換の中身まで見ていない** — 既存
  エントリを別のクエリにしておき、保存後に新しいクエリへ置き換わっている
  ことまで確かめる形に強めた。

## 前スライスの積み残しのうち、このスライスで解消したもの

- **積み残し 4 番「`s` は未割当」**（`search-tab-followups.md`）— `s` は名前を
  付けて保存する操作に割り当てた（コミット `864f9cd`）。
- **`config.Store` に `SaveQueries` を足す**という申し送り — `internal/config/config.go:109`
  の `Store.SaveQueries` として入った（コミット `6736334`）。
- **`default_tab: search`** という申し送り — `config.Config.DefaultTabName()`
  （`internal/config/config.go:43`）が `"repos"` / `"search"` の両方を返す形に
  なり、`config.Config.WantsRepos()` と `app.Options.DefaultRepos` は廃止された
  （コミット `6736334`）。
