package topology

type RelationType string

const (
	RelCalls    RelationType = "calls"
	RelRunsOn   RelationType = "runs_on"
	RelContains RelationType = "contains"
	RelRoutesTo RelationType = "routes_to"
)
