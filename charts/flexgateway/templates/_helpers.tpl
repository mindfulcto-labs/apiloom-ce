{{/*
Expand the name of the chart.
*/}}
{{- define "flexgateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "flexgateway.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "flexgateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "flexgateway.labels" -}}
helm.sh/chart: {{ include "flexgateway.chart" . }}
{{ include "flexgateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "flexgateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flexgateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "flexgateway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "flexgateway.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Database DSN - uses external if set, otherwise constructs from internal values
*/}}
{{- define "flexgateway.databaseDSN" -}}
{{- if .Values.postgresql.external.dsn }}
{{- .Values.postgresql.external.dsn }}
{{- else }}
{{- printf "postgres://%s:%s@%s-postgresql:5432/%s?sslmode=disable" .Values.postgresql.auth.username .Values.postgresql.auth.password (include "flexgateway.fullname" .) .Values.postgresql.auth.database }}
{{- end }}
{{- end }}
