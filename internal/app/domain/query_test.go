package domain_test

import (
	"reflect"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// The settings file's shape belongs to internal/app/config; this type
// is what the application is written in terms of. A tag here would mean
// the two had been merged back together.
func TestSavedQueryCarriesNoSerialisationTags(t *testing.T) {
	typ := reflect.TypeOf(domain.SavedQuery{})
	for i := range typ.NumField() {
		if tag := typ.Field(i).Tag; tag != "" {
			t.Errorf("SavedQuery.%s carries a struct tag %q", typ.Field(i).Name, tag)
		}
	}
}
