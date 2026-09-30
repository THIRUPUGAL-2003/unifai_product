import { LoadBalancerConfig, LoadBalancerRoutesResponse } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const loadBalancerApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getLoadBalancerConfig: builder.query<LoadBalancerConfig, void>({
			query: () => ({ url: "/load-balancer" }),
			providesTags: ["LoadBalancerConfig"],
		}),
		getLoadBalancerRoutes: builder.query<LoadBalancerRoutesResponse, void>({
			query: () => ({ url: "/load-balancer/routes" }),
			providesTags: ["LoadBalancerConfig"],
		}),
		updateLoadBalancerConfig: builder.mutation<LoadBalancerConfig, LoadBalancerConfig>({
			query: (body) => ({ url: "/load-balancer", method: "PUT", body }),
			invalidatesTags: ["LoadBalancerConfig"],
		}),
	}),
});

export const { useGetLoadBalancerConfigQuery, useGetLoadBalancerRoutesQuery, useUpdateLoadBalancerConfigMutation } = loadBalancerApi;
