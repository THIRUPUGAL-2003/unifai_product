import { Link } from "@tanstack/react-router";
import type { UserAccessProfile } from "@enterprise/lib/types/accessProfile";
import { IdCard } from "lucide-react";

interface ManagedVirtualKeyNoticeProps {
	managingProfile?: UserAccessProfile;
}

export default function ManagedVirtualKeyNotice({ managingProfile }: ManagedVirtualKeyNoticeProps) {
	if (!managingProfile) return null;

	return (
		<div
			className="border-amber-500/30 bg-amber-500/10 text-amber-100 flex items-start gap-3 rounded-lg border px-3 py-2.5 text-sm"
			data-testid="vk-managed-by-access-profile"
		>
			<IdCard className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
			<div className="min-w-0 space-y-1">
				<p className="font-medium text-amber-200">Managed by Access Profile</p>
				<p className="text-amber-100/80 text-xs">
					Budgets, providers, and MCP settings on this key come from{" "}
					<span className="font-medium text-amber-50">{managingProfile.name}</span>. Edit the profile instead of this key.
				</p>
				<Link
					to="/workspace/governance/access-profiles"
					className="text-xs font-medium text-teal-300 underline-offset-2 hover:underline"
				>
					Open Access Profiles
				</Link>
			</div>
		</div>
	);
}
