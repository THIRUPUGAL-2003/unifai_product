import FullPageLoader from "@/components/fullPageLoader";
import { useIsAuthEnabledQuery } from "@/lib/store";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { AlertCircle, Loader2 } from "lucide-react";
import { PromptSidebar } from "./fragments/sidebar";
import { PlaygroundPanel } from "./fragments/playgroundPanel";
import { SettingsPanel } from "./fragments/settingsPanel";
import { DeleteFolderDialog, DeletePromptDialog } from "./components/alerts";
import { PromptSheets } from "./components/sheets";
import { PromptAccessDialog } from "./components/promptAccessDialog";
import { EmptyState, PromptsEmptyState } from "./components/emptyState";
import PromptsViewHeader from "./components/promptsViewHeader";
import { usePromptContext } from "./context";
import { isPromptMemberRole } from "./utils/memberRole";
import PromptHistoryControls from "./components/promptHistoryControls";

export default function PromptsView() {
	const { folders, prompts, foldersLoading, promptsLoading, foldersError, promptsError, isLoadingPlayground, selectedPromptId } =
		usePromptContext();

	const { data: authStatus } = useIsAuthEnabledQuery();
	const isUserRole = isPromptMemberRole(authStatus?.role);

	if (foldersLoading || promptsLoading) {
		return <FullPageLoader />;
	}

	if (foldersError || promptsError) {
		return (
			<div className="no-padding-parent no-border-parent p-4">
				<Alert variant="destructive">
					<AlertCircle className="h-4 w-4" />
					<AlertDescription>Failed to load prompt repository</AlertDescription>
				</Alert>
			</div>
		);
	}

	return (
		<div className="no-padding-parent no-border-parent bg-background h-[calc(100dvh_-_16px)] w-full">
			<DeleteFolderDialog />
			<DeletePromptDialog />
			<PromptSheets />
			<PromptAccessDialog />

			<ResizablePanelGroup direction="horizontal" className="h-full">
				<ResizablePanel defaultSize="24%" minSize="18%" maxSize="35%" className="bg-card mr-1 overflow-hidden rounded-r-md">
					<PromptSidebar />
				</ResizablePanel>

				<ResizableHandle className="mr-1 bg-transparent" />

				<ResizablePanel defaultSize="76%" minSize="65%" className="overflow-hidden">
					<div className="bg-card h-full w-full min-w-0 overflow-hidden rounded-md">
						{selectedPromptId ? (
							<div className="flex h-full flex-col">
								<PromptsViewHeader />

								{isLoadingPlayground ? (
									<div className="flex flex-1 items-center justify-center">
										<Loader2 className="text-muted-foreground h-5 w-5 animate-spin" />
									</div>
								) : (
									<ResizablePanelGroup direction="horizontal" className="flex-1">
										<ResizablePanel defaultSize="70%" minSize="40%">
											<PlaygroundPanel />
										</ResizablePanel>
										<ResizableHandle />
										<ResizablePanel defaultSize="30%" minSize="20%">
											<SettingsPanel />
										</ResizablePanel>
									</ResizablePanelGroup>
								)}
							</div>
						) : (
							<div className="flex h-full flex-col">
								{!isUserRole && (
									<div className="flex items-center justify-end border-b px-4 py-3">
										<PromptHistoryControls />
									</div>
								)}
								<div className="flex-1">
									<EmptyState />
								</div>
							</div>
						)}
					</div>
				</ResizablePanel>
			</ResizablePanelGroup>
		</div>
	);
}