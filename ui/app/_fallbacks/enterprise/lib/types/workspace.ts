export interface AccessProfile {
	id: number;
	name: string;
	description?: string;
	is_active: boolean;
	version: number;
	tags?: string[];
	provider_configs?: { provider_name: string; all_models_allowed?: boolean; allowed_models?: string[]; weight?: number }[];
	budgets?: { max_limit?: number; reset_duration?: string }[];
	rate_limit?: {
		request_max_limit?: number;
		request_reset_duration?: string;
		token_max_limit?: number;
		token_reset_duration?: string;
	};
	calendar_aligned?: boolean;
	virtual_key_ids?: string[];
	mcp_servers?: Record<string, unknown>[];
	mcp_tool_groups?: Record<string, unknown>[];
	mcp_tool_overrides?: Record<string, unknown>[];
	role_ids?: number[];
	created_at: string;
	updated_at?: string;
}

export interface GetAccessProfilesResponse {
	access_profiles: AccessProfile[];
	count: number;
	total_count: number;
}

export interface RBACRole {
	id: number;
	name: string;
	description?: string;
	is_system_role: boolean;
	dac: string;
	permission_ids?: number[];
	created_at?: string;
	updated_at?: string;
}

export interface RBACPermission {
	id: number;
	resource: string;
	operation: string;
}

export interface CircuitBreakerPolicy {
	name: string;
	enabled?: boolean;
	primary_provider: string;
	primary_model: string;
	primary_key_ids?: string[];
	fallback_provider: string;
	fallback_model: string;
	condition: {
		operator: string;
		signals: { source: string; header_name: string; header_value?: string; header_contains?: string }[];
	};
	default_cooldown?: string;
	cooldown_header?: string;
}

export interface CircuitState {
	status: string;
	opened_at: string;
	expires_at: string;
}

export interface AlertChannel {
	id: number;
	name: string;
	type: string;
	enabled: boolean;
	config: Record<string, string>;
	created_at?: string;
	updated_at?: string;
}

export interface AuditLog {
	id: number;
	action: string;
	outcome: string;
	initiator: string;
	target: string;
	method: string;
	path: string;
	ip: string;
	duration_ms: number;
	created_at: string;
}

export interface AuditSettings {
	disabled: boolean;
	retention_days: number;
	hmac_key?: string;
}

export interface ClusterConfig {
	enabled: boolean;
	type: string;
	region: string;
	peers: string[];
	gossip?: { port: number; config?: { timeout_seconds: number; success_threshold: number; failure_threshold: number } };
	grpc?: { port: number; dial_timeout_seconds: number };
	node?: { address: string; mode: string };
}

export interface LoadBalancerConfig {
	enabled: boolean;
	direction_selection_enabled: boolean;
	route_selection_enabled: boolean;
	reroute_failed_directions?: boolean;
	prune_failed_fallbacks?: boolean;
}

export interface LoadBalancerRoute {
	provider: string;
	key_id: string;
	key_name: string;
	weight: number;
	enabled: boolean;
	status: string;
	models?: string[];
}

export interface LoadBalancerDirection {
	provider: string;
	key_count: number;
	status: string;
}

export interface LoadBalancerRoutesResponse {
	config: LoadBalancerConfig;
	directions: LoadBalancerDirection[];
	routes: LoadBalancerRoute[];
}

export interface SCIMConfig {
	enabled: boolean;
	provider: string;
	bearer_token?: string;
	config: Record<string, string>;
}

export interface BusinessUnit {
	id: string;
	name: string;
	team_count?: number;
	team_ids?: string[];
	created_at?: string;
	updated_at?: string;
}

export interface MCPToolGroup {
	id: number;
	name: string;
	description?: string;
	enabled: boolean;
	tools: { mcp_client_id?: string; mcp_client_name?: string; tool_name?: string; tool_names?: string[]; name?: string; id?: string }[];
	virtual_key_ids?: string[];
}

export interface PromptDeployment {
	id: number;
	prompt_id: string;
	prompt_name?: string;
	version_number: number;
	environment: string;
	enabled: boolean;
}

export interface ConnectorConfig {
	name: string;
	enabled: boolean;
	config: Record<string, string>;
}
