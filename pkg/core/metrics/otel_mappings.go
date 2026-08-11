package metrics

type MetricKind string

const (
	MetricKindGauge     MetricKind = "gauge"
	MetricKindCounter   MetricKind = "counter"
	MetricKindHistogram MetricKind = "histogram"
)

// OTLPMetricMapping describes semantic metric metadata plus optional unit conversion.
type OTLPMetricMapping struct {
	ConversionFactor float64
	Unit             Unit
	Kind             MetricKind
}

// OTLPSemconvMetrics is the intentionally small PoC ingestion boundary after
// source-specific dialect translation. Metrics outside this table are rejected
// rather than silently entering storage without a canonical query contract.
var OTLPSemconvMetrics = map[string]OTLPMetricMapping{
	"k8s.pod.cpu.usage":                        {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.pod.memory.working_set":               {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.node.cpu.usage":                       {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.node.memory.working_set":              {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.cpu.request":                {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.cpu.limit":                  {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.memory.request":             {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.memory.limit":               {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.cpu.usage":                  {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.memory.usage":               {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.ready":                      {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.container.restarts":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"k8s.volume.available":                     {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.volume.capacity":                      {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.volume.inodes.free":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.volume.inodes.used":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.volume.inodes":                        {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.cpu.utilization":                   {ConversionFactor: 100.0, Unit: UnitPercent, Kind: MetricKindGauge},
	"system.memory.usage":                      {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.disk.io":                           {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindCounter},
	"system.disk.io.read":                      {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindCounter},
	"system.disk.io.write":                     {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindCounter},
	"system.disk.operations":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"system.filesystem.usage":                  {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.filesystem.utilization":            {ConversionFactor: 100.0, Unit: UnitPercent, Kind: MetricKindGauge},
	"system.network.io.receive":                {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindCounter},
	"system.network.io":                        {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindCounter},
	"system.network.io.transmit":               {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindCounter},
	"system.network.errors":                    {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"k8s.pod.phase":                            {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.pod.status_reason":                    {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition":                       {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.ready":                 {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.memory_pressure":       {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.disk_pressure":         {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"http.server.request.count":                {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"http.server.request.duration":             {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindHistogram},
	"servicegraph.request.count":               {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"servicegraph.request.duration":            {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindHistogram},
	"servicegraph.request.error.count":         {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"servicegraph.grpc.request.message.count":  {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"servicegraph.grpc.response.message.count": {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"servicegraph.tcp.connection.open.count":   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"servicegraph.tcp.connection.close.count":  {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"servicegraph.tcp.bytes.sent":              {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindCounter},
	"servicegraph.tcp.bytes.received":          {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindCounter},
}
