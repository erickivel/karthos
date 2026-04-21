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

// OTLPSemconvMetrics stores semconv-native metric metadata after source-specific
// dialect translation has already happened.
var OTLPSemconvMetrics = map[string]OTLPMetricMapping{
	"k8s.pod.cpu.usage":                     {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.pod.memory.working_set":            {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.node.cpu.usage":                    {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.node.memory.working_set":           {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.cpu.request":             {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.cpu.limit":               {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.memory.request":          {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.memory.limit":            {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.cpu.usage":               {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.memory.usage":            {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.cpu.utilization":                {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"system.memory.usage":                   {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.disk.io":                        {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.disk.io.read":                   {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.disk.io.write":                  {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.network.io":                     {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.network.io.receive":             {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.network.io.transmit":            {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"k8s.volume.available":                  {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.volume.capacity":                   {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.volume.inodes.free":                {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.volume.inodes.used":                {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.filesystem.usage":               {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.filesystem.utilization":         {ConversionFactor: 1.0, Unit: UnitRatio, Kind: MetricKindGauge},
	"system.cpu.load_average.1m":            {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.cpu.load_average.5m":            {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.cpu.load_average.15m":           {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.paging.usage":                   {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.paging.operations":              {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindGauge},
	"k8s.replicaset.desired":                {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.deployment.desired":                {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.deployment.available":              {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.pod.phase":                         {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.namespace.phase":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition":                    {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.ready":              {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.memory_pressure":    {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.disk_pressure":      {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.allocatable_cpu":              {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.node.allocatable_memory":           {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.daemonset.desired_scheduled_nodes": {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.daemonset.ready_nodes":             {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.statefulset.desired_pods":          {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.statefulset.ready_pods":            {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"http.server.request.count":             {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"http.server.request.duration":          {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindHistogram},
	"rpc.server.request.count":              {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"rpc.server.duration":                   {ConversionFactor: 1e6, Unit: UnitNanoseconds, Kind: MetricKindHistogram},
	"servicegraph.request.count":            {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"servicegraph.request.duration":         {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindHistogram},
	"servicegraph.request.error.count":      {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	// Note: servicegraph.request.error.ratio is query-derived (see metric_store.buildPromQL)
	// and intentionally NOT in this table — it never arrives as an OTLP sample.
}
