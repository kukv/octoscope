---
paths:
  - "internal/tui/**"
  - "internal/i18n/**"
---

# TUI

## ライブラリ

Bubble Tea 系の import パスは **`charm.land/*/v2`**。

```go
tea "charm.land/bubbletea/v2"
"charm.land/bubbles/v2/viewport"
"charm.land/lipgloss/v2"
```

`github.com/charmbracelet/bubbletea/v2` は壊れている。Renovate が
そちらへの更新を提案してきても取り込まない。

`github.com/charmbracelet/x/*`（`ansi` など）は `github.com/` のままで正しい。

## Model / Update / View

- **`View()` は副作用を持たない。** I/O も状態変更もしない。同じ状態からは常に同じ文字列
- I/O は `tea.Cmd` の中で行い、結果をメッセージ型にして `Update` に返す
- メッセージ型は用途ごとに分ける。1 つの型を使い回すと `Update` の分岐が読めなくなる

## サブモデル

ビューごとにサブモデルを持ち、自分の状態だけを持つ。
親モデルとはメッセージ型でだけ通信し、親のフィールドを直接触らない。

サブモデルが必要とするデータ取得の interface は、**そのサブモデルのファイルで宣言する**
（`.claude/rules/architecture.md`）。

## 表示幅

**日本語は 1 文字が 2 桁を占める。** 桁数を数えるときは必ず
`github.com/charmbracelet/x/ansi` を使う。

```go
w := ansi.StringWidth(s)       // 正しい
w := len(s)                    // バイト数。間違い
w := utf8.RuneCountInString(s) // 文字数。全角で 2 倍ずれる
```

切り詰めも桁数ベースで行う。桁を揃える表、幅の狭いカード、
キーバインドを並べるフッターで特にずれやすい。

## 文字列

画面に出す文字列は `internal/i18n` のカタログから引く。
Go のコードに英語も日本語も直書きしない。

```go
i18n.T("list.no_open_prs")
i18n.Tf("compose.title", map[string]any{"Title": title})
i18n.Tn("time.hours_ago", 3)   // 複数形。テンプレート変数は .Count
```

**翻訳しないもの:**

- キー入力の判定に使う文字列（`"j"`、`"ctrl+c"`、`"esc"`）
- GitHub から来た内容（タイトル、本文、ラベル名、ユーザー名、ジョブ名）
- GitHub の検索構文（`is:open`、`label:`）
- `--help` の出力。`--lang` を解析し終えるまで言語が決まらないため英語固定

文字列を足すときは **`en` と `ja` の両方のカタログに足す。**
片方だけだとカタログ整合のテストが落ちる。

語順が言語で変わる文は、単語を連結せず文ごとメッセージ ID にする。
「動詞 + 名詞」を組み立てる実装にしない。

## 色

**色は `internal/tui/theme` にだけ書く。** ビューは `theme.Dim()` のような
役割の名前で引き、16 進の色をビューのファイルに書かない。
状態（approved / changes requested / check failure など）に色を足すときも
theme に足す。同じ状態が画面ごとに違う色になるのを防ぐためである。

ターミナルの背景色は前提にできない。v2 に `lipgloss.AdaptiveColor` は無く、
代わりに `lipgloss.LightDark(isDark)` を使う。`isDark` は起動時に
`tea.RequestBackgroundColor` で問い合わせ、`tea.BackgroundColorMsg` が
返ってきた時点でルートモデルが `theme.SetDark` に渡す。
返事が来るまでは暗い背景を仮定する。

**シンタックスハイライトも theme が返す。** chroma のスタイルは配色そのものなので、
ビューが chroma を呼ぶとこの規約が崩れる。ビューは `theme.Highlight(path, code)` を
呼び、chroma を import しない。スタイル名（明背景 / 暗背景）は theme が持つ。

## 確認

TUI の変更は、テストが通っただけで完了にしない。必ず起動して見る。

```bash
go run ./cmd/octoscope
go run ./cmd/octoscope --lang ja   # 桁がずれていないか
```

狭い端末（80 桁）でも崩れないことを確認する。
