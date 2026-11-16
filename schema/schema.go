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

// VisualV1ID is the canonical URL of the version 1 visual schema.
const VisualV1ID = "https://raw.githubusercontent.com/primandproper/docent/main/schema/visual.v1.json"

// VisualV1 is the version 1 schema for animated visuals.
//
//go:embed visual.v1.json
var VisualV1 []byte
