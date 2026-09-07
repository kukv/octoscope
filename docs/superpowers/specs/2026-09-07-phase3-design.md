# Phase 3 設計: checks / merge / ページング

画面の設計は
`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`
の §4.4.3（checks ビュー）と §4.4.4（merge）にある。**画面についてはそちらが正。**
この文書は、そこに書ききれないバックエンドと境界、テスト、計画の割り方を扱う。

## 1. 範囲

| 入れる | 入れない |
|---|---|
| checks 一覧・失敗ジョブのログ・ワークフローの再実行 | スレッドへの返信・解決（Phase 2 の範囲外のまま） |
| merge（squash / merge / rebase）と auto-merge | Repos サイドバー・追加ダイアログ、Search タブ（Phase 4） |
| `contexts` と各スレッドの `comments` のページング | `internal/gh/api` の API フォールバック（Phase 4） |

## 2. 実測して確かめたこと（2026-09-07）

**推測ではなく `gh api graphql` と `gh run` の実出力で確認した。**
この節の値が設計の前提であり、外れたら設計を直す。

| 確かめたこと | 結果 |
|---|---|
| `CheckRun.databaseId` は Actions のジョブ id か | **そう。** `detailsUrl` の `.../job/<databaseId>` と一致する。`gh run view --job <id>` にそのまま渡せる |
| run id の在り処 | `CheckRun.checkSuite.workflowRun.databaseId`。`gh run rerun [--failed] <id>` に渡せる |
| `mergeable` / `mergeStateStatus` にプレビューヘッダが要るか | **要らない。** ただし `UNKNOWN` が普通に返る（GitHub が計算中） |
| リポジトリ側のマージ設定 | `squashMergeAllowed` / `mergeCommitAllowed` / `rebaseMergeAllowed` / `autoMergeAllowed` / `deleteBranchOnMerge` が引ける。実測した kukv/octoscope は `autoMergeAllowed: false` |
| `gh run view --log` の行の形 | `ジョブ名 \t ステップ名 \t タイムスタンプ 本文` のタブ区切り。タイムスタンプを持たない継続行がある |
| ログの量 | 失敗ジョブ 1 件で `--log-failed` 28 行、`--log` 251 行 |
| 成功ジョブに `--log-failed` | **出力が空・終了コード 0。** エラーではない |
| `gh run rerun --job` が要る id | Web の URL に出る番号ではなく `databaseId`（`gh` 自身が help で注意している）。§2 の 1 行目の値と同じ |

進行中の run に `gh run view --log` を投げたときの挙動は**まだ測っていない**。
実装のときに実物で測り、その stderr を fixture に録って専用の文言を出す。
測るまで文言を決め打ちしない。

## 3. バックエンドの取り方

**merge と auto-merge は GraphQL の mutation、ログと再実行は `gh run`。**

| したいこと | 手段 |
|---|---|
| マージ | `mergePullRequest` |
| auto-merge の有効化 / 解除 | `enablePullRequestAutoMerge` / `disablePullRequestAutoMerge` |
| 失敗ステップのログ | `gh run view --job <jobID> --log-failed` |
| 全ログ | `gh run view --job <jobID> --log` |
| 再実行 | `gh run rerun <runID>` / `gh run rerun --failed <runID>` |

`gh pr merge` を採らない理由: ローカルの作業ツリーに触れうるうえ、対象 PR を
cwd から推測する経路が混ざる。mutation ならレビュー（§4.4.2）と同じく
GitHub 側だけで完結し、Phase 4 で API 実装に差し替える面も小さい。

再実行に mutation は存在しない。ログの取得も GraphQL からは行単位で取れない。
この 2 つだけ `gh` のサブコマンドに落ちるのは、**GraphQL に無いから**であって
実装が楽だからではない。

## 4. 境界

`internal/tui` は `internal/gh/cli` を import しない（depguard が落とす）。
Phase 2 と同じく、**サブモデルが自分の要る操作を interface で宣言し、
`internal/usecase` がそれを実装する。**

- `internal/gh` にドメイン型を足す: ログの 1 行（ステップ名・時刻・本文）、
  再実行の粒度、マージ方式、マージの可否とその理由
- `internal/gh/cli` に `gh run` の呼び出しとタブ区切りのパーサ、
  merge 系の mutation を置く。既存の `run` フィールド差し替えでテストする
- `internal/usecase` に checks 用と merge 用の口を足す。既存の `review.go` に倣う

**`gh` の出力の解釈は `internal/gh/cli` に閉じる。** タブ区切りを剥がすのも、
空出力を「失敗したステップは無い」に翻訳するのも、TUI ではなくここでやる。
TUI が受け取るのは既に意味の付いた型である。

## 5. ページング

**書き始めた時点の想定は外れていた。** Phase 2 の handoff と standalone spec §4.4.1 は
「`reviewThreads`（100 件）、各スレッドの `comments`（50 件）、pending review の
`comments`（100 件）がページングされていない」と書いているが、コードを読むと:

- `reviewThreads` は **既に全ページ取れている**（`cfdd925`「walk every page of review
  threads instead of stopping at 100」）。上限も置いていない
- pending review の `comments` は **そもそも取っていない**。`review.graphql` は
  未提出レビューから id しか選ばない。穴ではない

実際に残っている穴は 2 か所である。

| 場所 | 現在 | 何が起きるか |
|---|---|---|
| `statusCheckRollup.contexts` | `first: 100` | 101 件目以降の check が黙って消える。4.4.3 がそこを読む |
| 各スレッドの `comments` | `first: 50` | 51 件目以降のコメントが黙って消える |

**上限（何ページまで）は置かない。** `reviewThreads` が `cfdd925` で上限なしに
全ページ取る形になっており、ここだけ別の流儀にする理由が無い。ページ数の上限を
置くとしたら実際に遅くて困った測定が根拠になるはずで、その測定はまだ無い。

`comments` は nested connection なので、追うにはスレッド 1 件につき 1 リクエストが
要る（`review.graphql` のコメントがそう書いている）。**50 件を超えたスレッドだけ**
追加で引く形にし、超えていないスレッドには 1 リクエストも足さない。

`contexts` は `statusCheckRollup` の直下なので、`reviewThreads` と同じ形で
cursor を追える。

**この計画で扱わない別の切り詰め**: Work の search は 1 列 `first: 50`、
`labels` は `first: 100`。どちらも Work 板の話で、checks とも review とも
関係が無い。見つけたことだけ記録し、直すのは別の機会にする。

## 6. テスト

`.claude/rules/testing.md` に従う。Phase 3 で特に効くもの:

- **ネットワークもサブプロセスも叩かない。** `gh run view --log` の出力、
  merge の mutation の応答、ページングの `pageInfo` は、実物から録った
  `testdata` の fixture を使う（個人情報とトークンは伏せる）
- **状態は `Update` にメッセージとキーを渡して到達させる。** フィールドを
  直接組み立てない
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してから
  コミットする**（`tests-that-cannot-fail` の再発防止）
- golden は checks ビューと merge ポップアップを en / ja × 80 / 120 / 160 で録る。
  キーバーは **ja の 80 桁に収まること**
- 新しい文字列は `active.en.yaml` と `active.ja.yaml` の両方に足す
- ページングは「2 ページ目がある fixture」を持ち、2 ページ目の中身が
  結果に入ることを確かめる。`reviewThreads` の既存のテストに倣う

## 7. 計画の割り方

3 本 → 3 PR。順に **checks → merge → ページング**。互いにデータの依存は無い。

1. **checks**（§4.4.3 + §5 の `contexts`）
2. **merge**（§4.4.4）
3. **ページング**（§5 の各スレッドの `comments`。Phase 2 の積み残し）

各 PR は `make check` が緑であること。

## 8. 完了条件

1. checks ビューが失敗した check を先頭に出し、既定で失敗ステップのログを見せる
2. 成功ジョブと外部 CI（StatusContext）で `L` / `R` が無言で無視されず、理由が出る
3. `R` から失敗ジョブのみ / 全体の再実行ができる
4. merge ポップアップがリポジトリの許した方式だけを出し、`mergeable: UNKNOWN` を
   エラーではなく「計算中」として扱う
5. auto-merge を有効化・解除できる。使えないリポジトリではその理由が出る
6. `contexts` と各スレッドの `comments` が 1 ページを超えて取れる
7. `internal/tui` が `internal/gh/cli` を import していない
8. golden が en / ja × 80 / 120 / 160 で録れており、ja の 80 桁でキーバーが収まる

9 は人手。**実在の PR を TUI からマージでき、失敗した check のログを読める**こと。
TTY が要るのでこの環境では代行できない。受け渡しの手順は Phase 2 と同じ形で
`docs/superpowers/` に書く。
