import { User } from "@enterprise/lib/types/user";
import { baseApi } from "@/lib/store/apis/baseApi";

export interface GetVirtualKeyUsersResponse {
	users: User[];
}

export interface GetUserVirtualKeysResponse {
	virtual_keys: Array<{ id: string; name: string; is_active?: boolean; created_at?: string }>;
}

export const virtualKeyUsersApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getVirtualKeyUsers: builder.query<GetVirtualKeyUsersResponse, string>({
			query: (vkId) => ({ url: `/governance/virtual-keys/${vkId}/users` }),
			providesTags: (_result, _error, vkId) => [{ type: "VirtualKeys", id: vkId }],
		}),
		getUserVirtualKeys: builder.query<GetUserVirtualKeysResponse, string>({
			query: (userId) => ({ url: `/governance/users/${userId}/virtual-keys` }),
			providesTags: (_result, _error, userId) => [{ type: "VirtualKeys", id: `user-${userId}` }],
		}),
		setVirtualKeyUser: builder.mutation<GetVirtualKeyUsersResponse, { vkId: string; user_id: string }>({
			query: ({ vkId, user_id }) => ({
				url: `/governance/virtual-keys/${vkId}/users`,
				method: "PUT",
				body: { user_id },
			}),
			invalidatesTags: (_result, _error, { vkId, user_id }) => [
				{ type: "VirtualKeys", id: vkId },
				{ type: "VirtualKeys", id: `user-${user_id}` },
				{ type: "VirtualKeys", id: "LIST" },
			],
		}),
		deleteVirtualKeyUser: builder.mutation<{ users: User[] }, { vkId: string; user_id?: string } | string>({
			query: (arg) => {
				const vkId = typeof arg === "string" ? arg : arg.vkId;
				const userId = typeof arg === "string" ? undefined : arg.user_id;
				return {
					url: `/governance/virtual-keys/${vkId}/users`,
					method: "DELETE",
					params: userId ? { user_id: userId } : undefined,
					body: userId ? { user_id: userId } : undefined,
				};
			},
			invalidatesTags: (_result, _error, arg) => {
				const vkId = typeof arg === "string" ? arg : arg.vkId;
				const userId = typeof arg === "string" ? undefined : arg.user_id;
				const tags: Array<{ type: "VirtualKeys"; id: string }> = [
					{ type: "VirtualKeys", id: vkId },
					{ type: "VirtualKeys", id: "LIST" },
				];
				if (userId) {
					tags.push({ type: "VirtualKeys", id: `user-${userId}` });
				}
				return tags;
			},
		}),
	}),
});

export const {
	useGetVirtualKeyUsersQuery,
	useGetUserVirtualKeysQuery,
	useSetVirtualKeyUserMutation,
	useDeleteVirtualKeyUserMutation,
} = virtualKeyUsersApi;
