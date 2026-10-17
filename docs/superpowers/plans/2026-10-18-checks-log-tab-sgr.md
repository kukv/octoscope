# checks: ジョブログのタブを空白で描き、色の指定を取り除く（#204）

## 問題

checks のログで `go test` の出力が崩れる。

```
15:56:26  ^[[36;1mgo test ./...^[[0m
15:56:30  FAILgithub.com/kukv/demo0.003s
```

1. **タブが消える。** 元は `FAIL\tgithub.com/kukv/demo\t0.003s`。`sanitizeControls`
   （`internal/github/api/joblog.go`）はタブを残しているが、描画で幅 0 として扱われて語がくっつく
2. **SGR（`ESC [ ... m`）が `^[[36;1m` として文字のまま出る。** エスケープを端末に解釈させないための
   caret 表記で、設計どおりではある。ただ Actions の runner はコマンドの行に毎回 SGR を付けるので、
   読む人にはノイズでしかない

## 方針

1. **タブ:** `buildLogLines`（`internal/app/presentation/tui/checks/render.go`）で、タブを空白に置き換える。
   diff ビューの `expandTabs`（`diff/render.go`、固定幅の置換）と同じ形にする。パッケージをまたいで
   共有するほどのものではないので、checks に同じ 1 行を持つ
2. **SGR:** SGR だけを取り除き、ほかのエスケープ（カーソル移動など）は今の caret 表記のまま残す。
   色として描くことはしない（外から来た文字列を端末に解釈させることになるため）
   - api バックエンド: `sanitizeControls` で、caret に変える前に `ESC [ <数字と ;> m` を取り除く
   - gh バックエンド: gh がログをどの形で渡すか（生の ESC か、gh 自身の caret 表記か）を、実際の出力を
     録って確かめ、その形で取り除く。どちらの形でも、取り除くのは SGR の形に合うものだけにする

## タスク

1. テスト用リポジトリに、SGR とタブを出して失敗するワークフローを置く（`go test` の失敗か、
   `printf` で同じ形を出すもの）。gh の `run view --log` の出力と、api のログ zip を録る
2. テスト（赤）: タブを含む行が、語と語のあいだを空白にして描かれる
3. テスト（赤）: `ESC[36;1m` と `ESC[0m` が取り除かれる。SGR でないエスケープ（例 `ESC[2K`）は caret 表記の
   まま残る。gh バックエンドでも、録った出力で同じことを確かめる
4. 実装して緑にする。SGR を取り除く処理を一時的に外して、3 が落ちることを確かめる
5. `make golden` / `make check`
6. 実際に起動して、1 のワークフローのログを見る。gh / `--api` の両方で
