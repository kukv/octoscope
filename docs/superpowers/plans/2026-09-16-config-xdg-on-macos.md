# 設定ファイルを macOS でも `~/.config` に置く 実装計画

**Goal:** Windows 以外（macOS と Linux）で、設定ファイルを
`$XDG_CONFIG_HOME/octoscope/config.yaml`、無ければ `~/.config/octoscope/config.yaml`
から読む。Windows は `%AppData%` のまま。

**Architecture:** `internal/app/config` の `Path()` だけで閉じる。読み書きの経路は
`cmd/octoscope/main.go:45` が `config.Path()` を 1 回呼ぶだけで、`datasource.Store`
はそこから受け取った文字列に書き戻す。パスの決め方を知っているのは `Path()` ひとつ。

**Tech Stack:** Go 1.25

**Spec:** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §5 /
`docs/superpowers/specs/2026-09-08-phase4-design.md` §3（どちらも `os.UserConfigDir()`
と書いてある。タスク 3 で追従させる）

**Branch:** `feat/config-xdg-on-macos`（`fix/config-path-test-on-every-os` の上に積む）

## なぜ

`os.UserConfigDir()` は macOS で `~/Library/Application Support` を返す。dotfiles
リポジトリからこの場所を扱うには、パスに空白が入り、OS ごとにリンク先を変える分岐が
要る。macOS と Linux を行き来する人にとって、設定ファイルが 1 箇所に無いのは実際の
手間である。

XDG の規約は Linux 固有のものではなく、macOS でもこれに従う CLI は多い。

## 決めたこと

**darwin を特別扱いしない。Windows 以外は同じ規則にする。** `runtime.GOOS` の分岐を
「Windows か、それ以外か」の 1 つに留める。macOS と Linux が同じ場所になるのは、
たまたまではなく構造としてそうなる。Linux の挙動は変わらない（`os.UserConfigDir()`
が Linux でやっていたことと同じ規則を自分で書くだけ）。

**Windows は `%AppData%` のまま。** dotfiles の都合は主に Unix のもので、Windows の
利用者の期待を裏切る理由がない。

**旧パスからの読み取りフォールバックは作らない。** 「新パスに無ければ旧パスを読む」
は、書き戻し先がどちらになるかの説明が要る分岐を永久に抱えることになる。移行は
1 回だけの `mv` で済み、それを README とリリースノートに書く。

**これは破壊的変更である。** macOS の既存利用者は、移行しなければ設定が既定値に
戻ったように見える（設定は消えないが読まれない）。次のタグは patch ではなく
`v0.7.0`。

## Global Constraints

- 各タスクの末尾で `make check` が緑
- 期待値は各 OS の規約をベタ書きする。`os.UserConfigDir()` を呼び直して比べない
  （`.claude/rules/testing.md`）
- コメントは外部の事情・正しい理由・doc の 3 つだけ（`.claude/rules/go-style.md`）

## ファイル構成

| ファイル | 変更 |
|---|---|
| `config/config.go` | `Path()` の決め方と doc コメント |
| `config/config_test.go` | darwin のケースを消し、`~/.config` へ落ちるテストを足す |
| `README.md` / `README.ja.md` | 設定ファイルの場所と、移行の 1 行 |
| 設計書 §5 / phase4 設計書 §3 | `os.UserConfigDir()` の記述 |

## Task 1: テストを先に書く

**Files:** `config/config_test.go`

**Steps:**

- [ ] `TestPathPutsTheFileUnderTheConfigDirectory` の `darwin` のケースを消す。
      macOS は default（`XDG_CONFIG_HOME`）側に入る
- [ ] `TestPathFallsBackToDotConfig` を足す。`XDG_CONFIG_HOME` を**空文字に設定**し
      （シェルが export していると拾ってしまう）、`HOME` を temp dir にして、
      `<dir>/.config/octoscope/config.yaml` を期待する。Windows には
      フォールバックが無いので `t.Skip` する
- [ ] `go test ./internal/app/config/` が赤いことを見る

**Verify:** 2 つとも落ちる。落ちる理由が「macOS が Library を返す」であること

## Task 2: `Path()`

**Files:** `config/config.go`

**Steps:**

- [ ] Windows なら `os.UserConfigDir()`、それ以外は `XDG_CONFIG_HOME` →
      `os.UserHomeDir()/.config` の順で決める
- [ ] doc コメントを 2 つの規則に書き直す。なぜ macOS も `~/.config` なのか
      （dotfiles が届く 1 箇所）を書く
- [ ] `go test ./internal/app/config/` が緑
- [ ] 実装を壊して（join のファイル名を変える）2 つとも落ちることを確かめ、戻す
- [ ] `GOOS=linux go vet` と `GOOS=windows go vet`
- [ ] `make check`

**Verify:** `make check` が緑。空振りの確認をしている

## Task 3: README と設計書

**Files:** `README.md`, `README.ja.md`, 設計書 §5, phase4 設計書 §3

**Steps:**

- [ ] 両 README の「設定ファイル」節を新しい場所に直し、macOS からの移行の
      `mv` を 1 行載せる
- [ ] 設計書 §5 と phase4 設計書 §3 の `os.UserConfigDir()` の記述を直す
- [ ] `make check`

**Verify:** `grep -rn "Application Support" README.md README.ja.md docs/superpowers/specs`
が、移行を説明する行以外に出てこない
