/** True for Prompt Repository members (everything except workspace admin). */
export function isPromptMemberRole(role?: string | null): boolean {
	return Boolean(role && role !== "admin");
}
