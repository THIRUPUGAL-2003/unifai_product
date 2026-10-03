/**
 * Parses a duration string (e.g., "1m", "5m", "1h", "1d", "1w", "1M") into human readable format
 */
export function parseResetPeriod(duration: string): string {
	if (!duration) return "Unknown";

	const timeValue = parseInt(duration.slice(0, -1));
	const timeUnit = duration.slice(-1);

	const unitMap: Record<string, { singular: string; plural: string }> = {
		s: { singular: "second", plural: "seconds" },
		m: { singular: "minute", plural: "minutes" },
		h: { singular: "hour", plural: "hours" },
		d: { singular: "day", plural: "days" },
		w: { singular: "week", plural: "weeks" },
		M: { singular: "month", plural: "months" },
		Y: { singular: "year", plural: "years" },
	};

	const unit = unitMap[timeUnit];
	if (!unit) return duration;

	const unitName = timeValue === 1 ? unit.singular : unit.plural;
	return `${timeValue} ${unitName}`;
}

import type { Team, VirtualKey } from "@/lib/types/governance";
import { formatCompactNumber } from "./numbers";

/** Team ids a VK is linked to (multi-team join table plus the legacy single field). */
export function vkTeamIds(vk: VirtualKey): string[] {
	const ids = new Set<string>(vk.team_ids ?? []);
	for (const t of vk.teams ?? []) ids.add(t.id);
	if (vk.team_id) ids.add(vk.team_id);
	return [...ids];
}

/** Customer ids a VK is assigned to directly (multi-customer join table plus the legacy single field). */
export function vkCustomerIds(vk: VirtualKey): string[] {
	const ids = new Set<string>(vk.customer_ids ?? []);
	for (const c of vk.customers ?? []) ids.add(c.id);
	if (vk.customer_id) ids.add(vk.customer_id);
	return [...ids];
}

export type CustomerVirtualKey = {
	vk: VirtualKey;
	/** "customer": every team and user under the customer; "team": only the listed teams. */
	scope: "customer" | "team";
	teamNames: string[];
};

/**
 * Keys visible under a customer: keys assigned to the customer itself, plus keys assigned only to
 * some of its teams (reported with those team names — they are NOT shared with the other teams).
 */
export function virtualKeysForCustomer(virtualKeys: VirtualKey[], customerId: string, customerTeams: Team[]): CustomerVirtualKey[] {
	const teamNameById = new Map(customerTeams.map((t) => [t.id, t.name]));
	const result: CustomerVirtualKey[] = [];
	for (const vk of virtualKeys) {
		if (vkCustomerIds(vk).includes(customerId)) {
			result.push({ vk, scope: "customer", teamNames: [] });
			continue;
		}
		const teamNames = vkTeamIds(vk)
			.map((id) => teamNameById.get(id))
			.filter((n): n is string => !!n);
		if (teamNames.length > 0) result.push({ vk, scope: "team", teamNames });
	}
	return result;
}

export type TeamVirtualKey = {
	vk: VirtualKey;
	/** "team": assigned to this team; "customer": inherited from the team's customer. */
	via: "team" | "customer";
};

/** Keys a team's members get: keys linked to the team, plus keys assigned to the team's customer. */
export function virtualKeysForTeam(virtualKeys: VirtualKey[], team: Pick<Team, "id" | "customer_id">): TeamVirtualKey[] {
	const result: TeamVirtualKey[] = [];
	for (const vk of virtualKeys) {
		if (vkTeamIds(vk).includes(team.id)) result.push({ vk, via: "team" });
		else if (team.customer_id && vkCustomerIds(vk).includes(team.customer_id)) result.push({ vk, via: "customer" });
	}
	return result;
}

export function describeTeamVirtualKey(entry: TeamVirtualKey): string {
	return entry.via === "team" ? entry.vk.name : `${entry.vk.name} (from customer)`;
}

export function describeCustomerVirtualKey(entry: CustomerVirtualKey): string {
	return entry.scope === "customer" ? `${entry.vk.name} (all teams)` : `${entry.vk.name} (only: ${entry.teamNames.join(", ")})`;
}

export function formatCurrency(dollars: number | null | undefined) {
	const value = typeof dollars === "number" && Number.isFinite(dollars) ? dollars : 0;
	return `$${value.toFixed(2)}`;
}

const shortDurationLabels: Record<string, string> = {
	"1m": "/min",
	"5m": "/5min",
	"15m": "/15min",
	"30m": "/30min",
	"1h": "/hr",
	"6h": "/6hr",
	"1d": "/day",
	"1w": "/wk",
	"1M": "/mo",
	"1Y": "/yr",
};

/**
 * Formats rate limit into compact display lines.
 * e.g. ["10K tokens/hr", "100 req/hr"]
 */
export function formatRateLimitLines(
	rateLimits:
		| {
				token_max_limit?: number | null;
				token_reset_duration?: string | null;
				request_max_limit?: number | null;
				request_reset_duration?: string | null;
		  }
		| null
		| undefined,
): string[] {
	if (!rateLimits) return [];
	const lines: string[] = [];
	if (rateLimits.token_max_limit != null) {
		const duration = rateLimits.token_reset_duration ?? "";
		const suffix = shortDurationLabels[duration] ?? (duration ? `/${duration}` : "");
		lines.push(`${formatCompactNumber(rateLimits.token_max_limit)} tokens${suffix}`);
	}
	if (rateLimits.request_max_limit != null) {
		const duration = rateLimits.request_reset_duration ?? "";
		const suffix = shortDurationLabels[duration] ?? (duration ? `/${duration}` : "");
		lines.push(`${formatCompactNumber(rateLimits.request_max_limit)} req${suffix}`);
	}
	return lines;
}

/**
 * Calculates usage percentage for rate limits
 */
export function calculateUsagePercentage(current: number, max: number): number {
	if (max === 0) return 0;
	return Math.round((current / max) * 100);
}

/**
 * Gets the appropriate variant for usage percentage badges
 */
export function getUsageVariant(percentage: number): "default" | "secondary" | "destructive" | "outline" {
	if (percentage >= 90) return "destructive";
	if (percentage >= 75) return "secondary";
	return "default";
}