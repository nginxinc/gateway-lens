{{/*
Chart name.
*/}}
{{- define "gateway-lens.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified app name.
*/}}
{{- define "gateway-lens.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "gateway-lens.labels" -}}
app.kubernetes.io/name: {{ include "gateway-lens.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{/*
Selector labels.
*/}}
{{- define "gateway-lens.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gateway-lens.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Service account name.
*/}}
{{- define "gateway-lens.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "gateway-lens.fullname" .) .Values.serviceAccount.name -}}
{{- else if .Values.serviceAccount.name -}}
{{- .Values.serviceAccount.name -}}
{{- else -}}
{{- fail "gateway-lens: serviceAccount.name must be set when serviceAccount.create is false" -}}
{{- end -}}
{{- end -}}

{{/*
ClusterRole/Binding names.
*/}}
{{- define "gateway-lens.clusterScopedName" -}}
{{- $full := printf "%s-%s" .Release.Namespace (include "gateway-lens.fullname" .) -}}
{{- if gt (len $full) 63 -}}
{{- printf "%s-%s" ($full | trunc 54 | trimSuffix "-") ($full | sha256sum | trunc 8) -}}
{{- else -}}
{{- $full | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
Container args built from the explicit values fields, plus any extraArgs appended.
*/}}
{{- define "gateway-lens.args" -}}
- --port={{ .Values.port }}
- --log-level={{ .Values.logLevel }}
{{- if .Values.namespaces }}
- --namespaces={{ join "," .Values.namespaces }}
{{- end }}
{{- if .Values.basePath }}
- --base-path={{ .Values.basePath }}
{{- end }}
{{- range .Values.extraArgs }}
- {{ . | quote }}
{{- end }}
{{- end -}}
