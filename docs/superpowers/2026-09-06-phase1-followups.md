# Phase 1 の積み残し

Phase 1（PR #46 / #47 / #49 / #51、いずれも main にマージ済み）で
**満たせていない spec 要件**と、意図的に後回しにした宿題をまとめる。

spec は `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`。
実装計画は `docs/superpowers/plans/2026-09-06-octoscope-phase1.md`。

**状況（2026-09-06 追記）:** 1〜7 とその他の宿題は対応済み。
**8（実端末での目視確認）だけが未了で、これは環境に TTY が無いため人手が要る。**

## なぜこれが起きたか

この環境には TTY が無く、Phase 1 の実装中に**一度も画面を見ないまま 4 PR を
マージした**。テストは 246 本すべて緑、カバレッジも 89% あったが、
**色・グリフ・カードの行数を検証しているテストが 1 つも無かった**ため、
spec §4.5 の「装飾の強度は最大限」が丸ごと未達のまま通ってしまった。

TTY が無くても、テストの中で `View()` を出力すれば見える。1 番がそれである。

---

## 1. 描画ハーネス — 済

`internal/golden` と各ビューの `golden_test.go` / `testdata/*.golden`。
en / ja × 80 / 120 / 160 桁で `View()` を録る。ANSI エスケープは落とさない。

```bash
make golden                              # 録り直す
cat -v internal/tui/work/testdata/work_ja_80.golden   # 目で見る
```

規約は `.claude/rules/testing.md` の「画面は golden に録る」に入れた。

この過程で `repo.View()` が `time.Now()` を読んでいた（`View` は副作用を
持たない、の違反）のを Update 側へ移した。

## 2. 色 — 済

`internal/tui/theme` に集約。ビューは `theme.Dim()` のような役割名で引き、
16 進の色をビューに書かない。

**v2 に `lipgloss.AdaptiveColor` は無い。** `lipgloss.LightDark(isDark)` を使い、
`isDark` は `tea.RequestBackgroundColor` → `tea.BackgroundColorMsg` で得て
`theme.SetDark` に渡す。`.claude/rules/tui.md` もそのとおりに直した。

## 3. Nerd Font グリフと ASCII フォールバック — 済

`unicode`（既定）/ `nerd` / `ascii` の 3 セット。`--icons` と `OCTOSCOPE_ICONS`。

**自動判定はしない。** 端末は使用中のフォントも収録範囲も報告しないため、
検出する信頼できる手段が無い。推測して外すと盤面が豆腐になる。
根拠は spec §4.5 に追記した。

Nerd Font のグリフは私用領域にあり幅を前提にできないので、
`TestEveryGlyphIsOneColumn` が全セットを 1 桁に縛っている。

## 4. カードを 2 行に — 済

80 桁では 1 列 18 桁しかないため、2 行目は bar と経過時間を右に固定し、
リポジトリ名に残りを与える（残りが無ければリポジトリ名は出ない）。

## 5. ラベルの塗りつぶしバッジ — 済

`work.graphql` に `LabelFields` フラグメントを PR / Issue 両方へ。
`theme.Badge` が背景の輝度から白／黒の文字色を選ぶ。
タイトルが切り詰められる幅ではバッジを出さない。

## 6. ドロワーに本文と checks 一覧 — 済

`bodyText` と各 check の名前（CheckRun は `name`、StatusContext は `context`）。
失敗した check を先頭に並べ、本文 3 行 / checks 5 件で打ち切る。
ドロワーは盤面の下に描くので、高さは端末サイズに関係なく固定にしてある。

## 7. マウス操作 — 済

spec に **§4.0 マウス操作**を追加した。要点:

- `View.MouseMode = tea.MouseModeCellMotion`（v2 に `tea.WithMouseCellMotion` は無い）
- 1 回目のクリックで選択、選択中をもう 1 回で詳細。**ダブルクリックは使わない**
  （Bubble Tea が報告しないため、自前で測ると `Update` に時計が入る）
- マウスはブロードキャストせず、表示中のタブ 1 つにだけ配る
- 当たり判定は描画と同じ関数を読む。テストは「実際に描かれた位置」を
  探してそこをクリックする

## 8. 実端末での目視確認 — **未了（人手が要る）**

この環境には TTY が無い。golden は桁ずれと色の変化を捉えるが、
実端末での見え方そのものは代替できない。

```bash
go run ./cmd/octoscope
go run ./cmd/octoscope --lang ja      # 全角で桁がずれていないか
go run ./cmd/octoscope --icons nerd   # パッチ済みフォントがある環境で
go run ./cmd/octoscope --icons ascii
```

確認する境界:

- **80 桁**で崩れないこと
- **100 桁未満**でドロワーが消えること
- **60 桁未満**で 1 列ページングになること
- マウスのクリック位置と選択が実際に一致すること（golden では
  「描かれた位置」までしか確かめられない）

### 8 と一緒に見るべき既知の問題

**`work.View()` は `m.height` を見ていない。** カードの多い列があると、
盤面がそのぶん伸びてドロワーとフッターが画面外へ押し出される。
Phase 1 からある問題（3 行カードの頃も同じ）だが、ドロワーが最大 12 行に
なったぶん目につきやすくなった。今回の範囲外として残す。
直すなら「盤面に高さ予算を与えて列内をスクロールさせる」形になる。

---

## その他の宿題 — 済

- **`hasRepo` の起動時ブロック** — ルートモデルが最初のサイズを受け取った
  時点で非同期に問い合わせ、`repoResolvedMsg` が返ってから Repos タブを出す
- **`gh.PR.State` / `ReviewDecision` が生文字列** — `gh.ItemState` /
  `gh.ReviewState` に変換。`internal/gh/cli` に DTO を置き、
  `gh.PR` / `gh.Issue` からは JSON タグを外した。
  状態語は両カタログに入れた（`state.*` / `review.*`）
- **`prMsg` / `issueMsg` のクロストーク** — `errMsg` と同じく `ref` を持たせ、
  自分宛でない応答を捨てるようにした
