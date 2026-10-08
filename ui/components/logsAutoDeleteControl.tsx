import { Label } from "@/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage, useGetCoreConfigQuery, useTriggerLogCleanupMutation, useUpdateCoreConfigMutation } from "@/lib/store";
import { cn } from "@/lib/utils";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { Clock, Loader2 } from "lucide-react";
import React, { useCallback, useMemo, useState } from "react";
import { toast } from "sonner";

interface LogsAutoDeleteControlProps {
	onRetentionChange?: (days: number) => void;
	className?: string;
	testIdPrefix?: string;
	compact?: boolean;
}

const RETENTION_OPTIONS = [
	{ value: "1d", days: 1, label: "1 day" },
	{ value: "7d", days: 7, label: "7 days (1w)" },
	{ value: "30d", days: 30, label: "30 days (1m)" },
	{ value: "90d", days: 90, label: "90 days (3m)" },
	{ value: "180d", days: 180, label: "180 days (6m)" },
	{ value: "365d", days: 365, label: "365 days (1y)" },
] as const;

export function LogsAutoDeleteControl({
	onRetentionChange,
	className,
	testIdPrefix = "logs",
	compact = false,
}: LogsAutoDeleteControlProps) {
	const hasSettingsUpdateAccess = useRbac(RbacResource.Settings, RbacOperation.Update);
	const { data: gatewayConfig, isLoading: isConfigLoading } = useGetCoreConfigQuery({ fromDB: true });
	const [updateCoreConfig, { isLoading: isUpdating }] = useUpdateCoreConfigMutation();
	const [triggerLogCleanup, { isLoading: isCleaning }] = useTriggerLogCleanupMutation();

	// Local optimistic state for smooth UI interaction
	const [optimisticDays, setOptimisticDays] = useState<number | null>(null);

	const clientConfig = gatewayConfig?.client_config;
	const currentRetentionDays = optimisticDays !== null ? optimisticDays : (clientConfig?.log_retention_days ?? 365);
	const isAutoDeleteEnabled = currentRetentionDays > 0;

	// Resolve the select option value matching the retention days
	const selectedValue = useMemo(() => {
		const matched = RETENTION_OPTIONS.find((opt) => opt.days === currentRetentionDays);
		if (matched) return matched.value;
		return `${currentRetentionDays}d`;
	}, [currentRetentionDays]);

	const handleToggle = useCallback(
		async (enabled: boolean) => {
			if (!gatewayConfig || !clientConfig) return;
			const targetDays = enabled ? (clientConfig.log_retention_days > 0 ? clientConfig.log_retention_days : 30) : 0;
			setOptimisticDays(targetDays);

			try {
				await updateCoreConfig({
					...gatewayConfig,
					client_config: {
						...clientConfig,
						log_retention_days: targetDays,
					},
				}).unwrap();

				if (enabled) {
					try {
						const cleanupRes = await triggerLogCleanup({ retention_days: targetDays }).unwrap();
						const totalDeleted = (cleanupRes?.deleted_logs || 0) + (cleanupRes?.deleted_mcp_logs || 0);
						if (totalDeleted > 0) {
							toast.success(`Log auto-delete enabled (${targetDays} days retention, ${totalDeleted} older logs pruned)`);
						} else {
							toast.success(`Log auto-delete enabled (${targetDays} days retention)`);
						}
					} catch {
						toast.success(`Log auto-delete enabled (${targetDays} days retention)`);
					}
				} else {
					toast.info("Log auto-delete disabled (logs will be retained indefinitely)");
				}
				onRetentionChange?.(targetDays);
			} catch (err) {
				setOptimisticDays(null);
				toast.error("Failed to update auto-delete configuration", {
					description: getErrorMessage(err),
				});
			}
		},
		[gatewayConfig, clientConfig, updateCoreConfig, triggerLogCleanup, onRetentionChange],
	);

	const handleRetentionPeriodChange = useCallback(
		async (value: string) => {
			if (!gatewayConfig || !clientConfig) return;
			const matched = RETENTION_OPTIONS.find((opt) => opt.value === value);
			const targetDays = matched ? matched.days : parseInt(value) || 30;
			setOptimisticDays(targetDays);

			try {
				await updateCoreConfig({
					...gatewayConfig,
					client_config: {
						...clientConfig,
						log_retention_days: targetDays,
					},
				}).unwrap();

				try {
					const cleanupRes = await triggerLogCleanup({ retention_days: targetDays }).unwrap();
					const totalDeleted = (cleanupRes?.deleted_logs || 0) + (cleanupRes?.deleted_mcp_logs || 0);
					if (totalDeleted > 0) {
						toast.success(`Log retention set to ${matched?.label || `${targetDays} days`} (${totalDeleted} older logs pruned)`);
					} else {
						toast.success(`Log retention updated to ${matched?.label || `${targetDays} days`}`);
					}
				} catch {
					toast.success(`Log retention updated to ${matched?.label || `${targetDays} days`}`);
				}

				onRetentionChange?.(targetDays);
			} catch (err) {
				setOptimisticDays(null);
				toast.error("Failed to update retention period", {
					description: getErrorMessage(err),
				});
			}
		},
		[gatewayConfig, clientConfig, updateCoreConfig, triggerLogCleanup, onRetentionChange],
	);

	const switchId = `${testIdPrefix}-auto-delete-switch`;

	const isBusy = isUpdating || isCleaning;

	const controlContent = (
		<div
			className={cn(
				"flex items-center gap-2 bg-card border border-border px-2.5 py-1 rounded-md text-xs shadow-xs transition-colors",
				isBusy && "opacity-80 pointer-events-none",
				className,
			)}
			data-testid={`${testIdPrefix}-auto-delete-container`}
		>
			{isBusy ? (
				<Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
			) : (
				<Clock className="h-3.5 w-3.5 text-muted-foreground" />
			)}

			<Switch
				id={switchId}
				size="default"
				checked={isAutoDeleteEnabled}
				disabled={isConfigLoading || isBusy || !hasSettingsUpdateAccess}
				onCheckedChange={handleToggle}
				data-testid={`${testIdPrefix}-auto-delete-toggle`}
			/>

			<Label
				htmlFor={switchId}
				className="cursor-pointer font-medium text-xs whitespace-nowrap select-none"
			>
				Auto-delete
			</Label>

			{isAutoDeleteEnabled ? (
				<Select
					value={selectedValue}
					disabled={isConfigLoading || isBusy || !hasSettingsUpdateAccess}
					onValueChange={handleRetentionPeriodChange}
				>
					<SelectTrigger
						className={cn(
							"h-7 w-[7.8rem] text-xs border-border bg-background focus:ring-1",
							compact && "w-[6.8rem]",
						)}
						data-testid={`${testIdPrefix}-auto-delete-period-trigger`}
					>
						<SelectValue placeholder="Retention" />
					</SelectTrigger>
					<SelectContent>
						{RETENTION_OPTIONS.map((opt) => (
							<SelectItem key={opt.value} value={opt.value} className="text-xs">
								{opt.label}
							</SelectItem>
						))}
						{!RETENTION_OPTIONS.some((opt) => opt.value === selectedValue) && currentRetentionDays > 0 ? (
							<SelectItem value={selectedValue} className="text-xs">
								{currentRetentionDays} days
							</SelectItem>
						) : null}
					</SelectContent>
				</Select>
			) : null}
		</div>
	);

	if (!hasSettingsUpdateAccess) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>{controlContent}</TooltipTrigger>
				<TooltipContent side="bottom">
					<p className="text-xs">You need settings update permission to configure log auto-deletion.</p>
				</TooltipContent>
			</Tooltip>
		);
	}

	return controlContent;
}
