import { GetUserAccessProfilesResponse } from "@enterprise/lib/types/accessProfile";
import { AccessProfile, GetAccessProfilesResponse } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const accessProfilesApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getAccessProfiles: builder.query<GetAccessProfilesResponse, { search?: string } | void>({
			query: (params) => ({
				url: "/access-profiles",
				params: params?.search ? { search: params.search } : undefined,
			}),
			providesTags: ["AccessProfiles"],
		}),
		createAccessProfile: builder.mutation<AccessProfile, Partial<AccessProfile>>({
			query: (body) => ({ url: "/access-profiles", method: "POST", body }),
			transformResponse: (response: { access_profile: AccessProfile }) => response.access_profile,
			invalidatesTags: ["AccessProfiles"],
		}),
		activateAccessProfile: builder.mutation<void, { id: number; activate: boolean }>({
			query: ({ id, activate }) => ({
				url: `/access-profiles/${id}/${activate ? "activate" : "deactivate"}`,
				method: "POST",
			}),
			invalidatesTags: ["AccessProfiles"],
		}),
		cloneAccessProfile: builder.mutation<AccessProfile, number>({
			query: (id) => ({ url: `/access-profiles/${id}/clone`, method: "POST", body: {} }),
			transformResponse: (response: { access_profile: AccessProfile }) => response.access_profile,
			invalidatesTags: ["AccessProfiles"],
		}),
		deleteAccessProfile: builder.mutation<void, number>({
			query: (id) => ({ url: `/access-profiles/${id}`, method: "DELETE" }),
			invalidatesTags: ["AccessProfiles"],
		}),
		getAccessProfile: builder.query<{ access_profile: AccessProfile }, number>({
			query: (id) => ({ url: `/access-profiles/${id}` }),
			providesTags: (_result, _error, id) => [{ type: "AccessProfiles", id }],
		}),
		updateAccessProfile: builder.mutation<AccessProfile, { id: number; updates: Partial<AccessProfile> }>({
			query: ({ id, updates }) => ({ url: `/access-profiles/${id}`, method: "PUT", body: updates }),
			transformResponse: (response: { access_profile: AccessProfile }) => response.access_profile,
			invalidatesTags: ["AccessProfiles"],
		}),
		getUserAccessProfiles: builder.query<GetUserAccessProfilesResponse, string>({
			query: (userId) => ({ url: `/users/${encodeURIComponent(userId)}/access-profiles` }),
			providesTags: (_result, _error, userId) => [{ type: "AccessProfiles", id: userId }],
		}),
	}),
});

export const {
	useGetAccessProfilesQuery,
	useGetAccessProfileQuery,
	useCreateAccessProfileMutation,
	useUpdateAccessProfileMutation,
	useActivateAccessProfileMutation,
	useCloneAccessProfileMutation,
	useDeleteAccessProfileMutation,
	useGetUserAccessProfilesQuery,
} = accessProfilesApi;
