{{- define "oksg-otel.name" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "oksg-otel.namespace" -}}
{{- .Release.Namespace -}}
{{- end -}}

{{- define "oksg-otel.agentName" -}}
{{- printf "%s-agent" (include "oksg-otel.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "oksg-otel.gatewayName" -}}
{{- printf "%s-gateway" (include "oksg-otel.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "oksg-otel.backendName" -}}
{{- printf "%s-backend" (include "oksg-otel.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "oksg-otel.frontendName" -}}
{{- printf "%s-frontend" (include "oksg-otel.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "oksg-otel.frontendConfigName" -}}
{{- printf "%s-frontend-nginx" (include "oksg-otel.name" .) | trunc 63 | trimSuffix "-" -}}
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

{{- define "oksg-otel.sourceContractName" -}}
{{- printf "%s-source-contract" (include "oksg-otel.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "oksg-otel.frontendImage" -}}
{{ printf "%s:%s" .Values.frontend.image.repository .Values.frontend.image.tag }}
{{- end -}}

{{- define "oksg-otel.gatewayChecksum" -}}
{{- .Values.gateway.collectorConfig | sha256sum -}}
{{- end -}}

{{- define "oksg-otel.agentChecksum" -}}
{{- .Values.agent.collectorConfig | sha256sum -}}
{{- end -}}

{{/*
PostgreSQL DSN: use explicit dsn if set, otherwise build from subchart service name.
Bitnami postgresql subchart creates a service named <release>-postgresql.
*/}}
{{- define "oksg-otel.postgresDSN" -}}
{{- if .Values.storage.postgresql.dsn -}}
{{ .Values.storage.postgresql.dsn }}
{{- else -}}
{{ printf "postgres://%s:%s@%s-postgresql:5432/%s?sslmode=disable" (index .Values "postgresql" "auth" "username") (index .Values "postgresql" "auth" "password") .Release.Name (index .Values "postgresql" "auth" "database") }}
{{- end -}}
{{- end -}}

{{/*
VictoriaMetrics URL: use explicit url if set, otherwise derive from subchart service name.
VM single subchart creates a service named <release>-victoria-metrics-single-server.
*/}}
{{- define "oksg-otel.vmURL" -}}
{{- if .Values.storage.victoriametrics.url -}}
{{ .Values.storage.victoriametrics.url }}
{{- else -}}
{{ printf "http://%s-victoria-metrics-single-server:8428" .Release.Name }}
{{- end -}}
{{- end -}}
