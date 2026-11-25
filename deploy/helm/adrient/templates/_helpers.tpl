{{/* Chart name */}}
{{- define "adrient.name" -}}
adrient
{{- end -}}

{{/* Common labels */}}
{{- define "adrient.labels" -}}
app.kubernetes.io/name: adrient
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{/* adrient app image ref */}}
{{- define "adrient.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{/* Public external URL for adrient (scheme + domain) */}}
{{- define "adrient.externalURL" -}}
{{- printf "%s://%s" .Values.scheme .Values.domain -}}
{{- end -}}

{{/* In-cluster Postgres host */}}
{{- define "adrient.pgHost" -}}
{{ .Release.Name }}-postgres.{{ .Release.Namespace }}.svc.cluster.local
{{- end -}}

{{/* adrient Postgres DSN. Uses externalDsn when set, else the bundled PG. */}}
{{- define "adrient.pgDSN" -}}
{{- if .Values.postgres.externalDsn -}}
{{ .Values.postgres.externalDsn }}
{{- else -}}
postgres://{{ .Values.postgres.auth.username }}:{{ .Values.postgres.auth.password }}@{{ include "adrient.pgHost" . }}:5432/{{ .Values.postgres.auth.database }}?sslmode=disable
{{- end -}}
{{- end -}}

{{/* PDF Postgres DSN */}}
{{- define "adrient.pdfDSN" -}}
postgres://{{ .Values.postgres.auth.username }}:{{ .Values.postgres.auth.password }}@{{ include "adrient.pgHost" . }}:5432/{{ .Values.pdf.database }}?sslmode=disable
{{- end -}}

{{/* ---------------------------------------------------------------------
     Object storage (S3). 0.2.0 replaced the bundled MinIO with rustfs.
     --------------------------------------------------------------------- */}}

{{/* Bundled rustfs Service name. */}}
{{- define "adrient.rustfsName" -}}
{{ .Release.Name }}-rustfs
{{- end -}}

{{/* Bundled (legacy) MinIO Service name. */}}
{{- define "adrient.minioName" -}}
{{ .Release.Name }}-minio
{{- end -}}

{{/* adrient's app bucket. rustfs.bucket wins; else the 0.1.x minio.bucket so an
     upgraded install keeps its bucket name; else adrient-default. */}}
{{- define "adrient.bucket" -}}
{{- .Values.rustfs.bucket | default .Values.minio.bucket | default "adrient-default" -}}
{{- end -}}

{{/* rustfs PVC size: rustfs.persistence.size, else the 0.1.x
     minio.persistence.size, else 20Gi. */}}
{{- define "adrient.rustfsSize" -}}
{{- .Values.rustfs.persistence.size | default .Values.minio.persistence.size | default "20Gi" -}}
{{- end -}}

{{/* rustfs PVC storageClass: rustfs.persistence.storageClass, else global. */}}
{{- define "adrient.rustfsStorageClass" -}}
{{- .Values.rustfs.persistence.storageClass | default .Values.storageClass -}}
{{- end -}}

{{/* S3 endpoint adrient uses: bundled rustfs Service, or an external endpoint
     (rustfs.external.endpoint, falling back to the 0.1.x
     minio.external.endpoint). */}}
{{- define "adrient.s3Endpoint" -}}
{{- if .Values.rustfs.enabled -}}
http://{{ include "adrient.rustfsName" . }}.{{ .Release.Namespace }}.svc.cluster.local:{{ .Values.rustfs.port }}
{{- else -}}
{{ .Values.rustfs.external.endpoint | default .Values.minio.external.endpoint }}
{{- end -}}
{{- end -}}

{{/* Secret holding the S3 credentials (keys access_key / secret_key) that
     adrient, the pdf app, rustfs itself and the storage Jobs read.
       - rustfs.existingSecret, when set
       - bundled rustfs: {release}-rustfs (generated once, lookup-preserved)
       - external S3:   {release}-s3 (from rustfs.external / minio keys) */}}
{{- define "adrient.s3CredsSecret" -}}
{{- if .Values.rustfs.existingSecret -}}
{{ .Values.rustfs.existingSecret }}
{{- else if .Values.rustfs.enabled -}}
{{ include "adrient.rustfsName" . }}
{{- else -}}
{{ .Release.Name }}-s3
{{- end -}}
{{- end -}}

{{/* Buckets the chart manages, as "src:dst" pairs for the MinIO -> rustfs
     migration (the app bucket keeps its MinIO name as the source). */}}
{{- define "adrient.bucketPairs" -}}
{{- $pairs := list (printf "%s:%s" (.Values.minio.bucket | default (include "adrient.bucket" .)) (include "adrient.bucket" .)) -}}
{{- if .Values.pdf.enabled -}}
{{- $pairs = append $pairs (printf "%s:%s" .Values.pdf.bucket .Values.pdf.bucket) -}}
{{- end -}}
{{- join " " $pairs -}}
{{- end -}}

{{/* In-cluster Zitadel issuer used by adrient (server-to-server). */}}
{{- define "adrient.zitadelInternalURL" -}}
http://{{ .Release.Name }}-zitadel.{{ .Release.Namespace }}.svc.cluster.local:8080
{{- end -}}

{{/* Browser-facing issuer URL for the bundled Zitadel, derived from
     zitadel.externalDomain (+ externalSecure/externalPort). Empty when no
     external domain is configured. */}}
{{- define "adrient.zitadelExternalURL" -}}
{{- if .Values.zitadel.externalDomain -}}
{{- $scheme := ternary "https" "http" (.Values.zitadel.externalSecure | toString | eq "true") -}}
{{- $port := .Values.zitadel.externalPort | toString -}}
{{- if or (and (eq $scheme "https") (eq $port "443")) (and (eq $scheme "http") (eq $port "80")) -}}
{{- printf "%s://%s" $scheme .Values.zitadel.externalDomain -}}
{{- else -}}
{{- printf "%s://%s:%s" $scheme .Values.zitadel.externalDomain $port -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* OIDC issuer handed to adrient.
     - auth.mode=external           -> auth.external.issuer
     - bundled + zitadel.externalDomain set -> the browser-facing Zitadel URL
       (so the OIDC redirect to the issuer is reachable from the user's browser)
     - bundled, no externalDomain   -> in-cluster Zitadel Service URL (kind/local) */}}
{{- define "adrient.oidcIssuer" -}}
{{- if eq .Values.auth.mode "external" -}}
{{ .Values.auth.external.issuer }}
{{- else if .Values.zitadel.externalDomain -}}
{{ include "adrient.zitadelExternalURL" . }}
{{- else -}}
{{ include "adrient.zitadelInternalURL" . }}
{{- end -}}
{{- end -}}

{{/* OIDC redirect URL (public) */}}
{{- define "adrient.oidcRedirectURL" -}}
{{ include "adrient.externalURL" . }}/api/v1/auth/callback
{{- end -}}

{{/* Resolved image pull secret name (existing, created, or empty) */}}
{{- define "adrient.pullSecretName" -}}
{{- if .Values.imagePullSecrets.existingSecret -}}
{{ .Values.imagePullSecrets.existingSecret }}
{{- else if .Values.imagePullSecrets.create.enabled -}}
{{ .Values.imagePullSecrets.create.name }}
{{- end -}}
{{- end -}}

{{/* ---------------------------------------------------------------------
     Home Assistant (optional, bring-your-own is the primary path).
     --------------------------------------------------------------------- */}}

{{/* Home Assistant resource name. */}}
{{- define "adrient.haName" -}}
{{ .Release.Name }}-homeassistant
{{- end -}}

{{/* Public hostname Home Assistant is served on: homeAssistant.host, else
     ha.<domain>. HA can't live under a sub-path, so it gets its own host. */}}
{{- define "adrient.haHost" -}}
{{- .Values.homeAssistant.host | default (printf "ha.%s" .Values.domain) -}}
{{- end -}}

{{/* URL adrient's Home Assistant tile defaults to (ADRIENT_HOMEASSISTANT_URL):
     homeAssistant.url, else <scheme>://<haHost>. */}}
{{- define "adrient.haURL" -}}
{{- .Values.homeAssistant.url | default (printf "%s://%s" .Values.scheme (include "adrient.haHost" .)) -}}
{{- end -}}

{{/* Secret holding the HA owner's username/password (onboarding sidecar):
     homeAssistant.owner.existingSecret, else {release}-homeassistant-owner
     (generated once, lookup-preserved). */}}
{{- define "adrient.haOwnerSecret" -}}
{{- .Values.homeAssistant.owner.existingSecret | default (printf "%s-owner" (include "adrient.haName" .)) -}}
{{- end -}}

{{/* In-cluster URL of the adrient HA integration bundle (the HA pod fetches it
     on every start). */}}
{{- define "adrient.haIntegrationURL" -}}
{{- .Values.homeAssistant.adrientIntegration.url | default (printf "http://%s-adrient.%s.svc.cluster.local:%v/integrations/homeassistant/adrient.zip" .Release.Name .Release.Namespace .Values.adrient.service.httpPort) -}}
{{- end -}}
