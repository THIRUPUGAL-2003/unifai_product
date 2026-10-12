import { CreateFeatureFlagRequest, FeatureFlagStatus, FeatureFlagsListResponse, UpdateFeatureFlagRequest } from "@/lib/types/featureFlag";
import { baseApi } from "./baseApi";

export const featureFlagsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		// List all known feature flags (registered + any stale overrides).
		listFeatureFlags: builder.query<FeatureFlagsListResponse, void>({
			query: () => ({
				url: "/feature-flags",
			}),
			providesTags: ["FeatureFlags"],
		}),

		// Toggle a single feature flag.
		updateFeatureFlag: builder.mutation<FeatureFlagStatus, UpdateFeatureFlagRequest>({
			query: ({ id, enabled }) => ({
				url: `/feature-flags/${encodeURIComponent(id)}`,
				method: "PUT",
				body: { enabled },
			}),
			invalidatesTags: ["FeatureFlags"],
		}),

		// Create/register a new feature flag.
		createFeatureFlag: builder.mutation<FeatureFlagStatus, CreateFeatureFlagRequest>({
			query: (body) => ({
				url: "/feature-flags",
				method: "POST",
				body,
			}),
			invalidatesTags: ["FeatureFlags"],
		}),

		// Delete a feature flag override / dynamically registered flag.
		deleteFeatureFlag: builder.mutation<{ status: string; id: string }, string>({
			query: (id) => ({
				url: `/feature-flags/${encodeURIComponent(id)}`,
				method: "DELETE",
			}),
			invalidatesTags: ["FeatureFlags"],
		}),
	}),
});

export const { useListFeatureFlagsQuery, useUpdateFeatureFlagMutation, useCreateFeatureFlagMutation, useDeleteFeatureFlagMutation } =
	featureFlagsApi;