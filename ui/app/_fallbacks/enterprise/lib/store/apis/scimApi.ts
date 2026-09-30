import { SCIMConfig } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

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
	}),
});

export const { useGetAuthTypeQuery, useGetSCIMProvidersQuery, useGetSCIMConfigQuery, useUpdateSCIMConfigMutation } = scimApi;
