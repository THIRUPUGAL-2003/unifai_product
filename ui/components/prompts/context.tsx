import {
	extractVariablesFromMessages,
	mergeVariables,
	Message,
	MessageRole,
	MessageType,
	type ToolCall,
	type VariableMap,
} from "@/lib/message";
import { baseApi, getErrorMessage, useAppDispatch, useGetVirtualKeyBillingBlocksQuery, useGetVirtualKeysQuery, useIsAuthEnabledQuery } from "@/lib/store";
import { useGetCoreConfigQuery } from "@/lib/store/apis/configApi";
import { v4 as uuidv4 } from "uuid";
import {
	useDeleteFolderMutation,
	useDeletePromptMutation,
	useGetFoldersQuery,
	useGetPromptsQuery,
	useGetPromptVersionQuery,
	useGetSessionQuery,
	useGetSessionsQuery,
	useUpdateFolderMutation,
	useUpdatePromptMutation,
	useCreateSessionMutation,
	useUpdateSessionMutation,
} from "@/lib/store/apis/promptsApi";
import { useGetModelParametersQuery } from "@/lib/store/apis/providersApi";
import { useGetSkillQuery } from "@/lib/store/apis/skillsApi";
import { Folder, ModelParams, Prompt, PromptFilters, PromptSession, PromptVersion } from "@/lib/types/prompts";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { useGetUserVirtualKeysQuery } from "@enterprise/lib/store/apis/virtualKeyUsersApi";
import { parseAsInteger, parseAsString, useQueryStates } from "nuqs";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { executePrompt, executeToolCall, MCPAuthRequiredError } from "./utils/executor";
import { isPromptMemberRole } from "./utils/memberRole";

/** Resolve a playground VK selection (id / value / allowed list / member fallback) to an sk-uf- secret. */
function resolveExecVirtualKey(opts: {
	virtualKeyId: string;
	allowedVirtualKeys: string[] | null;
	virtualKeys: Array<{ id: string; value?: string }> | undefined;
	isUserRole: boolean;
	assignedMemberVkValue: string;
	apiKeyId?: string;
}): string {
	// If the user explicitly deselected the VK (empty or __none__), do not force any VK!
	if (opts.virtualKeyId === "" || opts.virtualKeyId === "__none__") {
		return "";
	}
	const allVKs = opts.virtualKeys ?? [];
	const findVkVal = (target: string) => {
		const match = allVKs.find((vk) => vk.value === target || vk.id === target);
		return match?.value && String(match.value).startsWith("sk-uf-") ? match.value : "";
	};
	let execVkId = opts.virtualKeyId;
	if (execVkId && execVkId !== "__auto__") {
		const resolved = findVkVal(execVkId);
		if (resolved) execVkId = resolved;
	}
	if (!String(execVkId).startsWith("sk-uf-")) {
		for (const val of opts.allowedVirtualKeys ?? []) {
			const resolved = findVkVal(val) || (String(val).startsWith("sk-uf-") ? val : "");
			if (resolved) {
				execVkId = resolved;
				break;
			}
		}
	}
	// Fall back to assignedMemberVkValue only if virtualKeyId is "__auto__" AND no direct provider API key is chosen
	const hasDirectApiKey = Boolean(opts.apiKeyId && opts.apiKeyId !== "__auto__" && opts.apiKeyId !== "" && !String(opts.apiKeyId).startsWith("sk-uf-"));
	if (!String(execVkId).startsWith("sk-uf-") && opts.isUserRole && opts.assignedMemberVkValue && !hasDirectApiKey && opts.virtualKeyId === "__auto__") {
		execVkId = opts.assignedMemberVkValue;
	}
	return execVkId || "";
}

/** Resolve effective provider and model for execution, auto-inferring from active VK if empty or wildcard. */
function resolveExecProviderAndModel(opts: {
	provider: string;
	model: string;
	committedProvider?: string;
	committedModel?: string;
	execVkId: string;
	virtualKeys: Array<{ id: string; value?: string; provider_configs?: Array<{ provider: string; allowed_models?: string[] }> }> | undefined;
}): { execProvider: string; execModel: string } {
	let execProvider = opts.provider || opts.committedProvider || "";
	let execModel = opts.model || opts.committedModel || "";

	if (opts.execVkId) {
		const allVks = opts.virtualKeys ?? [];
		const activeVk = allVks.find((k) => k.value === opts.execVkId || k.id === opts.execVkId);
		if (activeVk?.provider_configs && activeVk.provider_configs.length > 0) {
			const vkProv = activeVk.provider_configs[0].provider;
			if (!execProvider && vkProv && vkProv !== "*") {
				execProvider = vkProv;
			}
			if (!execModel || execModel === "*") {
				const concreteModels = (activeVk.provider_configs[0].allowed_models || []).filter((m) => m && m !== "*");
				if (concreteModels.length > 0) {
					execModel = concreteModels[0];
				}
			}
		}
		if ((!execModel || execModel === "*") && execProvider) {
			if (execProvider === "openrouter") {
				execModel = "aion-labs/aion-2.0";
			} else if (execProvider === "openai") {
				execModel = "gpt-4o-mini";
			} else if (execProvider === "anthropic") {
				execModel = "claude-3-5-haiku-20241022";
			}
		}
	}
	return { execProvider, execModel };
}

interface PromptContextValue {
	// Data
	folders: Folder[];
	prompts: Prompt[];
	selectedPrompt?: Prompt;
	sessions: PromptSession[];
	selectedSession?: PromptSession;
	selectedVersion?: PromptVersion;

	// Loading states
	foldersLoading: boolean;
	promptsLoading: boolean;
	foldersError: unknown;
	promptsError: unknown;
	isLoadingPlayground: boolean;
	/** Set when session/version fetch fails (avoids infinite loading / empty playground). */
	playgroundError: string | null;
	isStreaming: boolean;

	// URL state
	selectedPromptId: string | null;
	selectedSessionId: number | null;
	selectedVersionId: number | null;
	setUrlState: (state: { promptId?: string | null; sessionId?: number | null; versionId?: number | null }) => void;

	// Playground state
	messages: Message[];
	setMessages: React.Dispatch<React.SetStateAction<Message[]>>;
	provider: string;
	setProvider: React.Dispatch<React.SetStateAction<string>>;
	model: string;
	setModel: React.Dispatch<React.SetStateAction<string>>;
	modelParams: ModelParams;
	setModelParams: React.Dispatch<React.SetStateAction<ModelParams>>;
	virtualKeyId: string;
	setVirtualKeyId: React.Dispatch<React.SetStateAction<string>>;
	apiKeyId: string;
	setApiKeyId: React.Dispatch<React.SetStateAction<string>>;
	/** Optional Skills Repository skill id for playground injection */
	skillId: string;
	setSkillId: React.Dispatch<React.SetStateAction<string>>;
	/** null = every connected MCP tool. A list = only the ticked tools. */
	mcpIncludeTools: string[] | null;
	setMcpIncludeTools: React.Dispatch<React.SetStateAction<string[] | null>>;
	/** null = not committed yet. A list = only these options are shown to prompt users. */
	allowedProviders: string[] | null;
	setAllowedProviders: React.Dispatch<React.SetStateAction<string[] | null>>;
	allowedModels: string[] | null;
	setAllowedModels: React.Dispatch<React.SetStateAction<string[] | null>>;
	allowedVirtualKeys: string[] | null;
	setAllowedVirtualKeys: React.Dispatch<React.SetStateAction<string[] | null>>;
	allowedApiKeys: string[] | null;
	setAllowedApiKeys: React.Dispatch<React.SetStateAction<string[] | null>>;
	allowedSkills: string[] | null;
	setAllowedSkills: React.Dispatch<React.SetStateAction<string[] | null>>;

	// Jinja2 variables
	variables: VariableMap;
	setVariables: React.Dispatch<React.SetStateAction<VariableMap>>;

	// Custom request headers (used to satisfy server-configured required headers)
	customHeaders: Record<string, string>;
	setCustomHeaders: React.Dispatch<React.SetStateAction<Record<string, string>>>;
	requiredHeaders: string[];

	// Sheet states
	folderSheet: { open: boolean; folder?: Folder; parentId?: string };
	setFolderSheet: React.Dispatch<React.SetStateAction<{ open: boolean; folder?: Folder; parentId?: string }>>;
	promptSheet: { open: boolean; prompt?: Prompt; folderId?: string };
	setPromptSheet: React.Dispatch<React.SetStateAction<{ open: boolean; prompt?: Prompt; folderId?: string }>>;
	commitSheet: { open: boolean; session?: PromptSession };
	setCommitSheet: React.Dispatch<React.SetStateAction<{ open: boolean; session?: PromptSession }>>;

	// Delete dialog states
	deleteFolderDialog: { open: boolean; folder?: Folder };
	setDeleteFolderDialog: React.Dispatch<React.SetStateAction<{ open: boolean; folder?: Folder }>>;
	deletePromptDialog: { open: boolean; prompt?: Prompt };
	setDeletePromptDialog: React.Dispatch<React.SetStateAction<{ open: boolean; prompt?: Prompt }>>;

	// Access dialog state
	promptAccessDialog: { open: boolean; prompt?: Prompt };
	setPromptAccessDialog: React.Dispatch<React.SetStateAction<{ open: boolean; prompt?: Prompt }>>;

	// Filter state (admin customer, team, user scoping filters)
	promptFilters: PromptFilters;
	setPromptFilters: React.Dispatch<React.SetStateAction<PromptFilters>>;

	// Mutation loading states
	isDeletingFolder: boolean;
	isDeletingPrompt: boolean;

	// Model capabilities
	supportsVision: boolean;

	// Diff detection
	hasChanges: boolean;
	hasVersionChanges: boolean;
	hasSessionChanges: boolean;

	// Handlers
	handleSelectPrompt: (id: string) => void;
	handleMovePrompt: (promptId: string, folderId: string | null) => Promise<void>;
	handleMoveTeamFolder: (folderId: string, parentId: string) => Promise<void>;
	handleMoveFolder: (folderId: string, parentId: string | null) => Promise<void>;
	handleDeleteFolder: () => Promise<void>;
	handleDeletePrompt: () => Promise<void>;
	handleSendMessage: (pendingMessage?: Message, overrideMessages?: Message[]) => Promise<void>;
	handleStopStreaming: () => void;
	handleSubmitToolResult: (afterIndex: number, toolCallId: string, content: string) => Promise<void>;
	handleExecuteToolCall: (afterIndex: number, toolCall: ToolCall) => Promise<void>;
	handleSubmitAllToolResults: (afterIndex: number, results: { toolCallId: string; content: string }[]) => Promise<void>;
	handleExecuteAllToolCalls: (afterIndex: number, toolCalls: ToolCall[]) => Promise<{ toolCallId: string; content: string }[] | undefined>;
	fetchToolResult: (toolCall: ToolCall) => Promise<string>;

	// RBAC permissions
	canCreate: boolean;
	canUpdate: boolean;
	canDelete: boolean;

	// Panel collapse states
	isSidebarOpen: boolean;
	setIsSidebarOpen: React.Dispatch<React.SetStateAction<boolean>>;
	toggleSidebar: () => void;
	isSettingsOpen: boolean;
	setIsSettingsOpen: React.Dispatch<React.SetStateAction<boolean>>;
	toggleSettings: () => void;
}

const PromptContext = createContext<PromptContextValue | null>(null);

export function usePromptContext() {
	const context = useContext(PromptContext);
	if (!context) {
		throw new Error("usePromptContext must be used within a PromptProvider");
	}
	return context;
}

export function PromptProvider({ children }: { children: ReactNode }) {
	const PROMPTS_POLL_MS = 5000;
	const [authPollMs, setAuthPollMs] = useState(PROMPTS_POLL_MS);
	const [vkPollMs, setVkPollMs] = useState(PROMPTS_POLL_MS);
	const [billingPollMs, setBillingPollMs] = useState(PROMPTS_POLL_MS);

	const { data: authStatus, error: authPollError } = useIsAuthEnabledQuery(undefined, { pollingInterval: authPollMs });
	const isUserRole = isPromptMemberRole(authStatus?.role);
	const isBuiltInUser = (authStatus?.role || "").trim().toLowerCase() === "user";

	// RBAC permissions — built-in "user" always has full Prompt Repository (never View Only).
	const rbacCanCreate = useRbac(RbacResource.PromptRepository, RbacOperation.Create);
	const rbacCanUpdate = useRbac(RbacResource.PromptRepository, RbacOperation.Update);
	const rbacCanDelete = useRbac(RbacResource.PromptRepository, RbacOperation.Delete);
	const canCreate = isBuiltInUser || rbacCanCreate;
	const canUpdate = isBuiltInUser || rbacCanUpdate;
	const canDelete = isBuiltInUser || rbacCanDelete;

	// API queries
	const { data: foldersData, isLoading: foldersLoading, error: foldersError } = useGetFoldersQuery();
	const { data: promptsData, isLoading: promptsLoading, error: promptsError } = useGetPromptsQuery();

	// Mutations
	const [deleteFolder, { isLoading: isDeletingFolder }] = useDeleteFolderMutation();
	const [deletePrompt, { isLoading: isDeletingPrompt }] = useDeletePromptMutation();
	const [updatePrompt] = useUpdatePromptMutation();
	const [updateFolder] = useUpdateFolderMutation();
	const [createSession] = useCreateSessionMutation();
	const [updateSession] = useUpdateSessionMutation();

	const dispatch = useAppDispatch();
	const { data: memberUserKeys } = useGetUserVirtualKeysQuery(authStatus?.user_id || "", {
		skip: !isUserRole || !authStatus?.user_id,
	});

	// Prefer an assigned Virtual Key when present; otherwise Auto (provider keys).
	const { data: virtualKeysData, error: vkPollError } = useGetVirtualKeysQuery(undefined, {
		pollingInterval: vkPollMs,
	});
	const { data: billingBlocks, error: billingPollError } = useGetVirtualKeyBillingBlocksQuery(undefined, {
		pollingInterval: billingPollMs,
	});

	useEffect(() => {
		setAuthPollMs(authPollError ? 0 : PROMPTS_POLL_MS);
	}, [authPollError]);
	useEffect(() => {
		setVkPollMs(vkPollError ? 0 : PROMPTS_POLL_MS);
	}, [vkPollError]);
	useEffect(() => {
		setBillingPollMs(billingPollError ? 0 : PROMPTS_POLL_MS);
	}, [billingPollError]);

	// UI state — persisted in URL query params
	const [{ promptId: selectedPromptId, sessionId: selectedSessionId, versionId: selectedVersionId }, setUrlState] = useQueryStates(
		{
			promptId: parseAsString,
			sessionId: parseAsInteger,
			versionId: parseAsInteger,
		},
		{ history: "replace" },
	);

	// Sheet states
	const [folderSheet, setFolderSheet] = useState<{ open: boolean; folder?: Folder; parentId?: string }>({ open: false });
	const [promptSheet, setPromptSheet] = useState<{ open: boolean; prompt?: Prompt; folderId?: string }>({ open: false });
	const [commitSheet, setCommitSheet] = useState<{ open: boolean; session?: PromptSession }>({ open: false });

	// Delete dialog states
	const [deleteFolderDialog, setDeleteFolderDialog] = useState<{ open: boolean; folder?: Folder }>({ open: false });
	const [deletePromptDialog, setDeletePromptDialog] = useState<{ open: boolean; prompt?: Prompt }>({ open: false });

	// Access dialog state
	const [promptAccessDialog, setPromptAccessDialog] = useState<{ open: boolean; prompt?: Prompt }>({ open: false });

	// Filter state
	const [promptFilters, setPromptFilters] = useState<PromptFilters>({
		customer_ids: [],
		team_ids: [],
		user_ids: [],
	});

	// Panel collapse states
	const [isSidebarOpen, setIsSidebarOpen] = useState(() => {
		if (typeof window !== "undefined") {
			const saved = localStorage.getItem("unifai_prompt_sidebar_open");
			return saved !== null ? saved === "true" : true;
		}
		return true;
	});

	const toggleSidebar = useCallback(() => {
		setIsSidebarOpen((prev) => {
			const next = !prev;
			if (typeof window !== "undefined") {
				localStorage.setItem("unifai_prompt_sidebar_open", String(next));
			}
			return next;
		});
	}, []);

	const [isSettingsOpen, setIsSettingsOpen] = useState(() => {
		if (typeof window !== "undefined") {
			const saved = localStorage.getItem("unifai_prompt_settings_open");
			return saved !== null ? saved === "true" : true;
		}
		return true;
	});

	const toggleSettings = useCallback(() => {
		setIsSettingsOpen((prev) => {
			const next = !prev;
			if (typeof window !== "undefined") {
				localStorage.setItem("unifai_prompt_settings_open", String(next));
			}
			return next;
		});
	}, []);

	// Playground state
	const [messages, setMessagesRaw] = useState<Message[]>([Message.system("")]);
	const setMessages = useCallback<React.Dispatch<React.SetStateAction<Message[]>>>((action) => {
		setMessagesRaw((prev) => {
			const next = typeof action === "function" ? action(prev) : action;
			return next.map((msg, i) => msg.withIndex(i));
		});
	}, []);
	const [provider, setProvider] = useState("");
	const [model, setModel] = useState("");
	const [modelParams, setModelParams] = useState<ModelParams>({ stream: true, temperature: 0.7, max_tokens: 4096 });
	const [virtualKeyId, setVirtualKeyId] = useState("__auto__");
	const [apiKeyId, setApiKeyId] = useState("__auto__");
	const [skillId, setSkillId] = useState("");
	const [mcpIncludeTools, setMcpIncludeTools] = useState<string[] | null>(null);
	const [allowedProviders, setAllowedProviders] = useState<string[] | null>(null);
	const [allowedModels, setAllowedModels] = useState<string[] | null>(null);
	const [allowedVirtualKeys, setAllowedVirtualKeys] = useState<string[] | null>(null);
	const [allowedApiKeys, setAllowedApiKeys] = useState<string[] | null>(null);
	const [allowedSkills, setAllowedSkills] = useState<string[] | null>(null);
	const [isStreaming, setIsStreaming] = useState(false);
	const activeRunRef = useRef<symbol | null>(null);
	const abortRef = useRef<AbortController | null>(null);
	// Tracks which session/version/prompt was last loaded into the playground.
	// Prevents RTK refetches from wiping locally added messages (Add button).
	const loadedPlaygroundKeyRef = useRef<string>("");
	const [variables, setVariables] = useState<VariableMap>({});
	const [customHeaders, setCustomHeaders] = useState<Record<string, string>>({});

	const assignedMemberVkValue = useMemo(() => {
		if (!isUserRole) return "";
		// Only consider virtual keys that were committed on this prompt by admin
		const committedVkIds = new Set<string>();
		if (Array.isArray(allowedVirtualKeys)) {
			for (const id of allowedVirtualKeys) {
				if (id) committedVkIds.add(id);
			}
		}
		if (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__") {
			committedVkIds.add(virtualKeyId);
		}

		// If no virtual keys were selected/committed by admin on this prompt, do NOT auto-assign any VK!
		if (committedVkIds.size === 0) {
			return "";
		}

		const vks = virtualKeysData?.virtual_keys ?? [];
		const now = Date.now();
		const validKeys = vks.filter((vk) => {
			if (vk.is_active === false) return false;
			if (vk.expires_at && new Date(vk.expires_at).getTime() < now) return false;
			if (typeof vk.value !== "string" || !vk.value.startsWith("sk-uf-")) return false;
			return committedVkIds.has(vk.id) || committedVkIds.has(vk.value);
		});

		if (validKeys.length === 0) return "";

		const currentUserId = authStatus?.user_id;
		const userAssignedKeyIds = new Set((memberUserKeys?.virtual_keys ?? []).map((k) => k.id));

		// Priority resolution: Direct User VK > Assigned to User's key list > Team/Customer VK > Other
		const getVkPriority = (vk: (typeof validKeys)[0]) => {
			if (userAssignedKeyIds.has(vk.id)) return 1;
			if (currentUserId && ((vk.user_ids && vk.user_ids.includes(currentUserId)) || (vk.users && vk.users.some((u) => u.user_id === currentUserId)))) return 1;
			if (vk.team_id || (vk.team_ids && vk.team_ids.length > 0) || (vk.teams && vk.teams.length > 0)) return 2;
			if ((vk.business_unit_ids && vk.business_unit_ids.length > 0) || (vk.business_units && vk.business_units.length > 0)) return 3;
			if (vk.customer_id || (vk.customer_ids && vk.customer_ids.length > 0) || (vk.customers && vk.customers.length > 0)) return 4;
			return 5;
		};

		const sortedKeys = [...validKeys].sort((a, b) => getVkPriority(a) - getVkPriority(b));

		const withBudget = sortedKeys.find((vk) => {
			if (billingBlocks?.blocks?.[vk.id]) return false;
			if (!vk.budgets || vk.budgets.length === 0) return true;
			return !vk.budgets.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) >= b.max_limit);
		});
		return withBudget?.value ?? sortedKeys[0]?.value ?? "";
	}, [virtualKeysData, billingBlocks, isUserRole, authStatus, memberUserKeys, allowedVirtualKeys, virtualKeyId]);

	// Do not auto-select VKs. Only restore what the user saved (loadFromParams on session/prompt).

	const { data: skillData } = useGetSkillQuery(skillId, { skip: !skillId });
	const skillSystemPrompt = skillData?.skill?.skill_md_body?.trim() || "";

	// Sync customHeaders keys with the server-configured required_headers list.
	// Adds new keys (empty), removes keys no longer required, preserves user-entered values.
	const { data: coreConfig } = useGetCoreConfigQuery({});
	const requiredHeaders = useMemo<string[]>(() => {
		const raw = coreConfig?.client_config?.required_headers;
		if (!Array.isArray(raw)) return [];
		return raw.map((item) => String(item)).filter((s) => s.length > 0);
	}, [coreConfig]);
	useEffect(() => {
		setCustomHeaders((prev) => {
			if (requiredHeaders.length === 0) {
				return Object.keys(prev).length > 0 ? {} : prev;
			}
			const next: Record<string, string> = {};
			let changed = false;
			for (const name of requiredHeaders) {
				next[name] = prev[name] ?? "";
				if (!(name in prev)) changed = true;
			}
			for (const name of Object.keys(prev)) {
				if (!requiredHeaders.includes(name)) changed = true;
			}
			return changed ? next : prev;
		});
	}, [requiredHeaders]);

	// Derived data
	const folders = useMemo(() => foldersData?.folders ?? [], [foldersData]);
	const prompts = useMemo(() => promptsData?.prompts ?? [], [promptsData]);
	const selectedPrompt = useMemo(() => prompts.find((p) => p.id === selectedPromptId), [prompts, selectedPromptId]);

	// Members run on the model they picked, falling back to the committed one.
	const capabilityModel = model || (isUserRole && selectedPrompt?.latest_version?.model) || "";
	const { data: datasheetData } = useGetModelParametersQuery(capabilityModel, { skip: !capabilityModel });
	const supportsVision = datasheetData?.supports_vision ?? false;

	// Fetch versions and sessions for selected prompt
	const {
		data: sessionsData,
		isLoading: isSessionsLoading,
		isError: sessionsFailed,
		error: sessionsError,
	} = useGetSessionsQuery(selectedPromptId ?? "", { skip: !selectedPromptId });

	// Filter sessions to current prompt — RTK Query may briefly return stale cached data from the previous prompt
	const sessions = useMemo(() => {
		const all = sessionsData?.sessions ?? [];
		if (!selectedPromptId) return [];
		return all.filter((s) => s.prompt_id === selectedPromptId);
	}, [sessionsData, selectedPromptId]);
	// The sessions list omits messages; the selected session is loaded in full on its own.
	const selectedSessionListed = useMemo(() => sessions.some((s) => s.id === selectedSessionId), [sessions, selectedSessionId]);
	const {
		data: selectedSessionData,
		isLoading: isSessionDetailLoading,
		isError: sessionDetailFailed,
		error: sessionDetailError,
	} = useGetSessionQuery(selectedSessionId ?? 0, {
		skip: !selectedSessionId || !selectedSessionListed,
	});
	const selectedSession = useMemo(() => {
		const session = selectedSessionData?.session;
		if (!session || !selectedSessionListed || session.id !== selectedSessionId || session.prompt_id !== selectedPromptId) return undefined;
		return session;
	}, [selectedSessionData, selectedSessionListed, selectedSessionId, selectedPromptId]);

	// Fetch full version data (with messages) when a version is selected
	const {
		currentData: selectedVersionData,
		isLoading: isVersionLoading,
		isFetching: isVersionFetching,
		isError: versionFailed,
		error: versionError,
	} = useGetPromptVersionQuery(selectedVersionId ?? 0, {
		skip: !selectedVersionId,
	});
	const selectedVersion = selectedVersionData?.version;

	const playgroundError = sessionsFailed
		? getErrorMessage(sessionsError) || "Failed to load sessions."
		: sessionDetailFailed
			? getErrorMessage(sessionDetailError) || "Failed to load session."
			: versionFailed
				? getErrorMessage(versionError) || "Failed to load prompt version."
				: null;

	// Show loader only on initial fetch, not on cache refetches (avoids flicker on save)
	const isLoadingPlayground = !!(
		selectedPromptId &&
		!playgroundError &&
		(isSessionsLoading ||
			(selectedSessionId && isSessionDetailLoading) ||
			(selectedVersionId && isVersionLoading) ||
			// Sessions loaded but auto-select hasn't happened yet
			(sessions.length > 0 && !selectedSessionId && !selectedVersionId))
	);

	// Load session or version data when selection changes — not on every refetch.
	useEffect(() => {
		// Don't reset state while waiting for data that hasn't arrived yet
		if (selectedSessionId && !selectedSession && !sessionDetailFailed) return;
		if (selectedVersionId && !selectedVersion && !versionFailed) return;

		const playgroundKey = selectedSessionId
			? `session:${selectedSessionId}`
			: selectedVersionId
				? `version:${selectedVersionId}`
				: selectedPromptId
					? `prompt:${selectedPromptId}`
					: "empty";

		const loadFromParams = (params: ModelParams, prov: string, mod: string) => {
			const {
				api_key_id,
				virtual_key_id,
				skill_id,
				prompt_allowed_providers,
				prompt_allowed_models,
				prompt_allowed_virtual_keys,
				prompt_allowed_api_keys,
				prompt_allowed_skills,
				prompt_allowed_mcp_tools,
				...rest
			} = params || ({} as ModelParams);
			const asList = (value: unknown): string[] | null =>
				Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : null;
			setModelParams({
				stream: true,
				temperature: typeof rest.temperature === "number" ? rest.temperature : 0.7,
				max_tokens: typeof rest.max_tokens === "number" ? rest.max_tokens : 4096,
				...rest,
			});
			let resolvedVk = virtual_key_id || "";
			let resolvedApi = api_key_id || "";
			if (!resolvedVk && resolvedApi && resolvedApi.startsWith("sk-uf-")) {
				resolvedVk = resolvedApi;
				resolvedApi = "__auto__";
			}
			setVirtualKeyId(resolvedVk || "__auto__");
			setApiKeyId(resolvedApi || "__auto__");
			setSkillId(typeof skill_id === "string" ? skill_id : "");
			setMcpIncludeTools(asList(prompt_allowed_mcp_tools));
			setAllowedProviders(asList(prompt_allowed_providers));
			setAllowedModels(asList(prompt_allowed_models));
			setAllowedVirtualKeys(asList(prompt_allowed_virtual_keys));
			setAllowedApiKeys(asList(prompt_allowed_api_keys));
			setAllowedSkills(asList(prompt_allowed_skills));
			setProvider(prov || "");
			setModel(mod || "");
		};

		const loadMessages = (msgs: Message[]) => {
			setMessages(msgs);
			const varNames = extractVariablesFromMessages(msgs);
			setVariables((prev) => mergeVariables(prev, varNames));
		};

		// Same session/version still selected — keep local edits (Add / inline edits).
		if (loadedPlaygroundKeyRef.current === playgroundKey) {
			return;
		}

		if (selectedSession) {
			const raw = (selectedSession.messages ?? []).map((m) => m.message);
			const loaded = Message.fromLegacyAll(raw);
			loadMessages(loaded.length > 0 ? loaded : [Message.system("")]);
			if (isUserRole && selectedPrompt?.latest_version) {
				// Regular user: always enforce the committed version's configuration (model, provider, keys, tools)
				const committedVer = selectedPrompt.latest_version;
				loadFromParams(committedVer.model_params, committedVer.provider, committedVer.model);
			} else {
				loadFromParams(selectedSession.model_params, selectedSession.provider, selectedSession.model);
			}
			// Restore variables (key:value) from session
			if (selectedSession.variables && Object.keys(selectedSession.variables).length > 0) {
				setVariables(selectedSession.variables);
			}
			loadedPlaygroundKeyRef.current = playgroundKey;
		} else if (selectedVersion) {
			// If sessions are still loading and no session is explicitly selected,
			// wait — a session may auto-select and take priority
			if (isSessionsLoading && !selectedSessionId) return;
			const raw = (selectedVersion.messages ?? []).map((m) => m.message);
			const loaded = Message.fromLegacyAll(raw);
			loadMessages(loaded.length > 0 ? loaded : [Message.system("")]);
			loadFromParams(selectedVersion.model_params, selectedVersion.provider, selectedVersion.model);
			// Initialize variables from version (keys with empty values)
			if (selectedVersion.variables && Object.keys(selectedVersion.variables).length > 0) {
				setVariables((prev) => mergeVariables(prev, Object.keys(selectedVersion.variables!)));
			}
			loadedPlaygroundKeyRef.current = playgroundKey;
		} else if (selectedPrompt?.latest_version) {
			// Only fall back to latest_version after sessions have settled
			// to avoid racing with the session auto-select effect
			if (isSessionsLoading) return;
			const version = selectedPrompt.latest_version;
			const raw = (version.messages ?? []).map((m) => m.message);
			const loaded = Message.fromLegacyAll(raw);
			loadMessages(loaded.length > 0 ? loaded : [Message.system("")]);
			loadFromParams(version.model_params, version.provider, version.model);
			// Initialize variables from version (keys with empty values)
			if (version.variables && Object.keys(version.variables).length > 0) {
				setVariables((prev) => mergeVariables(prev, Object.keys(version.variables!)));
			}
			if (sessions.length === 0) {
				setUrlState({ versionId: version.id });
			}
			loadedPlaygroundKeyRef.current = playgroundKey;
		} else {
			loadMessages([Message.system("")]);
			setProvider("");
			setModel("");
			setModelParams({ stream: true, temperature: 0.7, max_tokens: 4096 });
			setVirtualKeyId("__auto__");
			setApiKeyId("__auto__");
			setSkillId("");
			setMcpIncludeTools(null);
			setAllowedProviders(null);
			setAllowedModels(null);
			setAllowedVirtualKeys(null);
			setAllowedApiKeys(null);
			setAllowedSkills(null);
			loadedPlaygroundKeyRef.current = playgroundKey;
		}
	}, [
		selectedSession,
		selectedVersion,
		selectedPrompt,
		selectedPromptId,
		selectedSessionId,
		selectedVersionId,
		setUrlState,
		isSessionsLoading,
		sessions.length,
	]);

	// Auto-select user's personal prompt if none is selected
	useEffect(() => {
		if (selectedPromptId || promptsLoading || prompts.length === 0) return;
		const userEmail = authStatus?.email?.trim().toLowerCase();
		const username = authStatus?.username?.trim().toLowerCase();

		let targetPrompt = prompts.find(
			(p) =>
				(userEmail && p.name.trim().toLowerCase() === userEmail) ||
				(username && p.name.trim().toLowerCase() === username),
		);
		if (!targetPrompt && prompts.length > 0) {
			targetPrompt = prompts[0];
		}
		if (targetPrompt) {
			setUrlState({ promptId: targetPrompt.id });
		}
	}, [selectedPromptId, prompts, promptsLoading, authStatus, setUrlState]);

	// Auto-select the most recent session when sessions load and none is selected
	// Sessions take priority over versions for initial loading
	useEffect(() => {
		if (sessions.length > 0 && !selectedSessionId && !selectedVersionId) {
			setUrlState({ sessionId: sessions[0].id });
		}
	}, [selectedPromptId, sessions, selectedSessionId, selectedVersionId, setUrlState]);

	// Diff detection helper — compares current playground state against a reference config
	const diffAgainst = useCallback(
		(ref: { messages?: any[]; model_params?: ModelParams; provider?: string; model?: string } | undefined) => {
			if (!ref) return true; // No reference — treat as changed
			const refMessages = ref.messages ?? [];
			const refProvider = ref.provider;
			const refModel = ref.model;
			const refParams = ref.model_params;

			if (provider !== refProvider) return true;
			if (model !== refModel) return true;

			const { api_key_id: refApiKeyId, virtual_key_id: refVkId, skill_id: refSkillId, ...refParamsRest } = refParams || ({} as ModelParams);
			const currentVkId = virtualKeyId !== "__auto__" ? virtualKeyId : undefined;
			const currentApiKeyId = apiKeyId !== "__auto__" ? apiKeyId : undefined;
			const expectedRefVk = refVkId || (refApiKeyId?.startsWith("sk-uf-") ? refApiKeyId : undefined);
			const expectedRefApiKey = !refApiKeyId?.startsWith("sk-uf-") ? refApiKeyId : undefined;
			if (currentVkId !== (expectedRefVk || undefined)) return true;
			if (currentApiKeyId !== (expectedRefApiKey || undefined)) return true;
			if ((skillId || undefined) !== (refSkillId || undefined)) return true;
			const sameList = (current: string[] | null, saved: unknown) => {
				const refList = Array.isArray(saved) ? saved.filter((item): item is string => typeof item === "string") : null;
				// null and [] both mean "no restriction / none selected" — treat as equal so
				// sessions without saved prompt_allowed_* don't look forever dirty.
				const norm = (list: string[] | null) => (list && list.length > 0 ? list : null);
				return JSON.stringify(norm(current)) === JSON.stringify(norm(refList));
			};
			if (!sameList(allowedProviders, refParams?.prompt_allowed_providers)) return true;
			if (!sameList(allowedModels, refParams?.prompt_allowed_models)) return true;
			if (!sameList(allowedVirtualKeys, refParams?.prompt_allowed_virtual_keys)) return true;
			if (!sameList(allowedApiKeys, refParams?.prompt_allowed_api_keys)) return true;
			if (!sameList(allowedSkills, refParams?.prompt_allowed_skills)) return true;
			if (!sameList(mcpIncludeTools, refParams?.prompt_allowed_mcp_tools)) return true;

			// Normalize: treat missing stream/temperature/max_tokens as defaults so legacy params don't appear changed
			const normalizeParams = (p: ModelParams): ModelParams => {
				const { stream = true, temperature = 0.7, max_tokens = 4096, ...rest } = p || {};
				return {
					stream: Boolean(stream),
					temperature: Number(temperature),
					max_tokens: Number(max_tokens),
					...rest,
				};
			};
			const normalizedCurrent = normalizeParams(modelParams);
			const normalizedRef = normalizeParams(refParamsRest);
			if (
				JSON.stringify(normalizedCurrent, Object.keys(normalizedCurrent).sort()) !==
				JSON.stringify(normalizedRef, Object.keys(normalizedRef).sort())
			)
				return true;

			const currentSerialized = Message.serializeAll(messages);
			if (JSON.stringify(currentSerialized) !== JSON.stringify(refMessages)) return true;

			return false;
		},
		[provider, model, modelParams, virtualKeyId, apiKeyId, skillId, messages, allowedProviders, allowedModels, allowedVirtualKeys, allowedApiKeys, allowedSkills, mcpIncludeTools],
	);

	// Diff detection — compare current playground state against the loaded session/version
	const hasChanges = useMemo(() => {
		// Suppress diff while version data is in flight to avoid flicker
		if (selectedVersionId && (isVersionFetching || selectedVersion?.id !== selectedVersionId)) return false;
		if (selectedSession) {
			return diffAgainst({
				messages: selectedSession.messages?.map((m) => m.message) ?? [],
				model_params: selectedSession.model_params,
				provider: selectedSession.provider,
				model: selectedSession.model,
			});
		}
		if (selectedVersion) {
			return diffAgainst({
				messages: selectedVersion.messages?.map((m) => m.message) ?? [],
				model_params: selectedVersion.model_params,
				provider: selectedVersion.provider,
				model: selectedVersion.model,
			});
		}
		return true;
	}, [selectedSession, selectedVersion, diffAgainst, selectedVersionId, isVersionFetching]);

	// Diff against the active version — drives "unpublished changes" badge & commit button
	// Uses the explicitly selected version if available, otherwise falls back to latest_version
	const activeVersionRef = selectedVersion ?? selectedPrompt?.latest_version;

	const hasVersionChanges = useMemo(() => {
		// Suppress diff while version data is in flight or mismatched to avoid flash
		if (selectedVersionId && (isVersionFetching || selectedVersion?.id !== selectedVersionId)) return false;
		if (!activeVersionRef) return true; // No versions yet — always allow commit
		return diffAgainst({
			messages: activeVersionRef.messages?.map((m) => m.message) ?? [],
			model_params: activeVersionRef.model_params,
			provider: activeVersionRef.provider,
			model: activeVersionRef.model,
		});
	}, [activeVersionRef, diffAgainst, selectedVersionId, isVersionFetching, selectedVersion?.id]);

	// Diff against the selected session — drives red asterisk indicator
	const hasSessionChanges = useMemo(() => {
		if (!selectedSession) return false;
		return diffAgainst({
			messages: selectedSession.messages?.map((m) => m.message) ?? [],
			model_params: selectedSession.model_params,
			provider: selectedSession.provider,
			model: selectedSession.model,
		});
	}, [selectedSession, diffAgainst]);

	// Handlers
	const handleSelectPrompt = useCallback(
		(id: string) => {
			abortRef.current?.abort();
			abortRef.current = null;
			activeRunRef.current = null;
			setIsStreaming(false);
			loadedPlaygroundKeyRef.current = "";
			setMessages([Message.system("")]);
			setProvider("");
			setModel("");
			setModelParams({ stream: true, temperature: 0.7, max_tokens: 4096 });
			setApiKeyId("__auto__");
			setUrlState({ promptId: id, sessionId: null, versionId: null });
		},
		[setUrlState],
	);

	useEffect(() => {
		if (isUserRole && !selectedPromptId && promptsData?.prompts && promptsData.prompts.length > 0) {
			handleSelectPrompt(promptsData.prompts[0].id);
		}
	}, [isUserRole, selectedPromptId, promptsData, handleSelectPrompt]);

	const handleMovePrompt = useCallback(
		async (promptId: string, folderId: string | null) => {
			try {
				await updatePrompt({ id: promptId, data: { folder_id: folderId } }).unwrap();
				toast.success("Prompt moved successfully");
			} catch (err) {
				toast.error(getErrorMessage(err) || "Failed to move prompt");
			}
		},
		[updatePrompt],
	);

	const handleMoveTeamFolder = useCallback(
		async (folderId: string, parentId: string) => {
			try {
				await updateFolder({ id: folderId, data: { parent_id: parentId } }).unwrap();
				dispatch(baseApi.util.invalidateTags(["Prompts", "Teams", "BusinessUnits", "Customers"]));
				toast.success("Team moved");
			} catch (err) {
				toast.error(getErrorMessage(err) || "Failed to move team");
			}
		},
		[updateFolder, dispatch],
	);

	const handleMoveFolder = useCallback(
		async (folderId: string, parentId: string | null) => {
			try {
				await updateFolder({ id: folderId, data: { parent_id: parentId ?? "" } }).unwrap();
				dispatch(baseApi.util.invalidateTags(["Prompts", "Teams", "BusinessUnits", "Customers"]));
				toast.success("Folder moved");
			} catch (err) {
				toast.error(getErrorMessage(err) || "Failed to move folder");
			}
		},
		[updateFolder, dispatch],
	);

	const handleDeleteFolder = useCallback(async () => {
		if (!deleteFolderDialog.folder) return;

		try {
			await deleteFolder(deleteFolderDialog.folder.id).unwrap();
			toast.success("Folder deleted");
			setDeleteFolderDialog({ open: false });
			if (selectedPrompt?.folder_id === deleteFolderDialog.folder.id) {
				setUrlState({ promptId: null, sessionId: null, versionId: null });
			}
		} catch (err) {
			toast.error("Failed to delete folder", { description: getErrorMessage(err) });
		}
	}, [deleteFolderDialog.folder, deleteFolder, selectedPrompt, setUrlState]);

	const handleDeletePrompt = useCallback(async () => {
		if (!deletePromptDialog.prompt) return;

		try {
			await deletePrompt(deletePromptDialog.prompt.id).unwrap();
			toast.success("Prompt deleted");
			setDeletePromptDialog({ open: false });
			if (selectedPromptId === deletePromptDialog.prompt.id) {
				setUrlState({ promptId: null, sessionId: null, versionId: null });
			}
		} catch (err) {
			toast.error("Failed to delete prompt", { description: getErrorMessage(err) });
		}
	}, [deletePromptDialog.prompt, deletePrompt, selectedPromptId, setUrlState]);

	// Persist playground chat to DB for every login (admin + user), keyed by user_id on the server.
	const buildPersistParams = useCallback((): ModelParams => {
		const committed = selectedPrompt?.latest_version?.model_params || ({} as ModelParams);
		if (isUserRole) {
			const params = { ...committed, ...modelParams };
			if (committed.prompt_allowed_providers) params.prompt_allowed_providers = committed.prompt_allowed_providers;
			if (committed.prompt_allowed_models) params.prompt_allowed_models = committed.prompt_allowed_models;
			if (committed.prompt_allowed_virtual_keys) params.prompt_allowed_virtual_keys = committed.prompt_allowed_virtual_keys;
			if (committed.prompt_allowed_api_keys) params.prompt_allowed_api_keys = committed.prompt_allowed_api_keys;
			if (committed.prompt_allowed_skills) params.prompt_allowed_skills = committed.prompt_allowed_skills;
			if (committed.prompt_allowed_mcp_tools) params.prompt_allowed_mcp_tools = committed.prompt_allowed_mcp_tools;
			if (committed.virtual_key_id) params.virtual_key_id = committed.virtual_key_id;
			if (committed.api_key_id) params.api_key_id = committed.api_key_id;
			return params;
		}
		const params = { ...modelParams };
		if (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__") {
			params.virtual_key_id = virtualKeyId;
		}
		if (apiKeyId && apiKeyId !== "__auto__" && apiKeyId !== "__none__") {
			params.api_key_id = apiKeyId;
		}
		if (skillId.trim()) {
			params.skill_id = skillId.trim();
		}
		const effProviders = (allowedProviders && allowedProviders.length > 0) ? allowedProviders : (provider ? [provider] : undefined);
		if (effProviders) params.prompt_allowed_providers = effProviders;

		const effModels = (allowedModels && allowedModels.length > 0) ? allowedModels : (model ? [model] : undefined);
		if (effModels) params.prompt_allowed_models = effModels;

		const effVks = (allowedVirtualKeys && allowedVirtualKeys.length > 0) ? allowedVirtualKeys : (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__" ? [virtualKeyId] : undefined);
		if (effVks) params.prompt_allowed_virtual_keys = effVks;

		const effApiKeys = (allowedApiKeys && allowedApiKeys.length > 0) ? allowedApiKeys : (apiKeyId && apiKeyId !== "__auto__" && apiKeyId !== "__none__" ? [apiKeyId] : undefined);
		if (effApiKeys) params.prompt_allowed_api_keys = effApiKeys;

		if (allowedSkills) params.prompt_allowed_skills = allowedSkills;
		if (mcpIncludeTools) params.prompt_allowed_mcp_tools = mcpIncludeTools;
		return params;
	}, [isUserRole, selectedPrompt, modelParams, virtualKeyId, apiKeyId, skillId, allowedProviders, provider, allowedModels, model, allowedVirtualKeys, allowedApiKeys, allowedSkills, mcpIncludeTools]);

	const persistPlaygroundSession = useCallback(
		async (finalMessages: Message[]) => {
			if (!selectedPrompt) return;
			const data = {
				messages: Message.serializeForStorage(finalMessages),
				model_params: buildPersistParams(),
				provider,
				model,
				variables: Object.keys(variables).length > 0 ? variables : undefined,
			};
			try {
				if (selectedSessionId) {
					await updateSession({
						id: selectedSessionId,
						promptId: selectedPrompt.id,
						data,
					}).unwrap();
				} else {
					const result = await createSession({
						promptId: selectedPrompt.id,
						data,
					}).unwrap();
					setUrlState({ sessionId: result.session.id, versionId: null });
				}
			} catch (e) {
				toast.error("Chat could not be saved", {
					description: getErrorMessage(e) || "Your messages are still on screen. Use Save to retry.",
				});
			}
		},
		[selectedPrompt, selectedSessionId, buildPersistParams, provider, model, variables, updateSession, createSession, setUrlState],
	);

	const handleSendMessage = useCallback(
		async (pendingMessage?: Message, overrideMessages?: Message[]) => {
			const runToken = Symbol();
			activeRunRef.current = runToken;
			const abortController = new AbortController();
			abortRef.current = abortController;
			const isActive = () => activeRunRef.current === runToken;

			let execApiKeyId = apiKeyId;
			if ((!execApiKeyId || execApiKeyId === "__auto__") && allowedApiKeys && allowedApiKeys.length > 0) {
				execApiKeyId = allowedApiKeys[0];
			}
			const committed = selectedPrompt?.latest_version;
			const execVkId = resolveExecVirtualKey({
				virtualKeyId,
				allowedVirtualKeys,
				virtualKeys: virtualKeysData?.virtual_keys,
				isUserRole,
				assignedMemberVkValue,
				apiKeyId: execApiKeyId,
			});
			const { execProvider, execModel } = resolveExecProviderAndModel({
				provider,
				model,
				committedProvider: committed?.provider,
				committedModel: committed?.model,
				execVkId,
				virtualKeys: virtualKeysData?.virtual_keys,
			});
			if (!execProvider || !execModel) {
				toast.error("Provider and Model Required", {
					description: "Please select a provider and model in the Settings panel before running.",
				});
				setIsStreaming(false);
				activeRunRef.current = null;
				return;
			}
			const hasVk = String(execVkId).startsWith("sk-uf-");
			const hasApiKey = Boolean(execApiKeyId && execApiKeyId !== "__auto__" && !String(execApiKeyId).startsWith("sk-uf-"));
			if (!hasVk && !hasApiKey) {
				toast.error("Virtual Key or API Key Required", {
					description: "Select a Virtual Key (or a provider API key) in Settings before running. The dropdown must show a real selection — not just a budget preview.",
				});
				setIsStreaming(false);
				activeRunRef.current = null;
				return;
			}

			const baseMessages = overrideMessages ?? messages;
			setIsStreaming(true);
			await executePrompt(
				baseMessages,
				pendingMessage,
				{
					provider: execProvider,
					model: execModel,
					modelParams,
					apiKeyId: execApiKeyId,
					virtualKeyId: execVkId,
					variables,
					customHeaders,
					skillSystemPrompt,
					skillId,
					promptId: selectedPromptId ?? undefined,
					mcpIncludeTools: mcpIncludeTools ?? undefined,
				},
				{
					onStreamingStart: (allMessages, placeholder) => {
						if (!isActive()) return;
						setMessages([...allMessages, placeholder]);
					},
					onStreamChunk: (content) => {
						if (!isActive()) return;
						setMessages((prev) => {
							const updated = [...prev];
							const last = updated[updated.length - 1];
							const clone = last.clone();
							clone.content = content;
							updated[updated.length - 1] = clone;
							return updated;
						});
					},
					onComplete: async (content, usage) => {
						if (!isActive()) return;
						const finalMessages = [...baseMessages];
						if (pendingMessage) finalMessages.push(pendingMessage);
						finalMessages.push(Message.response(content, 0, usage));
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onToolCallComplete: async (content, toolCalls, usage) => {
						if (!isActive()) return;
						const finalMessages = [...baseMessages];
						if (pendingMessage) finalMessages.push(pendingMessage);
						finalMessages.push(Message.toolCallResponse(content, toolCalls, 0, usage));
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onEmptyResponse: () => {
						if (!isActive()) return;
						setMessages((prev) => prev.slice(0, -1));
					},
					onError: async (error) => {
						if (!isActive()) return;
						const finalMessages = [...baseMessages];
						if (pendingMessage) finalMessages.push(pendingMessage);
						finalMessages.push(Message.error(error));
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onFinally: () => {
						if (!isActive()) return;
						setIsStreaming(false);
						dispatch(baseApi.util.invalidateTags(["Logs", "MCPLogs", "VirtualKeys", "Sessions", "Budgets", "Teams", "Customers", "ProviderGovernance"]));
					},
				},
				abortController.signal,
			);
		},
		[
			messages,
			provider,
			model,
			modelParams,
			virtualKeyId,
			apiKeyId,
			allowedVirtualKeys,
			virtualKeysData,
			variables,
			customHeaders,
			skillSystemPrompt,
			skillId,
			mcpIncludeTools,
			selectedPromptId,
			selectedPrompt,
			isUserRole,
			assignedMemberVkValue,
			persistPlaygroundSession,
		],
	);

	const handleSubmitToolResult = useCallback(
		async (afterIndex: number, toolCallId: string, content: string) => {
			const runToken = Symbol();
			activeRunRef.current = runToken;
			const abortController = new AbortController();
			abortRef.current = abortController;
			const isActive = () => activeRunRef.current === runToken;

			const toolResultMsg = new Message(uuidv4(), 0, MessageType.ToolResult, {
				role: MessageRole.TOOL,
				content,
				tool_call_id: toolCallId,
			});
			const newMessages = [...messages];
			// Insert after any existing tool results that follow the assistant message
			let insertAt = afterIndex + 1;
			while (insertAt < newMessages.length && newMessages[insertAt].type === MessageType.ToolResult) {
				insertAt++;
			}
			newMessages.splice(insertAt, 0, toolResultMsg);
			setMessages(newMessages);

			// Execute with the updated messages
			let execApiKeyId = apiKeyId;
			if ((!execApiKeyId || execApiKeyId === "__auto__") && allowedApiKeys && allowedApiKeys.length > 0) {
				execApiKeyId = allowedApiKeys[0];
			}
			const committed = selectedPrompt?.latest_version;
			const execVkId = resolveExecVirtualKey({
				virtualKeyId,
				allowedVirtualKeys,
				virtualKeys: virtualKeysData?.virtual_keys,
				isUserRole,
				assignedMemberVkValue,
				apiKeyId: execApiKeyId,
			});
			const { execProvider, execModel } = resolveExecProviderAndModel({
				provider,
				model,
				committedProvider: committed?.provider,
				committedModel: committed?.model,
				execVkId,
				virtualKeys: virtualKeysData?.virtual_keys,
			});
			if (!execProvider || !execModel) {
				toast.error("Provider and Model Required", {
					description: "Please select a provider and model in Settings before executing tool calls.",
				});
				setIsStreaming(false);
				activeRunRef.current = null;
				return;
			}
			{
				const hasVk = String(execVkId).startsWith("sk-uf-");
				const hasApiKey = Boolean(execApiKeyId && execApiKeyId !== "__auto__" && !String(execApiKeyId).startsWith("sk-uf-"));
				if (!hasVk && !hasApiKey) {
					toast.error("Virtual Key or API Key Required", {
						description: "Select a Virtual Key (or a provider API key) in Settings before running.",
					});
					setIsStreaming(false);
					activeRunRef.current = null;
					return;
				}
			}
			setIsStreaming(true);
			await executePrompt(
				newMessages,
				undefined,
				{
					provider: execProvider,
					model: execModel,
					modelParams,
					apiKeyId: execApiKeyId,
					virtualKeyId: execVkId,
					variables,
					customHeaders,
					skillSystemPrompt,
					skillId,
					promptId: selectedPromptId ?? undefined,
					mcpIncludeTools: mcpIncludeTools ?? undefined,
				},
				{
					onStreamingStart: (allMessages, placeholder) => {
						if (!isActive()) return;
						setMessages([...allMessages, placeholder]);
					},
					onStreamChunk: (content) => {
						if (!isActive()) return;
						setMessages((prev) => {
							const updated = [...prev];
							const last = updated[updated.length - 1];
							const clone = last.clone();
							clone.content = content;
							updated[updated.length - 1] = clone;
							return updated;
						});
					},
					onComplete: async (content, usage) => {
						if (!isActive()) return;
						const finalMessages = [...newMessages, Message.response(content, 0, usage)];
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onToolCallComplete: async (content, toolCalls, usage) => {
						if (!isActive()) return;
						const finalMessages = [...newMessages, Message.toolCallResponse(content, toolCalls, 0, usage)];
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onEmptyResponse: () => {
						if (!isActive()) return;
						setMessages((prev) => prev.slice(0, -1));
					},
					onError: async (error) => {
						if (!isActive()) return;
						const finalMessages = [...newMessages, Message.error(error)];
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onFinally: () => {
						if (!isActive()) return;
						setIsStreaming(false);
						dispatch(baseApi.util.invalidateTags(["Logs", "MCPLogs", "VirtualKeys", "Sessions", "Budgets", "Teams", "Customers", "ProviderGovernance"]));
					},
				},
				abortController.signal,
			);
		},
		[messages, provider, model, modelParams, virtualKeyId, apiKeyId, allowedVirtualKeys, virtualKeysData, variables, customHeaders, skillSystemPrompt, skillId, mcpIncludeTools, selectedPromptId, selectedPrompt, isUserRole, assignedMemberVkValue, persistPlaygroundSession, dispatch],
	);

	const handleExecuteToolCall = useCallback(
		async (afterIndex: number, toolCall: ToolCall) => {
			try {
				const content = await executeToolCall(toolCall, { apiKeyId, virtualKeyId, customHeaders });
				await handleSubmitToolResult(afterIndex, toolCall.id, content);
			} catch (err) {
				if (err instanceof MCPAuthRequiredError) throw err;
				toast.error("Failed to execute tool", {
					description: getErrorMessage(err),
				});
			}
		},
		[apiKeyId, virtualKeyId, customHeaders, handleSubmitToolResult],
	);

	const handleSubmitAllToolResults = useCallback(
		async (afterIndex: number, results: { toolCallId: string; content: string }[]) => {
			if (results.length === 0) return;
			if (results.length === 1) {
				return handleSubmitToolResult(afterIndex, results[0].toolCallId, results[0].content);
			}

			const runToken = Symbol();
			activeRunRef.current = runToken;
			const abortController = new AbortController();
			abortRef.current = abortController;
			const isActive = () => activeRunRef.current === runToken;

			const newMessages = [...messages];
			let insertAt = afterIndex + 1;
			while (insertAt < newMessages.length && newMessages[insertAt].type === MessageType.ToolResult) {
				insertAt++;
			}
			for (const { toolCallId, content } of results) {
				const toolResultMsg = new Message(uuidv4(), 0, MessageType.ToolResult, {
					role: MessageRole.TOOL,
					content,
					tool_call_id: toolCallId,
				});
				newMessages.splice(insertAt, 0, toolResultMsg);
				insertAt++;
			}
			setMessages(newMessages);

			let execApiKeyId = apiKeyId;
			if ((!execApiKeyId || execApiKeyId === "__auto__") && allowedApiKeys && allowedApiKeys.length > 0) {
				execApiKeyId = allowedApiKeys[0];
			}
			const committed = selectedPrompt?.latest_version;
			const execVkId = resolveExecVirtualKey({
				virtualKeyId,
				allowedVirtualKeys,
				virtualKeys: virtualKeysData?.virtual_keys,
				isUserRole,
				assignedMemberVkValue,
				apiKeyId: execApiKeyId,
			});
			const { execProvider, execModel } = resolveExecProviderAndModel({
				provider,
				model,
				committedProvider: committed?.provider,
				committedModel: committed?.model,
				execVkId,
				virtualKeys: virtualKeysData?.virtual_keys,
			});
			if (!execProvider || !execModel) {
				toast.error("Provider and Model Required", {
					description: "Please select a provider and model in Settings before executing tool calls.",
				});
				setIsStreaming(false);
				activeRunRef.current = null;
				return;
			}
			{
				const hasVk = String(execVkId).startsWith("sk-uf-");
				const hasApiKey = Boolean(execApiKeyId && execApiKeyId !== "__auto__" && !String(execApiKeyId).startsWith("sk-uf-"));
				if (!hasVk && !hasApiKey) {
					toast.error("Virtual Key or API Key Required", {
						description: "Select a Virtual Key (or a provider API key) in Settings before running.",
					});
					setIsStreaming(false);
					activeRunRef.current = null;
					return;
				}
			}

			setIsStreaming(true);
			await executePrompt(
				newMessages,
				undefined,
				{
					provider: execProvider,
					model: execModel,
					modelParams,
					apiKeyId: execApiKeyId,
					virtualKeyId: execVkId,
					variables,
					customHeaders,
					skillSystemPrompt,
					skillId,
					promptId: selectedPromptId ?? undefined,
					mcpIncludeTools: mcpIncludeTools ?? undefined,
				},
				{
					onStreamingStart: (allMessages, placeholder) => {
						if (!isActive()) return;
						setMessages([...allMessages, placeholder]);
					},
					onStreamChunk: (content) => {
						if (!isActive()) return;
						setMessages((prev) => {
							const updated = [...prev];
							const last = updated[updated.length - 1];
							const clone = last.clone();
							clone.content = content;
							updated[updated.length - 1] = clone;
							return updated;
						});
					},
					onComplete: async (content, usage) => {
						if (!isActive()) return;
						const finalMessages = [...newMessages, Message.response(content, 0, usage)];
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onToolCallComplete: async (content, toolCalls, usage) => {
						if (!isActive()) return;
						const finalMessages = [...newMessages, Message.toolCallResponse(content, toolCalls, 0, usage)];
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onEmptyResponse: () => {
						if (!isActive()) return;
						setMessages((prev) => prev.slice(0, -1));
					},
					onError: async (error) => {
						if (!isActive()) return;
						const finalMessages = [...newMessages, Message.error(error)];
						setMessages(finalMessages);
						await persistPlaygroundSession(finalMessages);
					},
					onFinally: () => {
						if (!isActive()) return;
						setIsStreaming(false);
						dispatch(baseApi.util.invalidateTags(["Logs", "MCPLogs", "VirtualKeys", "Sessions", "Budgets", "Teams", "Customers", "ProviderGovernance"]));
					},
				},
				abortController.signal,
			);
		},
		[
			messages,
			provider,
			model,
			modelParams,
			virtualKeyId,
			apiKeyId,
			allowedVirtualKeys,
			virtualKeysData,
			variables,
			customHeaders,
			handleSubmitToolResult,
			skillSystemPrompt,
			skillId,
			mcpIncludeTools,
			selectedPromptId,
			selectedPrompt,
			isUserRole,
			assignedMemberVkValue,
			persistPlaygroundSession,
		],
	);

	const handleExecuteAllToolCalls = useCallback(
		async (afterIndex: number, toolCalls: ToolCall[]): Promise<{ toolCallId: string; content: string }[] | undefined> => {
			if (toolCalls.length === 0) return undefined;
			if (toolCalls.length === 1) {
				await handleExecuteToolCall(afterIndex, toolCalls[0]);
				return undefined;
			}

			const settled = await Promise.allSettled(
				toolCalls.map(async (tc) => {
					const content = await executeToolCall(tc, { apiKeyId, customHeaders });
					return { toolCallId: tc.id, content };
				}),
			);

			const successes = settled
				.filter((r): r is PromiseFulfilledResult<{ toolCallId: string; content: string }> => r.status === "fulfilled")
				.map((r) => r.value);

			const failures = settled.filter((r): r is PromiseRejectedResult => r.status === "rejected").map((r) => getErrorMessage(r.reason));

			if (failures.length > 0) {
				const detail = failures.length <= 3 ? failures.join("; ") : `${failures.slice(0, 2).join("; ")} and ${failures.length - 2} more`;
				toast.error(`${failures.length} of ${toolCalls.length} tool executions failed`, {
					description: failures.length === toolCalls.length ? detail : `${detail}. Successful results were kept — fill the rest manually.`,
				});
			}

			if (successes.length === toolCalls.length) {
				try {
					await handleSubmitAllToolResults(afterIndex, successes);
				} catch (err) {
					toast.error("Failed to submit tool results", {
						description: getErrorMessage(err),
					});
				}
				return undefined;
			}

			return successes.length > 0 ? successes : undefined;
		},
		[apiKeyId, customHeaders, handleExecuteToolCall, handleSubmitAllToolResults],
	);

	const handleStopStreaming = useCallback(() => {
		abortRef.current?.abort();
		abortRef.current = null;
		activeRunRef.current = null;
		setIsStreaming(false);
	}, []);

	const fetchToolResult = useCallback(
		async (toolCall: ToolCall): Promise<string> => {
			return executeToolCall(toolCall, { apiKeyId, customHeaders });
		},
		[apiKeyId, customHeaders],
	);

	const value: PromptContextValue = {
		folders,
		prompts,
		selectedPrompt,
		sessions,
		selectedSession,
		selectedVersion,
		foldersLoading,
		promptsLoading,
		foldersError,
		promptsError,
		isLoadingPlayground,
		playgroundError,
		isStreaming,
		selectedPromptId,
		selectedSessionId,
		selectedVersionId,
		setUrlState,
		messages,
		setMessages,
		provider,
		setProvider,
		model,
		setModel,
		modelParams,
		setModelParams,
		virtualKeyId,
		setVirtualKeyId,
		apiKeyId,
		setApiKeyId,
		skillId,
		setSkillId,
		mcpIncludeTools,
		setMcpIncludeTools,
		allowedProviders,
		setAllowedProviders,
		allowedModels,
		setAllowedModels,
		allowedVirtualKeys,
		setAllowedVirtualKeys,
		allowedApiKeys,
		setAllowedApiKeys,
		allowedSkills,
		setAllowedSkills,
		variables,
		setVariables,
		customHeaders,
		setCustomHeaders,
		requiredHeaders,
		folderSheet,
		setFolderSheet,
		promptSheet,
		setPromptSheet,
		commitSheet,
		setCommitSheet,
		deleteFolderDialog,
		setDeleteFolderDialog,
		deletePromptDialog,
		setDeletePromptDialog,
		promptAccessDialog,
		setPromptAccessDialog,
		promptFilters,
		setPromptFilters,
		isDeletingFolder,
		isDeletingPrompt,
		supportsVision,
		hasChanges,
		hasVersionChanges,
		hasSessionChanges,
		handleSelectPrompt,
		handleMovePrompt,
		handleMoveTeamFolder,
		handleMoveFolder,
		handleDeleteFolder,
		handleDeletePrompt,
		handleSendMessage,
		handleStopStreaming,
		handleSubmitToolResult,
		handleExecuteToolCall,
		handleSubmitAllToolResults,
		handleExecuteAllToolCalls,
		fetchToolResult,
		canCreate,
		canUpdate,
		canDelete,
		isSidebarOpen,
		setIsSidebarOpen,
		toggleSidebar,
		isSettingsOpen,
		setIsSettingsOpen,
		toggleSettings,
	};

	return <PromptContext.Provider value={value}>{children}</PromptContext.Provider>;
}