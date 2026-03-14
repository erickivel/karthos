package entities

import "strconv"

type Endpoint struct {
	EntityID EntityID          `json:"id"`
	Address  string            `json:"address"`
	Port     uint32            `json:"port,omitempty"`
	Metadata map[string]string `json:"labels,omitempty"`
}

func NewEndpoint(address string, port uint32, labels map[string]string) Endpoint {
	return Endpoint{
		EntityID: ComputeEntityID(KindEndpoint, address, strconv.FormatUint(uint64(port), 10)),
		Address:  address,
		Port:     port,
		Metadata: cloneLabels(labels),
	}
}

func (e Endpoint) ID() EntityID { return e.EntityID }

func (e Endpoint) Kind() EntityKind { return KindEndpoint }

func (e Endpoint) Labels() map[string]string { return cloneLabels(e.Metadata) }
