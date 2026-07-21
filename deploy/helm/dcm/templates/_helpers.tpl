{{/*
Expand the name of the chart.
*/}}
{{- define "dcm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fullname helper.
*/}}
{{- define "dcm.fullname" -}}
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
Common labels.
*/}}
{{- define "dcm.labels" -}}
helm.sh/chart: {{ include "dcm.name" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: dcm
{{- end }}

{{/*
Selector labels for a component.
Usage: {{ include "dcm.selectorLabels" (dict "context" . "component" "control-plane") }}
*/}}
{{- define "dcm.selectorLabels" -}}
app.kubernetes.io/name: {{ .component }}
app.kubernetes.io/instance: {{ .context.Release.Name }}
{{- end }}

{{/*
Resolve image tag: per-component tag > global.imageTag > "main"
Usage: {{ include "dcm.imageTag" (dict "tag" .Values.controlPlane.tag "global" .Values.global) }}
*/}}
{{- define "dcm.imageTag" -}}
{{- default (default "main" .global.imageTag) .tag }}
{{- end }}

{{/*
Init container that waits for postgres to be ready.
Usage: {{ include "dcm.waitForPostgres" . | nindent 8 }}
*/}}
{{- define "dcm.waitForPostgres" -}}
- name: wait-for-postgres
  image: {{ .Values.postgres.image }}
  command: ["sh", "-c", "until pg_isready -h {{ include "dcm.fullname" . }}-postgres -p 5432 -U {{ .Values.postgres.user }}; do echo 'Waiting for postgres...'; sleep 2; done"]
  securityContext:
    runAsNonRoot: true
    allowPrivilegeEscalation: false
    capabilities:
      drop:
        - ALL
    seccompProfile:
      type: RuntimeDefault
{{- end }}
