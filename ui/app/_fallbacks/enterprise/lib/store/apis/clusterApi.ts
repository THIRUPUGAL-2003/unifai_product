import { ClusterConfig } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const clusterApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getClusterConfig: builder.query<ClusterConfig, void>({
			query: () => ({ url: "/cluster" }),
			providesTags: ["ClusterNodes"],
		}),
		updateClusterConfig: builder.mutation<ClusterConfig, Partial<ClusterConfig>>({
			query: (body) => ({ url: "/cluster", method: "PUT", body }),
			invalidatesTags: ["ClusterNodes"],
		}),
	}),
});

export const { useGetClusterConfigQuery, useUpdateClusterConfigMutation } = clusterApi;
