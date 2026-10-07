import { QueryErrorBanner } from "@/components/queryErrorBanner";
import {
	useGetLogsModelHistogramQuery,
	useGetModelRankingsQuery,
	useLazyGetLogsModelHistogramQuery,
	useLazyGetModelRankingsQuery,
} from "@/lib/store";
import type { LogFilters } from "@/lib/types/logs";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useState } from "react";
import type { DashboardData } from "../../utils/exportUtils";
import { ModelRankingsTab } from "../modelRankingsTab";

export interface ModelRankingsTabViewHandle {
	getData: () => Partial<DashboardData>;
	loadData: () => Promise<Partial<DashboardData>>;
}

interface ModelRankingsTabViewProps {
	filters: LogFilters;
	active: boolean;
	startTime: number;
	endTime: number;
	pollingInterval?: number;
}

export const ModelRankingsTabView = forwardRef<ModelRankingsTabViewHandle, ModelRankingsTabViewProps>(function ModelRankingsTabView(
	{ filters, active, startTime, endTime, pollingInterval = 0 },
	ref,
) {
	const fetchArg = useMemo(() => ({ filters }), [filters]);
	const [pollMs, setPollMs] = useState(pollingInterval ?? 0);
	const skipOpts = useMemo(
		() => ({ skip: !active, pollingInterval: pollMs, skipPollingIfUnfocused: false, refetchOnFocus: true, refetchOnReconnect: true }),
		[active, pollMs],
	);

	const { data: rankingsData, isLoading: loadingRankings, isError: errRankings } = useGetModelRankingsQuery(fetchArg, skipOpts);
	const { data: modelData, isLoading: loadingModels, isError: errModels } = useGetLogsModelHistogramQuery(fetchArg, skipOpts);
	const queryFailed = errRankings || errModels;

	useEffect(() => {
		setPollMs(queryFailed ? 0 : (pollingInterval ?? 0));
	}, [queryFailed, pollingInterval]);

	const [triggerRankings] = useLazyGetModelRankingsQuery();
	const [triggerModels] = useLazyGetLogsModelHistogramQuery();

	const loadData = useCallback(async () => {
		const [rankings, models] = await Promise.all([triggerRankings(fetchArg, true), triggerModels(fetchArg, true)]);
		return { rankingsData: rankings.data ?? null, modelData: models.data ?? null };
	}, [fetchArg, triggerRankings, triggerModels]);

	useImperativeHandle(
		ref,
		() => ({
			getData: () => ({
				rankingsData: rankingsData ?? null,
				modelData: modelData ?? null,
			}),
			loadData,
		}),
		[rankingsData, modelData, loadData],
	);

	return (
		<div className="flex h-full flex-col gap-3">
			{queryFailed ? <QueryErrorBanner testId="dashboard-model-rankings-query-error" /> : null}
			<ModelRankingsTab
				rankingsData={rankingsData ?? null}
				loading={loadingRankings}
				modelData={modelData ?? null}
				loadingModels={loadingModels}
				startTime={startTime}
				endTime={endTime}
			/>
		</div>
	);
});