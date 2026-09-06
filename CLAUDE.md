# CLAUDE.md

octoscope は GitHub のプルリクエストと Issue を見渡すためのターミナル UI。
Windows / macOS / Linux で動く単一バイナリ。

## 作業を始める前に

**`internal/` と `cmd/` を編集する前に、該当する実装計画を読む。**
無ければ、設計から実装計画を書き、承認を得てから編集に入る。

- 設計: `docs/superpowers/specs/`
- 実装計画: `docs/superpowers/plans/`

読まずに編集しようとすると `PreToolUse` フックが止める。止められたら、設計と計画を
読んでから続ける。フックを外して回避しない。

設計に書かれているパッケージ構成が、まだコードに存在しないことがある。
その場合は設計が誤りなのではなく、まだそこまで実装が進んでいない。
コードと設計が食い違っていたら、どちらが正しいかを判断する前に理由を確かめる。

## コマンド

```bash
make check        # CI と同じ検査を全部（tidy / lint / fmt / test）
make test         # race 検出とカバレッジつきでテスト
make lint         # golangci-lint
make fmt          # gofumpt + goimports で整形
make release-check # goreleaser の設定と 3 OS のクロスコンパイル
```

`make check` が通らない状態でコミットしない。

## 規約

規約は `.claude/rules/` にあり、**触るファイルに応じて自動で読み込まれる**（frontmatter の `paths`）。
自分で読みに行く必要はない。読み込まれたことは `/context` で確認できる。

規約に無理があると感じたら、黙って逸脱せず、規約の変更を提案する。
提案の仕方は `.claude/rules/architecture.md` の「規約そのものを変える」にある。

## 動かして確かめる

```bash
go run ./cmd/octoscope                      # カレントディレクトリのリポジトリ
go run ./cmd/octoscope --repo kukv/koto     # 任意のリポジトリ
go run ./cmd/octoscope --lang ja            # 日本語表示
```

TUI の変更は、テストが通っただけで完了とみなさない。実際に起動して見る。
日本語は全角で桁を 2 つ使うので、`--lang ja` でも確認する。
