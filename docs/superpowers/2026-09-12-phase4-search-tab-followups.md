# Search タブ本体スライスの積み残し

Phase 4 スライス 3-2「Search タブ本体」（`docs/superpowers/plans/2026-09-12-phase4-search-tab.md`、
Task 1〜8、全部完了）で見つかったが、このスライスでは直さなかったもの。
**直さないと決めた理由も書く。**

## 実端末での確認の依頼

TTY の無い環境では代行できないため、利用者に見てほしいものを列挙する。
`.claude/rules/tui.md` と過去のスライスと同じ受け渡しである。golden は
`internal/tui/search/testdata/search_*`（loaded / editing / empty / candidates
の 4 状態 × en/ja × 80/120/160）。

- **Search タブ**（120 桁以上と `--lang ja` の 80 桁）。フィルタ・生クエリ・結果・
  キーバーが読めること
- `space` で値が変わり、`enter` で検索が走ること。**`space` の連打で `gh` が
  何本も起きないこと**（確定するまで走らない）
- 生クエリに `q` や `3` を含む語を打っても octoscope が終了・タブ移動しないこと
- 入力中のキーバーが `esc:取消` / `enter:確定` になっていること
- 一致なしのときの見え方、GitHub にクエリを拒まれたときの通知行
- 80 桁でフィルタペインが畳まれ、120 桁で戻ること（`minPaneWidth = 100`）
- `repo:` を入れて label 行に移ったときにチップが出ること、`repo:` を消すと
  チップが消えること

## 実装中に見つかった 5 件の実バグ（このスライスで直した）

見つけ方が 3 通りに分かれたので、そこも記録する。

**自己レビュー（advisor）が見つけた（Task 4）:**

- **幅 100 未満でフィルタペインのカーソルが見えないペインに残っていた。**
  `pane` の既定値が `paneFilters` のままで、`enter` が画面に無い入力欄を開いていた。
  `internal/tui/repo/repo.go` の `sidebarCols() == 0` と同じ形で、`WindowSizeMsg` で
  `paneCols() == 0` になったら `pane` を `paneResults` に戻すようにした。
  `TestNarrowWidthKeepsTheCursorOnWhatIsDrawn` が再発を止める
- **入力欄が端末幅を超えることがあった。** `openField` / `openRaw` の `SetWidth` が
  `"> "` プロンプトの分を引いていなかった。`promptCols` を差し引く形にし、
  `render.go` 側にも `layout.Clip` を足した。`TestTheRawEditorFitsTheTerminal` /
  `TestATypedFilterFitsThePane` が再発を止める

**golden を目で見て見つけた（Task 6、テストは全部緑だった）:**

- **左右のペインで一覧の開始行が 1 行ずれていた。** `filterPane()` が見出し
  `Filters` の下にだけ空行を挟んでおり、`Results` 側の 1 件目と揃っていなかった。
  `render.go` の `filterPane()` からその空行を削除した。専用のテストはなく、
  golden 120/160 幅 12 枚の目視で確認して再録した
- **入力中もキーバーが閲覧時のままだった。** `space` / `j` / `k` / `q` などが
  全部入力に文字として入るだけで、抜け方（`esc`）が画面に無かった。
  `internal/tui/repo` の `addDialogHints()` に倣い、`Capturing()` のときは
  `footer.search.cancel` / `footer.search.apply` の 2 つだけを返すようにした。
  こちらも専用のテストはなく、`search_editing_*` golden 6 枚の目視で確認した

**レビューが見つけた（Task 8、テストも golden も緑だった）:**

- **キャッシュを検証する唯一のテストが空振りだった。** `onFilter` ヘルパーが
  `press()` の返す `tea.Cmd` を毎回捨てており、テストは「実行された cmd の
  副作用」だけを見ていたため、`m.labelCandidatesRepo == repo` の判定を
  削っても緑のままだった。`onFilterCmd` を足し、1 度目は `cmd != nil`
  （fetch が始まった）、2 度目は `cmd == nil`（キャッシュが効いた）の両方を
  見る形に直した。`TestTheSameRepositoryIsNotAskedForTwice` が再発を止める
- **`repo:` を空に戻しても古いリポジトリのチップが出続けた。** `candidateChips()`
  がカーソル位置だけで描画し、キャッシュされたリポジトリと現在の `repo:` が
  一致するかを見ていなかった。「一致しないときは描かない」ガードを label /
  author 両方に足した。`TestClearingTheRepoHidesItsStaleCandidates` が再発を止める

**教訓（実バグではなく、テスト自体の欠陥 2 件）:**

- Task 8 の author 候補のテストは最初ユーザー名に `kukv` を使っており、
  リポジトリ名 `kukv/octoscope` の行にたまたま部分一致していたため、機能を
  止めても RED にならず偶然 PASS していた。画面のどこにも出ない `octocat` に
  差し替えて RED を取り直した
- 上のキャッシュ空振りと合わせて、`tests-that-cannot-fail` の同じ形
  （「主張する状態に到達しないヘルパーからモデルを組む」「ヘルパーの戻り値を
  捨てる」）が今回も 2 件出た。**キーを送るテストは、送った結果の `tea.Cmd` /
  戻り値を本当に使っているか、機能を外して RED になるかを都度確かめる。**

## 見つかったが直さなかったこと（理由つき）

### 1. マウスを持たない

Repos タブは `internal/tui/repo/mouse.go` を持つが、Search タブに同種のファイルはない。

**直さなかった理由:** このスライスはキーボードで完結する形を先に固め、
当たり判定はペインの寸法（フィルタペインの折りたたみ・列幅）が落ち着いてから
足すほうが手戻りが少ない。

### 2. `first: 50` のページングが無く、`50+` と描くだけ

スライス 3-1 の積み残し 1 番「50 件で切れていることを画面に出すかどうかは
3-2 で判断する」を、このスライスは「出す」で決着させた（`render.go` の
`50+` 表示）。ページング自体は引き継いだまま。

**直さなかった理由:** Work 板から引き継いだ制約で、設計（spec の §1）が
切り詰めの解消を別の機会に送っている。Search も同じ上限になる。

### 3. `searchCap` が `work.graphql` の `first: 50` と黙って結合している

`internal/tui/search/render.go:44` の `searchCap = 50` は、
`internal/gh/cli/work.graphql:11` の `first: 50` と一致していることが前提の
値である。片方だけを変えても、それを検知するテストは無く、`50+` の表示が
実態と食い違ったまま緑が続く。

**直さなかった理由:** `internal/tui` は `internal/gh/cli` を import できないので、
一致を保証する定数を置くなら `internal/gh` になる。ドメイン型のパッケージに
UI の表示都合の定数を置く判断が要り、この局面で決めるより、ページングを
入れる機会（積み残し 2 番）に一緒に決めるほうが筋が良い。

### 4. `s` は未割当

`internal/tui/search/search.go` はフィルタ・結果どちらのペインでも `s` に
何も割り当てていない（`TestSDoesNothingYet` がそれを確認している）。

**直さなかった理由:** 3-3（保存クエリ）で使う予約であり、このスライスの
範囲外。

### 5. 生クエリからフィルタへの逆解析をしない

`e` で編集した生クエリは左のフィルタに戻らない。

**直さなかった理由:** 設計が「生クエリの編集は一方向とする」と明記している
（`docs/superpowers/specs/2026-09-08-phase4-design.md:113`）。

### 6. チップを選んで入力欄に入れる操作が無い

label / author のチップは表示だけで、選ぶと入力欄に入る、という操作は無い。

**直さなかった理由:** brief の範囲外で、モックアップにもその操作は無い。

### 7. 候補の取得中に再要求を防いでいない

`maybeFetchCandidates` はカーソルが `FilterLabel` / `FilterAuthor` の行に来た
瞬間にだけ評価し、同じリポジトリを 2 度は引かない（取得済みならキャッシュが効く）。
ただし応答が届く前にカーソルが離れてまた戻ると、再度取得コマンドが積まれうる。

**直さなかった理由:** 「1 打鍵ごとに引かない」という要求は満たしている。
取得は速く、応答待ちの間に往復すること自体が稀なので許容範囲と判断した。

### 8. 設計 §5 の「結果の表を Repos と共有する」を満たしておらず、複製になっている

設計（`docs/superpowers/specs/2026-09-08-phase4-design.md:119-120`）は
「結果の表は Repos の右ペインと同じ描き方をする。`internal/tui/repo` の表の部分を
切り出して両方から使う」と書いているが、実際には次の形で重複している。

- `internal/tui/search/render.go:328` の `resultRow` と
  `internal/tui/repo/render.go:196` の `row` が同型
- 同じく `resultWindow`（`search/render.go:319`）と `rowWindow`
  （`repo/render.go:187`）が実質同一
- `stateColumn` / `numberColumn` / `ageColumn`（`search/render.go:28-31` と
  `repo/render.go:20-23`）、`footerHeight`（`search/render.go:40` と
  `repo/render.go:31`）が両パッケージで別々に宣言されている
- 加えて `labelChips` / `authorChips`（`internal/tui/search/render.go:215,231`）は
  「入らないものは落とす」ルールが `internal/tui/repo/render.go:279` の
  `badges()` と同じ形をしている（`badges()` が非公開で呼べないための重複。
  同じ 2 パッケージ間の同じ判断なので、ここに統合する）

**直さなかった理由:** 列構成が違う（Search は `rp`（repo）があり `ck`（checks）が
無い）ため、行の描画関数をそのまま共有できない。共通化するなら「どちらの
列構成も描ける関数」を設計するところからになる。このスライスは計画の前提 1
（桁の道具だけを共通化し、`layout.Clip` / `Pad` / `Right` / `JoinPanes` はすでに
統合済み）でこの判断を一度しており、実際に 2 つ並べてみた結果を持ち越す。
3 人目の利用者が付く機会か、3-1 の積み残し 4 番（`work`/`checks`/`diff` の
`clip`/`fit` 重複）とまとめて扱う。

### 9. en のキーバーだけコロンの後に空白がある

`internal/i18n/locales/active.en.yaml` の Search のキーバー（`footer.search.*`、
274 行目以降）は `j/k: field` のようにコロンの後に空白があるが、既存の他タブ
（`footer.list.*`、249 行目以降）は `j/k:move` / `g:import` のように空白が無い。
ja は `j/k:項目` のようにどちらも既存と揃っている（空白は無い）。
en だけ 1 桁ずつ幅を食うため、`FitKeyBar` が ja より早くヒントを落とす。

**直さなかった理由:** 実装計画がこの形（コロンの後に空白）を指定しており、
揃える先（既存の詰めた形に寄せるか、Search の形に全部寄せるか）は Search
だけの話ではなく文言全体の話になる。このスライスで片側だけ変えない。

### 10. 起動時に必ず 1 回 `is:open` の検索が走る

`internal/tui/app/app.go:436` は最初の取得で `m.repo.Init()` と並べて
`m.search.Init()` を呼んでいる。`search.Model.Init()`（`internal/tui/search/search.go:162`）
は `runSearch` を即座に投げるため、利用者が `3` を押して Search タブを
開かなくても `gh` が 1 本増え、`repo:` も `org:` も指定していない検索
（フィルタの既定値だけの `is:open` 相当、GitHub 全体からの 50 件）が返る。

**直さなかった理由:** タブを開いた瞬間に結果が出ているほうが速く感じられ、
Repos タブも同じ形（起動時に一覧を取り終えている）。ただし「最初に `3` を
押したときに走らせる」選択肢はあり、実端末で起動直後の遅さが気になったら
見直す。

### 11. 生クエリを空にして確定すると、フィルタから組み立て直した結果に戻る

`e` で生クエリを全部消して enter すると、`m.raw` が空文字列になり、
「フィルタから組み立て直す」の扱いに戻る。空クエリそのものを検索したい、
という要求は brief のテストに無く、この曖昧さは未解決のまま残っている
（Task 4 実装者の懸念）。

**直さなかった理由:** 現状の型（`string`）では「未設定」と「空文字列」を
区別できない。3-3 で保存クエリに空の生クエリを保存する意味を決めるときに、
`*string` などへの変更とあわせて判断するのが筋が良い。

## スライス 3-3（保存クエリ）に渡すもの

- `s` が空いている。`Ctrl+O` のポップアップは `internal/tui/dialog` が
  2 人目の利用者を得る場面（Repos の追加ダイアログの積み残し 8 番、候補
  スクロールの決定と合わせて検討する）
- `config.Store` に `SaveQueries` を足す（`SaveRepositories` と同じ形。
  `save(Config)` は共通化済み）
- `default_tab` に `search` を足す。今は `config.Config.WantsRepos()` が
  `default_tab: repos` かどうかの真偽値で持っている（`internal/config/config.go:29`）
- 生クエリの「未設定」と「空文字列」を区別しない現状（積み残し 8 番）は、
  保存クエリが生クエリを保存対象にするなら 3-3 で先に決める
