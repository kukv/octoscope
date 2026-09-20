# Item 統合 PR 4（一覧を `ListItems` へ、repo ビュー移行）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `ListPRs` / `ListIssues` を `ListItems(ctx, repo, kind)` 1 本に差し替え、repo ビューを
`domain.Item` に移す。これでポート対 14 本が 6 本になり、`internal/app` から `domain.PR` /
`domain.Issue` を読む場所が gateway の変換関数だけになる。

**Architecture:** PR 3 と同じ形の差し替え。usecase のポート 2 本が 1 本になり、
`kind` は分岐ではなく**クエリの引数**である（設計書 §5）——Repos タブは PR の一覧と Issue の
一覧を別のペインに描くので、呼ぶ側は最初からどちらが欲しいか決まっている。
gateway の `ListItems` は PR 2 で実装済み。

**Tech Stack:** Go、golangci-lint（depguard / gofumpt / goimports）、gotestsum、`internal/golden`

**Spec:** `docs/superpowers/specs/2026-09-20-item-unification-design.md`
（§5 ポートの形、§4.3 `WorkItem` を分けたままにする理由、§8 PR 4）

---

## Global Constraints

- **golden 354 枚と `testdata/` 全体を 1 バイトも変えない。** repo ビューが動くので、
  ここが実効性のある検査である。`OCTOSCOPE_UPDATE_GOLDEN` を使いたくなったら、
  それは**画面を変えてしまった証拠**。そのステップを止めて原因を調べる
- **gateway の `ListPRs` / `ListIssues` を消さない。** 削除は PR 5。`internal/github` も触らない
- **detail ビューを触らない。** PR 3 で移行済み
- **`domain.WorkItem` を統合しない**（設計書 §4.3）。`selectedWorkItem` の変換は残り、
  読む先が `Change` になるだけ
- **モデルの持ち方は変えない**（利用者判断、案 A）。`m.prs` / `m.issues` の 2 スライスと
  `prListMsg` / `issueListMsg` はそのまま残し、**要素の型だけ** `domain.Item` にする。
  `items [2][]domain.Item` に畳む案 B は採らない
- 各タスクの最後に `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 着手前に調べた事実（2026-09-20）

計画の判断はここから来ている。実装者はこれを前提にしてよい。

### A. gateway の `ListItems` は既にある（PR 2）

`internal/app/adapter/gateway/gh/lists.go:39` に
`ListItems(ctx, repo string, kind domain.ItemKind) ([]domain.Item, error)` があり、
`listPRItems` / `listIssueItems` に振り分けている。**usecase は宣言を変えるだけ。**

### B. `fetchList` に渡る `repo` は空文字になり得る

`sidebar.go:52-63` の doc のとおり、`selectedRepo()` は設定ファイルの一覧と
カレントディレクトリのリポジトリが両方判明するまで空を返し、`fetchList` はそれを
そのままポートに渡す（空はクライアント自身のリポジトリを意味する）。

gateway の `toItemFromPR(n, repo)` は**その引数から `Ref.Repo` を埋める**ので、
返ってくる Item の `Ref.Repo` が空になる場合がある。一方ビューが ref を組み立てる時点の
`m.selectedRepo()` は実名を返し得る。

**規則: ref の組み立ては今までどおり `m.selectedRepo()` から行い、Item からはフィールドしか読まない。**
`item.Ref` をそのまま使うと `OpenChecksMsg` と drawer に流れる `Repo` が変わり得る。

### C. repo ビューが `domain.PR` / `domain.Issue` に触るのは 2 ファイル

| ファイル | 箇所 |
|---|---|
| `repo.go` | `prSource`（20）、`issueSource`（25）、`prListMsg.prs`（72）、`issueListMsg.issues`（76）、`Model.prs` / `Model.issues`（213-214）、`fetchList`（347, 353） |
| `render.go` | `selectedWorkItem`（93, 105）、`row`（274, 280） |

`repo.go` の残り（`m.prs[...].URL`、`m.prs[...].Number`、`len(m.prs)`）は
**`domain.Item` にも同じ名前である**ので、型が変わっても書き換えは要らない。

### D. `row()` と `selectedWorkItem()` のタブ分岐は種別の分岐ではない

どちらも `m.tab` で分かれ、アイコン（`icon.Review` か `icon.Issue`）と
どちらの `fetchedAt` を使うかを決めている。**これは「どちらのペインを描いているか」であって
`ItemKind` の分岐ではない。** 残す。変わるのは PR 固有のフィールドの読み先だけ。

### E. `selectedWorkItem` が写す PR 固有のフィールドは 7 つ

`IsDraft` `Review` `Head` `Base` `Additions` `Deletions` `Checks`。
**`domain.Change` の 7 フィールドと完全に一致する**ので、`pr.X` → `it.Change.X` で足りる。
`Body: pr.BodyText` は `domain.Item` にも `BodyText` があるのでそのまま。

### F. テストのフェイクは `prRepos` / `issueRepos` を別々に記録している

`repo_test.go:32-33`、`root_test.go`、`scenario_test.go` の 3 つが
「どのリポジトリで `ListPRs` が呼ばれたか」を順序つきで記録し、
`TestSidebarMoveAsksListPRsForTheNewRow` などがそれを検証している。

**フェイクの `ListItems` は `kind` で記録先を振り分ける。** これはフェイクの都合であって、
本体の分岐ではない。振り分けないとこれらのテストが何も守らなくなる。

### G. `repo_test.go` は 1140 行

計画の行数チェックはテストを対象外にしているが、**これ以上太らせない。**
このタスクで足すのは変換ヘルパーだけで、テストケースは増えない。

---

## ファイル構成（このタスク後）

```
internal/app/usecase/
  lists.go       lister（3 メソッド）+ itemLister（1 メソッド）+ 委譲
  lists_test.go  fakeViewer + fakeLists（ListItems の素通し）

internal/app/presentation/tui/repo/
  repo.go        itemLister 1 本、prs/issues は []domain.Item
  render.go      PR 固有の読み出しは Change 経由
```

---

### Task 1: usecase のポートを `ListItems` に差し替える

**Files:**
- `internal/app/usecase/lists.go`（変更）
- `internal/app/usecase/usecase.go`（変更）
- `internal/app/usecase/lists_test.go`（変更）
- `internal/app/presentation/tui/repo/repo.go`（変更）
- `internal/app/presentation/tui/repo/render.go`（変更）
- `internal/app/presentation/tui/repo/sidebar.go`（変更、コメント 1 行）
- `internal/app/presentation/tui/repo/repo_test.go`（変更）
- `internal/app/presentation/tui/root/root_test.go`（変更）
- `internal/app/presentation/tui/root/scenario_test.go`（変更）

PR 3 と同じく、型の境界が usecase とビューを跨ぐので**1 コミットで渡りきる。**
ビルドが通ることを確認するのは Step 7。

- [ ] **Step 1: `usecase/lists.go`**

`lister` から `ListPRs` / `ListIssues` を外し、1 メソッドのポートを同じファイルに足す:

```go
// itemLister is one repository's open items of one kind. The kind is a
// parameter of the query, not a branch: the Repos tab draws pull requests
// and issues in separate panes, so the caller knows which it wants.
type itemLister interface {
	ListItems(ctx context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error)
}
```

`Usecase.ListPRs` / `ListIssues` を消し、`ListItems` の 1 行委譲を置く。

- [ ] **Step 2: `usecase/usecase.go` を配線する**

`source` に `itemLister` を足し、`Usecase` に `itemLists itemLister` を足し、`New` で `src` を入れる
（PR 3 で `search` を足したときと同じ形）。`lists` はそのまま残る。

- [ ] **Step 3: `repo/repo.go` のポートとモデル**

`prSource` と `issueSource` を 1 本にまとめる:

```go
// itemLister is what the table needs: one repository's open items of the
// kind the visible tab draws.
type itemLister interface {
	ListItems(ctx context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error)
}
```

`Source` の埋め込みを差し替える。`prListMsg.prs` / `issueListMsg.issues` と
`Model.prs` / `Model.issues` を `[]domain.Item` にする（**フィールド名は変えない**）。

`fetchList` はポートが 1 本になるので次の形になる。**タブの分岐は残る**——
どちらのメッセージを返すかを決めているのはタブである:

```go
func fetchList(src Source, t tabID, repo string, gen int) tea.Cmd {
	return func() tea.Msg {
		kind := domain.ItemPR
		if t == tabIssues {
			kind = domain.ItemIssue
		}
		items, err := src.ListItems(context.Background(), repo, kind)
		if err != nil {
			return errMsg{gen: gen, tab: t, err: err}
		}
		if t == tabPRs {
			return prListMsg{gen: gen, prs: items}
		}
		return issueListMsg{gen: gen, issues: items}
	}
}
```

- [ ] **Step 4: `repo/render.go`**

`selectedWorkItem` の PR 側（105 行〜）で `pr.IsDraft` `pr.Review` `pr.Head` `pr.Base`
`pr.Additions` `pr.Deletions` `pr.Checks` を `Change` から読む。事実 B の規則どおり、
`Ref` は `m.selectedRepo()` から組み立てたままにする（**`it.Ref` を使わない**）。

`row()` の PR 側（274 行〜）で `pr.Review` `pr.IsDraft` `pr.Checks` を同様に。

**`Change == nil` に当たった場合をどう書くか。** PR のペインを描いている以上 `Change` は
非 nil である（gateway が保証する不変条件）。**ゼロ値で描くために `nil` を握りつぶさない**——
`it.Change` を素直に参照し、壊れていれば panic させる。ここで `if` を足すと、
不変条件が破れたことが画面上は「チェックが 0 件の PR」に見えて気づけなくなる。

- [ ] **Step 5: `repo/sidebar.go` のコメント**

55 行の「which fetchList passes straight to ListPRs/ListIssues」を `ListItems` に直す。
**コメントの他の部分は変えない。**

- [ ] **Step 6: テストのフェイクとフィクスチャ**

PR 3 と同じ規則——**フィクスチャは `domain.PR` / `domain.Issue` のまま、変換ヘルパーで `domain.Item` にする。**

3 つのフェイク（`repo_test.go` / `root_test.go` / `scenario_test.go`）の
`ListPRs` / `ListIssues` を 1 本にまとめる。事実 F のとおり、**記録先は `kind` で振り分ける**:

```go
func (f *fakeSource) ListItems(_ context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error) {
	if kind == domain.ItemPR {
		f.prRepos = append(f.prRepos, repo)
		return itemsFromPRs(f.prs), f.err
	}
	f.issueRepos = append(f.issueRepos, repo)
	return itemsFromIssues(f.issues), f.err
}
```

`root_test.go` には PR 3 で入れた `itemFromPR` があるので、それを使う。
`itemFromIssue` が要るなら足す。**`repo` パッケージにも同じ変換が要る**（パッケージが違うので
共有できない。PR 5 でフィクスチャごと `domain.Item` にすれば消える重複である）。

- [ ] **Step 7: ビルドが通ることを確認する**

```bash
go build ./... && go vet ./...
```

通らない箇所が上のファイル一覧の外に出ていたら、**止めて報告する。**

- [ ] **Step 8: テストと整形**

```bash
make fmt
make check
```

- [ ] **Step 9: golden が無傷であることを確認する（このタスクの本丸）**

```bash
git status --porcelain | grep -c testdata    # 0 であること
```

1 枚でも変わっていたら画面を変えている。`OCTOSCOPE_UPDATE_GOLDEN` を使わず、
差分を読んで原因を直す。疑う先は変換ヘルパーの写し漏れ（事実 E の 7 フィールド）と、
`Ref` を `m.selectedRepo()` ではなく `it.Ref` から取ってしまった箇所（事実 B）。

- [ ] **Step 10: ポートが 6 本になったことを確認する**

```bash
grep -rn "ListPRs\|ListIssues" internal/app/usecase internal/app/presentation   # 0 件
```

- [ ] **Step 11: コミット**

```bash
git add internal/app
git commit -m "refactor: move the repository lists onto ListItems" \
  -m "The Repos tab asks for one kind of item rather than one of two list
methods. The kind is a parameter of the query, not a branch: the tab draws
pull requests and issues in separate panes." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: PR 全体の確認と設計書の更新

**Files:**
- `docs/superpowers/specs/2026-09-20-item-unification-design.md`（変更）

- [ ] **Step 1: 完了条件を機械的に確かめる**

```bash
grep -rn "ListPRs\|ListIssues" internal/app/usecase internal/app/presentation   # 0 件
grep -rn "domain\.PR\b\|domain\.Issue\b" internal/app/presentation --include="*.go" | grep -v _test   # 0 件
git diff --stat origin/main -- internal/app/adapter internal/github             # 空
git diff --stat origin/main -- internal/app/presentation/tui/detail             # 空
```

3 つ目・4 つ目が空でなければ、PR 5 / PR 3 の範囲に踏み込んでいる。

- [ ] **Step 2: 全体検査**

```bash
make check
make release-check
```

- [ ] **Step 3: 実機確認を利用者に依頼する**

Claude のセッションからは pty を起動できない（PR 3 で確認済み）。
**利用者に次を依頼し、返事を待ってから PR を作る。**

```bash
go run ./cmd/octoscope --repo kukv/octoscope
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

Repos タブで PR と Issue の両ペイン、サイドバーでのリポジトリ切り替え、
行のチェックバーとラベル、`enter` で detail に入ること、`s` でチェック画面に入ること。

- [ ] **Step 4: 設計書 §8 を更新する**

「### PR 4: 一覧と repo ビュー」に **`**完了: 2026-09-20。**`** を足す。
§8 の他の節と §9 の完了条件は変えない。

- [ ] **Step 5: コミット**

```bash
git add docs/superpowers/specs/2026-09-20-item-unification-design.md
git commit -m "docs: mark PR 4 done" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## この PR の完了条件

- `internal/app/usecase` と `internal/app/presentation` に `ListPRs` / `ListIssues` が 0 件
- **ポート対 14 本が 6 本になった**（設計書 §9 の条件の 1 つがここで満たされる）
- `internal/app/presentation` の非テストコードに `domain.PR` / `domain.Issue` が 0 件
- repo ビューのモデルは `prs` / `issues` の 2 スライスのまま、要素が `domain.Item`
- ref の組み立てが `m.selectedRepo()` のままである（`it.Ref` を使っていない）
- **golden 354 枚と `testdata/` 全体が無変更**
- gateway / `internal/github` / detail ビューが無変更
- `make check` と `make release-check` が通る
- 利用者が実機で Repos タブを確認した

## 次の PR

- **PR 5**: `domain.PR` / `domain.Issue` の削除、gateway の旧ポート 14 本の削除、
  `.claude/rules/architecture.md` の 3 節更新、テストフィクスチャの `domain.Item` 化
- **独立 PR**: `internal/github` の書き込みメソッドに `ctx` を通し、`writes.go` の `_ = ctx` を消す
