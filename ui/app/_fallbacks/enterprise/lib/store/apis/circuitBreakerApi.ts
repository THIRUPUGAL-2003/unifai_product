import { CircuitBreakerPolicy, CircuitState } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const circuitBreakerApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getCircuitBreakerPolicies: builder.query<{ policies: CircuitBreakerPolicy[]; count: number }, void>({
			query: () => ({ url: "/circuit-breaker/policies" }),
			providesTags: ["CircuitBreakerPolicies"],
		}),
		getCircuitBreakerState: builder.query<{ circuits: Record<string, CircuitState> }, void>({
			query: () => ({ url: "/circuit-breaker/state" }),
			providesTags: ["CircuitBreakerState"],
		}),
		createCircuitBreakerPolicy: builder.mutation<CircuitBreakerPolicy, CircuitBreakerPolicy>({
			query: (body) => ({ url: "/circuit-breaker/policies", method: "POST", body }),
			invalidatesTags: ["CircuitBreakerPolicies"],
		}),
		updateCircuitBreakerPolicy: builder.mutation<CircuitBreakerPolicy, CircuitBreakerPolicy>({
			query: (body) => ({ url: `/circuit-breaker/policies/${encodeURIComponent(body.name)}`, method: "PUT", body }),
			invalidatesTags: ["CircuitBreakerPolicies"],
		}),
		deleteCircuitBreakerPolicy: builder.mutation<void, string>({
			query: (name) => ({ url: `/circuit-breaker/policies/${encodeURIComponent(name)}`, method: "DELETE" }),
			invalidatesTags: ["CircuitBreakerPolicies", "CircuitBreakerState"],
		}),
		resetCircuitBreakerPolicy: builder.mutation<void, string>({
			query: (name) => ({ url: `/circuit-breaker/policies/${encodeURIComponent(name)}/reset`, method: "POST" }),
			invalidatesTags: ["CircuitBreakerState"],
		}),
	}),
});

export const {
	useGetCircuitBreakerPoliciesQuery,
	useGetCircuitBreakerStateQuery,
	useCreateCircuitBreakerPolicyMutation,
	useUpdateCircuitBreakerPolicyMutation,
	useDeleteCircuitBreakerPolicyMutation,
	useResetCircuitBreakerPolicyMutation,
} = circuitBreakerApi;
