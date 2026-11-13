# repo: 長いリポジトリ名がサイドバーの 1 行に収まることを確かめる（#174）

## 問題

Repos タブのサイドバーは幅 30 桁の固定列で（`repo/sidebar.go` の `sidebarWidth`）、名前は
`layout.Pad(r.name, sidebarWidth-badgeColumn)` で切り詰めている。ところが、golden とテストの fixture に入っているのは
短い名前（`kukv/octoscope`・`kukv/koto`）だけで、切り詰めを一度も通っていない。切り詰めが外れると、
長い名前の行がサイドバーからはみ出して、表の列を右に押し出す。

## 方針

**サイドバーのテストだけを足す。root の golden は足さない（利用者の判断）。**

Issue はもう 1 つ、root に `Options{}`（リポジトリ未確定）の通常画面の golden を足すことを求めている。
2026-11-13 に描いて比べたところ、`Options{}` の起動直後の画面は、既存の `app_tabs_work`（`--repo` 付きで起動して
1 で Work に移った画面）と en 80 / 120 でバイト単位まで同一だった。タブ行（`root/render.go` の `tabRow`）は
リポジトリに依存しないので、録っても同じファイルが 6 枚増えるだけになる。これは Issue にコメントで残す。

テストは `repo/repo_test.go` の内部テスト（`package repo`）に置く。`sidebar()` と定数を直接読むため。

- `TestALongRepositoryNameStaysOnOneSidebarLine`
  - 幅 120 で、`Repositories` に `kukv/octoscope` と 54 桁の `kukv/a-repository-with-a-name-nobody-would-type-twice`
    （`golden_test.go` の候補一覧にある名前）を入れ、件数（`repoCountsMsg`、長い方は PRs 1 / Issues 2）を渡す
  - `m.sidebar()` のどの行も `ansi.StringWidth` が `sidebarWidth` 以下であること
  - 長い名前の行に、名前の先頭（`kukv/a-repository`）とバッジ（`1/2`）が両方あること。
    名前がバッジを押し出していないことを見る
  - `m.View()` のどの行も 120 桁以下であること

## タスク

1. 上のテストを足す
2. **空振りの確認。** `sidebar()` の `layout.Pad(r.name, ...)` を一時的に `r.name` にして、テストが落ちることを見てから戻す
3. `make check` を通し、`test(repo): ...`（`Closes #174`）でコミットする。PR の説明と Issue のコメントに、
   root の golden を足さなかった理由を書く
