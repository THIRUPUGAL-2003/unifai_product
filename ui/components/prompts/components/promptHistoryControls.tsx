import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Label } from "@/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { getErrorMessage, useIsAuthEnabledQuery } from "@/lib/store";
import {
	useGetPromptHistorySettingsQuery,
	useUpdatePromptHistorySettingsMutation,
} from "@/lib/store/apis/promptsApi";
import { toast } from "sonner";
import { isPromptMemberRole } from "../utils/memberRole";

interface PromptHistoryControlsProps {
	className?: string;
}

export default function PromptHistoryControls({ className }: PromptHistoryControlsProps) {
	const { data: authStatus } = useIsAuthEnabledQuery();
	const isUserRole = isPromptMemberRole(authStatus?.role);

	const { data: settings, isLoading, isError: settingsFailed, error: settingsError } = useGetPromptHistorySettingsQuery();
	const [updateSettings, { isLoading: isUpdating }] = useUpdatePromptHistorySettingsMutation();

	const handleToggleAutoDelete = async (checked: boolean) => {
		try {
			await updateSettings({
				auto_delete: checked,
				retention: settings?.retention || "7d",
			}).unwrap();
			toast.success(checked ? "Prompt history auto-delete enabled" : "Prompt history auto-delete disabled");
		} catch (err) {
			toast.error("Failed to update auto-delete setting", { description: getErrorMessage(err) });
		}
	};

	const handleChangeRetention = async (retention: string) => {
		try {
			await updateSettings({
				auto_delete: true,
				retention,
			}).unwrap();
			toast.success(`Prompt history retention set to ${retention}`);
		} catch (err) {
			toast.error("Failed to update retention period", { description: getErrorMessage(err) });
		}
	};

	if (settingsFailed) {
		return (
			<QueryErrorBanner
				className={className}
				testId="prompt-history-settings-query-error"
				message={getErrorMessage(settingsError) || "Failed to load prompt history settings."}
			/>
		);
	}

	return (
		<div className={`flex items-center gap-2 ${className || ""}`}>
			<div className="flex items-center gap-2 bg-card border border-border px-2.5 py-1 rounded-md">
				<Switch
					checked={!!settings?.auto_delete}
					onCheckedChange={handleToggleAutoDelete}
					disabled={isLoading || isUpdating}
					id="prompt-history-auto-delete"
					data-testid="prompt-history-auto-delete-switch"
				/>
				<Label htmlFor="prompt-history-auto-delete" className="cursor-pointer font-medium text-xs whitespace-nowrap">
					Auto-delete
				</Label>
				{settings?.auto_delete ? (
					<Select
						value={
							["1d", "7d", "30d", "90d", "180d", "365d"].includes(settings?.retention || "")
								? settings.retention
								: "7d"
						}
						onValueChange={handleChangeRetention}
						disabled={isLoading || isUpdating}
					>
						<SelectTrigger className="h-7 w-[7.5rem] text-xs border-border bg-background" data-testid="prompt-history-retention-select">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="1d">1 day</SelectItem>
							<SelectItem value="7d">7 days (1w)</SelectItem>
							<SelectItem value="30d">30 days (1m)</SelectItem>
							<SelectItem value="90d">90 days (3m)</SelectItem>
							<SelectItem value="180d">180 days (6m)</SelectItem>
							<SelectItem value="365d">365 days (1y)</SelectItem>
						</SelectContent>
					</Select>
				) : null}
			</div>
		</div>
	);
}
