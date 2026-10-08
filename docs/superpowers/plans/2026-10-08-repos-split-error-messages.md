# Repos: 失敗のメッセージ型を種類ごとに分ける（#171）

## 問題

`errMsg` は一覧の取得・`o`（ブラウザ）・保存の 3 種類を 1 つの型で運び、`kind` で
分岐している（`repo/repo.go`）。`Update` の `gen` による破棄は `noticeSave` 以外に
掛かるので、`o` を押してからブラウザの失敗が返るまでにサイドバーの行が動くと、
`noticeOpen` の失敗が「古い行の取得の失敗」と同じ扱いで捨てられる。

`o` の失敗は行に属さない。利用者が押したキーの結果であり、行が動いても知らせるべきである。
errors.md / tui.md の「メッセージ型は用途（表示場所）ごとに分ける」にも反している。

## 方針

`errMsg` を 3 つの型に分ける。`noticeKind` は notice の前置きを選ぶ表示側の値として残す。

| 型 | フィールド | 破棄 | loading を下ろす |
|---|---|---|---|
| `fetchFailedMsg` | `gen, tab, err` | `gen` が古ければ捨てる | する |
| `openFailedMsg` | `tab, err` | 捨てない | しない |
| `saveFailedMsg` | `tab, err` | 捨てない（`saveDone` を通す） | しない |

- `openWeb` は `gen` を受け取らなくなる
- 3 つに共通する「致命的なら `FatalMsg`、そうでなければ notice」は 1 つの関数にまとめる
- 行を移ると `leaveRow` が notice を消す。その後に返った `o` の失敗は、
  移った先の行の notice として出る（押したキーの結果なので、どの行かは問わない）

範囲外: notice を行ではなく画面に持たせ直すこと。

## タスク

1. テスト（赤）: `o` → 失敗の Cmd を実行せずに保持 → サイドバーで行を移す →
   保持した Cmd のメッセージを `Update` に渡す → notice に `notice.open_failed` の文言が出る。
   今の実装では `gen` が古いので捨てられ、落ちることを確認する
2. 型を分けて緑にする。既存テスト・golden の `errMsg{...}` を新しい型に置き換える
   （`kind` 無し → `fetchFailedMsg`、`noticeOpen` → `openFailedMsg`）
3. `make check`。golden は不変であること
