# octoscope: スタンドアローン GitHub TUI への刷新 設計書

- 日付: 2026-09-05
- 対象リポジトリ: `kukv/herdr-plugin-github-dash` → `kukv/octoscope`
- UI モックアップ: https://claude.ai/code/artifact/96b1dad5-ed75-4176-b110-20923b1b565e

## 1. 背景と目的

現在のこのリポジトリは Herdr プラグイン専用の pane プロセスとして作られている。
起動経路が Herdr に固定されており（`HERDR_PLUGIN_CONTEXT_JSON` からの cwd 解決、
`open.sh` / `run.sh` という bash スクリプト、`herdr-plugin.toml` のアクション定義）、
そのため次の制約を抱えている。

- **Windows で使えない。** プラグインの起動経路が bash スクリプトであり、
  `herdr-plugin.toml` の `platforms` も `["linux", "macos"]` に限定されている。
- **単体で起動できない。** Herdr のワークスペース外からは何もできない。
- **機能が少ない。** 単一リポジトリの PR/Issue 一覧・詳細・コメント・
  ラベル/アサイニー編集・close/reopen のみ。レビュー、diff、CI、マージ、
  リポジトリ横断の閲覧はいずれもできない。

本設計はこれを、**Windows / macOS / Linux で動くスタンドアローンの
GitHub ダッシュボード TUI** として作り直すためのものである。
Herdr との統合は完全に廃止する（Herdr からは通常のコマンドとして起動すればよい）。

## 2. 名前の由来

**octoscope** = **Octo**cat + **-scope**。

- **Octo-**: GitHub のマスコットである Octocat から。このツールが見るものが
  GitHub であることを一目で示す。
- **-scope**: telescope（望遠鏡）、periscope（潜望鏡）、microscope（顕微鏡）と
  同じ接尾辞で、ギリシャ語 *skopein*（見る・観察する）に由来する。
  「道具を通して、肉眼では見えないところまで見渡す」という意味を持つ。

つまり **「GitHub を見渡すための望遠鏡」**。
このツールの中心的な価値は「操作」よりも先に「散らばった自分の仕事を
1 画面で見渡せること」にある、という設計思想をそのまま名前にしている。

命名候補として `ghd`（短いが一般名詞的で検索性が低い）と
`gh-dash`（用途は明快だが著名な同名 OSS `dlvhdr/gh-dash` と衝突する）も
検討したが、固有性と意味の両立を理由に `octoscope` を採用した。

- リポジトリ名: `octoscope`
- Go モジュールパス: `github.com/kukv/octoscope`
- バイナリ名: `octoscope`

## 3. 全体アーキテクチャ

### 3.1 パッケージ構成

凡例: `[Phase 1]` は実装済み、無印は未着手（設計上の置き場所として予約してある）。

```
cmd/octoscope/        エントリポイント、フラグ解析                          [Phase 1]
internal/gh/          GitHub アクセス層が返すドメイン型                     [Phase 1]
  ├ cli/              gh CLI バックエンド（既存 internal/ghcli を移設）     [Phase 1]
  └ api/              go-github + githubv4 バックエンド                    (Phase 4)
internal/config/      設定ファイルの読み込み                                (未着手)
internal/i18n/        メッセージカタログ（en / ja、go:embed）               [Phase 1]
internal/tui/         Bubble Tea モデル群
  ├ app/              ルートモデル、タブ切替、共通キーマップ                [Phase 1]
  ├ work/             「自分に関係する仕事」ビュー                         [Phase 1]
  ├ repo/             Repos タブの右ペイン（1 リポジトリの PR/Issue 一覧） [Phase 1]
  ├ repos/            Repos タブ全体（サイドバー + リポジトリ追加ダイアログ + repo/）(Phase 4)
  ├ search/           クエリビルダーと結果                                  (Phase 4)
  ├ dialog/           ポップアップ（リポジトリ追加、保存クエリ呼び出し）    (Phase 4)
  ├ detail/           PR/Issue 詳細                                        [Phase 1]
  ├ icon/             状態アイコン                                        [Phase 1]
  ├ layout/           表示幅に応じた行の折り返し・切り詰め（`ClipLines`）  [Phase 1]
  ├ diff/             diff ビューア（ファイルサイドバー + 行コメント）      (Phase 2)
  └ review/           レビュー提出のポップアップ                            (Phase 2)
```

`internal/tui/repo`（単数）と `internal/tui/repos`（複数）は綴りが似ているが別のパッケージ。
`repo` は Phase 1 で作った、1 つのリポジトリの PR/Issue 一覧だけを描く右ペイン。
`repos` はサイドバーとリポジトリ追加ダイアログを含む Repos タブ全体で、Phase 4 で新設する。
`repo` を `repos` に改名するのではなく、`repos` が `repo` を右ペインとして内包する形になる。

刷新前の UI は、一覧・詳細・コメント入力・ピッカーの状態が 1 つのモデルに
同居していた。ここにタブ・diff・checks を足すと破綻するため、
刷新の一環としてビュー単位のサブモデルへ分割する。各サブモデルは
「何を表示するか」「どの操作を受け付けるか」を自身の `Update`/`View` に閉じ込め、
親モデルとはメッセージ型のみで通信する。

### 3.2 バックエンド抽象

`internal/gh` は interface を export しない（`.claude/rules/architecture.md`
「interface は利用側で定義する」）。バックエンドの差し替えは、`internal/gh` 側に
共通の `Client` interface を置くのではなく、両バックエンドが**同じドメイン型を返す
具体型**として実装され、それぞれの利用側（`internal/tui` の各サブモデル）が宣言する
小さな interface を両方とも満たす、という形で成立する。利用側は「自分が呼ぶメソッド」
だけを interface として宣言するため、`cli` と `api` のどちらが裏で動いているかを
知る必要がない。

| 実装 | 手段 | 認証 |
|---|---|---|
| `cli`（既定） | `gh` コマンドの実行（`gh api graphql` を含む） | `gh auth login` に委ねる |
| `api`（フォールバック） | `go-github` / `githubv4` による HTTP 直叩き | `GH_TOKEN` / `GITHUB_TOKEN` |

起動時に `exec.LookPath("gh")` を試み、見つかれば `cli`、見つからなければ
環境変数のトークンで `api` を構築する。どちらも利用できない場合は、
`gh auth login` の実行かトークンの設定を促すエラー画面を表示する。

`internal/gh/cli` の `Client` は `run`（`runFunc` 型）を差し替え可能なフィールドとして
持っており、この抽象化はその延長線上にある。

### 3.3 データ取得

主軸は GraphQL（`gh api graphql` もしくは `githubv4`）。
リポジトリ横断の取得では `search(type: ISSUE, query: ...)` を使い、
`review-requested:@me`、`author:@me`、`assignee:@me`、`mentions:@me` といった
検索クエリで各セクションを構成する。リポジトリごとに `gh pr list` を
繰り返し呼ぶ方式に比べ、リクエスト数が桁違いに少ない。

また `reviewDecision` や `isDraft` といった既存コードが依存しているフィールドは
REST では取得できず GraphQL 専用である。この点でも GraphQL を主軸に据える。

### 3.4 対象リポジトリの決定

Herdr のコンテキストに代わり、次の優先順で決定する。

1. `--repo owner/name` フラグ（明示指定）
2. カレントディレクトリの git remote（`gh` と同じ暗黙の挙動）
3. どちらも無い場合はリポジトリ非依存の Work タブから開始

## 4. 画面構成

3 つのタブを並列に持ち、`1` / `2` / `3` で切り替える。起動直後は Work タブ。
タブごとに役割が違うため、共通のレイアウトを被せず、それぞれに合った形を採る。

タブは一度に全部揃うわけではない。Search タブは Phase 4 で追加されるまで存在しない。
また `--repo` フラグも git remote も無く対象リポジトリが決まらないときは、Repos タブ
自体を出さず（§3.4 参照）、Work タブだけで動く。

### 4.0 マウス操作

キーボードだけで全機能に届くことを前提としたうえで、マウスも受け付ける。
Bubble Tea v2 では `View.MouseMode = tea.MouseModeCellMotion` を立てて有効化し、
`tea.MouseClickMsg` / `tea.MouseWheelMsg` として受け取る。

| 操作 | 動作 |
|---|---|
| タブ行のクリック | そのタブへ切り替え |
| カード / 行のクリック | 選択 |
| **選択中**のカード / 行の再クリック | 詳細を開く |
| ホイール | ポインタの下の列 / 一覧でカーソルを移動、詳細ビューでは本文をスクロール |

**ダブルクリックは使わない。** Bubble Tea はダブルクリックを報告しないため、
自前で 2 回のクリックの間隔を測る必要があり、`Update` に時計が入る。
「1 回目で選択、選択中をもう 1 回で開く」なら時計もしきい値も要らず、
テストも決定的に書ける。

**ドロワーはスクロールしない。** 表示量が固定（本文 3 行 + checks 5 件）で
スクロール状態を持たないため、ドロワー上のホイールは何もしない。

マウスのメッセージは**ブロードキャストしない。** キー入力と同じく、
表示中のタブ 1 つにだけ配る。ルートモデルは自分が描いたタブ行の高さ分だけ
Y 座標を引いてから子に渡すので、子は自分が画面のどこに置かれたかを知らない。

**当たり判定は描画と同じ関数を読む。** `boardTop`、`columnWidth`、
`cardLineCount`、タブのラベル一覧などを `View` と共有する。
レイアウトを 2 度計算する当たり判定は必ずずれる。

### 4.1 Work タブ — カンバン

セクションを**列**として横に並べる。`h` / `l` で列、`j` / `k` で行を移動する。

| 列 | 検索クエリ |
|---|---|
| Review requested | `is:open is:pr review-requested:@me` |
| Your PRs | `is:open is:pr author:@me` |
| Assigned | `is:open assignee:@me` |
| Mentioned | `is:open mentions:@me` |

**画面は必ず端末に収まる。** 上から タブ行 / 盤面 / ドロワー / キーバー で、
ドロワーとキーバーの高さは固定。盤面は残りをもらい、**カーソルのある列だけが
スクロールする**（他の列は先頭から）。盤面が伸びて下のものを画面外へ
押し出してはならない。

**列見出しには件数を右詰めで置く。** Review requested は 0 件でないとき赤。
列の高さがそのまま滞留量になるのがこのレイアウトの要点だが、スクロールする
以上は数字でも言う必要がある。

**列の区切りは縦罫線。** 見出しの下に横罫線は引かない。罫線は最も長い列の
終わりで止める。空白に枠を描くと、どの列がどこまで伸びているかが見えなくなる。

**各カードは枠付きのボックス**で、中身は 2 行。1 行目に状態アイコン・番号・
タイトル、2 行目にリポジトリ名（owner を除いた短名）・checks の進捗バー・
ラベル・経過時間を置く。checks が無い PR は、バーの代わりに `approved` /
`draft` の語を置く。**選択中のカードは枠の色と背景で示す。**

画面下部のドロワーは左右 2 段組。左に本文と
`owner/name #番号 · head → base · +追加 −削除 · ラベル`、
右に checks の一覧。`enter` を押さずに中身を読める。

タブ行の右端に盤面の要約（要対応 N・CI 失敗 M・更新からの経過）を出す。

### 4.2 Repos タブ — サイドバー + サブタブ

左ペインにリポジトリ一覧、右ペインに選択中リポジトリの中身を置く 2 ペイン構成。

- サイドバーの各行には PR 件数 / Issue 件数のバッジを出し、開く前に状況が分かるようにする
- 右ペインは `tab` で **Pull Requests** / **Issues** のサブタブを切り替える。両者を混ぜない。
  サブタブ名の横に件数を出し、開かずに向こう側の量が分かるようにする
- 右ペインのリストは桁を揃えた表として描く。**状態・番号は固定幅、経過時間は右詰め、
  checks は固定幅、タイトルが残りをもらう。** 日本語のタイトルが後続の桁を押さないこと
- リストの下に選択中アイテムの要約（作成者、ブランチ、変更量、checks 一覧）を
  **固定の高さで**出す。選択が動くたびに表が上下してはならない

リポジトリ一覧は「所有者を切り替える」のではなく、**利用者が育てる一覧**とする。
サイドバー末尾の「＋ リポジトリを追加」ボタン、またはどこからでも `a` で
追加ダイアログをポップアップし、`owner/name` を入力して一覧に足す。
入力中は GitHub 検索で候補を引いて提示する。削除は `x`。
一覧は設定ファイルに永続化する。

初回起動時は一覧が空になるため、自分のリポジトリと所属 Org のリポジトリを
初期投入する導線を用意する。

### 4.3 Search タブ — クエリビルダー

左ペインでフィルタ項目（type / state / org / repo / author / label / review / sort）を
組み立て、右ペインに結果を出す。組み立て結果の生クエリは上段に常時表示し、
`e` で直接編集もできる。label や author は候補をチップとして提示する。

GitHub の検索構文を覚えていなくても絞り込めることがこの形の目的であり、
構文を網羅することは目的としない。網羅が必要な場合は生クエリの直接編集に落とす。

組み立てたクエリは `s` で保存し、保存済みクエリは `Ctrl+O` のポップアップから
呼び出す。Repos の追加ダイアログと同じ「ポップアップで選ぶ」操作に揃える。

### 4.4 詳細ビュー

PR / Issue の詳細からは、diff 閲覧、レビュー提出、checks 一覧、merge 操作へ遷移する。
diff とレビューは Phase 2、checks 一覧と merge は Phase 3 で作る。

モックアップに diff とレビューの画面は無く、キーバーに `d` / `v` / `m` が
予約されているだけである。以下の 4.4.1 と 4.4.2 がその 2 つの設計であり、
**この 2 節についてはモックアップではなくこの文章が正**である。

#### 4.4.1 diff ビュー

**左にファイルのサイドバー、右にそのファイルの diff** を置く 2 ペイン構成とし、
Repos タブと骨格を揃える。unified 表示のみとする。左右 2 分割の split は
80 桁で片側 40 桁しか残らず、コードの行がほぼ読めないため採らない。

```
┌─ 1 Work  2 Repos ────────────────────────────────────────┐
│ kukv/koto #128 feat: add relation graph traversal        │
│ feat/graph → main · 4 files +218 −31 · pending · 2       │
├─ Files ────────────┬────────────────────────────────────┤
│ ▸ graph/walk.go    │ @@ -12,7 +12,9 @@ func Walk(...)    │
│     +182 −4     ●2 │  12  12   ctx, cancel := ...       │
│   graph/walk_test  │  13  13   defer cancel()           │
│     +30  −0        │  14     - if depth == 0 {          │
│   cmd/root.go      │      14 + if depth <= 0 {          │
│     +6   −27       │      15 +   depth = defaultDepth   │
│   README.md        │  15  16   }                        │
│     +0   −0        │ ▌ ● kukv: ここは 2 が既定では?      │
└────────────────────┴────────────────────────────────────┘
 j/k 行  [/] ファイル  {/} ハンク  c 行コメント  v レビュー
```

- サイドバーの各行はパス、`+追加 −削除`、その ファイルに付いたスレッドの件数バッジ
- diff の行は `旧行番号 新行番号 記号 本文`。**両側の行番号を各行が持つ。**
  行コメントの投稿には行番号と、それが旧側か新側かの区別が要る
- 行の本文はシンタックスハイライトする（4.5 の「装飾は最大限」）
- レビューのスレッドは該当行の直下に `▌` 付きで展開する。
  **解決済み（resolved）と outdated のスレッドは畳んで件数だけ出し、
  その行で `enter` を押したときに開く。** 既に片付いた議論が diff を押し流さないため
- 自分の未提出コメントは既存スレッドと同じ場所に、色で区別して並べる

入口は Work のカード、Repos タブの行、詳細ビューのいずれからも `d`。

| キー | 動作 |
|---|---|
| `j` / `k` | 行 |
| `[` / `]` | ファイル |
| `{` / `}` | ハンク |
| `h` / `l` | ペイン |
| `c` | この行にコメントを書く |
| `enter` | 畳まれたスレッドを開く |
| `v` | レビューを提出する |
| `X` | 未提出のレビューを破棄する |
| `esc` | 戻る |

マウスはサイドバーの行のクリックでファイル選択、ホイールで diff のスクロール
（4.0 に従い、当たり判定は描画と同じ関数を読む）。

#### 4.4.2 レビューの提出

**未提出のレビューは GitHub 側の pending review として持つ。** 行コメントを
octoscope のメモリに溜めて提出時に一括で送る方式と比べ、エンドポイントは増えるが、
**octoscope を閉じても書きかけが消えず、GitHub の Web UI とも同じものを見る。**

GraphQL の 4 つの mutation で足りる。

| したいこと | mutation |
|---|---|
| 未提出のレビューを始める | `addPullRequestReview` |
| 行コメントを 1 件足す | `addPullRequestReviewThread` |
| 提出する | `submitPullRequestReview` |
| 書きかけが無い状態でそのまま提出する | `addPullRequestReview`（`event` と `body` を付ければ 1 回で済む） |
| 破棄する | `deletePullRequestReview` |

diff を開いた時点で `pullRequest.reviews(states: [PENDING])` を引き、
**既にあれば黙って引き継ぐ**。ヘッダに `pending · N` と出して、
書きかけが存在することを隠さない。

`v` で提出のポップアップを開く。event（approve / request changes / comment）を
選び、本文を書いて `ctrl+s` で提出する。提出後は PR を取り直し、
一覧のレビュー状態を更新する。

**書きかけが無くても `v` は開く。** diff を読んで何も言うことがなく approve する、
というのがレビューで最も多い形であり、そこに未提出レビューを先に作らせる理由が無い。
書きかけが無いときは `addPullRequestReview` に `event` を付けて 1 回で送る。

**提出のポップアップは詳細ビューからも開く。** diff を読まずに approve したい
場面はあり、そのために diff を経由させる理由が無い。

### 4.5 装飾

装飾の強度は**最大限**とする。具体的には次を含む。

- 行内の checks 進捗バー（`▰▰▰▰▰▱▱`）
- 変更量の推移スパークライン（`▁▃▅▇▅▂`）
- GitHub ラベルの実色を再現した塗りつぶしバッジ
- 状態アイコン（成功 / 失敗 / 実行中 / draft / approved）
- diff のシンタックスハイライト（Phase 2）

シンタックスハイライトは chroma で行う。`.claude/rules/tui.md` は
「色は `internal/tui/theme` にだけ書く」としているが、chroma のスタイルは
配色そのものであり、ビューに持たせるとこの規約が崩れる。
**chroma のスタイル名を `theme` に置き、`theme` がハイライト済みの文字列を返す。**
ビューは chroma を import しない。実装時に rules も同じ内容へ更新する。

アイコンは 3 つのセットを持つ。`unicode`（既定）、`nerd`、`ascii`。

**自動判定はしない。** 端末は使用中のフォントもその収録範囲も報告せず、
`TERM_PROGRAM` はパッチ済みフォントの有無について何も言わないため、
「Nerd Font が使えるか」を検出する信頼できる手段が無い。
推測して外すと、盤面が豆腐で埋まって読めなくなる。

したがって選択は利用者が行い、既定はフォントを要求しないセットとする。

| 指定方法 | 内容 |
|---|---|
| `--icons unicode\|nerd\|ascii` | その場限りの上書き |
| 環境変数 `OCTOSCOPE_ICONS` | 恒久的な指定 |
| 設定ファイルの `nerd_font`（§5） | 未実装。実装時は環境変数の下に置く |

`--icons auto` および未知の値は `unicode` として扱う。
タイプミスで盤面が描けなくなるより、既定に落ちるほうがよい。

**どのセットのグリフも表示幅は 1 桁でなければならない。** Nerd Font の
グリフは私用領域にあり、幅は前提にできない（§6.4）。
`TestEveryGlyphIsOneColumn` がこれを縛っている。

### 4.6 幅への対応

端末幅が足りない場合は、次の順に段階的に劣化させる。

1. サイドバーを畳む（Repos / Search / diff。diff は 100 桁未満で畳み、
   現在のファイル名だけをヘッダに残す）
2. ドロワー・詳細ペインを畳む（Work / Repos）。**同時に Work のカードの枠も外す**
   （100 桁未満）。1 列は 17 桁ほどしかなく、枠が 2 桁を取るとタイトルが残らない。
   枠を外した分、選択はカーソル記号で示す
3. Work タブのカンバンを 1 列ずつのページングに切り替える（60 桁未満）

## 5. 設定ファイル

`os.UserConfigDir()` 配下の `octoscope/config.yaml` を読む。
カタログと同じ形式に揃え、YAML のパーサを 1 つに保つ。
これにより Windows（`%AppData%`）、macOS（`~/Library/Application Support`）、
Linux（`~/.config`）で自然な場所に収まる。

保持する内容は次に留める。設定が無くても全機能が動くことを前提とする。

| 項目 | 内容 |
|---|---|
| `repositories` | Repos タブに表示する `owner/name` の一覧 |
| `saved_queries` | Search タブで保存したクエリ（名前とクエリ文字列） |
| `default_tab` | 起動時に開くタブ |
| `nerd_font` | Nerd Font グリフの使用可否の明示指定（既定は自動判定） |
| `language` | 表示言語（`en` / `ja`、既定は自動判定） |

### 5.1 フラグと設定の関係

コマンドラインのフラグは**その場限りの上書き**に限る。恒久的な設定は設定ファイルに置く。

Phase 1 の追補で `--icons` が加わり、フラグは `--repo` / `--lang` / `--icons` /
`--version` の 4 つになった。恒久的な指定は環境変数 `OCTOSCOPE_ICONS` に逃がしてあり、
設定ファイル（`nerd_font`）を実装した時点でそちらへ寄せる。**次にフラグを足すときは、
設定ファイル側に置けないかを先に検討する。**

Phase 0 時点のフラグは `--repo` / `--lang` / `--version` の 3 つで、
フラグ定義はエントリポイントに直接書いてよい。ただしフェーズが進んでフラグが増える場合、
**4 つを超えた時点で設定ファイル側に寄せられないかを先に検討する**。
「毎回変える値」だけがフラグに残り、それ以外は設定ファイルへ移す。

フラグを増やすかどうかの判定は「利用者が起動ごとに変える値か」。
変えないならフラグではなく設定である。

### 5.2 リリースと保護ルール

配布は GoReleaser で行う。GoReleaser 自身は**コミットもタグも作らない**。
既に存在するタグを読んで GitHub Release を作り、成果物を添付するだけである。
タグを作るのは、ローカルで `git tag` して push する人間。

`kukv/structure` のリポジトリモジュールはタグに対する保護ルールセット
（作成・更新・削除を禁止）を張っており、bypass できるのは管理者ロールと
`kita_chan_bot` の GitHub App だけである。したがって:

- **タグを push できるのはリポジトリ管理者本人。** これは現在の運用と一致する
- リリースワークフローの `GITHUB_TOKEN` は Release の作成にしか使わないので、
  タグ保護には抵触しない
- **将来 CI からタグを作る形にする場合**（release-please のような自動化を入れる、
  Homebrew tap へ push するなど）は、`GITHUB_TOKEN` が bypass 対象でないため
  必ず弾かれる。そのときは `kita_chan_bot` の App トークンに切り替える。
  `GITHUB_TOKEN` で作った Release やタグは後続のワークフローを起動しない、
  という別の制約もある

## 6. 多言語対応

英語と日本語に対応する。既定は英語とし、日本語は環境または設定から選ばれたときに使う。

### 6.1 翻訳の対象と対象外

翻訳するのは**このツール自身が書く文字列**に限る。

| 対象 | 例 |
|---|---|
| ラベル・見出し | `Review requested` / `レビュー依頼` |
| キーバインドの説明 | `move` / `移動` |
| 状態語 | `draft`、`approved`、`running` |
| エラーメッセージ | `gh CLI not found...` |
| 相対時刻 | `2h ago` / `2 時間前` |
| 日時 | ロケールに応じた書式 |

翻訳しないもの:

- GitHub から取得した内容（タイトル、本文、コメント、ラベル名、ユーザー名、
  ブランチ名、checks のジョブ名）
- GitHub の検索構文（`is:open`、`label:` など）。Search タブのクエリビルダーでは、
  **項目名は翻訳するが、生成されるクエリ文字列は翻訳しない**
- コマンドラインの `--help` 出力。`--lang` 自体がフラグである以上、
  フラグを解析し終えるまで言語が決まらないため、英語固定とする
- タブ名（`Work` / `Repos` / `Search`）。キー `1` / `2` / `3` と対で
  覚える画面上の識別子であるため

### 6.2 実装方針

- カタログは `nicksnyder/go-i18n/v2` を使い、`internal/i18n` に置く
- 翻訳ファイルは YAML（`active.en.yaml`、`active.ja.yaml`）とし、`go:embed` で
  バイナリに埋め込む。3 OS でファイル配置を気にしなくて済む。
  メッセージ ID は `list.no_open_prs` のように階層を持つため、
  ネストで表現できる YAML の方が読み書きしやすい
- コード側はメッセージ ID で参照する（`i18n.T("work.review_requested")`）。
  日本語・英語のどちらもハードコードしない
- 複数形は go-i18n の plural 機能に任せる。日本語には複数形が無いため
  `other` のみを定義する

### 6.3 言語の決定順

1. `--lang en` / `--lang ja` フラグ
2. 設定ファイルの `language`
3. OS のロケール。Unix 系は `LC_ALL` → `LC_MESSAGES` → `LANG`、
   Windows はユーザー既定ロケールを参照する（`jeandeaual/go-locale` を使う。
   Windows では `LANG` が未設定なのが通常のため、環境変数だけでは判定できない）
4. 上記で決まらなければ英語

### 6.4 表示上の注意

日本語は多くが全角で、ターミナル上では 1 文字 2 桁を占める。
既存コードは `github.com/charmbracelet/x/ansi` で表示幅を計算しており、
この方針を全ビューで守る。`len()` や `utf8.RuneCountInString()` で
桁を数える実装を混入させない。特に次の箇所で桁ずれが出やすい。

- Repos タブ・Search タブの桁揃えテーブル
- Work タブのカンバンのカード（1 列あたり 30 桁前後しかない）
- キーバインドを並べるフッター

### 6.5 テスト

各ビューの golden test を `en` と `ja` の両方で回し、
桁揃えが崩れないことを検証する。翻訳漏れ（カタログに無い ID）は
テストで検出できるようにする。

## 7. フェーズ分割

各フェーズが単独で「動いて使える」状態になるように切る。
フェーズごとに spec → 実装計画 → 実装のサイクルを回す。

### Phase 0: スタンドアローン化とリネーム

- GitHub 上でリポジトリ名を `octoscope` に変更（旧 URL はリダイレクトされる）
- `structure` リポジトリ側の Terraform を追従させる（別 PR、詳細は §9）
- `go.mod` を `github.com/kukv/octoscope` に変更し、全 import を更新
- Herdr 資産の削除: `internal/herdrctx`、`herdr-plugin.toml`、`open.sh`、`run.sh`、
  および `GITHUB_DASH_URL` によるリンクハンドラ経路
- `main.go` を `cmd/octoscope/main.go` へ移動
- `--repo` フラグと cwd の git remote による対象リポジトリ解決を実装
- `internal/i18n` の土台を作り、既存の全文字列をカタログへ移す（§6）。
  以降のフェーズでは、文字列を足すときに必ず両言語のカタログへ足す
- GoReleaser 設定とリリースワークフローを追加（GitHub Actions は
  full-length commit SHA でピンする org ポリシーに従う）
- README を全面的に書き換える。英語版（`README.md`）と日本語版（`README.ja.md`）を
  用意し、相互にリンクする。ツール自身が両言語に対応する以上、
  入り口のドキュメントも揃える

**検証**: `GOOS=windows/darwin/linux` のクロスコンパイルが通ること。
`octoscope --repo kukv/octoscope` で既存の PR/Issue ビューが従来どおり動くこと。
`--lang ja` と `--lang en` の両方で golden test が通ること。

### Phase 1: Work タブと GraphQL バックエンド

- **翻訳漏れ検出ガード**（§6.5）。Phase 0 で入れたテストは `i18n.IDs()` を
  回しておりカタログ自身の ID しか見ないため、コード側の ID のタイポを検出できない。
  各ビューのレンダリング結果を en / ja で走査し、未解決 ID（`!` 前置き）が
  混ざっていないことを確認する形に置き換える
- 日時書式のカタログ化（§6.1 の「日時 — ロケールに応じた書式」）
- `internal/gh` をドメイン型のみのパッケージへ整理し、`ghcli` を `internal/gh/cli` として移設
- GraphQL search によるリポジトリ横断取得
- Work タブのカンバン（4 列 + ドロワー）の実装、タブ切替を持つルートモデル

**検証**: 複数リポジトリにまたがる自分関連の PR/Issue が 1 画面に表示される。

### Phase 2: PR レビュー

- `internal/gh` に diff とレビューのドメイン型を足す
  （`FileDiff` / `Hunk` / `DiffLine` / `ReviewThread` / `ReviewEvent`）
- `gh pr diff --color=never` の unified diff をパースする（`internal/gh/cli`）。
  **各行が旧側・新側の両方の行番号を持つ。** 行コメントの投稿がそれを要求する
- diff ビューア（4.4.1）。ファイルサイドバー、ハンク単位の移動、
  シンタックスハイライト、既存レビュースレッドの表示
- レビューの提出と行コメント（4.4.2）。GitHub 側の pending review として持ち、
  GraphQL の `addPullRequestReview` / `addPullRequestReviewThread` /
  `submitPullRequestReview` / `deletePullRequestReview` を使う
- ルートモデルのナビゲーションを `showingDetail bool` からビューのスタックに変える

**検証**: 実際の PR に対して TUI からレビューを提出できる。
**この検証は TTY と実在の PR が要るため、TTY の無い環境では代行できない。**
自動で確かめられるのは golden までであり、最後は人手で確認する。

### Phase 3: checks / merge

- checks 一覧、失敗ジョブのログ閲覧、ワークフローの再実行
- merge（squash / merge / rebase）、auto-merge の有効化

**検証**: 実際の PR を TUI からマージできる。

### Phase 4: Repos タブ / Search タブ / API フォールバック

- Repos タブ（サイドバー + サブタブ + 追加ダイアログ）
- Search タブ（クエリビルダー、保存クエリのポップアップ）
- `internal/gh/api` の実装（go-github + githubv4 + トークン管理）

**検証**: `gh` を PATH から外した状態で、`GH_TOKEN` のみで全機能が動作する。

API フォールバックを最後に置いたのは、バックエンドを 2 系統とも最初から
並走させると Phase 0〜3 のすべてで二重実装の負担が生じるためである。
Phase 0〜3 は `gh` を前提に進め、interface の境界だけを先に確定させておく。

## 8. テスト方針

既存の `runFunc` 差し替えによるテストパターンを踏襲する。

- fake は `internal/gh` に一元化せず、`internal/tui` の各サブモデルが自分の宣言した
  interface に対して自分の `_test.go` に書く。`internal/gh/cli` 自身は `run` フィールドの
  差し替えでテストする
- TUI は `tea.Model` の `Update` / `View` を golden test で検証する
  （既存の `internal/ui/ui_test.go` と同じ形）。golden は `en` / `ja` の両方で持つ
- ネットワークを実際に叩くテストは書かない

## 9. リネーム作業の注意点

`structure` リポジトリの `terraform/repository_herdr-plugin-github-dash.tf` は
モジュールラベル `repository_herdr_plugin_github_dash` でリソースを管理している。
これをリネームする際、`moved {}` ブロックを伴わずにラベルだけ変更すると
Terraform はリポジトリの destroy / recreate を計画してしまう。

手順:

1. GitHub 上でリポジトリ名を先に変更する（旧 URL は自動でリダイレクトされる）
2. `structure` 側でファイル名とモジュールラベルを変更し、`moved {}` を追加する
3. `terraform plan` の結果が `~ update in-place` であること（`-/+` でないこと）を
   確認してから適用する

この作業は `structure` リポジトリ側の独立した PR として行う。

## 10. 廃止するもの

| 対象 | 理由 |
|---|---|
| `internal/herdrctx` | Herdr のコンテキスト解決が不要になる |
| `herdr-plugin.toml` | プラグイン定義が不要になる |
| `open.sh` / `run.sh` | bash 依存の起動経路が不要になる（Windows 対応の妨げでもある） |
| `GITHUB_DASH_URL` 経路 | Herdr のリンクハンドラ専用の入口 |
