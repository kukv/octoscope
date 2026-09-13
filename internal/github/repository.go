// Package github holds the types shared by its subpackages (cli, api, gql)
// that speak for GitHub itself rather than for one particular way of
// reaching it.
package github

// Repository is one repository as the add dialog's search or listing
// offers it. cli and api decode it from different shapes -- gh's JSON
// output and the REST endpoint respectively -- but both return this type,
// which is what lets the gateway convert either one the same way.
type Repository struct {
	Name    string
	Stars   int
	Private bool
}
