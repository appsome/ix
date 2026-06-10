{{- define "app.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "app.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Worker identity. A distinct app.kubernetes.io/name keeps worker pods out of the
api Service's selector (which matches on name + instance only).
*/}}
{{- define "app.worker.fullname" -}}
{{- printf "%s-worker" (include "app.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "app.worker.selectorLabels" -}}
app.kubernetes.io/name: {{ printf "%s-worker" (include "app.name" .) | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "app.labels" -}}
app.kubernetes.io/name: {{ include "app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "app.selectorLabels" -}}
app.kubernetes.io/name: {{ include "app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
