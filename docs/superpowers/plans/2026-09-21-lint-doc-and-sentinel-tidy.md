# 拾ってあった宿題 3 件を片付ける実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** #126 / #127 のレビューで拾って未着手だった 2 件に、F-06 の取りこぼし 1 件を足して
1 本の PR で片付ける。

1. depguard の `domain-layer` に `internal/app/adapter` の deny を足す
2. `root.go` の `openDiff` の doc が Search を落としている
3. `domain.ErrTransient` の文言がドメインのセンチネルでサービスを名指ししている

**Architecture:** 3 件とも独立していて、互いのファイルが重ならない。振る舞いは変わらない——
1 は lint 設定、2 はコメント、3 はどこにも表示されていない文字列（事実 C で確認済み）。

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** 設計書は書かない。1 と 2 は #126 / #127 のレビュー指摘、3 は違和感 F-06
（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）の取りこぼし。利用者の承認済み。

---

## 着手前に調べた事実（2026-09-21）

### A. depguard の `domain-layer` は adapter を deny していない

`.golangci.yml` の `domain-layer` は `usecase` / `presentation` / `config` /
`internal/github` / `i18n` / `browser` / `encoding/json` を deny するが、
`internal/app/adapter` が無い。domain → gateway を実際に止めているのは
**Go の import cycle であって lint ではない**。

### B. 発火の実証はできる。ただし #127 と同じ形ではできない

2 通り試した（プローブは実験後に削除済み。ブランチには残っていない）。

| 試したこと | 結果 |
|---|---|
| `domain` から `adapter/gateway/gh` を import | `import cycle not allowed in test (typecheck)`。**depguard は動かない**——型検査で止まる |
| domain を import しない捨てパッケージ `adapter/zzprobe` を作り、`domain` から import | deny 追加**前**は違反なし。**追加後に depguard が発火した**（`ドメインは腐敗防止層を知らない`） |

つまりこの deny が効くのは「**domain を import しない adapter のサブパッケージが将来できたとき**」で、
今日ある 2 つ（`datasource` / `gateway/gh`）はどちらも domain を import しているので
import cycle が先に捕まえる。**PR 本文にはこの区別をそのまま書く。**
「#127 と同じように発火を観測した」とは書かない——観測したのは捨てパッケージ相手である。

**途中で分かった depguard の性質:** **depguard は blank import（`import _ "..."`）を見ない。**
最初のプローブが blank import だったため「`i18n` の既存 deny すら発火しない」という
誤った観測をした。実際に使う import に変えたら発火した。実証タスクではこれを踏まない。

### C. `domain.ErrTransient` は「GitHub did not answer」と言い、どこにも表示されない

`domain/errors.go:15`。F-06（#123）は `ErrBackendUnavailable` と `ErrUnauthenticated` を
直したが、これは対象外にしていた（「プレゼン層は対処方法を言ってよい」とは別の話で、
単に**対処方法ではなくサービス名**なので禁止語リストに掛からなかった）。

表示されないことの根拠は 2 つ:

- `classified.Error()` は `msg` だけを返す。`ErrTransient` は必ず
  `domain.Classify(domain.ErrTransient, err.Error())` として包まれる（`gateway/gh/errors.go:26`）
- `work_test.go:303` が `!strings.Contains(view, domain.ErrTransient.Error())` を**主張している**——
  この英文が画面に出ないことは既にテストで守られている

**`internal/github.ErrTransient`（同じ文言）は対象外。** インフラ層はサービスの言葉でよい
（`.claude/rules/errors.md`、#123 で明文化）。

### D. `openChecks` の doc は正しい。直すのは `openDiff` だけ

記憶には「`openDiff` / `openChecks` の doc が Search を落としている」と書いてあったが、
調べると **Search は `OpenChecksMsg` を送っていない**。

| 送り手 | OpenDetail | OpenDiff | OpenChecks |
|---|---|---|---|
| `work` | ○ | ○ | ○ |
| `repo` | ○ | ○ | ○ |
| `search` | ○ | ○ | **×** |

`openChecks`（`root.go:519-520`）の「the Work board and a Repos row」は**事実として正しい**。
`openDiff`（`root.go:497-498`）だけが Search を落としている。**openChecks は触らない。**

### E. 対象外

- `internal/github.ErrTransient` の文言（事実 C）
- `domain-layer` 以外の depguard ルール
- `openChecks` の doc（事実 D）
- F-05（別の PR。`2026-09-21-two-identifier-systems.md`）

## Global Constraints

- **golden 354 枚（`internal/app/presentation/**/testdata/*.golden`）を 1 バイトも変えない。**
  `OCTOSCOPE_UPDATE_GOLDEN` を使わない
- **振る舞いを変えない。** 3 件とも lint 設定・コメント・表示されない文字列である
- 各タスクの終わりに `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

---

### Task 1: depguard の `domain-layer` に adapter を足す

**Files:**
- Modify: `.golangci.yml`（`domain-layer` の `deny` に 1 項目）
- Modify: `.claude/rules/architecture.md`（lint が守る範囲の記述があれば合わせる。無ければ触らない）

**Step 1: 発火を先に観測する**

- [ ] 捨てパッケージ `internal/app/adapter/zzprobe`（domain を import しない）を作る
- [ ] `internal/app/domain/zz_probe.go` から**実際に使う形で** import する
      （blank import では発火しない——事実 B）
- [ ] `golangci-lint run ./internal/app/domain/...` が違反を**報告しない**ことを確認する
      （deny がまだ無いので通る。これが対照）

**Step 2: deny を足す**

- [ ] `domain-layer` の `deny` に追加する。`internal/github` の項目の**直前**に置く
      （上の層 → 隣の層 → 外の世界、という既存の並び順に合わせる）

```yaml
            - pkg: github.com/kukv/octoscope/internal/app/adapter
              desc: ドメインは腐敗防止層を知らない
```

- [ ] 同じコマンドで depguard が発火することを確認する
- [ ] 捨てパッケージとプローブを**削除する**

**Step 3: import cycle 側も確認して記録する**

- [ ] `domain` から `adapter/gateway/gh` を import して
      `import cycle not allowed in test (typecheck)` になることを観察する
- [ ] プローブを削除する
- [ ] **Verification:** `make check`

**Commit:** `build: deny the adapter from the domain in depguard`

コミット本文に事実 B の区別を書く——今日ある adapter 2 つは import cycle が先に捕まえるので、
この deny が効くのは domain を import しないサブパッケージが将来できたときである。

---

### Task 2: `openDiff` の doc に Search を足す

**Files:**
- Modify: `internal/app/presentation/tui/root/root.go:497-498`

- [ ] doc の「the Work board and a Repos row」に Search の結果行を足す。
      `openChecks` は**触らない**（事実 D）
- [ ] **Verification:** `make check`（コメントのみなので golden は動かない）

**Commit:** `docs: say that a Search result also opens the diff on its own`

---

### Task 3: `domain.ErrTransient` からサービス名を外す

**Files:**
- Modify: `internal/app/domain/errors.go:12-15`
- Modify: `internal/app/domain/errors_test.go`（`TestSentinelsNameTheKindAndNotTheRemedy` と、
  `TestIsFatalOnlyForWhatTheUserMustActOn` のケース名 `"GitHub did not answer"`）

- [ ] 文言を `"the backend did not answer"` に変える。doc も「GitHub's front end」を
      サービス中立な言い方に直す。**ただし「502 / 503 / 504」は残す**——HTTP の語は
      サービス固有ではないし、この種別が何を指すかを一番よく説明している
- [ ] `TestSentinelsNameTheKindAndNotTheRemedy` を拡張する。現行の禁止語は対処方法だけ
      （`gh CLI` / `gh auth` / `install` / `run:`）なので、**サービス名の禁止語リストを別に持ち**、
      3 つのセンチネル全部に掛ける。テスト名と doc コメントも「対処方法とサービス名」に合わせて改める
- [ ] `errors_test.go:25` のケース名を新しい文言に合わせる
- [ ] **Verification:** `make check`。特に `go test ./internal/app/presentation/tui/work/...`
      が通ること（`work_test.go:303` が `ErrTransient.Error()` を参照しており、
      文言が変わっても「画面に出ない」主張は変わらないはずである）

**Commit:** `refactor: stop the transient sentinel from naming the service`

コミット本文に事実 C を書く——この文言はどこにも表示されていない。`internal/github` 側の
同じ文言はインフラ層なので残す。

---

## 完了条件

- [ ] `make check` が通る
- [ ] golden 354 枚が 1 枚も変わっていない（`git diff --stat` に `.golden` が出ない）
- [ ] depguard の新しい deny が捨てパッケージ相手に発火することを観測した
- [ ] `openChecks` の doc を触っていない
- [ ] `internal/github.ErrTransient` を触っていない
- [ ] PR 本文に、実証の限界（事実 B）と「画面は変わらない」根拠（事実 C）を書いた
- [ ] TUI は触っていないが、#124〜#127 の実機確認が未実施のままであることを PR 本文に書く
