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
Cluster-scoped resource name (includes namespace to avoid collisions).
*/}}
{{- define "dcm.clusterResourceName" -}}
{{- printf "%s-%s" (include "dcm.fullname" .) .Release.Namespace | trunc 63 | trimSuffix "-" }}
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
  envFrom:
    - secretRef:
        name: {{ include "dcm.dbSecretName" . }}
  command: ["sh", "-c", "until pg_isready -h {{ include "dcm.fullname" . }}-postgres -p 5432 -U \"$POSTGRES_USER\"; do echo 'Waiting for postgres...'; sleep 2; done"]
  securityContext:
    runAsNonRoot: true
    allowPrivilegeEscalation: false
    capabilities:
      drop:
        - ALL
    seccompProfile:
      type: RuntimeDefault
{{- end }}


{{/*
Database credentials Secret name (pre-created in release namespace).
*/}}
{{- define "dcm.dbSecretName" -}}
{{- if not .Values.postgres.dbSecretRef -}}
{{- fail "postgres.dbSecretRef is required" -}}
{{- end -}}
{{- .Values.postgres.dbSecretRef -}}
{{- end }}

{{/*
Auth credentials Secret name (pre-created in release namespace).
*/}}
{{- define "dcm.authSecretName" -}}
{{- if not .Values.auth.authSecretRef -}}
{{- fail "auth.authSecretRef is required when auth.enabled=true" -}}
{{- end -}}
{{- .Values.auth.authSecretRef -}}
{{- end }}

{{/*
Resolve AUTH_ISSUER_URL: explicit value or in-cluster Keycloak service URL.
*/}}
{{- define "dcm.authIssuerURL" -}}
{{- if .Values.auth.issuerURL -}}
{{- if not (hasSuffix "/realms/dcm" .Values.auth.issuerURL) -}}
{{- fail "auth.issuerURL must end with /realms/dcm" -}}
{{- end -}}
{{- .Values.auth.issuerURL -}}
{{- else -}}
{{- printf "http://%s-keycloak:8080/realms/dcm" (include "dcm.fullname" .) -}}
{{- end -}}
{{- end }}

{{/*
Keycloak KC_HOSTNAME (must match token issuer host). Derived from issuerURL when set.
*/}}
{{- define "dcm.keycloakHostname" -}}
{{- $issuer := include "dcm.authIssuerURL" . -}}
{{- trimSuffix "/realms/dcm" $issuer -}}
{{- end }}

{{/*
Init container that waits for Keycloak readiness (auth.enabled only).
Uses HTTP health check from compose.yaml healthcheck pattern (/dev/tcp requires bash).
Usage: {{ include "dcm.waitForKeycloak" . | nindent 8 }}
*/}}
{{- define "dcm.waitForKeycloak" -}}
- name: wait-for-keycloak
  image: {{ .Values.auth.keycloak.image }}
  command:
    - /bin/bash
    - -c
    - |
      max_attempts=150
      attempt=0
      until [ $attempt -ge $max_attempts ] || (exec 3<>/dev/tcp/{{ include "dcm.fullname" . }}-keycloak/9000 && echo -e 'GET /health/ready HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n' >&3 && cat <&3 | grep -q '"status": "UP"' && exec 3>&-); do
        attempt=$((attempt+1))
        echo "Waiting for keycloak... ($attempt/$max_attempts)"
        sleep 2
      done
      if [ $attempt -ge $max_attempts ]; then
        echo "Keycloak failed to become ready"
        exit 1
      fi
  securityContext:
    runAsNonRoot: true
    allowPrivilegeEscalation: false
    capabilities:
      drop:
        - ALL
    seccompProfile:
      type: RuntimeDefault
{{- end }}
