{{- define "gateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gateway.fullname" -}}
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

{{- define "gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gateway.labels" -}}
helm.sh/chart: {{ include "gateway.chart" . }}
{{ include "gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "gateway.serverSelectorLabels" -}}
{{ include "gateway.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{- define "gateway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "gateway.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "gateway.postgresql.host" -}}
{{- if .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.host }}
{{- else }}
{{- printf "%s-postgresql" (include "gateway.fullname" .) }}
{{- end }}
{{- end }}

{{- define "gateway.postgresql.port" -}}
{{- if .Values.postgresql.external.enabled -}}
{{- .Values.postgresql.external.port -}}
{{- else -}}
5432
{{- end -}}
{{- end -}}

{{- define "gateway.postgresql.database" -}}
{{- if .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.database }}
{{- else }}
{{- .Values.postgresql.auth.database }}
{{- end }}
{{- end }}

{{- define "gateway.postgresql.username" -}}
{{- if .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.user }}
{{- else }}
{{- .Values.postgresql.auth.username }}
{{- end }}
{{- end }}

{{- define "gateway.postgresql.password" -}}
{{- if .Values.postgresql.external.enabled -}}
{{- if .Values.postgresql.external.existingSecret -}}
env.GATEWAY_POSTGRES_PASSWORD
{{- else -}}
{{- .Values.postgresql.external.password -}}
{{- end -}}
{{- else -}}
{{- if .Values.postgresql.auth.existingSecret -}}
env.GATEWAY_POSTGRES_PASSWORD
{{- else -}}
{{- .Values.postgresql.auth.password -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "gateway.postgresql.sslMode" -}}
{{- if .Values.postgresql.external.enabled -}}
{{- .Values.postgresql.external.sslMode -}}
{{- else -}}
disable
{{- end -}}
{{- end -}}

{{- define "gateway.weaviate.host" -}}
{{- if .Values.vectorStore.weaviate.external.enabled }}
{{- .Values.vectorStore.weaviate.external.host }}
{{- else }}
{{- printf "%s-weaviate" (include "gateway.fullname" .) }}
{{- end }}
{{- end }}

{{- define "gateway.weaviate.scheme" -}}
{{- if .Values.vectorStore.weaviate.external.enabled -}}
{{- .Values.vectorStore.weaviate.external.scheme -}}
{{- else -}}
http
{{- end -}}
{{- end -}}

{{- define "gateway.weaviate.apiKey" -}}
{{- if .Values.vectorStore.weaviate.external.enabled -}}
{{- if .Values.vectorStore.weaviate.external.existingSecret -}}
env.GATEWAY_WEAVIATE_API_KEY
{{- else -}}
{{- .Values.vectorStore.weaviate.external.apiKey -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "gateway.redis.host" -}}
{{- if .Values.vectorStore.redis.external.enabled }}
{{- .Values.vectorStore.redis.external.host }}
{{- else }}
{{- printf "%s-redis-master" (include "gateway.fullname" .) }}
{{- end }}
{{- end }}

{{- define "gateway.redis.port" -}}
{{- if .Values.vectorStore.redis.external.enabled -}}
{{- .Values.vectorStore.redis.external.port -}}
{{- else -}}
6379
{{- end -}}
{{- end -}}

{{- define "gateway.redis.password" -}}
{{- if .Values.vectorStore.redis.external.enabled -}}
{{- if .Values.vectorStore.redis.external.existingSecret -}}
env.GATEWAY_REDIS_PASSWORD
{{- else -}}
{{- .Values.vectorStore.redis.external.password -}}
{{- end -}}
{{- else -}}
{{- .Values.vectorStore.redis.auth.password -}}
{{- end -}}
{{- end -}}

{{- define "gateway.qdrant.host" -}}
{{- if .Values.vectorStore.qdrant.external.enabled }}
{{- .Values.vectorStore.qdrant.external.host }}
{{- else }}
{{- printf "%s-qdrant" (include "gateway.fullname" .) }}
{{- end }}
{{- end }}

{{- define "gateway.qdrant.port" -}}
{{- if .Values.vectorStore.qdrant.external.enabled -}}
{{- .Values.vectorStore.qdrant.external.port -}}
{{- else -}}
6334
{{- end -}}
{{- end -}}

{{- define "gateway.qdrant.apiKey" -}}
{{- if .Values.vectorStore.qdrant.external.enabled -}}
{{- if .Values.vectorStore.qdrant.external.existingSecret -}}
env.GATEWAY_QDRANT_API_KEY
{{- else -}}
{{- .Values.vectorStore.qdrant.external.apiKey -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "gateway.pinecone.apiKey" -}}
{{- if .Values.vectorStore.pinecone.external.enabled -}}
{{- if .Values.vectorStore.pinecone.external.existingSecret -}}
env.GATEWAY_PINECONE_API_KEY
{{- else -}}
{{- .Values.vectorStore.pinecone.external.apiKey -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "gateway.qdrant.useTls" -}}
{{- if .Values.vectorStore.qdrant.external.enabled -}}
{{- .Values.vectorStore.qdrant.external.useTls -}}
{{- else -}}
false
{{- end -}}
{{- end -}}

{{- define "gateway.config" -}}
{{- $config := dict "$schema" "https://www.getgateway.ai/schema" }}
{{- if .Values.gateway.sourceOfTruth }}
{{- $_ := set $config "source_of_truth" .Values.gateway.sourceOfTruth }}
{{- end }}
{{- if .Values.gateway.encryptionKeySecret.name }}
{{- $_ := set $config "encryption_key" "env.GATEWAY_ENCRYPTION_KEY" }}
{{- else if .Values.gateway.encryptionKey }}
{{- $_ := set $config "encryption_key" .Values.gateway.encryptionKey }}
{{- end }}
{{- if .Values.gateway.envLabel }}
{{- $_ := set $config "env_label" .Values.gateway.envLabel }}
{{- end }}
{{- if .Values.gateway.client }}
{{- $client := dict }}
{{- if hasKey .Values.gateway.client "dropExcessRequests" }}
{{- $_ := set $client "drop_excess_requests" .Values.gateway.client.dropExcessRequests }}
{{- end }}
{{- if .Values.gateway.client.initialPoolSize }}
{{- $_ := set $client "initial_pool_size" .Values.gateway.client.initialPoolSize }}
{{- end }}
{{- if .Values.gateway.client.allowedOrigins }}
{{- $_ := set $client "allowed_origins" .Values.gateway.client.allowedOrigins }}
{{- end }}
{{- if hasKey .Values.gateway.client "enableLogging" }}
{{- $_ := set $client "enable_logging" .Values.gateway.client.enableLogging }}
{{- end }}
{{- if hasKey .Values.gateway.client "enforceAuthOnInference" }}
{{- $_ := set $client "enforce_auth_on_inference" .Values.gateway.client.enforceAuthOnInference }}
{{- end }}
{{- if hasKey .Values.gateway.client "enforceGovernanceHeader" }}
{{- $_ := set $client "enforce_governance_header" .Values.gateway.client.enforceGovernanceHeader }}
{{- end }}
{{- if .Values.gateway.client.maxRequestBodySizeMb }}
{{- $_ := set $client "max_request_body_size_mb" .Values.gateway.client.maxRequestBodySizeMb }}
{{- end }}
{{- if .Values.gateway.client.compat }}
{{- $compat := dict }}
{{- if hasKey .Values.gateway.client.compat "convertTextToChat" }}
{{- $_ := set $compat "convert_text_to_chat" .Values.gateway.client.compat.convertTextToChat }}
{{- end }}
{{- if hasKey .Values.gateway.client.compat "convertChatToResponses" }}
{{- $_ := set $compat "convert_chat_to_responses" .Values.gateway.client.compat.convertChatToResponses }}
{{- end }}
{{- if hasKey .Values.gateway.client.compat "shouldDropParams" }}
{{- $_ := set $compat "should_drop_params" .Values.gateway.client.compat.shouldDropParams }}
{{- end }}
{{- if hasKey .Values.gateway.client.compat "shouldConvertParams" }}
{{- $_ := set $compat "should_convert_params" .Values.gateway.client.compat.shouldConvertParams }}
{{- end }}
{{- $_ := set $client "compat" $compat }}
{{- end }}
{{- if .Values.gateway.client.prometheusLabels }}
{{- $_ := set $client "prometheus_labels" .Values.gateway.client.prometheusLabels }}
{{- end }}
{{- if hasKey .Values.gateway.client "disableContentLogging" }}
{{- $_ := set $client "disable_content_logging" .Values.gateway.client.disableContentLogging }}
{{- end }}
{{- if hasKey .Values.gateway.client "allowPerRequestContentStorageOverride" }}
{{- $_ := set $client "allow_per_request_content_storage_override" .Values.gateway.client.allowPerRequestContentStorageOverride }}
{{- end }}
{{- if hasKey .Values.gateway.client "allowPerRequestRawOverride" }}
{{- $_ := set $client "allow_per_request_raw_override" .Values.gateway.client.allowPerRequestRawOverride }}
{{- end }}
{{- if .Values.gateway.client.logRetentionDays }}
{{- $_ := set $client "log_retention_days" .Values.gateway.client.logRetentionDays }}
{{- end }}
{{- if hasKey .Values.gateway.client "disableDbPingsInHealth" }}
{{- $_ := set $client "disable_db_pings_in_health" .Values.gateway.client.disableDbPingsInHealth }}
{{- end }}
{{- if hasKey .Values.gateway.client "dumpErrorsInConsoleLogs" }}
{{- $_ := set $client "dump_errors_in_console_logs" .Values.gateway.client.dumpErrorsInConsoleLogs }}
{{- end }}
{{- if .Values.gateway.client.headerFilterConfig }}
{{- $headerFilter := dict }}
{{- if .Values.gateway.client.headerFilterConfig.allowlist }}
{{- $_ := set $headerFilter "allowlist" .Values.gateway.client.headerFilterConfig.allowlist }}
{{- end }}
{{- if .Values.gateway.client.headerFilterConfig.denylist }}
{{- $_ := set $headerFilter "denylist" .Values.gateway.client.headerFilterConfig.denylist }}
{{- end }}
{{- if or $headerFilter.allowlist $headerFilter.denylist }}
{{- $_ := set $client "header_filter_config" $headerFilter }}
{{- end }}
{{- end }}
{{- if .Values.gateway.client.asyncJobResultTTL }}
{{- $_ := set $client "async_job_result_ttl" .Values.gateway.client.asyncJobResultTTL }}
{{- end }}
{{- if .Values.gateway.client.requiredHeaders }}
{{- $_ := set $client "required_headers" .Values.gateway.client.requiredHeaders }}
{{- end }}
{{- if .Values.gateway.client.loggingHeaders }}
{{- $_ := set $client "logging_headers" .Values.gateway.client.loggingHeaders }}
{{- end }}
{{- if .Values.gateway.client.whitelistedRoutes }}
{{- $_ := set $client "whitelisted_routes" .Values.gateway.client.whitelistedRoutes }}
{{- end }}
{{- if .Values.gateway.client.allowedHeaders }}
{{- $_ := set $client "allowed_headers" .Values.gateway.client.allowedHeaders }}
{{- end }}
{{- if .Values.gateway.client.mcpAgentDepth }}
{{- $_ := set $client "mcp_agent_depth" .Values.gateway.client.mcpAgentDepth }}
{{- end }}
{{- if .Values.gateway.client.mcpToolExecutionTimeout }}
{{- $_ := set $client "mcp_tool_execution_timeout" .Values.gateway.client.mcpToolExecutionTimeout }}
{{- end }}
{{- if .Values.gateway.client.mcpCodeModeBindingLevel }}
{{- $_ := set $client "mcp_code_mode_binding_level" .Values.gateway.client.mcpCodeModeBindingLevel }}
{{- end }}
{{- if hasKey .Values.gateway.client "mcpToolSyncInterval" }}
{{- $_ := set $client "mcp_tool_sync_interval" .Values.gateway.client.mcpToolSyncInterval }}
{{- end }}
{{- if hasKey .Values.gateway.client "hideDeletedVirtualKeysInFilters" }}
{{- $_ := set $client "hide_deleted_virtual_keys_in_filters" .Values.gateway.client.hideDeletedVirtualKeysInFilters }}
{{- end }}
{{- if hasKey .Values.gateway.client "mcpDisableAutoToolInject" }}
{{- $_ := set $client "mcp_disable_auto_tool_inject" .Values.gateway.client.mcpDisableAutoToolInject }}
{{- end }}
{{- if hasKey .Values.gateway.client "mcpEnableTempTokenAuth" }}
{{- $_ := set $client "mcp_enable_temp_token_auth" .Values.gateway.client.mcpEnableTempTokenAuth }}
{{- end }}
{{- if .Values.gateway.client.routingChainMaxDepth }}
{{- $_ := set $client "routing_chain_max_depth" .Values.gateway.client.routingChainMaxDepth }}
{{- end }}
{{- if hasKey .Values.gateway.client "allowDirectKeys" }}
{{- $_ := set $client "allow_direct_keys" .Values.gateway.client.allowDirectKeys }}
{{- end }}
{{- if .Values.gateway.client.mcpExternalClientUrl }}
{{- $_ := set $client "mcp_external_client_url" .Values.gateway.client.mcpExternalClientUrl }}
{{- end }}
{{- $_ := set $config "client" $client }}
{{- end }}
{{- /* Server */ -}}
{{- if .Values.gateway.server }}
{{- $server := dict }}
{{- if .Values.gateway.server.readBufferSize }}
{{- $_ := set $server "read_buffer_size" .Values.gateway.server.readBufferSize }}
{{- end }}
{{- if $server }}
{{- $_ := set $config "server" $server }}
{{- end }}
{{- end }}
{{- /* Framework */ -}}
{{- if .Values.gateway.framework }}
{{- $framework := dict }}
{{- if .Values.gateway.framework.pricing }}
{{- $pricing := dict }}
{{- if .Values.gateway.framework.pricing.pricingUrl }}
{{- $_ := set $pricing "pricing_url" .Values.gateway.framework.pricing.pricingUrl }}
{{- end }}
{{- if .Values.gateway.framework.pricing.modelParametersUrl }}
{{- $_ := set $pricing "model_parameters_url" .Values.gateway.framework.pricing.modelParametersUrl }}
{{- end }}
{{- if .Values.gateway.framework.pricing.pricingSyncInterval }}
{{- $_ := set $pricing "pricing_sync_interval" .Values.gateway.framework.pricing.pricingSyncInterval }}
{{- end }}
{{- if .Values.gateway.framework.pricing.mcpLibraryUrl }}
{{- $_ := set $pricing "mcp_library_url" .Values.gateway.framework.pricing.mcpLibraryUrl }}
{{- end }}
{{- if .Values.gateway.framework.pricing.mcpLibrarySyncInterval }}
{{- $_ := set $pricing "mcp_library_sync_interval" .Values.gateway.framework.pricing.mcpLibrarySyncInterval }}
{{- end }}
{{- if or $pricing.pricing_url $pricing.model_parameters_url $pricing.pricing_sync_interval $pricing.mcp_library_url $pricing.mcp_library_sync_interval }}
{{- $_ := set $framework "pricing" $pricing }}
{{- end }}
{{- end }}
{{- if $framework }}
{{- $_ := set $config "framework" $framework }}
{{- end }}
{{- end }}
{{- if .Values.gateway.providers }}
{{- $providers := dict }}
{{- range $providerName, $providerConfig := .Values.gateway.providers }}
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
{{- if .Values.gateway.governance }}
{{- $governance := dict }}
{{- if .Values.gateway.governance.budgets }}
{{- $_ := set $governance "budgets" .Values.gateway.governance.budgets }}
{{- end }}
{{- if .Values.gateway.governance.rateLimits }}
{{- $rateLimits := list }}
{{- range .Values.gateway.governance.rateLimits }}
{{- $rl := dict "id" .id }}
{{- if .token_max_limit }}{{- $_ := set $rl "token_max_limit" .token_max_limit }}{{- end }}
{{- if .token_reset_duration }}{{- $_ := set $rl "token_reset_duration" .token_reset_duration }}{{- end }}
{{- if .request_max_limit }}{{- $_ := set $rl "request_max_limit" .request_max_limit }}{{- end }}
{{- if .request_reset_duration }}{{- $_ := set $rl "request_reset_duration" .request_reset_duration }}{{- end }}
{{- $rateLimits = append $rateLimits $rl }}
{{- end }}
{{- $_ := set $governance "rate_limits" $rateLimits }}
{{- end }}
{{- if .Values.gateway.governance.customers }}
{{- $_ := set $governance "customers" .Values.gateway.governance.customers }}
{{- end }}
{{- if .Values.gateway.governance.teams }}
{{- $_ := set $governance "teams" .Values.gateway.governance.teams }}
{{- end }}
{{- if .Values.gateway.governance.businessUnits }}
{{- $businessUnits := list }}
{{- range .Values.gateway.governance.businessUnits }}
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
{{- if .Values.gateway.governance.roles }}
{{- $roles := list }}
{{- range .Values.gateway.governance.roles }}
{{- $role := dict "name" .name }}
{{- if .description }}{{- $_ := set $role "description" .description }}{{- end }}
{{- if .dac }}{{- $_ := set $role "dac" .dac }}{{- end }}
{{- if .access_profile }}{{- $_ := set $role "access_profile" .access_profile }}{{- end }}
{{- if .permissions }}{{- $_ := set $role "permissions" .permissions }}{{- end }}
{{- $roles = append $roles $role }}
{{- end }}
{{- $_ := set $governance "roles" $roles }}
{{- end }}
{{- if .Values.gateway.governance.virtualKeys }}
{{- $vks := list }}
{{- range .Values.gateway.governance.virtualKeys }}
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
{{- if .Values.gateway.governance.routingRules }}
{{- $_ := set $governance "routing_rules" .Values.gateway.governance.routingRules }}
{{- end }}
{{- if .Values.gateway.governance.modelConfigs }}
{{- $_ := set $governance "model_configs" .Values.gateway.governance.modelConfigs }}
{{- end }}
{{- if .Values.gateway.governance.providers }}
{{- $_ := set $governance "providers" .Values.gateway.governance.providers }}
{{- end }}
{{- if .Values.gateway.governance.pricingOverrides }}
{{- $_ := set $governance "pricing_overrides" .Values.gateway.governance.pricingOverrides }}
{{- end }}
{{- if .Values.gateway.governance.complexityAnalyzerConfig }}
{{- $_ := set $governance "complexity_analyzer_config" .Values.gateway.governance.complexityAnalyzerConfig }}
{{- end }}
{{- if .Values.gateway.governance.authConfig }}
{{- $authConfig := dict }}
{{- if and .Values.gateway.governance.authConfig.existingSecret .Values.gateway.governance.authConfig.usernameKey }}
{{- $_ := set $authConfig "admin_username" "env.GATEWAY_ADMIN_USERNAME" }}
{{- else if .Values.gateway.governance.authConfig.adminUsername }}
{{- $_ := set $authConfig "admin_username" .Values.gateway.governance.authConfig.adminUsername }}
{{- end }}
{{- if and .Values.gateway.governance.authConfig.existingSecret .Values.gateway.governance.authConfig.passwordKey }}
{{- $_ := set $authConfig "admin_password" "env.GATEWAY_ADMIN_PASSWORD" }}
{{- else if .Values.gateway.governance.authConfig.adminPassword }}
{{- $_ := set $authConfig "admin_password" .Values.gateway.governance.authConfig.adminPassword }}
{{- end }}
{{- if hasKey .Values.gateway.governance.authConfig "isEnabled" }}
{{- $_ := set $authConfig "is_enabled" .Values.gateway.governance.authConfig.isEnabled }}
{{- end }}
{{- if hasKey .Values.gateway.governance.authConfig "disableAuthOnInference" }}
{{- $_ := set $authConfig "disable_auth_on_inference" .Values.gateway.governance.authConfig.disableAuthOnInference }}
{{- end }}
{{- if or $authConfig.admin_username $authConfig.admin_password $authConfig.is_enabled $authConfig.disable_auth_on_inference }}
{{- $_ := set $governance "auth_config" $authConfig }}
{{- end }}
{{- end }}
{{- if or $governance.budgets $governance.rate_limits $governance.customers $governance.teams $governance.business_units $governance.roles $governance.virtual_keys $governance.routing_rules $governance.model_configs $governance.providers $governance.pricing_overrides $governance.complexity_analyzer_config $governance.auth_config }}
{{- $_ := set $config "governance" $governance }}
{{- end }}
{{- end }}
{{- /* Top-level Auth Config - for main Gateway authentication */ -}}
{{- if .Values.gateway.authConfig }}
{{- $authConfig := dict }}
{{- /* Only use env var reference if governance auth secret is NOT already configured (to avoid referencing uninjected env vars) */ -}}
{{- if and .Values.gateway.authConfig.existingSecret .Values.gateway.authConfig.usernameKey (not (and .Values.gateway.governance .Values.gateway.governance.authConfig .Values.gateway.governance.authConfig.existingSecret)) }}
{{- $_ := set $authConfig "admin_username" "env.GATEWAY_ADMIN_USERNAME" }}
{{- else if .Values.gateway.authConfig.adminUsername }}
{{- $_ := set $authConfig "admin_username" .Values.gateway.authConfig.adminUsername }}
{{- end }}
{{- if and .Values.gateway.authConfig.existingSecret .Values.gateway.authConfig.passwordKey (not (and .Values.gateway.governance .Values.gateway.governance.authConfig .Values.gateway.governance.authConfig.existingSecret)) }}
{{- $_ := set $authConfig "admin_password" "env.GATEWAY_ADMIN_PASSWORD" }}
{{- else if .Values.gateway.authConfig.adminPassword }}
{{- $_ := set $authConfig "admin_password" .Values.gateway.authConfig.adminPassword }}
{{- end }}
{{- if hasKey .Values.gateway.authConfig "isEnabled" }}
{{- $_ := set $authConfig "is_enabled" .Values.gateway.authConfig.isEnabled }}
{{- end }}
{{- if hasKey .Values.gateway.authConfig "disableAuthOnInference" }}
{{- $_ := set $authConfig "disable_auth_on_inference" .Values.gateway.authConfig.disableAuthOnInference }}
{{- end }}
{{- if or $authConfig.admin_username $authConfig.admin_password $authConfig.is_enabled $authConfig.disable_auth_on_inference }}
{{- $_ := set $config "auth_config" $authConfig }}
{{- end }}
{{- end }}
{{- /* Cluster Config */ -}}
{{- if and .Values.gateway.cluster .Values.gateway.cluster.enabled }}
{{- $cluster := dict "enabled" true }}
{{- if .Values.gateway.cluster.peers }}
{{- $_ := set $cluster "peers" .Values.gateway.cluster.peers }}
{{- end }}
{{- if .Values.gateway.cluster.region }}
{{- $_ := set $cluster "region" .Values.gateway.cluster.region }}
{{- end }}
{{- if .Values.gateway.cluster.gossip }}
{{- $gossip := dict }}
{{- if .Values.gateway.cluster.gossip.port }}
{{- $_ := set $gossip "port" .Values.gateway.cluster.gossip.port }}
{{- end }}
{{- if .Values.gateway.cluster.gossip.config }}
{{- $gossipConfig := dict }}
{{- if .Values.gateway.cluster.gossip.config.timeoutSeconds }}
{{- $_ := set $gossipConfig "timeout_seconds" .Values.gateway.cluster.gossip.config.timeoutSeconds }}
{{- end }}
{{- if .Values.gateway.cluster.gossip.config.successThreshold }}
{{- $_ := set $gossipConfig "success_threshold" .Values.gateway.cluster.gossip.config.successThreshold }}
{{- end }}
{{- if .Values.gateway.cluster.gossip.config.failureThreshold }}
{{- $_ := set $gossipConfig "failure_threshold" .Values.gateway.cluster.gossip.config.failureThreshold }}
{{- end }}
{{- $_ := set $gossip "config" $gossipConfig }}
{{- end }}
{{- $_ := set $cluster "gossip" $gossip }}
{{- end }}
{{- if .Values.gateway.cluster.grpc }}
{{- $grpc := dict }}
{{- if .Values.gateway.cluster.grpc.port }}
{{- $_ := set $grpc "port" .Values.gateway.cluster.grpc.port }}
{{- end }}
{{- if .Values.gateway.cluster.grpc.dialTimeoutSeconds }}
{{- $_ := set $grpc "dial_timeout_seconds" .Values.gateway.cluster.grpc.dialTimeoutSeconds }}
{{- end }}
{{- if $grpc }}
{{- $_ := set $cluster "grpc" $grpc }}
{{- end }}
{{- end }}
{{- if and .Values.gateway.cluster.discovery .Values.gateway.cluster.discovery.enabled }}
{{- $discovery := dict "enabled" true "type" .Values.gateway.cluster.discovery.type }}
{{- $serviceName := .Values.gateway.cluster.discovery.serviceName }}
{{- if and (not $serviceName) (or (eq .Values.gateway.cluster.discovery.type "consul") (eq .Values.gateway.cluster.discovery.type "etcd") (eq .Values.gateway.cluster.discovery.type "udp")) }}
{{- fail "ERROR: gateway.cluster.discovery.serviceName is required for consul/etcd/udp discovery." }}
{{- end }}
{{- if $serviceName }}
{{- $_ := set $discovery "service_name" $serviceName }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.bindPort }}
{{- $_ := set $discovery "bind_port" .Values.gateway.cluster.discovery.bindPort }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.dialTimeout }}
{{- $_ := set $discovery "dial_timeout" .Values.gateway.cluster.discovery.dialTimeout }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.allowedAddressSpace }}
{{- $_ := set $discovery "allowed_address_space" .Values.gateway.cluster.discovery.allowedAddressSpace }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.k8sNamespace }}
{{- $_ := set $discovery "k8s_namespace" .Values.gateway.cluster.discovery.k8sNamespace }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.k8sLabelSelector }}
{{- $_ := set $discovery "k8s_label_selector" .Values.gateway.cluster.discovery.k8sLabelSelector }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.dnsNames }}
{{- $_ := set $discovery "dns_names" .Values.gateway.cluster.discovery.dnsNames }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.udpBroadcastPort }}
{{- $_ := set $discovery "udp_broadcast_port" .Values.gateway.cluster.discovery.udpBroadcastPort }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.consulAddress }}
{{- $_ := set $discovery "consul_address" .Values.gateway.cluster.discovery.consulAddress }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.etcdEndpoints }}
{{- $_ := set $discovery "etcd_endpoints" .Values.gateway.cluster.discovery.etcdEndpoints }}
{{- end }}
{{- if .Values.gateway.cluster.discovery.mdnsService }}
{{- $_ := set $discovery "mdns_service" .Values.gateway.cluster.discovery.mdnsService }}
{{- end }}
{{- $_ := set $cluster "discovery" $discovery }}
{{- end }}
{{- $_ := set $config "cluster_config" $cluster }}
{{- end }}
{{- /* SCIM Config */ -}}
{{- $scimValues := .Values.gateway.scim }}
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
{{- if and .Values.gateway.loadBalancer .Values.gateway.loadBalancer.enabled }}
{{- $lb := dict "enabled" true }}
{{- if hasKey .Values.gateway.loadBalancer "directionSelectionEnabled" }}
{{- $_ := set $lb "direction_selection_enabled" .Values.gateway.loadBalancer.directionSelectionEnabled }}
{{- end }}
{{- if hasKey .Values.gateway.loadBalancer "routeSelectionEnabled" }}
{{- $_ := set $lb "route_selection_enabled" .Values.gateway.loadBalancer.routeSelectionEnabled }}
{{- end }}
{{- if hasKey .Values.gateway.loadBalancer "rerouteFailedDirections" }}
{{- $_ := set $lb "reroute_failed_directions" .Values.gateway.loadBalancer.rerouteFailedDirections }}
{{- end }}
{{- if hasKey .Values.gateway.loadBalancer "pruneFailedFallbacks" }}
{{- $_ := set $lb "prune_failed_fallbacks" .Values.gateway.loadBalancer.pruneFailedFallbacks }}
{{- end }}
{{- if .Values.gateway.loadBalancer.trackerConfig }}
{{- $_ := set $lb "tracker_config" .Values.gateway.loadBalancer.trackerConfig }}
{{- end }}
{{- if .Values.gateway.loadBalancer.bootstrap }}
{{- $_ := set $lb "bootstrap" .Values.gateway.loadBalancer.bootstrap }}
{{- end }}
{{- $_ := set $config "load_balancer_config" $lb }}
{{- end }}
{{- /* Guardrails Config */ -}}
{{- if .Values.gateway.guardrails }}
{{- $guardrails := dict }}
{{- if .Values.gateway.guardrails.rules }}
{{- $rules := list }}
{{- range .Values.gateway.guardrails.rules }}
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
{{- if .Values.gateway.guardrails.providers }}
{{- $providers := list }}
{{- range .Values.gateway.guardrails.providers }}
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
{{- if .Values.gateway.skillsRegistry }}
{{- $_ := set $config "skills_registry" .Values.gateway.skillsRegistry }}
{{- end }}
{{- /* Access Profiles (Enterprise) */ -}}
{{- if .Values.gateway.accessProfiles }}
{{- $_ := set $config "access_profiles" .Values.gateway.accessProfiles }}
{{- end }}
{{- /* Config Store */ -}}
{{- if .Values.storage.configStore.enabled }}
{{- $configStoreType := .Values.storage.configStore.type | default .Values.storage.mode }}
{{- if eq $configStoreType "postgres" }}
{{- $pgConfig := dict "host" (include "gateway.postgresql.host" .) "port" (include "gateway.postgresql.port" .) "db_name" (include "gateway.postgresql.database" .) "user" (include "gateway.postgresql.username" .) "password" (include "gateway.postgresql.password" .) "ssl_mode" (include "gateway.postgresql.sslMode" .) }}
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
{{- $sqliteConfigStore := dict "enabled" true "type" "sqlite" "config" (dict "path" (printf "%s/config.db" .Values.gateway.appDir)) }}
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
{{- $pgConfig := dict "host" (include "gateway.postgresql.host" .) "port" (include "gateway.postgresql.port" .) "db_name" (include "gateway.postgresql.database" .) "user" (include "gateway.postgresql.username" .) "password" (include "gateway.postgresql.password" .) "ssl_mode" (include "gateway.postgresql.sslMode" .) }}
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
{{- $sqliteLogsStore := dict "enabled" true "type" "sqlite" "config" (dict "path" (printf "%s/logs.db" .Values.gateway.appDir)) }}
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
{{- $_ := set $osConfig "access_key_id" "env.GATEWAY_OBJECT_STORAGE_ACCESS_KEY_ID" }}
{{- end }}
{{- if $os.secretAccessKeyKey }}
{{- $_ := set $osConfig "secret_access_key" "env.GATEWAY_OBJECT_STORAGE_SECRET_ACCESS_KEY" }}
{{- end }}
{{- if $os.sessionTokenKey }}
{{- $_ := set $osConfig "session_token" "env.GATEWAY_OBJECT_STORAGE_SESSION_TOKEN" }}
{{- end }}
{{- $_ := set $osConfig "role_arn" "env.GATEWAY_OBJECT_STORAGE_ROLE_ARN" }}
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
{{- $_ := set $osConfig "credentials_json" "env.GATEWAY_OBJECT_STORAGE_CREDENTIALS_JSON" }}
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
{{- $weaviateConfig := dict "scheme" (include "gateway.weaviate.scheme" .) "host" (include "gateway.weaviate.host" .) }}
{{- if .Values.vectorStore.weaviate.external.enabled }}
{{- $weaviateApiKey := include "gateway.weaviate.apiKey" . }}
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
{{- $redisConfig := dict "addr" (printf "%s:%s" (include "gateway.redis.host" .) (include "gateway.redis.port" .)) }}
{{- $password := include "gateway.redis.password" . }}
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
{{- $qdrantConfig := dict "host" (include "gateway.qdrant.host" .) "port" (include "gateway.qdrant.port" . | int) }}
{{- $apiKey := include "gateway.qdrant.apiKey" . }}
{{- if $apiKey }}
{{- $_ := set $qdrantConfig "api_key" $apiKey }}
{{- end }}
{{- $useTls := include "gateway.qdrant.useTls" . }}
{{- if eq $useTls "true" }}
{{- $_ := set $qdrantConfig "use_tls" true }}
{{- else }}
{{- $_ := set $qdrantConfig "use_tls" false }}
{{- end }}
{{- $_ := set $vectorStore "config" $qdrantConfig }}
{{- else if eq .Values.vectorStore.type "pinecone" }}
{{- $pineconeConfig := dict }}
{{- $apiKey := include "gateway.pinecone.apiKey" . }}
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
{{- if .Values.gateway.mcp.enabled }}
{{- $clientConfigs := list }}
{{- range $idx, $client := .Values.gateway.mcp.clientConfigs }}
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
{{- $envName := printf "GATEWAY_MCP_%s_CONNECTION_STRING" (regexReplaceAll "[^A-Z0-9]+" (upper $client.name) "_") }}
{{- $_ := set $cc "connection_string" (printf "env.%s" $envName) }}
{{- end }}
{{- $clientConfigs = append $clientConfigs $cc }}
{{- end }}
{{- $mcpConfig := dict "client_configs" $clientConfigs }}
{{- if .Values.gateway.mcp.toolManagerConfig }}
{{- $tmConfig := dict }}
{{- if .Values.gateway.mcp.toolManagerConfig.toolExecutionTimeout }}
{{- $_ := set $tmConfig "tool_execution_timeout" .Values.gateway.mcp.toolManagerConfig.toolExecutionTimeout }}
{{- end }}
{{- if .Values.gateway.mcp.toolManagerConfig.maxAgentDepth }}
{{- $_ := set $tmConfig "max_agent_depth" .Values.gateway.mcp.toolManagerConfig.maxAgentDepth }}
{{- end }}
{{- if .Values.gateway.mcp.toolManagerConfig.codeModeBindingLevel }}
{{- $_ := set $tmConfig "code_mode_binding_level" .Values.gateway.mcp.toolManagerConfig.codeModeBindingLevel }}
{{- end }}
{{- if hasKey .Values.gateway.mcp.toolManagerConfig "disableAutoToolInject" }}
{{- $_ := set $tmConfig "disable_auto_tool_inject" .Values.gateway.mcp.toolManagerConfig.disableAutoToolInject }}
{{- end }}
{{- if $tmConfig }}
{{- $_ := set $mcpConfig "tool_manager_config" $tmConfig }}
{{- end }}
{{- end }}
{{- if hasKey .Values.gateway.mcp "toolSyncInterval" }}
{{- $_ := set $mcpConfig "tool_sync_interval" .Values.gateway.mcp.toolSyncInterval }}
{{- end }}
{{- if .Values.gateway.mcp.toolGroups }}
{{- $toolGroups := list }}
{{- range .Values.gateway.mcp.toolGroups }}
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
{{- if .Values.gateway.plugins.telemetry.enabled }}
{{- $plugin := dict "enabled" true "name" "telemetry" "config" .Values.gateway.plugins.telemetry.config }}
{{- if hasKey .Values.gateway.plugins.telemetry "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.telemetry.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.gateway.plugins.logging.enabled }}
{{- $plugin := dict "enabled" true "name" "logging" "config" .Values.gateway.plugins.logging.config }}
{{- if hasKey .Values.gateway.plugins.logging "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.logging.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.gateway.plugins.governance.enabled }}
{{- $governanceConfig := dict }}
{{- if hasKey .Values.gateway.plugins.governance.config "is_vk_mandatory" }}
{{- $_ := set $governanceConfig "is_vk_mandatory" .Values.gateway.plugins.governance.config.is_vk_mandatory }}
{{- end }}
{{- if .Values.gateway.plugins.governance.config.required_headers }}
{{- $_ := set $governanceConfig "required_headers" .Values.gateway.plugins.governance.config.required_headers }}
{{- end }}
{{- if hasKey .Values.gateway.plugins.governance.config "is_enterprise" }}
{{- $_ := set $governanceConfig "is_enterprise" .Values.gateway.plugins.governance.config.is_enterprise }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "governance" "config" $governanceConfig }}
{{- if hasKey .Values.gateway.plugins.governance "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.governance.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.gateway.plugins.maxim.enabled }}
{{- $maximConfig := dict }}
{{- if and .Values.gateway.plugins.maxim.secretRef .Values.gateway.plugins.maxim.secretRef.name }}
{{- $_ := set $maximConfig "api_key" "env.GATEWAY_MAXIM_API_KEY" }}
{{- else if .Values.gateway.plugins.maxim.config.api_key }}
{{- $_ := set $maximConfig "api_key" .Values.gateway.plugins.maxim.config.api_key }}
{{- end }}
{{- if .Values.gateway.plugins.maxim.config.log_repo_id }}
{{- $_ := set $maximConfig "log_repo_id" .Values.gateway.plugins.maxim.config.log_repo_id }}
{{- end }}
{{- $plugin := dict "enabled" true "name" "maxim" "config" $maximConfig }}
{{- if hasKey .Values.gateway.plugins.maxim "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.maxim.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.gateway.plugins.semanticCache.enabled }}
{{- $scConfig := dict }}
{{- $inputConfig := .Values.gateway.plugins.semanticCache.config | default dict }}
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
{{- if hasKey .Values.gateway.plugins.semanticCache "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.semanticCache.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.gateway.plugins.otel.enabled }}
{{- $otelConfig := dict }}
{{- $inputConfig := .Values.gateway.plugins.otel.config | default dict }}
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
{{- if hasKey .Values.gateway.plugins.otel "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.otel.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.gateway.plugins.datadog.enabled }}
{{- $datadogConfig := dict }}
{{- $inputConfig := .Values.gateway.plugins.datadog.config | default dict }}
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
{{- if hasKey .Values.gateway.plugins.datadog "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.datadog.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if .Values.gateway.plugins.bigquery.enabled }}
{{- $bigqueryConfig := dict }}
{{- $inputConfig := .Values.gateway.plugins.bigquery.config | default dict }}
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
{{- if hasKey .Values.gateway.plugins.bigquery "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.bigquery.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if (.Values.gateway.plugins.kafka).enabled }}
{{- $kafkaConfig := dict }}
{{- $inputConfig := .Values.gateway.plugins.kafka.config | default dict }}
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
{{- if hasKey .Values.gateway.plugins.kafka "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.kafka.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- if (.Values.gateway.plugins.pubsub).enabled }}
{{- $pubsubConfig := dict }}
{{- $inputConfig := .Values.gateway.plugins.pubsub.config | default dict }}
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
{{- if hasKey .Values.gateway.plugins.pubsub "version" }}{{- $_ := set $plugin "version" (.Values.gateway.plugins.pubsub.version | int) }}{{- end }}
{{- $plugins = append $plugins $plugin }}
{{- end }}
{{- /* Custom plugins */ -}}
{{- if .Values.gateway.plugins.custom }}
{{- range .Values.gateway.plugins.custom }}
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
{{- if .Values.gateway.auditLogs }}
{{- $auditLogs := dict }}
{{- if hasKey .Values.gateway.auditLogs "disabled" }}
{{- $_ := set $auditLogs "disabled" .Values.gateway.auditLogs.disabled }}
{{- end }}
{{- if .Values.gateway.auditLogs.hmacKey }}
{{- $_ := set $auditLogs "hmac_key" .Values.gateway.auditLogs.hmacKey }}
{{- end }}
{{- if or (hasKey $auditLogs "disabled") $auditLogs.hmac_key }}
{{- $_ := set $config "audit_logs" $auditLogs }}
{{- end }}
{{- end }}
{{- /* Large Payload Optimization */ -}}
{{- if .Values.gateway.largePayloadOptimization }}
{{- $lpo := dict }}
{{- if hasKey .Values.gateway.largePayloadOptimization "enabled" }}
{{- $_ := set $lpo "enabled" .Values.gateway.largePayloadOptimization.enabled }}
{{- end }}
{{- if hasKey .Values.gateway.largePayloadOptimization "requestThresholdBytes" }}
{{- $_ := set $lpo "request_threshold_bytes" .Values.gateway.largePayloadOptimization.requestThresholdBytes }}
{{- end }}
{{- if hasKey .Values.gateway.largePayloadOptimization "responseThresholdBytes" }}
{{- $_ := set $lpo "response_threshold_bytes" .Values.gateway.largePayloadOptimization.responseThresholdBytes }}
{{- end }}
{{- if hasKey .Values.gateway.largePayloadOptimization "prefetchSizeBytes" }}
{{- $_ := set $lpo "prefetch_size_bytes" .Values.gateway.largePayloadOptimization.prefetchSizeBytes }}
{{- end }}
{{- if hasKey .Values.gateway.largePayloadOptimization "maxPayloadBytes" }}
{{- $_ := set $lpo "max_payload_bytes" .Values.gateway.largePayloadOptimization.maxPayloadBytes }}
{{- end }}
{{- if hasKey .Values.gateway.largePayloadOptimization "truncatedLogBytes" }}
{{- $_ := set $lpo "truncated_log_bytes" .Values.gateway.largePayloadOptimization.truncatedLogBytes }}
{{- end }}
{{- if $lpo }}
{{- $_ := set $config "large_payload_optimization" $lpo }}
{{- end }}
{{- end }}
{{- /* WebSocket Config */ -}}
{{- if .Values.gateway.websocket }}
{{- $ws := dict }}
{{- if .Values.gateway.websocket.maxConnectionsPerUser }}
{{- $_ := set $ws "max_connections_per_user" .Values.gateway.websocket.maxConnectionsPerUser }}
{{- end }}
{{- if .Values.gateway.websocket.transcriptBufferSize }}
{{- $_ := set $ws "transcript_buffer_size" .Values.gateway.websocket.transcriptBufferSize }}
{{- end }}
{{- if .Values.gateway.websocket.pool }}
{{- $pool := dict }}
{{- if .Values.gateway.websocket.pool.maxIdlePerKey }}
{{- $_ := set $pool "max_idle_per_key" .Values.gateway.websocket.pool.maxIdlePerKey }}
{{- end }}
{{- if .Values.gateway.websocket.pool.maxTotalConnections }}
{{- $_ := set $pool "max_total_connections" .Values.gateway.websocket.pool.maxTotalConnections }}
{{- end }}
{{- if .Values.gateway.websocket.pool.idleTimeoutSeconds }}
{{- $_ := set $pool "idle_timeout_seconds" .Values.gateway.websocket.pool.idleTimeoutSeconds }}
{{- end }}
{{- if .Values.gateway.websocket.pool.maxConnectionLifetimeSeconds }}
{{- $_ := set $pool "max_connection_lifetime_seconds" .Values.gateway.websocket.pool.maxConnectionLifetimeSeconds }}
{{- end }}
{{- if $pool }}
{{- $_ := set $ws "pool" $pool }}
{{- end }}
{{- end }}
{{- if $ws }}
{{- $_ := set $config "websocket" $ws }}
{{- end }}
{{- end }}
{{- if .Values.gateway.featureFlags }}
{{- $flags := dict }}
{{- range $name, $cfg := .Values.gateway.featureFlags }}
{{- if not (kindIs "map" $cfg) }}
{{- fail (printf "ERROR: gateway.featureFlags.%s must be an object with an 'enabled' field." $name) }}
{{- end }}
{{- if not (hasKey $cfg "enabled") }}
{{- fail (printf "ERROR: gateway.featureFlags.%s.enabled is required." $name) }}
{{- end }}
{{- $_ := set $flags $name (dict "enabled" $cfg.enabled) }}
{{- end }}
{{- if $flags }}
{{- $_ := set $config "feature_flags" (dict "flags" $flags) }}
{{- end }}
{{- end }}
{{- /* Circuit Breaker Config */ -}}
{{- if .Values.gateway.circuitBreakerConfig }}
{{- $_ := set $config "circuit_breaker_config" .Values.gateway.circuitBreakerConfig }}
{{- end }}
{{- $config | toJson }}
{{- end }}

{{/*
Validation template - validates required fields from config.schema.json
Call this template at the beginning of deployment/stateful templates
*/}}
{{- define "gateway.validate" -}}

{{/* Validate gateway.sourceOfTruth enum */}}
{{- if .Values.gateway.sourceOfTruth }}
{{- if and (ne .Values.gateway.sourceOfTruth "split") (ne .Values.gateway.sourceOfTruth "config.json") }}
{{- fail (printf "ERROR: gateway.sourceOfTruth must be 'split' or 'config.json', got: %s" .Values.gateway.sourceOfTruth) }}
{{- end }}
{{- end }}

{{/* Validate semantic cache plugin when enabled */}}
{{- if and .Values.gateway.plugins.telemetry.enabled (hasKey .Values.gateway.plugins.telemetry "version") (lt (int .Values.gateway.plugins.telemetry.version) 1) }}
{{- fail "ERROR: gateway.plugins.telemetry.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.gateway.plugins.telemetry.enabled (hasKey .Values.gateway.plugins.telemetry "version") (gt (int .Values.gateway.plugins.telemetry.version) 32767) }}
{{- fail "ERROR: gateway.plugins.telemetry.version must be <= 32767." }}
{{- end }}
{{- if and .Values.gateway.plugins.logging.enabled (hasKey .Values.gateway.plugins.logging "version") (lt (int .Values.gateway.plugins.logging.version) 1) }}
{{- fail "ERROR: gateway.plugins.logging.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.gateway.plugins.logging.enabled (hasKey .Values.gateway.plugins.logging "version") (gt (int .Values.gateway.plugins.logging.version) 32767) }}
{{- fail "ERROR: gateway.plugins.logging.version must be <= 32767." }}
{{- end }}
{{- if and .Values.gateway.plugins.governance.enabled (hasKey .Values.gateway.plugins.governance "version") (lt (int .Values.gateway.plugins.governance.version) 1) }}
{{- fail "ERROR: gateway.plugins.governance.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.gateway.plugins.governance.enabled (hasKey .Values.gateway.plugins.governance "version") (gt (int .Values.gateway.plugins.governance.version) 32767) }}
{{- fail "ERROR: gateway.plugins.governance.version must be <= 32767." }}
{{- end }}
{{- if and .Values.gateway.plugins.maxim.enabled (hasKey .Values.gateway.plugins.maxim "version") (lt (int .Values.gateway.plugins.maxim.version) 1) }}
{{- fail "ERROR: gateway.plugins.maxim.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.gateway.plugins.maxim.enabled (hasKey .Values.gateway.plugins.maxim "version") (gt (int .Values.gateway.plugins.maxim.version) 32767) }}
{{- fail "ERROR: gateway.plugins.maxim.version must be <= 32767." }}
{{- end }}
{{- if and .Values.gateway.plugins.semanticCache.enabled (hasKey .Values.gateway.plugins.semanticCache "version") (lt (int .Values.gateway.plugins.semanticCache.version) 1) }}
{{- fail "ERROR: gateway.plugins.semanticCache.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.gateway.plugins.semanticCache.enabled (hasKey .Values.gateway.plugins.semanticCache "version") (gt (int .Values.gateway.plugins.semanticCache.version) 32767) }}
{{- fail "ERROR: gateway.plugins.semanticCache.version must be <= 32767." }}
{{- end }}
{{- if and .Values.gateway.plugins.otel.enabled (hasKey .Values.gateway.plugins.otel "version") (lt (int .Values.gateway.plugins.otel.version) 1) }}
{{- fail "ERROR: gateway.plugins.otel.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.gateway.plugins.otel.enabled (hasKey .Values.gateway.plugins.otel "version") (gt (int .Values.gateway.plugins.otel.version) 32767) }}
{{- fail "ERROR: gateway.plugins.otel.version must be <= 32767." }}
{{- end }}
{{- if and .Values.gateway.plugins.datadog.enabled (hasKey .Values.gateway.plugins.datadog "version") (lt (int .Values.gateway.plugins.datadog.version) 1) }}
{{- fail "ERROR: gateway.plugins.datadog.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.gateway.plugins.datadog.enabled (hasKey .Values.gateway.plugins.datadog "version") (gt (int .Values.gateway.plugins.datadog.version) 32767) }}
{{- fail "ERROR: gateway.plugins.datadog.version must be <= 32767." }}
{{- end }}
{{- $ddCfg := (.Values.gateway.plugins.datadog.config | default dict) }}
{{- if and .Values.gateway.plugins.datadog.enabled $ddCfg.agentless (not $ddCfg.api_key) }}
{{- fail "ERROR: gateway.plugins.datadog.config.api_key is required when gateway.plugins.datadog.config.agentless is true." }}
{{- end }}
{{- if and .Values.gateway.plugins.bigquery.enabled (hasKey .Values.gateway.plugins.bigquery "version") (lt (int .Values.gateway.plugins.bigquery.version) 1) }}
{{- fail "ERROR: gateway.plugins.bigquery.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and .Values.gateway.plugins.bigquery.enabled (hasKey .Values.gateway.plugins.bigquery "version") (gt (int .Values.gateway.plugins.bigquery.version) 32767) }}
{{- fail "ERROR: gateway.plugins.bigquery.version must be <= 32767." }}
{{- end }}
{{- if and .Values.gateway.plugins.bigquery.enabled (not (.Values.gateway.plugins.bigquery.config | default dict).project_id) }}
{{- fail "ERROR: gateway.plugins.bigquery.config.project_id is required when the BigQuery plugin is enabled." }}
{{- end }}
{{- if and (.Values.gateway.plugins.kafka).enabled (hasKey .Values.gateway.plugins.kafka "version") (lt (int .Values.gateway.plugins.kafka.version) 1) }}
{{- fail "ERROR: gateway.plugins.kafka.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and (.Values.gateway.plugins.kafka).enabled (hasKey .Values.gateway.plugins.kafka "version") (gt (int .Values.gateway.plugins.kafka.version) 32767) }}
{{- fail "ERROR: gateway.plugins.kafka.version must be <= 32767." }}
{{- end }}
{{- if (.Values.gateway.plugins.kafka).enabled }}
{{- $kafkaInputConfig := .Values.gateway.plugins.kafka.config | default dict }}
{{- if not $kafkaInputConfig.brokers }}
{{- fail "ERROR: gateway.plugins.kafka.config.brokers is required when the Kafka plugin is enabled." }}
{{- end }}
{{- if not $kafkaInputConfig.topic }}
{{- fail "ERROR: gateway.plugins.kafka.config.topic is required when the Kafka plugin is enabled." }}
{{- end }}
{{- end }}
{{- if and (.Values.gateway.plugins.pubsub).enabled (hasKey .Values.gateway.plugins.pubsub "version") (lt (int .Values.gateway.plugins.pubsub.version) 1) }}
{{- fail "ERROR: gateway.plugins.pubsub.version must be >= 1. Bump to >1 to force DB-backed plugin config updates." }}
{{- end }}
{{- if and (.Values.gateway.plugins.pubsub).enabled (hasKey .Values.gateway.plugins.pubsub "version") (gt (int .Values.gateway.plugins.pubsub.version) 32767) }}
{{- fail "ERROR: gateway.plugins.pubsub.version must be <= 32767." }}
{{- end }}
{{- if (.Values.gateway.plugins.pubsub).enabled }}
{{- $pubsubInputConfig := .Values.gateway.plugins.pubsub.config | default dict }}
{{- if not $pubsubInputConfig.project_id }}
{{- fail "ERROR: gateway.plugins.pubsub.config.project_id is required when the Pub/Sub plugin is enabled." }}
{{- end }}
{{- if not $pubsubInputConfig.topic_id }}
{{- fail "ERROR: gateway.plugins.pubsub.config.topic_id is required when the Pub/Sub plugin is enabled." }}
{{- end }}
{{- end }}

{{/* Validate semantic cache plugin when enabled */}}
{{- if .Values.gateway.plugins.semanticCache.enabled }}
{{/* When dimension is 1, direct (hash-based) caching is used — provider and keys are not required. */}}
{{- if ne (int .Values.gateway.plugins.semanticCache.config.dimension) 1 }}
{{- if not .Values.gateway.plugins.semanticCache.config.provider }}
{{- fail "ERROR: gateway.plugins.semanticCache.config.provider is required for semantic caching. Supported providers: openai, anthropic, gemini, bedrock, azure, cohere, mistral, groq, ollama, openrouter, vertex, cerebras, parasail, perplexity, sgl, huggingface. For direct (hash-based) caching, set dimension: 1." }}
{{- end }}
{{- if not .Values.gateway.plugins.semanticCache.config.keys }}
{{- fail "ERROR: gateway.plugins.semanticCache.config.keys is required for semantic caching. Provide at least one API key for the embedding provider. For direct (hash-based) caching, set dimension: 1." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate OTEL plugin when enabled */}}
{{- if .Values.gateway.plugins.otel.enabled }}
{{- $otelInputConfig := .Values.gateway.plugins.otel.config | default dict }}
{{- if hasKey $otelInputConfig "profiles" }}
{{- if not $otelInputConfig.profiles }}
{{- fail "ERROR: gateway.plugins.otel.config.profiles must contain at least one profile when OTEL plugin is enabled." }}
{{- end }}
{{- range $idx, $profile := $otelInputConfig.profiles }}
{{- $profileEnabled := true }}
{{- if hasKey $profile "enabled" }}
{{- $profileEnabled = $profile.enabled }}
{{- end }}
{{- if $profileEnabled }}
{{- if not $profile.collector_url }}
{{- fail (printf "ERROR: gateway.plugins.otel.config.profiles[%d].collector_url is required for enabled OTEL profiles." $idx) }}
{{- end }}
{{- if not $profile.trace_type }}
{{- fail (printf "ERROR: gateway.plugins.otel.config.profiles[%d].trace_type is required. Supported values: genai_extension, vercel, open_inference" $idx) }}
{{- end }}
{{- if not $profile.protocol }}
{{- fail (printf "ERROR: gateway.plugins.otel.config.profiles[%d].protocol is required. Supported values: http, grpc" $idx) }}
{{- end }}
{{- if and $profile.metrics_enabled (not $profile.metrics_endpoint) }}
{{- fail (printf "ERROR: gateway.plugins.otel.config.profiles[%d].metrics_endpoint is required when metrics_enabled is true." $idx) }}
{{- end }}
{{- end }}
{{- end }}
{{- else }}
{{- if not $otelInputConfig.collector_url }}
{{- fail "ERROR: gateway.plugins.otel.config.collector_url is required when OTEL plugin is enabled. Provide the URL of your OpenTelemetry collector." }}
{{- end }}
{{- if not $otelInputConfig.trace_type }}
{{- fail "ERROR: gateway.plugins.otel.config.trace_type is required when OTEL plugin is enabled. Supported values: genai_extension, vercel, open_inference" }}
{{- end }}
{{- if not $otelInputConfig.protocol }}
{{- fail "ERROR: gateway.plugins.otel.config.protocol is required when OTEL plugin is enabled. Supported values: http, grpc" }}
{{- end }}
{{- if and $otelInputConfig.metrics_enabled (not $otelInputConfig.metrics_endpoint) }}
{{- fail "ERROR: gateway.plugins.otel.config.metrics_endpoint is required when metrics_enabled is true." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate Maxim plugin when enabled */}}
{{- if .Values.gateway.plugins.maxim.enabled }}
{{- if and (not .Values.gateway.plugins.maxim.config.api_key) (not .Values.gateway.plugins.maxim.secretRef.name) }}
{{- fail "ERROR: gateway.plugins.maxim.config.api_key or gateway.plugins.maxim.secretRef.name is required when Maxim plugin is enabled." }}
{{- end }}
{{- end }}

{{/* Validate SCIM/SSO config when enabled */}}
{{- $scimValidation := .Values.gateway.scim }}
{{- if and $scimValidation $scimValidation.enabled }}
{{- if eq $scimValidation.provider "okta" }}
{{- if not $scimValidation.config.issuerUrl }}
{{- fail "ERROR: gateway.scim.config.issuerUrl is required when SCIM provider is Okta. Example: https://your-domain.okta.com/oauth2/default" }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: gateway.scim.config.clientId is required when SCIM provider is Okta." }}
{{- end }}
{{- if not $scimValidation.config.clientSecret }}
{{- fail "ERROR: gateway.scim.config.clientSecret is required when SCIM provider is Okta." }}
{{- end }}
{{- if not $scimValidation.config.apiToken }}
{{- fail "ERROR: gateway.scim.config.apiToken is required when SCIM provider is Okta." }}
{{- end }}
{{- end }}
{{- if eq $scimValidation.provider "entra" }}
{{- if not $scimValidation.config.tenantId }}
{{- fail "ERROR: gateway.scim.config.tenantId is required when SCIM provider is Entra (Azure AD)." }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: gateway.scim.config.clientId is required when SCIM provider is Entra (Azure AD)." }}
{{- end }}
{{- end }}
{{- if eq $scimValidation.provider "keycloak" }}
{{- if not $scimValidation.config.serverUrl }}
{{- fail "ERROR: gateway.scim.config.serverUrl is required when SCIM provider is Keycloak. Example: https://keycloak.company.com (must NOT include /realms/{realm})." }}
{{- end }}
{{- if not $scimValidation.config.realm }}
{{- fail "ERROR: gateway.scim.config.realm is required when SCIM provider is Keycloak." }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: gateway.scim.config.clientId is required when SCIM provider is Keycloak." }}
{{- end }}
{{- if not $scimValidation.config.clientSecret }}
{{- fail "ERROR: gateway.scim.config.clientSecret is required when SCIM provider is Keycloak." }}
{{- end }}
{{- end }}
{{- if eq $scimValidation.provider "zitadel" }}
{{- if not $scimValidation.config.domain }}
{{- fail "ERROR: gateway.scim.config.domain is required when SCIM provider is Zitadel. Example: my-instance.zitadel.cloud (no scheme)." }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: gateway.scim.config.clientId is required when SCIM provider is Zitadel." }}
{{- end }}
{{- end }}
{{- if eq $scimValidation.provider "google" }}
{{- if not $scimValidation.config.domain }}
{{- fail "ERROR: gateway.scim.config.domain is required when SCIM provider is Google Workspace. Example: company.com" }}
{{- end }}
{{- if not $scimValidation.config.clientId }}
{{- fail "ERROR: gateway.scim.config.clientId is required when SCIM provider is Google Workspace." }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate cluster config when enabled */}}
{{- if and .Values.gateway.cluster .Values.gateway.cluster.enabled }}
{{- if not .Values.gateway.cluster.gossip }}
{{- fail "ERROR: gateway.cluster.gossip is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.gateway.cluster.gossip.port }}
{{- fail "ERROR: gateway.cluster.gossip.port is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.gateway.cluster.gossip.config }}
{{- fail "ERROR: gateway.cluster.gossip.config is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.gateway.cluster.gossip.config.timeoutSeconds }}
{{- fail "ERROR: gateway.cluster.gossip.config.timeoutSeconds is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.gateway.cluster.gossip.config.successThreshold }}
{{- fail "ERROR: gateway.cluster.gossip.config.successThreshold is required when cluster mode is enabled." }}
{{- end }}
{{- if not .Values.gateway.cluster.gossip.config.failureThreshold }}
{{- fail "ERROR: gateway.cluster.gossip.config.failureThreshold is required when cluster mode is enabled." }}
{{- end }}
{{- if and .Values.gateway.cluster.discovery .Values.gateway.cluster.discovery.enabled }}
{{- if not .Values.gateway.cluster.discovery.type }}
{{- fail "ERROR: gateway.cluster.discovery.type is required when cluster discovery is enabled. Supported types: kubernetes, dns, udp, consul, etcd, mdns" }}
{{- end }}
{{- if eq .Values.gateway.cluster.discovery.type "udp" }}
{{- if not .Values.gateway.cluster.discovery.udpBroadcastPort }}
{{- fail "ERROR: gateway.cluster.discovery.udpBroadcastPort is required when using udp discovery." }}
{{- end }}
{{- if not .Values.gateway.cluster.discovery.allowedAddressSpace }}
{{- fail "ERROR: gateway.cluster.discovery.allowedAddressSpace is required when using udp discovery." }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate RBAC pod discovery + service account configuration */}}
{{- if and .Values.rbac .Values.rbac.podDiscovery .Values.rbac.podDiscovery.enabled }}
{{- if and .Values.gateway.cluster.enabled .Values.gateway.cluster.discovery.enabled (eq .Values.gateway.cluster.discovery.type "kubernetes") }}
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
{{- if .Values.gateway.governance.budgets }}
{{- range $idx, $budget := .Values.gateway.governance.budgets }}
{{- if not $budget.id }}
{{- fail (printf "ERROR: gateway.governance.budgets[%d].id is required." $idx) }}
{{- end }}
{{- if not $budget.max_limit }}
{{- fail (printf "ERROR: gateway.governance.budgets[%d].max_limit is required for budget '%s'." $idx $budget.id) }}
{{- end }}
{{- if not $budget.reset_duration }}
{{- fail (printf "ERROR: gateway.governance.budgets[%d].reset_duration is required for budget '%s'. Example values: 30s, 5m, 1h, 1d, 1w, 1M, 1Y" $idx $budget.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance rate limits */}}
{{- if .Values.gateway.governance.rateLimits }}
{{- range $idx, $rl := .Values.gateway.governance.rateLimits }}
{{- if not $rl.id }}
{{- fail (printf "ERROR: gateway.governance.rateLimits[%d].id is required." $idx) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance customers */}}
{{- if .Values.gateway.governance.customers }}
{{- range $idx, $customer := .Values.gateway.governance.customers }}
{{- if not $customer.id }}
{{- fail (printf "ERROR: gateway.governance.customers[%d].id is required." $idx) }}
{{- end }}
{{- if not $customer.name }}
{{- fail (printf "ERROR: gateway.governance.customers[%d].name is required for customer '%s'." $idx $customer.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance teams */}}
{{- if .Values.gateway.governance.teams }}
{{- range $idx, $team := .Values.gateway.governance.teams }}
{{- if not $team.id }}
{{- fail (printf "ERROR: gateway.governance.teams[%d].id is required." $idx) }}
{{- end }}
{{- if not $team.name }}
{{- fail (printf "ERROR: gateway.governance.teams[%d].name is required for team '%s'." $idx $team.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance business units */}}
{{- if .Values.gateway.governance.businessUnits }}
{{- range $idx, $bu := .Values.gateway.governance.businessUnits }}
{{- if not $bu.id }}
{{- fail (printf "ERROR: gateway.governance.businessUnits[%d].id is required." $idx) }}
{{- end }}
{{- if not $bu.name }}
{{- fail (printf "ERROR: gateway.governance.businessUnits[%d].name is required for business unit '%s'." $idx $bu.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance virtual keys */}}
{{- if .Values.gateway.governance.virtualKeys }}
{{- range $idx, $vk := .Values.gateway.governance.virtualKeys }}
{{- if not $vk.id }}
{{- fail (printf "ERROR: gateway.governance.virtualKeys[%d].id is required." $idx) }}
{{- end }}
{{- if not $vk.name }}
{{- fail (printf "ERROR: gateway.governance.virtualKeys[%d].name is required for virtual key '%s'." $idx $vk.id) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate governance roles */}}
{{- if .Values.gateway.governance.roles }}
{{- range $idx, $role := .Values.gateway.governance.roles }}
{{- if not $role.name }}
{{- fail (printf "ERROR: gateway.governance.roles[%d].name is required." $idx) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate guardrails rules */}}
{{- if .Values.gateway.guardrails.rules }}
{{- range $idx, $rule := .Values.gateway.guardrails.rules }}
{{- if not $rule.id }}
{{- fail (printf "ERROR: gateway.guardrails.rules[%d].id is required." $idx) }}
{{- end }}
{{- if not $rule.name }}
{{- fail (printf "ERROR: gateway.guardrails.rules[%d].name is required for rule id '%v'." $idx $rule.id) }}
{{- end }}
{{- if not (hasKey $rule "enabled") }}
{{- fail (printf "ERROR: gateway.guardrails.rules[%d].enabled is required for rule '%s'." $idx $rule.name) }}
{{- end }}
{{- if not $rule.cel_expression }}
{{- fail (printf "ERROR: gateway.guardrails.rules[%d].cel_expression is required for rule '%s'." $idx $rule.name) }}
{{- end }}
{{- if not $rule.apply_to }}
{{- fail (printf "ERROR: gateway.guardrails.rules[%d].apply_to is required for rule '%s'. Values: input, output, both" $idx $rule.name) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate guardrails providers */}}
{{- if .Values.gateway.guardrails.providers }}
{{- range $idx, $provider := .Values.gateway.guardrails.providers }}
{{- if not $provider.id }}
{{- fail (printf "ERROR: gateway.guardrails.providers[%d].id is required." $idx) }}
{{- end }}
{{- if not $provider.provider_name }}
{{- fail (printf "ERROR: gateway.guardrails.providers[%d].provider_name is required for provider id '%v'." $idx $provider.id) }}
{{- end }}
{{- if not $provider.policy_name }}
{{- fail (printf "ERROR: gateway.guardrails.providers[%d].policy_name is required for provider '%s'." $idx $provider.provider_name) }}
{{- end }}
{{- if not (hasKey $provider "enabled") }}
{{- fail (printf "ERROR: gateway.guardrails.providers[%d].enabled is required for provider '%s'." $idx $provider.provider_name) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate MCP client configs when MCP is enabled */}}
{{- if .Values.gateway.mcp.enabled }}
{{- if .Values.gateway.mcp.clientConfigs }}
{{- range $idx, $client := .Values.gateway.mcp.clientConfigs }}
{{- if not $client.name }}
{{- fail (printf "ERROR: gateway.mcp.clientConfigs[%d].name is required." $idx) }}
{{- end }}
{{- if not $client.connectionType }}
{{- fail (printf "ERROR: gateway.mcp.clientConfigs[%d].connectionType is required for client '%s'. Values: stdio, websocket, http" $idx $client.name) }}
{{- end }}
{{- if eq $client.connectionType "stdio" }}
{{- if not $client.stdioConfig }}
{{- fail (printf "ERROR: gateway.mcp.clientConfigs[%d].stdioConfig is required when connectionType is 'stdio' for client '%s'." $idx $client.name) }}
{{- end }}
{{- if not $client.stdioConfig.command }}
{{- fail (printf "ERROR: gateway.mcp.clientConfigs[%d].stdioConfig.command is required for client '%s'." $idx $client.name) }}
{{- end }}
{{- end }}
{{- if eq $client.connectionType "websocket" }}
{{- if not $client.websocketConfig }}
{{- fail (printf "ERROR: gateway.mcp.clientConfigs[%d].websocketConfig is required when connectionType is 'websocket' for client '%s'." $idx $client.name) }}
{{- end }}
{{- if not $client.websocketConfig.url }}
{{- fail (printf "ERROR: gateway.mcp.clientConfigs[%d].websocketConfig.url is required for client '%s'." $idx $client.name) }}
{{- end }}
{{- end }}
{{- if eq $client.connectionType "http" }}
{{- if not $client.httpConfig }}
{{- fail (printf "ERROR: gateway.mcp.clientConfigs[%d].httpConfig is required when connectionType is 'http' for client '%s'." $idx $client.name) }}
{{- end }}
{{- if not $client.httpConfig.url }}
{{- fail (printf "ERROR: gateway.mcp.clientConfigs[%d].httpConfig.url is required for client '%s'." $idx $client.name) }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- if .Values.gateway.mcp.toolGroups }}
{{- range $idx, $group := .Values.gateway.mcp.toolGroups }}
{{- if not $group.name }}
{{- fail (printf "ERROR: gateway.mcp.toolGroups[%d].name is required." $idx) }}
{{- end }}
{{- if not $group.tools }}
{{- fail (printf "ERROR: gateway.mcp.toolGroups[%d].tools is required for group '%s'." $idx $group.name) }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{/* Validate custom plugins */}}
{{- if .Values.gateway.plugins.custom }}
{{- range $idx, $plugin := .Values.gateway.plugins.custom }}
{{- if not $plugin.name }}
{{- fail (printf "ERROR: gateway.plugins.custom[%d].name is required." $idx) }}
{{- end }}
{{- if not (hasKey $plugin "enabled") }}
{{- fail (printf "ERROR: gateway.plugins.custom[%d].enabled is required for plugin '%s'." $idx $plugin.name) }}
{{- end }}
{{- if and (hasKey $plugin "version") (lt (int $plugin.version) 1) }}
{{- fail (printf "ERROR: gateway.plugins.custom[%d].version must be >= 1 for plugin '%s'." $idx $plugin.name) }}
{{- end }}
{{- if and (hasKey $plugin "version") (gt (int $plugin.version) 32767) }}
{{- fail (printf "ERROR: gateway.plugins.custom[%d].version must be <= 32767 for plugin '%s'." $idx $plugin.name) }}
{{- end }}
{{- end }}
{{- end }}

{{- end -}}
