package i18n_test

import (
	"testing"
	"time"

	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/i18n"
)

func TestTranslatesInEnglishAndJapanese(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	i18n.SetLanguage(language.English)
	if got := i18n.T("list.no_open_prs"); got != "No open pull requests" {
		t.Errorf("en = %q", got)
	}

	i18n.SetLanguage(language.Japanese)
	if got := i18n.T("list.no_open_prs"); got != "オープンなプルリクエストはありません" {
		t.Errorf("ja = %q", got)
	}
}

func TestPluralInEnglish(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })
	i18n.SetLanguage(language.English)

	if got := i18n.Tn("time.hours_ago", 1); got != "1h ago" {
		t.Errorf("n=1: %q", got)
	}
	if got := i18n.Tn("time.hours_ago", 3); got != "3h ago" {
		t.Errorf("n=3: %q", got)
	}
}

func TestPluralInJapanese(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })
	i18n.SetLanguage(language.Japanese)

	if got := i18n.Tn("time.hours_ago", 3); got != "3 時間前" {
		t.Errorf("n=3: %q", got)
	}
}

func TestResolveOrder(t *testing.T) {
	cases := []struct {
		name          string
		flag, osLocal string
		want          language.Tag
	}{
		{"flag wins", "ja", "en-US", language.Japanese},
		{"os locale used when no flag", "", "ja-JP", language.Japanese},
		{"english by default", "", "", language.English},
		{"unsupported falls back to english", "", "de-DE", language.English},
		{"invalid flag falls back to os locale", "zzz", "ja-JP", language.Japanese},
	}
	for _, c := range cases {
		if got := i18n.Resolve(c.flag, "", c.osLocal); got != c.want {
			t.Errorf("%s: Resolve(%q, %q) = %v, want %v", c.name, c.flag, c.osLocal, got, c.want)
		}
	}
}

// The flag is a one-off override; the settings file is the standing choice.
// A user who set language: ja and passes --lang en wants English this once.
func TestResolvePrefersTheFlagOverTheSettingsFile(t *testing.T) {
	if got := i18n.Resolve("en", "ja", "ja_JP.UTF-8"); got != language.English {
		t.Errorf("Resolve = %v, want English", got)
	}
}

// The settings file is a deliberate choice; the operating system locale is
// only a guess at one.
func TestResolvePrefersTheSettingsFileOverTheOSLocale(t *testing.T) {
	if got := i18n.Resolve("", "ja", "en_US.UTF-8"); got != language.Japanese {
		t.Errorf("Resolve = %v, want Japanese", got)
	}
}

// A language with no catalog is not a reason to fall all the way back to
// English: the next candidate still gets its turn.
func TestResolveSkipsASettingsFileLanguageWithNoCatalog(t *testing.T) {
	if got := i18n.Resolve("", "fr", "ja_JP.UTF-8"); got != language.Japanese {
		t.Errorf("Resolve = %v, want Japanese", got)
	}
}

func TestCatalogsHaveTheSameIDs(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	i18n.SetLanguage(language.English)
	en := i18n.IDs()
	i18n.SetLanguage(language.Japanese)
	ja := i18n.IDs()

	inEn := map[string]bool{}
	for _, id := range en {
		inEn[id] = true
	}
	inJa := map[string]bool{}
	for _, id := range ja {
		inJa[id] = true
	}

	var missingFromJa, missingFromEn []string
	for _, id := range en {
		if !inJa[id] {
			missingFromJa = append(missingFromJa, id)
		}
	}
	for _, id := range ja {
		if !inEn[id] {
			missingFromEn = append(missingFromEn, id)
		}
	}
	if len(missingFromJa) > 0 {
		t.Errorf("ja catalog is missing IDs: %v", missingFromJa)
	}
	if len(missingFromEn) > 0 {
		t.Errorf("en catalog is missing IDs: %v", missingFromEn)
	}
}

func TestDateTimeUsesThePerLanguageLayout(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	when := time.Date(2026, 9, 6, 14, 5, 0, 0, time.UTC)

	tests := []struct {
		lang language.Tag
		want string
	}{
		{language.English, "Sep 6, 2026 14:05"},
		{language.Japanese, "2026年9月6日 14:05"},
	}

	for _, tt := range tests {
		i18n.SetLanguage(tt.lang)
		if got := i18n.DateTime(when); got != tt.want {
			t.Errorf("lang %s: got %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestRelTimePicksTheRightUnit(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })
	i18n.SetLanguage(language.English)

	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"under a minute", 59 * time.Second, "now"},
		{"exactly a minute", time.Minute, "1m ago"},
		{"under an hour", 59 * time.Minute, "59m ago"},
		{"exactly an hour", time.Hour, "1h ago"},
		{"under a day", 25 * time.Hour, "1d ago"},
		{"several days", 72 * time.Hour, "3d ago"},
	}

	for _, tt := range tests {
		if got := i18n.RelTime(now, now.Add(-tt.ago)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
