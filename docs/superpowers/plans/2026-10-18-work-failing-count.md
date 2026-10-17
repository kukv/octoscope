# Work: タブ行の「CI 失敗」を PR ごとに 1 回だけ数える（#140）

## 問題

`Summary`（`internal/app/presentation/tui/work/work.go`）は全列の全 item を回して
`Checks.State == CheckFailure` を足す。同じ PR が複数の列に並ぶ（自分の PR で assignee も自分、など）と、
その PR を列の数だけ数える。

## 方針

数えた `domain.ItemRef` を集合に覚え、同じ Ref は 1 回だけ数える。`ItemRef` は比較可能な struct なので
そのまま map のキーにできる。

範囲外: `Attention`（review-requested 1 列の長さで、重複が起きない）。

## タスク

1. テスト（赤）: 同じ Ref の失敗 PR を 2 列に置き、別の失敗 PR を 1 件足すと `Failing == 2`
2. `Summary` を直して緑にする。root の `TestTheTabRowReportsOnTheBoard` は同じ Ref の PR を
   3 つ置いて「3 件」を期待していた（このバグに乗っていた）。狙いはタブ行の描画なので、
   Number を別々にして期待値は変えない
3. `make check`（golden は変わらない見込み）
