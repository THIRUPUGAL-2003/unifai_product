import { AuditLog, AuditLogFilters, AuditSettings } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

const filterParams = (params?: AuditLogFilters | void) => ({
	...(params?.search && { search: params.search }),
	...(params?.action && { action: params.action }),
	...(params?.outcome && { outcome: params.outcome }),
});

export const auditLogsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getAuditLogs: builder.query<
			{ logs: AuditLog[]; count: number; total_count: number },
			(AuditLogFilters & { limit?: number; offset?: number }) | void
		>({
			query: (params) => ({
				url: "/audit-logs",
				params: {
					...filterParams(params),
					...(params?.limit && { limit: params.limit }),
					...(params?.offset && { offset: params.offset }),
				},
			}),
			providesTags: ["AuditLogs"],
		}),
		exportAuditLogs: builder.query<{ logs: AuditLog[]; count: number }, AuditLogFilters | void>({
			query: (params) => ({ url: "/audit-logs/export", params: filterParams(params) }),
		}),
		getAuditSettings: builder.query<AuditSettings, void>({
			query: () => ({ url: "/audit-logs/settings" }),
			providesTags: ["AuditLogs"],
		}),
		updateAuditSettings: builder.mutation<AuditSettings, AuditSettings>({
			query: (body) => ({ url: "/audit-logs/settings", method: "PUT", body }),
			invalidatesTags: ["AuditLogs"],
		}),
		clearAuditLogs: builder.mutation<{ status: string; message: string }, { period?: string; date?: string } | void>({
			query: (params) => ({
				url: "/audit-logs",
				method: "DELETE",
				params: {
					...(params?.period && { period: params.period }),
					...(params?.date && { date: params.date }),
				},
			}),
			invalidatesTags: ["AuditLogs"],
		}),
		bulkDeleteAuditLogs: builder.mutation<{ status: string; deleted: number }, { ids: (number | string)[] }>({
			query: (body) => ({
				url: "/audit-logs/bulk-delete",
				method: "POST",
				body,
			}),
			invalidatesTags: ["AuditLogs"],
		}),
		deleteAuditLog: builder.mutation<{ status: string; deleted: number }, number | string>({
			query: (id) => ({
				url: `/audit-logs/${id}`,
				method: "DELETE",
			}),
			invalidatesTags: ["AuditLogs"],
		}),
	}),
});

export const {
	useGetAuditLogsQuery,
	useLazyExportAuditLogsQuery,
	useGetAuditSettingsQuery,
	useUpdateAuditSettingsMutation,
	useClearAuditLogsMutation,
	useBulkDeleteAuditLogsMutation,
	useDeleteAuditLogMutation,
} = auditLogsApi;
