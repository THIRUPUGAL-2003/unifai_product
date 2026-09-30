import { AuditLog, AuditSettings } from "@enterprise/lib/types/workspace";
import { baseApi } from "@/lib/store/apis/baseApi";

export const auditLogsApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getAuditLogs: builder.query<{ logs: AuditLog[]; count: number; total_count: number }, { search?: string; action?: string; outcome?: string } | void>({
			query: (params) => ({
				url: "/audit-logs",
				params: {
					...(params?.search && { search: params.search }),
					...(params?.action && { action: params.action }),
					...(params?.outcome && { outcome: params.outcome }),
				},
			}),
			providesTags: ["AuditLogs"],
		}),
		exportAuditLogs: builder.query<{ logs: AuditLog[]; count: number }, void>({
			query: () => ({ url: "/audit-logs/export" }),
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
