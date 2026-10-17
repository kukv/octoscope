# diff: コメント 0 件の pending review もヘッダに出す（#144）

## 問題

diff のヘッダ（`internal/app/presentation/tui/diff/render.go` の `sizeLine`）は、`PendingCount() > 0` のときだけ
`pending · N` を出す。GraphQL は `reviews(states: [PENDING], first: 1)` で pending review を取っており、
コメントが 0 件でも `ReviewContext.Pending` の handle は埋まっている。それなのにヘッダには何も出ず、
書きかけがあることが見えない。設計 4.4.2 の「書きかけが存在することを隠さない」に反する。

## 方針

条件を `n > 0 || m.review.Pending != ""` にする。0 件なら `pending · 0`（ja は `未提出 · 0`）と出る。
既存のメッセージをそのまま使い、カタログは変えない。

却下: 件数を付けない `pending` だけの表示。カタログのキーが 1 つ増え、件数があるときと形が変わる。

## タスク

1. テスト（赤）: `Pending` に handle があってスレッドが 0 件なら、ヘッダに `diff.pending_note`（0 件）が出る。
   pending review が無ければ出ない
2. `sizeLine` を直して緑にする
3. `make golden`。submit / discard の golden は pending の handle があってコメントが 0 件の状態そのものなので、
   ヘッダに `pending · 0` が加わる（80 桁でも 65 桁で収まる）
4. `make check`
