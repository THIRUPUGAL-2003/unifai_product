import { RBACPermission, RBACRole } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const rbacApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getRoles: builder.query<{ roles: RBACRole[] }, void>({
			query: () => ({ url: "/roles" }),
			providesTags: ["Roles"],
		}),
		getPermissions: builder.query<{ permissions: RBACPermission[] }, void>({
			query: () => ({ url: "/permissions" }),
			providesTags: ["Permissions"],
		}),
		getRolePermissions: builder.query<{ permissions: RBACPermission[] }, number>({
			query: (id) => ({ url: `/roles/${id}/permissions` }),
			providesTags: (_result, _error, id) => [{ type: "Permissions", id }],
		}),
		createRole: builder.mutation<{ role: RBACRole }, Partial<RBACRole>>({
			query: (body) => ({ url: "/roles", method: "POST", body }),
			invalidatesTags: ["Roles"],
		}),
		updateRolePermissions: builder.mutation<void, { id: number; permission_ids: number[] }>({
			query: ({ id, permission_ids }) => ({
				url: `/roles/${id}/permissions`,
				method: "PUT",
				body: { permission_ids },
			}),
			invalidatesTags: ["Roles", "Permissions"],
		}),
		deleteRole: builder.mutation<void, number>({
			query: (id) => ({ url: `/roles/${id}`, method: "DELETE" }),
			invalidatesTags: ["Roles"],
		}),
		getMyRBACPermissions: builder.query<
			{ role: string; permissions: Record<string, Record<string, boolean>> },
			void
		>({
			query: () => ({ url: "/rbac/me/permissions" }),
			providesTags: ["Permissions"],
		}),
		assignUserRole: builder.mutation<void, { id: string; role_id?: number; role_name?: string }>({
			query: ({ id, role_id, role_name }) => ({
				url: `/users/${id}/role`,
				method: "PUT",
				body: { role_id, role_name },
			}),
			invalidatesTags: ["Users", "Roles", "Permissions"],
		}),
	}),
});

export const {
	useGetRolesQuery,
	useGetPermissionsQuery,
	useGetRolePermissionsQuery,
	useCreateRoleMutation,
	useUpdateRolePermissionsMutation,
	useDeleteRoleMutation,
	useGetMyRBACPermissionsQuery,
	useAssignUserRoleMutation,
} = rbacApi;
