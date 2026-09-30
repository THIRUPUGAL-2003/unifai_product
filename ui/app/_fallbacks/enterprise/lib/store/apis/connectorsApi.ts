import { ConnectorConfig } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const connectorsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getConnectors: builder.query<ConnectorConfig[], void>({
			query: () => ({ url: `/connectors` }),
			transformResponse: (response: { connectors?: ConnectorConfig[] }) => response.connectors || [],
			providesTags: (result) => [
				{ type: "Connectors", id: "LIST" },
				...(result?.map((connector) => ({ type: "Connectors" as const, id: connector.name })) ?? []),
			],
		}),
		getConnector: builder.query<ConnectorConfig, string>({
			query: (name) => ({ url: `/connectors/${name}` }),
			providesTags: (_result, _error, name) => [{ type: "Connectors", id: name }],
		}),
		updateConnector: builder.mutation<ConnectorConfig, ConnectorConfig>({
			query: ({ name, ...body }) => ({ url: `/connectors/${name}`, method: "PUT", body: { name, ...body } }),
			invalidatesTags: (_result, _error, arg) => [
				{ type: "Connectors", id: "LIST" },
				{ type: "Connectors", id: arg.name },
			],
		}),
		deleteConnector: builder.mutation<{ name: string; deleted: boolean }, string>({
			query: (name) => ({ url: `/connectors/${name}`, method: "DELETE" }),
			invalidatesTags: (_result, _error, name) => [
				{ type: "Connectors", id: "LIST" },
				{ type: "Connectors", id: name },
			],
		}),
	}),
});

export const {
	useGetConnectorsQuery,
	useGetConnectorQuery,
	useUpdateConnectorMutation,
	useDeleteConnectorMutation,
} = connectorsApi;
