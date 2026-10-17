# root: 警告が 2 つ立ってもタブ行を 1 行に収める（#160）

## 問題

`tabRow`（`internal/app/presentation/tui/root/render.go`）は `tab.repo_lookup_timeout` と `tab.config_unreadable` を
切らずに連結する。両方立つと ja で 97 桁、en で 108 桁になり、80 桁で折り返す。
以前は「稀であること」「片方を切ると、どちらの警告か分からなくなる」ことから見送った
（`docs/superpowers/2026-09-08-phase4-config-followups.md`）。

## 方針（利用者と決めた）

- 警告ごとに短い文言（`tab.repo_lookup_timeout_short` / `tab.config_unreadable_short`）を en・ja のカタログに足す
  - en: `repo unknown` / `settings unreadable`
  - ja: `リポジトリ不明` / `設定を読めません`
- 普段の文言でタブ行が幅を超えるときだけ、両方を短い方にする。片方だけのときや幅が足りるときは今と同じ表示
- それでも収まらない幅では、タブ行を幅で切って折り返さない（`layout.ClipLines`）

却下: 常に短い文言にする（幅があっても「既定値で動作」の補足が消える）。末尾を … で切るだけ（2 つ目の警告がほぼ消え、見送った理由がそのまま残る）。

## タスク

1. テスト（赤）: 両方の警告が立った状態の root を、ja・en の 80 桁で描くと、タブ行が 1 行で 80 桁以内に収まり、
   2 つの短い文言がどちらも出る。120 桁以上では今の文言のまま
2. カタログに短い文言を足し、`tabRow` を直して緑にする
3. golden `app_two_warnings`（en・ja × 80 / 120 / 160）を足す
4. `make check`
