import { Button } from "@/components/ui/button";
import { useIsAuthEnabledQuery } from "@/lib/store";
import { SquareTerminal } from "lucide-react";
import { usePromptContext } from "../context";
import { isPromptMemberRole } from "../utils/memberRole";

export function EmptyState() {
	const { setPromptSheet, canCreate } = usePromptContext();
	const { data: authStatus } = useIsAuthEnabledQuery();
	const isUserRole = isPromptMemberRole(authStatus?.role);

	return (
		<div className="text-muted-foreground flex h-full items-center justify-center">
			<div className="text-center">
				<p className="text-lg font-medium">No prompt selected</p>
				<p className="text-sm">
					{isUserRole
						? "Select a prompt from Your prompts in the sidebar"
						: canCreate ? (
								<>
									Select a prompt from the sidebar or{" "}
									<Button
										variant="link"
										className="h-auto p-0 text-sm"
										data-testid="empty-state-create-prompt-link"
										onClick={() => setPromptSheet({ open: true })}
									>
										create a new one
									</Button>
								</>
							) : (
								"Select a prompt from the sidebar"
							)}
				</p>
			</div>
		</div>
	);
}

export function PromptsEmptyState() {
	const { setPromptSheet, canCreate } = usePromptContext();
	const { data: authStatus } = useIsAuthEnabledQuery();
	const isUserRole = isPromptMemberRole(authStatus?.role);

	if (isUserRole) {
		return (
			<div className="flex min-h-[80vh] w-full flex-col items-center justify-center gap-4 py-16 text-center">
				<div className="text-muted-foreground">
					<SquareTerminal className="h-[5.5rem] w-[5.5rem]" strokeWidth={1} />
				</div>
				<div className="flex flex-col gap-1">
					<h1 className="text-muted-foreground text-xl font-medium">No prompts available</h1>
					<div className="text-muted-foreground mx-auto mt-2 max-w-[520px] text-sm font-normal">
						Your admin has not assigned any Prompt Repositories to your account yet. Ask them to open Governance → Users →
						Edit → Allowed Prompt Repositories and check the prompts you should use.
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="flex min-h-[80vh] w-full flex-col items-center justify-center gap-4 py-16 text-center">
			<div className="text-muted-foreground">
				<SquareTerminal className="h-[5.5rem] w-[5.5rem]" strokeWidth={1} />
			</div>
			<div className="flex flex-col gap-1">
				<h1 className="text-muted-foreground text-xl font-medium">Build, test, and version your prompts</h1>
				<div className="text-muted-foreground mx-auto mt-2 max-w-[600px] text-sm font-normal">
					{canCreate
						? "Create prompts, test them with different models and parameters in the playground, and version your changes for deployment."
						: "View prompts and test them with different models and parameters in the playground."}
				</div>
				{canCreate && (
					<div className="mx-auto mt-6 flex flex-row flex-wrap items-center justify-center gap-2">
						<Button
							aria-label="Create your first prompt"
							data-testid="empty-state-create-prompt"
							onClick={() => setPromptSheet({ open: true })}
						>
							Create Prompt
						</Button>
					</div>
				)}
			</div>
		</div>
	);
}
