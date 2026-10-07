import { QueryErrorBanner } from "@/components/queryErrorBanner";
import {
	useGetLogsProviderCostHistogramQuery,
	useGetLogsProviderLatencyHistogramQuery,
	useGetLogsProviderTokenHistogramQuery,
	useLazyGetLogsProviderCostHistogramQuery,
	useLazyGetLogsProviderLatencyHistogramQuery,
	useLazyGetLogsProviderTokenHistogramQuery,
} from "@/lib/store";
import type { LogFilters } from "@/lib/types/logs";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useState } from "react";
import type { DashboardData } from "../../utils/exportUtils";
import type { ChartType } from "../charts/chartTypeToggle";
import { ProviderUsageTab } from "../providerUsageTab";

export interface ProviderUsageTabViewHandle {
	getData: () => Partial<DashboardData>;
	loadData: () => Promise<Partial<DashboardData>>;
}

const sanitizeSeriesLabels = (values?: string[]): string[] => {
	if (!values) return [];
	const trimmed = values.map((v) => v.trim()).filter((v) => v.length > 0);
	return [...new Set(trimmed)];
};

interface ProviderUsageTabViewProps {
	filters: LogFilters;
	active: boolean;
	startTime: number;
	endTime: number;
	providerCostChartType: ChartType;
	providerTokenChartType: ChartType;
	providerLatencyChartType: ChartType;
	providerCostProvider: string;
	providerTokenProvider: string;
	providerLatencyProvider: string;
	pollingInterval?: number;
	onProviderCostChartToggle: (type: ChartType) => void;
	onProviderTokenChartToggle: (type: ChartType) => void;
	onProviderLatencyChartToggle: (type: ChartType) => void;
	onProviderCostProviderChange: (provider: string) => void;
	onProviderTokenProviderChange: (provider: string) => void;
	onProviderLatencyProviderChange: (provider: string) => void;
}

export const ProviderUsageTabView = forwardRef<ProviderUsageTabViewHandle, ProviderUsageTabViewProps>(function ProviderUsageTabView(
	{
		filters,
		active,
		startTime,
		endTime,
		providerCostChartType,
		providerTokenChartType,
		providerLatencyChartType,
		providerCostProvider,
		providerTokenProvider,
		providerLatencyProvider,
		pollingInterval = 0,
		onProviderCostChartToggle,
		onProviderTokenChartToggle,
		onProviderLatencyChartToggle,
		onProviderCostProviderChange,
		onProviderTokenProviderChange,
		onProviderLatencyProviderChange,
	},
	ref,
) {
	const fetchArg = useMemo(() => ({ filters }), [filters]);
	const [pollMs, setPollMs] = useState(pollingInterval ?? 0);
	const skipOpts = useMemo(
		() => ({ skip: !active, pollingInterval: pollMs, skipPollingIfUnfocused: false, refetchOnFocus: true, refetchOnReconnect: true }),
		[active, pollMs],
	);

	const { data: providerCostData, isLoading: loadingProviderCost, isError: errCost } = useGetLogsProviderCostHistogramQuery(fetchArg, skipOpts);
	const { data: providerTokenData, isLoading: loadingProviderTokens, isError: errTokens } = useGetLogsProviderTokenHistogramQuery(fetchArg, skipOpts);
	const {
		data: providerLatencyData,
		isLoading: loadingProviderLatency,
		isError: errLatency,
	} = useGetLogsProviderLatencyHistogramQuery(fetchArg, skipOpts);
	const queryFailed = errCost || errTokens || errLatency;

	useEffect(() => {
		setPollMs(queryFailed ? 0 : (pollingInterval ?? 0));
	}, [queryFailed, pollingInterval]);

	const [triggerProviderCost] = useLazyGetLogsProviderCostHistogramQuery();
	const [triggerProviderTokens] = useLazyGetLogsProviderTokenHistogramQuery();
	const [triggerProviderLatency] = useLazyGetLogsProviderLatencyHistogramQuery();

	const loadData = useCallback(async () => {
		const [cost, tokens, latency] = await Promise.all([
			triggerProviderCost(fetchArg, true),
			triggerProviderTokens(fetchArg, true),
			triggerProviderLatency(fetchArg, true),
		]);
		return {
			providerCostData: cost.data ?? null,
			providerTokenData: tokens.data ?? null,
			providerLatencyData: latency.data ?? null,
		};
	}, [fetchArg, triggerProviderCost, triggerProviderTokens, triggerProviderLatency]);

	useImperativeHandle(
		ref,
		() => ({
			getData: () => ({
				providerCostData: providerCostData ?? null,
				providerTokenData: providerTokenData ?? null,
				providerLatencyData: providerLatencyData ?? null,
			}),
			loadData,
		}),
		[providerCostData, providerTokenData, providerLatencyData, loadData],
	);

	const availableProviders = useMemo(
		() =>
			sanitizeSeriesLabels([
				...(providerCostData?.providers ?? []),
				...(providerTokenData?.providers ?? []),
				...(providerLatencyData?.providers ?? []),
			]),
		[providerCostData?.providers, providerTokenData?.providers, providerLatencyData?.providers],
	);
	const providerCostProviders = useMemo(() => sanitizeSeriesLabels(providerCostData?.providers), [providerCostData?.providers]);
	const providerTokenProviders = useMemo(() => sanitizeSeriesLabels(providerTokenData?.providers), [providerTokenData?.providers]);
	const providerLatencyProviders = useMemo(() => sanitizeSeriesLabels(providerLatencyData?.providers), [providerLatencyData?.providers]);

	return (
		<div className="flex h-full flex-col gap-3">
			{queryFailed ? <QueryErrorBanner testId="dashboard-provider-usage-query-error" /> : null}
			<ProviderUsageTab
				providerCostData={providerCostData ?? null}
				providerTokenData={providerTokenData ?? null}
				providerLatencyData={providerLatencyData ?? null}
				loadingProviderCost={loadingProviderCost}
				loadingProviderTokens={loadingProviderTokens}
				loadingProviderLatency={loadingProviderLatency}
				startTime={startTime}
				endTime={endTime}
				providerCostChartType={providerCostChartType}
				providerTokenChartType={providerTokenChartType}
				providerLatencyChartType={providerLatencyChartType}
				providerCostProvider={providerCostProvider}
				providerTokenProvider={providerTokenProvider}
				providerLatencyProvider={providerLatencyProvider}
				availableProviders={availableProviders}
				providerCostProviders={providerCostProviders}
				providerTokenProviders={providerTokenProviders}
				providerLatencyProviders={providerLatencyProviders}
				onProviderCostChartToggle={onProviderCostChartToggle}
				onProviderTokenChartToggle={onProviderTokenChartToggle}
				onProviderLatencyChartToggle={onProviderLatencyChartToggle}
				onProviderCostProviderChange={onProviderCostProviderChange}
				onProviderTokenProviderChange={onProviderTokenProviderChange}
				onProviderLatencyProviderChange={onProviderLatencyProviderChange}
			/>
		</div>
	);
});