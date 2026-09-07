import type { MetricQuery, MetricScope } from '@/types/metrics'

export interface SeriesLabel {
  label: string
  value: string
}

interface MetricPresentationInput {
  canonicalName: string
  scope: Extract<MetricScope, 'entity' | 'edge'>
  labels?: Record<string, string>
  fallbackMeaning?: string
  fallbackTitle?: string
}

interface MetricPresentation {
  title: string
  meaning: string
  seriesLabels: SeriesLabel[]
}

export interface MetricDetailRow {
  label: string
  value: string
}

const SERVICE_ENTITY_PRESENTATION: Record<string, Pick<MetricPresentation, 'title' | 'meaning'>> = {
  'http.server.request.count': {
    title: 'Incoming request rate',
    meaning: 'Requests this service handles per second.',
  },
  'http.server.request.duration': {
    title: 'Incoming response time',
    meaning: 'Mean time this service takes to respond.',
  },
  'servicegraph.request.count': {
    title: 'Outbound request rate',
    meaning: 'Requests this service sends to other services per second.',
  },
  'servicegraph.request.error.count': {
    title: 'Outbound error rate',
    meaning: 'Failed requests this service sends to other services per second.',
  },
}

const EDGE_PRESENTATION: Record<string, Pick<MetricPresentation, 'title' | 'meaning'>> = {
  'servicegraph.request.count': {
    title: 'Request rate',
    meaning: 'Requests travelling across this connection per second.',
  },
  'servicegraph.request.duration': {
    title: 'Response time',
    meaning: 'Mean response time for requests on this connection.',
  },
  'servicegraph.request.error.count': {
    title: 'Error rate',
    meaning: 'Failed requests on this connection per second.',
  },
  'servicegraph.grpc.request.message.count': {
    title: 'gRPC request messages',
    meaning: 'gRPC request messages sent across this connection per second.',
  },
  'servicegraph.grpc.response.message.count': {
    title: 'gRPC response messages',
    meaning: 'gRPC response messages returned across this connection per second.',
  },
  'servicegraph.tcp.connection.open.count': {
    title: 'TCP connections opened',
    meaning: 'New TCP connections established per second.',
  },
  'servicegraph.tcp.connection.close.count': {
    title: 'TCP connections closed',
    meaning: 'TCP connections closed per second.',
  },
  'servicegraph.tcp.bytes.sent': {
    title: 'TCP data sent',
    meaning: 'Data sent across this connection per second.',
  },
  'servicegraph.tcp.bytes.received': {
    title: 'TCP data received',
    meaning: 'Data received across this connection per second.',
  },
}

const NODE_PRESENTATION: Record<string, Pick<MetricPresentation, 'title' | 'meaning'>> = {
  'k8s.node.cpu.usage': {
    title: 'CPU used',
    meaning: 'Direct CPU use across this node, reported by kubelet.',
  },
  'k8s.node.memory.working_set': {
    title: 'Working memory',
    meaning: 'Memory actively used by workloads on this node, reported by kubelet.',
  },
  'k8s.node.condition.ready': {
    title: 'Node readiness',
    meaning: 'Whether Kubernetes reports this node as ready to run workloads.',
  },
  'k8s.node.condition.memory_pressure': {
    title: 'Memory pressure',
    meaning: 'Whether Kubernetes reports memory pressure on this node.',
  },
  'k8s.node.condition.disk_pressure': {
    title: 'Disk pressure',
    meaning: 'Whether Kubernetes reports disk pressure on this node.',
  },
}

const POD_PRESENTATION: Record<string, Pick<MetricPresentation, 'title' | 'meaning'>> = {
  'k8s.pod.cpu.usage': {
    title: 'CPU used',
    meaning: 'Direct CPU use for this Pod, reported by kubelet.',
  },
  'k8s.pod.memory.working_set': {
    title: 'Working memory',
    meaning: 'Memory actively used by this Pod, reported by kubelet.',
  },
}

const CONTAINER_PRESENTATION: Record<string, Pick<MetricPresentation, 'title' | 'meaning'>> = {
  'k8s.container.cpu.usage': {
    title: 'CPU used',
    meaning: 'Direct CPU use for this Container, reported by kubelet.',
  },
  'k8s.container.cpu.request': {
    title: 'CPU request',
    meaning: 'CPU Kubernetes reserves for this Container when scheduling.',
  },
  'k8s.container.cpu.limit': {
    title: 'CPU limit',
    meaning: 'Maximum CPU Kubernetes allows this Container to use.',
  },
  'k8s.container.memory.usage': {
    title: 'Working memory',
    meaning: 'Memory currently used by this Container, reported by kubelet.',
  },
  'k8s.container.memory.request': {
    title: 'Memory request',
    meaning: 'Memory Kubernetes reserves for this Container when scheduling.',
  },
  'k8s.container.memory.limit': {
    title: 'Memory limit',
    meaning: 'Maximum memory Kubernetes allows this Container to use.',
  },
  'k8s.container.ready': {
    title: 'Container readiness',
    meaning: 'The direct readiness signal reported by Kubernetes for this Container.',
  },
  'k8s.container.restarts': {
    title: 'Restart count',
    meaning: 'Cumulative number of times Kubernetes restarted this Container.',
  },
}

function withLabel(title: string, value: string | undefined) {
  return value ? `${title} · ${value}` : title
}

function nodePresentation(canonicalName: string, labels?: Record<string, string>): Pick<MetricPresentation, 'title' | 'meaning'> | undefined {
  const staticPresentation = NODE_PRESENTATION[canonicalName]
  if (staticPresentation) return staticPresentation

  switch (canonicalName) {
    case 'system.cpu.utilization':
      return {
        title: withLabel(`CPU ${labels?.state ?? 'state'} time`, labels?.cpu),
        meaning: 'The share of time this CPU core spent in the named state.',
      }
    case 'system.memory.usage':
      return {
        title: withLabel('Memory', labels?.state),
        meaning: 'Host memory in the named state.',
      }
    case 'system.disk.io.read':
      return {
        title: withLabel('Disk read throughput', labels?.device),
        meaning: 'Bytes read from this disk per second over the selected window.',
      }
    case 'system.disk.io.write':
      return {
        title: withLabel('Disk write throughput', labels?.device),
        meaning: 'Bytes written to this disk per second over the selected window.',
      }
    case 'system.disk.operations':
      return {
        title: withLabel(`Disk ${labels?.direction ?? 'I/O'} operations`, labels?.device),
        meaning: 'Disk operations per second over the selected window.',
      }
    case 'system.filesystem.usage':
      return {
        title: withLabel(`Filesystem ${labels?.state ?? 'space'}`, labels?.mountpoint),
        meaning: 'Filesystem space in the named state.',
      }
    case 'system.filesystem.utilization':
      return {
        title: withLabel('Filesystem utilization', labels?.mountpoint),
        meaning: 'Percentage of space used on this filesystem.',
      }
    case 'system.network.io.receive':
      return {
        title: withLabel('Network received throughput', labels?.network_interface_name),
        meaning: 'Bytes received per second on this network interface over the selected window.',
      }
    case 'system.network.io.transmit':
      return {
        title: withLabel('Network sent throughput', labels?.network_interface_name),
        meaning: 'Bytes sent per second on this network interface over the selected window.',
      }
    case 'system.network.errors':
      return {
        title: withLabel(`Network ${labels?.direction === 'receive' ? 'received' : labels?.direction === 'transmit' ? 'transmitted' : labels?.direction ?? 'interface'} errors`, labels?.network_interface_name),
        meaning: 'Network interface errors per second over the selected window.',
      }
    default:
      return undefined
  }
}

function podPresentation(canonicalName: string): Pick<MetricPresentation, 'title' | 'meaning'> | undefined {
  return POD_PRESENTATION[canonicalName]
}

function containerPresentation(canonicalName: string): Pick<MetricPresentation, 'title' | 'meaning'> | undefined {
  return CONTAINER_PRESENTATION[canonicalName]
}

function httpStatus(labels?: Record<string, string>) {
  return labels?.['http.response.status_code'] ?? labels?.http_status_code
}

function titleForHTTPStatus(base: Pick<MetricPresentation, 'title' | 'meaning'>, status: string | undefined): Pick<MetricPresentation, 'title' | 'meaning'> {
  if (!status) return base
  const code = Number(status)
  const outcome = code >= 200 && code < 300
    ? { adjective: 'Successful', detail: `completed with a ${status} success status` }
    : code >= 400 && code < 500
      ? { adjective: 'Client error', detail: `ended with a ${status} client error` }
      : code >= 500 && code < 600
        ? { adjective: 'Server error', detail: 'ended with a 5xx server error' }
        : { adjective: `HTTP ${status}`, detail: `returned HTTP status ${status}` }

  if (base.title === 'Request rate') {
    return {
      title: `${outcome.adjective} request rate`,
      meaning: `Requests on this connection that ${outcome.detail}, per second.`,
    }
  }
  if (base.title === 'Response time') {
    return {
      title: `${outcome.adjective} response time`,
      meaning: `Mean response time for requests on this connection that ${outcome.detail}.`,
    }
  }
  if (base.title === 'Error rate') {
    return {
      title: code >= 200 && code < 300
        ? `HTTP ${status} error rate`
        : `${outcome.adjective} rate`,
      meaning: `Failed requests on this connection that ${outcome.detail}, per second.`,
    }
  }

  if (code >= 200 && code < 300) {
    return {
      title: `Successful ${base.title.toLowerCase()}`,
      meaning: `${base.meaning} HTTP status: ${status}.`,
    }
  }
  if (code >= 400 && code < 500) {
    return {
      title: `Client error ${base.title.toLowerCase()}`,
      meaning: `${base.meaning} HTTP status: ${status}.`,
    }
  }
  if (code >= 500 && code < 600) {
    return {
      title: `Server error ${base.title.toLowerCase()}`,
      meaning: `${base.meaning} HTTP status: ${status}.`,
    }
  }
  return {
    title: `HTTP ${status} ${base.title.toLowerCase()}`,
    meaning: `${base.meaning} HTTP status: ${status}.`,
  }
}

function displaySecurity(value: string) {
  return value === 'mutual_tls' ? 'mTLS' : value.replace(/_/g, ' ')
}

function serviceEntityPresentation(canonicalName: string, labels?: Record<string, string>): Pick<MetricPresentation, 'title' | 'meaning'> | undefined {
  const base = SERVICE_ENTITY_PRESENTATION[canonicalName]
  if (!base) return undefined

  const destination = canonicalName.startsWith('servicegraph.') ? labels?.edge_server : undefined
  const source = canonicalName.startsWith('http.server.') ? labels?.edge_client : undefined
  if (destination) return { ...base, title: `${base.title} · to ${destination}` }
  if (source) return { ...base, title: `${base.title} · from ${source}` }
  return base
}

function relevantServiceSeriesLabels(
  labels?: Record<string, string>,
  peer?: { label: 'Destination' | 'Source'; value?: string },
): SeriesLabel[] {
  if (!labels) return []
  const status = httpStatus(labels)
  const protocol = labels['network.protocol.name']
  const security = labels['mesh.security.policy']
  const responseFlags = labels['mesh.envoy.response_flags']
  const grpcStatus = labels['rpc.grpc.status_code']

  return [
    peer?.value && { label: peer.label, value: peer.value },
    status && { label: 'HTTP status', value: status },
    grpcStatus && { label: 'gRPC status', value: grpcStatus },
    protocol && { label: 'Protocol', value: protocol.toUpperCase() },
    security && { label: 'Transport security', value: displaySecurity(security) },
    responseFlags && responseFlags !== '-' && { label: 'Proxy response flags', value: responseFlags },
  ].filter((label): label is SeriesLabel => Boolean(label))
}

function relevantNodeSeriesLabels(canonicalName: string, labels?: Record<string, string>): SeriesLabel[] {
  if (!labels) return []
  const common = canonicalName === 'system.cpu.utilization'
    ? [
        labels.cpu && { label: 'CPU core', value: labels.cpu },
        labels.state && { label: 'CPU state', value: labels.state },
      ]
    : canonicalName === 'system.memory.usage'
      ? [labels.state && { label: 'Memory state', value: labels.state }]
      : canonicalName === 'system.filesystem.usage'
        ? [
            labels.mountpoint && { label: 'Mount point', value: labels.mountpoint },
            labels.state && { label: 'Filesystem state', value: labels.state },
            labels.device && { label: 'Device', value: labels.device },
            labels.filesystem_type && { label: 'Filesystem type', value: labels.filesystem_type },
            labels.filesystem_mode && { label: 'Filesystem mode', value: labels.filesystem_mode },
          ]
        : canonicalName === 'system.filesystem.utilization'
          ? [
              labels.mountpoint && { label: 'Mount point', value: labels.mountpoint },
              labels.device && { label: 'Device', value: labels.device },
              labels.filesystem_type && { label: 'Filesystem type', value: labels.filesystem_type },
              labels.filesystem_mode && { label: 'Filesystem mode', value: labels.filesystem_mode },
            ]
          : canonicalName.startsWith('system.disk.')
            ? [
                labels.direction && { label: 'Direction', value: labels.direction },
                labels.device && { label: 'Device', value: labels.device },
              ]
            : canonicalName.startsWith('system.network.')
              ? [
                  labels.direction && { label: 'Direction', value: labels.direction },
                  labels.network_interface_name && { label: 'Network interface', value: labels.network_interface_name },
                ]
              : [labels.resource && { label: 'Resource', value: labels.resource }]
  return common.filter((label): label is SeriesLabel => Boolean(label))
}

export function metricPresentation({ canonicalName, scope, labels, fallbackMeaning = '', fallbackTitle }: MetricPresentationInput): MetricPresentation {
  const base = scope === 'entity'
    ? nodePresentation(canonicalName, labels) ?? podPresentation(canonicalName) ?? containerPresentation(canonicalName) ?? serviceEntityPresentation(canonicalName, labels)
    : EDGE_PRESENTATION[canonicalName]
  if (!base) {
    return { title: fallbackTitle ?? canonicalName, meaning: fallbackMeaning, seriesLabels: relevantServiceSeriesLabels(labels) }
  }

  const withStatus = scope === 'edge' ? titleForHTTPStatus(base, httpStatus(labels)) : base
  const peer = scope === 'entity'
    ? canonicalName.startsWith('servicegraph.')
      ? { label: 'Destination' as const, value: labels?.edge_server }
      : canonicalName.startsWith('http.server.')
        ? { label: 'Source' as const, value: labels?.edge_client }
        : undefined
    : undefined
  return {
    ...withStatus,
    seriesLabels: scope === 'entity' && nodePresentation(canonicalName, labels)
      ? relevantNodeSeriesLabels(canonicalName, labels)
      : relevantServiceSeriesLabels(labels, peer),
  }
}

export function metricDetailRows({
  meaning,
  observedSource,
  query,
  seriesLabels,
}: {
  meaning: string
  observedSource?: string
  query: MetricQuery
  seriesLabels: SeriesLabel[]
}): MetricDetailRow[] {
  const aggregation = query.aggregation.replace(/_/g, ' ')
  const window = query.window ? ` · ${query.window} window` : ''
  const rows: MetricDetailRow[] = [
    { label: 'What this shows', value: meaning },
  ]
  if (observedSource) rows.push({ label: 'Observed source', value: observedSource })
  rows.push({ label: 'Query', value: `${aggregation}${window} · ${query.step} grid` })
  if (seriesLabels.length > 0) {
    rows.push({
      label: 'Series labels',
      value: seriesLabels.map(({ label, value }) => `${label}: ${value}`).join(' · '),
    })
  }
  return rows
}
