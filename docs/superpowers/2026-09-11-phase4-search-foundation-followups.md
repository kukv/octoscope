# Search 基盤スライスの積み残し

Phase 4 スライス 3-1「Search タブの土台」（`docs/superpowers/plans/2026-09-11-phase4-search-foundation.md`、
Task 1〜5）で見つかったが、このスライスでは直さなかったもの。
**直さないと決めた理由も書く。**

## 見つかったが直さなかったこと（理由つき）

### 1. `first: 50` の上限とページングの無さ

Work 板から引き継いだ制約。`SearchItems` は任意のクエリを投げられるので、
1 リポジトリに絞った Work 板より引きやすく、50 件で打ち切られることが起きやすい。

**直さなかった理由:** 設計（`docs/superpowers/specs/2026-09-08-phase4-design.md` §1）が
Work 板の切り詰めを別の機会に送っており、Search も同じ文書を使う以上同じ上限になる。
スライス 3-2 で「50 件で切れている」ことを画面に出すかどうかを判断する。

### 2. `internal/usecase` への配線が無い

`SearchItems` を呼ぶ画面がまだ無く、interface は利用側で宣言する設計なので、配線していない。

**直さなかった理由:** `internal/tui/search` が生まれるスライス 3-2 で、そこから見えるように
interface を宣言してから配線を足す。計画の「このスライスに入れないもの」に書いてある。

### 3. 録りもの逸脱: `SearchItems` の fixture が計画と異なる

Task 4 で `repo:kukv/octoscope is:pr sort:updated-desc` での録りを計画していた。
実行時に該当する open PR が無かったため、`repo:kukv/octoscope` で全件
（closed を含む）を録った。

**直さなかった理由:** `TestSearchItemsParsesARecordedSearch` は 2 つ以上の異なる状態が
入っていることを検査する。状態の多様性が検証対象であり、状態フィルタの有無は問わない。
理由は `internal/gh/cli/testdata/README.md` に記録済み（スライス 3 の他の逸脱と同じ形）。

### 4. `clip` / `fit` の重複が `internal/tui/layout.Clip` と共通化されていない

`internal/tui/layout/columns.go:11` の `layout.Clip` と同じ実装の `clip` が
`internal/tui/work/render.go:392`、`internal/tui/checks/render.go:357`、
`internal/tui/diff/render.go:570` の 3 箇所に残っている（同型の `fit` も
それぞれ 397 / 360 / 573 行目に残っている）。

**直さなかった理由:** このスライスのスコープは Search が実際に使う 2 パッケージ
（`internal/tui/repo` と `internal/tui/dialog`）だけで、Work / Checks / Diff の
3 つの表示系を巻き込むと golden が動かないことの確認範囲が広がるため。

## スライス 3-2（Search タブの UI）に渡すもの

- **共通の layout 関数:** `layout.Clip` / `layout.Pad` / `layout.Right` / `layout.JoinPanes`。
  Task 1 で `internal/tui/repo` と `internal/tui/dialog` が持っていた同じ関数を統合した。
  次のスライスで結果表の描き分けに使う

- **WorkItem.State:** `internal/gh` に `State`（`gh.ItemState` 型）フィールドを足し、work.graphql で
  PullRequest と Issue の両方から `state` を選ぶようにした。SearchItems の結果にも含まれる

- **結果行の列構成:** モックアップの S2 どおり `st / rp / num / ttl / ag`（state / repo / number /
  title / age）。Repos の右ペイン（`st / num / ttl / ck / ag`）との違いは
  repo 列が増え、checks 列が無いこと

## スライス 3-3（保存クエリ）に渡すもの

- **config.Store の拡張方針:** `SaveRepositories` は既にあり、`save(Config)` の共通パスで書き込んでいる。
  `SaveQueries` を足すときも同じ形に乗せる

- **ダイアログの候補スクロール決定は保留:** 前のスライス（スライス 2-3）の積み残し 8 番
  （初回投入の候補に上限がない）から引き継いだ。このスライス（3-3）で新しく保存クエリの
  ポップアップが 2 人目の利用者になるので、2 つの候補一覧を一度に整える方が筋が良い
