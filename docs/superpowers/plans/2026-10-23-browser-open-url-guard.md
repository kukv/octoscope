# browser.Open: http(s) 以外を開かず、WSL で URL をスクリプトとして読ませない

## 問題

セキュリティ監査 run-1 の要検証リード `octoscope/internal/browser/Open/url-unvalidated-to-opener`。

- `internal/browser/browser.go` の `Open` は受け取った文字列を検証せずに opener の argv の末尾に足す
- WSL の opener は `powershell.exe -NoProfile -Command Start-Process <url>`。`-Command` は残りの引数を
  スクリプトとして読み直すので、URL の `;` の後ろは別の文として走り、`$(...)` は評価される
  （監査時にこのホストで `Write-Output` に置き換えて確認済み）
- `browser.go` のコメントは「GitHub item URLs carry no query string」と、item の URL しか来ない前提で書かれている。
  しかし checks ビューの `o` は、チェックの作成者が書いた `detailsUrl` / `targetUrl` をそのまま渡す
  （`tui/checks/checks.go:299,307`）。閲覧しているリポジトリにステータスを書ける人が、URL の中身を決められる
- 他の OS でも、`-` で始まる文字列や `file:` などの scheme が opener にそのまま渡る

## 方針

- **開く前に URL を検証する。** `Open` の先頭で `net/url.Parse` し、scheme が `http` か `https`（大文字小文字を問わない）で、
  host が空でないものだけを受け付ける。それ以外は opener を探さずにエラーを返す。
  エラーは `fmt.Errorf("refuse to open %q: not an http or https URL", url)`。`%q` なので、URL に制御文字があっても
  画面にはエスケープされて出る。呼び出し側で分岐しないのでセンチネルにはしない（`.claude/rules/errors.md`）。
  呼び出し側は今の `default: err.Error()` / `noticeText` のまま原文を出す
- **WSL では URL をスクリプトとして読ませない。** `-Command Start-Process <url>` をやめ、
  `powershell.exe -NoProfile -NonInteractive -EncodedCommand <base64>` にする。中身は `Start-Process '<url>'`
  を UTF-16LE にして base64 にしたもの。URL は単一引用符の文字列リテラルに入れ、その中で特別な意味を持つ
  単一引用符（`'` と、PowerShell が同じに扱う U+2018・U+2019・U+201A・U+201B）は二重にする。
  base64 は英数字と `+/=` だけなので、WSL から Windows へのコマンドラインの受け渡しでも崩れない
- この形にすると URL が opener の引数の途中に入るので、`opener.command` を
  `command(env environment, url string) []string`（URL を含む argv 全体を返す）に変える。他の opener は
  今までどおり URL を最後の引数に足すだけ
- `wslBridge` のコメントの「`&` で切れる」「item の URL しかない」の前提は消え、`&` を含む URL も開けるようになる。
  コメントを今の挙動に合わせて書き直す

却下: WSL で `-Command` のまま単一引用符で囲む。WSL の interop が `"` やスペースを Windows のコマンドラインに
どう引用するかと、PowerShell 5 のコマンドライン解析が絡み、どの文字が残るかを確かめきれない。

却下: checks ビューの側だけで検証する。opener の手前で 1 か所で守れば、今後 `Open` を呼ぶ場所が増えても漏れない。

範囲外: 監査の他の 2 件（api バックエンドの制御文字、承認・merge の head 固定）。別に扱う。

## タスク

各タスクは、まず失敗するテストを書き、実装して緑にし、検証している行を一時的に壊してテストが落ちることを
確かめてから戻す（`.claude/rules/testing.md`）。

1. **URL の検証。** `browser_test.go` に、`Open` が opener を起動せずにエラーを返すケースを足す:
   `file:///etc/passwd`、`javascript:alert(1)`、`-foo`、`https:///nohost`、`powershell:x`、空文字。
   `https://github.com/o/r/pull/1` と `HTTP://example.com/x?a=1&b=2` は検証を通る。
   検証は `Open` から切り出した `validate(url string) error` で試す（opener を実際に起動しないため）
2. **WSL の argv。** `TestCommandPerEnvironment` を新しい `command(env, url)` に合わせる。WSL のケースは
   `-EncodedCommand` の引数を base64 → UTF-16LE で戻し、`Start-Process '<url>'` と一致することを確かめる。
   `https://example.invalid/x;Write-Output(1)`、`https://example.invalid/$(6*7)`、`https://example.invalid/a'b`、
   `https://example.invalid/a’b`（U+2019）で、URL がリテラルの外に出ないこと（引用符が二重になること）を見る
3. **`TestNoCommandGoesThroughAShell`** を新しいシグネチャに合わせ、WSL の argv に `-Command` が無いことも見る
4. `make check`（lint の出力が `0 issues.` であることも見る）
5. **このホストで確かめる。** `Start-Process` を `Write-Output` に差し替えた同じ形の `-EncodedCommand` を
   `powershell.exe` に渡し、上の悪意ある URL がそのまま 1 つの文字列として出力され、2 つ目の文や `42` が出ないことを見る。
   そのあと `go run ./cmd/octoscope` で詳細ビューから `o` を押し、本物の GitHub の URL がブラウザで開くことを見る
