import { QueryErrorBanner } from "@/components/queryErrorBanner";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alertDialog";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import NumberAndSelect from "@/components/ui/numberAndSelect";
import { Progress } from "@/components/ui/progress";
import { budgetDurationOptions, resetDurationLabels, resetDurationOptions } from "@/lib/constants/governance";
import { formatCurrency } from "@/lib/utils/governance";
import { cn } from "@/lib/utils";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage, useGetCustomersQuery, useGetTeamsQuery } from "@/lib/store";
import {
	useAssignBusinessUnitTeamMutation,
	useCreateBusinessUnitMutation,
	useUpdateBusinessUnitMutation,
	useDeleteBusinessUnitMutation,
	useGetBusinessUnitTeamsQuery,
	useGetBusinessUnitsQuery,
	useRemoveBusinessUnitTeamMutation,
} from "@enterprise/lib/store/apis/businessUnitsApi";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { BusinessUnit } from "@enterprise/lib/types/workspace";
import { Building2, Pencil, Plus, Search, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { DataTablePagination } from "@/components/table/dataTablePagination";

function budgetPayload(limit: number | undefined, reset: string) {
	if (limit === undefined || !Number.isFinite(limit) || limit <= 0) {
		return { max_limit: 0, reset_duration: reset || "1M" };
	}
	return { max_limit: limit, reset_duration: reset || "1M" };
}

function ratePayload(tokens: number | undefined, tokenReset: string, requests: number | undefined, requestReset: string) {
	const rate: {
		token_max_limit?: number;
		token_reset_duration?: string;
		request_max_limit?: number;
		request_reset_duration?: string;
	} = {};
	if (tokens !== undefined && Number.isFinite(tokens) && tokens > 0) {
		rate.token_max_limit = Math.floor(tokens);
		rate.token_reset_duration = tokenReset || "1h";
	}
	if (requests !== undefined && Number.isFinite(requests) && requests > 0) {
		rate.request_max_limit = Math.floor(requests);
		rate.request_reset_duration = requestReset || "1h";
	}
	return rate;
}

function moneyLabel(used?: number, max?: number) {
	if (!max || max <= 0) return "—";
	return `${formatCurrency(used ?? 0)} / ${formatCurrency(max)}`;
}

function countLabel(used?: number, max?: number) {
	if (!max || max <= 0) return "—";
	return `${Math.round(used ?? 0).toLocaleString()} / ${Math.round(max).toLocaleString()}`;
}

function LimitMeter({
	used = 0,
	max,
	reset,
	money,
}: {
	used?: number;
	max?: number;
	reset?: string;
	money?: boolean;
}) {
	if (!max || max <= 0) return <span className="text-muted-foreground text-sm">-</span>;
	const pct = Math.min((used / max) * 100, 100);
	const exhausted = used >= max;
	const limitLabel = money ? formatCurrency(max) : max.toLocaleString();
	const usedLabel = money ? formatCurrency(used) : used.toLocaleString();
	return (
		<div className="min-w-[140px] space-y-1">
			<div className="flex items-center justify-between gap-3">
				<span className="text-sm font-medium">{limitLabel}</span>
				<span className="text-muted-foreground text-xs">{resetDurationLabels[reset || ""] || reset}</span>
			</div>
			<Progress
				value={pct}
				className={cn(
					"bg-muted/70 h-1.5",
					exhausted ? "[&>div]:bg-red-500/70" : pct > 80 ? "[&>div]:bg-amber-500/70" : "[&>div]:bg-emerald-500/70",
				)}
			/>
			<p className="text-muted-foreground text-xs">{usedLabel} used</p>
		</div>
	);
}

export function BusinessUnitsView() {
	const [selected, setSelected] = useState<BusinessUnit | null>(null);
	const [search, setSearch] = useState("");
	const [name, setName] = useState("");
	const [budgetLimit, setBudgetLimit] = useState<number | undefined>(undefined);
	const [budgetReset, setBudgetReset] = useState("1M");
	const [tokenLimit, setTokenLimit] = useState<number | undefined>(undefined);
	const [tokenReset, setTokenReset] = useState("1h");
	const [requestLimit, setRequestLimit] = useState<number | undefined>(undefined);
	const [requestReset, setRequestReset] = useState("1h");
	const [createTeamIds, setCreateTeamIds] = useState<string[]>([]);
	const [open, setOpen] = useState(false);
	const [unitToDelete, setUnitToDelete] = useState<BusinessUnit | null>(null);
	const [unitToEdit, setUnitToEdit] = useState<BusinessUnit | null>(null);
	const [editName, setEditName] = useState("");
	const [editBudgetLimit, setEditBudgetLimit] = useState<number | undefined>(undefined);
	const [editBudgetReset, setEditBudgetReset] = useState("1M");
	const [editTokenLimit, setEditTokenLimit] = useState<number | undefined>(undefined);
	const [editTokenReset, setEditTokenReset] = useState("1h");
	const [editRequestLimit, setEditRequestLimit] = useState<number | undefined>(undefined);
	const [editRequestReset, setEditRequestReset] = useState("1h");
	const [editTeamIds, setEditTeamIds] = useState<string[]>([]);
	const [offset, setOffset] = useState(0);
	const [limit, setLimit] = useState(10);
	const hasCreateAccess = useRbac(RbacResource.Teams, RbacOperation.Create);
	const hasUpdateAccess = useRbac(RbacResource.Teams, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.Teams, RbacOperation.Delete);
	const { data: unitData, isError: unitsFailed, error: unitsError } = useGetBusinessUnitsQuery();
	const { data: teamData, isError: teamsFailed, error: teamsError } = useGetTeamsQuery();
	const { data: customersData, isError: customersFailed, error: customersError } = useGetCustomersQuery();
	const listQueryFailed = unitsFailed || teamsFailed || customersFailed;
	const listQueryError = unitsError || teamsError || customersError;
	const {
		data: assignedData,
		isError: assignedTeamsFailed,
		error: assignedTeamsError,
	} = useGetBusinessUnitTeamsQuery(selected?.id ?? "", { skip: !selected });
	const [createUnit] = useCreateBusinessUnitMutation();
	const [updateUnit, { isLoading: isUpdating }] = useUpdateBusinessUnitMutation();
	const [deleteUnit] = useDeleteBusinessUnitMutation();
	const [assignTeam] = useAssignBusinessUnitTeamMutation();
	const [removeTeam] = useRemoveBusinessUnitTeamMutation();
	const units = unitData?.business_units || [];
	const teams = teamData?.teams || [];
	const customers = customersData?.customers || [];
	const assigned = assignedData?.teams || [];

	const filteredUnits = useMemo(() => {
		const q = search.trim().toLowerCase();
		if (!q) return units;
		return units.filter((u) => u.name.toLowerCase().includes(q) || u.id.toLowerCase().includes(q));
	}, [units, search]);

	useEffect(() => {
		setOffset(0);
	}, [search]);

	const pagedUnits = useMemo(() => filteredUnits.slice(offset, offset + limit), [filteredUnits, offset, limit]);

	const teamById = useMemo(() => new Map(teams.map((t) => [t.id, t])), [teams]);
	const customerNameById = useMemo(() => new Map(customers.map((c) => [c.id, c.name])), [customers]);

	const create = async () => {
		try {
			const created = await createUnit({
				name: name.trim(),
				budget: budgetPayload(budgetLimit, budgetReset),
				rate_limit: ratePayload(tokenLimit, tokenReset, requestLimit, requestReset),
			}).unwrap();
			for (const team of createTeamIds) {
				await assignTeam({ id: created.business_unit.id, team_id: team }).unwrap();
			}
			toast.success("Business unit created");
			setOpen(false);
			setName("");
			setBudgetLimit(undefined);
			setBudgetReset("1M");
			setTokenLimit(undefined);
			setTokenReset("1h");
			setRequestLimit(undefined);
			setRequestReset("1h");
			setCreateTeamIds([]);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const openEdit = (unit: BusinessUnit) => {
		setUnitToEdit(unit);
		setEditName(unit.name);
		setEditBudgetLimit(unit.budget?.max_limit || undefined);
		setEditBudgetReset(unit.budget?.reset_duration || "1M");
		setEditTokenLimit(unit.rate_limit?.token_max_limit || undefined);
		setEditTokenReset(unit.rate_limit?.token_reset_duration || "1h");
		setEditRequestLimit(unit.rate_limit?.request_max_limit || undefined);
		setEditRequestReset(unit.rate_limit?.request_reset_duration || "1h");
		setEditTeamIds(unit.team_ids || []);
	};

	const remove = async (id: string) => {
		try {
			await deleteUnit(id).unwrap();
			if (selected?.id === id) setSelected(null);
			toast.success("Business unit deleted");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const handleUpdate = async () => {
		if (!unitToEdit || !editName.trim()) return;
		try {
			const saved = await updateUnit({
				id: unitToEdit.id,
				name: editName.trim(),
				budget: budgetPayload(editBudgetLimit, editBudgetReset),
				rate_limit: ratePayload(editTokenLimit, editTokenReset, editRequestLimit, editRequestReset),
			}).unwrap();
			toast.success("Business unit updated");
			const previous = new Set(unitToEdit.team_ids || []);
			const next = new Set(editTeamIds);
			for (const team of editTeamIds) {
				if (!previous.has(team)) await assignTeam({ id: unitToEdit.id, team_id: team }).unwrap();
			}
			for (const team of previous) {
				if (!next.has(team)) await removeTeam({ id: unitToEdit.id, team_id: team }).unwrap();
			}
			if (selected?.id === unitToEdit.id) {
				setSelected({ ...saved.business_unit, team_ids: editTeamIds, team_count: editTeamIds.length });
			}
			setUnitToEdit(null);
			setEditName("");
			setEditBudgetLimit(undefined);
			setEditBudgetReset("1M");
			setEditTokenLimit(undefined);
			setEditTokenReset("1h");
			setEditRequestLimit(undefined);
			setEditRequestReset("1h");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const toggleTeam = (ids: string[], team: string, setIds: (next: string[]) => void) => {
		setIds(ids.includes(team) ? ids.filter((id) => id !== team) : [...ids, team]);
	};

	const teamLabel = (teamIdValue: string, fallbackName?: string) => {
		const team = teamById.get(teamIdValue);
		const nameLabel = fallbackName || team?.name || teamIdValue;
		const customerName = team?.customer?.name || (team?.customer_id ? customerNameById.get(team.customer_id) : undefined);
		if (customerName) return `${nameLabel} · ${customerName}`;
		return `${nameLabel} · no customer`;
	};

	return (
		<div className="grid h-full grid-cols-1 gap-4 lg:grid-cols-[1fr_360px]">
			<div className="flex flex-col gap-4">
				<div className="flex items-center justify-between">
					<div>
						<h1 className="flex items-center gap-2 text-2xl font-semibold">
							<Building2 className="h-6 w-6" />
							Business Units
						</h1>
						<p className="text-muted-foreground text-sm">
							Group teams and set a spend limit for this unit. Requests from a team in the unit count against this budget.
						</p>
					</div>
					<Button
						onClick={() => {
							setName("");
							setBudgetLimit(undefined);
							setBudgetReset("1M");
							setTokenLimit(undefined);
							setTokenReset("1h");
							setRequestLimit(undefined);
							setRequestReset("1h");
							setCreateTeamIds([]);
							setOpen(true);
						}}
						disabled={!hasCreateAccess}
					>
						<Plus className="h-4 w-4" />
						New unit
					</Button>
				</div>
				{listQueryFailed ? (
					<QueryErrorBanner
						testId="business-units-query-error"
						message={getErrorMessage(listQueryError) || "Failed to load business units, teams, or customers."}
					/>
				) : null}

				{/* Search Bar */}
				<div className="relative max-w-sm w-full">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						placeholder="Search business units by name..."
						value={search}
						onChange={(e) => setSearch(e.target.value)}
						className="pl-9 pr-9 bg-muted/20 border-border/60 focus:border-teal-500/50"
						data-testid="business-units-search-input"
					/>
					{search ? (
						<button
							type="button"
							onClick={() => setSearch("")}
							className="text-muted-foreground hover:text-foreground absolute top-1/2 right-3 -translate-y-1/2"
							aria-label="Clear search"
						>
							<X className="h-4 w-4" />
						</button>
					) : null}
				</div>

				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Budget</TableHead>
							<TableHead>Tokens</TableHead>
							<TableHead>Requests</TableHead>
							<TableHead>Teams</TableHead>
							<TableHead />
						</TableRow>
					</TableHeader>
					<TableBody>
						{filteredUnits.length === 0 ? (
							<TableRow>
								<TableCell colSpan={6} className="text-center py-10 text-muted-foreground text-sm">
									{search ? `No business units match "${search}"` : "No business units found."}
								</TableCell>
							</TableRow>
						) : (
							pagedUnits.map((unit) => (
								<TableRow
									key={unit.id}
									className={`cursor-pointer transition-colors hover:bg-muted/30 ${selected?.id === unit.id ? "bg-muted/50 font-medium" : ""}`}
									onClick={() => setSelected(unit)}
								>
									<TableCell className="font-medium">{unit.name}</TableCell>
									<TableCell>
										<LimitMeter used={unit.budget?.current_usage} max={unit.budget?.max_limit} reset={unit.budget?.reset_duration} money />
									</TableCell>
									<TableCell>
										<LimitMeter
											used={unit.rate_limit?.token_current_usage}
											max={unit.rate_limit?.token_max_limit}
											reset={unit.rate_limit?.token_reset_duration}
										/>
									</TableCell>
									<TableCell>
										<LimitMeter
											used={unit.rate_limit?.request_current_usage}
											max={unit.rate_limit?.request_max_limit}
											reset={unit.rate_limit?.request_reset_duration}
										/>
									</TableCell>
									<TableCell>
										{(unit.team_ids || []).length === 0 ? (
											<span className="text-muted-foreground text-sm">-</span>
										) : (unit.team_ids || []).length === 1 ? (
											<span className="text-sm">{teamLabel(unit.team_ids![0])}</span>
										) : (
											<span className="text-sm">{(unit.team_ids || []).length}</span>
										)}
									</TableCell>
									<TableCell className="text-right">
										<div className="flex items-center justify-end gap-1">
											<Button
												size="icon"
												variant="ghost"
												disabled={!hasUpdateAccess}
												onClick={(e) => {
													e.stopPropagation();
													openEdit(unit);
												}}
												className="h-8 w-8 text-muted-foreground hover:text-foreground"
												title="Edit business unit"
											>
												<Pencil className="h-4 w-4" />
											</Button>
											<Button
												size="icon"
												variant="ghost"
												disabled={!hasDeleteAccess}
												onClick={(e) => {
													e.stopPropagation();
													setUnitToDelete(unit);
												}}
												className="h-8 w-8 text-muted-foreground hover:text-red-400"
												title="Delete business unit"
											>
												<Trash2 className="h-4 w-4" />
											</Button>
										</div>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
				<DataTablePagination
					offset={offset}
					limit={limit}
					totalCount={filteredUnits.length}
					onOffsetChange={setOffset}
					onLimitChange={(newLimit) => {
						setLimit(newLimit);
						setOffset(0);
					}}
					itemLabel="business units"
					perPageLabel="Units per page"
					dataTestId="business-units-pagination"
				/>
			</div>

			<div className="rounded-xl border p-4">
				{!selected ? (
					<p className="text-muted-foreground text-sm">Select a business unit to see its teams.</p>
				) : (
					<div className="space-y-4">
						<div className="flex items-center justify-between">
							<div>
								<h2 className="text-lg font-semibold">{selected.name}</h2>
								<p className="text-muted-foreground text-xs">
									Used {moneyLabel(selected.budget?.current_usage, selected.budget?.max_limit)} ·{" "}
									{countLabel(selected.rate_limit?.token_current_usage, selected.rate_limit?.token_max_limit)} tokens ·{" "}
									{countLabel(selected.rate_limit?.request_current_usage, selected.rate_limit?.request_max_limit)} requests
								</p>
							</div>
							<Button
								size="sm"
								variant="outline"
								disabled={!hasUpdateAccess}
								onClick={() => openEdit(selected)}
								className="gap-1.5 h-8 text-xs"
							>
								<Pencil className="h-3.5 w-3.5" />
								Edit
							</Button>
						</div>
						<div className="max-h-72 space-y-2 overflow-y-auto pr-1">
							{assignedTeamsFailed ? (
								<QueryErrorBanner
									testId="business-unit-teams-query-error"
									message={getErrorMessage(assignedTeamsError) || "Failed to load assigned teams."}
								/>
							) : assigned.length === 0 ? (
								<p className="text-muted-foreground text-sm">No teams assigned yet. Add teams from New unit or Edit.</p>
							) : (
								assigned.map((team) => (
									<div key={team.id} className="rounded-lg border px-3 py-2 text-sm">
										{teamLabel(team.id, team.name)}
									</div>
								))
							)}
						</div>
					</div>
				)}
			</div>

			<Dialog open={open} onOpenChange={setOpen}>
				<DialogContent className="max-h-[90vh] overflow-y-auto">
					<DialogHeader>
						<DialogTitle>Create business unit</DialogTitle>
					</DialogHeader>
					<div className="space-y-3 py-2">
						<div className="space-y-1">
							<Label>Name</Label>
							<Input value={name} onChange={(e) => setName(e.target.value)} />
						</div>
						<NumberAndSelect
							id="bu-budget"
							label="Maximum Spend (USD)"
							value={budgetLimit}
							selectValue={budgetReset}
							onChangeNumber={setBudgetLimit}
							onChangeSelect={setBudgetReset}
							options={budgetDurationOptions}
							placeholder="No limit"
						/>
						<NumberAndSelect
							id="bu-tokens"
							label="Maximum Tokens"
							value={tokenLimit}
							selectValue={tokenReset}
							onChangeNumber={setTokenLimit}
							onChangeSelect={setTokenReset}
							options={resetDurationOptions}
							placeholder="No limit"
						/>
						<NumberAndSelect
							id="bu-requests"
							label="Maximum Requests"
							value={requestLimit}
							selectValue={requestReset}
							onChangeNumber={setRequestLimit}
							onChangeSelect={setRequestReset}
							options={resetDurationOptions}
							placeholder="No limit"
						/>
						<div className="space-y-1">
							<Label>Teams</Label>
							<div className="max-h-40 space-y-1 overflow-y-auto rounded-md border p-2">
								{teams.length === 0 ? (
									<p className="text-muted-foreground text-xs">No teams yet.</p>
								) : (
									teams.map((team) => (
										<label key={team.id} className="flex items-center gap-2 text-sm">
											<input
												type="checkbox"
												checked={createTeamIds.includes(team.id)}
												onChange={() => toggleTeam(createTeamIds, team.id, setCreateTeamIds)}
											/>
											<span>{teamLabel(team.id, team.name)}</span>
										</label>
									))
								)}
							</div>
						</div>
						<p className="text-muted-foreground text-xs">Leave a limit blank for no limit. Checked teams share this unit budget.</p>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setOpen(false)}>
							Cancel
						</Button>
						<Button onClick={() => void create()} disabled={!name.trim() || !hasCreateAccess}>
							Create
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<Dialog open={!!unitToEdit} onOpenChange={(isOpen) => !isOpen && setUnitToEdit(null)}>
				<DialogContent className="max-h-[90vh] overflow-y-auto">
					<DialogHeader>
						<DialogTitle>Edit business unit</DialogTitle>
					</DialogHeader>
					<div className="space-y-3 py-2">
						<div className="space-y-1">
							<Label>Name</Label>
							<Input
								value={editName}
								onChange={(e) => setEditName(e.target.value)}
								placeholder="Business unit name"
								autoFocus
								onKeyDown={(e) => {
									if (e.key === "Enter" && editName.trim()) {
										void handleUpdate();
									}
								}}
							/>
						</div>
						<NumberAndSelect
							id="bu-edit-budget"
							label="Maximum Spend (USD)"
							value={editBudgetLimit}
							selectValue={editBudgetReset}
							onChangeNumber={setEditBudgetLimit}
							onChangeSelect={setEditBudgetReset}
							options={budgetDurationOptions}
							placeholder="No limit"
						/>
						<NumberAndSelect
							id="bu-edit-tokens"
							label="Maximum Tokens"
							value={editTokenLimit}
							selectValue={editTokenReset}
							onChangeNumber={setEditTokenLimit}
							onChangeSelect={setEditTokenReset}
							options={resetDurationOptions}
							placeholder="No limit"
						/>
						<NumberAndSelect
							id="bu-edit-requests"
							label="Maximum Requests"
							value={editRequestLimit}
							selectValue={editRequestReset}
							onChangeNumber={setEditRequestLimit}
							onChangeSelect={setEditRequestReset}
							options={resetDurationOptions}
							placeholder="No limit"
						/>
						<div className="space-y-1">
							<Label>Teams</Label>
							<div className="max-h-40 space-y-1 overflow-y-auto rounded-md border p-2">
								{teams.length === 0 ? (
									<p className="text-muted-foreground text-xs">No teams yet.</p>
								) : (
									teams.map((team) => (
										<label key={team.id} className="flex items-center gap-2 text-sm">
											<input
												type="checkbox"
												checked={editTeamIds.includes(team.id)}
												onChange={() => toggleTeam(editTeamIds, team.id, setEditTeamIds)}
											/>
											<span>{teamLabel(team.id, team.name)}</span>
										</label>
									))
								)}
							</div>
						</div>
						<p className="text-muted-foreground text-xs">Clear a limit to remove it. Checked teams stay on this unit.</p>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setUnitToEdit(null)}>
							Cancel
						</Button>
						<Button onClick={() => void handleUpdate()} disabled={!editName.trim() || !hasUpdateAccess || isUpdating}>
							{isUpdating ? "Saving..." : "Save changes"}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<AlertDialog open={!!unitToDelete} onOpenChange={(isOpen) => !isOpen && setUnitToDelete(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Delete business unit?</AlertDialogTitle>
						<AlertDialogDescription>
							&quot;{unitToDelete?.name}&quot; will be deleted and its teams unassigned. The teams themselves are not deleted.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => {
								if (unitToDelete) void remove(unitToDelete.id);
								setUnitToDelete(null);
							}}
						>
							Delete
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
