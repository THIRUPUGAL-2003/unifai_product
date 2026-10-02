import type { FetchBaseQueryError, FetchBaseQueryMeta } from "@reduxjs/toolkit/query";
import { invalidateSessionAuthCache } from "@/lib/utils/workspaceAccess";
import { baseApi, clearAuthStorage } from "./baseApi";

export interface LoginRequest {
	username: string;
	password: string;
}

export interface LoginResponse {
	message: string;
	role?: string;
	allowed_sections?: string;
}

export interface IsAuthEnabledResponse {
	is_auth_enabled: boolean;
	has_valid_token: boolean;
	auth_type?: "sso" | "password" | "none";
	role?: string;
	username?: string;
	email?: string;
	user_id?: string;
	allowed_sections?: string;
	budget?: number;
	budget_current_usage?: number;
}

export interface LogoutResponse {
	message: string;
}

export const sessionApi = baseApi.injectEndpoints({
	overrideExisting: false,
	endpoints: (builder) => ({
		// Check if auth is enabled
		isAuthEnabled: builder.query<IsAuthEnabledResponse, void>({
			query: () => ({
				url: "/session/is-auth-enabled",
				method: "GET",
			}),
			providesTags: ["Sessions"],
		}),
		// Login endpoint
		login: builder.mutation<LoginResponse, LoginRequest>({
			query: (credentials) => ({
				url: "/session/login",
				method: "POST",
				body: credentials,
			}),
			transformErrorResponse: (response, meta) => {
				const error = response as FetchBaseQueryError;
				const retryAfterSeconds = Number((meta as FetchBaseQueryMeta | undefined)?.response?.headers.get("Retry-After"));
				if (error?.status === 429 && Number.isFinite(retryAfterSeconds) && retryAfterSeconds > 0) {
					return { ...error, retryAfterSeconds };
				}
				return error;
			},
			async onQueryStarted(_arg, { queryFulfilled }) {
				try {
					await queryFulfilled;
					invalidateSessionAuthCache();
				} catch {}
			},
			// Cookie is set on success — force config/session refetch so the
			// dashboard does not keep a pre-login 401 and show the false
			// "Config store setup is missing" banner.
			invalidatesTags: ["Sessions", "Config"],
		}),

		forgotPassword: builder.mutation<{ message: string }, { username?: string; email?: string }>({
			query: (body) => ({
				url: "/session/forgot-password",
				method: "POST",
				body,
			}),
		}),

		verifyOTP: builder.mutation<{ valid: boolean; message: string; reset_token?: string }, { username?: string; email?: string; otp: string }>({
			query: (body) => ({
				url: "/session/verify-otp",
				method: "POST",
				body,
			}),
		}),

		resetPassword: builder.mutation<
			{ message: string },
			{ username?: string; email?: string; reset_token: string; new_password: string; otp?: string }
		>({
			query: (body) => ({
				url: "/session/reset-password",
				method: "POST",
				body,
			}),
		}),

		forgotUsername: builder.mutation<{ message: string }, { email: string }>({
			query: (body) => ({
				url: "/session/forgot-username",
				method: "POST",
				body,
			}),
		}),

		// Logout endpoint
		logout: builder.mutation<LogoutResponse, void>({
			async queryFn(_arg, _api, _extraOptions, baseQuery) {
				const passwordLogout = await baseQuery({
					url: "/session/logout",
					method: "POST",
				});

				// Primary session logout: if the server returns 401, 403, or 404,
				// the session is already non-existent or expired on the server,
				// so treat it as successfully cleared.
				if (passwordLogout.error) {
					const status = (passwordLogout.error as FetchBaseQueryError)?.status;
					if (status !== 401 && status !== 403 && status !== 404) {
						return { error: passwordLogout.error };
					}
				}

				// SCIM OAuth tokens belong to the workspace's IdP connection, not to this
				// user's sign-in, so dashboard logout must not revoke them.
				return { data: { message: "Logout successful" } };
			},
			// After logout, clear token and all cached data
			async onQueryStarted(arg, { dispatch, queryFulfilled }) {
				try {
					await queryFulfilled;
				} catch {
				} finally {
					invalidateSessionAuthCache();
					clearAuthStorage();
					dispatch(baseApi.util.resetApiState());
				}
			},
			invalidatesTags: ["Sessions", "Providers", "Logs", "VirtualKeys", "Teams", "Customers", "Budgets", "RateLimits"],
		}),
	}),
});

export const {
	useIsAuthEnabledQuery,
	useLoginMutation,
	useLogoutMutation,
	useForgotPasswordMutation,
	useVerifyOTPMutation,
	useResetPasswordMutation,
	useForgotUsernameMutation,
} = sessionApi;