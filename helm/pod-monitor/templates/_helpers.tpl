{{/*
Common labels
*/}}
{{- define "pod-monitor.labels" -}}
app.kubernetes.io/name: pod-monitor
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Backend image
*/}}
{{- define "pod-monitor.backend.image" -}}
{{- if .Values.image.registry -}}
{{ .Values.image.registry }}/{{ .Values.backend.image.repository }}:{{ .Values.backend.image.tag }}
{{- else -}}
{{ .Values.backend.image.repository }}:{{ .Values.backend.image.tag }}
{{- end -}}
{{- end }}

{{/*
Frontend image
*/}}
{{- define "pod-monitor.frontend.image" -}}
{{- if .Values.image.registry -}}
{{ .Values.image.registry }}/{{ .Values.frontend.image.repository }}:{{ .Values.frontend.image.tag }}
{{- else -}}
{{ .Values.frontend.image.repository }}:{{ .Values.frontend.image.tag }}
{{- end -}}
{{- end }}
