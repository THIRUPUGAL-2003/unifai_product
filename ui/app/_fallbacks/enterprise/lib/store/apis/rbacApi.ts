import { RBACPermission, RBACRole, RBACScopeGrants, RBACScopeType } from "@enterprise/lib/types/workspace";
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
			invalidatesTags: (_result, _error, { id }) => [
				{ type: "Permissions", id },
				{ type: "Permissions" },
				"Permissions",
				"Roles",
			],
		}),
		deleteRole: builder.mutation<void, number>({
			query: (id) => ({ url: `/roles/${id}`, method: "DELETE" }),
			// Users on a deleted role are moved to "user" server-side.
			invalidatesTags: (_result, _error, id) => [
				{ type: "Permissions", id },
				{ type: "Permissions" },
				"Roles",
				"Users",
				"Permissions",
			],
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
			invalidatesTags: ["Users", "Roles", "Permissions", { type: "Permissions" }],
		}),
		getRBACScopeGrants: builder.query<RBACScopeGrants, void>({
			query: () => ({ url: "/rbac/scope-grants" }),
			providesTags: ["RBACScopeGrants"],
		}),
		updateRBACScopeGrant: builder.mutation<
			RBACScopeGrants,
			{ scope_type: RBACScopeType; scope_id?: string; permission_ids: number[]; allowed_sections: string }
		>({
			query: (body) => ({ url: "/rbac/scope-grants", method: "PUT", body }),
			invalidatesTags: ["RBACScopeGrants", "Users", "Permissions", { type: "Permissions" }],
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
	useGetRBACScopeGrantsQuery,
	useUpdateRBACScopeGrantMutation,
} = rbacApi;
