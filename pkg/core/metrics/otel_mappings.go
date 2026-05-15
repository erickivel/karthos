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
	"k8s.pod.cpu.usage":                        {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.pod.memory.working_set":               {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.node.cpu.usage":                       {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.node.memory.working_set":              {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.cpu.request":                {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.cpu.limit":                  {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.memory.request":             {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.memory.limit":               {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.storage.request":            {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.storage.limit":              {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.ephemeral_storage.request":  {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.ephemeral_storage.limit":    {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.cpu.usage":                  {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.container.memory.usage":               {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.ready":                      {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.container.restarts":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindCounter},
	"system.cpu.utilization":                   {ConversionFactor: 100.0, Unit: UnitPercent, Kind: MetricKindGauge},
	"system.memory.usage":                      {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.disk.io":                           {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.disk.io.read":                      {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.disk.io.write":                     {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.network.io":                        {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.network.io.receive":                {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.network.io.transmit":               {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindGauge},
	"system.network.errors":                    {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"system.network.dropped":                   {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"system.network.packets":                   {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"system.disk.operations":                   {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"system.disk.operation_time":               {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindCounter},
	"k8s.volume.available":                     {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.volume.capacity":                      {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.volume.inodes.free":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.volume.inodes.used":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.filesystem.usage":                  {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.filesystem.utilization":            {ConversionFactor: 100.0, Unit: UnitPercent, Kind: MetricKindGauge},
	"system.cpu.load_average.1m":               {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.cpu.load_average.5m":               {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.cpu.load_average.15m":              {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.paging.usage":                      {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"system.paging.operations":                 {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindGauge},
	"k8s.replicaset.desired":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.replicaset.available":                 {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.replication_controller.desired":       {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.replication_controller.available":     {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.deployment.desired":                   {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.deployment.available":                 {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.cronjob.active_jobs":                  {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.namespace.phase":                      {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.pod.phase":                            {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.pod.status_reason":                    {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition":                       {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.ready":                 {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.memory_pressure":       {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.condition.disk_pressure":         {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.node.allocatable_cpu":                 {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
	"k8s.node.allocatable_memory":              {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.node.allocatable_pods":                {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.daemonset.desired_scheduled_nodes":    {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.daemonset.current_scheduled_nodes":    {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.daemonset.misscheduled_nodes":         {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.daemonset.ready_nodes":                {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.statefulset.desired_pods":             {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.statefulset.ready_pods":               {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.statefulset.current_pods":             {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.statefulset.updated_pods":             {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.hpa.max_replicas":                     {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.hpa.min_replicas":                     {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.hpa.current_replicas":                 {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.hpa.desired_replicas":                 {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.job.active_pods":                      {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.job.desired_successful_pods":          {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.job.failed_pods":                      {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.job.max_parallel_pods":                {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.job.successful_pods":                  {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"k8s.resource_quota.hard_limit":            {ConversionFactor: 1.0, Unit: UnitResource, Kind: MetricKindGauge},
	"k8s.resource_quota.used":                  {ConversionFactor: 1.0, Unit: UnitResource, Kind: MetricKindGauge},
	"http.server.request.count":                {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"http.server.request.duration":             {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindHistogram},
	"rpc.server.request.count":                 {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"rpc.server.duration":                      {ConversionFactor: 1e6, Unit: UnitNanoseconds, Kind: MetricKindHistogram},
	"servicegraph.request.count":               {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"servicegraph.request.duration":            {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindHistogram},
	"servicegraph.request.error.count":         {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"servicegraph.request.body.size":           {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindHistogram},
	"servicegraph.response.body.size":          {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindHistogram},
	"servicegraph.grpc.request.message.count":  {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"servicegraph.grpc.response.message.count": {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"servicegraph.tcp.connection.open.count":   {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"servicegraph.tcp.connection.close.count":  {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"servicegraph.tcp.bytes.sent":              {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindCounter},
	"servicegraph.tcp.bytes.received":          {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindCounter},

	// Kubelet pod & node network (Sum, cumulative).
	"k8s.pod.network.io":      {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindCounter},
	"k8s.pod.network.errors":  {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"k8s.node.network.io":     {ConversionFactor: 1.0, Unit: UnitBytesPerSecond, Kind: MetricKindCounter},
	"k8s.node.network.errors": {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},

	// Kubelet filesystem at pod / node / container scopes (Gauge, bytes).
	"k8s.pod.filesystem.usage":           {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.pod.filesystem.capacity":        {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.pod.filesystem.available":       {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.node.filesystem.usage":          {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.node.filesystem.capacity":       {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.node.filesystem.available":      {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.filesystem.usage":     {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.filesystem.capacity":  {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},
	"k8s.container.filesystem.available": {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},

	// Volume inodes total (free/used already mapped above).
	"k8s.volume.inodes": {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},

	// k8s_cluster node allocatable ephemeral storage.
	"k8s.node.allocatable_ephemeral_storage": {ConversionFactor: 1.0, Unit: UnitBytes, Kind: MetricKindGauge},

	// Hostmetrics CPU time (Sum, seconds → ns to mirror system.disk.operation_time).
	"system.cpu.time": {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindCounter},

	// Hostmetrics disk saturation. io_time / weighted_io_time mirror operation_time
	// (cumulative seconds → nanoseconds, query-side rate()). merged is a cumulative
	// op count. pending_operations is an instantaneous queue depth (Gauge).
	"system.disk.io_time":            {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindCounter},
	"system.disk.weighted_io_time":   {ConversionFactor: 1e9, Unit: UnitNanoseconds, Kind: MetricKindCounter},
	"system.disk.merged":             {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"system.disk.pending_operations": {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},

	// Hostmetrics filesystem inodes / paging faults / network connections.
	"system.filesystem.inodes.usage": {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	"system.paging.faults":           {ConversionFactor: 1.0, Unit: UnitPerSecond, Kind: MetricKindCounter},
	"system.network.connections":     {ConversionFactor: 1.0, Unit: UnitCount, Kind: MetricKindGauge},
	// Note: servicegraph.request.error.ratio is query-derived (see metric_store.buildPromQL)
	// and intentionally NOT in this table — it never arrives as an OTLP sample.
}
