import { DataTablePagination } from "@/components/table/dataTablePagination";
import { PIN_SHADOW_RIGHT } from "@/components/table/columnPinning";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Progress } from "@/components/ui/progress";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { resetDurationLabels } from "@/lib/constants/governance";
import { getErrorMessage, useDeleteCustomerMutation } from "@/lib/store";
import { Customer, Team, VirtualKey } from "@/lib/types/governance";
import { cn } from "@/lib/utils";
import { describeCustomerVirtualKey, formatCurrency, virtualKeysForCustomer } from "@/lib/utils/governance";
import { CustomerDetailSheet } from "@enterprise/components/user-groups/sheets/customerDetailSheet";
import { useGetBusinessUnitsQuery } from "@enterprise/lib/store/apis/businessUnitsApi";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { Link } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, Edit, MoreHorizontal, Plus, ScrollText, Search, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import { Checkbox } from "@/components/ui/checkbox";
import { CustomersEmptyState } from "./customersEmptyState";
import CustomerSheet from "./customerSheet";

// Helper to format reset duration for display
const formatResetDuration = (duration: string) => {
	return resetDurationLabels[duration] || duration;
};

const ACTIONS_COLUMN_CLASS = `sticky right-0 z-10 w-[56px] min-w-[56px] text-right ${PIN_SHADOW_RIGHT}`;

function CustomerBusinessUnits({ units }: { units: { id: string; name: string }[] }) {
	const [open, setOpen] = useState(false);
	if (units.length === 0) return <span className="text-muted-foreground text-sm">-</span>;
	if (units.length === 1) return <span className="text-sm">{units[0].name}</span>;
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					className="text-sm font-medium underline-offset-2 hover:underline"
					onClick={(e) => e.stopPropagation()}
				>
					{units.length}
				</button>
			</PopoverTrigger>
			<PopoverContent className="w-56 p-2" align="start" onClick={(e) => e.stopPropagation()}>
				<ul className="space-y-1 text-sm">
					{units.map((unit) => (
						<li key={unit.id}>{unit.name}</li>
					))}
				</ul>
			</PopoverContent>
		</Popover>
	);
}

interface CustomerActionsMenuProps {
	customer: Customer;
	canUpdate: boolean;
	canDelete: boolean;
	onEdit: (customer: Customer) => void;
	onDelete: (customer: Customer) => void;
}

function CustomerActionsMenu({ customer, canUpdate, canDelete, onEdit, onDelete }: CustomerActionsMenuProps) {
	const [isOpen, setIsOpen] = useState(false);

	return (
		<DropdownMenu open={isOpen} onOpenChange={setIsOpen}>
			<DropdownMenuTrigger asChild>
				<Button
					variant="ghost"
					size="icon"
					className="h-8 w-8"
					aria-label={`Customer actions ${customer.name}`}
					data-testid={`customer-actions-btn-${customer.id}`}
					onClick={(e) => e.stopPropagation()}
					onPointerDown={(e) => e.stopPropagation()}
				>
					<MoreHorizontal className="h-4 w-4" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem
					disabled={!canUpdate}
					title={!canUpdate ? "View-only access: editing is disabled" : undefined}
					data-testid={`customer-button-edit-${customer.id}`}
					onSelect={(e) => {
						e.stopPropagation();
						e.preventDefault();
						onEdit(customer);
						setIsOpen(false);
					}}
					onClick={(e) => e.stopPropagation()}
					onPointerDown={(e) => e.stopPropagation()}
				>
					<Edit className="h-4 w-4" />
					Edit
				</DropdownMenuItem>
				<DropdownMenuItem asChild className="cursor-pointer" data-testid={`customer-button-view-logs-${customer.id}`}>
					<Link
						to="/workspace/logs"
						search={{ customer_ids: [customer.id] }}
						onClick={(e) => {
							e.stopPropagation();
							setIsOpen(false);
						}}
						onPointerDown={(e) => e.stopPropagation()}
					>
						<ScrollText className="h-4 w-4" />
						View logs
					</Link>
				</DropdownMenuItem>
				<DropdownMenuItem
					variant="destructive"
					disabled={!canDelete}
					title={!canDelete ? "Delete permission is required" : undefined}
					data-testid={`customer-button-delete-${customer.id}`}
					onSelect={(e) => {
						e.preventDefault();
						onDelete(customer);
						setIsOpen(false);
					}}
					onClick={(e) => e.stopPropagation()}
					onPointerDown={(e) => e.stopPropagation()}
				>
					<Trash2 className="h-4 w-4" />
					Delete
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
}

interface CustomersTableProps {
	customers: Customer[];
	totalCount: number;
	teams: Team[];
	virtualKeys: VirtualKey[];
	search: string;
	debouncedSearch: string;
	onSearchChange: (value: string) => void;
	offset: number;
	limit: number;
	onOffsetChange: (offset: number) => void;
	onLimitChange?: (limit: number) => void;
	isFetching?: boolean;
}

export default function CustomersTable({
	customers,
	totalCount,
	teams,
	virtualKeys,
	search,
	debouncedSearch,
	onSearchChange,
	offset,
	limit,
	onOffsetChange,
	onLimitChange,
	isFetching,
}: CustomersTableProps) {
	const [showCustomerSheet, setShowCustomerSheet] = useState(false);
	const [editingCustomer, setEditingCustomer] = useState<Customer | null>(null);
	const [confirmDeleteCustomer, setConfirmDeleteCustomer] = useState<Customer | null>(null);
	const [viewingCustomer, setViewingCustomer] = useState<Customer | null>(null);

	const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
	const [showBulkDeleteDialog, setShowBulkDeleteDialog] = useState(false);
	const [isBulkDeleting, setIsBulkDeleting] = useState(false);

	const visibleIds = useMemo(() => customers.map((c) => c.id), [customers]);
	const selectedVisibleIds = useMemo(() => visibleIds.filter((id) => selectedIds.has(id)), [selectedIds, visibleIds]);
	const selectedCount = selectedIds.size;
	const allVisibleSelected = visibleIds.length > 0 && selectedVisibleIds.length === visibleIds.length;
	const someVisibleSelected = selectedVisibleIds.length > 0 && selectedVisibleIds.length < visibleIds.length;

	const toggleSelectAllVisible = (checked: boolean) => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			for (const id of visibleIds) {
				if (checked) {
					next.add(id);
				} else {
					next.delete(id);
				}
			}
			return next;
		});
	};

	const toggleSelectCustomer = (customerId: string, checked: boolean) => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			if (checked) {
				next.add(customerId);
			} else {
				next.delete(customerId);
			}
			return next;
		});
	};

	const hasCreateAccess = useRbac(RbacResource.Customers, RbacOperation.Create);
	const hasUpdateAccess = useRbac(RbacResource.Customers, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.Customers, RbacOperation.Delete);

	const [deleteCustomer, { isLoading: isDeleting }] = useDeleteCustomerMutation();
	const { data: businessUnitData } = useGetBusinessUnitsQuery();
	const businessUnitsByCustomer = useMemo(() => {
		const grouped = new Map<string, { id: string; name: string }[]>();
		for (const unit of businessUnitData?.business_units || []) {
			if (!unit.customer_id) continue;
			const list = grouped.get(unit.customer_id) || [];
			list.push({ id: unit.id, name: unit.name });
			grouped.set(unit.customer_id, list);
		}
		return grouped;
	}, [businessUnitData]);
	const teamsByCustomer = useMemo(() => {
		const teamById = new Map(teams.map((team) => [team.id, team]));
		const grouped = new Map<string, Team[]>();
		const add = (customerId: string, team: Team) => {
			const list = grouped.get(customerId) || [];
			if (!list.some((item) => item.id === team.id)) list.push(team);
			grouped.set(customerId, list);
		};
		for (const team of teams) {
			if (team.customer_id) add(team.customer_id, team);
		}
		for (const unit of businessUnitData?.business_units || []) {
			if (!unit.customer_id) continue;
			for (const teamId of unit.team_ids || []) {
				const team = teamById.get(teamId);
				if (team) add(unit.customer_id, team);
			}
		}
		return grouped;
	}, [teams, businessUnitData]);

	const handleBulkDelete = async () => {
		if (selectedCount === 0) return;
		setIsBulkDeleting(true);
		let successCount = 0;
		let failCount = 0;
		const ids = Array.from(selectedIds);
		for (const id of ids) {
			try {
				await deleteCustomer(id).unwrap();
				successCount++;
			} catch {
				failCount++;
			}
		}
		setIsBulkDeleting(false);
		setShowBulkDeleteDialog(false);
		setSelectedIds(new Set());
		if (successCount > 0) {
			toast.success(`Successfully deleted ${successCount} customer${successCount > 1 ? "s" : ""}`);
		}
		if (failCount > 0) {
			toast.error(`Failed to delete ${failCount} customer${failCount > 1 ? "s" : ""}`);
		}
	};

	const handleDelete = async (customerId: string) => {
		try {
			await deleteCustomer(customerId).unwrap();
			setSelectedIds((prev) => {
				const next = new Set(prev);
				next.delete(customerId);
				return next;
			});
			toast.success("Customer deleted successfully");
		} catch (error) {
			toast.error(getErrorMessage(error));
		} finally {
			setConfirmDeleteCustomer(null);
		}
	};

	const handleAddCustomer = () => {
		setEditingCustomer(null);
		setShowCustomerSheet(true);
	};

	const handleEditCustomer = (customer: Customer) => {
		setEditingCustomer(customer);
		setShowCustomerSheet(true);
	};

	const handleCustomerSaved = () => {
		setShowCustomerSheet(false);
		setEditingCustomer(null);
	};

	const getTeamsForCustomer = (customerId: string) => teamsByCustomer.get(customerId) || [];

	const getVirtualKeysForCustomer = (customerId: string) =>
		virtualKeysForCustomer(virtualKeys, customerId, getTeamsForCustomer(customerId));

	const hasActiveFilters = debouncedSearch;

	// True empty state: no customers at all (not just filtered to zero)
	if (totalCount === 0 && !hasActiveFilters) {
		return (
			<>
				<TooltipProvider>
					<CustomerSheet
						open={showCustomerSheet}
						onOpenChange={(open) => {
							setShowCustomerSheet(open);
							if (!open) setEditingCustomer(null);
						}}
						customer={editingCustomer}
						onSuccess={handleCustomerSaved}
					/>
					<CustomersEmptyState onAddClick={handleAddCustomer} canCreate={hasCreateAccess} />
				</TooltipProvider>
			</>
		);
	}

	return (
		<>
			<TooltipProvider>
				<CustomerSheet
					open={showCustomerSheet}
					onOpenChange={(open) => {
						setShowCustomerSheet(open);
						if (!open) setEditingCustomer(null);
					}}
					customer={editingCustomer}
					onSuccess={handleCustomerSaved}
				/>

				<CustomerDetailSheet
					open={!!viewingCustomer}
					onOpenChange={(open) => {
						if (!open) setViewingCustomer(null);
					}}
					customer={viewingCustomer}
				/>

				<AlertDialog open={showBulkDeleteDialog} onOpenChange={setShowBulkDeleteDialog}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>Delete Selected Customers</AlertDialogTitle>
							<AlertDialogDescription>
								Are you sure you want to delete {selectedCount} selected customer{selectedCount > 1 ? "s" : ""}? This will also unassign any teams and virtual keys from these customers. This action cannot be undone.
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel disabled={isBulkDeleting}>Cancel</AlertDialogCancel>
							<AlertDialogAction
								onClick={handleBulkDelete}
								disabled={isBulkDeleting || selectedCount === 0}
								className="bg-red-600 hover:bg-red-700"
								data-testid="customers-bulk-delete-confirm-btn"
							>
								{isBulkDeleting ? "Deleting..." : "Delete Selected"}
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>

				<div className="flex grow flex-col">
					<div className="mb-4 flex items-center justify-between">
						<div>
							<div className="flex items-center gap-2">
								<h2 className="text-lg font-semibold">Customers</h2>
								{!hasUpdateAccess && (
									<Badge variant="outline" className="text-amber-500 border-amber-500/30 bg-amber-500/10 text-xs">
										View Only
									</Badge>
								)}
							</div>
							<p className="text-muted-foreground text-sm">
								Manage customer accounts with teams, budgets, virtual keys, and related business units (via shared teams).
							</p>
						</div>
						<div className="flex items-center gap-2">
							{selectedCount > 0 && (
								<Button
									variant="destructive"
									data-testid="customers-bulk-delete-btn"
									onClick={() => setShowBulkDeleteDialog(true)}
									disabled={!hasDeleteAccess || isBulkDeleting}
								>
									<Trash2 className="h-4 w-4 mr-1" />
									Delete Selected ({selectedCount})
								</Button>
							)}
							<Button data-testid="customer-button-create" onClick={handleAddCustomer} disabled={!hasCreateAccess}>
								<Plus className="h-4 w-4" />
								Add Customer
							</Button>
						</div>
					</div>

					<div className="mb-4 flex items-center gap-3">
						<div className="relative max-w-sm flex-1">
							<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
							<Input
								aria-label="Search customers by name"
								placeholder="Search by name..."
								value={search}
								onChange={(e) => onSearchChange(e.target.value)}
								className="pl-9"
								data-testid="customers-search-input"
							/>
						</div>
					</div>

					<div className="mb-2 grow overflow-auto rounded-sm border" data-testid="customer-table-container">
						<Table className="min-w-[1100px]">
							<TableHeader>
								<TableRow>
									<TableHead className="w-[48px]">
										<Checkbox
											checked={allVisibleSelected || (someVisibleSelected ? "indeterminate" : false)}
											onCheckedChange={(checked) => toggleSelectAllVisible(checked === true)}
											aria-label="Select all customers on this page"
											data-testid="customers-select-all-checkbox"
										/>
									</TableHead>
									<TableHead>Name</TableHead>
									<TableHead>Business Units</TableHead>
									<TableHead>Teams</TableHead>
									<TableHead>Budget</TableHead>
									<TableHead>Rate Limit</TableHead>
									<TableHead>Virtual Keys</TableHead>
									<TableHead className={`bg-muted ${ACTIONS_COLUMN_CLASS}`}></TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{customers.length === 0 ? (
									<TableRow>
										<TableCell colSpan={8} className="h-24 text-center">
											<span className="text-muted-foreground text-sm">No matching customers found.</span>
										</TableCell>
									</TableRow>
								) : (
									customers.map((customer) => {
										const customerTeams = getTeamsForCustomer(customer.id);
										const vks = getVirtualKeysForCustomer(customer.id);

										// Budget calculations (most-exhausted budget drives the row highlight)
										const budgets = customer.budgets ?? [];
										const isBudgetExhausted = budgets.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) >= b.max_limit);

										// Rate limit calculations
										const isTokenLimitExhausted =
											customer.rate_limit?.token_max_limit &&
											customer.rate_limit.token_max_limit > 0 &&
											(customer.rate_limit.token_current_usage ?? 0) >= customer.rate_limit.token_max_limit;
										const isRequestLimitExhausted =
											customer.rate_limit?.request_max_limit &&
											customer.rate_limit.request_max_limit > 0 &&
											(customer.rate_limit.request_current_usage ?? 0) >= customer.rate_limit.request_max_limit;
										const isRateLimitExhausted = isTokenLimitExhausted || isRequestLimitExhausted;
										const tokenPercentage =
											customer.rate_limit?.token_max_limit && customer.rate_limit.token_max_limit > 0
												? Math.min(((customer.rate_limit.token_current_usage ?? 0) / customer.rate_limit.token_max_limit) * 100, 100)
												: 0;
										const requestPercentage =
											customer.rate_limit?.request_max_limit && customer.rate_limit.request_max_limit > 0
												? Math.min(((customer.rate_limit.request_current_usage ?? 0) / customer.rate_limit.request_max_limit) * 100, 100)
												: 0;

										const isExhausted = isBudgetExhausted || isRateLimitExhausted;

										return (
											<TableRow
												key={customer.id}
												data-testid={`customer-row-${customer.name}`}
												className={cn(
													"group cursor-pointer transition-colors",
													isExhausted ? "bg-red-500/10 border-l-4 border-l-red-500 hover:bg-red-500/15 dark:bg-red-950/20" : "hover:bg-muted/50",
												)}
												role="button"
												tabIndex={0}
												onClick={() => setViewingCustomer(customer)}
												onKeyDown={(e) => {
													if (e.target !== e.currentTarget) return;
													if (e.key === "Enter" || e.key === " ") {
														e.preventDefault();
														setViewingCustomer(customer);
													}
												}}
											>
												<TableCell className="w-[48px]" onClick={(e) => e.stopPropagation()}>
													<Checkbox
														checked={selectedIds.has(customer.id)}
														onCheckedChange={(checked) => toggleSelectCustomer(customer.id, checked === true)}
														aria-label={`Select ${customer.name}`}
														data-testid={`customer-select-checkbox-${customer.id}`}
														onClick={(e) => e.stopPropagation()}
													/>
												</TableCell>
												<TableCell className="max-w-[200px] py-4">
													<div className="flex flex-col gap-2">
														<span className="truncate font-medium">{customer.name}</span>
														{isBudgetExhausted ? (
															<Badge variant="destructive" className="w-fit text-xs bg-red-600 hover:bg-red-700 text-white font-medium">
																Budget Exhausted
															</Badge>
														) : isExhausted ? (
															<Badge variant="destructive" className="w-fit text-xs">
																Rate Limit Reached
															</Badge>
														) : null}
													</div>
												</TableCell>
												<TableCell onClick={(e) => e.stopPropagation()}>
													<CustomerBusinessUnits units={businessUnitsByCustomer.get(customer.id) || []} />
												</TableCell>
												<TableCell>
													{customerTeams?.length > 0 ? (
														<div className="flex items-center gap-2">
															<Tooltip>
																<TooltipTrigger>
																	<Badge variant="outline" className="text-xs">
																		{customerTeams.length} {customerTeams.length === 1 ? "team" : "teams"}
																	</Badge>
																</TooltipTrigger>
																<TooltipContent>{customerTeams.map((team) => team.name).join(", ")}</TooltipContent>
															</Tooltip>
														</div>
													) : (
														<span className="text-muted-foreground text-sm">-</span>
													)}
												</TableCell>
												<TableCell className="min-w-[180px]">
													{budgets.length > 0 ? (
														<div className="space-y-2">
															{budgets.map((budget) => {
																const usage = budget.current_usage ?? 0;
																const pct = budget.max_limit > 0 ? Math.min((usage / budget.max_limit) * 100, 100) : 0;
																const exhausted = budget.max_limit > 0 && usage >= budget.max_limit;
																return (
																	<Tooltip key={budget.id}>
																		<TooltipTrigger asChild>
																			<div className="space-y-1">
																				<div className="flex items-center justify-between gap-4">
																					<span className="text-sm font-medium">{formatCurrency(budget.max_limit)}</span>
																					<span className="text-muted-foreground text-xs">
																						{formatResetDuration(budget.reset_duration)}
																					</span>
																				</div>
																				<Progress
																					value={pct}
																					className={cn(
																						"bg-muted/70 dark:bg-muted/30 h-1.5",
																						exhausted
																							? "[&>div]:bg-red-500/70"
																							: pct > 80
																								? "[&>div]:bg-amber-500/70"
																								: "[&>div]:bg-emerald-500/70",
																					)}
																				/>
																			</div>
																		</TooltipTrigger>
																		<TooltipContent>
																			<p className="font-medium">
																				{formatCurrency(budget.current_usage)} / {formatCurrency(budget.max_limit)}
																			</p>
																			<p className="text-primary-foreground/80 text-xs">
																				Resets {formatResetDuration(budget.reset_duration)}
																			</p>
																		</TooltipContent>
																	</Tooltip>
																);
															})}
														</div>
													) : (
														<span className="text-muted-foreground text-sm">-</span>
													)}
												</TableCell>
												<TableCell className="min-w-[180px]">
													{customer.rate_limit ? (
														<div className="space-y-2.5">
															{customer.rate_limit.token_max_limit && (
																<Tooltip>
																	<TooltipTrigger asChild>
																		<div className="space-y-1.5">
																			<div className="flex items-center justify-between gap-4 text-xs">
																				<span className="font-medium">{customer.rate_limit.token_max_limit.toLocaleString()} tokens</span>
																				<span className="text-muted-foreground">
																					{formatResetDuration(customer.rate_limit.token_reset_duration || "1h")}
																				</span>
																			</div>
																			<Progress
																				value={tokenPercentage}
																				className={cn(
																					"bg-muted/70 dark:bg-muted/30 h-1",
																					isTokenLimitExhausted
																						? "[&>div]:bg-red-500/70"
																						: tokenPercentage > 80
																							? "[&>div]:bg-amber-500/70"
																							: "[&>div]:bg-emerald-500/70",
																				)}
																			/>
																		</div>
																	</TooltipTrigger>
																	<TooltipContent>
																		<p className="font-medium">
																			{(customer.rate_limit.token_current_usage ?? 0).toLocaleString()} /{" "}
																			{(customer.rate_limit.token_max_limit ?? 0).toLocaleString()} tokens
																		</p>
																		<p className="text-primary-foreground/80 text-xs">
																			Resets {formatResetDuration(customer.rate_limit.token_reset_duration || "1h")}
																		</p>
																	</TooltipContent>
																</Tooltip>
															)}
															{customer.rate_limit.request_max_limit && (
																<Tooltip>
																	<TooltipTrigger asChild>
																		<div className="space-y-1.5">
																			<div className="flex items-center justify-between gap-4 text-xs">
																				<span className="font-medium">{customer.rate_limit.request_max_limit.toLocaleString()} req</span>
																				<span className="text-muted-foreground">
																					{formatResetDuration(customer.rate_limit.request_reset_duration || "1h")}
																				</span>
																			</div>
																			<Progress
																				value={requestPercentage}
																				className={cn(
																					"bg-muted/70 dark:bg-muted/30 h-1",
																					isRequestLimitExhausted
																						? "[&>div]:bg-red-500/70"
																						: requestPercentage > 80
																							? "[&>div]:bg-amber-500/70"
																							: "[&>div]:bg-emerald-500/70",
																				)}
																			/>
																		</div>
																	</TooltipTrigger>
																	<TooltipContent>
																		<p className="font-medium">
																			{(customer.rate_limit.request_current_usage ?? 0).toLocaleString()} /{" "}
																			{(customer.rate_limit.request_max_limit ?? 0).toLocaleString()} requests
																		</p>
																		<p className="text-primary-foreground/80 text-xs">
																			Resets {formatResetDuration(customer.rate_limit.request_reset_duration || "1h")}
																		</p>
																	</TooltipContent>
																</Tooltip>
															)}
														</div>
													) : (
														<span className="text-muted-foreground text-sm">-</span>
													)}
												</TableCell>
												<TableCell>
													{vks?.length > 0 ? (
														<div className="flex items-center gap-2">
															<Tooltip>
																<TooltipTrigger>
																	<Badge variant="outline" className="text-xs">
																		{vks.length} {vks.length === 1 ? "key" : "keys"}
																	</Badge>
																</TooltipTrigger>
																<TooltipContent>{vks.map(describeCustomerVirtualKey).join(", ")}</TooltipContent>
															</Tooltip>
														</div>
													) : (
														<span className="text-muted-foreground text-sm">-</span>
													)}
												</TableCell>
												<TableCell
													className={cn(
														"dark:bg-card dark:group-hover:bg-muted",
														isExhausted ? "bg-red-500/5 group-hover:bg-red-500/10" : "bg-white group-hover:bg-muted",
														ACTIONS_COLUMN_CLASS,
													)}
												>
													<CustomerActionsMenu
														customer={customer}
														canUpdate={hasUpdateAccess}
														canDelete={hasDeleteAccess}
														onEdit={handleEditCustomer}
														onDelete={setConfirmDeleteCustomer}
													/>
												</TableCell>
											</TableRow>
										);
									})
								)}
							</TableBody>
						</Table>
					</div>

					{/* Standard DataTable Pagination Footer */}
					<DataTablePagination
						offset={offset}
						limit={limit}
						totalCount={totalCount}
						onOffsetChange={onOffsetChange}
						onLimitChange={onLimitChange}
						itemLabel="customers"
						perPageLabel="Customers per page"
						dataTestId="customers-pagination"
					/>
				</div>

				<AlertDialog open={!!confirmDeleteCustomer} onOpenChange={(open) => !open && setConfirmDeleteCustomer(null)}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>Delete Customer</AlertDialogTitle>
							<AlertDialogDescription>
								Are you sure you want to delete &quot;{confirmDeleteCustomer?.name}&quot;? Its budgets and rate limit are deleted; its teams
								and virtual keys are kept but unassigned from this customer. This action cannot be undone.
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel data-testid="customer-button-delete-cancel">Cancel</AlertDialogCancel>
							<AlertDialogAction
								data-testid="customer-button-delete-confirm"
								onClick={() => confirmDeleteCustomer && handleDelete(confirmDeleteCustomer.id)}
								disabled={isDeleting}
								className="bg-red-600 hover:bg-red-700"
							>
								{isDeleting ? "Deleting..." : "Delete"}
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			</TooltipProvider>
		</>
	);
}