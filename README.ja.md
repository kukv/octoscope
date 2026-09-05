# octoscope

[English](README.md)

GitHub のプルリクエストと Issue を見渡すためのターミナルダッシュボード。

octoscope = **Octo**cat + **-scope**: 自分の GitHub の仕事を見渡す望遠鏡。

## 必要なもの

- [GitHub CLI](https://cli.github.com/)（`gh`）。`gh auth login` で認証済みであること

## インストール

### mise

[mise](https://mise.jdx.dev/) で GitHub Releases のバイナリをそのまま入れられる。

    mise use -g github:kukv/octoscope@latest

プロジェクトごとに固定するなら `mise.toml` に書く。

    [tools]
    "github:kukv/octoscope" = "latest"

### 手動でダウンロード

[リリースページ](https://github.com/kukv/octoscope/releases)からプラットフォーム向けの
アーカイブを取得し、展開して `octoscope` を `PATH` の通った場所に置く。

    tar xzf octoscope_<version>_linux_amd64.tar.gz
    install -m 0755 octoscope ~/.local/bin/

Windows では zip を展開し、`octoscope.exe` を `PATH` の通ったディレクトリに置く。

アーカイブと一緒に `checksums.txt` も公開している。

    sha256sum -c checksums.txt --ignore-missing

### go install

    go install github.com/kukv/octoscope/cmd/octoscope@latest

この方法では `--version` は `dev` と表示される。バージョン文字列はリリースビルド時に埋め込まれる。

### ソースからビルド

    git clone https://github.com/kukv/octoscope.git
    cd octoscope
    go build -o octoscope ./cmd/octoscope

## 使い方

git リポジトリの中で実行する。

    octoscope

または任意のリポジトリを指定する。

    octoscope --repo kukv/octoscope

### フラグ

| フラグ | 説明 |
|---|---|
| `--repo owner/name` | 対象リポジトリ。デフォルトはカレントディレクトリのリポジトリ |
| `--lang en\|ja` | 表示言語。デフォルトはオペレーティングシステムのロケール |
| `--icons unicode\|nerd\|ascii` | グリフの種類。デフォルトは `unicode`。`OCTOSCOPE_ICONS` で恒久的に指定できる |
| `--version` | バージョンを表示して終了する |

[Nerd Font](https://www.nerdfonts.com/) のパッチ済みフォントを入れているなら
`--icons nerd`、Unicode 記号が描けない環境なら `--icons ascii` を渡す。
使用中のフォントもその収録範囲も端末は報告しないため、パッチ済みフォントの
有無は検出できない。したがってデフォルトはフォントを要求しない側にしてある。

### キー

| キー | 一覧 | 詳細 |
|---|---|---|
| `j` / `k` | カーソル移動 | スクロール |
| `enter` | 詳細を開く | — |
| `tab` | PR / Issue を切り替え | — |
| `r` | 更新 | 更新 |
| `o` | ブラウザで開く | ブラウザで開く |
| `c` | — | コメント（`Ctrl+S` 送信 / `Esc` 中止） |
| `x` | — | クローズ / 再オープン（`y` 確定 / `n` 中止） |
| `l` | — | ラベルを編集（`space` 選択 / `enter` 適用） |
| `a` | — | 担当者を編集（`space` 選択 / `enter` 適用） |
| `esc` | — | 一覧に戻る |
| `q` | 終了 | 一覧に戻る |

## 多言語対応

octoscope は英語と日本語に対応している。表示言語はまず `--lang`、次にオペレーティング
システムのロケールから選ばれ、どちらもなければ英語にフォールバックする。
