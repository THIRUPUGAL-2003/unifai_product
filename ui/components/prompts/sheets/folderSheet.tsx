import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage } from "@/lib/store";
import { useCreateFolderMutation, useGetFoldersQuery, useUpdateFolderMutation } from "@/lib/store/apis/promptsApi";
import { Folder } from "@/lib/types/prompts";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";

interface FolderFormData {
	name: string;
	description: string;
}

interface FolderSheetProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	folder?: Folder;
	parentId?: string | null;
	onSaved: () => void;
}

export function FolderSheet({ open, onOpenChange, folder, parentId, onSaved }: FolderSheetProps) {
	const [createFolder, { isLoading: isCreating }] = useCreateFolderMutation();
	const [updateFolder, { isLoading: isUpdating }] = useUpdateFolderMutation();
	const { data: foldersData } = useGetFoldersQuery(undefined, { skip: !open });

	const [selectedParentId, setSelectedParentId] = useState<string>("");

	const isLoading = isCreating || isUpdating;
	const isEditing = !!folder;

	const {
		register,
		handleSubmit,
		reset,
		formState: { errors },
	} = useForm<FolderFormData>({
		defaultValues: { name: "", description: "" },
	});

	useEffect(() => {
		if (open) {
			reset({
				name: folder?.name ?? "",
				description: folder?.description ?? "",
			});
			if (isEditing) {
				setSelectedParentId(folder?.parent_id ?? "");
			} else {
				setSelectedParentId(parentId ?? "");
			}
		}
	}, [open, folder, parentId, isEditing, reset]);

	// Filter out current folder when editing to prevent self-parenting loop
	const availableFolders = useMemo(() => {
		const all = foldersData?.folders ?? [];
		if (!isEditing || !folder) return all;
		return all.filter((f) => f.id !== folder.id);
	}, [foldersData?.folders, isEditing, folder]);

	async function onSubmit(data: FolderFormData) {
		try {
			if (isEditing) {
				await updateFolder({
					id: folder.id,
					data: {
						name: data.name.trim(),
						description: data.description.trim() || undefined,
						parent_id: selectedParentId || null,
					},
				}).unwrap();
				toast.success("Folder updated");
			} else {
				await createFolder({
					name: data.name.trim(),
					parent_id: selectedParentId ? selectedParentId : undefined,
					description: data.description.trim() || undefined,
				}).unwrap();
				toast.success("Folder created");
			}
			onSaved();
			onOpenChange(false);
		} catch (err) {
			toast.error(`Failed to ${isEditing ? "update" : "create"} folder`, {
				description: getErrorMessage(err),
			});
		}
	}

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent
				className="p-8"
				onOpenAutoFocus={(e) => {
					e.preventDefault();
					document.getElementById("name")?.focus();
				}}
			>
				<form onSubmit={handleSubmit(onSubmit)}>
					<SheetHeader className="flex flex-col items-start">
						<SheetTitle>{isEditing ? "Edit Folder" : selectedParentId ? "Create Subfolder" : "Create Folder"}</SheetTitle>
						<SheetDescription>
							{isEditing
								? "Update the folder name, description, or parent folder location."
								: "Create a new folder or subfolder to organize your prompts."}
						</SheetDescription>
					</SheetHeader>

					<div className="mt-6 space-y-4">
						<div className="space-y-2">
							<Label htmlFor="name">Name</Label>
							<Input
								id="name"
								data-testid="folder-name-input"
								placeholder="e.g. Sales Team, Customer Alpha, QA Tests"
								{...register("name", {
									required: "Folder name is required",
									validate: (v) => v.trim().length > 0 || "Folder name cannot be blank",
								})}
								autoFocus
							/>
							{errors.name && <p className="text-destructive text-xs">{errors.name.message}</p>}
						</div>

						<div className="space-y-2">
							<Label htmlFor="parent-folder-select">Parent Folder (Place inside)</Label>
							<select
								id="parent-folder-select"
								data-testid="folder-parent-select"
								value={selectedParentId}
								onChange={(e) => setSelectedParentId(e.target.value)}
								className="bg-muted/20 border-border/50 text-foreground w-full rounded-lg border p-2.5 text-sm focus:border-teal-500/50 focus:outline-none"
							>
								<option value="">(None - Root level)</option>
								{availableFolders.map((f) => (
									<option key={f.id} value={f.id}>
										{f.name} {f.type ? `(${f.type})` : ""}
									</option>
								))}
							</select>
							<p className="text-muted-foreground text-xs">
								Choose a parent folder to create a subfolder (folder inside folder), or leave as Root level.
							</p>
						</div>

						<div className="space-y-2">
							<Label htmlFor="description">Description (optional)</Label>
							<Textarea
								id="description"
								data-testid="folder-description-input"
								placeholder="Describe what prompts or subfolders this workspace contains..."
								className="resize-none"
								{...register("description")}
							/>
						</div>
					</div>

					<SheetFooter className="mt-6 flex flex-row items-center justify-end gap-2 p-0">
						<Button type="button" variant="outline" data-testid="folder-cancel" onClick={() => onOpenChange(false)}>
							Cancel
						</Button>
						<Button type="submit" data-testid="folder-submit" disabled={isLoading}>
							{isLoading ? "Saving..." : isEditing ? "Update" : "Create"}
						</Button>
					</SheetFooter>
				</form>
			</SheetContent>
		</Sheet>
	);
}