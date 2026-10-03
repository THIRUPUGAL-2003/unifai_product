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
	}),
});

export const { useGetAuditLogsQuery, useLazyExportAuditLogsQuery, useGetAuditSettingsQuery, useUpdateAuditSettingsMutation } = auditLogsApi;
