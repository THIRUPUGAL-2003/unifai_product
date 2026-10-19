import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { resetDurationLabels } from "@/lib/constants/governance";
import { getErrorMessage, useGetTeamMembersQuery, useGetTeamsQuery, useGetVirtualKeysQuery } from "@/lib/store";
import { Customer, Team } from "@/lib/types/governance";
import { BusinessUnit } from "@enterprise/lib/types/workspace";
import { cn } from "@/lib/utils";
import { virtualKeysForCustomer } from "@/lib/utils/governance";
import { formatCompactNumber } from "@/lib/utils/numbers";
import { useGetBusinessUnitsQuery } from "@enterprise/lib/store/apis/businessUnitsApi";

interface Props {
	customer: Customer | null;
	open: boolean;
	onOpenChange: (open: boolean) => void;
}

function formatResetDuration(duration: string | null | undefined): string {
	if (!duration) return "";
	return resetDurationLabels[duration] || duration;
}

function formatCurrency(value: number): string {
	return `$${value.toFixed(2)}`;
}

function DetailCard({ title, children, contentClassName }: { title: string; children: React.ReactNode; contentClassName?: string }) {
	return (
		<div className="space-y-2">
			<div className="flex items-center justify-between">
				<h3 className="text-muted-foreground text-xs font-medium tracking-wider uppercase">{title}</h3>
			</div>
			<div className={cn("rounded-md border px-4 py-3", contentClassName)}>{children}</div>
		</div>
	);
}

function BudgetLineBar({ current, max, resetDuration }: { current: number; max: number; resetDuration?: string }) {
	const pct = max > 0 ? Math.min((current / max) * 100, 100) : 0;
	const isOver80 = pct >= 80;
	const isOver100 = pct >= 100;
	return (
		<div className="space-y-1.5">
			<div className="flex items-center justify-between text-xs">
				<span className="font-medium" />
				<span className="text-muted-foreground">{formatResetDuration(resetDuration)}</span>
			</div>
			<div className="bg-muted h-1.5 w-full overflow-hidden rounded-full">
				<div
					className={`h-full rounded-full transition-all ${isOver100 ? "bg-destructive" : isOver80 ? "bg-orange-500" : "bg-primary"}`}
					style={{ width: `${pct}%` }}
				/>
			</div>
			<div className="text-muted-foreground flex items-center justify-between text-xs">
				<span>
					{formatCurrency(current)} / {formatCurrency(max)}
				</span>
				<span>{pct.toFixed(0)}%</span>
			</div>
		</div>
	);
}

function RateLimitBar({ label, current, max, resetDuration }: { label: string; current: number; max: number; resetDuration?: string }) {
	const pct = max > 0 ? Math.min((current / max) * 100, 100) : 0;
	const isOver80 = pct >= 80;
	const isOver100 = pct >= 100;
	return (
		<div className="space-y-1.5">
			<div className="flex items-center justify-between text-xs">
				<span>{label}</span>
				<span className="text-muted-foreground">{formatResetDuration(resetDuration)}</span>
			</div>
			<div className="bg-muted h-1.5 w-full overflow-hidden rounded-full">
				<div
					className={`h-full rounded-full transition-all ${isOver100 ? "bg-destructive" : isOver80 ? "bg-orange-500" : "bg-primary"}`}
					style={{ width: `${pct}%` }}
				/>
			</div>
			<div className="text-muted-foreground flex items-center justify-between text-xs">
				<span>
					{formatCompactNumber(current)} / {formatCompactNumber(max)}
				</span>
				<span>{pct.toFixed(0)}%</span>
			</div>
		</div>
	);
}

function EmptyLine({ children }: { children: React.ReactNode }) {
	return <p className="text-muted-foreground py-1 text-center text-sm">{children}</p>;
}

function TeamWithUsers({ team }: { team: Pick<Team, "id" | "name"> }) {
	const { data, isError } = useGetTeamMembersQuery(team.id);
	const members = data?.members || [];
	return (
		<li className="space-y-1 rounded-md border px-3 py-2">
			<span className="text-sm font-medium">{team.name}</span>
			{isError ? (
				<p className="text-muted-foreground ml-3 text-xs">Could not load users.</p>
			) : members.length === 0 ? (
				<p className="text-muted-foreground ml-3 text-xs">No users</p>
			) : (
				<ul className="ml-3 space-y-0.5">
					{members.map((member) => (
						<li key={member.user_id || member.id} className="text-muted-foreground text-xs">
							{member.username || member.email || member.user_id}
						</li>
					))}
				</ul>
			)}
		</li>
	);
}

function BusinessUnitBranch({ unit, teams }: { unit: BusinessUnit; teams: Team[] }) {
	const ids = new Set(unit.team_ids || []);
	const unitTeams = teams.filter((team) => ids.has(team.id));
	return (
		<li className="space-y-2 rounded-md border px-3 py-2">
			<span className="text-sm font-medium">{unit.name}</span>
			{unitTeams.length === 0 ? (
				<p className="text-muted-foreground ml-3 text-xs">No teams in this unit</p>
			) : (
				<ul className="ml-3 space-y-2">
					{unitTeams.map((team) => (
						<TeamWithUsers key={team.id} team={team} />
					))}
				</ul>
			)}
		</li>
	);
}

export function CustomerDetailSheet({ customer, open, onOpenChange }: Props) {
	const budgets = customer?.budgets ?? [];
	const rateLimit = customer?.rate_limit;
	const hasRateLimit = rateLimit?.token_max_limit != null || rateLimit?.request_max_limit != null;

	const { data: teamsData, isError: teamsFailed, error: teamsError } = useGetTeamsQuery(undefined, { skip: !open || !customer });
	const { data: vksData, isError: vkFailed, error: vkError } = useGetVirtualKeysQuery(undefined, { skip: !open || !customer });
	const { data: buData, isError: buFailed, error: buError } = useGetBusinessUnitsQuery(undefined, { skip: !open || !customer });
	const helperQueryFailed = teamsFailed || vkFailed || buFailed;
	const helperQueryError = teamsError || vkError || buError;

	const allTeams = teamsData?.teams || [];
	const customerTeams = allTeams.filter((t) => t.customer_id === customer?.id);
	const customerVKs = customer ? virtualKeysForCustomer(vksData?.virtual_keys || [], customer.id, customerTeams) : [];
	const customerBUs = (buData?.business_units || []).filter((bu) => bu.customer_id === customer?.id);
	const buTeamIds = new Set(customerBUs.flatMap((bu) => bu.team_ids || []));
	const directTeams = customerTeams.filter((team) => !buTeamIds.has(team.id));

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="max-w-[700px] overflow-y-auto p-0 pt-4">
				<SheetHeader className="flex flex-col items-start px-0 py-4" headerClassName="mb-0 px-8 sticky -top-4 bg-card z-10">
					<SheetTitle className="text-lg">{customer?.name || "Customer Details"}</SheetTitle>
					<SheetDescription>
						Customer, business units, teams, and users.
					</SheetDescription>
				</SheetHeader>

				{customer && (
					<div className="space-y-6 px-8 py-4">
						{helperQueryFailed ? (
							<QueryErrorBanner
								testId="customer-detail-query-error"
								message={getErrorMessage(helperQueryError) || "Failed to load teams, virtual keys, or business units."}
							/>
						) : null}
						<DetailCard title="Info">
							<div className="grid grid-cols-2 gap-x-8 gap-y-4">
								<div>
									<Label className="text-muted-foreground text-xs">Name</Label>
									<p className="mt-0.5 text-sm">{customer.name ?? "—"}</p>
								</div>
							</div>
						</DetailCard>

						<DetailCard title="Budgets">
							{budgets.length > 0 ? (
								<div className="space-y-3">
									{[...budgets]
										.sort((a, b) => (b.max_limit || 0) - (a.max_limit || 0))
										.map((b) => (
											<BudgetLineBar key={b.id} current={b.current_usage} max={b.max_limit} resetDuration={b.reset_duration} />
										))}
								</div>
							) : (
								<EmptyLine>No budgets configured</EmptyLine>
							)}
						</DetailCard>

						<DetailCard title="Rate Limits">
							{rateLimit && hasRateLimit ? (
								<div className="space-y-3">
									{rateLimit.token_max_limit != null && (
										<RateLimitBar
											label="Tokens"
											current={rateLimit.token_current_usage ?? 0}
											max={rateLimit.token_max_limit}
											resetDuration={rateLimit.token_reset_duration}
										/>
									)}
									{rateLimit.request_max_limit != null && (
										<RateLimitBar
											label="Requests"
											current={rateLimit.request_current_usage ?? 0}
											max={rateLimit.request_max_limit}
											resetDuration={rateLimit.request_reset_duration}
										/>
									)}
								</div>
							) : (
								<EmptyLine>No rate limits configured</EmptyLine>
							)}
						</DetailCard>

						<DetailCard title="Business Units">
							{customerBUs.length > 0 ? (
								<ul className="space-y-2">
									{customerBUs.map((unit) => (
										<BusinessUnitBranch key={unit.id} unit={unit} teams={allTeams} />
									))}
								</ul>
							) : (
								<EmptyLine>No business units linked to this customer</EmptyLine>
							)}
						</DetailCard>

						<DetailCard title="Teams">
							{directTeams.length > 0 ? (
								<ul className="space-y-2">
									{directTeams.map((team) => (
										<TeamWithUsers key={team.id} team={team} />
									))}
								</ul>
							) : (
								<EmptyLine>No teams linked directly to this customer</EmptyLine>
							)}
						</DetailCard>

						<DetailCard title="Virtual Keys">
							{customerVKs.length > 0 ? (
								<ul className="space-y-1.5 text-sm">
									{customerVKs.map(({ vk, scope, teamNames }) => (
										<li key={vk.id} className="flex items-center justify-between gap-2">
											<span className="min-w-0 truncate">{vk.name}</span>
											{scope === "customer" ? (
												<Badge variant="outline" className="shrink-0 text-xs">
													All teams
												</Badge>
											) : (
												<Badge variant="secondary" className="max-w-[60%] shrink truncate text-xs" title={teamNames.join(", ")}>
													Only: {teamNames.join(", ")}
												</Badge>
											)}
										</li>
									))}
								</ul>
							) : (
								<EmptyLine>No virtual keys (attach VK to this customer or its teams)</EmptyLine>
							)}
						</DetailCard>

					</div>
				)}
			</SheetContent>
		</Sheet>
	);
}

export default CustomerDetailSheet;
