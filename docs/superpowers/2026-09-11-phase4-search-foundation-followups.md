# Search 基盤スライスの積み残し

Phase 4 スライス 3-1「Search タブの土台」（`docs/superpowers/plans/2026-09-11-phase4-search-foundation.md`、
Task 1〜4）で見つかったが、このスライスでは直さなかったもの。
**直さないと決めた理由も書く。**

## 見つかったが直さなかったこと（理由つき）

### 1. `first: 50` の上限とページングの無さ

Work 板から引き継いだ制約。`SearchItems` は任意のクエリを投げられるので、
1 リポジトリに絞った Work 板より引きやすく、50 件で打ち切られることが起きやすい。

**直さなかった理由:** 設計（`docs/superpowers/specs/2026-09-11-phase4-search-foundation.md` §1）が
Work の切り詰めを別の機会に送っており、Search でまとめて決めるのが筋が良い。
スライス 3-2 で「50 件で切れている」ことを画面に出すかどうかを判断する。

### 2. `internal/usecase` への配線が無い

`internal/usecase` の interface は利用側で宣言する設計なので、新しい `SearchItems` の
handler については配線を足していない。

**直さなかった理由:** 画面からの呼び出しが無く、今は testdata の検証だけが経路になっている。
スライス 3-2 で Filters タブの UI を実装するときに、`internal/usecase` に宣言と配線を足す。

### 3. 録りもの逸脱: `SearchItems` の fixture が計画と異なる

Task 4 で `repo:kukv/octoscope is:pr sort:updated-desc` での録りを計画していた。
実行時に該当する open PR が無かったため、`repo:kukv/octoscope` で全件
（closed を含む）を録った。

**直さなかった理由:** テストは open PR の存在を前提とせず、構造のみ検証する。
理由は `internal/gh/cli/testdata/README.md` に記録済み（スライス 3 の他の逸脱と同じ形）。

## スライス 3-2（Filters・並替・ページング）に渡すもの

- **共通の layout 関数:** `layout.Clip` / `layout.Pad` / `layout.Right` / `layout.JoinPanes`。
  Task 1 で `internal/tui/repo` と `internal/tui/dialog` が持っていた同じ関数を統合した。
  次のスライスで Results 行の描き分けに使う

- **WorkItem.State:** `internal/gh` に `WorkItem.State` 型を足し、work.graphql で PullRequest と
  Issue の両方から `state` を選ぶようにした。SearchItems の結果にも含まれる

- **結果行の列構成:** モックアップの S2 どおり `st / rp / num / ttl / ag`（state / repo / number /
  title / author）。Repos の右ペイン（pr / st / num / ttl）と異なり、state と repo が前に出ている

## スライス 3-3（保存クエリ）に渡すもの

- **config.Store の拡張方針:** `SaveRepositories` は既にある。`SaveQueries` を足すときは
  `save(Config)` の共通パスに乗せる（Task 3-2 で確認済み）

- **ダイアログの候補スクロール決定は保留:** Repos スライスからの積み残し 8 番（初回投入の
  候補に上限がない）が 3-2 で追加ダイアログ用に出た。保存クエリのポップアップも候補を持つため、
  2 つ一度に整えるのが筋が良い。次のスライスで決める
