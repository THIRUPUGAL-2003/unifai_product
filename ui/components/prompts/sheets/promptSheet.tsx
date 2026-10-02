import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { getErrorMessage } from "@/lib/store";
import { useCreatePromptMutation, useGetFoldersQuery, useUpdatePromptMutation } from "@/lib/store/apis/promptsApi";
import { Prompt } from "@/lib/types/prompts";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";

interface PromptFormData {
	name: string;
}

interface PromptSheetProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	prompt?: Prompt;
	folderId?: string;
	onSaved: (promptId?: string) => void;
}

export function PromptSheet({ open, onOpenChange, prompt, folderId, onSaved }: PromptSheetProps) {
	const [createPrompt, { isLoading: isCreating }] = useCreatePromptMutation();
	const [updatePrompt, { isLoading: isUpdating }] = useUpdatePromptMutation();
	const { data: foldersData } = useGetFoldersQuery(undefined, { skip: !open });

	const [selectedFolderId, setSelectedFolderId] = useState<string>("");

	const isLoading = isCreating || isUpdating;
	const isEditing = !!prompt;

	const {
		register,
		handleSubmit,
		reset,
		formState: { errors },
	} = useForm<PromptFormData>({
		defaultValues: { name: "" },
	});

	useEffect(() => {
		if (open) {
			reset({ name: prompt?.name ?? "" });
			if (isEditing) {
				setSelectedFolderId(prompt?.folder_id ?? "");
			} else {
				setSelectedFolderId(folderId ?? "");
			}
		}
	}, [open, prompt, folderId, isEditing, reset]);

	const folders = foldersData?.folders ?? [];

	async function onSubmit(data: PromptFormData) {
		try {
			if (isEditing) {
				await updatePrompt({
					id: prompt.id,
					data: {
						name: data.name.trim(),
						folder_id: selectedFolderId || null,
					},
				}).unwrap();
				toast.success("Prompt updated");
				onSaved();
			} else {
				const result = await createPrompt({
					name: data.name.trim(),
					...(selectedFolderId ? { folder_id: selectedFolderId } : {}),
				}).unwrap();
				toast.success("Prompt created");
				onSaved(result.prompt.id);
			}
			onOpenChange(false);
		} catch (err) {
			toast.error(`Failed to ${isEditing ? "update" : "create"} prompt`, {
				description: getErrorMessage(err),
			});
		}
	}

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent
				className="p-0"
				onOpenAutoFocus={(e) => {
					e.preventDefault();
					document.getElementById("name")?.focus();
				}}
			>
				<form onSubmit={handleSubmit(onSubmit)} className="flex grow flex-col">
					<SheetHeader className="flex flex-col items-start px-8 pt-8">
						<SheetTitle>{isEditing ? "Edit Prompt" : "Create Prompt"}</SheetTitle>
						<SheetDescription>
							{isEditing ? "Update the prompt name or move it to a different folder." : "Create a new prompt in your chosen workspace or folder."}
						</SheetDescription>
					</SheetHeader>

					<div className="flex grow flex-col gap-6">
						<div className="grow space-y-4 px-8">
							<div className="space-y-2">
								<Label htmlFor="name">Name</Label>
								<Input
									id="name"
									data-testid="prompt-name-input"
									placeholder="e.g. Customer Support Assistant"
									{...register("name", {
										required: "Prompt name is required",
										validate: (v) => v.trim().length > 0 || "Prompt name cannot be blank",
									})}
									autoFocus
								/>
								{errors.name && <p className="text-destructive text-xs">{errors.name.message}</p>}
							</div>

							<div className="space-y-2">
								<Label htmlFor="prompt-folder-select">Folder (Place inside)</Label>
								<select
									id="prompt-folder-select"
									data-testid="prompt-folder-select"
									value={selectedFolderId}
									onChange={(e) => setSelectedFolderId(e.target.value)}
									className="bg-muted/20 border-border/50 text-foreground w-full rounded-lg border p-2.5 text-sm focus:border-teal-500/50 focus:outline-none"
								>
									<option value="">(None - Root level)</option>
									{folders.map((f) => (
										<option key={f.id} value={f.id}>
											{f.name} {f.type ? `(${f.type})` : ""}
										</option>
									))}
								</select>
								<p className="text-muted-foreground text-xs">
									Select which customer, team, or custom folder to place this prompt in.
								</p>
							</div>
						</div>

						<SheetFooter className="flex flex-row items-center justify-end gap-2 border-t px-8 py-4">
							<Button type="button" variant="outline" data-testid="prompt-cancel" onClick={() => onOpenChange(false)}>
								Cancel
							</Button>
							<Button type="submit" data-testid="prompt-submit" disabled={isLoading}>
								{isLoading ? "Saving..." : isEditing ? "Update" : "Create"}
							</Button>
						</SheetFooter>
					</div>
				</form>
			</SheetContent>
		</Sheet>
	);
}