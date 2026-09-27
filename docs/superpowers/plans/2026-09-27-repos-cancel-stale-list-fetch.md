# Repos: 行を移ったら前の行の一覧取得をやめる（#169）

## 問題

`selectRow` は 1 行動くたびに `fetchList` を返し、`fetchList` は `context.Background()` を
使う。古い答えは `gen` で捨てるが、取得そのものは最後まで走る。サイドバーを連打・
ホイールで流すと、通った行の数だけ取得が並行する。go-style.md の context の規約
（再取得・画面を離れるときは前の context をキャンセルする）にも反する。

## 方針

Work の `Refresh` / `Cancel` と同じ形にする。

- `Model` にタブごとの `cancel [2]context.CancelFunc` を持たせる（ctx そのものは持たない）
- `fetchList` は `ctx` を受け取る。キャンセルされた取得は `nil` を返す
  （Work の `fetchSection` と同じ。gh サブプロセスのキャンセルは `signal: killed` で
  返り、`context.Canceled` を包まないので、判定は `ctx.Err()` で行う）
- `selectRow`: 両タブの取得をキャンセルしてから、今のタブの取得を新しい ctx で始める
- `Refresh` / `showTab`: そのタブの前の取得をキャンセルしてから始める（`r` の連打も積まない）
- `Init` は値レシーバで `cancel` を持ち帰れないので今のまま（最初の 1 本だけ。
  以後の `selectRow` の対象にはならないが、並行は最大 2 本に収まる）

範囲外: `fetchCounts`（行数に比例しない 1 本）、終了時のキャンセル（Work だけが対象の既存挙動）。

## タスク

1. テスト: 返ってきた Cmd を**実行せずに溜め**、N 行動いたあとで順に実行する。
   fake の `ListItems` が呼ばれた時点の `ctx.Err()` を記録し、最後の 1 本以外が
   キャンセル済みであることを確かめる。`r` を N 回押した場合も同様。赤を確認
2. 実装して緑に。既存の gen による破棄のテストが通ること
3. `make check`。golden は不変
