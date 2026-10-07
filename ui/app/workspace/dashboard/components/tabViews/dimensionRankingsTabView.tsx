import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { useGetDimensionRankingsQuery, useLazyGetDimensionRankingsQuery } from "@/lib/store";
import type { LogFilters, RankingDimension } from "@/lib/types/logs";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useState } from "react";
import type { DashboardData } from "../../utils/exportUtils";
import { DimensionRankingsTab } from "../dimensionRankingsTab";

export interface DimensionRankingsTabViewHandle {
	getData: () => Partial<DashboardData>;
	loadData: () => Promise<Partial<DashboardData>>;
}

interface DimensionRankingsTabViewProps {
	filters: LogFilters;
	active: boolean;
	dimension: RankingDimension;
	dimensionLabel: string;
	testIdPrefix: string;
	dataKey: keyof DashboardData;
	pollingInterval?: number;
}

export const DimensionRankingsTabView = forwardRef<DimensionRankingsTabViewHandle, DimensionRankingsTabViewProps>(
	function DimensionRankingsTabView({ filters, active, dimension, dimensionLabel, testIdPrefix, dataKey, pollingInterval = 0 }, ref) {
		const fetchArg = useMemo(() => ({ filters, dimension }), [filters, dimension]);
		const [pollMs, setPollMs] = useState(pollingInterval ?? 0);
		const skipOpts = useMemo(
			() => ({ skip: !active, pollingInterval: pollMs, skipPollingIfUnfocused: false, refetchOnFocus: true, refetchOnReconnect: true }),
			[active, pollMs],
		);

		const { data, isLoading: loading, isError } = useGetDimensionRankingsQuery(fetchArg, skipOpts);

		useEffect(() => {
			setPollMs(isError ? 0 : (pollingInterval ?? 0));
		}, [isError, pollingInterval]);

		const [triggerDimensionRankings] = useLazyGetDimensionRankingsQuery();

		const loadData = useCallback(async () => {
			const result = await triggerDimensionRankings(fetchArg, true);
			return { [dataKey]: result.data ?? null };
		}, [fetchArg, dataKey, triggerDimensionRankings]);

		useImperativeHandle(
			ref,
			() => ({
				getData: () => ({ [dataKey]: data ?? null }),
				loadData,
			}),
			[data, dataKey, loadData],
		);

		return (
			<div className="flex h-full flex-col gap-3">
				{isError ? <QueryErrorBanner testId={`${testIdPrefix}-query-error`} /> : null}
				<DimensionRankingsTab
					data={data ?? null}
					loading={loading}
					dimensionLabel={dimensionLabel}
					testIdPrefix={testIdPrefix}
					attributed={dimension === "team" || dimension === "business_unit" || dimension === "customer"}
				/>
			</div>
		);
	},
);