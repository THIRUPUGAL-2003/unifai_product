import { User } from "@enterprise/lib/types/user";
import { baseApi } from "@/lib/store/apis/baseApi";

/** A user who can use the key: assigned directly, or a member of an assigned team/customer. */
export type VirtualKeyUser = User & {
	origin?: "direct" | "team" | "customer";
	origin_name?: string;
};

export interface GetVirtualKeyUsersResponse {
	users: VirtualKeyUser[];
}

export interface GetUserVirtualKeysResponse {
	virtual_keys: Array<{
		id: string;
		name: string;
		is_active?: boolean;
		created_at?: string;
		origin?: "direct" | "team" | "customer";
	}>;
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
			invalidatesTags: ["VirtualKeys"],
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
			invalidatesTags: ["VirtualKeys"],
		}),
	}),
});

export const {
	useGetVirtualKeyUsersQuery,
	useGetUserVirtualKeysQuery,
	useSetVirtualKeyUserMutation,
	useDeleteVirtualKeyUserMutation,
} = virtualKeyUsersApi;
