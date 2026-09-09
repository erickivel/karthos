{{- define "karthos.name" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "karthos.namespace" -}}
{{- .Release.Namespace -}}
{{- end -}}

{{- define "karthos.agentName" -}}
{{- printf "%s-agent" (include "karthos.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "karthos.gatewayName" -}}
{{- printf "%s-gateway" (include "karthos.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "karthos.backendName" -}}
{{- printf "%s-backend" (include "karthos.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "karthos.frontendName" -}}
{{- printf "%s-frontend" (include "karthos.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "karthos.frontendConfigName" -}}
{{- printf "%s-frontend-nginx" (include "karthos.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "karthos.labels" -}}
app.kubernetes.io/name: {{ include "karthos.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
karthos.io/cluster: {{ .Values.global.clusterName | quote }}
{{- end -}}

{{- define "karthos.componentLabels" -}}
{{ include "karthos.labels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "karthos.image" -}}
{{ printf "%s:%s" .Values.global.image.repository .Values.global.image.tag }}
{{- end -}}

{{- define "karthos.backendImage" -}}
{{ printf "%s:%s" .Values.backend.image.repository .Values.backend.image.tag }}
{{- end -}}

{{- define "karthos.sourceContractName" -}}
{{- printf "%s-source-contract" (include "karthos.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "karthos.frontendImage" -}}
{{ printf "%s:%s" .Values.frontend.image.repository .Values.frontend.image.tag }}
{{- end -}}

{{- define "karthos.gatewayChecksum" -}}
{{- .Values.gateway.collectorConfig | sha256sum -}}
{{- end -}}

{{- define "karthos.agentChecksum" -}}
{{- .Values.agent.collectorConfig | sha256sum -}}
{{- end -}}

{{/*
PostgreSQL DSN: use explicit dsn if set, otherwise build from subchart service name.
Bitnami postgresql subchart creates a service named <release>-postgresql.
*/}}
{{- define "karthos.postgresDSN" -}}
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
{{- define "karthos.vmURL" -}}
{{- if .Values.storage.victoriametrics.url -}}
{{ .Values.storage.victoriametrics.url }}
{{- else -}}
{{ printf "http://%s-victoria-metrics-single-server:8428" .Release.Name }}
{{- end -}}
{{- end -}}
