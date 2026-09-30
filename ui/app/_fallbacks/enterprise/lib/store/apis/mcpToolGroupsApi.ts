import { MCPToolGroup } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const mcpToolGroupsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getMCPToolGroups: builder.query<{ tool_groups: MCPToolGroup[]; count: number }, void>({
			query: () => ({ url: "/mcp/tool-groups" }),
			providesTags: ["MCPToolGroups"],
		}),
		createMCPToolGroup: builder.mutation<MCPToolGroup, Partial<MCPToolGroup>>({
			query: (body) => ({ url: "/mcp/tool-groups", method: "POST", body }),
			invalidatesTags: ["MCPToolGroups"],
		}),
		updateMCPToolGroup: builder.mutation<MCPToolGroup, MCPToolGroup>({
			query: ({ id, ...body }) => ({ url: `/mcp/tool-groups/${id}`, method: "PUT", body }),
			invalidatesTags: ["MCPToolGroups"],
		}),
		deleteMCPToolGroup: builder.mutation<void, number>({
			query: (id) => ({ url: `/mcp/tool-groups/${id}`, method: "DELETE" }),
			invalidatesTags: ["MCPToolGroups"],
		}),
	}),
});

export const { useGetMCPToolGroupsQuery, useCreateMCPToolGroupMutation, useUpdateMCPToolGroupMutation, useDeleteMCPToolGroupMutation } =
	mcpToolGroupsApi;
