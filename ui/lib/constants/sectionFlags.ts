import type { FeatureFlagStatus } from "@/lib/types/featureFlag";
import { WORKSPACE_SECTIONS, type WorkspaceSectionKey } from "@/lib/constants/workspaceSections";

/** Flag id for a workspace section. Settings is never gated. */
export function sectionFlagId(sectionKey: string): string {
	return `section.${sectionKey}`;
}

/** True while flags are still loading, and when the server has no row yet. */
export function isSectionFlagEnabled(flags: FeatureFlagStatus[] | undefined, sectionKey: string): boolean {
	if (sectionKey === "settings") return true;
	if (!flags) return true;
	const flag = flags.find((f) => f.id === sectionFlagId(sectionKey));
	if (!flag) return true;
	return flag.enabled;
}

/** Longest matching workspace section for a path. Settings stays ungated. */
export function sectionKeyForWorkspacePath(pathname: string): WorkspaceSectionKey | null {
	let best: { key: WorkspaceSectionKey; len: number } | null = null;
	for (const section of WORKSPACE_SECTIONS) {
		if (section.key === "settings" || section.key === "cluster-config") continue;
		const paths = [section.defaultPath, ...(section.items ?? []).map((item) => item.path)];
		for (const raw of paths) {
			const path = raw.split("?")[0];
			if (pathname !== path && !pathname.startsWith(`${path}/`)) continue;
			if (!best || path.length > best.len) best = { key: section.key, len: path.length };
		}
	}
	return best?.key ?? null;
}
