import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scrollArea";
import { Skeleton } from "@/components/ui/skeleton";
import { TruncatedLabel } from "@/components/ui/truncatedLabel";
import { useGetCustomersQuery, useGetSessionUsersQuery, useGetTeamsQuery } from "@/lib/store";
import { cn } from "@/lib/utils";
import {
	Building2,
	ChevronDown,
	PanelLeftClose,
	PanelLeftOpen,
	RotateCcw,
	Search,
	UserCheck,
	Users,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePromptContext } from "../context";

const COLLAPSE_STORAGE_KEY = "prompt-filter-sidebar-collapsed";

export function PromptFilterSidebar() {
	const { promptFilters, setPromptFilters, prompts, folders } = usePromptContext();
	const [collapsed, setCollapsed] = useState(false);

	// Load persisted collapsed state on mount
	useEffect(() => {
		if (typeof window === "undefined") return;
		const stored = window.localStorage.getItem(COLLAPSE_STORAGE_KEY);
		if (stored === "true") setCollapsed(true);
	}, []);

	const toggleCollapsed = useCallback(() => {
		setCollapsed((prev) => {
			const next = !prev;
			if (typeof window !== "undefined") {
				window.localStorage.setItem(COLLAPSE_STORAGE_KEY, String(next));
			}
			return next;
		});
	}, []);

	const activeFilterCount = useMemo(() => {
		return (
			(promptFilters.customer_ids?.length || 0) +
			(promptFilters.team_ids?.length || 0) +
			(promptFilters.user_ids?.length || 0)
		);
	}, [promptFilters]);

	const handleReset = useCallback(() => {
		setPromptFilters({
			customer_ids: [],
			team_ids: [],
			user_ids: [],
		});
	}, [setPromptFilters]);

	// Fetch governance metadata
	const { data: customersData, isLoading: isCustomersLoading } = useGetCustomersQuery();
	const { data: teamsData, isLoading: isTeamsLoading } = useGetTeamsQuery();
	const { data: sessionUsersData, isLoading: isUsersLoading } = useGetSessionUsersQuery();

	const customers = customersData?.customers || [];
	const teams = teamsData?.teams || [];
	const users = (sessionUsersData || []).filter((u) => (u.status || "approved") === "approved");

	// Pre-build folder ancestry map for quick lookup
	const folderMap = useMemo(() => {
		const map = new Map<string, any>();
		for (const f of folders) map.set(f.id, f);
		return map;
	}, [folders]);

	// Precompute prompt counts for customers, teams, and users
	const promptCounts = useMemo(() => {
		const custCount = new Map<string, number>();
		const teamCount = new Map<string, number>();
		const userCount = new Map<string, number>();

		// Team -> Customer map
		const teamCustomerMap = new Map<string, string>();
		for (const t of teams) {
			if (t.customer_id) teamCustomerMap.set(t.id, t.customer_id);
		}

		for (const p of prompts) {
			const seenC = new Set<string>();
			const seenT = new Set<string>();
			const seenU = new Set<string>();

			// 1. Direct field scoping
			if (p.customer_ids) {
				for (const cid of p.customer_ids.split(",")) {
					const trimmed = cid.trim();
					if (trimmed) seenC.add(trimmed);
				}
			}
			if (p.team_ids) {
				for (const tid of p.team_ids.split(",")) {
					const trimmed = tid.trim();
					if (trimmed) {
						seenT.add(trimmed);
						const cid = teamCustomerMap.get(trimmed);
						if (cid) seenC.add(cid);
					}
				}
			}
			if (p.user_ids) {
				for (const uid of p.user_ids.split(",")) {
					const trimmed = uid.trim();
					if (trimmed) seenU.add(trimmed);
				}
			}
			if (p.owner_user_id) {
				seenU.add(p.owner_user_id);
			}

			// 2. Folder tree inheritance
			let curr = p.folder_id ? folderMap.get(p.folder_id) : undefined;
			while (curr) {
				if (curr.entity_id) {
					seenC.add(curr.entity_id);
					seenT.add(curr.entity_id);
					seenU.add(curr.entity_id);
				}
				curr = curr.parent_id ? folderMap.get(curr.parent_id) : undefined;
			}

			// 3. User allowed repos
			for (const u of users) {
				if (u.allowed_prompt_repos) {
					for (const repoId of u.allowed_prompt_repos.split(",")) {
						if (repoId.trim() === p.id) {
							seenU.add(u.id);
						}
					}
				}
			}

			for (const c of seenC) custCount.set(c, (custCount.get(c) || 0) + 1);
			for (const t of seenT) teamCount.set(t, (teamCount.get(t) || 0) + 1);
			for (const u of seenU) userCount.set(u, (userCount.get(u) || 0) + 1);
		}

		return { custCount, teamCount, userCount };
	}, [prompts, folders, folderMap, teams, users]);

	// Collapsed: sleek rail with vertical "Filters" label
	if (collapsed) {
		return (
			<button
				type="button"
				onClick={toggleCollapsed}
				className="bg-card group flex h-full w-10 shrink-0 cursor-pointer flex-col items-center gap-3 rounded-md mr-1 py-4 text-sm font-medium transition-colors hover:bg-accent/40"
				title="Show prompt filters"
				aria-label="Show prompt filters"
				data-testid="prompt-filter-sidebar-collapsed"
			>
				<PanelLeftOpen className="text-muted-foreground group-hover:text-foreground size-4 transition-colors" />
				<span className="rotate-180 select-none [writing-mode:vertical-rl] text-xs font-semibold tracking-wide">
					Filters
				</span>
				{activeFilterCount > 0 && (
					<span className="bg-primary/10 text-primary flex size-5 items-center justify-center rounded-full text-[11px] font-bold">
						{activeFilterCount}
					</span>
				)}
			</button>
		);
	}

	return (
		<div
			className="bg-card flex h-full w-60 shrink-0 flex-col rounded-md mr-1 overflow-hidden"
			data-testid="prompt-filter-sidebar"
		>
			{/* Header */}
			<div className="flex h-11 items-center justify-between border-b px-3">
				<div className="flex items-center gap-1.5">
					<span className="text-xs font-semibold">Filters</span>
					{activeFilterCount > 0 && (
						<Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
							{activeFilterCount}
						</Badge>
					)}
				</div>
				<div className="flex items-center gap-1">
					{activeFilterCount > 0 && (
						<Button
							variant="ghost"
							size="sm"
							className="text-muted-foreground hover:text-foreground h-6 px-1.5 text-[11px]"
							onClick={handleReset}
							data-testid="prompt-filter-reset"
						>
							<RotateCcw className="size-3 mr-1" />
							Reset
						</Button>
					)}
					<Button
						variant="ghost"
						size="icon"
						className="size-6 text-muted-foreground hover:text-foreground"
						onClick={toggleCollapsed}
						title="Hide filters"
						aria-label="Hide filters"
						data-testid="prompt-filter-collapse"
					>
						<PanelLeftClose className="size-3.5" />
					</Button>
				</div>
			</div>

			{/* Filter Sections */}
			<ScrollArea className="flex flex-1 overflow-y-auto p-2" viewportClassName="no-table">
				<div className="flex grow flex-col gap-1">
					{/* Customer Filter */}
					<FilterSection
						title="Customer"
						icon={<Building2 className="size-3.5 text-muted-foreground" />}
						defaultOpen={promptFilters.customer_ids.length > 0 || true}
						loading={isCustomersLoading}
						count={promptFilters.customer_ids.length}
					>
						<SearchableCheckboxList
							placeholder="Search customers..."
							items={customers.map((c) => ({
								key: c.id,
								label: c.name,
								count: promptCounts.custCount.get(c.id) || 0,
							}))}
							isSelected={(id) => promptFilters.customer_ids.includes(id)}
							onToggle={(id) => {
								const current = promptFilters.customer_ids;
								const next = current.includes(id) ? current.filter((v) => v !== id) : [...current, id];
								setPromptFilters((prev) => ({ ...prev, customer_ids: next }));
							}}
						/>
					</FilterSection>

					{/* Team Filter */}
					<FilterSection
						title="Team"
						icon={<Users className="size-3.5 text-muted-foreground" />}
						defaultOpen={promptFilters.team_ids.length > 0 || true}
						loading={isTeamsLoading}
						count={promptFilters.team_ids.length}
					>
						<SearchableCheckboxList
							placeholder="Search teams..."
							items={teams.map((t) => ({
								key: t.id,
								label: t.name,
								count: promptCounts.teamCount.get(t.id) || 0,
							}))}
							isSelected={(id) => promptFilters.team_ids.includes(id)}
							onToggle={(id) => {
								const current = promptFilters.team_ids;
								const next = current.includes(id) ? current.filter((v) => v !== id) : [...current, id];
								setPromptFilters((prev) => ({ ...prev, team_ids: next }));
							}}
						/>
					</FilterSection>

					{/* User Filter */}
					<FilterSection
						title="User"
						icon={<UserCheck className="size-3.5 text-muted-foreground" />}
						defaultOpen={promptFilters.user_ids.length > 0 || false}
						loading={isUsersLoading}
						count={promptFilters.user_ids.length}
					>
						<SearchableCheckboxList
							placeholder="Search users..."
							items={users.map((u) => ({
								key: u.id,
								label: u.username || u.email || u.id,
								count: promptCounts.userCount.get(u.id) || 0,
							}))}
							isSelected={(id) => promptFilters.user_ids.includes(id)}
							onToggle={(id) => {
								const current = promptFilters.user_ids;
								const next = current.includes(id) ? current.filter((v) => v !== id) : [...current, id];
								setPromptFilters((prev) => ({ ...prev, user_ids: next }));
							}}
						/>
					</FilterSection>
				</div>
			</ScrollArea>
		</div>
	);
}

// ---------------------------------------------------------------------------
// Collapsible FilterSection
// ---------------------------------------------------------------------------

function FilterSection({
	title,
	icon,
	children,
	defaultOpen = false,
	loading = false,
	count = 0,
}: {
	title: string;
	icon?: React.ReactNode;
	children: React.ReactNode;
	defaultOpen?: boolean;
	loading?: boolean;
	count?: number;
}) {
	const [open, setOpen] = useState(defaultOpen);

	useEffect(() => {
		if (count > 0) setOpen(true);
	}, [count]);

	return (
		<Collapsible open={open} onOpenChange={setOpen} className="last:pb-2">
			<CollapsibleTrigger className="flex h-8 w-full cursor-pointer items-center justify-between px-2 text-xs font-medium hover:opacity-80">
				<div className="flex items-center gap-1.5">
					<ChevronDown className={cn("size-3.5 transition-transform", open ? "rotate-0" : "-rotate-90")} />
					{icon}
					<span>{title}</span>
				</div>
				{count > 0 && (
					<Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
						{count}
					</Badge>
				)}
			</CollapsibleTrigger>
			<CollapsibleContent className="pt-1">
				<div className="border rounded-md overflow-hidden bg-background">
					{loading ? (
						<div className="p-2 space-y-1.5">
							<Skeleton className="h-6 w-full" />
							<Skeleton className="h-6 w-full" />
						</div>
					) : (
						children
					)}
				</div>
			</CollapsibleContent>
		</Collapsible>
	);
}

// ---------------------------------------------------------------------------
// SearchableCheckboxList
// ---------------------------------------------------------------------------

function SearchableCheckboxList({
	items,
	isSelected,
	onToggle,
	placeholder = "Search...",
}: {
	items: { key: string; label: string; count?: number }[];
	isSelected: (key: string) => boolean;
	onToggle: (key: string) => void;
	placeholder?: string;
}) {
	const [search, setSearch] = useState("");

	const filteredItems = useMemo(() => {
		if (!search.trim()) return items;
		const q = search.toLowerCase();
		return items.filter((item) => item.label.toLowerCase().includes(q));
	}, [items, search]);

	return (
		<div className="flex flex-col">
			{items.length > 5 && (
				<div className="border-b p-1.5">
					<div className="relative">
						<Search className="text-muted-foreground absolute top-1/2 left-2 size-3 -translate-y-1/2" />
						<Input
							value={search}
							onChange={(e) => setSearch(e.target.value)}
							placeholder={placeholder}
							className="h-6 pl-7 text-[11px] rounded"
						/>
					</div>
				</div>
			)}
			<ScrollArea className="max-h-[140px] overflow-y-auto p-1" viewportClassName="no-table">
				{filteredItems.length === 0 ? (
					<div className="text-muted-foreground py-3 text-center text-[11px]">No results found</div>
				) : (
					<div className="flex flex-col gap-0.5">
						{filteredItems.map((item) => {
							const checked = isSelected(item.key);
							return (
								<label
									key={item.key}
									className={cn(
										"hover:bg-muted/60 flex cursor-pointer items-center justify-between rounded px-2 py-1.5 text-xs transition-colors",
										checked && "bg-primary/5 text-primary font-medium",
									)}
								>
									<div className="flex items-center gap-2 min-w-0">
										<Checkbox
											checked={checked}
											onCheckedChange={() => onToggle(item.key)}
											className="size-3.5"
										/>
										<TruncatedLabel className="text-xs">{item.label}</TruncatedLabel>
									</div>
									{typeof item.count === "number" && item.count > 0 && (
										<span className="text-[10px] text-muted-foreground font-mono ml-1 shrink-0">
											{item.count}
										</span>
									)}
								</label>
							);
						})}
					</div>
				)}
			</ScrollArea>
		</div>
	);
}

export default PromptFilterSidebar;
