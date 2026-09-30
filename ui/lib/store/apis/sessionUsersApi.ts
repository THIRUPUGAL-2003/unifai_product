import { baseApi } from "./baseApi";

export interface SessionUser {
	id: string;
	username: string;
	email?: string;
	role: string;
	status?: string;
	budget: number;
	rate_limit: number;
	budget_id?: string | null;
	rate_limit_id?: string | null;
	budget_current_usage?: number;
	allowed_prompt_repos?: string;
	allowed_sections?: string;
	created_at: string;
	email_sent?: boolean;
	email_error?: string;
}

export interface SessionUserPayload {
	username: string;
	email?: string;
	password?: string;
	role: string;
	budget?: number;
	rate_limit?: number;
	allowed_prompt_repos?: string;
	allowed_sections?: string;
}

export const sessionUsersApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		getSessionUsers: builder.query<SessionUser[], void>({
			query: () => ({ url: "/session/users" }),
			providesTags: ["Users"],
		}),
		createSessionUser: builder.mutation<SessionUser, SessionUserPayload>({
			query: (body) => ({ url: "/session/users", method: "POST", body }),
			invalidatesTags: ["Users"],
		}),
		updateSessionUser: builder.mutation<SessionUser, { id: string; updates: SessionUserPayload }>({
			query: ({ id, updates }) => ({ url: `/session/users/${id}`, method: "PUT", body: updates }),
			invalidatesTags: ["Users"],
		}),
		deleteSessionUser: builder.mutation<void, string>({
			query: (id) => ({ url: `/session/users/${id}`, method: "DELETE" }),
			invalidatesTags: ["Users"],
		}),
		approveSessionUser: builder.mutation<SessionUser, string>({
			query: (id) => ({ url: `/session/users/${id}/approve`, method: "POST" }),
			invalidatesTags: ["Users"],
		}),
		rejectSessionUser: builder.mutation<Pick<SessionUser, "id" | "status" | "email_sent" | "email_error"> & { message?: string }, string>({
			query: (id) => ({ url: `/session/users/${id}/reject`, method: "POST" }),
			invalidatesTags: ["Users"],
		}),
	}),
});

export const {
	useGetSessionUsersQuery,
	useCreateSessionUserMutation,
	useUpdateSessionUserMutation,
	useDeleteSessionUserMutation,
	useApproveSessionUserMutation,
	useRejectSessionUserMutation,
} = sessionUsersApi;
