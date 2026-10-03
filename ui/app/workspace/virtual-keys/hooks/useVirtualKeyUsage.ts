import { Budget, RateLimit, VirtualKey } from "@/lib/types/governance";
import { useGetUserAccessProfilesQuery } from "@enterprise/lib/store/apis/accessProfileApi";
import { useGetVirtualKeyUsersQuery, VirtualKeyUser } from "@enterprise/lib/store/apis/virtualKeyUsersApi";
import { UserAccessProfile } from "@enterprise/lib/types/accessProfile";

/**
 * When a VK is attached to users via an access profile, the governance plugin tracks usage on the
 * AP rather than on the VK itself (to avoid double-counting). This hook resolves the managing AP
 * for a VK and returns budget/rate-limit values that prefer AP counters, falling back to the VK's
 * own when there is no managing profile.
 *
 * The AP query polls every 5s so bars reflect live usage without manual refresh.
 *
 * `assignedUsers` lists everyone who can use the key (direct, team and customer members);
 * `directUsers` only the users linked to the key itself — the only ones the edit form owns.
 * An access profile provisions its key for a single user, who is the first direct link.
 */
export function useVirtualKeyUsage(vk: VirtualKey | null | undefined): {
	assignedUsers: VirtualKeyUser[];
	directUsers: VirtualKeyUser[];
	isManagedByProfile: boolean;
	managingProfile: UserAccessProfile | undefined;
	hasApRateLimit: boolean;
	displayBudgets: Budget[] | undefined;
	displayRateLimit: RateLimit | undefined;
	isExhausted: boolean;
} {
	const { data: vkUsersData } = useGetVirtualKeyUsersQuery(vk?.id ?? "", { skip: !vk?.id });
	const assignedUsers = vkUsersData?.users ?? [];
	const directUsers = assignedUsers.filter((u) => !u.origin || u.origin === "direct");

	const managingUserId = directUsers[0]?.id;
	const { data: userAPsData } = useGetUserAccessProfilesQuery(managingUserId ?? "", {
		skip: !managingUserId,
		pollingInterval: managingUserId ? 5000 : 0,
	});
	const userAPs = userAPsData?.access_profiles ?? [];

	// Only treat the VK as AP-managed when an AP explicitly lists this VK in its virtual_key_ids.
	// No fallback to "first active" / "first AP" — that misattributed budgets in multi-AP setups.
	const managingProfile = vk ? userAPs.find((p) => p.virtual_key_ids?.includes(vk.id)) : undefined;
	const isManagedByProfile = managingProfile !== undefined;

	const displayBudgets: Budget[] | undefined = managingProfile
		? (managingProfile.budgets ?? []).map((line, idx) => ({
				id: line.id || `ap-budget-${managingProfile.id}-${idx}`,
				max_limit: line.max_limit ?? 0,
				reset_duration: line.reset_duration ?? "",
				current_usage: line.current_usage ?? 0,
				last_reset: line.last_reset ?? "",
			}))
		: vk?.budgets;

	const apRL = managingProfile?.rate_limit;
	const hasApRateLimit = !!(apRL && (apRL.token_max_limit != null || apRL.request_max_limit != null));
	// When profile-managed, never fall back to raw VK rate limits (that would contradict the
	// locked edit/delete UX). If the profile has no rate limit, displayRateLimit is undefined.
	const displayRateLimit: RateLimit | undefined = managingProfile
		? hasApRateLimit
			? {
					id: "",
					token_max_limit: apRL?.token_max_limit,
					token_reset_duration: apRL?.token_reset_duration,
					token_current_usage: apRL?.token_current_usage ?? 0,
					token_last_reset: apRL?.token_last_reset ?? "",
					request_max_limit: apRL?.request_max_limit,
					request_reset_duration: apRL?.request_reset_duration,
					request_current_usage: apRL?.request_current_usage ?? 0,
					request_last_reset: apRL?.request_last_reset ?? "",
				}
			: undefined
		: vk?.rate_limit;

	const isExhausted =
		(displayBudgets?.some((b) => b.current_usage >= b.max_limit) ?? false) ||
		(displayRateLimit?.token_current_usage != null &&
			displayRateLimit?.token_max_limit != null &&
			displayRateLimit.token_max_limit > 0 &&
			displayRateLimit.token_current_usage >= displayRateLimit.token_max_limit) ||
		(displayRateLimit?.request_current_usage != null &&
			displayRateLimit?.request_max_limit != null &&
			displayRateLimit.request_max_limit > 0 &&
			displayRateLimit.request_current_usage >= displayRateLimit.request_max_limit);

	return { assignedUsers, directUsers, isManagedByProfile, managingProfile, hasApRateLimit, displayBudgets, displayRateLimit, isExhausted };
}