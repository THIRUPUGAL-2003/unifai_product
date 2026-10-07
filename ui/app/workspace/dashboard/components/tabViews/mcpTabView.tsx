import { QueryErrorBanner } from "@/components/queryErrorBanner";
import {
	useGetMCPCostHistogramQuery,
	useGetMCPHistogramQuery,
	useGetMCPTopToolsQuery,
	useLazyGetMCPCostHistogramQuery,
	useLazyGetMCPHistogramQuery,
	useLazyGetMCPTopToolsQuery,
} from "@/lib/store";
import type { MCPToolLogFilters } from "@/lib/types/logs";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useState } from "react";
import type { DashboardData } from "../../utils/exportUtils";
import type { ChartType } from "../charts/chartTypeToggle";
import { MCPTab } from "../mcpTab";

export interface MCPTabViewHandle {
	getData: () => Partial<DashboardData>;
	loadData: () => Promise<Partial<DashboardData>>;
}

interface MCPTabViewProps {
	filters: MCPToolLogFilters;
	active: boolean;
	startTime: number;
	endTime: number;
	mcpVolumeChartType: ChartType;
	mcpCostChartType: ChartType;
	pollingInterval?: number;
	onMcpVolumeChartToggle: (type: ChartType) => void;
	onMcpCostChartToggle: (type: ChartType) => void;
}

export const MCPTabView = forwardRef<MCPTabViewHandle, MCPTabViewProps>(function MCPTabView(
	{ filters, active, startTime, endTime, mcpVolumeChartType, mcpCostChartType, pollingInterval = 0, onMcpVolumeChartToggle, onMcpCostChartToggle },
	ref,
) {
	const fetchArg = useMemo(() => ({ filters }), [filters]);
	const [pollMs, setPollMs] = useState(pollingInterval ?? 0);
	const skipOpts = useMemo(
		() => ({ skip: !active, pollingInterval: pollMs, skipPollingIfUnfocused: false, refetchOnFocus: true, refetchOnReconnect: true }),
		[active, pollMs],
	);

	const { data: mcpHistogramData, isLoading: loadingMcpHistogram, isError: errHist } = useGetMCPHistogramQuery(fetchArg, skipOpts);
	const { data: mcpCostData, isLoading: loadingMcpCost, isError: errCost } = useGetMCPCostHistogramQuery(fetchArg, skipOpts);
	const { data: mcpTopToolsData, isLoading: loadingMcpTopTools, isError: errTop } = useGetMCPTopToolsQuery(fetchArg, skipOpts);
	const queryFailed = errHist || errCost || errTop;

	useEffect(() => {
		setPollMs(queryFailed ? 0 : (pollingInterval ?? 0));
	}, [queryFailed, pollingInterval]);

	const [triggerMcpHistogram] = useLazyGetMCPHistogramQuery();
	const [triggerMcpCost] = useLazyGetMCPCostHistogramQuery();
	const [triggerMcpTopTools] = useLazyGetMCPTopToolsQuery();

	const loadData = useCallback(async () => {
		const [histogram, cost, topTools] = await Promise.all([
			triggerMcpHistogram(fetchArg, true),
			triggerMcpCost(fetchArg, true),
			triggerMcpTopTools(fetchArg, true),
		]);
		return {
			mcpHistogramData: histogram.data ?? null,
			mcpCostData: cost.data ?? null,
			mcpTopToolsData: topTools.data ?? null,
		};
	}, [fetchArg, triggerMcpHistogram, triggerMcpCost, triggerMcpTopTools]);

	useImperativeHandle(
		ref,
		() => ({
			getData: () => ({
				mcpHistogramData: mcpHistogramData ?? null,
				mcpCostData: mcpCostData ?? null,
				mcpTopToolsData: mcpTopToolsData ?? null,
			}),
			loadData,
		}),
		[mcpHistogramData, mcpCostData, mcpTopToolsData, loadData],
	);

	return (
		<div className="flex h-full flex-col gap-3">
			{queryFailed ? <QueryErrorBanner testId="dashboard-mcp-query-error" /> : null}
			<MCPTab
				mcpHistogramData={mcpHistogramData ?? null}
				mcpCostData={mcpCostData ?? null}
				mcpTopToolsData={mcpTopToolsData ?? null}
				loadingMcpHistogram={loadingMcpHistogram}
				loadingMcpCost={loadingMcpCost}
				loadingMcpTopTools={loadingMcpTopTools}
				startTime={startTime}
				endTime={endTime}
				mcpVolumeChartType={mcpVolumeChartType}
				mcpCostChartType={mcpCostChartType}
				onMcpVolumeChartToggle={onMcpVolumeChartToggle}
				onMcpCostChartToggle={onMcpCostChartToggle}
			/>
		</div>
	);
});