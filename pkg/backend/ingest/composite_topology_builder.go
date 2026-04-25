package ingest

import (
	"context"

	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

type CompositeTopologyBuilder struct {
	builders []TopologyBuilder
}

func NewCompositeTopologyBuilder(builders ...TopologyBuilder) *CompositeTopologyBuilder {
	return &CompositeTopologyBuilder{builders: builders}
}

func (b *CompositeTopologyBuilder) Build(ctx context.Context, envelope schema.RawEnvelope) ([]topology.Edge, error) {
	var edges []topology.Edge
	for _, builder := range b.builders {
		built, err := builder.Build(ctx, envelope)
		if err != nil {
			return nil, err
		}
		edges = append(edges, built...)
	}
	return edges, nil
}

func (b *CompositeTopologyBuilder) BuildNodes(ctx context.Context, envelope schema.RawEnvelope) []topology.Node {
	var nodes []topology.Node
	for _, builder := range b.builders {
		nodeBuilder, ok := builder.(NodeBuilder)
		if !ok {
			continue
		}
		nodes = append(nodes, nodeBuilder.BuildNodes(ctx, envelope)...)
	}
	return nodes
}
