// Package schema holds the JSON Schemas Docent ships, embedded so the binary,
// `docent schema` and `docent validate` all read the same bytes.
package schema

import _ "embed"

// TourV1ID is the canonical URL of the version 1 tour schema.
const TourV1ID = "https://raw.githubusercontent.com/primandproper/docent/main/schema/tour.v1.json"

// TourV1 is the version 1 tour schema.
//
//go:embed tour.v1.json
var TourV1 []byte
