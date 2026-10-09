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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage, useGetCustomersQuery, useGetTeamsQuery } from "@/lib/store";
import {
	useAssignBusinessUnitTeamMutation,
	useCreateBusinessUnitMutation,
	useDeleteBusinessUnitMutation,
	useGetBusinessUnitTeamsQuery,
	useGetBusinessUnitsQuery,
	useRemoveBusinessUnitTeamMutation,
} from "@enterprise/lib/store/apis/businessUnitsApi";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { BusinessUnit } from "@enterprise/lib/types/workspace";
import { Building2, Plus, Search, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { DataTablePagination } from "@/components/table/dataTablePagination";

export function BusinessUnitsView() {
	const [selected, setSelected] = useState<BusinessUnit | null>(null);
	const [search, setSearch] = useState("");
	const [name, setName] = useState("");
	const [open, setOpen] = useState(false);
	const [teamId, setTeamId] = useState("");
	const [unitToDelete, setUnitToDelete] = useState<BusinessUnit | null>(null);
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

	const assignedIds = useMemo(() => new Set(assigned.map((t) => t.id)), [assigned]);
	const teamById = useMemo(() => new Map(teams.map((t) => [t.id, t])), [teams]);
	const customerNameById = useMemo(() => new Map(customers.map((c) => [c.id, c.name])), [customers]);

	const create = async () => {
		try {
			await createUnit({ name }).unwrap();
			toast.success("Business unit created");
			setOpen(false);
			setName("");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
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

	const assign = async () => {
		if (!selected || !teamId) return;
		try {
			await assignTeam({ id: selected.id, team_id: teamId }).unwrap();
			toast.success("Team assigned");
			setTeamId("");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const unassign = async (id: string) => {
		if (!selected) return;
		try {
			await removeTeam({ id: selected.id, team_id: id }).unwrap();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
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
							Group teams for reporting and log filters. Budgets stay on Customer, Team, Virtual Key, User, and Model — not on the
							business unit itself.
						</p>
					</div>
					<Button onClick={() => setOpen(true)} disabled={!hasCreateAccess}>
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
							<TableHead>Teams</TableHead>
							<TableHead />
						</TableRow>
					</TableHeader>
					<TableBody>
						{filteredUnits.length === 0 ? (
							<TableRow>
								<TableCell colSpan={3} className="text-center py-10 text-muted-foreground text-sm">
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
									<TableCell>{unit.team_count}</TableCell>
									<TableCell className="text-right">
										<Button
											size="icon"
											variant="ghost"
											disabled={!hasDeleteAccess}
											onClick={(e) => {
												e.stopPropagation();
												setUnitToDelete(unit);
											}}
											className="h-8 w-8 text-muted-foreground hover:text-red-400"
										>
											<Trash2 className="h-4 w-4" />
										</Button>
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
					<p className="text-muted-foreground text-sm">Select a business unit to assign teams.</p>
				) : (
					<div className="space-y-4">
						<div>
							<h2 className="text-lg font-semibold">{selected.name}</h2>
							<p className="text-muted-foreground text-xs">
								Assign any team (with or without a customer). Related customers appear on the Customer detail view via shared teams.
							</p>
						</div>
						<div className="flex gap-2">
							<select value={teamId} onChange={(e) => setTeamId(e.target.value)} className="border-input bg-background h-9 flex-1 rounded-md border px-3 text-sm">
								<option value="">Select team</option>
								{teams
									.filter((team) => !assignedIds.has(team.id))
									.map((team) => (
										<option key={team.id} value={team.id}>
											{teamLabel(team.id, team.name)}
										</option>
									))}
							</select>
							<Button onClick={() => void assign()} disabled={!teamId || !hasUpdateAccess}>
								Assign
							</Button>
						</div>
						<div className="space-y-2">
							{assignedTeamsFailed ? (
								<QueryErrorBanner
									testId="business-unit-teams-query-error"
									message={getErrorMessage(assignedTeamsError) || "Failed to load assigned teams."}
								/>
							) : assigned.length === 0 ? (
								<p className="text-muted-foreground text-sm">No teams assigned yet.</p>
							) : (
								assigned.map((team) => (
									<div key={team.id} className="flex items-center justify-between rounded-lg border px-3 py-2 text-sm">
										<span>{teamLabel(team.id, team.name)}</span>
										<Button size="icon" variant="ghost" disabled={!hasUpdateAccess} onClick={() => void unassign(team.id)}>
											<Trash2 className="h-4 w-4" />
										</Button>
									</div>
								))
							)}
						</div>
					</div>
				)}
			</div>

			<Dialog open={open} onOpenChange={setOpen}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Create business unit</DialogTitle>
					</DialogHeader>
					<div className="space-y-1 py-2">
						<Label>Name</Label>
						<Input value={name} onChange={(e) => setName(e.target.value)} />
						<p className="text-muted-foreground text-xs">After create, assign teams. Spend limits remain on Customer / Team / VK / User.</p>
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
