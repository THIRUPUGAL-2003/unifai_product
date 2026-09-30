{{- define "unifai.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "unifai.fullname" -}}
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

{{- define "unifai.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "unifai.labels" -}}
helm.sh/chart: {{ include "unifai.chart" . }}
{{ include "unifai.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "unifai.selectorLabels" -}}
app.kubernetes.io/name: {{ include "unifai.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "unifai.serverSelectorLabels" -}}
{{ include "unifai.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{- define "unifai.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "unifai.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "unifai.postgresql.host" -}}
{{- if .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.host }}
{{- else }}
{{- printf "%s-postgresql" (include "unifai.fullname" .) }}
{{- end }}
{{- end }}

{{- define "unifai.postgresql.port" -}}
{{- if .Values.postgresql.external.enabled -}}
{{- .Values.postgresql.external.port -}}
{{- else -}}
5432
{{- end -}}
{{- end -}}

{{- define "unifai.postgresql.database" -}}
{{- if .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.database }}
{{- else }}
{{- .Values.postgresql.auth.database }}
{{- end }}
{{- end }}

{{- define "unifai.postgresql.username" -}}
{{- if .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.user }}
{{- else }}
{{- .Values.postgresql.auth.username }}
{{- end }}
{{- end }}

{{- define "unifai.postgresql.password" -}}
{{- if .Values.postgresql.external.enabled -}}
{{- if .Values.postgresql.external.existingSecret -}}
env.UNIFAI_POSTGRES_PASSWORD
{{- else -}}
{{- .Values.postgresql.external.password -}}
{{- end -}}
{{- else -}}
{{- if .Values.postgresql.auth.existingSecret -}}
env.UNIFAI_POSTGRES_PASSWORD
{{- else -}}
{{- .Values.postgresql.auth.password -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "unifai.postgresql.sslMode" -}}
{{- if .Values.postgresql.external.enabled -}}
{{- .Values.postgresql.external.sslMode -}}
{{- else -}}
disable
{{- end -}}
{{- end -}}

{{- define "unifai.weaviate.host" -}}
{{- if .Values.vectorStore.weaviate.external.enabled }}
{{- .Values.vectorStore.weaviate.external.host }}
{{- else }}
{{- printf "%s-weaviate" (include "unifai.fullname" .) }}
{{- end }}
{{- end }}

{{- define "unifai.weaviate.scheme" -}}
{{- if .Values.vectorStore.weaviate.external.enabled -}}
{{- .Values.vectorStore.weaviate.external.scheme -}}
{{- else -}}
http
{{- end -}}
{{- end -}}

{{- define "unifai.weaviate.apiKey" -}}
{{- if .Values.vectorStore.weaviate.external.enabled -}}
{{- if .Values.vectorStore.weaviate.external.existingSecret -}}
env.UNIFAI_WEAVIATE_API_KEY
{{- else -}}
{{- .Values.vectorStore.weaviate.external.apiKey -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "unifai.redis.host" -}}
{{- if .Values.vectorStore.redis.external.enabled }}
{{- .Values.vectorStore.redis.external.host }}
{{- else }}
{{- printf "%s-redis-master" (include "unifai.fullname" .) }}
{{- end }}
{{- end }}

{{- define "unifai.redis.port" -}}
{{- if .Values.vectorStore.redis.external.enabled -}}
{{- .Values.vectorStore.redis.external.port -}}
{{- else -}}
6379
{{- end -}}
{{- end -}}

{{- define "unifai.redis.password" -}}
{{- if .Values.vectorStore.redis.external.enabled -}}
{{- if .Values.vectorStore.redis.external.existingSecret -}}
env.UNIFAI_REDIS_PASSWORD
{{- else -}}
{{- .Values.vectorStore.redis.external.password -}}
{{- end -}}
{{- else -}}
{{- .Values.vectorStore.redis.auth.password -}}
{{- end -}}
{{- end -}}

{{- define "unifai.qdrant.host" -}}
{{- if .Values.vectorStore.qdrant.external.enabled }}
{{- .Values.vectorStore.qdrant.external.host }}
{{- else }}
{{- printf "%s-qdrant" (include "unifai.fullname" .) }}
{{- end }}
{{- end }}

{{- define "unifai.qdrant.port" -}}
{{- if .Values.vectorStore.qdrant.external.enabled -}}
{{- .Values.vectorStore.qdrant.external.port -}}
{{- else -}}
6334
{{- end -}}
{{- end -}}

{{- define "unifai.qdrant.apiKey" -}}
{{- if .Values.vectorStore.qdrant.external.enabled -}}
{{- if .Values.vectorStore.qdrant.external.existingSecret -}}
env.UNIFAI_QDRANT_API_KEY
{{- else -}}
{{- .Values.vectorStore.qdrant.external.apiKey -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "unifai.pinecone.apiKey" -}}
{{- if .Values.vectorStore.pinecone.external.enabled -}}
{{- if .Values.vectorStore.pinecone.external.existingSecret -}}
env.UNIFAI_PINECONE_API_KEY
{{- else -}}
{{- .Values.vectorStore.pinecone.external.apiKey -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "unifai.qdrant.useTls" -}}
{{- if .Values.vectorStore.qdrant.external.enabled -}}
{{- .Values.vectorStore.qdrant.external.useTls -}}
{{- else -}}
false
{{- end -}}
{{- end -}}

{{- define "unifai.config" -}}
{{- $config := dict "$schema" "https://www.getunifai.ai/schema" }}
{{- if .Values.unifai.sourceOfTruth }}
{{- $_ := set $config "source_of_truth" .Values.unifai.sourceOfTruth }}
{{- end }}
{{- if .Values.unifai.encryptionKey }}
{{- $_ := set $config "encryption_key" .Values.unifai.encryptionKey }}
{{- end }}
{{- if .Values.unifai.envLabel }}
{{- $_ := set $config "env_label" .Values.unifai.envLabel }}
{{- end }}
{{- if .Values.unifai.client }}
{{- $client := dict }}
{{- if hasKey .Values.unifai.client "dropExcessRequests" }}
{{- $_ := set $client "drop_excess_requests" .Values.unifai.client.dropExcessRequests }}
{{- end }}
{{- if .Values.unifai.client.initialPoolSize }}
{{- $_ := set $client "initial_pool_size" .Values.unifai.client.initialPoolSize }}
{{- end }}
{{- if .Values.unifai.client.allowedOrigins }}
{{- $_ := set $client "allowed_origins" .Values.unifai.client.allowedOrigins }}
{{- end }}
{{- if hasKey .Values.unifai.client "enableLogging" }}
{{- $_ := set $client "enable_logging" .Values.unifai.client.enableLogging }}
{{- end }}
{{- if hasKey .Values.unifai.client "enforceAuthOnInference" }}
{{- $_ := set $client "enforce_auth_on_inference" .Values.unifai.client.enforceAuthOnInference }}
{{- end }}
{{- if hasKey .Values.unifai.client "enforceGovernanceHeader" }}
{{- $_ := set $client "enforce_governance_header" .Values.unifai.client.enforceGovernanceHeader }}
{{- end }}
{{- if .Values.unifai.client.maxRequestBodySizeMb }}
{{- $_ := set $client "max_request_body_size_mb" .Values.unifai.client.maxRequestBodySizeMb }}
{{- end }}
{{- if .Values.unifai.client.compat }}
{{- $compat := dict }}
{{- if hasKey .Values.unifai.client.compat "convertTextToChat" }}
{{- $_ := set $compat "convert_text_to_chat" .Values.unifai.client.compat.convertTextToChat }}
{{- end }}
{{- if hasKey .Values.unifai.client.compat "convertChatToResponses" }}
{{- $_ := set $compat "convert_chat_to_responses" .Values.unifai.client.compat.convertChatToResponses }}
{{- end }}
{{- if hasKey .Values.unifai.client.compat "shouldDropParams" }}
{{- $_ := set $compat "should_drop_params" .Values.unifai.client.compat.shouldDropParams }}
{{- end }}
{{- if hasKey .Values.unifai.client.compat "shouldConvertParams" }}
{{- $_ := set $compat "should_convert_params" .Values.unifai.client.compat.shouldConvertParams }}
{{- end }}
{{- $_ := set $client "compat" $compat }}
{{- end }}
{{- if .Values.unifai.client.prometheusLabels }}
{{- $_ := set $client "prometheus_labels" .Values.unifai.client.prometheusLabels }}
{{- end }}
{{- if hasKey .Values.unifai.client "disableContentLogging" }}
{{- $_ := set $client "disable_content_logging" .Values.unifai.client.disableContentLogging }}
{{- end }}
{{- if hasKey .Values.unifai.client "allowPerRequestContentStorageOverride" }}
{{- $_ := set $client "allow_per_request_content_storage_override" .Values.unifai.client.allowPerRequestContentStorageOverride }}
{{- end }}
{{- if hasKey .Values.unifai.client "allowPerRequestRawOverride" }}
{{- $_ := set $client "allow_per_request_raw_override" .Values.unifai.client.allowPerRequestRawOverride }}
{{- end }}
{{- if .Values.unifai.client.logRetentionDays }}
{{- $_ := set $client "log_retention_days" .Values.unifai.client.logRetentionDays }}
{{- end }}
{{- if hasKey .Values.unifai.client "disableDbPingsInHealth" }}
{{- $_ := set $client "disable_db_pings_in_health" .Values.unifai.client.disableDbPingsInHealth }}
{{- end }}
{{- if hasKey .Values.unifai.client "dumpErrorsInConsoleLogs" }}
{{- $_ := set $client "dump_errors_in_console_logs" .Values.unifai.client.dumpErrorsInConsoleLogs }}
{{- end }}
{{- if .Values.unifai.client.headerFilterConfig }}
{{- $headerFilter := dict }}
{{- if .Values.unifai.client.headerFilterConfig.allowlist }}
{{- $_ := set $headerFilter "allowlist" .Values.unifai.client.headerFilterConfig.allowlist }}
{{- end }}
{{- if .Values.unifai.client.headerFilterConfig.denylist }}
{{- $_ := set $headerFilter "denylist" .Values.unifai.client.headerFilterConfig.denylist }}
{{- end }}
{{- if or $headerFilter.allowlist $headerFilter.denylist }}
{{- $_ := set $client "header_filter_config" $headerFilter }}
{{- end }}
{{- end }}
{{- if .Values.unifai.client.asyncJobResultTTL }}
{{- $_ := set $client "async_job_result_ttl" .Values.unifai.client.asyncJobResultTTL }}
{{- end }}
{{- if .Values.unifai.client.requiredHeaders }}
{{- $_ := set $client "required_headers" .Values.unifai.client.requiredHeaders }}
{{- end }}
{{- if .Values.unifai.client.loggingHeaders }}
{{- $_ := set $client "logging_headers" .Values.unifai.client.loggingHeaders }}
{{- end }}
{{- if .Values.unifai.client.whitelistedRoutes }}
{{- $_ := set $client "whitelisted_routes" .Values.unifai.client.whitelistedRoutes }}
{{- end }}
{{- if .Values.unifai.client.allowedHeaders }}
{{- $_ := set $client "allowed_headers" .Values.unifai.client.allowedHeaders }}
{{- end }}
{{- if .Values.unifai.client.mcpAgentDepth }}
{{- $_ := set $client "mcp_agent_depth" .Values.unifai.client.mcpAgentDepth }}
{{- end }}
{{- if .Values.unifai.client.mcpToolExecutionTimeout }}
{{- $_ := set $client "mcp_tool_execution_timeout" .Values.unifai.client.mcpToolExecutionTimeout }}
{{- end }}
{{- if .Values.unifai.client.mcpCodeModeBindingLevel }}
{{- $_ := set $client "mcp_code_mode_binding_level" .Values.unifai.client.mcpCodeModeBindingLevel }}
{{- end }}
{{- if hasKey .Values.unifai.client "mcpToolSyncInterval" }}
{{- $_ := set $client "mcp_tool_sync_interval" .Values.unifai.client.mcpToolSyncInterval }}
{{- end }}
{{- if hasKey .Values.unifai.client "hideDeletedVirtualKeysInFilters" }}
{{- $_ := set $client "hide_deleted_virtual_keys_in_filters" .Values.unifai.client.hideDeletedVirtualKeysInFilters }}
{{- end }}
{{- if hasKey .Values.unifai.client "mcpDisableAutoToolInject" }}
{{- $_ := set $client "mcp_disable_auto_tool_inject" .Values.unifai.client.mcpDisableAutoToolInject }}
{{- end }}
{{- if hasKey .Values.unifai.client "mcpEnableTempTokenAuth" }}
{{- $_ := set $client "mcp_enable_temp_token_auth" .Values.unifai.client.mcpEnableTempTokenAuth }}
{{- end }}
{{- if .Values.unifai.client.routingChainMaxDepth }}
{{- $_ := set $client "routing_chain_max_depth" .Values.unifai.client.routingChainMaxDepth }}
{{- end }}
{{- if hasKey .Values.unifai.client "allowDirectKeys" }}
{{- $_ := set $client "allow_direct_keys" .Values.unifai.client.allowDirectKeys }}
{{- end }}
{{- if .Values.unifai.client.mcpExternalClientUrl }}
{{- $_ := set $client "mcp_external_client_url" .Values.unifai.client.mcpExternalClientUrl }}
{{- end }}
{{- $_ := set $config "client" $client }}
{{- end }}
{{- /* Server */ -}}
{{- if .Values.unifai.server }}
{{- $server := dict }}
{{- if .Values.unifai.server.readBufferSize }}
{{- $_ := set $server "read_buffer_size" .Values.unifai.server.readBufferSize }}
{{- end }}
{{- if $server }}
{{- $_ := set $config "server" $server }}
{{- end }}
{{- end }}
{{- /* Framework */ -}}
{{- if .Values.unifai.framework }}
{{- $framework := dict }}
{{- if .Values.unifai.framework.pricing }}
{{- $pricing := dict }}
{{- if .Values.unifai.framework.pricing.pricingUrl }}
{{- $_ := set $pricing "pricing_url" .Values.unifai.framework.pricing.pricingUrl }}
{{- end }}
{{- if .Values.unifai.framework.pricing.modelParametersUrl }}
{{- $_ := set $pricing "model_parameters_url" .Values.unifai.framework.pricing.modelParametersUrl }}
{{- end }}
{{- if .Values.unifai.framework.pricing.pricingSyncInterval }}
{{- $_ := set $pricing "pricing_sync_interval" .Values.unifai.framework.pricing.pricingSyncInterval }}
{{- end }}
{{- if .Values.unifai.framework.pricing.mcpLibraryUrl }}
{{- $_ := set $pricing "mcp_library_url" .Values.unifai.framework.pricing.mcpLibraryUrl }}
{{- end }}
{{- if .Values.unifai.framework.pricing.mcpLibrarySyncInterval }}
{{- $_ := set $pricing "mcp_library_sync_interval" .Values.unifai.framework.pricing.mcpLibrarySyncInterval }}
{{- end }}
{{- if or $pricing.pricing_url $pricing.model_parameters_url $pricing.pricing_sync_interval $pricing.mcp_library_url $pricing.mcp_library_sync_interval }}
{{- $_ := set $framework "pricing" $pricing }}
{{- end }}
{{- end }}
{{- if $framework }}
{{- $_ := set $config "framework" $framework }}
{{- end }}
{{- end }}
{{- if .Values.unifai.providers }}
{{- $providers := dict }}
{{- range $providerName, $providerConfig := .Values.unifai.providers }}
{{- $providerCopy := deepCopy $providerConfig }}
{{- if $providerConfig.network_config }}
{{- $networkConfig := dict }}
{{- if $providerConfig.network_config.base_url }}
{{- $_ := set $networkConfig "base_url" $providerConfig.network_config.base_url }}
{{- end }}
{{- if $providerConfig.network_config.extra_headers }}
{{- $_ := set $networkConfig "extra_headers" $providerConfig.network_config.extra_headers }}
{{- end }}
{{- if hasKey $providerConfig.network_config "default_request_timeout_in_seconds" }}
{{- $_ := set $networkConfig "default_request_timeout_in_seconds" $providerConfig.network_config.default_request_timeout_in_seconds }}
{{- end }}
{{- if hasKey $providerConfig.network_config "max_retries" }}
{{- $_ := set $networkConfig "max_retries" $providerConfig.network_config.max_retries }}
{{- end }}
{{- if hasKey $providerConfig.network_config "retry_backoff_initial" }}
{{- $_ := set $networkConfig "retry_backoff_initial" $providerConfig.network_config.retry_backoff_initial }}
{{- end }}
{{- if hasKey $providerConfig.network_config "retry_backoff_initial_ms" }}
{{- $_ := set $networkConfig "retry_backoff_initial" $providerConfig.network_config.retry_backoff_initial_ms }}
{{- end }}
{{- if hasKey $providerConfig.network_config "retry_backoff_max" }}
{{- $_ := set $networkConfig "retry_backoff_max" $providerConfig.network_config.retry_backoff_max }}
{{- end }}
{{- if hasKey $providerConfig.network_config "retry_backoff_max_ms" }}
{{- $_ := set $networkConfig "retry_backoff_max" $providerConfig.network_config.retry_backoff_max_ms }}
{{- end }}
{{- if hasKey $providerConfig.network_config "insecure_skip_verify" }}
{{- $_ := set $networkConfig "insecure_skip_verify" $providerConfig.network_config.insecure_skip_verify }}
{{- end }}
{{- if hasKey $providerConfig.network_config "ca_cert_pem" }}
{{- $_ := set $networkConfig "ca_cert_pem" $providerConfig.network_config.ca_cert_pem }}
{{- end }}
{{- if hasKey $providerConfig.network_config "stream_idle_timeout_in_seconds" }}
{{- $_ := set $networkConfig "stream_idle_timeout_in_seconds" $providerConfig.network_config.stream_idle_timeout_in_seconds }}
{{- end }}
{{- if hasKey $providerConfig.network_config "max_conns_per_host" }}
{{- $_ := set $networkConfig "max_conns_per_host" $providerConfig.network_config.max_conns_per_host }}
{{- end }}
{{- if hasKey $providerConfig.network_config "enforce_http2" }}
{{- $_ := set $networkConfig "enforce_http2" $providerConfig.network_config.enforce_http2 }}
{{- end }}
{{- if $providerConfig.network_config.beta_header_overrides }}
{{- $_ := set $networkConfig "beta_header_overrides" $providerConfig.network_config.beta_header_overrides }}
{{- end }}
{{- if hasKey $providerConfig.network_config "allow_private_network" }}
{{- $_ := set $networkConfig "allow_private_network" $providerConfig.network_config.allow_private_network }}
{{- end }}
{{- $_ := set $providerCopy "network_config" $networkConfig }}
{{- end }}
{{- if $providerConfig.keys }}
{{- $keys := list }}
{{- range $key := $providerConfig.keys }}
{{- $keyCopy := deepCopy $key }}
{{- if and (not (hasKey $keyCopy "id")) (hasKey $keyCopy "name") $keyCopy.name }}
{{- $_ := set $keyCopy "id" $keyCopy.name }}
{{- end }}
{{- if not (hasKey $keyCopy "weight") }}
{{- $_ := set $keyCopy "weight" 1 }}
{{- end }}
{{- $keys = append $keys $keyCopy }}
{{- end }}
{{- $_ := set $providerCopy "keys" $keys }}
{{- end }}
{{- $_ := set $providers $providerName $providerCopy }}
{{- end }}
{{- $_ := set $config "providers" $providers }}
{{- end }}
{{- /* Governance */ -}}
{{- if .Values.unifai.governance }}
{{- $governance := dict }}
{{- if .Values.unifai.governance.budgets }}
{{- $_ := set $governance "budgets" .Values.unifai.governance.budgets }}
{{- end }}
{{- if .Values.unifai.governance.rateLimits }}
{{- $rateLimits := list }}
{{- range .Values.unifai.governance.rateLimits }}
{{- $rl := dict "id" .id }}
{{- if .token_max_limit }}{{- $_ := set $rl "token_max_limit" .token_max_limit }}{{- end }}
{{- if .token_reset_duration }}{{- $_ := set $rl "token_reset_duration" .token_reset_duration }}{{- end }}
{{- if .request_max_limit }}{{- $_ := set $rl "request_max_limit" .request_max_limit }}{{- end }}
{{- if .request_reset_duration }}{{- $_ := set $rl "request_reset_duration" .request_reset_duration }}{{- end }}
{{- $rateLimits = append $rateLimits $rl }}
{{- end }}
{{- $_ := set $governance "rate_limits" $rateLimits }}
{{- end }}
{{- if .Values.unifai.governance.customers }}
{{- $_ := set $governance "customers" .Values.unifai.governance.customers }}
{{- end }}
{{- if .Values.unifai.governance.teams }}
{{- $_ := set $governance "teams" .Values.unifai.governance.teams }}
{{- end }}
{{- if .Values.unifai.governance.businessUnits }}
{{- $businessUnits := list }}
{{- range .Values.unifai.governance.businessUnits }}
{{- $bu := dict "id" .id "name" .name }}
{{- if .budget_id }}{{- $_ := set $bu "budget_id" .budget_id }}{{- end }}
{{- if .rate_limit_id }}{{- $_ := set $bu "rate_limit_id" .rate_limit_id }}{{- end }}
{{- if .profile }}{{- $_ := set $bu "profile" .profile }}{{- end }}
{{- if .config }}{{- $_ := set $bu "config" .config }}{{- end }}
{{- if .claims }}{{- $_ := set $bu "claims" .claims }}{{- end }}
{{- if .teamIds }}{{- $_ := set $bu "team_ids" .teamIds }}{{- end }}
{{- $businessUnits = append $businessUnits $bu }}
{{- end }}
{{- $_ := set $governance "business_units" $businessUnits }}
{{- end }}
{{- if .Values.unifai.governance.roles }}
{{- $roles := list }}
{{- range .Values.unifai.governance.roles }}
{{- $role := dict "name" .name }}
{{- if .description }}{{- $_ := set $role "description" .description }}{{- end }}
{{- if .dac }}{{- $_ := set $role "dac" .dac }}{{- end }}
{{- if .access_profile }}{{- $_ := set $role "access_profile" .access_profile }}{{- end }}
{{- if .permissions }}{{- $_ := set $role "permissions" .permissions }}{{- end }}
{{- $roles = append $roles $role }}
{{- end }}
{{- $_ := set $governance "roles" $roles }}
{{- end }}
{{- if .Values.unifai.governance.virtualKeys }}
{{- $vks := list }}
{{- range .Values.unifai.governance.virtualKeys }}
{{- $vk := dict "id" .id "name" .name }}
{{- if .value }}{{- $_ := set $vk "value" .value }}{{- end }}
{{- if .description }}{{- $_ := set $vk "description" .description }}{{- end }}
{{- if hasKey . "is_active" }}{{- $_ := set $vk "is_active" .is_active }}{{- end }}
{{- if .team_id }}{{- $_ := set $vk "team_id" .team_id }}{{- end }}
{{- if .customer_id }}{{- $_ := set $vk "customer_id" .customer_id }}{{- end }}
{{- if hasKey . "access_profile_id" }}{{- $_ := set $vk "access_profile_id" .access_profile_id }}{{- end }}
{{- if .rate_limit_id }}{{- $_ := set $vk "rate_limit_id" .rate_limit_id }}{{- end }}
{{- if .provider_configs }}{{- $_ := set $vk "provider_configs" .provider_configs }}{{- end }}
{{- if .mcp_configs }}{{- $_ := set $vk "mcp_configs" .mcp_configs }}{{- end }}
{{- $vks = append $vks $vk }}
{{- end }}
{{- $_ := set $governance "virtual_keys" $vks }}
{{- end }}
{{- if .Values.unifai.governance.routingRules }}
{{- $_ := set $governance "routing_rules" .Values.unifai.governance.routingRules }}
{{- end }}
{{- if .Values.unifai.governance.modelConfigs }}
{{- $_ := set $governance "model_configs" .Values.unifai.governance.modelConfigs }}
{{- end }}
{{- if .Values.unifai.governance.providers }}
{{- $_ := set $governance "providers" .Values.unifai.governance.providers }}
{{- end }}
{{- if .Values.unifai.governance.pricingOverrides }}
{{- $_ := set $governance "pricing_overrides" .Values.unifai.governance.pricingOverrides }}
{{- end }}
{{- if .Values.unifai.governance.complexityAnalyzerConfig }}
{{- $_ := set $governance "complexity_analyzer_config" .Values.unifai.governance.complexityAnalyzerConfig }}
{{- end }}
{{- if .Values.unifai.governance.authConfig }}
{{- $authConfig := dict }}
{{- if and .Values.unifai.governance.authConfig.existingSecret .Values.unifai.governance.authConfig.usernameKey }}
{{- $_ := set $authConfig "admin_username" "env.UNIFAI_ADMIN_USERNAME" }}
{{- else if .Values.unifai.governance.authConfig.adminUsername }}
{{- $_ := set $authConfig "admin_username" .Values.unifai.governance.authConfig.adminUsername }}
{{- end }}
{{- if and .Values.unifai.governance.authConfig.existingSecret .Values.unifai.governance.authConfig.passwordKey }}
{{- $_ := set $authConfig "admin_password" "env.UNIFAI_ADMIN_PASSWORD" }}
{{- else if .Values.unifai.governance.authConfig.adminPassword }}
{{- $_ := set $authConfig "admin_password" .Values.unifai.governance.authConfig.adminPassword }}
{{- end }}
{{- if hasKey .Values.unifai.governance.authConfig "isEnabled" }}
{{- $_ := set $authConfig "is_enabled" .Values.unifai.governance.authConfig.isEnabled }}
{{- end }}
{{- if hasKey .Values.unifai.governance.authConfig "disableAuthOnInference" }}
{{- $_ := set $authConfig "disable_auth_on_inference" .Values.unifai.governance.authConfig.disableAuthOnInference }}
{{- end }}
{{- if or $authConfig.admin_username $authConfig.admin_password $authConfig.is_enabled $authConfig.disable_auth_on_inference }}
{{- $_ := set $governance "auth_config" $authConfig }}
{{- end }}
{{- end }}
{{- if or $governance.budgets $governance.rate_limits $governance.customers $governance.teams $governance.business_units $governance.roles $governance.virtual_keys $governance.routing_rules $governance.model_configs $governance.providers $governance.pricing_overrides $governance.complexity_analyzer_config $governance.auth_config }}
{{- $_ := set $config "governance" $governance }}
{{- end }}
{{- end }}
{{- /* Top-level Auth Config - for main UnifAI authentication */ -}}
{{- if .Values.unifai.authConfig }}
{{- $authConfig := dict }}
{{- /* Only use env var reference if governance auth secret is NOT already configured (to avoid referencing uninjected env vars) */ -}}
{{- if and .Values.unifai.authConfig.existingSecret .Values.unifai.authConfig.usernameKey (not (and .Values.unifai.governance .Values.unifai.governance.authConfig .Values.unifai.governance.authConfig.existingSecret)) }}
{{- $_ := set $authConfig "admin_username" "env.UNIFAI_ADMIN_USERNAME" }}
{{- else if .Values.unifai.authConfig.adminUsername }}
{{- $_ := set $authConfig "admin_username" .Values.unifai.authConfig.adminUsername }}
{{- end }}
{{- if and .Values.unifai.authConfig.existingSecret .Values.unifai.authConfig.passwordKey (not (and .Values.unifai.governance .Values.unifai.governance.authConfig .Values.unifai.governance.authConfig.existingSecret)) }}
{{- $_ := set $authConfig "admin_password" "env.UNIFAI_ADMIN_PASSWORD" }}
{{- else if .Values.unifai.authConfig.adminPassword }}
{{- $_ := set $authConfig "admin_password" .Values.unifai.authConfig.adminPassword }}
{{- end }}
{{- if hasKey .Values.unifai.authConfig "isEnabled" }}
{{- $_ := set $authConfig "is_enabled" .Values.unifai.authConfig.isEnabled }}
{{- end }}
{{- if hasKey .Values.unifai.authConfig "disableAuthOnInference" }}
{{- $_ := set $authConfig "disable_auth_on_inference" .Values.unifai.authConfig.disableAuthOnInference }}
{{- end }}
{{- if or $authConfig.admin_username $authConfig.admin_password $authConfig.is_enabled $authConfig.disable_auth_on_inference }}
{{- $_ := set $config "auth_config" $authConfig }}
{{- end }}
{{- end }}
{{- /* Cluster Config */ -}}
{{- if and .Values.unifai.cluster .Values.unifai.cluster.enabled }}
{{- $cluster := dict "enabled" true }}
{{- if .Values.unifai.cluster.peers }}
{{- $_ := set $cluster "peers" .Values.unifai.cluster.peers }}
{{- end }}
{{- if .Values.unifai.cluster.region }}
{{- $_ := set $cluster "region" .Values.unifai.cluster.region }}
{{- end }}
{{- if .Values.unifai.cluster.gossip }}
{{- $gossip := dict }}
{{- if .Values.unifai.cluster.gossip.port }}
{{- $_ := set $gossip "port" .Values.unifai.cluster.gossip.port }}
{{- end }}
{{- if .Values.unifai.cluster.gossip.config }}
{{- $gossipConfig := dict }}
{{- if .Values.unifai.cluster.gossip.config.timeoutSeconds }}
{{- $_ := set $gossipConfig "timeout_seconds" .Values.unifai.cluster.gossip.config.timeoutSeconds }}
{{- end }}
{{- if .Values.unifai.cluster.gossip.config.successThreshold }}
{{- $_ := set $gossipConfig "success_threshold" .Values.unifai.cluster.gossip.config.successThreshold }}
{{- end }}
{{- if .Values.unifai.cluster.gossip.config.failureThreshold }}
{{- $_ := set $gossipConfig "failure_threshold" .Values.unifai.cluster.gossip.config.failureThreshold }}
{{- end }}
{{- $_ := set $gossip "config" $gossipConfig }}
{{- end }}
{{- $_ := set $cluster "gossip" $gossip }}
{{- end }}
{{- if .Values.unifai.cluster.grpc }}
{{- $grpc := dict }}
{{- if .Values.unifai.cluster.grpc.port }}
{{- $_ := set $grpc "port" .Values.unifai.cluster.grpc.port }}
{{- end }}
{{- if .Values.unifai.cluster.grpc.dialTimeoutSeconds }}
{{- $_ := set $grpc "dial_timeout_seconds" .Values.unifai.cluster.grpc.dialTimeoutSeconds }}
{{- end }}
{{- if $grpc }}
{{- $_ := set $cluster "grpc" $grpc }}
{{- end }}
{{- end }}
{{- if and .Values.unifai.cluster.discovery .Values.unifai.cluster.discovery.enabled }}
{{- $discovery := dict "enabled" true "type" .Values.unifai.cluster.discovery.type }}
{{- $serviceName := .Values.unifai.cluster.discovery.serviceName }}
{{- if and (not $serviceName) (or (eq .Values.unifai.cluster.discovery.type "consul") (eq .Values.unifai.cluster.discovery.type "etcd") (eq .Values.unifai.cluster.discovery.type "udp")) }}
{{- fail "ERROR: unifai.cluster.discovery.serviceName is required for consul/etcd/udp discovery." }}
{{- end }}
{{- if $serviceName }}
{{- $_ := set $discovery "service_name" $serviceName }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.bindPort }}
{{- $_ := set $discovery "bind_port" .Values.unifai.cluster.discovery.bindPort }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.dialTimeout }}
{{- $_ := set $discovery "dial_timeout" .Values.unifai.cluster.discovery.dialTimeout }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.allowedAddressSpace }}
{{- $_ := set $discovery "allowed_address_space" .Values.unifai.cluster.discovery.allowedAddressSpace }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.k8sNamespace }}
{{- $_ := set $discovery "k8s_namespace" .Values.unifai.cluster.discovery.k8sNamespace }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.k8sLabelSelector }}
{{- $_ := set $discovery "k8s_label_selector" .Values.unifai.cluster.discovery.k8sLabelSelector }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.dnsNames }}
{{- $_ := set $discovery "dns_names" .Values.unifai.cluster.discovery.dnsNames }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.udpBroadcastPort }}
{{- $_ := set $discovery "udp_broadcast_port" .Values.unifai.cluster.discovery.udpBroadcastPort }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.consulAddress }}
{{- $_ := set $discovery "consul_address" .Values.unifai.cluster.discovery.consulAddress }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.etcdEndpoints }}
{{- $_ := set $discovery "etcd_endpoints" .Values.unifai.cluster.discovery.etcdEndpoints }}
{{- end }}
{{- if .Values.unifai.cluster.discovery.mdnsService }}
{{- $_ := set $discovery "mdns_service" .Values.unifai.cluster.discovery.mdnsService }}
{{- end }}
{{- $_ := set $cluster "discovery" $discovery }}
{{- end }}
{{- $_ := set $config "cluster_config" $cluster }}
{{- end }}
{{- /* SCIM Config */ -}}
{{- $scimValues := .Values.unifai.scim }}
{{- if and $scimValues $scimValues.enabled }}
{{- $scim := dict "enabled" true }}
{{- if $scimValues.provider }}
{{- $_ := set $scim "provider" $scimValues.provider }}
{{- end }}
{{- if $scimValues.config }}
{{- $_ := set $scim "config" $scimValues.config }}
{{- end }}
{{- $_ := set $config "scim_config" $scim }}
{{- end }}
{{- /* Load Balancer Config */ -}}
{{- if and .Values.unifai.loadBalancer .Values.unifai.loadBalancer.enabled }}
{{- $lb := dict "enabled" true }}
{{- if hasKey .Values.unifai.loadBalancer "directionSelectionEnabled" }}
{{- $_ := set $lb "direction_selection_enabled" .Values.unifai.loadBalancer.directionSelectionEnabled }}
{{- end }}
{{- if hasKey .Values.unifai.loadBalancer "routeSelectionEnabled" }}
{{- $_ := set $lb "route_selection_enabled" .Values.unifai.loadBalancer.routeSelectionEnabled }}
{{- end }}
{{- if hasKey .Values.unifai.loadBalancer "rerouteFailedDirections" }}
{{- $_ := set $lb "reroute_failed_directions" .Values.unifai.loadBalancer.rerouteFailedDirections }}
{{- end }}
{{- if hasKey .Values.unifai.loadBalancer "pruneFailedFallbacks" }}
{{- $_ := set $lb "prune_failed_fallbacks" .Values.unifai.loadBalancer.pruneFailedFallbacks }}
{{- end }}
{{- if .Values.unifai.loadBalancer.trackerConfig }}
{{- $_ := set $lb "tracker_config" .Values.unifai.loadBalancer.trackerConfig }}
{{- end }}
{{- if .Values.unifai.loadBalancer.bootstrap }}
{{- $_ := set $lb "bootstrap" .Values.unifai.loadBalancer.bootstrap }}
{{- end }}
{{- $_ := set $config "load_balancer_config" $lb }}
{{- end }}
{{- /* Guardrails Config */ -}}
{{- if .Values.unifai.guardrails }}
{{- $guardrails := dict }}
{{- if .Values.unifai.guardrails.rules }}
{{- $rules := list }}
{{- range .Values.unifai.guardrails.rules }}
{{- $rule := dict "id" .id "name" .name "enabled" .enabled "cel_expression" .cel_expression "apply_to" .apply_to }}
{{- if .description }}{{- $_ := set $rule "description" .description }}{{- end }}
{{- if hasKey . "query" }}{{- $_ := set $rule "query" .query }}{{- end }}
{{- if .sampling_rate }}{{- $_ := set $rule "sampling_rate" .sampling_rate }}{{- end }}
{{- if .timeout }}{{- $_ := set $rule "timeout" .timeout }}{{- end }}
{{- if hasKey . "max_turns_to_send" }}{{- $_ := set $rule "max_turns_to_send" .max_turns_to_send }}{{- end }}
{{- if .evaluation_mode }}{{- $_ := set $rule "evaluation_mode" .evaluation_mode }}{{- end }}
{{- if .provider_config_ids }}{{- $_ := set $rule "provider_config_ids" .provider_config_ids }}{{- end }}
{{- $rules = append $rules $rule }}
{{- end }}
{{- $_ := set $guardrails "guardrail_rules" $rules }}
{{- end }}
{{- if .Values.unifai.guardrails.providers }}
{{- $providers := list }}
{{- range .Values.unifai.guardrails.providers }}
{{- $provider := dict "id" .id "provider_name" .provider_name "policy_name" .policy_name "enabled" .enabled }}
{{- if .timeout }}{{- $_ := set $provider "timeout" .timeout }}{{- end }}
{{- if .config }}{{- $_ := set $provider "config" .config }}{{- end }}
{{- $providers = append $providers $provider }}
{{- end }}
{{- $_ := set $guardrails "guardrail_providers" $providers }}
{{- end }}
{{- if or $guardrails.guardrail_rules $guardrails.guardrail_providers }}
{{- $_ := set $config "guardrails_config" $guardrails }}
{{- end }}
{{- end }}
{{- /* Skills Registry */ -}}
{{- if .Values.unifai.skillsRegistry }}
{{- $_ := set $config "skills_registry" .Values.unifai.skillsRegistry }}
{{- end }}
{{- /* Access Profiles (Enterprise) */ -}}
{{- if .Values.unifai.accessProfiles }}
{{- $_ := set $config "access_profiles" .Values.unifai.accessProfiles }}
{{- end }}
{{- /* Config Store */ -}}
{{- if .Values.storage.configStore.enabled }}
{{- $configStoreType := .Values.storage.configStore.type | default .Values.storage.mode }}
{{- if eq $configStoreType "postgres" }}
{{- $pgConfig := dict "host" (include "unifai.postgresql.host" .) "port" (include "unifai.postgresql.port" .) "db_name" (include "unifai.postgresql.database" .) "user" (include "unifai.postgresql.username" .) "password" (include "unifai.postgresql.password" .) "ssl_mode" (include "unifai.postgresql.sslMode" .) }}
{{- if and .Values.postgresql.external.enabled .Values.postgresql.external.passwordCommand }}
{{- $_ := set $pgConfig "password_command" .Values.postgresql.external.passwordCommand }}
{{- $_ := unset $pgConfig "password" }}
{{- end }}
{{- if and .Values.postgresql.external.enabled .Values.postgresql.external.connMaxLifetime }}
{{- $_ := set $pgConfig "conn_max_lifetime" .Values.postgresql.external.connMaxLifetime }}
{{- end }}
{{- if .Values.storage.configStore.maxIdleConns }}
{{- $_ := set $pgConfig "max_idle_conns" (.Values.storage.configStore.maxIdleConns | int) }}
{{- end }}
{{- if .Values.storage.configStore.maxOpenConns }}
{{- $_ := set $pgConfig "max_open_conns" (.Values.storage.configStore.maxOpenConns | int) }}
{{- end }}
{{- $configStore := dict "enabled" true "type" "postgres" "config" $pgConfig }}
{{- $_ := set $config "config_store" $configStore }}
{{- else }}
{{- $sqliteConfigStore := dict "enabled" true "type" "sqlite" "config" (dict "path" (printf "%s/config.db" .Values.unifai.appDir)) }}
{{- $_ := set $config "config_store" $sqliteConfigStore }}
{{- end }}
{{- /* Vault Store (enterprise secret management) */ -}}
{{- if and .Values.storage.configStore.vaultStore .Values.storage.configStore.vaultStore.enabled }}
{{- $vs := .Values.storage.configStore.vaultStore }}
{{- $vaultStore := dict "enabled" true "type" $vs.type }}
{{- if $vs.prefix }}
{{- $_ := set $vaultStore "prefix" $vs.prefix }}
{{- end }}
{{- if $vs.accessMode }}
{{- $_ := set $vaultStore "access_mode" $vs.accessMode }}
{{- end }}
{{- if $vs.aws }}
{{- $aws := dict }}
{{- if $vs.aws.region }}{{- $_ := set $aws "region" $vs.aws.region }}{{- end }}
{{- if $vs.aws.accessKeyId }}{{- $_ := set $aws "access_key_id" $vs.aws.accessKeyId }}{{- end }}
{{- if $vs.aws.secretAccessKey }}{{- $_ := set $aws "secret_access_key" $vs.aws.secretAccessKey }}{{- end }}
{{- if $vs.aws.sessionToken }}{{- $_ := set $aws "session_token" $vs.aws.sessionToken }}{{- end }}
{{- if $vs.aws.roleArn }}{{- $_ := set $aws "role_arn" $vs.aws.roleArn }}{{- end }}
{{- if $vs.aws.kmsKeyId }}{{- $_ := set $aws "kms_key_id" $vs.aws.kmsKeyId }}{{- end }}
{{- $_ := set $vaultStore "aws" $aws }}
{{- end }}
{{- if $vs.gcp }}
{{- $gcp := dict }}
{{- if $vs.gcp.projectId }}{{- $_ := set $gcp "project_id" $vs.gcp.projectId }}{{- end }}
{{- if $vs.gcp.credentialsJson }}{{- $_ := set $gcp "credentials_json" $vs.gcp.credentialsJson }}{{- end }}
{{- $_ := set $vaultStore "gcp" $gcp }}
{{- end }}
{{- if $vs.hashicorp }}
{{- $hashicorp := dict }}
{{- if $vs.hashicorp.address }}{{- $_ := set $hashicorp "address" $vs.hashicorp.address }}{{- end }}
{{- if $vs.hashicorp.token }}{{- $_ := set $hashicorp "token" $vs.hashicorp.token }}{{- end }}
{{- if $vs.hashicorp.namespace }}{{- $_ := set $hashicorp "namespace" $vs.hashicorp.namespace }}{{- end }}
{{- if $vs.hashicorp.mountPath }}{{- $_ := set $hashicorp "mount_path" $vs.hashicorp.mountPath }}{{- end }}
{{- if $vs.hashicorp.roleId }}{{- $_ := set $hashicorp "role_id" $vs.hashicorp.roleId }}{{- end }}
{{- if $vs.hashicorp.secretId }}{{- $_ := set $hashicorp "secret_id" $vs.hashicorp.secretId }}{{- end }}
{{- $_ := set $vaultStore "hashicorp" $hashicorp }}
{{- end }}
{{- $cs := index $config "config_store" }}
{{- $_ := set $cs "vault_store" $vaultStore }}
{{- end }}
{{- end }}
{{- /* Logs Store */ -}}
{{- if .Values.storage.logsStore.enabled }}
{{- $logsStoreType := .Values.storage.logsStore.type | default .Values.storage.mode }}
{{- if eq $logsStoreType "postgres" }}
{{- $pgConfig := dict "host" (include "unifai.postgresql.host" .) "port" (include "unifai.postgresql.port" .) "db_name" (include "unifai.postgresql.database" .) "user" (include "unifai.postgresql.username" .) "password" (include "unifai.postgresql.password" .) "ssl_mode" (include "unifai.postgresql.sslMode" .) }}
{{- if and .Values.postgresql.external.enabled .Values.postgresql.external.passwordCommand }}
{{- $_ := set $pgConfig "password_command" .Values.postgresql.external.passwordCommand }}
{{- $_ := unset $pgConfig "password" }}
{{- end }}
{{- if and .Values.postgresql.external.enabled .Values.postgresql.external.connMaxLifetime }}
{{- $_ := set $pgConfig "conn_max_lifetime" .Values.postgresql.external.connMaxLifetime }}
{{- end }}
{{- if .Values.storage.logsStore.maxIdleConns }}
{{- $_ := set $pgConfig "max_idle_conns" (.Values.storage.logsStore.maxIdleConns | int) }}
{{- end }}
{{- if .Values.storage.logsStore.maxOpenConns }}
{{- $_ := set $pgConfig "max_open_conns" (.Values.storage.logsStore.maxOpenConns | int) }}
{{- end }}
{{- if .Values.storage.logsStore.matviewRefreshInterval }}
{{- $_ := set $pgConfig "matview_refresh_interval" .Values.storage.logsStore.matviewRefreshInterval }}
{{- end }}
{{- $logsStore := dict "enabled" true "type" "postgres" "config" $pgConfig }}
{{- if .Values.storage.logsStore.writer }}
{{- $writer := dict }}
{{- with .Values.storage.logsStore.writer.maxBatchSize }}{{- $_ := set $writer "max_batch_size" (. | int) }}{{- end }}
{{- with .Values.storage.logsStore.writer.batchInterval }}{{- $_ := set $writer "batch_interval" . }}{{- end }}
{{- with .Values.storage.logsStore.writer.maxBatchBytes }}{{- $_ := set $writer "max_batch_bytes" (. | int) }}{{- end }}
{{- with .Values.storage.logsStore.writer.writeQueueCapacity }}{{- $_ := set $writer "write_queue_capacity" (. | int) }}{{- end }}
{{- with .Values.storage.logsStore.writer.deferredUsageConcurrency }}{{- $_ := set $writer "deferred_usage_concurrency" (. | int) }}{{- end }}
{{- if $writer }}{{- $_ := set $logsStore "writer" $writer }}{{- end }}
{{- end }}
{{- $_ := set $config "logs_store" $logsStore }}
{{- else }}
{{- $sqliteLogsStore := dict "enabled" true "type" "sqlite" "config" (dict "path" (printf "%s/logs.db" .Values.unifai.appDir)) }}
{{- if .Values.storage.logsStore.writer }}
{{- $writer := dict }}
{{- with .Values.storage.logsStore.writer.maxBatchSize }}{{- $_ := set $writer "max_batch_size" (. | int) }}{{- end }}
{{- with .Values.storage.logsStore.writer.batchInterval }}{{- $_ := set $writer "batch_interval" . }}{{- end }}
{{- with .Values.storage.logsStore.writer.maxBatchBytes }}{{- $_ := set $writer "max_batch_bytes" (. | int) }}{{- end }}
{{- with .Values.storage.logsStore.writer.writeQueueCapacity }}{{- $_ := set $writer "write_queue_capacity" (. | int) }}{{- end }}
{{- with .Values.storage.logsStore.writer.deferredUsageConcurrency }}{{- $_ := set $writer "deferred_usage_concurrency" (. | int) }}{{- end }}
{{- if $writer }}{{- $_ := set $sqliteLogsStore "writer" $writer }}{{- end }}
{{- end }}
{{- $_ := set $config "logs_store" $sqliteLogsStore }}
{{- end }}
{{- /* Object Storage for log payloads */ -}}
{{- if and .Values.storage.logsStore.objectStorage .Values.storage.logsStore.objectStorage.enabled }}
{{- $os := .Values.storage.logsStore.objectStorage }}
{{- $osConfig := dict "type" $os.type "bucket" $os.bucket }}
{{- if $os.prefix }}
{{- $_ := set $osConfig "prefix" $os.prefix }}
{{- end }}
{{- if $os.compress }}
{{- $_ := set $osConfig "compress" true }}
{{- end }}
{{- if eq $os.type "s3" }}
{{- if $os.region }}
{{- $_ := set $osConfig "region" $os.region }}
{{- end }}
{{- if $os.endpoint }}
{{- $_ := set $osConfig "endpoint" $os.endpoint }}
{{- end }}
{{- if $os.existingSecret }}
{{- if $os.accessKeyIdKey }}
{{- $_ := set $osConfig "access_key_id" "env.UNIFAI_OBJECT_STORAGE_ACCESS_KEY_ID" }}
{{- end }}
{{- if $os.secretAccessKeyKey }}
{{- $_ := set $osConfig "secret_access_key" "env.UNIFAI_OBJECT_STORAGE_SECRET_ACCESS_KEY" }}
{{- end }}
{{- if $os.sessionTokenKey }}
{{- $_ := set $osConfig "session_token" "env.UNIFAI_OBJECT_STORAGE_SESSION_TOKEN" }}
{{- end }}
{{- $_ := set $osConfig "role_arn" "env.UNIFAI_OBJECT_STORAGE_ROLE_ARN" }}
{{- else }}
{{- if $os.accessKeyId }}
{{- $_ := set $osConfig "access_key_id" $os.accessKeyId }}
{{- end }}
{{- if $os.secretAccessKey }}
{{- $_ := set $osConfig "secret_access_key" $os.secretAccessKey }}
{{- end }}
{{- if $os.sessionToken }}
{{- $_ := set $osConfig "session_token" $os.sessionToken }}
{{- end }}
{{- if $os.roleArn }}
{{- $_ := set $osConfig "role_arn" $os.roleArn }}
{{- end }}
{{- end }}
{{- if $os.forcePathStyle }}
{{- $_ := set $osConfig "force_path_style" true }}
{{- end }}
{{- end }}
{{- if eq $os.type "gcs" }}
{{- if $os.projectId }}
{{- $_ := set $osConfig "project_id" $os.projectId }}
{{- end }}
{{- if $os.existingSecret }}
{{- $_ := set $osConfig "credentials_json" "env.UNIFAI_OBJECT_STORAGE_CREDENTIALS_JSON" }}
{{- else if $os.credentialsJson }}
{{- $_ := set $osConfig "credentials_json" $os.credentialsJson }}
{{- end }}
{{- end }}
{{- $_ := set (index $config "logs_store") "object_storage" $osConfig }}
{{- end }}
{{- if .Values.storage.logsStore.objectStorageExcludeFields }}
{{- $_ := set (index $config "logs_store") "object_storage_exclude_fields" .Values.storage.logsStore.objectStorageExcludeFields }}
{{- end }}
{{- end }}
{{- /* Vector Store */ -}}
{{- if and .Values.vectorStore.enabled (ne .Values.vectorStore.type "none") }}
{{- $vectorStore := dict "enabled" true "type" .Values.vectorStore.type }}
{{- if eq .Values.vectorStore.type "weaviate" }}
{{- $weaviateConfig := dict "scheme" (include "unifai.weaviate.scheme" .) "host" (include "unifai.weaviate.host" .) }}
{{- if .Values.vectorStore.weaviate.external.enabled }}
{{- $weaviateApiKey := include "unifai.weaviate.apiKey" . }}
{{- if $weaviateApiKey }}
{{- $_ := set $weaviateConfig "api_key" $weaviateApiKey }}
{{- end }}
{{- if or .Values.vectorStore.weaviate.external.grpcHost (hasKey .Values.vectorStore.weaviate.external "grpcSecured") }}
{{- $grpcConfig := dict }}
{{- if .Values.vectorStore.weaviate.external.grpcHost }}
{{- $_ := set $grpcConfig "host" .Values.vectorStore.weaviate.external.grpcHost }}
{{- end }}
{{- if hasKey .Values.vectorStore.weaviate.external "grpcSecured" }}
{{- $_ := set $grpcConfig "secured" .Values.vectorStore.weaviate.external.grpcSecured }}
{{- end }}
{{- $_ := set $weaviateConfig "grpc_config" $grpcConfig }}
{{- end }}
{{- if .Values.vectorStore.weaviate.external.timeout }}
{{- $_ := set $weaviateConfig "timeout" .Values.vectorStore.weaviate.external.timeout }}
{{- end }}
{{- if .Values.vectorStore.weaviate.external.className }}
{{- $_ := set $weaviateConfig "class_name" .Values.vectorStore.weaviate.external.className }}
{{- end }}
{{- end }}
{{- $_ := set $vectorStore "config" $weaviateConfig }}
{{- else if eq .Values.vectorStore.type "redis" }}
{{- $redisConfig := dict "addr" (printf "%s:%s" (include "unifai.redis.host" .) (include "unifai.redis.port" .)) }}
{{- $password := include "unifai.redis.password" . }}
{{- if $password }}
{{- $_ := set $redisConfig "password" $password }}
{{- end }}
{{- if .Values.vectorStore.redis.external.enabled }}
{{- if .Values.vectorStore.redis.external.username }}
{{- $_ := set $redisConfig "username" .Values.vectorStore.redis.external.username }}
{{- end }}
{{- if .Values.vectorStore.redis.external.database }}
{{- $_ := set $redisConfig "db" .Values.vectorStore.redis.external.database }}
{{- end }}
{{- if .Values.vectorStore.redis.external.poolSize }}
{{- $_ := set $redisConfig "pool_size" .Values.vectorStore.redis.external.poolSize }}
{{- end }}
{{- if .Values.vectorStore.redis.external.maxActiveConns }}
{{- $_ := set $redisConfig "max_active_conns" .Values.vectorStore.redis.external.maxActiveConns }}
{{- end }}
{{- if .Values.vectorStore.redis.external.minIdleConns }}
{{- $_ := set $redisConfig "min_idle_conns" .Values.vectorStore.redis.external.minIdleConns }}
{{- end }}
{{- if .Values.vectorStore.redis.external.maxIdleConns }}
{{- $_ := set $redisConfig "max_idle_conns" .Values.vectorStore.redis.external.maxIdleConns }}
{{- end }}
{{- if .Values.vectorStore.redis.external.connMaxLifetime }}
{{- $_ := set $redisConfig "conn_max_lifetime" .Values.vectorStore.redis.external.connMaxLifetime }}
{{- end }}
{{- if .Values.vectorStore.redis.external.connMaxIdleTime }}
{{- $_ := set $redisConfig "conn_max_idle_time" .Values.vectorStore.redis.external.connMaxIdleTime }}
{{- end }}
{{- if .Values.vectorStore.redis.external.dialTimeout }}
{{- $_ := set $redisConfig "dial_timeout" .Values.vectorStore.redis.external.dialTimeout }}
{{- end }}
{{- if .Values.vectorStore.redis.external.readTimeout }}
{{- $_ := set $redisConfig "read_timeout" .Values.vectorStore.redis.external.readTimeout }}
{{- end }}
{{- if .Values.vectorStore.redis.external.writeTimeout }}
{{- $_ := set $redisConfig "write_timeout" .Values.vectorStore.redis.external.writeTimeout }}
{{- end }}
{{- if .Values.vectorStore.redis.external.contextTimeout }}
{{- $_ := set $redisConfig "context_timeout" .Values.vectorStore.redis.external.contextTimeout }}
{{- end }}
{{- if .Values.vectorStore.redis.external.useTls }}
{{- $_ := set $redisConfig "use_tls" true }}
{{- end }}
{{- if .Values.vectorStore.redis.external.insecureSkipVerify }}
{{- $_ := set $redisConfig "insecure_skip_verify" true }}
{{- end }}
{{- if .Values.vectorStore.redis.external.caCertPem }}
{{- $_ := set $redisConfig "ca_cert_pem" .Values.vectorStore.redis.external.caCertPem }}
{{- end }}
{{- if .Values.vectorStore.redis.external.clusterMode }}
{{- $_ := set $redisConfig "cluster_mode" true }}
{{- end }}
{{- end }}
{{- $_ := set $vectorStore "config" $redisConfig }}
{{- else if eq .Values.vectorStore.type "qdrant" }}
{{- $qdrantConfig := dict "host" (include "unifai.qdrant.host" .) "port" (include "unifai.qdrant.port" . | int) }}
{{- $apiKey := include "unifai.qdrant.apiKey" . }}
{{- if $apiKey }}
{{- $_ := set $qdrantConfig "api_key" $apiKey }}
{{- end }}
{{- $useTls := include "unifai.qdrant.useTls" . }}
{{- if eq $useTls "true" }}
{{- $_ := set $qdrantConfig "use_tls" true }}
{{- else }}
{{- $_ := set $qdrantConfig "use_tls" false }}
{{- end }}
{{- $_ := set $vectorStore "config" $qdrantConfig }}
{{- else if eq .Values.vectorStore.type "pinecone" }}
{{- $pineconeConfig := dict }}
{{- $apiKey := include "unifai.pinecone.apiKey" . }}
{{- if $apiKey }}
{{- $_ := set $pineconeConfig "api_key" $apiKey }}
{{- end }}
{{- if .Values.vectorStore.pinecone.external.indexHost }}
{{- $_ := set $pineconeConfig "index_host" .Values.vectorStore.pinecone.external.indexHost }}
{{- end }}
{{- $_ := set $vectorStore "config" $pineconeConfig }}
{{- end }}
{{- $_ := set $config "vector_store" $vectorStore }}
{{- end }}
{{- /* MCP */ -}}
{{- if .Values.unifai.mcp.enabled }}
{{- $clientConfigs := list }}
{{- range $idx, $client := .Values.unifai.mcp.clientConfigs }}
{{- $cc := dict "name" $client.name }}
{{- /* Map connectionType: websocket -> sse, others pass through */ -}}
{{- if eq $client.connectionType "websocket" }}
{{- $_ := set $cc "connection_type" "sse" }}
{{- else }}
{{- $_ := set $cc "connection_type" $client.connectionType }}
{{- end }}
{{- /* Map httpConfig.url / websocketConfig.url -> connection_string */ -}}
{{- if and (eq $client.connectionType "http") $client.httpConfig }}
{{- $_ := set $cc "connection_string" $client.httpConfig.url }}
{{- end }}
{{- if and (eq $client.connectionType "websocket") $client.websocketConfig }}
{{- $_ := set $cc "connection_string" $client.websocketConfig.url }}
{{- end }}
{{- /* Map connectionString for SSE connections */ -}}
{{- if and (eq $client.connectionType "sse") $client.connectionString }}
{{- $_ := set $cc "connection_string" $client.connectionString }}
{{- end }}
{{- /* Map stdioConfig -> stdio_config */ -}}
{{- if $client.stdioConfig }}
{{- $stdio := dict "command" $client.stdioConfig.command }}
{{- if $client.stdioConfig.args }}
{{- $_ := set $stdio "args" $client.stdioConfig.args }}
{{- end }}
{{- if $client.stdioConfig.envs }}
{{- $_ := set $stdio "envs" $client.stdioConfig.envs }}
{{- end }}
{{- $_ := set $cc "stdio_config" $stdio }}
{{- end }}
{{- /* Pass through fields that are already snake_case or flat */ -}}
{{- if $client.headers }}
{{- $_ := set $cc "headers" $client.headers }}
{{- end }}
{{- if hasKey $client "tools_to_execute" }}
{{- $_ := set $cc "tools_to_execute" $client.tools_to_execute }}
{{- else if hasKey $client "toolsToExecute" }}
{{- $_ := set $cc "tools_to_execute" $client.toolsToExecute }}
{{- end }}
{{- if hasKey $client "tools_to_auto_execute" }}
{{- $_ := set $cc "tools_to_auto_execute" $client.tools_to_auto_execute }}
{{- else if hasKey $client "toolsToAutoExecute" }}
{{- $_ := set $cc "tools_to_auto_execute" $client.toolsToAutoExecute }}
{{- end }}
{{- if hasKey $client "auth_type" }}
{{- $_ := set $cc "auth_type" $client.auth_type }}
{{- else if hasKey $client "authType" }}
{{- $_ := set $cc "auth_type" $client.authType }}
{{- end }}
{{- if hasKey $client "oauth_config_id" }}
{{- $_ := set $cc "oauth_config_id" $client.oauth_config_id }}
{{- else if hasKey $client "oauthConfigId" }}
{{- $_ := set $cc "oauth_config_id" $client.oauthConfigId }}
{{- end }}
{{- if hasKey $client "isPingAvailable" }}
{{- $_ := set $cc "is_ping_available" $client.isPingAvailable }}
{{- end }}
{{- if $client.clientId }}
{{- $_ := set $cc "client_id" $client.clientId }}
{{- end }}
{{- if hasKey $client "isCodeModeClient" }}
{{- $_ := set $cc "is_code_mode_client" $client.isCodeModeClient }}
{{- end }}
{{- if $client.toolSyncInterval }}
{{- $_ := set $cc "tool_sync_interval" $client.toolSyncInterval }}
{{- end }}
{{- if $client.toolPricing }}
{{- $_ := set $cc "tool_pricing" $client.toolPricing }}
{{- end }}
{{- if $client.allowedExtraHeaders }}
{{- $_ := set $cc "allowed_extra_headers" $client.allowedExtraHeaders }}
{{- end }}
{{- if hasKey $client "allowOnAllVirtualKeys" }}
{{- $_ := set $cc "allow_on_all_virtual_keys" $client.allowOnAllVirtualKeys }}
{{- end }}
{{- /* Map tlsConfig -> tls_config (only for http/sse/websocket connection types) */ -}}
{{- if and $client.tlsConfig (or (eq $client.connectionType "http") (eq $client.connectionType "sse") (eq $client.connectionType "websocket")) }}
{{- $tls := dict }}
{{- if hasKey $client.tlsConfig "insecureSkipVerify" }}
{{- $_ := set $tls "insecure_skip_verify" $client.tlsConfig.insecureSkipVerify }}
{{- end }}
{{- if $client.tlsConfig.caCertPem }}
{{- $_ := set $tls "ca_cert_pem" $client.tlsConfig.caCertPem }}
{{- end }}
{{- if $tls }}
{{- $_ := set $cc "tls_config" $tls }}
{{- end }}
{{- end }}
{{- /* Override connection_string with env var placeholder when secretRef is set */ -}}
{{- if and $client.secretRef $client.secretRef.name }}
{{- $envName := printf "UNIFAI_MCP_%s_CONNECTION_STRING" (regexReplaceAll "[^A-Z0-9]+" (upper $client.name) "_") }}
{{- $_ := set $cc "connection_string" (printf "env.%s" $envName) }}
{{- end }}
{{- $clientConfigs = append $clientConfigs $cc }}
{{- end }}
{{- $mcpConfig := dict "client_configs" $clientConfigs }}
{{- if .Values.unifai.mcp.toolManagerConfig }}
{{- $tmConfig := dict }}
{{- if .Values.unifai.mcp.toolManagerConfig.toolExecutionTimeout }}
{{- $_ := set $tmConfig "tool_execution_timeout" .Values.unifai.mcp.toolManagerConfig.toolExecutionTimeout }}
{{- end }}
{{- if .Values.unifai.mcp.toolManagerConfig.maxAgentDepth }}
{{- $_ := set $tmConfig "max_agent_depth" .Values.unifai.mcp.toolManagerConfig.maxAgentDepth }}
{{- end }}
{{- if .Values.unifai.mcp.toolManagerConfig.codeModeBindingLevel }}
{{- $_ := set $tmConfig "code_mode_binding_level" .Values.unifai.mcp.toolManagerConfig.codeModeBindingLevel }}
{{- end }}
{{- if hasKey .Values.unifai.mcp.toolManagerConfig "disableAutoToolInject" }}
{{- $_ := set $tmConfig "disable_auto_tool_inject" .Values.unifai.mcp.toolManagerConfig.disableAutoToolInject }}
{{- end }}
{{- if $tmConfig }}
{{- $_ := set $mcpConfig "tool_manager_config" $tmConfig }}
{{- end }}
{{- end }}
{{- if hasKey .Values.unifai.mcp "toolSyncInterval" }}
{{- $_ := set $mcpConfig "tool_sync_interval" .Values.unifai.mcp.toolSyncInterval }}
{{- end }}
{{- if .Values.unifai.mcp.toolGroups }}
{{- $toolGroups := list }}
{{- range .Values.unifai.mcp.toolGroups }}
{{- $group := dict "name" .name }}
{{- if hasKey . "enabled" }}{{- $_ := set $group "enabled" .enabled }}{{- end }}
{{- if .description }}{{- $_ := set $group "description" .description }}{{- end }}
{{- if .tools }}
{{- $tools := list }}
{{- range .tools }}
{{- $tool := dict }}
{{- if .mcpClientId }}{{- $_ := set $tool "mcp_client_id" .mcpClientId }}{{- end }}
{{- if .mcpClientName }}{{- $_ := set $tool "mcp_client_name" .mcpClientName }}{{- end }}
{{- if .toolNames }}{{- $_ := set $tool "tool_names" .toolNames }}{{- end }}
{{- $tools = append $tools $tool }}
{{- end }}
{{- $_ := set $group "tools" $tools }}
{{- end }}
{{- if .virtualKeyIds }}{{- $_ := set $group "virtual_key_ids" .virtualKeyIds }}{{- end }}
{{- if .teamIds }}{{- $_ := set $group "team_ids" .teamIds }}{{- end }}
{{- if .customerIds }}{{- $_ := set $group "customer_ids" .customerIds }}{{- end }}
{{- if .userIds }}{{- $_ := set $group "user_ids" .userIds }}{{- end }}
{{- if .providerNames }}{{- $_ := set $group "provider_names" .providerNames }}{{- end }}
{{- if .apiKeyIds }}{{- $_ := set $group "api_key_ids" .apiKeyIds }}{{- end }}
{{- $toolGroups = append $toolGroups $group }}
{{- end }}
{{- $_ := set $mcpConfig "tool_groups" $toolGroups }}
{{- end }}
{{- $_ := set $config "mcp" $mcpConfig }}
{{- end }}
{{- /* Plugins - as array per schema */ -}}
{{- $plugins := list }}
{{- if .Values.unifai.plugins.telemetry.enabled }}
{{- $plugin := dict "enabled" true "name" "telemetry" "config" .Values.unifai.plugins.telemetry.config }}
{{- if hasKey .Values.unifai.plugins.telemetry "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.telemetry.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.unifai.plugins.logging.enabled }}
{{- $plugin := dict "enabled" true "name" "logging" "config" .Values.unifai.plugins.logging.config }}
{{- if hasKey .Values.unifai.plugins.logging "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.logging.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.unifai.plugins.governance.enabled }}
{{- $governanceConfig := dict }}
{{- if hasKey .Values.unifai.plugins.governance.config "is_vk_mandatory" }}
{{- $_ := set $governanceConfig "is_vk_mandatory" .Values.unifai.plugins.governance.config.is_vk_mandatory }}
{{- end }}
{{- if .Values.unifai.plugins.governance.config.required_headers }}
{{- $_ := set $governanceConfig "required_headers" .Values.unifai.plugins.governance.config.required_headers }}
{{- end }}
{{- if hasKey .Values.unifai.plugins.governance.config "is_enterprise" }}
{{- $_ := set $governanceConfig "is_enterprise" .Values.unifai.plugins.governance.config.is_enterprise }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "governance" "config" $governanceConfig }}
{{- if hasKey .Values.unifai.plugins.governance "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.governance.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.unifai.plugins.maxim.enabled }}
{{- $maximConfig := dict }}
{{- if and .Values.unifai.plugins.maxim.secretRef .Values.unifai.plugins.maxim.secretRef.name }}
{{- $_ := set $maximConfig "api_key" "env.UNIFAI_MAXIM_API_KEY" }}
{{- else if .Values.unifai.plugins.maxim.config.api_key }}
{{- $_ := set $maximConfig "api_key" .Values.unifai.plugins.maxim.config.api_key }}
{{- end }}
{{- if .Values.unifai.plugins.maxim.config.log_repo_id }}
{{- $_ := set $maximConfig "log_repo_id" .Values.unifai.plugins.maxim.config.log_repo_id }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "maxim" "config" $maximConfig }}
{{- if hasKey .Values.unifai.plugins.maxim "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.maxim.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.unifai.plugins.semanticCache.enabled }}
{{- $scConfig := dict }}
{{- $inputConfig := .Values.unifai.plugins.semanticCache.config | default dict }}
{{- if $inputConfig.dimension }}
{{- $_ := set $scConfig "dimension" $inputConfig.dimension }}
{{- end }}
{{/* Only include embedding provider config when not in direct cache mode (dimension: 1) */}}
{{- if ne (int ($inputConfig.dimension | default 1536)) 1 }}
{{- if $inputConfig.provider }}
{{- $_ := set $scConfig "provider" $inputConfig.provider }}
{{- end }}
{{- if $inputConfig.keys }}
{{- $_ := set $scConfig "keys" $inputConfig.keys }}
{{- end }}
{{- if $inputConfig.embedding_model }}
{{- $_ := set $scConfig "embedding_model" $inputConfig.embedding_model }}
{{- end }}
{{- end }}
{{- if $inputConfig.threshold }}
{{- $_ := set $scConfig "threshold" $inputConfig.threshold }}
{{- end }}
{{- if $inputConfig.ttl }}
{{- $_ := set $scConfig "ttl" $inputConfig.ttl }}
{{- end }}
{{- if $inputConfig.vector_store_namespace }}
{{- $_ := set $scConfig "vector_store_namespace" $inputConfig.vector_store_namespace }}
{{- end }}
{{- if $inputConfig.default_cache_key }}
{{- $_ := set $scConfig "default_cache_key" $inputConfig.default_cache_key }}
{{- end }}
{{- if hasKey $inputConfig "conversation_history_threshold" }}
{{- $_ := set $scConfig "conversation_history_threshold" $inputConfig.conversation_history_threshold }}
{{- end }}
{{- if hasKey $inputConfig "cache_by_model" }}
{{- $_ := set $scConfig "cache_by_model" $inputConfig.cache_by_model }}
{{- end }}
{{- if hasKey $inputConfig "cache_by_provider" }}
{{- $_ := set $scConfig "cache_by_provider" $inputConfig.cache_by_provider }}
{{- end }}
{{- if hasKey $inputConfig "exclude_system_prompt" }}
{{- $_ := set $scConfig "exclude_system_prompt" $inputConfig.exclude_system_prompt }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "semantic_cache" "config" $scConfig }}
{{- if hasKey .Values.unifai.plugins.semanticCache "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.semanticCache.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.unifai.plugins.otel.enabled }}
{{- $otelConfig := dict }}
{{- $inputConfig := .Values.unifai.plugins.otel.config | default dict }}
{{- if hasKey $inputConfig "profiles" }}
{{- $_ := set $otelConfig "profiles" $inputConfig.profiles }}
{{- if $inputConfig.plugin_span_filter }}
{{- $_ := set $otelConfig "plugin_span_filter" $inputConfig.plugin_span_filter }}
{{- end }}
{{- else }}
{{- if $inputConfig.service_name }}
{{- $_ := set $otelConfig "service_name" $inputConfig.service_name }}
{{- end }}
{{- if $inputConfig.collector_url }}
{{- $_ := set $otelConfig "collector_url" $inputConfig.collector_url }}
{{- end }}
{{- if $inputConfig.trace_type }}
{{- $_ := set $otelConfig "trace_type" $inputConfig.trace_type }}
{{- end }}
{{- if $inputConfig.protocol }}
{{- $_ := set $otelConfig "protocol" $inputConfig.protocol }}
{{- end }}
{{- if hasKey $inputConfig "metrics_enabled" }}
{{- $_ := set $otelConfig "metrics_enabled" $inputConfig.metrics_enabled }}
{{- end }}
{{- if $inputConfig.metrics_endpoint }}
{{- $_ := set $otelConfig "metrics_endpoint" $inputConfig.metrics_endpoint }}
{{- end }}
{{- if $inputConfig.metrics_push_interval }}
{{- $_ := set $otelConfig "metrics_push_interval" $inputConfig.metrics_push_interval }}
{{- end }}
{{- if $inputConfig.headers }}
{{- $_ := set $otelConfig "headers" $inputConfig.headers }}
{{- end }}
{{- if $inputConfig.tls_ca_cert }}
{{- $_ := set $otelConfig "tls_ca_cert" $inputConfig.tls_ca_cert }}
{{- end }}
{{- if hasKey $inputConfig "insecure" }}
{{- $_ := set $otelConfig "insecure" $inputConfig.insecure }}
{{- end }}
{{- if hasKey $inputConfig "disable_content_logging" }}
{{- $_ := set $otelConfig "disable_content_logging" $inputConfig.disable_content_logging }}
{{- end }}
{{- if hasKey $inputConfig "group_traces_by_session" }}
{{- $_ := set $otelConfig "group_traces_by_session" $inputConfig.group_traces_by_session }}
{{- end }}
{{- if hasKey $inputConfig "disable_root_span_content" }}
{{- $_ := set $otelConfig "disable_root_span_content" $inputConfig.disable_root_span_content }}
{{- end }}
{{- if $inputConfig.plugin_span_filter }}
{{- $_ := set $otelConfig "plugin_span_filter" $inputConfig.plugin_span_filter }}
{{- end }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "otel" "config" $otelConfig }}
{{- if hasKey .Values.unifai.plugins.otel "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.otel.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.unifai.plugins.datadog.enabled }}
{{- $datadogConfig := dict }}
{{- $inputConfig := .Values.unifai.plugins.datadog.config | default dict }}
{{- if $inputConfig.service_name }}
{{- $_ := set $datadogConfig "service_name" $inputConfig.service_name }}
{{- end }}
{{- if $inputConfig.ml_app }}
{{- $_ := set $datadogConfig "ml_app" $inputConfig.ml_app }}
{{- end }}
{{- if $inputConfig.agent_addr }}
{{- $_ := set $datadogConfig "agent_addr" $inputConfig.agent_addr }}
{{- end }}
{{- if $inputConfig.agent_host }}
{{- $_ := set $datadogConfig "agent_host" $inputConfig.agent_host }}
{{- end }}
{{- if $inputConfig.agent_port }}
{{- $_ := set $datadogConfig "agent_port" $inputConfig.agent_port }}
{{- end }}
{{- if $inputConfig.dogstatsd_addr }}
{{- $_ := set $datadogConfig "dogstatsd_addr" $inputConfig.dogstatsd_addr }}
{{- end }}
{{- if $inputConfig.dogstatsd_host }}
{{- $_ := set $datadogConfig "dogstatsd_host" $inputConfig.dogstatsd_host }}
{{- end }}
{{- if $inputConfig.dogstatsd_port }}
{{- $_ := set $datadogConfig "dogstatsd_port" $inputConfig.dogstatsd_port }}
{{- end }}
{{- if $inputConfig.env }}
{{- $_ := set $datadogConfig "env" $inputConfig.env }}
{{- end }}
{{- if $inputConfig.version }}
{{- $_ := set $datadogConfig "version" $inputConfig.version }}
{{- end }}
{{- if $inputConfig.custom_tags }}
{{- $_ := set $datadogConfig "custom_tags" $inputConfig.custom_tags }}
{{- end }}
{{- if hasKey $inputConfig "enable_metrics" }}
{{- $_ := set $datadogConfig "enable_metrics" $inputConfig.enable_metrics }}
{{- end }}
{{- if hasKey $inputConfig "enable_traces" }}
{{- $_ := set $datadogConfig "enable_traces" $inputConfig.enable_traces }}
{{- end }}
{{- if hasKey $inputConfig "enable_llm_obs" }}
{{- $_ := set $datadogConfig "enable_llm_obs" $inputConfig.enable_llm_obs }}
{{- end }}
{{- if hasKey $inputConfig "disable_content_logging" }}
{{- $_ := set $datadogConfig "disable_content_logging" $inputConfig.disable_content_logging }}
{{- end }}
{{- if hasKey $inputConfig "group_traces_by_session" }}
{{- $_ := set $datadogConfig "group_traces_by_session" $inputConfig.group_traces_by_session }}
{{- end }}
{{- if hasKey $inputConfig "agentless" }}
{{- $_ := set $datadogConfig "agentless" $inputConfig.agentless }}
{{- end }}
{{- if $inputConfig.api_key }}
{{- $_ := set $datadogConfig "api_key" $inputConfig.api_key }}
{{- end }}
{{- if $inputConfig.site }}
{{- $_ := set $datadogConfig "site" $inputConfig.site }}
{{- end }}
{{- if $inputConfig.request_headers }}
{{- $_ := set $datadogConfig "request_headers" $inputConfig.request_headers }}
{{- end }}
{{- if $inputConfig.plugin_span_filter }}
{{- $_ := set $datadogConfig "plugin_span_filter" $inputConfig.plugin_span_filter }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "datadog" "config" $datadogConfig }}
{{- if hasKey .Values.unifai.plugins.datadog "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.datadog.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.unifai.plugins.bigquery.enabled }}
{{- $bigqueryConfig := dict }}
{{- $inputConfig := .Values.unifai.plugins.bigquery.config | default dict }}
{{- if $inputConfig.project_id }}
{{- $_ := set $bigqueryConfig "project_id" $inputConfig.project_id }}
{{- end }}
{{- if $inputConfig.dataset_id }}
{{- $_ := set $bigqueryConfig "dataset_id" $inputConfig.dataset_id }}
{{- end }}
{{- if $inputConfig.table_id }}
{{- $_ := set $bigqueryConfig "table_id" $inputConfig.table_id }}
{{- end }}
{{- if $inputConfig.location }}
{{- $_ := set $bigqueryConfig "location" $inputConfig.location }}
{{- end }}
{{- if $inputConfig.service_account_key }}
{{- $_ := set $bigqueryConfig "service_account_key" $inputConfig.service_account_key }}
{{- end }}
{{- if hasKey $inputConfig "create_table_if_not_exists" }}
{{- $_ := set $bigqueryConfig "create_table_if_not_exists" $inputConfig.create_table_if_not_exists }}
{{- end }}
{{- if hasKey $inputConfig "flush_interval_seconds" }}
{{- $_ := set $bigqueryConfig "flush_interval_seconds" $inputConfig.flush_interval_seconds }}
{{- end }}
{{- if hasKey $inputConfig "buffer_size" }}
{{- $_ := set $bigqueryConfig "buffer_size" $inputConfig.buffer_size }}
{{- end }}
{{- if $inputConfig.custom_labels }}
{{- $_ := set $bigqueryConfig "custom_labels" $inputConfig.custom_labels }}
{{- end }}
{{- if hasKey $inputConfig "disable_content_logging" }}
{{- $_ := set $bigqueryConfig "disable_content_logging" $inputConfig.disable_content_logging }}
{{- end }}
{{- if $inputConfig.request_headers }}
{{- $_ := set $bigqueryConfig "request_headers" $inputConfig.request_headers }}
{{- end }}
{{- if $inputConfig.plugin_span_filter }}
{{- $_ := set $bigqueryConfig "plugin_span_filter" $inputConfig.plugin_span_filter }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "bigquery" "config" $bigqueryConfig }}
{{- if hasKey .Values.unifai.plugins.bigquery "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.bigquery.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if (.Values.unifai.plugins.kafka).enabled }}
{{- $kafkaConfig := dict }}
{{- $inputConfig := .Values.unifai.plugins.kafka.config | default dict }}
{{- if $inputConfig.brokers }}
{{- $_ := set $kafkaConfig "brokers" $inputConfig.brokers }}
{{- end }}
{{- if $inputConfig.topic }}
{{- $_ := set $kafkaConfig "topic" $inputConfig.topic }}
{{- end }}
{{- if hasKey $inputConfig "sasl_enabled" }}
{{- $_ := set $kafkaConfig "sasl_enabled" $inputConfig.sasl_enabled }}
{{- end }}
{{- if $inputConfig.sasl }}
{{- $_ := set $kafkaConfig "sasl" $inputConfig.sasl }}
{{- end }}
{{- if hasKey $inputConfig "tls_enabled" }}
{{- $_ := set $kafkaConfig "tls_enabled" $inputConfig.tls_enabled }}
{{- end }}
{{- if $inputConfig.ca_cert }}
{{- $_ := set $kafkaConfig "ca_cert" $inputConfig.ca_cert }}
{{- end }}
{{- if $inputConfig.compression }}
{{- $_ := set $kafkaConfig "compression" $inputConfig.compression }}
{{- end }}
{{- if hasKey $inputConfig "batch_size" }}
{{- $_ := set $kafkaConfig "batch_size" $inputConfig.batch_size }}
{{- end }}
{{- if hasKey $inputConfig "flush_interval_ms" }}
{{- $_ := set $kafkaConfig "flush_interval_ms" $inputConfig.flush_interval_ms }}
{{- end }}
{{- if hasKey $inputConfig "auto_create_topic" }}
{{- $_ := set $kafkaConfig "auto_create_topic" $inputConfig.auto_create_topic }}
{{- end }}
{{- if hasKey $inputConfig "disable_content_logging" }}
{{- $_ := set $kafkaConfig "disable_content_logging" $inputConfig.disable_content_logging }}
{{- end }}
{{- if $inputConfig.request_headers }}
{{- $_ := set $kafkaConfig "request_headers" $inputConfig.request_headers }}
{{- end }}
{{- if $inputConfig.plugin_span_filter }}
{{- $_ := set $kafkaConfig "plugin_span_filter" $inputConfig.plugin_span_filter }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "kafka" "config" $kafkaConfig }}
{{- if hasKey .Values.unifai.plugins.kafka "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.kafka.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if (.Values.unifai.plugins.pubsub).enabled }}
{{- $pubsubConfig := dict }}
{{- $inputConfig := .Values.unifai.plugins.pubsub.config | default dict }}
{{- if $inputConfig.project_id }}
{{- $_ := set $pubsubConfig "project_id" $inputConfig.project_id }}
{{- end }}
{{- if $inputConfig.topic_id }}
{{- $_ := set $pubsubConfig "topic_id" $inputConfig.topic_id }}
{{- end }}
{{- if $inputConfig.service_account_key }}
{{- $_ := set $pubsubConfig "service_account_key" $inputConfig.service_account_key }}
{{- end }}
{{- if hasKey $inputConfig "auto_create_topic" }}
{{- $_ := set $pubsubConfig "auto_create_topic" $inputConfig.auto_create_topic }}
{{- end }}
{{- if hasKey $inputConfig "disable_content_logging" }}
{{- $_ := set $pubsubConfig "disable_content_logging" $inputConfig.disable_content_logging }}
{{- end }}
{{- if $inputConfig.request_headers }}
{{- $_ := set $pubsubConfig "request_headers" $inputConfig.request_headers }}
{{- end }}
{{- if $inputConfig.plugin_span_filter }}
{{- $_ := set $pubsubConfig "plugin_span_filter" $inputConfig.plugin_span_filter }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "pubsub" "config" $pubsubConfig }}
{{- if hasKey .Values.unifai.plugins.pubsub "version" }}{{- $_ := set $plugin "version" (.Values.unifai.plugins.pubsub.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- /* Custom plugins */ -}}
{{- if .Values.unifai.plugins.custom }}
{{- range .Values.unifai.plugins.custom }}
{{- $customPlugin := dict "enabled" .enabled "name" .name }}
{{- if .path }}{{- $_ := set $customPlugin "path" .path }}{{- end }}
{{- if hasKey . "version" }}{{- $_ := set $customPlugin "version" (.version | int) }}{{- end }}
{{- if .config }}{{- $_ := set $customPlugin "config" .config }}{{- end }}
{{- if .placement }}{{- $_ := set $customPlugin "placement" .placement }}{{- end }}
{{- if .order }}{{- $_ := set $customPlugin "order" (.order | int) }}{{- end }}
{{- $plugins = append $plugins $customPlugin }}
{{- end }}
{{- end }}
{{- if $plugins }}
{{- $_ := set $config "plugins" $plugins }}
{{- end }}
{{- /* Audit Logs */ -}}
{{- if .Values.unifai.auditLogs }}
{{- $auditLogs := dict }}
{{- if hasKey .Values.unifai.auditLogs "disabled" }}
{{- $_ := set $auditLogs "disabled" .Values.unifai.auditLogs.disabled }}
{{- end }}
{{- if .Values.unifai.auditLogs.hmacKey }}
{{- $_ := set $auditLogs "hmac_key" .Values.unifai.auditLogs.hmacKey }}
{{- end }}
{{- if or (hasKey $auditLogs "disabled") $auditLogs.hmac_key }}
{{- $_ := set $config "audit_logs" $auditLogs }}
{{- end }}
{{- end }}
{{- /* Large Payload Optimization */ -}}
{{- if .Values.unifai.largePayloadOptimization }}
{{- $lpo := dict }}
{{- if hasKey .Values.unifai.largePayloadOptimization "enabled" }}
{{- $_ := set $lpo "enabled" .Values.unifai.largePayloadOptimization.enabled }}
{{- end }}
{{- if hasKey .Values.unifai.largePayloadOptimization "requestThresholdBytes" }}
{{- $_ := set $lpo "request_threshold_bytes" .Values.unifai.largePayloadOptimization.requestThresholdBytes }}
{{- end }}
{{- if hasKey .Values.unifai.largePayloadOptimization "responseThresholdBytes" }}
{{- $_ := set $lpo "response_threshold_bytes" .Values.unifai.largePayloadOptimization.responseThresholdBytes }}
{{- end }}
{{- if hasKey .Values.unifai.largePayloadOptimization "prefetchSizeBytes" }}
{{- $_ := set $lpo "prefetch_size_bytes" .Values.unifai.largePayloadOptimization.prefetchSizeBytes }}
{{- end }}
{{- if hasKey .Values.unifai.largePayloadOptimization "maxPayloadBytes" }}
{{- $_ := set $lpo "max_payload_bytes" .Values.unifai.largePayloadOptimization.maxPayloadBytes }}
{{- end }}
{{- if hasKey .Values.unifai.largePayloadOptimization "truncatedLogBytes" }}
{{- $_ := set $lpo "truncated_log_bytes" .Values.unifai.largePayloadOptimization.truncatedLogBytes }}
{{- end }}
{{- if $lpo }}
{{- $_ := set $config "large_payload_optimization" $lpo }}
{{- end }}
{{- end }}
{{- /* WebSocket Config */ -}}
{{- if .Values.unifai.websocket }}
{{- $ws := dict }}
{{- if .Values.unifai.websocket.maxConnectionsPerUser }}
{{- $_ := set $ws "max_connections_per_user" .Values.unifai.websocket.maxConnectionsPerUser }}
{{- end }}
{{- if .Values.unifai.websocket.transcriptBufferSize }}
{{- $_ := set $ws "transcript_buffer_size" .Values.unifai.websocket.transcriptBufferSize }}
{{- end }}
{{- if .Values.unifai.websocket.pool }}
{{- $pool := dict }}
{{- if .Values.unifai.websocket.pool.maxIdlePerKey }}
{{- $_ := set $pool "max_idle_per_key" .Values.unifai.websocket.pool.maxIdlePerKey }}
{{- end }}
{{- if .Values.unifai.websocket.pool.maxTotalConnections }}
{{- $_ := set $pool "max_total_connections" .Values.unifai.websocket.pool.maxTotalConnections }}
{{- end }}
{{- if .Values.unifai.websocket.pool.idleTimeoutSeconds }}
{{- $_ := set $pool "idle_timeout_seconds" .Values.unifai.websocket.pool.idleTimeoutSeconds }}
{{- end }}
{{- if .Values.unifai.websocket.pool.maxConnectionLifetimeSeconds }}
{{- $_ := set $pool "max_connection_lifetime_seconds" .Values.unifai.websocket.pool.maxConnectionLifetimeSeconds }}
{{- end }}
{{- if $pool }}
{{- $_ := set $ws "pool" $pool }}
{{- end }}
{{- end }}
{{- if $ws }}
{{- $_ := set $config "websocket" $ws }}
{{- end }}
{{- end }}
{{- if .Values.unifai.featureFlags }}
{{- $flags := dict }}
{{- range $name, $cfg := .Values.unifai.featureFlags }}
{{- if not (kindIs "map" $cfg) }}
{{- fail (printf "ERROR: unifai.featureFlags.%s must be an object with an 'enabled' field." $name) }}
{{- end }}
{{- if not (hasKey $cfg "enabled") }}
{{- fail (printf "ERROR: unifai.featureFlags.%s.enabled is required." $name) }}
{{- end }}
{{- $_ := set $flags $name (dict "enabled" $cfg.enabled) }}
{{- end }}
{{- if $flags }}
{{- $_ := set $config "feature_flags" (dict "flags" $flags) }}
{{- end }}
{{- end }}
{{- /* Circuit Breaker Config */ -}}
{{- if .Values.unifai.circuitBreakerConfig }}
{{- $_ := set $config "circuit_breaker_config" .Values.unifai.circuitBreakerConfig }}
{{- end }}
{{- $config | toJson }}
{{- end }}

{{/*
Validation template - validates required fields from config.schema.json
Call this template at the beginning of deployment/stateful templates
*/}}
{{- define "unifai.validate" -}}

{{/* Validate unifai.sourceOfTruth enum */}}
{{- if .Values.unifai.sourceOfTruth }}
{{- if and (ne .Values.unifai.sourceOfTruth "split") (ne .Values.unifai.sourceOfTruth "config.json") }}
{{- fail (printf "ERROR: unifai.sourceOfTruth must be 'split' or 'config.json', got: %s" .Values.unifai.sourceOfTruth) }}
{{- end }}
{{- end }}

{{/* Validate semantic cache plugin when enabled */}}
{{- if and .Values.unifai.plugins.telemetry.enabled (hasKey .Values.unifai.plugins.telemetry "version") (lt (int .Values.unifai.plugins.telemetry.version) 1) }}
{{- fail "ERROR: unifai.plugins.telemetry.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.unifai.plugins.telemetry.enabled (hasKey .Values.unifai.plugins.telemetry "version") (gt (int .Values.unifai.plugins.telemetry.version) 32767) }}
{{- fail "ERROR: unifai.plugins.telemetry.version must be <= 32767." }}
{{- end }}
{{- if and .Values.unifai.plugins.logging.enabled (hasKey .Values.unifai.plugins.logging "version") (lt (int .Values.unifai.plugins.logging.version) 1) }}
{{- fail "ERROR: unifai.plugins.logging.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.unifai.plugins.logging.enabled (hasKey .Values.unifai.plugins.logging "version") (gt (int .Values.unifai.plugins.logging.version) 32767) }}
{{- fail "ERROR: unifai.plugins.logging.version must be <= 32767." }}
{{- end }}
{{- if and .Values.unifai.plugins.governance.enabled (hasKey .Values.unifai.plugins.governance "version") (lt (int .Values.unifai.plugins.governance.version) 1) }}
{{- fail "ERROR: unifai.plugins.governance.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.unifai.plugins.governance.enabled (hasKey .Values.unifai.plugins.governance "version") (gt (int .Values.unifai.plugins.governance.version) 32767) }}
{{- fail "ERROR: unifai.plugins.governance.version must be <= 32767." }}
{{- end }}
{{- if and .Values.unifai.plugins.maxim.enabled (hasKey .Values.unifai.plugins.maxim "version") (lt (int .Values.unifai.plugins.maxim.version) 1) }}
{{- fail "ERROR: unifai.plugins.maxim.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.unifai.plugins.maxim.enabled (hasKey .Values.unifai.plugins.maxim "version") (gt (int .Values.unifai.plugins.maxim.version) 32767) }}
{{- fail "ERROR: unifai.plugins.maxim.version must be <= 32767." }}
{{- end }}
{{- if and .Values.unifai.plugins.semanticCache.enabled (hasKey .Values.unifai.plugins.semanticCache "version") (lt (int .Values.unifai.plugins.semanticCache.version) 1) }}
{{- fail "ERROR: unifai.plugins.semanticCache.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.unifai.plugins.semanticCache.enabled (hasKey .Values.unifai.plugins.semanticCache "version") (gt (int .Values.unifai.plugins.semanticCache.version) 32767) }}
{{- fail "ERROR: unifai.plugins.semanticCache.version must be <= 32767." }}
{{- end }}
{{- if and .Values.unifai.plugins.otel.enabled (hasKey .Values.unifai.plugins.otel "version") (lt (int .Values.unifai.plugins.otel.version) 1) }}
{{- fail "ERROR: unifai.plugins.otel.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.unifai.plugins.otel.enabled (hasKey .Values.unifai.plugins.otel "version") (gt (int .Values.unifai.plugins.otel.version) 32767) }}
{{- fail "ERROR: unifai.plugins.otel.version must be <= 32767." }}
{{- end }}
{{- if and .Values.unifai.plugins.datadog.enabled (hasKey .Values.unifai.plugins.datadog "version") (lt (int .Values.unifai.plugins.datadog.version) 1) }}
{{- fail "ERROR: unifai.plugins.datadog.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.unifai.plugins.datadog.enabled (hasKey .Values.unifai.plugins.datadog "version") (gt (int .Values.unifai.plugins.datadog.version) 32767) }}
{{- fail "ERROR: unifai.plugins.datadog.version must be <= 32767." }}
{{- end }}
{{- $ddCfg := (.Values.unifai.plugins.datadog.config | default dict) }}
{{- if and .Values.unifai.plugins.datadog.enabled $ddCfg.agentless (not $ddCfg.api_key) }}
{{- fail "ERROR: unifai.plugins.datadog.config.api_key is required when unifai.plugins.datadog.config.agentless is true." }}
{{- end }}
{{- if and .Values.unifai.plugins.bigquery.enabled (hasKey .Values.unifai.plugins.bigquery "version") (lt (int .Values.unifai.plugins.bigquery.version) 1) }}
{{- fail "ERROR: unifai.plugins.bigquery.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.unifai.plugins.bigquery.enabled (hasKey .Values.unifai.plugins.bigquery "version") (gt (int .Values.unifai.plugins.bigquery.version) 32767) }}
{{- fail "ERROR: unifai.plugins.bigquery.version must be <= 32767." }}
{{- end }}
{{- if and .Values.unifai.plugins.bigquery.enabled (not (.Values.unifai.plugins.bigquery.config | default dict).project_id) }}
{{- fail "ERROR: unifai.plugins.bigquery.config.project_id is required when the BigQuery plugin is enabled." }}
{{- end }}
{{- if and (.Values.unifai.plugins.kafka).enabled (hasKey .Values.unifai.plugins.kafka "version") (lt (int .Values.unifai.plugins.kafka.version) 1) }}
{{- fail "ERROR: unifai.plugins.kafka.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and (.Values.unifai.plugins.kafka).enabled (hasKey .Values.unifai.plugins.kafka "version") (gt (int .Values.unifai.plugins.kafka.version) 32767) }}
{{- fail "ERROR: unifai.plugins.kafka.version must be <= 32767." }}
{{- end }}
{{- if (.Values.unifai.plugins.kafka).enabled }}
{{- $kafkaInputConfig := .Values.unifai.plugins.kafka.config | default dict }}
{{- if not $kafkaInputConfig.brokers }}
{{- fail "ERROR: unifai.plugins.kafka.config.brokers is required when the Kafka plugin is enabled." }}
{{- end }}
{{- if not $kafkaInputConfig.topic }}
{{- fail "ERROR: unifai.plugins.kafka.config.topic is required when the Kafka plugin is enabled." }}
{{- end }}
{{- end }}
{{- if and (.Values.unifai.plugins.pubsub).enabled (hasKey .Values.unifai.plugins.pubsub "version") (lt (int .Values.unifai.plugins.pubsub.version) 1) }}
{{- fail "ERROR: unifai.plugins.pubsub.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and (.Values.unifai.plugins.pubsub).enabled (hasKey .Values.unifai.plugins.pubsub "version") (gt (int .Values.unifai.plugins.pubsub.version) 32767) }}
{{- fail "ERROR: unifai.plugins.pubsub.version must be <= 32767." }}
{{- end }}
{{- if (.Values.unifai.plugins.pubsub).enabled }}
{{- $pubsubInputConfig := .Values.unifai.plugins.pubsub.config | default dict }}
{{- if not $pubsubInputConfig.project_id }}
{{- fail "ERROR: unifai.plugins.pubsub.config.project_id is required when the Pub/Sub plugin is enabled." }}
{{- end }}
{{- if not $pubsubInputConfig.topic_id }}
{{- fail "ERROR: unifai.plugins.pubsub.config.topic_id is required when the Pub/Sub plugin is enabled." }}
{{- end }}
{{- end }}

{{/* Validate semantic cache plugin when enabled */}}
{{- if .Values.unifai.plugins.semanticCache.enabled }}
{{/* When dimension is 1, direct (hash-based) caching is used — provider and keys are not required. */}}
{{- if ne (int .Values.unifai.plugins.semanticCache.config.dimension) 1 }}
{{- if not .Values.unifai.plugins.semanticCache.config.provider }}
{{- fail "ERROR: unifai.plugins.semanticCache.config.provider is required for semantic caching. Supported providers: openai, anthropic, gemini, bedrock, azure, cohere, mistral, groq, ollama, openrouter, vertex, cerebras, parasail, perplexity, sgl, huggingface. For direct (hash-based) caching, set dimension: 1." }}
{{- end }}
{{- if not .Values.unifai.plugins.semanticCache.config.keys }}
{{- fail "ERROR: unifai.plugins.semanticCache.config.keys is required for semantic caching. Provide at least one API key for the embedding provider. For direct (hash-based) caching, set dimension: 1." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate OTEL plugin when enabled */}}
{{- if .Values.unifai.plugins.otel.enabled }}
{{- $otelInputConfig := .Values.unifai.plugins.otel.config | default dict }}
{{- if hasKey $otelInputConfig "profiles" }}
{{- if not $otelInputConfig.profiles }}
{{- fail "ERROR: unifai.plugins.otel.config.profiles must contain at least one profile when OTEL plugin is enabled." }}
{{- end }}
{{- range $idx, $profile := $otelInputConfig.profiles }}
{{- $profileEnabled := true }}
{{- if hasKey $profile "enabled" }}
{{- $profileEnabled = $profile.enabled }}
{{- end }}
{{- if $profileEnabled }}
{{- if not $profile.collector_url }}
{{- fail (printf "ERROR: unifai.plugins.otel.config.profiles[%d].collector_url is required for enabled OTEL profiles." $idx) }}
{{- end }}
{{- if not $profile.trace_type }}
{{- fail (printf "ERROR: unifai.plugins.otel.config.profiles[%d].trace_type is required. Supported values: genai_extension, vercel, open_inference" $idx) }}
{{- end }}
{{- if not $profile.protocol }}
{{- fail (printf "ERROR: unifai.plugins.otel.config.profiles[%d].protocol is required. Supported values: http, grpc" $idx) }}
{{- end }}
{{- if and $profile.metrics_enabled (not $profile.metrics_endpoint) }}
{{- fail (printf "ERROR: unifai.plugins.otel.config.profiles[%d].metrics_endpoint is required when metrics_enabled is true." $idx) }}
{{- end }}
{{- end }}
{{- end }}
{{- else }}
{{- if not $otelInputConfig.collector_url }}
{{- fail "ERROR: unifai.plugins.otel.config.collector_url is required when OTEL plugin is enabled. Provide the URL of your OpenTelemetry collector." }}
{{- end }}
{{- if not $otelInputConfig.trace_type }}
{{- fail "ERROR: unifai.plugins.otel.config.trace_type is required when OTEL plugin is enabled. Supported values: genai_extension, vercel, open_inference" }}
{{- end }}
{{- if not $otelInputConfig.protocol }}
{{- fail "ERROR: unifai.plugins.otel.config.protocol is required when OTEL plugin is enabled. Supported values: http, grpc" }}
{{- end }}
{{- if and $otelInputConfig.metrics_enabled (not $otelInputConfig.metrics_endpoint) }}
{{- fail "ERROR: unifai.plugins.otel.config.metrics_endpoint is required when metrics_enabled is true." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate Maxim plugin when enabled */}}
{{- if .Values.unifai.plugins.maxim.enabled }}
{{- if and (not .Values.unifai.plugins.maxim.config.api_key) (not .Values.unifai.plugins.maxim.secretRef.name) }}
{{- fail "ERROR: unifai.plugins.maxim.config.api_key or unifai.plugins.maxim.secretRef.name is required when Maxim plugin is enabled." }}
{{- end }}
{{- end }}

{{/* Validate SCIM/SSO config when enabled */}}
{{- $scimValidation := .Values.unifai.scim }}
{{- if and $scimValidation $scimValidation.enabled }}
{{- if eq $scimValidation.provider "okta" }}
{{- if not $scimValidation.config.issuerUrl }}
{{- fail "ERROR: unifai.scim.config.issuerUrl is required when SCIM provider is Okta. Example: https://your-domain.okta.com/oauth2/default" }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: unifai.scim.config.clientId is required when SCIM provider is Okta." }}
{{- end }}
{{- if not $scimValidation.config.clientSecret }}
{{- fail "ERROR: unifai.scim.config.clientSecret is required when SCIM provider is Okta." }}
{{- end }}
{{- if not $scimValidation.config.apiToken }}
{{- fail "ERROR: unifai.scim.config.apiToken is required when SCIM provider is Okta." }}
{{- end }}
{{- end }}
{{- if eq $scimValidation.provider "entra" }}
{{- if not $scimValidation.config.tenantId }}
{{- fail "ERROR: unifai.scim.config.tenantId is required when SCIM provider is Entra (Azure AD)." }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: unifai.scim.config.clientId is required when SCIM provider is Entra (Azure AD)." }}
{{- end }}
{{- end }}
{{- if eq $scimValidation.provider "keycloak" }}
{{- if not $scimValidation.config.serverUrl }}
{{- fail "ERROR: unifai.scim.config.serverUrl is required when SCIM provider is Keycloak. Example: https://keycloak.company.com (must NOT include /realms/{realm})." }}
{{- end }}
{{- if not $scimValidation.config.realm }}
{{- fail "ERROR: unifai.scim.config.realm is required when SCIM provider is Keycloak." }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: unifai.scim.config.clientId is required when SCIM provider is Keycloak." }}
{{- end }}
{{- if not $scimValidation.config.clientSecret }}
{{- fail "ERROR: unifai.scim.config.clientSecret is required when SCIM provider is Keycloak." }}
{{- end }}
{{- end }}
{{- if eq $scimValidation.provider "zitadel" }}
{{- if not $scimValidation.config.domain }}
{{- fail "ERROR: unifai.scim.config.domain is required when SCIM provider is Zitadel. Example: my-instance.zitadel.cloud (no scheme)." }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: unifai.scim.config.clientId is required when SCIM provider is Zitadel." }}
{{- end }}
{{- end }}
{{- if eq $scimValidation.provider "google" }}
{{- if not $scimValidation.config.domain }}
{{- fail "ERROR: unifai.scim.config.domain is required when SCIM provider is Google Workspace. Example: company.com" }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: unifai.scim.config.clientId is required when SCIM provider is Google Workspace." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate cluster config when enabled */}}
{{- if and .Values.unifai.cluster .Values.unifai.cluster.enabled }}
{{- if not .Values.unifai.cluster.gossip }}
{{- fail "ERROR: unifai.cluster.gossip is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.unifai.cluster.gossip.port }}
{{- fail "ERROR: unifai.cluster.gossip.port is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.unifai.cluster.gossip.config }}
{{- fail "ERROR: unifai.cluster.gossip.config is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.unifai.cluster.gossip.config.timeoutSeconds }}
{{- fail "ERROR: unifai.cluster.gossip.config.timeoutSeconds is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.unifai.cluster.gossip.config.successThreshold }}
{{- fail "ERROR: unifai.cluster.gossip.config.successThreshold is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.unifai.cluster.gossip.config.failureThreshold }}
{{- fail "ERROR: unifai.cluster.gossip.config.failureThreshold is required when cluster mode is enabled." }}
{{- end }}
{{- if and .Values.unifai.cluster.discovery .Values.unifai.cluster.discovery.enabled }}
{{- if not .Values.unifai.cluster.discovery.type }}
{{- fail "ERROR: unifai.cluster.discovery.type is required when cluster discovery is enabled. Supported types: kubernetes, dns, udp, consul, etcd, mdns" }}
{{- end }}
{{- if eq .Values.unifai.cluster.discovery.type "udp" }}
{{- if not .Values.unifai.cluster.discovery.udpBroadcastPort }}
{{- fail "ERROR: unifai.cluster.discovery.udpBroadcastPort is required when using udp discovery." }}
{{- end }}
{{- if not .Values.unifai.cluster.discovery.allowedAddressSpace }}
{{- fail "ERROR: unifai.cluster.discovery.allowedAddressSpace is required when using udp discovery." }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate RBAC pod discovery + service account configuration */}}
{{- if and .Values.rbac .Values.rbac.podDiscovery .Values.rbac.podDiscovery.enabled }}
{{- if and .Values.unifai.cluster.enabled .Values.unifai.cluster.discovery.enabled (eq .Values.unifai.cluster.discovery.type "kubernetes") }}
{{- if and (not .Values.serviceAccount.create) (not .Values.serviceAccount.name) }}
{{- fail "ERROR: rbac.podDiscovery.enabled requires either serviceAccount.create=true or an explicit serviceAccount.name when serviceAccount.create=false." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate external Weaviate when vector store type is weaviate */}}
{{- if and .Values.vectorStore.enabled (eq .Values.vectorStore.type "weaviate") }}
{{- if .Values.vectorStore.weaviate.external.enabled }}
{{- if not .Values.vectorStore.weaviate.external.scheme }}
{{- fail "ERROR: vectorStore.weaviate.external.scheme is required when using external Weaviate. Values: http or https" }}
{{- end }}
{{- if not .Values.vectorStore.weaviate.external.host }}
{{- fail "ERROR: vectorStore.weaviate.external.host is required when using external Weaviate." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate external Redis when vector store type is redis */}}
{{- if and .Values.vectorStore.enabled (eq .Values.vectorStore.type "redis") }}
{{- if .Values.vectorStore.redis.external.enabled }}
{{- if not .Values.vectorStore.redis.external.host }}
{{- fail "ERROR: vectorStore.redis.external.host is required when using external Redis." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate external Qdrant when vector store type is qdrant */}}
{{- if and .Values.vectorStore.enabled (eq .Values.vectorStore.type "qdrant") }}
{{- if .Values.vectorStore.qdrant.external.enabled }}
{{- if not .Values.vectorStore.qdrant.external.host }}
{{- fail "ERROR: vectorStore.qdrant.external.host is required when using external Qdrant." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate external PostgreSQL when enabled */}}
{{- if .Values.postgresql.external.enabled }}
{{- if not .Values.postgresql.external.host }}
{{- fail "ERROR: postgresql.external.host is required when using external PostgreSQL." }}
{{- end }}
{{- if not .Values.postgresql.external.database }}
{{- fail "ERROR: postgresql.external.database is required when using external PostgreSQL." }}
{{- end }}
{{- if not .Values.postgresql.external.user }}
{{- fail "ERROR: postgresql.external.user is required when using external PostgreSQL." }}
{{- end }}
{{- if not .Values.postgresql.external.sslMode }}
{{- fail "ERROR: postgresql.external.sslMode is required when using external PostgreSQL. Values: disable, allow, prefer, require, verify-ca, verify-full" }}
{{- end }}
{{- end }}

{{/* Validate governance budgets */}}
{{- if .Values.unifai.governance.budgets }}
{{- range $idx, $budget := .Values.unifai.governance.budgets }}
{{- if not $budget.id }}
{{- fail (printf "ERROR: unifai.governance.budgets[%d].id is required." $idx) }}
{{- end }}
{{- if not $budget.max_limit }}
{{- fail (printf "ERROR: unifai.governance.budgets[%d].max_limit is required for budget '%s'." $idx $budget.id) }}
{{- end }}
{{- if not $budget.reset_duration }}
{{- fail (printf "ERROR: unifai.governance.budgets[%d].reset_duration is required for budget '%s'. Example values: 30s, 5m, 1h, 1d, 1w, 1M, 1Y" $idx $budget.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance rate limits */}}
{{- if .Values.unifai.governance.rateLimits }}
{{- range $idx, $rl := .Values.unifai.governance.rateLimits }}
{{- if not $rl.id }}
{{- fail (printf "ERROR: unifai.governance.rateLimits[%d].id is required." $idx) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance customers */}}
{{- if .Values.unifai.governance.customers }}
{{- range $idx, $customer := .Values.unifai.governance.customers }}
{{- if not $customer.id }}
{{- fail (printf "ERROR: unifai.governance.customers[%d].id is required." $idx) }}
{{- end }}
{{- if not $customer.name }}
{{- fail (printf "ERROR: unifai.governance.customers[%d].name is required for customer '%s'." $idx $customer.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance teams */}}
{{- if .Values.unifai.governance.teams }}
{{- range $idx, $team := .Values.unifai.governance.teams }}
{{- if not $team.id }}
{{- fail (printf "ERROR: unifai.governance.teams[%d].id is required." $idx) }}
{{- end }}
{{- if not $team.name }}
{{- fail (printf "ERROR: unifai.governance.teams[%d].name is required for team '%s'." $idx $team.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance business units */}}
{{- if .Values.unifai.governance.businessUnits }}
{{- range $idx, $bu := .Values.unifai.governance.businessUnits }}
{{- if not $bu.id }}
{{- fail (printf "ERROR: unifai.governance.businessUnits[%d].id is required." $idx) }}
{{- end }}
{{- if not $bu.name }}
{{- fail (printf "ERROR: unifai.governance.businessUnits[%d].name is required for business unit '%s'." $idx $bu.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance virtual keys */}}
{{- if .Values.unifai.governance.virtualKeys }}
{{- range $idx, $vk := .Values.unifai.governance.virtualKeys }}
{{- if not $vk.id }}
{{- fail (printf "ERROR: unifai.governance.virtualKeys[%d].id is required." $idx) }}
{{- end }}
{{- if not $vk.name }}
{{- fail (printf "ERROR: unifai.governance.virtualKeys[%d].name is required for virtual key '%s'." $idx $vk.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance roles */}}
{{- if .Values.unifai.governance.roles }}
{{- range $idx, $role := .Values.unifai.governance.roles }}
{{- if not $role.name }}
{{- fail (printf "ERROR: unifai.governance.roles[%d].name is required." $idx) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate guardrails rules */}}
{{- if .Values.unifai.guardrails.rules }}
{{- range $idx, $rule := .Values.unifai.guardrails.rules }}
{{- if not $rule.id }}
{{- fail (printf "ERROR: unifai.guardrails.rules[%d].id is required." $idx) }}
{{- end }}
{{- if not $rule.name }}
{{- fail (printf "ERROR: unifai.guardrails.rules[%d].name is required for rule id '%v'." $idx $rule.id) }}
{{- end }}
{{- if not (hasKey $rule "enabled") }}
{{- fail (printf "ERROR: unifai.guardrails.rules[%d].enabled is required for rule '%s'." $idx $rule.name) }}
{{- end }}
{{- if not $rule.cel_expression }}
{{- fail (printf "ERROR: unifai.guardrails.rules[%d].cel_expression is required for rule '%s'." $idx $rule.name) }}
{{- end }}
{{- if not $rule.apply_to }}
{{- fail (printf "ERROR: unifai.guardrails.rules[%d].apply_to is required for rule '%s'. Values: input, output, both" $idx $rule.name) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate guardrails providers */}}
{{- if .Values.unifai.guardrails.providers }}
{{- range $idx, $provider := .Values.unifai.guardrails.providers }}
{{- if not $provider.id }}
{{- fail (printf "ERROR: unifai.guardrails.providers[%d].id is required." $idx) }}
{{- end }}
{{- if not $provider.provider_name }}
{{- fail (printf "ERROR: unifai.guardrails.providers[%d].provider_name is required for provider id '%v'." $idx $provider.id) }}
{{- end }}
{{- if not $provider.policy_name }}
{{- fail (printf "ERROR: unifai.guardrails.providers[%d].policy_name is required for provider '%s'." $idx $provider.provider_name) }}
{{- end }}
{{- if not (hasKey $provider "enabled") }}
{{- fail (printf "ERROR: unifai.guardrails.providers[%d].enabled is required for provider '%s'." $idx $provider.provider_name) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate MCP client configs when MCP is enabled */}}
{{- if .Values.unifai.mcp.enabled }}
{{- if .Values.unifai.mcp.clientConfigs }}
{{- range $idx, $client := .Values.unifai.mcp.clientConfigs }}
{{- if not $client.name }}
{{- fail (printf "ERROR: unifai.mcp.clientConfigs[%d].name is required." $idx) }}
{{- end }}
{{- if not $client.connectionType }}
{{- fail (printf "ERROR: unifai.mcp.clientConfigs[%d].connectionType is required for client '%s'. Values: stdio, websocket, http" $idx $client.name) }}
{{- end }}
{{- if eq $client.connectionType "stdio" }}
{{- if not $client.stdioConfig }}
{{- fail (printf "ERROR: unifai.mcp.clientConfigs[%d].stdioConfig is required when connectionType is 'stdio' for client '%s'." $idx $client.name) }}
{{- end }}
{{- if not $client.stdioConfig.command }}
{{- fail (printf "ERROR: unifai.mcp.clientConfigs[%d].stdioConfig.command is required for client '%s'." $idx $client.name) }}
{{- end }}
{{- end }}
{{- if eq $client.connectionType "websocket" }}
{{- if not $client.websocketConfig }}
{{- fail (printf "ERROR: unifai.mcp.clientConfigs[%d].websocketConfig is required when connectionType is 'websocket' for client '%s'." $idx $client.name) }}
{{- end }}
{{- if not $client.websocketConfig.url }}
{{- fail (printf "ERROR: unifai.mcp.clientConfigs[%d].websocketConfig.url is required for client '%s'." $idx $client.name) }}
{{- end }}
{{- end }}
{{- if eq $client.connectionType "http" }}
{{- if not $client.httpConfig }}
{{- fail (printf "ERROR: unifai.mcp.clientConfigs[%d].httpConfig is required when connectionType is 'http' for client '%s'." $idx $client.name) }}
{{- end }}
{{- if not $client.httpConfig.url }}
{{- fail (printf "ERROR: unifai.mcp.clientConfigs[%d].httpConfig.url is required for client '%s'." $idx $client.name) }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- if .Values.unifai.mcp.toolGroups }}
{{- range $idx, $group := .Values.unifai.mcp.toolGroups }}
{{- if not $group.name }}
{{- fail (printf "ERROR: unifai.mcp.toolGroups[%d].name is required." $idx) }}
{{- end }}
{{- if not $group.tools }}
{{- fail (printf "ERROR: unifai.mcp.toolGroups[%d].tools is required for group '%s'." $idx $group.name) }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate custom plugins */}}
{{- if .Values.unifai.plugins.custom }}
{{- range $idx, $plugin := .Values.unifai.plugins.custom }}
{{- if not $plugin.name }}
{{- fail (printf "ERROR: unifai.plugins.custom[%d].name is required." $idx) }}
{{- end }}
{{- if not (hasKey $plugin "enabled") }}
{{- fail (printf "ERROR: unifai.plugins.custom[%d].enabled is required for plugin '%s'." $idx $plugin.name) }}
{{- end }}
{{- if and (hasKey $plugin "version") (lt (int $plugin.version) 1) }}
{{- fail (printf "ERROR: unifai.plugins.custom[%d].version must be >= 1 for plugin '%s'." $idx $plugin.name) }}
{{- end }}
{{- if and (hasKey $plugin "version") (gt (int $plugin.version) 32767) }}
{{- fail (printf "ERROR: unifai.plugins.custom[%d].version must be <= 32767 for plugin '%s'." $idx $plugin.name) }}
{{- end }}
{{- end }}
{{- end }}

{{- end -}}
