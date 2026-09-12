# 引き継ぎと手動確認 実装計画（Phase 4 スライス 4-5）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Phase 4 を閉じる。実端末でしか確かめられないことの手順を渡し、
4-4 の積み残しが 4-5 に送った 1 件（サインインの案内文が `gh` の存在を前提に
している）を決着させる。

**Architecture:** コードの変更は文言 1 つだけ。残りは文書である。

**Tech Stack:** 変更なし。

**Spec:** `docs/superpowers/specs/2026-09-08-phase4-design.md`（§10 の 4-5、
§11 の完了条件 10）。前スライスの積み残しは
`docs/superpowers/2026-09-13-phase4-actions-followups.md`。

---

## Global Constraints

- **i18n のキーは en / ja の両方に入れる。** 片方だけだと
  `internal/i18n/i18n_test.go:117` が落ちる
- 文言の変更は golden に出る可能性がある。`make check` が落ちたら
  `make golden` で録り直し、**diff を目で見てから**コミットする
- コメントは英語。実装計画や設計書への参照をコードに書かない
- `make check` が緑であること

---

## 着手前に確かめたこと（2026-09-13）

**この環境でも pty は割り当てられる**（`script -qec ... /dev/null` が
`/dev/pts/N` を返す）。それを使って次を実測した。

| 確かめたこと | 結果 |
|---|---|
| `api` バックエンドで起動するか | **起動する。** `PATH` から `gh` を外し `GH_TOKEN` だけを与えて 25 秒間動かし、alt screen に入ったまま落ちなかった |
| 画面の内容を取り込めるか | **取り込めない。** `script` 経由では alt screen に入った後の描画が捕まらず、145 バイト（エスケープのみ）しか残らない。`cli` バックエンドでも同じなので capture 側の制約であって `api` の問題ではない |
| `api` が `source` の読み取りを実データで全部答えるか | **答える。** 一時的な smoke テスト（`//go:build smoke`）を書いて `GH_TOKEN` だけで 16 メソッドを走らせ、全部成功した。内訳は下記 |

smoke の結果（`kukv/octoscope` と、open PR のある `cli/cli`）:

```
RepoName ok  ListPRs ok  ListIssues ok  ListLabels ok(19)  ListAssignees ok
ListOrgs ok  ListOwnRepos ok(5)  SearchRepos ok(5)  SearchItems ok(50)
RepoCounts ok(2)  ListWorkSection ok(50)
using cli/cli#14398:
GetPR ok  PRDiff ok  PRChecks ok(7)  PRReviewContext ok  PRMergeContext ok(3)
```

**したがって完了条件 10 のうち「トークンだけで取得が全部動く」部分は確認済みで、
残るのは画面とキー操作と書き込み系である。** smoke テストは一時的なもので、
確認後に削除した（リポジトリには残っていない）。

---

## Task 1: サインインの案内が `gh` の有無を前提にしないようにする

**Files:**
- Modify: `internal/i18n/locales/active.en.yaml`
- Modify: `internal/i18n/locales/active.ja.yaml`
- Test: `internal/i18n/i18n_test.go`（既存のカタログ一致テストが守る）

**なぜ:** `error.unauthenticated` は現在 `Not signed in to GitHub. Run: gh auth
login` と出る。これはトークンが設定されているが無効な場合に `api`
バックエンドでも表示され、**`gh` が入っていないマシンでは実行できない案内**に
なる。同じファイルの `error.no_backend` は既に両方の手段を並べており、
そちらに揃える。

- [ ] **Step 1: 文言を変える**

`internal/i18n/locales/active.en.yaml`:

```yaml
  unauthenticated:
    other: "Not signed in to GitHub. Run gh auth login, or set GH_TOKEN to a personal access token."
```

`internal/i18n/locales/active.ja.yaml`:

```yaml
  unauthenticated:
    other: "GitHub にサインインしていません。gh auth login を実行するか、GH_TOKEN に個人アクセストークンを設定してください。"
```

- [ ] **Step 2: 両カタログが揃っていることを確かめる**

Run: `go test ./internal/i18n/ -v`
Expected: PASS（`i18n_test.go:117` が en / ja の ID 一致を見る）

- [ ] **Step 3: 桁を確かめる**

この文言は画面のどこに出るか（全画面のエラーか、フッター 1 行か）を
`grep -rn "error.unauthenticated" internal/tui/` で確かめ、**80 桁で
折り返しが壊れていないこと**を golden で見る。

Run: `make check`
落ちたら `make golden` で録り直し、`git diff` を目で見てからコミットする。
**日本語は全角で桁を 2 つ使う。** ja の 80 桁を必ず見る。

- [ ] **Step 4: コミット**

```bash
make check
git add internal/i18n/locales/active.en.yaml internal/i18n/locales/active.ja.yaml
git commit -m "fix: say both ways to sign in, not just the one that needs gh"
```

---

## Task 2: 引き継ぎ文書

**Files:**
- Create: `docs/superpowers/2026-09-13-phase4-handover.md`

既存の受け渡し文書（`docs/superpowers/2026-09-07-phase3-checks-handoff.md`）と
同じ形にする。**この環境で確認済みのことと、実端末でしか確かめられないことを
はっきり分ける。** 前者を「もう一度やってください」と書かない。

- [ ] **Step 1: 「確認済み」の節を書く**

上の「着手前に確かめたこと」を移す。`api` が 16 メソッドを実データで
答えること、pty では画面を取り込めないこと、その根拠。

- [ ] **Step 2: 実端末でしかできないことの手順を書く**

`gh` を PATH から外し `GH_TOKEN` だけで起動する手順（`env -i` ではなく、
`go` と端末が要るので PATH を絞る形）。そのうえで:

- 3 つのタブ（Work / Repos / Search）が出て、それぞれ実データが描かれること
- PR / Issue の詳細、diff、checks、review、merge の各画面が出ること
- **書き込み系**（コメント、close / reopen、ラベル・担当者の編集、
  レビュー送信、マージ）が `api` バックエンドで実際に効くこと。
  **smoke で確認したのは読み取りだけで、書き込みは 1 つも試していない**
- **`RerunWorkflow`**（`R` キー、失敗のみ / 全体の両方）が実際に CI を
  動かすこと。この環境では本物の CI を走らせないために実行しなかった
- 設定ファイルの保存（リポジトリ追加・保存クエリ）が次の起動でも残ること

- [ ] **Step 3: 4-4 で分かった制約を書く**

**ログのステップ別表示と「失敗のみ」は現在の GitHub では効かない。**
全行 `UNKNOWN STEP` になる。`gh` も同じなので octoscope の不具合ではない。
実端末で見たとき「壊れている」と誤解しないために、ここに書いておく。
詳細は `docs/superpowers/2026-09-13-phase4-actions-followups.md`。

- [ ] **Step 4: 報告してほしいことを書く**

桁ずれなら端末とフォントと `--icons`、取得が遅いなら run の規模
（`responseTimeout` 16 秒の見直し材料になる）。

- [ ] **Step 5: コミット**

```bash
make check
git add docs/superpowers/2026-09-13-phase4-handover.md
git commit -m "docs: hand over what only a real terminal can check"
```

---

## Task 3: Phase 4 を閉じる

**Files:**
- Modify: `docs/superpowers/specs/2026-09-08-phase4-design.md`

- [ ] **Step 1: 完了条件を照合する**

§11 の 1〜9 を 1 つずつ、どこで満たされたかを確かめる。9（golden が
en / ja × 80 / 120 / 160）は `ls internal/tui/*/testdata/*.golden` で数える。
**満たせていないものがあれば、そう書く。** 満たしたことにしない。

- [ ] **Step 2: 10 の現状を書く**

完了条件 10 は「人手」とされていたが、**取得系は 4-5 で機械的に確認できた**。
§11 にその旨と、残りが画面・キー操作・書き込み系であることを書く。

- [ ] **Step 3: コミット**

```bash
make check
git add docs/superpowers/specs/2026-09-08-phase4-design.md
git commit -m "docs: close Phase 4 against its completion conditions"
```

---

## Self-Review

**1. 設計の網羅:** §10 が 4-5 に割り当てたのは「引き継ぎと手動確認の手順」で
Task 2。§11 の完了条件 10 の照合が Task 3。4-4 の積み残しが 4-5 に送った
案内文の件が Task 1。

**2. 型の一致:** コードの変更は i18n の文字列 2 つだけで、型は動かない。

**3. placeholder:** 無し。Task 1 の文言は実際に入れる文字列そのもの。
