package topology

import "time"

type Graph struct {
	Nodes     []Node    `json:"nodes"`
	Edges     []Edge    `json:"edges"`
	Timestamp time.Time `json:"timestamp"`
}
