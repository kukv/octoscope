# Repos: 件数の取得が丸ごと失敗した理由を通知の行に出す（#170）

## 問題

`internal/app/presentation/tui/repo/repo.go` の `fetchCounts` は、`RepoCounts` が失敗するとエラーを捨てて
`repoCountsMsg(nil)` を返す。認証切れやレート制限でも、全行のバッジが「—」（まだ取っていない行・
1 行だけ引けなかった行と同じ）になるだけで、理由が画面のどこにも出ない
（`docs/superpowers/2026-09-09-phase4-repos-sidebar-followups.md` #6、
`2026-09-11-phase4-repos-dialog-followups.md` #6）。

## 方針（利用者が決めたこと: 件数の失敗は別に持ち、通知の行に出す）

- 件数は全行をまとめて 1 回で取るので、どのタブにも行にも属さない。タブごとの `notice [2]notice` には入れず、
  `Model` に `counts notice` を足す。`noticeKind` に `noticeCounts` を足し、`prefixID` は
  新しいカタログキー `notice.counts_failed`（en: `Could not fetch counts` / ja: `件数を取得できませんでした`）。
  一覧は見えているのに「取得できませんでした」とだけ出ると、何が取れなかったのか分からないため
- `fetchCounts` は失敗したら `countsFailedMsg{gen, err}` を返す（`.claude/rules/tui.md`「メッセージ型は用途ごとに分ける」）。
  `Update` では:
  - `domain.IsFatal(err)` なら一覧と同じく `FatalMsg` を返す（全画面）
  - それ以外は `m.counts = notice{kind: noticeCounts, text: noticeText(err)}`。バッジは今までどおり「—」
- 件数が届いたら（`repoCountsMsg`）`m.counts` を消す。`leaveRow`（行を移る）では消さない
- 通知の行に出すのは、今のタブの `notice[m.tab]` があればそれ、無ければ `m.counts`。`noticeLine` と
  表の高さの勘定（`render.go` の `if m.notice[m.tab].text != "" { rows-- }`）が同じものを見るよう、
  出す通知を選ぶ関数を 1 つにして両方から使う。空の一覧の代わりに出す「取得できませんでした」
  （`body()`）はタブの一覧の失敗だけを見るまま変えない
- **古い答えを捨てる。** `Refresh`（`r`）・`SetCurrent`・行の追加と削除（`add.go`）が件数の取得を重ねて出すので、
  `Model` に `countsGen int` を持たせ、件数の取得を出すたびに増やし、`repoCountsMsg` と `countsFailedMsg` に載せる。
  番号が合わない答えは成功も失敗も捨てる（前の失敗が新しい成功を上書きしない・前の成功が新しい失敗を消さない）。
  `repoCountsMsg` は `[]domain.RepoCount` の別名型から `struct{ gen int; counts []domain.RepoCount }` に変える。
  `Init` は値レシーバで番号を増やせないので 0 のまま出す（最初の取得なので重なる相手がいない）
- 1 行だけ取れなかった（`RepoCount.Unavailable`）ときは今のまま「—」。範囲外

却下: 今のタブの通知に入れる。起動直後は一覧と件数を同時に取るので、一覧が後に返ると `answeredFetch` で消え、
行を移っても消える。出たり出なかったりする。

却下: 失敗した行のバッジを「—」以外の印にする。通知の行で理由が出れば足り、印の意味を覚えさせることになる。

## タスク

各タスクは、まず失敗するテストを書き、実装して緑にし、検証している行を一時的に壊してテストが落ちることを
確かめてから戻す（`.claude/rules/testing.md`）。

1. **件数の失敗を通知に出す。** `repo_test.go` に:
   - 件数の取得が（致命的でない）失敗をすると、`View()` に `Could not fetch counts` と原文が出る
   - 件数の答えがまだ来ていない間は出ない（失敗と未取得の区別。受け入れ条件）
   - 致命的な失敗（`domain.ErrUnauthenticated` を包む）は `FatalMsg`
   - 今のタブの一覧の失敗があればそちらが出て、件数の失敗は出ない。一覧の失敗が消えると件数の失敗が出る
   - 行を移っても消えない。次に件数が届くと消える
   - 古い番号の失敗・成功は捨てる（`r` を 2 回 → 2 回目の成功の後に 1 回目の失敗が届いても通知が出ない、
     2 回目の失敗の後に 1 回目の成功が届いても通知が消えない）
   - 件数の失敗の通知があるとき、表の行数が 1 減ってキーバーが画面内に残る
   - カタログ: en/ja に `notice.counts_failed`
   - 既存テストの `repoCountsMsg(...)` を新しい形に直す
2. **golden。** 件数の失敗の通知が出ている状態を en/ja × 80/120/160 で録る（`make golden`、diff を目で見る）
3. **ドキュメント。** 2 つの followups の #6 に、#170 で直したことを 1 行追記する
4. `make check`（lint の出力が `0 issues.` であることも見る）
5. 実際に起動して確かめる。shim（PATH の先頭に置いた `gh`）を、件数の GraphQL のときだけ止まるようにして
   Repos タブを開く → 16 秒後に通知の行に件数の失敗が出て、一覧は見える → 行を移っても残る →
   shim を戻して `r` で消える。`--lang ja` の 80 桁でも見る
