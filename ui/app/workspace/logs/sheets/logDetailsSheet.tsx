import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { SheetNavigationButtons } from "@/components/sheetNavigationButtons";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { getErrorMessage } from "@/lib/store";
import { useGetLogByIdQuery } from "@/lib/store/apis/logsApi";
import { useGetPromptQuery } from "@/lib/store/apis/promptsApi";
import type { LogEntry } from "@/lib/types/logs";
import { useSheetNavigation } from "@/hooks/useSheetNavigation";
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { LogDetailView } from "./logDetailView";

interface LogDetailSheetProps {
	log: LogEntry | null;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	handleDelete?: (log: LogEntry) => void;
	onNavigate?: (direction: "prev" | "next") => void;
	hasPrev?: boolean;
	hasNext?: boolean;
	onViewSession?: (sessionId: string, logId: string) => void;
	onFilterByParentRequestId?: (parentRequestId: string) => void;
}

export function LogDetailSheet({
	log,
	open,
	onOpenChange,
	handleDelete,
	onNavigate,
	hasPrev = false,
	hasNext = false,
	onViewSession,
	onFilterByParentRequestId,
}: LogDetailSheetProps) {
	const [pollingInterval, setPollingInterval] = useState(0);
	const {
		data: fullLog,
		isLoading,
		isError,
		error: logFetchError,
		refetch: refetchLog,
	} = useGetLogByIdQuery(log?.id ?? "", {
		skip: !open || !log?.id,
		pollingInterval,
	});

	// Only poll while still processing — never hammer on permanent fetch errors.
	const shouldPoll = !isError && fullLog?.status === "processing";

	const isFullDataReady = log != null && (isError || (fullLog?.id === log.id && !isLoading));
	// Prefer full log when loaded; otherwise list row — enables prompt fetch in parallel with getLogById
	const selectedPromptId = log ? (fullLog?.id === log.id ? fullLog : log).selected_prompt_id : undefined;
	const {
		data: selectedPromptData,
		isError: promptFailed,
		error: promptError,
		refetch: refetchPrompt,
	} = useGetPromptQuery(selectedPromptId ?? "", {
		skip: !open || !selectedPromptId,
	});

	useEffect(() => {
		setPollingInterval(shouldPoll ? 2000 : 0);
	}, [shouldPoll]);

	// Keyboard navigation: arrow up/down to navigate between logs
	const { prev: prevKeys, next: nextKeys } = useSheetNavigation({
		enabled: open,
		hasPrev,
		hasNext,
		onNavigate: (direction) => onNavigate?.(direction),
	});

	if (!log) return null;

	// Show a loader only on the initial fetch, not during background polling refetches.
	const displayLog: LogEntry = isFullDataReady && fullLog ? fullLog : log;
	const resolvedSelectedPromptName = selectedPromptData?.prompt?.name ?? displayLog.selected_prompt_name ?? "";

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="border-secondary flex w-full flex-col gap-4 overflow-x-hidden border p-8 sm:max-w-[60%]">
				{!isFullDataReady ? (
					<div className="flex h-full items-center justify-center">
						<SheetTitle className="sr-only">Loading log details</SheetTitle>
						<Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
					</div>
				) : (
					<>
						{isError ? (
							<div className="flex flex-col gap-2">
								<QueryErrorBanner
									testId="log-detail-fetch-error"
									message={`Could not load full log details: ${getErrorMessage(logFetchError)}. Showing list summary.`}
								/>
								<Button type="button" variant="outline" size="sm" className="w-fit" onClick={() => void refetchLog()}>
									Retry
								</Button>
							</div>
						) : null}
						{promptFailed && selectedPromptId ? (
							<div className="flex flex-col gap-2">
								<QueryErrorBanner
									testId="log-detail-prompt-fetch-error"
									message={`Could not load prompt name: ${getErrorMessage(promptError)}. Showing log summary name if available.`}
								/>
								<Button type="button" variant="outline" size="sm" className="w-fit" onClick={() => void refetchPrompt()}>
									Retry prompt
								</Button>
							</div>
						) : null}
						<LogDetailView
							log={displayLog}
							resolvedSelectedPromptName={resolvedSelectedPromptName}
							handleDelete={handleDelete}
							onClose={() => onOpenChange(false)}
							onFilterByParentRequestId={onFilterByParentRequestId}
							headerAction={
								<>
									{displayLog.parent_request_id && onViewSession ? (
										<Button
											variant="outline"
											size="sm"
											data-testid="session-button-view"
											onClick={() => onViewSession(displayLog.parent_request_id as string, displayLog.id)}
										>
											View Session
										</Button>
									) : null}
									<SheetNavigationButtons
										hasPrev={hasPrev}
										hasNext={hasNext}
										onNavigate={(dir) => onNavigate?.(dir)}
										prevKeys={prevKeys}
										nextKeys={nextKeys}
										entityLabel="log"
									/>
								</>
							}
						/>
					</>
				)}
			</SheetContent>
		</Sheet>
	);
}