package schema

type SchemaVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

var CurrentVersion = SchemaVersion{
	Major: 1,
	Minor: 0,
}
