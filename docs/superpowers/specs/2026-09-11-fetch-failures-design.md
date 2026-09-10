# 設計: 取得の失敗から立ち直る

Work 板の取得が `gh api` の 502 で落ち、そのたびにアプリを終了するしかない。
この文書は、その 2 つ（502 が出ること・出たら終わるしかないこと）を分けて扱う。

`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` の §4.1（Work タブ）と
`.claude/rules/errors.md` が上位にある。この文書はそこに書かれていない
「失敗したあと何が起きるか」を決める。

## 1. 実測して確かめたこと（2026-09-11）

**推測ではなく `gh` と GitHub の実出力で確認した。**
対象は `kukv` のアカウント、Work 板は 76 件返る状態。

| 確かめたこと | 結果 |
|---|---|
| `work.graphql` 全体を投げると何が起きるか | **11 回中 2 回が 502。** 成功した回は 8.3〜16.6 秒 |
| 502 の中身 | GraphQL のエラーではなく **nginx の HTML**（`<title>502 Bad Gateway</title>`）。`gh` は `gh: HTTP 502` を stderr に出して終了コード 1 |
| `reviewRequested` だけ | 5.5〜13.0 秒、2 回とも成功 |
| `yourPRs` だけ | 3.5〜4.1 秒、2 回とも成功 |
| `assigned` だけ | 3.5〜8.0 秒、2 回とも成功 |
| `mentioned` だけ | 1.8〜1.9 秒、2 回とも成功 |
| `statusCheckRollup` を丸ごと落とすと | 8.0〜14.4 秒。**縮むのは 2 秒ほどで、支配的な要因ではない** |
| `first: 50` を `20` にすると | 7.3〜12.8 秒（46 件）。件数におおむね比例する |
| `repo_counts` を 45 リポジトリで | 4.0 秒、502 は出ない |

**特定の重いフィールドがあるのではなく、4 つの search を 1 リクエストに詰めていることが
効いている。** 各 search の時間が 1 本の HTTP に直列に積み上がり、GitHub の手前の
プロキシが待ちきれずに切る。個別に投げればどれも 13 秒以内に収まり、8 回で 1 度も
502 にならなかった。

`internal/gh/cli` の `runGh` に再試行は無い。502 が 1 回返るとそのまま
`work.ErrorMsg` になり、`internal/tui/app` が全画面のエラー画面に遷移する。
そこでは `esc` も `q` も終了なので、**起動直後に 1 回 502 を引くと終了しか選べない。**

## 2. 範囲

| 入れる | 入れない |
|---|---|
| エラーの分類（一時的な失敗・認証切れ） | `first: 50` の切り詰め（Phase 3 が別の機会と記録済み） |
| 一時的な失敗をタブの 1 行に出す | `internal/gh/api` バックエンド、Search タブ（Phase 4 のスライス 3・4） |
| Work 板を列ごとの 4 リクエストに分ける | 列ごとの再試行キー（`r` で板ごと引き直せば足りる） |
| 読み取りの 1 回再試行 | 自動再取得・ポーリング |

## 3. エラーの分類

`.claude/rules/errors.md` は 3 種類を分けて扱うと言っている。
今センチネルになっているのは `ErrGhNotFound` だけで、502 は文字列に埋もれている。

`internal/gh` に 2 つ足す。**分岐に使うものだけをセンチネルにする**（同ルール）。

```go
// ErrTransient wraps a failure GitHub's front end produced rather than
// answered -- 502, 503, 504. The request was well-formed and retrying it
// is the right response.
var ErrTransient = errors.New("GitHub did not answer")

// ErrUnauthenticated is returned when gh has no usable credentials.
var ErrUnauthenticated = errors.New("not authenticated; run: gh auth login")
```

判定は `internal/gh/cli` の `runGh` で行う。`gh` の stderr に `HTTP 502` /
`HTTP 503` / `HTTP 504` があれば `ErrTransient` で包む。404 もレート制限も
センチネルにしない — 原文をそのまま見せるだけで、TUI は分岐しない。

`internal/gh/cli` に i18n を入れない（依存の向き）。案内文への差し替えは
今までどおり TUI 側で `errors.Is` して行う。

### どこに出すか

| 種別 | 行き先 |
|---|---|
| `ErrGhNotFound` / `ErrUnauthenticated` | 全画面。利用者が行動しないと何も動かない |
| それ以外すべて | そのタブの 1 行 |

## 4. 通知の置き場所

**各サブモデルが自分の `notice` を持つ**（`.claude/rules/tui.md`「サブモデルは
自分の状態だけを持つ」）。root に持たせると「どのタブの失敗か」を表す状態が
`errOverlay` / `errFromOverlay` に続いてもう 1 組増え、Repos の失敗が Work を
見ているときに出る混線も防げない。

```go
// notice is a failure the view can carry on despite: the previous answer
// is still on screen, and r asks again. It is cleared when a fetch
// succeeds, so a stale complaint never outlives what it described.
notice string
```

- 描画は `internal/tui/layout` に `Notice(text string, width int) string` を置いて
  共有する（`FitKeyBar` と同じ場所・同じ考え方）
- 文言は `internal/i18n` から引き、GitHub が言ったことを続けて出す
- **成功で消す。** 失敗したまま `r` が通ったのに警告が残るのは嘘になる
- `ErrorMsg` は「全画面に値する失敗」専用になる。名前もそう読めるものに変える

**キーバーとは別の行にする。** ja の 80 桁は既にぎりぎりで、混ぜると溢れる。
板の高さが 1 行減るので、`work` と `repo` の行数計算をそのぶんずらす。

この変更で、`internal/tui/repo` の `errMsg` が一覧の取得失敗とブラウザを開く失敗を
兼ねている問題（`docs/superpowers/2026-09-09-phase4-repos-sidebar-followups.md` の 9）も
解ける。ブラウザが開けなかったのは 1 行で足りる。

## 5. Work 板の分割

`work.graphql` を 1 つの search をパラメータで受ける形に変える。
**フラグメントは 1 組のまま**で、`schema_test.go` が全文書を録画スキーマに
当てている構造を壊さない。

```graphql
query ($query: String!) {
  results: search(type: ISSUE, first: 50, query: $query) {
    nodes { ...WorkItem }
  }
}
```

検索文字列は Go 側へ移す。列と検索式の対応表（`workSearches`）は既に Go にあるので、
そこに寄せる。

```go
func (c *Client) ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error)
```

`ListWork` は消す。`gh.Work`（`[WorkSectionCount][]WorkItem`）は `work.Model` の
格納先として残る。

`work.Model`:

- `Init` / `Refresh` が 4 本の `tea.Cmd` を返す。答えは自分の列番号を持って帰る
- `loading` と `notice` を**列ごと**に持つ。1 列が 502 でも他の 3 列は描かれる
- `Cancel()` は 1 つの context を 4 本で共有し、今までどおり 1 回で全部止める

**タブ行の集計**（`3件 要対応 · 2分前`）は、4 列すべてが一度答えるまで出さない。
時刻は**成功した答えのうち一番古いもの**とする。画面に載っている一番古いデータの
年齢がそれだからである。

## 6. 再試行

`internal/gh/cli` に読み取り専用の経路を 1 つ足し、`ErrTransient` のとき
**1 回だけ**引き直す。

**書き込みは通さない。** マージ・レビュー提出・コメント・close/reopen・
ラベルと担当者の編集・rerun を再試行すると、二重に届く危険がある。
502 は「届かなかった」ではなく「答えが返らなかった」であり、
サーバに届いていた可能性を排除できない。

**遅延は入れない。** 502 が返るまでに既に 11〜16 秒かかっており、その間に負荷の山は
動いていると考えられる。**ただしこれは推測であり、実装時に即時再試行の成功率を
実測してこの節に数字を足す。** 数字を持てなければ遅延の有無は決められない
（`.claude/rules/` の外だが、このリポジトリは根拠を持てない数字を書かない方針である）。

## 7. 境界

Phase 2・3・4 と同じ。`internal/tui` は `internal/gh/cli` を import しない。
サブモデルが自分の要る操作を interface で宣言し、`internal/usecase` が実装する。
`ListWorkSection` は `usecase` を通る。

## 8. テスト

`.claude/rules/testing.md` に従う。この変更で特に効くもの:

- **再試行は `run` の差し替えで検証する**（既存の `graphql_test.go` と同じ形）。
  **書き込み系が再試行されないことも主張する** — 通ってしまうと二重送信になる
- 列ごとの loading と notice は `Update` にメッセージを渡して到達させる。
  フィールドを直接組み立てない
- **通知が出ている板の golden を録る**（en / ja × 80 / 120 / 160）。
  **ja の 80 桁で通知とキーバーが両方収まること**
- 新しい文言は `active.en.yaml` と `active.ja.yaml` の両方に足す
- 502 の応答は実物から録った fixture を使う（nginx の HTML であって JSON ではない）

## 9. 完了条件

1. Work 板の取得が 502 で失敗しても板は残り、キーバーの上に 1 行出る
2. `r` で引き直せ、成功したら通知が消える
3. 1 列が失敗しても他の 3 列は描かれる
4. 列は届いた順に埋まる
5. `gh` が無い・認証していないときだけ全画面のエラー画面に行く
6. 読み取りは 502 で 1 回再試行し、書き込みは再試行しない
7. `internal/tui` が `internal/gh/cli` を import していない
8. `make check` が緑
