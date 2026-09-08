// Package i18n loads the message catalogs and renders localized strings.
package i18n

import (
	"embed"
	"slices"
	"sort"
	"sync"
	"time"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	yaml "go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
)

//go:embed locales/*.yaml
var localeFS embed.FS

// supported lists the languages with a catalog, most preferred first.
var supported = []language.Tag{language.English, language.Japanese}

var (
	mu        sync.RWMutex
	bundle    *goi18n.Bundle
	localizer *goi18n.Localizer
	catalog   = map[language.Tag][]string{}
	current   = language.English
)

func init() {
	bundle = goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)
	for _, name := range []string{"locales/active.en.yaml", "locales/active.ja.yaml"} {
		f, err := bundle.LoadMessageFileFS(localeFS, name)
		if err != nil {
			// The catalogs are embedded, so a failure here is a build-time bug.
			panic(err)
		}
		ids := make([]string, 0, len(f.Messages))
		for _, m := range f.Messages {
			ids = append(ids, m.ID)
		}
		sort.Strings(ids)
		catalog[f.Tag] = ids
	}
	SetLanguage(language.English)
}

// SetLanguage switches the language used by T, Tf and Tn.
func SetLanguage(tag language.Tag) {
	mu.Lock()
	defer mu.Unlock()
	current = tag
	localizer = goi18n.NewLocalizer(bundle, tag.String())
}

// Resolve picks the display language: the --lang flag first, then the
// settings file, then the locale reported by the operating system, then
// English.
func Resolve(flagLang, configLang, osLocale string) language.Tag {
	matcher := language.NewMatcher(supported)
	for _, candidate := range []string{flagLang, configLang, osLocale} {
		if candidate == "" {
			continue
		}
		// language.Parse returns a best-effort tag even when it also
		// returns an error: --lang is typed by hand and can be a
		// tag with an unrecognized subtag, which still resolves to its
		// base language. Only input it cannot parse at all comes back as
		// language.Und, which the confidence check below rejects.
		tag, _ := language.Parse(candidate)
		if _, index, conf := matcher.Match(tag); conf != language.No {
			return supported[index]
		}
	}
	return language.English
}

// IDs returns the message IDs in the current language's catalog, sorted.
func IDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	return slices.Clone(catalog[current])
}

// T renders the message with no template data.
func T(id string) string {
	return render(&goi18n.LocalizeConfig{MessageID: id})
}

// Tf renders the message with template data, e.g. {"Title": "..."}.
func Tf(id string, data map[string]any) string {
	return render(&goi18n.LocalizeConfig{MessageID: id, TemplateData: data})
}

// Tn renders a message whose wording depends on n; the template variable
// is .Count.
func Tn(id string, n int) string {
	return render(&goi18n.LocalizeConfig{
		MessageID:    id,
		PluralCount:  n,
		TemplateData: map[string]any{"Count": n},
	})
}

// DateTime formats t with the active language's layout. The catalog holds a
// Go reference-time layout ("Jan 2, 2006 15:04"), not a display string,
// because the field order differs by language.
func DateTime(t time.Time) string {
	return t.Format(T("time.datetime_layout"))
}

// RelTime renders how long ago t was, relative to now.
func RelTime(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return T("time.now")
	case d < time.Hour:
		return Tn("time.minutes_ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return Tn("time.hours_ago", int(d.Hours()))
	default:
		return Tn("time.days_ago", int(d.Hours()/24))
	}
}

func render(cfg *goi18n.LocalizeConfig) string {
	mu.RLock()
	l := localizer
	mu.RUnlock()
	s, err := l.Localize(cfg)
	if err != nil {
		// A missing ID is a programming error; surface it rather than
		// rendering an empty string that is hard to trace.
		return "!" + cfg.MessageID
	}
	return s
}
