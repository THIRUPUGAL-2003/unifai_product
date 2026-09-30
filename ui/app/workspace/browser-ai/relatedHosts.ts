/** Target Website helpers — admin-added domains only; no hardcoded AI site lists. */

export type HostRole = "" | "ui" | "chat" | "file";

export const HOST_ROLE_OPTIONS: { value: HostRole; label: string }[] = [
	{ value: "", label: "Auto" },
	{ value: "ui", label: "Main UI" },
	{ value: "chat", label: "Chat domain" },
	{ value: "file", label: "File domain" },
];

export function hostRoleLabel(role?: string): string {
	const found = HOST_ROLE_OPTIONS.find((o) => o.value === (role || ""));
	return found?.label || "Auto";
}

export type RelatedHostGroup = {
	label: string;
	reason: string;
	hosts: string[];
};

/** True when host is the domain itself or a subdomain of it. */
export function isCoveredByDomain(host: string, domain: string): boolean {
	const h = normalizeTargetDomain(host);
	const d = normalizeTargetDomain(domain);
	if (!h || !d) return false;
	return h === d || h.endsWith("." + d);
}

export function isCoveredByAny(host: string, domains: string[]): boolean {
	return domains.some((d) => isCoveredByDomain(host, d));
}

/** No hardcoded suggestions — only domains you add in Target Websites are used. */
export function relatedHostsForDomain(_domain: string): RelatedHostGroup | null {
	return null;
}

export function relatedHostOptions(_domain: string, _alreadyAdded: string[]): string[] {
	return [];
}

export type TargetHostNode = {
	id: string;
	domain?: string;
	parent_id?: string;
};

export type TargetHostGroup<T extends TargetHostNode> = {
	parent: T;
	children: T[];
};

/** Nest related hosts under the domain they were added to (one level). */
export function groupTargetsByParent<T extends TargetHostNode>(targets: T[]): TargetHostGroup<T>[] {
	const byId = new Map(targets.map((t) => [t.id, t]));
	const parentOf = new Map<string, string>();

	const link = (parentId: string, childId: string) => {
		if (!parentId || !byId.has(parentId) || parentId === childId || parentOf.has(childId)) return;
		parentOf.set(childId, parentId);
	};

	for (const tgt of targets) {
		const pid = (tgt.parent_id || "").trim();
		if (pid) link(pid, tgt.id);
	}

	const unassigned = () => targets.filter((t) => !parentOf.has(t.id));
	for (const tgt of unassigned()) {
		let best: T | undefined;
		const mine = (tgt.domain || "").toLowerCase();
		for (const other of unassigned()) {
			if (other.id === tgt.id) continue;
			const d = (other.domain || "").toLowerCase();
			if (d && mine.endsWith(`.${d}`) && (!best || d.length > (best.domain || "").length)) {
				best = other;
			}
		}
		if (best) link(best.id, tgt.id);
	}

	const rootId = (id: string) => {
		const seen = new Set<string>();
		let cur = id;
		while (parentOf.has(cur) && !seen.has(cur)) {
			seen.add(cur);
			cur = parentOf.get(cur) as string;
		}
		return cur;
	};

	const childrenOf = new Map<string, T[]>();
	for (const tgt of targets) {
		if (!parentOf.has(tgt.id)) continue;
		const rid = rootId(tgt.id);
		if (rid === tgt.id) continue;
		const list = childrenOf.get(rid) || [];
		list.push(tgt);
		childrenOf.set(rid, list);
	}

	const sortDom = (a: T, b: T) => (a.domain || "").localeCompare(b.domain || "");
	return targets
		.filter((t) => !parentOf.has(t.id) || rootId(t.id) === t.id)
		.sort(sortDom)
		.map((parent) => ({
			parent,
			children: (childrenOf.get(parent.id) || []).sort(sortDom),
		}));
}

export function normalizeTargetDomain(raw: string): string {
	let d = (raw || "").trim().toLowerCase();
	if (!d) return "";
	if (d.includes("://")) d = d.split("://", 2)[1];
	d = d.split("/")[0].split("?")[0].split("#")[0];
	if (d.startsWith("[")) {
		const end = d.indexOf("]");
		if (end > 0) d = d.slice(1, end);
	} else if (d.includes(":")) {
		d = d.replace(/:\d+$/, "");
	}
	if (d.startsWith("www.")) d = d.slice(4);
	return d.trim();
}
