import { SCIMConfig } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export interface ActiveDirectoryConfig {
	enabled: boolean;
	server_url: string;
	domain: string;
	base_dn: string;
	bind_dn: string;
	bind_password?: string;
	use_tls?: boolean;
	insecure_skip_tls?: boolean;
	user_filter?: string;
	username_attr?: string;
	email_attr?: string;
	display_name_attr?: string;
	default_role?: string;
	last_sync_at?: string;
	last_sync_status?: string;
	last_sync_message?: string;
	last_sync_count?: number;
	discovered_users_count?: number;
}

export interface ADTestResponse {
	success: boolean;
	message: string;
	discovered_count?: number;
	sample_users?: string[];
}

export interface ADSyncResponse {
	success: boolean;
	message: string;
	discovered_count?: number;
	created_count?: number;
	existing_count?: number;
	quota_reached?: boolean;
}

export const scimApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getAuthType: builder.query<{ type: string; provider?: string }, void>({
			query: () => ({ url: "/scim/config" }),
			transformResponse: (response: { enabled?: boolean; provider?: string }) => ({
				type: response.enabled ? "sso" : "password",
				provider: response.provider,
			}),
			providesTags: ["AuthType"],
		}),
		getSCIMProviders: builder.query<unknown[], void>({
			query: () => ({ url: "/scim/providers" }),
			transformResponse: (response: unknown) => (Array.isArray(response) ? response : []),
			providesTags: ["SCIMProviders"],
		}),
		getSCIMConfig: builder.query<SCIMConfig, void>({
			query: () => ({ url: "/scim/config" }),
			providesTags: ["SCIMProviders"],
		}),
		updateSCIMConfig: builder.mutation<SCIMConfig, SCIMConfig>({
			query: (body) => ({ url: "/scim/config", method: "PUT", body }),
			invalidatesTags: ["SCIMProviders", "AuthType"],
		}),
		getActiveDirectoryConfig: builder.query<ActiveDirectoryConfig, void>({
			query: () => ({ url: "/scim/ad/config" }),
			providesTags: ["SCIMProviders"],
		}),
		updateActiveDirectoryConfig: builder.mutation<ActiveDirectoryConfig, ActiveDirectoryConfig>({
			query: (body) => ({ url: "/scim/ad/config", method: "PUT", body }),
			invalidatesTags: ["SCIMProviders"],
		}),
		testActiveDirectoryConnection: builder.mutation<ADTestResponse, ActiveDirectoryConfig>({
			query: (body) => ({ url: "/scim/ad/test", method: "POST", body }),
		}),
		syncActiveDirectoryUsers: builder.mutation<ADSyncResponse, void>({
			query: () => ({ url: "/scim/ad/sync", method: "POST" }),
			invalidatesTags: ["SCIMProviders", "Users", "UserGovernance"],
		}),
	}),
});

export const {
	useGetAuthTypeQuery,
	useGetSCIMProvidersQuery,
	useGetSCIMConfigQuery,
	useUpdateSCIMConfigMutation,
	useGetActiveDirectoryConfigQuery,
	useUpdateActiveDirectoryConfigMutation,
	useTestActiveDirectoryConnectionMutation,
	useSyncActiveDirectoryUsersMutation,
} = scimApi;
