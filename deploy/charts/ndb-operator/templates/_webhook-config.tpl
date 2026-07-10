{{- /*
Name of the webhook service.
Used by the service, deployment and the validating webhook.
*/}}
{{- define "webhook-service.name" -}}
{{.Release.Name}}-webhook-service
{{- end -}}

{{- /*
Port used by the webhook service.
Used by the service, deployment and the validating webhook.
*/}}
{{- define "webhook-service.port" -}}
9443
{{- end -}}

{{- /*
Label applied to the webhook server pods.
Used by the service and deployment.
*/}}
{{- define "webhook-service.pod-label" -}}
app: ndb-operator-webhook-server
{{- end -}}

{{- /*
Namespace watched by a namespace-scoped operator. This must also be used by
the admission webhook namespace selectors.
*/}}
{{- define "ndb-operator.watch-namespace" -}}
{{- default .Release.Namespace .Values.watchNamespace -}}
{{- end -}}
