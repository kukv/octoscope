# Issue / PR 詳細画面 再設計

**日付:** 2026-09-16
**状態:** 設計
**対象:** `internal/app/presentation/tui/detail`
**置き換える対象:** `2026-09-05-octoscope-standalone-design.md` の詳細ビューの記述（本文と
コメントを 1 本の Markdown として描く前提）。他の章は有効なまま。

## 1. 目的

詳細画面は「どこからどこまでが何の情報か」が読み取れない。

現在の実装（`render.go` の `prMarkdown` / `issueMarkdown`）は、メタ情報・説明・コメントを
1 本の Markdown 文字列に連結し、glamour に渡して viewport へ流している。境界は
`---`（glamour が引く短い罫）だけで、メタ情報も箇条書きの `•` でコメント本文と同じ見た目になる。

```
  • 作成者: @kukv
  • 状態: オープン
  • レビュー: 承認済み
  • ラベル: enhancement
  • 更新: 2026年9月6日 12:00

  --------

  This replaces the renderer.

  • one            ← 本文の箇条書きがメタと同じ記号
  • two

  --------

  @bob — 2026年9月6日 12:00
```

（`testdata/detail_ja_120.golden` から ANSI を落としたもの）

**実測（2026-09-16）**

- 詳細画面が出しているメタ情報は 5 項目（作成者・状態・レビュー・ラベル・更新）
- `domain.PR` の公開フィールドは 18 個。うち `Assignees` `Checks` `Head` `Base`
  `Additions` `Deletions` は取得済みだが詳細画面には**一切出ていない**
- `a` キーで担当を変更できるのに、現在の担当者が画面のどこにも出ない
- Work ボードのドロワーは同じデータから `kukv/octoscope #12 · feat/graph → main · +218 −31 · bug`
  と checks の一覧を描いている。詳細画面の方が情報が少ない

## 2. 方針

**メタ情報を本文から空間的に分離する。** 左に固定のメタペイン、右にスクロールする本文。
横幅が足りないときだけ 1 カラムに落とす。

Work ボードのドロワー（2 ペイン）と同じ語彙を使うが、**`work` パッケージは import しない**。
画面同士の依存になるため（`.claude/rules/architecture.md`）。共有が要るのは
`theme` / `icon` / `layout` にすでにあるものだけで、新しい共有コードは作らない。

## 3. レイアウト

### 3.1 幅 100 桁以上: 2 ペイン

```
PR #12 レンダリングのパイプラインを置き換える          ← 全幅・固定
リポジトリ  kukv/octoscope #12 │ 説明
作成者      @kukv              │   This replaces the renderer.
状態        オープン            │   • one
レビュー    承認済み            │   • two
checks      ✓3 ×1 ◍1           │
ブランチ    feat/graph → main  │ コメント 2 件
変更        +218 −31           │
担当        @alice             │ ▌ @bob · 2026年9月6日 12:00
ラベル       bug               │ ▌ 見た目が良い
更新        2026年9月6日 12:00  │
                              │ ▌ @alice · 2026年9月6日 13:00
                              │ ▌ LGTM
esc:戻る  j/k:移動  c:コメント …                      ← 全幅・固定
```

- タイトル行とキーバーは**全幅**。2 ペインになるのはその間だけ。タイトルは今と同じく
  `layout.ClipLines` で切る
- 左ペインはスクロールしない。右ペインだけが viewport
- 左ペイン幅 = `width/3`、上限 **40 桁**。2 ペインになるのは 100 桁以上なので下限は要らない
  （最小でも 33 桁）。ラベル列 12 桁、値は残りに `layout.Clip`。12 なのは
  `layout.Pad` が渡された幅より 1 桁短く切るためで、最長のラベル「リポジトリ」
  （10 桁）が切られない最小の値がこれになる
- ペインの連結は既存の `layout.JoinPanes(left, right, leftWidth)`。縦罫は `theme.Rule()` の
  `│` で、Work ボードと同じ
- 左ペインが右より短いときは `JoinPanes` が罫を伸ばす（既存の振る舞い）

**端末が低いとき。** 左ペインはスクロールせず、PR では 10 行ある。`JoinPanes` は高い方の
ペインの高さぶん返すので、放っておくと高さ 12 行あたりからキーバーが画面外へ出る。
`View()` は最後に全体を端末の高さに収め、**キーバーと（あれば）エラー行を切り詰めの外に置く**。
失われるのは左ペインの末尾で、窓を広げれば戻る。キーバーを失ってよい理由は無い —
`esc` が詳細画面から出る唯一の手段で、それを知らせているのがキーバーだからである。
エラーの原文を残す理由は `.claude/rules/errors.md`（利用者がそれだけを頼りに次を決める）。

しきい値 100 は `work` の `drawerMinColumns` と同じ値・同じ形（`detail/render.go` の const
ブロックに理由つきのコメントを添える）。100 桁で右ペインは 100 − 33 − 1 = 66 桁残る。

### 3.2 幅 100 桁未満: 1 カラム

```
PR #12 レンダリングのパイプラインを置き換える
kukv/octoscope #12 · オープン · @kukv · 承認済み · ✓3 ×1 ◍1 ·
feat/graph → main · +218 −31 · 担当 @alice ·  bug  · 2026年9月6日 12:00
──────────────────────────────────────────────────────────────
説明
  This replaces the renderer.
```

- 2 ペインと**同じ項目**を ` · ` で連結し、`ansi.Wrap` で折り返す。項目は落とさない
- ラベル列は付けない。ただし担当だけは作成者と区別できないので `担当 @alice` と前置する
- ブロックの下に全幅の罫（`theme.Rule()`）を引き、その下が viewport
- 80 桁 ja で 2〜3 行に収まる見込み。行数は内容で変わるので、viewport の高さは
  この行数を数えてから決める（§5.2）

## 4. 何を描くか

### 4.1 左ペイン（1 カラムでは同じ項目を横に連結）

| 行 | 値 | PR | Issue |
|---|---|---|---|
| リポジトリ | `kukv/octoscope #12` | ✓ | ✓ |
| 作成者 | `@kukv` | ✓ | ✓ |
| 状態 | オープン / クローズ / マージ済み（draft は接尾辞） | ✓ | ✓ |
| レビュー | 承認済み / 変更要求 / レビュー待ち | ✓ | — |
| checks | `✓3 ×1 ◍1` | ✓ | — |
| ブランチ | `feat/graph → main` | ✓ | — |
| 変更 | `+218 −31` | ✓ | — |
| 担当 | `@alice @bob` | ✓ | ✓ |
| ラベル | バッジ | ✓ | ✓ |
| 更新 | `2026年9月6日 12:00` | ✓ | ✓ |

**値が空の行は描かない。** レビューが `ReviewNone`、checks が `Total == 0`、担当もラベルも
空、`Head`/`Base` が空のときは、その行ごと出さない（Work ボードの `metaLine` と同じ方針）。

色と記号はすべて既存のものを引く。`theme.Review` / `theme.Check` + `icon.Check` /
`theme.Added` / `theme.Removed` / `theme.Accent`（ブランチ）/ `theme.Badge`（ラベル）/
`theme.Dim`（ラベル列）。16 進はビューに書かない。

更新日時は今と同じ `i18n.DateTime`（絶対時刻）。`i18n.RelTime` にはしない。詳細画面は
一覧と違って 1 件を読み込む画面で、正確な時刻の方が役に立ち、ゴールデンも壁時計で腐らない。

### 4.2 右ペイン

```
説明
  This replaces the renderer.

コメント 2 件

▌ @bob · 2026年9月6日 12:00
▌ 見た目が良い
```

- Markdown を 1 本に連結するのをやめ、**説明とコメントを別々に glamour へ渡す**
- `# #12 タイトル` は生成しない。タイトルは全幅の行にあり、glamour の H1 は重複になる
- 見出し（`説明` / `コメント N 件`）は Markdown ではなく `theme.Heading()` で描く
- 説明が空なら既存の `md.no_description` の文言を出す
- コメントは `▌ @bob · 日時` の見出し行に続けて、本文の**全行**に `▌ ` を前置する。
  記号は `icon.CommentBar()`（diff のスレッド表示がすでに使っている。ASCII セットでは `|`）
- glamour が付ける前後の空行は落としてから前置する。落とさないと罫が途切れて見える
- 折り返し幅は「右ペイン幅 − 前置 2 桁」。`glamour.WithWordWrap` に渡す
- コメントが 0 件なら「コメント」の見出しごと出さない

## 5. 実装

### 5.1 ファイル

描画を足すと `render.go` が 300 行を超え、`detail.go` はもともと 925 行あった
（`.claude/rules/architecture.md` の見直しの合図）。パッケージは責務ごとに次の 7 つになる。

| ファイル | 責務 | 行数（2026-09-18） |
|---|---|---|
| `detail.go` | `Model`、4 つの interface、メッセージ型、mode / phase、`New`、寸法計算 | 354 |
| `commands.go` | GitHub に何かを頼む `tea.Cmd` 8 つ | 100 |
| `update.go` | `Update` と各メッセージのハンドラ | 319 |
| `keys.go` | mode ごとのキー処理 6 つ | 250 |
| `render.go` | `View()`、2 ペイン / 1 カラムの組み立て、ポップアップ各種 | 269 |
| `meta.go` | メタ行の組み立てと、その 2 通りの描き方 | 217 |
| `body.go` | 説明とコメントのブロック描画（glamour の呼び出しはここだけ） | 155 |
| `picker.go` | ラベル / 担当のピッカー | 132 |
| `mention.go` | 生 markdown が読み手を名指ししているかの判定 | 83 |

`prMarkdown` / `issueMarkdown` / `writeCommonMeta` / `writeBody` / `writeComments` は削除し、
`stateText` / `reviewText` は `meta.go` へ移す（呼ぶのはメタ行だけになるため）。

`update.go` は 300 をわずかに超えている。`detail.go` はメンション強調の配管が
`viewer` フィールドと `SetViewer` を足した分で 350 を超えた。300 は規則ではなく合図であり、
どちらも責務は 1 つに言えるのでこの形で止めた。

### 5.2 幅と高さ

今は `WindowSizeMsg` でしか寸法を決めていない（`detail.go:409`）。

```go
m.body.SetWidth(msg.Width)
m.body.SetHeight(max(msg.Height-4, 5))
```

新しいレイアウトでは右ペインの幅がペイン分割に、高さが 1 カラム時のヘッダー行数に依存し、
ヘッダー行数は**内容**で変わる。寸法の計算を 1 つの関数に集め、`WindowSizeMsg` と
**アイテム読み込み時（`itemMsg`）の両方**から呼ぶ。読み込み前は今と同じ全幅で構わない。

### 5.3 触らないもの

compose / confirm / submit / merge / picker の各ポップアップ、ローディング表示、
エラー行（ペインの下に全幅で出す今の位置）、キーバインドとフッターの中身、
マウスホイールの扱い（x 座標に関わらず本文へ流す今の振る舞い）。

usecase / domain / gateway には一切触らない。この変更は描画の中で閉じる。

### 5.4 i18n

`en` / `ja` 両方に追加する。片方だけだとカタログ整合テストが落ちる。

- `detail.section.description` / `detail.section.comments`（件数つき。`Tn`）
- `detail.meta.repo` / `.author` / `.state` / `.review` / `.checks` / `.branch` /
  `.changes` / `.assignees` / `.labels` / `.updated`
- 1 カラムの担当の前置は `detail.meta.assignees` を使い回す

不要になった `md.author` / `md.state` / `md.review` / `md.labels` / `md.updated` /
`md.draft_suffix` はカタログから削除する（`md.no_description` は §4.2 で残る）。

## 6. 検証

- **ゴールデンの fixture を太らせる。** 今の `goldenPR()` は `Head` `Base` `Additions`
  `Deletions` `Assignees` `Checks` がすべて空で、このままでは新しい行が 1 行も記録されない。
  6 つすべてと、コメント 2 件（1 件では罫の連続が見えない）を入れる
- 80（1 カラム）/ 120・160（2 ペイン）× en/ja のゴールデン。既存の幅がそのまま
  2 つのレイアウトを踏み分ける
- 単体テスト
  - 100 桁の前後でレイアウトが切り替わる
  - Issue では PR 専用の 4 行（レビュー / checks / ブランチ / 変更）が出ない
  - 担当・ラベル・checks が空のとき、その行が出ない
  - コメント本文の全行に `icon.CommentBar()` が付く
  - タイトルが 2 回出ない（`# #12` を落としたこと）
- `make check` が緑
- `go run ./cmd/octoscope --lang ja` を 80 桁と 120 桁で目視（`.claude/rules/tui.md`）

## 7. やらないこと

- **自分宛てメンションの強調**は `2026-09-17-mention-highlight-design.md` で設計し、実装した
- コメントの折りたたみ、返信、リアクション
- 左ペインへのカーソル移動やフォーカス切り替え（キーで畳む案は今回採らない）
- checks の一覧表示（`s` キーの checks 画面が持っている。左ペインは集計だけ）
- Work ボードと共有するメタ描画関数の抽出。形が違う（横 1 行 / 縦の行）ので、
  共有すると両方の都合を負う関数になる
