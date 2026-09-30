import { AlertChannel } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const alertChannelsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getAlertChannels: builder.query<{ channels: AlertChannel[]; count: number }, void>({
			query: () => ({ url: "/alert-channels" }),
			providesTags: ["AlertChannels"],
		}),
		createAlertChannel: builder.mutation<AlertChannel, Omit<AlertChannel, "id">>({
			query: (body) => ({ url: "/alert-channels", method: "POST", body }),
			invalidatesTags: ["AlertChannels"],
		}),
		updateAlertChannel: builder.mutation<AlertChannel, AlertChannel>({
			query: ({ id, ...body }) => ({ url: `/alert-channels/${id}`, method: "PUT", body: { id, ...body } }),
			invalidatesTags: ["AlertChannels"],
		}),
		testAlertChannel: builder.mutation<{ ok: boolean; message: string }, number>({
			query: (id) => ({ url: `/alert-channels/${id}/test`, method: "POST" }),
		}),
		deleteAlertChannel: builder.mutation<void, number>({
			query: (id) => ({ url: `/alert-channels/${id}`, method: "DELETE" }),
			invalidatesTags: ["AlertChannels"],
		}),
	}),
});

export const {
	useGetAlertChannelsQuery,
	useCreateAlertChannelMutation,
	useUpdateAlertChannelMutation,
	useTestAlertChannelMutation,
	useDeleteAlertChannelMutation,
} = alertChannelsApi;
