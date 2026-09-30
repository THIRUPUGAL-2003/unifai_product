import { PromptDeployment } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const promptDeploymentsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getPromptDeployments: builder.query<{ deployments: PromptDeployment[]; count: number }, { prompt_id?: string } | void>({
			query: (params) => ({
				url: "/prompt-deployments",
				params: params?.prompt_id ? { prompt_id: params.prompt_id } : undefined,
			}),
			providesTags: ["PromptDeployments"],
		}),
		createPromptDeployment: builder.mutation<PromptDeployment, Partial<PromptDeployment>>({
			query: (body) => ({ url: "/prompt-deployments", method: "POST", body }),
			invalidatesTags: ["PromptDeployments"],
		}),
		updatePromptDeployment: builder.mutation<PromptDeployment, PromptDeployment>({
			query: ({ id, ...body }) => ({ url: `/prompt-deployments/${id}`, method: "PUT", body }),
			invalidatesTags: ["PromptDeployments"],
		}),
		deletePromptDeployment: builder.mutation<void, number>({
			query: (id) => ({ url: `/prompt-deployments/${id}`, method: "DELETE" }),
			invalidatesTags: ["PromptDeployments"],
		}),
	}),
});

export const {
	useGetPromptDeploymentsQuery,
	useCreatePromptDeploymentMutation,
	useUpdatePromptDeploymentMutation,
	useDeletePromptDeploymentMutation,
} = promptDeploymentsApi;
