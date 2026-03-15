{{- define "oksg-otel.name" -}}
{{- default .Chart.Name .Values.global.releaseName | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "oksg-otel.namespace" -}}
{{- default .Release.Namespace .Values.namespace.name -}}
{{- end -}}

{{- define "oksg-otel.labels" -}}
app.kubernetes.io/name: {{ include "oksg-otel.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
oksg.io/cluster: {{ .Values.global.clusterName | quote }}
{{- end -}}

{{- define "oksg-otel.componentLabels" -}}
{{ include "oksg-otel.labels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "oksg-otel.image" -}}
{{ printf "%s:%s" .Values.global.image.repository .Values.global.image.tag }}
{{- end -}}

{{- define "oksg-otel.backendImage" -}}
{{ printf "%s:%s" .Values.backend.image.repository .Values.backend.image.tag }}
{{- end -}}

{{- define "oksg-otel.agentChecksum" -}}
{{- .Values.agent.collectorConfig | sha256sum -}}
{{- end -}}

{{- define "oksg-otel.gatewayChecksum" -}}
{{- .Values.gateway.collectorConfig | sha256sum -}}
{{- end -}}
