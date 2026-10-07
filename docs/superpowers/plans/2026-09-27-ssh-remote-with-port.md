# api: ポートを明示した ssh:// の remote を読む（#151）

## 問題

`api/repo.go` の `parseRemote` は URL 形（`://` を含む）でスキームとユーザーを落としたあと
`CutPrefix(s, "github.com/")` で判定する。`ssh://git@github.com:22/owner/repo.git` は
`github.com:22/...` になって一致せず、api バックエンドではカレントリポジトリが解決できない。

## 方針

URL 形のときだけ、ホスト部（最初の `/` まで）が `github.com:<数字>` ならポートを落とす。
scp 形（`git@host:owner/repo`）の `:` はポートではないので触らない——`://` が無い経路は
今のまま。

`net/url.Parse` に置き換える案は採らない。スキームの無い `github.com/owner/repo` を
受け付けている今の挙動が変わるため（Issue の範囲外）。

## タスク

1. `TestParseRemoteReadsEveryShapeGitWritesTheURLIn` に
   `ssh://git@github.com:22/kukv/octoscope.git` と `.git` 無しを、
   `TestParseRemoteRefusesAnythingButGitHubCom` に `ssh://git@gitlab.com:22/kukv/octoscope.git` と
   `ssh://git@github.com:abc/kukv/octoscope.git`（数字でないポート）を足して赤を確認
2. `parseRemote` を直して緑に
3. `make check`
