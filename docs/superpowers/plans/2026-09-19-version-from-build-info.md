# go install で入れたバイナリがバージョンを名乗る 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `go install github.com/kukv/octoscope/cmd/octoscope@v0.8.0` で入れたバイナリが `--version` に `dev` ではなくそのバージョンを表示する。

**Architecture:** リリース物はすでに正しい。goreleaser が `-X main.version={{.Version}}` で
`main.version` を上書きするので、GitHub Releases の配布バイナリはタグの値を表示する。
足りていないのは **goreleaser を通らない経路**で、`go install module@version` も
手元の `go build` もそこに入る。

Go はモジュールのバージョンをバイナリに埋めているので、`runtime/debug.ReadBuildInfo()` から
読み戻せる。ldflags で入っていればそれを使い、無ければ埋め込みを見て、それも使えなければ
`dev` に落ちる、という 3 段にする。

`ReadBuildInfo` の呼び出しと「どれを採るか」の判断は分ける。判断側は文字列 2 つを受け取る
純粋な関数にして、テストから直接呼べるようにする。

**Tech Stack:** Go / `runtime/debug`

**設計:** 設計文書は無い。挙動の変更が `--version` の 1 行だけで、画面仕様・データの流れ・
パッケージ境界のどれも動かないため。判断の根拠はこの計画に書く。

---

## 3 つの経路で何が表示されるか

| 経路 | ldflags | `ReadBuildInfo().Main.Version` | 今 | この計画のあと |
|---|---|---|---|---|
| GitHub Releases（goreleaser） | `v0.8.0` | — | `v0.8.0` | `v0.8.0`（変わらない） |
| `go install …@v0.8.0` | 無し | `v0.8.0` | **`dev`** | **`v0.8.0`** |
| 手元の `go build` / `go run` | 無し | `(devel)` | `dev` | `dev`（変わらない） |

`(devel)` は「バージョンの無いソースから建てた」という Go の印なので、そのまま出さずに
`dev` に落とす。利用者に見せる文字列としては `dev` のほうが意味が通る。

## ファイル構成

| ファイル | 役割 | 変更 |
|---|---|---|
| `cmd/octoscope/main.go` | 起動と `--version` | `version` の初期値を空にし、`resolveVersion` を追加、`--version` がそれを通る |
| `cmd/octoscope/main_test.go` | main のテスト | 新規または追記。`resolveVersion` の表駆動テスト |

`.goreleaser.yaml`、`.github/workflows/release.yaml` は**変更しない**。ldflags が
`main.version` を上書きする前提はそのまま。

---

### Task 1: どれを採るかを決める関数を足す

**Files:**
- Modify: `cmd/octoscope/main.go`
- Create or modify: `cmd/octoscope/main_test.go`

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestResolveVersion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		injected string
		module   string
		want     string
	}{
		{"goreleaser injected it", "v0.8.0", "", "v0.8.0"},
		{"go install left it in the binary", "", "v0.8.0", "v0.8.0"},
		{"injected wins over the module's own", "v0.8.0", "v0.7.0", "v0.8.0"},
		{"built from source without a version", "", "(devel)", "dev"},
		{"nothing to go on", "", "", "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveVersion(tc.injected, tc.module); got != tc.want {
				t.Errorf("resolveVersion(%q, %q) = %q, want %q", tc.injected, tc.module, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: 走らせて失敗を確かめる**

```bash
go test ./cmd/octoscope/ -run TestResolveVersion -v
```

期待: `resolveVersion` が未定義でコンパイルできず失敗する。

- [ ] **Step 3: 実装する**

`main.go` の `version` の宣言を差し替える。

```go
// version is what --version prints. GoReleaser sets it with -ldflags at
// release build time; a build that did not go through GoReleaser leaves it
// empty and the module's own version is read back out of the binary
// instead (see resolveVersion).
var version string

// resolveVersion picks what to call this build. injected is what -ldflags
// put in, empty when nothing did; module is what the Go toolchain recorded
// as the main module's version, which is a real version for a binary from
// `go install module@version` and "(devel)" for one built from a source
// tree. Neither being any use leaves "dev", which says what it is.
func resolveVersion(injected, module string) string {
	if injected != "" {
		return injected
	}
	if module != "" && module != "(devel)" {
		return module
	}
	return "dev"
}
```

`--version` の分岐を、埋め込みを読んで渡す形にする。

```go
	if *showVersion {
		module := ""
		if info, ok := debug.ReadBuildInfo(); ok {
			module = info.Main.Version
		}
		fmt.Println("octoscope " + resolveVersion(version, module))
		return
	}
```

import に `"runtime/debug"` を足す。

- [ ] **Step 4: テストを走らせる**

```bash
go test ./cmd/octoscope/ -run TestResolveVersion -v
```

期待: 全ケース PASS。

- [ ] **Step 5: 3 つの経路を実際に確かめる**

```bash
go run ./cmd/octoscope --version
go build -ldflags "-X main.version=v0.8.0" -o /tmp/octoscope-ldflags ./cmd/octoscope && /tmp/octoscope-ldflags --version
```

期待: 前者が `octoscope dev`、後者が `octoscope v0.8.0`。

`go install …@version` の経路はタグを切るまで試せない（モジュールプロキシがそのバージョンを
知らない）。**タグを切ったあとに確かめる**ので、Task 2 に置いてある。

- [ ] **Step 6: `make check` を通してコミット**

```bash
make check
git add cmd/octoscope/main.go cmd/octoscope/main_test.go
git commit -m "fix(version): let a go install build name itself"
```

---

### Task 2: タグを切り、3 つ目の経路を確かめる

- [ ] **Step 1: Task 1 を main に入れる**

PR を作って CI を通し、main にマージする。**マージされるまでタグを切らない** —
タグは main の commit を指す必要がある。

- [ ] **Step 2: タグを切って push する**

```bash
git tag -a v0.8.0 -m "v0.8.0"
git push origin v0.8.0
```

`.github/workflows/release.yaml` が `v*` の push で走り、goreleaser が 3 OS × 2 arch の
アーカイブと checksums を作って GitHub Releases に上げる。

- [ ] **Step 3: リリースのワークフローが緑になるのを見る**

```bash
gh run watch
gh release view v0.8.0
```

- [ ] **Step 4: 配布バイナリがバージョンを名乗ることを確かめる**

自分の OS のアーカイブを落として `--version` を見る。期待: `octoscope v0.8.0`。

- [ ] **Step 5: `go install` の経路を確かめる**

```bash
GOBIN=/tmp/octoscope-install go install github.com/kukv/octoscope/cmd/octoscope@v0.8.0
/tmp/octoscope-install/octoscope --version
```

期待: `octoscope v0.8.0`。**これがこの計画の本題で、ここまで来て初めて確かめられる。**
`dev` と出たら `resolveVersion` ではなくモジュールプロキシの反映待ちを疑い、
数分おいてやり直す。
