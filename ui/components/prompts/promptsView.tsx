import FullPageLoader from "@/components/fullPageLoader";
import { useIsAuthEnabledQuery } from "@/lib/store";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { AlertCircle, Loader2, PanelLeftOpen } from "lucide-react";
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
	const {
		folders,
		prompts,
		foldersLoading,
		promptsLoading,
		foldersError,
		promptsError,
		isLoadingPlayground,
		playgroundError,
		selectedPromptId,
		isSidebarOpen,
		toggleSidebar,
		isSettingsOpen,
	} = usePromptContext();

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
		<div className="no-padding-parent no-border-parent bg-background flex h-full max-h-full min-h-0 w-full flex-col overflow-hidden">
			<DeleteFolderDialog />
			<DeletePromptDialog />
			<PromptSheets />
			<PromptAccessDialog />

			<ResizablePanelGroup
				key={`outer-group-${isSidebarOpen ? "expanded" : "collapsed"}`}
				direction="horizontal"
				className="h-full min-h-0 flex-1"
			>
				{isSidebarOpen && (
					<>
						<ResizablePanel defaultSize="22%" minSize="18%" maxSize="32%" className="bg-card mr-1 overflow-hidden rounded-r-md">
							<PromptSidebar />
						</ResizablePanel>
						<ResizableHandle className="mr-1 bg-transparent" />
					</>
				)}

				<ResizablePanel defaultSize={isSidebarOpen ? "78%" : "100%"} minSize="40%" className="min-h-0 flex flex-col overflow-hidden">
					<div className="bg-card flex h-full w-full min-w-0 min-h-0 flex-col overflow-hidden rounded-md">
						{selectedPromptId ? (
							<div className="flex h-full min-h-0 flex-col">
								<PromptsViewHeader />

								{playgroundError ? (
									<div className="flex flex-1 items-center justify-center p-4">
										<Alert variant="destructive" className="max-w-md">
											<AlertCircle className="h-4 w-4" />
											<AlertDescription>{playgroundError}</AlertDescription>
										</Alert>
									</div>
								) : isLoadingPlayground ? (
									<div className="flex flex-1 items-center justify-center">
										<Loader2 className="text-muted-foreground h-5 w-5 animate-spin" />
									</div>
								) : (
									<ResizablePanelGroup
										key={`inner-group-${isSettingsOpen ? "expanded" : "collapsed"}`}
										direction="horizontal"
										className="flex-1 min-h-0"
									>
										<ResizablePanel defaultSize={isSettingsOpen ? "68%" : "100%"} minSize="35%" className="min-h-0 flex flex-col overflow-hidden">
											<PlaygroundPanel />
										</ResizablePanel>
										{isSettingsOpen && (
											<>
												<ResizableHandle />
												<ResizablePanel defaultSize="32%" minSize="22%" maxSize="50%" className="min-h-0 flex flex-col overflow-hidden">
													<SettingsPanel />
												</ResizablePanel>
											</>
										)}
									</ResizablePanelGroup>
								)}
							</div>
						) : (
							<div className="flex h-full flex-col">
								<div className="flex items-center justify-between border-b px-4 py-3">
									{!isSidebarOpen ? (
										<Button
											variant="outline"
											size="sm"
											onClick={toggleSidebar}
											className="h-8 gap-1.5"
											data-testid="show-prompts-sidebar-btn"
										>
											<PanelLeftOpen className="h-4 w-4" />
											Show Prompts
										</Button>
									) : (
										<div />
									)}
									{!isUserRole && (
										<div className="ml-auto flex items-center gap-2">
											<PromptHistoryControls />
										</div>
									)}
								</div>
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