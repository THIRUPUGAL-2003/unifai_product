import React, { useState, useEffect, useMemo, useRef, useCallback } from "react";
import { parseAsStringLiteral, useQueryState } from "nuqs";
import {
	Globe,
	RefreshCw,
	Shield,
	ShieldCheck,
	Plus,
	Search,
	CheckCircle2,
	AlertTriangle,
	AlertCircle,
	Copy,
	Check,
	Eye,
	EyeOff,
	Download,
	ExternalLink,
	Activity,
	Terminal,
	FileText,
	ChevronLeft,
	ChevronRight,
	CornerDownRight,
	Trash2,
	Pencil,
	X,
	SlidersHorizontal,
	Zap,
	BrainCircuit,
	Radio,
	FileKey,
	Upload,
	Bot,
	Save,
	Paperclip,
	Loader2,
	Compass,
	ChevronDown,
	MoreHorizontal,
	PowerOff,
	Mail,
	Send,
	FileSpreadsheet,
	KeyRound,
} from "lucide-react";
import { getProviderLabel } from "@/lib/constants/logs";
import { useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdownMenu";
import { Textarea } from "@/components/ui/textarea";
import { normalizeTargetDomain, groupTargetsByParent, relatedHostsForDomain, relatedHostOptions, HOST_ROLE_OPTIONS, hostRoleLabel, type HostRole } from "./relatedHosts";
import { buildAttachmentPreview, type AttachmentPreviewKind, type AttachmentSheetPreview } from "./attachmentPreview";
import {
	GUARD_BOT_OLLAMA_PROVIDER,
	GUARD_BOT_OLLAMA_MODEL,
} from "./browserAiConstants";
import type { RelatedHostEntry } from "./browserAiTypes";
import {
	predictReasonLabel,
	logFileStatusLine,
	logExtractedTextFromPrompt,
	logExtractedText,
	isFileUploadLog,
	logHasStoredAttachment,
	logAttachmentLabel,
	logUserCaption,
	securityVerdictFromLog,
} from "./browserAiLogHelpers";
import {
	isDownloadGuardSource,
	guardRuleNoticeCopy,
	guardRuleActionHint,
	referenceImageDataUrl,
	readReferenceImageFile,
} from "./browserAiGuardHelpers";
import { ExportFormatsDropdown } from "@/components/exportFormatsDropdown";
import type { ExportFormatsPayload } from "@/components/exportFormatsDropdown";

import { useToast } from "@/hooks/use-toast";
import {
	useGetBrowserAiLogsQuery,
	useGetBrowserAiLogStatsQuery,
	useClearBrowserAiLogsMutation,
	useDeleteBrowserAiLogsMutation,
	useGetBrowserAiSearchLogsQuery,
	useClearBrowserAiSearchLogsMutation,
	useDeleteBrowserAiSearchLogsMutation,
	useGetBrowserAiRulesQuery,
	useCreateBrowserAiRuleMutation,
	useUpdateBrowserAiRuleMutation,
	useDeleteBrowserAiRuleMutation,
	useGenerateBrowserAiRegexFromPolicyMutation,
	useTestBrowserAiGuardBotMutation,
	useGetBrowserAiControlsQuery,
	useUpdateBrowserAiControlsMutation,
	useGetBrowserAiTargetsQuery,
	useCreateBrowserAiTargetMutation,
	useUpdateBrowserAiTargetMutation,
	useDeleteBrowserAiTargetMutation,
	useGetBrowserAiAgentsQuery,
	useGetBrowserAiSetupInfoQuery,
	useGetBrowserAiRebuildHistoryQuery,
	useGetBrowserAiAgentSettingsQuery,
	useSaveBrowserAiUninstallKeyMutation,
	useBulkDeleteBrowserAiAgentsMutation,
	useRemoteUninstallBrowserAiAgentMutation,
	useLazyGetBrowserAiAgentUninstallKeyQuery,
	useRotateBrowserAiAgentUninstallKeyMutation,
	useDeleteBrowserAiAgentMutation,
	useSendBrowserAiWarningEmailMutation,
	useGetBrowserAiInsightStatsQuery,
	useUpdateBrowserAiAgentContactEmailMutation,
	BrowserAILogEntry,
	BrowserAISearchLogEntry,
	BrowserGuardRule,
	BrowserControlSettings,
	BrowserTargetWebsite,
	BrowserAIAgent,
} from "@/lib/store/apis/browserAiApi";
import { getErrorMessage, useGetSMTPConfigQuery } from "@/lib/store";
import { getApiBaseUrl } from "@/lib/utils/port";
import { GuardRuleAIEvaluatorFields } from "./guardRuleAIEvaluatorFields";
import { logActionBadge, getPlatformBadge } from "./logBadges";
import { LogPromptPreviewCell } from "./logPromptPreviewCell";
import { formatLogDate, formatLogTime } from "./browserAiFormat";
import { RegexLiveTestPanel } from "./regexLiveTestPanel";
import { GuardRuleImportDialog, downloadGuardRulesTemplate } from "./guardRuleImportDialog";
import { TargetImportDialog, downloadTargetsTemplate } from "./targetImportDialog";

const BROWSER_AI_TABS = ["overview", "targets", "rules", "logs", "search-logs", "setup", "agents", "telemetry"] as const;
type BrowserAiTab = (typeof BROWSER_AI_TABS)[number];

const BROWSER_AI_TAB_TITLES: Record<BrowserAiTab, string> = {
	overview: "Overview",
	targets: "Target Websites",
	rules: "Guard Rules",
	logs: "Prompt Logs",
	"search-logs": "Search Logs",
	setup: "Setup",
	agents: "Guard Agents",
	telemetry: "Guard Insights",
};

/** Numeric dotted-version compare ("1.1.13" vs "1.1.9"); non-numeric parts count as 0. */
function compareGuardVersions(a: string, b: string): number {
	const pa = a.replace(/^v/i, "").split(".").map((n) => parseInt(n, 10) || 0);
	const pb = b.replace(/^v/i, "").split(".").map((n) => parseInt(n, 10) || 0);
	for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
		const diff = (pa[i] || 0) - (pb[i] || 0);
		if (diff !== 0) return diff;
	}
	return 0;
}

/** First Guard version that can switch to server-published Guard code. */
const PROXY_BUNDLE_MIN_GUARD = "1.1.14";

export default function BrowserAiPage() {
	const [tabParam, setTabParam] = useQueryState(
		"tab",
		parseAsStringLiteral(BROWSER_AI_TABS).withDefault("overview"),
	);
	const activeTab: BrowserAiTab = tabParam;
	const setActiveTab = useCallback(
		(value: string) => {
			const next = (BROWSER_AI_TABS as readonly string[]).includes(value)
				? (value as BrowserAiTab)
				: "overview";
			void setTabParam(next);
		},
		[setTabParam],
	);

	// Live updates & Polling control
	const [liveUpdatesEnabled, setLiveUpdatesEnabled] = useState(true);

	// Pagination & Filters state
	const [searchQuery, setSearchQuery] = useState("");
	const [selectedPlatform, setSelectedPlatform] = useState("all");
	const [selectedAction, setSelectedAction] = useState("all");
	const [pageLimit, setPageLimit] = useState(25);
	const [pageOffset, setPageOffset] = useState(0);

	const [ruleSearch, setRuleSearch] = useState("");
	const [rulesPageLimit, setRulesPageLimit] = useState(10);
	const [rulesPageOffset, setRulesPageOffset] = useState(0);
	const [targetSearch, setTargetSearch] = useState("");
	const [targetPageLimit, setTargetPageLimit] = useState(10);
	const [targetPageOffset, setTargetPageOffset] = useState(0);
	const [selectedLog, setSelectedLog] = useState<BrowserAILogEntry | null>(null);
	const [pdfViewerLog, setPdfViewerLog] = useState<BrowserAILogEntry | null>(null);
	const [pdfViewerTab, setPdfViewerTab] = useState<"preview" | "extracted" | "details">("preview");
	const [pdfBlobUrl, setPdfBlobUrl] = useState<string | null>(null);
	const [attachmentPreviewKind, setAttachmentPreviewKind] = useState<AttachmentPreviewKind | null>(null);
	const [attachmentPreviewHtml, setAttachmentPreviewHtml] = useState("");
	const [attachmentPreviewText, setAttachmentPreviewText] = useState("");
	const [attachmentBlob, setAttachmentBlob] = useState<Blob | null>(null);
	const [attachmentSheets, setAttachmentSheets] = useState<AttachmentSheetPreview[]>([]);
	const [attachmentSheetIndex, setAttachmentSheetIndex] = useState(0);
	const [attachmentTruncated, setAttachmentTruncated] = useState(false);
	const [attachmentShowAll, setAttachmentShowAll] = useState(false);
	const [extractedTextExpanded, setExtractedTextExpanded] = useState(false);
	const [pdfLoading, setPdfLoading] = useState(false);
	const [pdfError, setPdfError] = useState("");
	const [copiedPrompt, setCopiedPrompt] = useState(false);
	const [downloadingPlatform, setDownloadingPlatform] = useState<"windows" | "mac" | null>(null);
	const [rebuildingPackages, setRebuildingPackages] = useState(false);
	const [setupPackageError, setSetupPackageError] = useState("");
	const [uninstallKeyInput, setUninstallKeyInput] = useState("");
	const [uninstallKeyMessage, setUninstallKeyMessage] = useState("");
	const [uninstallKeyError, setUninstallKeyError] = useState("");
	// Plaintext company uninstall key display (stored in DB encrypted and cached in session/localStorage)
	const [savedUninstallKeyDisplay, setSavedUninstallKeyDisplay] = useState(() => {
		if (typeof window !== "undefined") {
			return localStorage.getItem("raksha_company_uninstall_key") || "";
		}
		return "";
	});
	const [uninstallKeyEditing, setUninstallKeyEditing] = useState(false);
	const [showUninstallKey, setShowUninstallKey] = useState(false);
	const [agentSearch, setAgentSearch] = useState("");
	const [agentStatusFilter, setAgentStatusFilter] = useState("all");
	const [agentTypeFilter, setAgentTypeFilter] = useState("all");
	const [agentPageOffset, setAgentPageOffset] = useState(0);
	const [agentPageLimit, setAgentPageLimit] = useState(25);
	const [selectedAgentIds, setSelectedAgentIds] = useState<Set<string>>(new Set());
	const [showAgentDeleteDialog, setShowAgentDeleteDialog] = useState(false);
	const [agentDeleteError, setAgentDeleteError] = useState("");
	const [agentBulkAction, setAgentBulkAction] = useState("");

	// Remote Uninstall / Turn Off Guard State
	const [remoteUninstallDialogOpen, setRemoteUninstallDialogOpen] = useState(false);
	const [targetAgentToUninstall, setTargetAgentToUninstall] = useState<BrowserAIAgent | null>(null);
	const [remoteUninstallKey, setRemoteUninstallKey] = useState("");
	const [remoteUninstallError, setRemoteUninstallError] = useState("");
	const [remoteUninstallSuccess, setRemoteUninstallSuccess] = useState("");
	const [showRemoteUninstallKey, setShowRemoteUninstallKey] = useState(false);

	// Telemetry Tab State
	const [telemetrySearch, setTelemetrySearch] = useState("");
	const [telemetryFilter, setTelemetryFilter] = useState<"all" | "allowed" | "blocked" | "warn" | "redact" | "active">("all");
	const [selectedTelemetryAgentId, setSelectedTelemetryAgentId] = useState<string | null>(null);

	// Dialog states
	const [ruleDialogOpen, setRuleDialogOpen] = useState(false);
	const [ruleImportDialogOpen, setRuleImportDialogOpen] = useState(false);
	const [targetDialogOpen, setTargetDialogOpen] = useState(false);
	const [targetImportDialogOpen, setTargetImportDialogOpen] = useState(false);
	const [ruleError, setRuleError] = useState("");
	const [targetError, setTargetError] = useState("");

	// New Rule Form
	const [newRuleName, setNewRuleName] = useState("");
	const [newRuleType, setNewRuleType] = useState<"regex" | "ai_bot">("regex");
	const [newRuleBotProvider, setNewRuleBotProvider] = useState(GUARD_BOT_OLLAMA_PROVIDER);
	const [newRuleBotModel, setNewRuleBotModel] = useState(GUARD_BOT_OLLAMA_MODEL);
	const [newRuleBotPrompt, setNewRuleBotPrompt] = useState("");
	const [newRuleBotReferenceImage, setNewRuleBotReferenceImage] = useState("");
	const [newRuleBotReferenceImageType, setNewRuleBotReferenceImageType] = useState("");
	const [newRuleBotReferenceImagePreview, setNewRuleBotReferenceImagePreview] = useState("");
	const [newRuleSeverity, setNewRuleSeverity] = useState<"CRITICAL" | "HIGH" | "MEDIUM">("CRITICAL");
	const [newRuleAction, setNewRuleAction] = useState<"BLOCK" | "REDACT" | "WARN">("BLOCK");
	const [newRulePattern, setNewRulePattern] = useState("");
	const [newRuleDescription, setNewRuleDescription] = useState("");
	const [newRuleWarningMessage, setNewRuleWarningMessage] = useState("");
	const [newRuleBotEvalMode, setNewRuleBotEvalMode] = useState<"ai" | "regex">("ai");
	const [newRuleGeneratedPattern, setNewRuleGeneratedPattern] = useState("");
	const [newRuleGenerateError, setNewRuleGenerateError] = useState("");

	// New Target Form
	const [newTargetDomain, setNewTargetDomain] = useState("");
	const [newTargetPlatform, setNewTargetPlatform] = useState("");
	const [newTargetHostRole, setNewTargetHostRole] = useState<HostRole>("ui");
	const [newTargetBlockSite, setNewTargetBlockSite] = useState(false);
	const [customRelatedHosts, setCustomRelatedHosts] = useState<RelatedHostEntry[]>([{ host: "", role: "" }]);
	const [extraHostDrafts, setExtraHostDrafts] = useState<Record<string, string>>({});
	const [extraHostRoleDrafts, setExtraHostRoleDrafts] = useState<Record<string, HostRole>>({});

	// Edit Target Form
	const [editTarget, setEditTarget] = useState<BrowserTargetWebsite | null>(null);
	const [editTargetDomain, setEditTargetDomain] = useState("");
	const [editTargetPlatform, setEditTargetPlatform] = useState("");
	const [editTargetBlockSite, setEditTargetBlockSite] = useState(false);
	const [editTargetHostRole, setEditTargetHostRole] = useState<HostRole>("");
	const [editTargetDialogOpen, setEditTargetDialogOpen] = useState(false);

	// Edit Rule Form
	const [editRule, setEditRule] = useState<BrowserGuardRule | null>(null);
	const [editRuleName, setEditRuleName] = useState("");
	const [editRuleType, setEditRuleType] = useState<"regex" | "ai_bot">("regex");
	const [editRuleBotProvider, setEditRuleBotProvider] = useState("");
	const [editRuleBotModel, setEditRuleBotModel] = useState("");
	const [editRuleBotPrompt, setEditRuleBotPrompt] = useState("");
	const [editRuleBotReferenceImage, setEditRuleBotReferenceImage] = useState("");
	const [editRuleBotReferenceImageType, setEditRuleBotReferenceImageType] = useState("");
	const [editRuleBotReferenceImagePreview, setEditRuleBotReferenceImagePreview] = useState("");
	const [editRuleSeverity, setEditRuleSeverity] = useState<"CRITICAL" | "HIGH" | "MEDIUM">("CRITICAL");
	const [editRuleAction, setEditRuleAction] = useState<"BLOCK" | "REDACT" | "WARN">("BLOCK");
	const [editRulePattern, setEditRulePattern] = useState("");
	const [editRuleDescription, setEditRuleDescription] = useState("");
	const [editRuleWarningMessage, setEditRuleWarningMessage] = useState("");
	const [editRuleDialogOpen, setEditRuleDialogOpen] = useState(false);
	const [editRuleBotEvalMode, setEditRuleBotEvalMode] = useState<"ai" | "regex">("ai");
	const [editRuleGeneratedPattern, setEditRuleGeneratedPattern] = useState("");
	const [editRuleGenerateError, setEditRuleGenerateError] = useState("");

	const activePolling = liveUpdatesEnabled ? 3000 : undefined;

	// --- Violation Notification State ---
	const [violationToasts, setViolationToasts] = useState<
		Array<{ id: string; platform: string; reason: string; prompt: string; time: string }>
	>([]);
	const seenBlockedIds = useRef<Set<string>>(new Set());
	const notifPermission = useRef<NotificationPermission>("default");

	// Request browser notification permission on mount
	useEffect(() => {
		if (typeof window !== "undefined" && "Notification" in window) {
			Notification.requestPermission().then((perm) => {
				notifPermission.current = perm;
			});
		}
	}, []);

	// RTK Queries with auto-polling for real-time live updates
	const {
		data: logsData,
		refetch: refetchLogs,
		isFetching: logsLoading,
	} = useGetBrowserAiLogsQuery(
		{
			platform: selectedPlatform !== "all" ? selectedPlatform : undefined,
			action: selectedAction !== "all" ? selectedAction : undefined,
			search: searchQuery || undefined,
			limit: pageLimit,
			offset: pageOffset,
		},
		{
			pollingInterval: activePolling,
			skip: activeTab !== "logs" && activeTab !== "overview",
		}
	);
	// Overview cards aggregate every log server-side, independent of the page/filters above.
	const { data: overviewStats } = useGetBrowserAiLogStatsQuery(undefined, {
		pollingInterval: activePolling,
		skip: activeTab !== "overview",
	});

	const { data: rulesData, refetch: refetchRules } = useGetBrowserAiRulesQuery(undefined, { pollingInterval: activePolling });
	const { data: targetsData, refetch: refetchTargets } = useGetBrowserAiTargetsQuery(undefined, { pollingInterval: activePolling });
	const { data: controlsData } = useGetBrowserAiControlsQuery(undefined, { pollingInterval: activePolling });
	const { data: providersData } = useGetProvidersQuery();
	// Outsource = configured Model Providers (OpenRouter, OpenAI, …). Download = Ollama on server.
	const outsourceProviderOptions = useMemo(() => {
		const opts = (providersData || [])
			.map((p) => String(p?.name || "").trim())
			.filter((name) => name && name.toLowerCase() !== GUARD_BOT_OLLAMA_PROVIDER)
			.map((name) => ({ label: getProviderLabel(name), value: name }));
		opts.sort((a, b) => a.label.localeCompare(b.label));
		return opts;
	}, [providersData]);
	const {
		data: agentsData,
		refetch: refetchAgents,
		isFetching: agentsLoading,
	} = useGetBrowserAiAgentsQuery(
		{
			status: agentStatusFilter !== "all" ? agentStatusFilter : undefined,
			agent_type: agentTypeFilter !== "all" ? agentTypeFilter : undefined,
			search: agentSearch || undefined,
			limit: agentPageLimit,
			offset: agentPageOffset,
		},
		{ pollingInterval: activePolling }
	);

	const { data: setupInfo, refetch: refetchSetupInfo } = useGetBrowserAiSetupInfoQuery(undefined, {
		skip: activeTab !== "setup" && activeTab !== "agents",
	});
	const { data: rebuildHistoryData, refetch: refetchRebuildHistory } = useGetBrowserAiRebuildHistoryQuery(20, {
		skip: activeTab !== "setup",
	});
	const rebuildHistory = rebuildHistoryData?.history || [];
	const latestWinVersion = agentsData?.latest_version || setupInfo?.version || "";
	const latestMacVersion = agentsData?.latest_mac_version || setupInfo?.mac_version || "";

	// Telemetry uses its own fleet-wide fetches — not Agents/Prompt-Logs pagination.
	const TELEMETRY_AGENT_LIMIT = 2000;
	const TELEMETRY_LOG_LIMIT = 5000;
	const telemetryTabActive = activeTab === "telemetry";
	const { data: telemetryAgentsRaw } = useGetBrowserAiAgentsQuery(
		{ limit: TELEMETRY_AGENT_LIMIT, offset: 0 },
		{ pollingInterval: activePolling, skip: !telemetryTabActive }
	);
	const { data: telemetryLogsRaw } = useGetBrowserAiLogsQuery(
		{ limit: TELEMETRY_LOG_LIMIT, offset: 0 },
		{ pollingInterval: activePolling, skip: !telemetryTabActive }
	);
	const { data: insightStatsRaw } = useGetBrowserAiInsightStatsQuery(undefined, {
		pollingInterval: activePolling,
		skip: !telemetryTabActive,
	});

	// Guard Insights: DB totals (full history) + recent logs for the detail drawer.
	const telemetryAgents = useMemo(() => {
		const agentsList = telemetryAgentsRaw?.agents || [];
		const logs = telemetryLogsRaw?.logs || [];
		const statsById = new Map((insightStatsRaw?.stats || []).map((s) => [s.agent_id, s] as const));

		return agentsList.map((agent) => {
			const matchingLogs = logs.filter((log) => {
				if (log.agent_id && log.agent_id === agent.id) return true;
				if (log.agent_hostname && agent.hostname && log.agent_hostname.toLowerCase() === agent.hostname.toLowerCase()) return true;
				if (log.client_ip && agent.ip_address && log.client_ip === agent.ip_address) return true;
				return false;
			});

			const db = statsById.get(agent.id);
			let allowedCount = db?.allowed_count ?? 0;
			let blockedCount = db?.blocked_count ?? 0;
			let warnCount = db?.warn_count ?? 0;
			let redactCount = db?.redact_count ?? 0;
			let totalHits = db?.total_count ?? 0;

			if (!db) {
				allowedCount = 0;
				blockedCount = 0;
				warnCount = 0;
				redactCount = 0;
				for (const log of matchingLogs) {
					const act = (log.action || "").toLowerCase();
					if (act.includes("block")) blockedCount++;
					else if (act.includes("redact")) redactCount++;
					else if (act.includes("warn")) warnCount++;
					else allowedCount++;
				}
				totalHits = matchingLogs.length;
			}

			const lastSeen = agent.last_seen_at ? new Date(agent.last_seen_at).getTime() : 0;
			const isOnline = Date.now() - lastSeen < 5 * 60 * 1000 && agent.status === "active";

			return {
				agent,
				matchingLogs,
				allowedCount,
				blockedCount,
				warnCount,
				redactCount,
				totalHits,
				isOnline,
			};
		});
	}, [telemetryAgentsRaw?.agents, telemetryLogsRaw?.logs, insightStatsRaw?.stats]);

	const filteredTelemetryAgents = useMemo(() => {
		const searchLower = telemetrySearch.toLowerCase().trim();
		return telemetryAgents.filter((item) => {
			const { agent, allowedCount, blockedCount, warnCount, redactCount, isOnline } = item;
			const matchSearch =
				!searchLower ||
				(agent.hostname || "").toLowerCase().includes(searchLower) ||
				(agent.username || "").toLowerCase().includes(searchLower) ||
				(agent.ip_address || "").toLowerCase().includes(searchLower) ||
				(agent.mac_address || "").toLowerCase().includes(searchLower) ||
				(agent.transport_name || "").toLowerCase().includes(searchLower) ||
				(agent.agent_version || "").toLowerCase().includes(searchLower);

			if (!matchSearch) return false;

			if (telemetryFilter === "allowed") return allowedCount > 0;
			if (telemetryFilter === "blocked") return blockedCount > 0;
			if (telemetryFilter === "warn") return warnCount > 0;
			if (telemetryFilter === "redact") return redactCount > 0;
			if (telemetryFilter === "active") return isOnline;

			return true;
		});
	}, [telemetryAgents, telemetrySearch, telemetryFilter]);

	const selectedTelemetryItem = useMemo(
		() => (selectedTelemetryAgentId ? telemetryAgents.find((t) => t.agent.id === selectedTelemetryAgentId) ?? null : null),
		[selectedTelemetryAgentId, telemetryAgents]
	);
	const selectedTelemetryAgent = selectedTelemetryItem?.agent ?? null;

	const telemetryTotals = useMemo(() => {
		const t = insightStatsRaw?.totals;
		if (t && (t.total || t.allowed || t.blocked || t.warn || t.redact)) {
			let totalActive = 0;
			for (const item of telemetryAgents) {
				if (item.isOnline) totalActive++;
			}
			return {
				totalAllowed: t.allowed || 0,
				totalBlocked: t.blocked || 0,
				totalWarn: t.warn || 0,
				totalRedact: t.redact || 0,
				totalActive,
			};
		}
		let totalAllowed = 0;
		let totalBlocked = 0;
		let totalWarn = 0;
		let totalRedact = 0;
		let totalActive = 0;

		for (const item of telemetryAgents) {
			totalAllowed += item.allowedCount;
			totalBlocked += item.blockedCount;
			totalWarn += item.warnCount;
			totalRedact += item.redactCount;
			if (item.isOnline) totalActive++;
		}

		return { totalAllowed, totalBlocked, totalWarn, totalRedact, totalActive };
	}, [telemetryAgents, insightStatsRaw?.totals]);

	// Search Logs State & Query (in-memory live observability)
	const [searchLogQuery, setSearchLogQuery] = useState("");
	const [searchEngineFilter, setSearchEngineFilter] = useState("all");
	const [searchBrowserFilter, setSearchBrowserFilter] = useState("all");
	const [searchIncognitoFilter, setSearchIncognitoFilter] = useState("all");
	const [selectedSearchLog, setSelectedSearchLog] = useState<BrowserAISearchLogEntry | null>(null);
	const [searchLogPageLimit, setSearchLogPageLimit] = useState(25);
	const [searchLogPageOffset, setSearchLogPageOffset] = useState(0);

	const {
		data: searchLogsData,
		refetch: refetchSearchLogs,
		isFetching: searchLogsLoading,
	} = useGetBrowserAiSearchLogsQuery(
		{
			engine: searchEngineFilter !== "all" ? searchEngineFilter : undefined,
			browser: searchBrowserFilter !== "all" ? searchBrowserFilter : undefined,
			is_incognito: searchIncognitoFilter !== "all" ? searchIncognitoFilter : undefined,
			search: searchLogQuery || undefined,
			limit: searchLogPageLimit,
			offset: searchLogPageOffset,
		},
		{
			pollingInterval: activePolling,
			skip: activeTab !== "search-logs",
		}
	);
	const searchLogs = searchLogsData?.logs || [];
	const totalSearchLogs = searchLogsData?.total || 0;
	const incognitoSearchCount = searchLogsData?.incognito_count || 0;
	const queriesSearchCount = searchLogsData?.queries_count || 0;
	const clicksSearchCount = searchLogsData?.clicks_count || 0;

	const { data: agentSettingsData, refetch: refetchAgentSettings } = useGetBrowserAiAgentSettingsQuery();
	useEffect(() => {
		if (agentSettingsData?.uninstall_key) {
			setSavedUninstallKeyDisplay(agentSettingsData.uninstall_key);
			if (typeof window !== "undefined") {
				localStorage.setItem("raksha_company_uninstall_key", agentSettingsData.uninstall_key);
			}
		}
	}, [agentSettingsData?.uninstall_key]);
	const [saveUninstallKey, { isLoading: savingUninstallKey }] = useSaveBrowserAiUninstallKeyMutation();
	const [clearBrowserAiLogs, { isLoading: isClearingLogs }] = useClearBrowserAiLogsMutation();
	const [clearBrowserAiSearchLogs, { isLoading: isClearingSearchLogs }] = useClearBrowserAiSearchLogsMutation();
	const [clearLogsDialogOpen, setClearLogsDialogOpen] = useState(false);
	const [clearSearchLogsDialogOpen, setClearSearchLogsDialogOpen] = useState(false);
	const [clearLogsRetention, setClearLogsRetention] = useState<"all" | "1d" | "7d" | "30d">("all");
	const [clearSearchLogsRetention, setClearSearchLogsRetention] = useState<"all" | "1d" | "7d" | "30d">("all");

	const [deleteBrowserAiLogs, { isLoading: isDeletingSelectedLogs }] = useDeleteBrowserAiLogsMutation();
	const [deleteBrowserAiSearchLogs, { isLoading: isDeletingSelectedSearchLogs }] = useDeleteBrowserAiSearchLogsMutation();
	const [selectedLogIds, setSelectedLogIds] = useState<Set<string>>(() => new Set());
	const [selectedSearchLogIds, setSelectedSearchLogIds] = useState<Set<string>>(() => new Set());
	const [deleteSelectedTarget, setDeleteSelectedTarget] = useState<"logs" | "search-logs" | null>(null);

	// Selection only spans the visible page; drop IDs that scrolled away or were deleted.
	useEffect(() => {
		setSelectedLogIds((prev) => {
			if (prev.size === 0) return prev;
			const visible = new Set((logsData?.logs || []).map((l) => l.id));
			const next = new Set([...prev].filter((id) => visible.has(id)));
			return next.size === prev.size ? prev : next;
		});
	}, [logsData?.logs]);
	useEffect(() => {
		setSelectedSearchLogIds((prev) => {
			if (prev.size === 0) return prev;
			const visible = new Set((searchLogsData?.logs || []).map((l) => l.id));
			const next = new Set([...prev].filter((id) => visible.has(id)));
			return next.size === prev.size ? prev : next;
		});
	}, [searchLogsData?.logs]);

	const toggleId = (setter: React.Dispatch<React.SetStateAction<Set<string>>>, id: string, on: boolean) => {
		setter((prev) => {
			const next = new Set(prev);
			if (on) next.add(id);
			else next.delete(id);
			return next;
		});
	};

	const handleDeleteSelected = async () => {
		const target = deleteSelectedTarget;
		if (!target) return;
		const ids = [...(target === "logs" ? selectedLogIds : selectedSearchLogIds)];
		if (ids.length === 0) {
			setDeleteSelectedTarget(null);
			return;
		}
		try {
			const res =
				target === "logs" ? await deleteBrowserAiLogs({ ids }).unwrap() : await deleteBrowserAiSearchLogs({ ids }).unwrap();
			const label = target === "logs" ? "prompt" : "search";
			toast({
				title: `Deleted ${res?.deleted ?? ids.length} ${label} log${(res?.deleted ?? ids.length) === 1 ? "" : "s"}`,
			});
			if (target === "logs") {
				setSelectedLogIds(new Set());
				setPageOffset(0);
			} else {
				setSelectedSearchLogIds(new Set());
				setSearchLogPageOffset(0);
			}
			setDeleteSelectedTarget(null);
		} catch (err) {
			toast({
				title: "Failed to delete logs",
				description: getErrorMessage(err),
				variant: "destructive",
			});
		}
	};

	const handleClearLogs = async () => {
		try {
			await clearBrowserAiLogs({ period: clearLogsRetention }).unwrap();
			toast({
				title: "Prompt logs cleared",
				description:
					clearLogsRetention === "all"
						? "All prompt logs have been cleared."
						: `Prompt logs older than ${clearLogsRetention} have been cleared.`,
			});
			setClearLogsDialogOpen(false);
			setSelectedLogIds(new Set());
			setPageOffset(0);
			refetchLogs();
		} catch (err) {
			toast({
				title: "Failed to clear prompt logs",
				description: getErrorMessage(err),
				variant: "destructive",
			});
		}
	};

	const handleClearSearchLogs = async () => {
		try {
			await clearBrowserAiSearchLogs({ period: clearSearchLogsRetention }).unwrap();
			toast({
				title: "Search logs cleared",
				description:
					clearSearchLogsRetention === "all"
						? "All search logs have been cleared."
						: `Search logs older than ${clearSearchLogsRetention} have been cleared.`,
			});
			setClearSearchLogsDialogOpen(false);
			setSelectedSearchLogIds(new Set());
			setSearchLogPageOffset(0);
			refetchSearchLogs();
		} catch (err) {
			toast({
				title: "Failed to clear search logs",
				description: getErrorMessage(err),
				variant: "destructive",
			});
		}
	};

	const controls: BrowserControlSettings = controlsData?.controls || {
		id: "browser-controls-default",
		enabled: true,
		block_upload: false,
		upload_warning: "",
		search_log_auto_delete: false,
		search_log_retention: "7d",
		prompt_log_auto_delete: false,
		prompt_log_retention: "7d",
	};

	const { toast } = useToast();
	const [sendWarningEmail, { isLoading: isSendingWarningEmail }] = useSendBrowserAiWarningEmailMutation();
	const { data: smtpConfig } = useGetSMTPConfigQuery();
	const smtpReady = !!(smtpConfig?.enabled && (smtpConfig.host || "").trim());
	const [warningMailTarget, setWarningMailTarget] = useState<BrowserAIAgent | null>(null);
	const [warningMailTo, setWarningMailTo] = useState("");
	const [warningMailSubject, setWarningMailSubject] = useState("");
	const [warningMailMessage, setWarningMailMessage] = useState("");
	const [warningMailError, setWarningMailError] = useState("");
	const [warningMailStats, setWarningMailStats] = useState<{
		allowed: number;
		blocked: number;
		warn: number;
		redact: number;
	} | null>(null);

	const handleOpenWarningMail = (
		agent: BrowserAIAgent,
		stats: { allowed: number; blocked: number; warn: number; redact: number }
	) => {
		setWarningMailTarget(agent);
		setWarningMailStats(stats);
		setWarningMailTo(
			(agent.contact_email && agent.contact_email.includes("@")
				? agent.contact_email
				: agent.username && agent.username.includes("@")
					? agent.username
					: "") || ""
		);
		setWarningMailSubject(`[Security Alert] Raksha Browser Guard Policy Warning — ${agent.hostname || "Device"}`);
		const host = agent.hostname || "unknown-device";
		const user = agent.username || "—";
		const ip = agent.ip_address || "—";
		setWarningMailMessage(
			`Dear Employee,\n\n` +
				`This is an official Raksha Browser Guard security compliance report for your assigned workstation.\n\n` +
				`--- DEVICE REPORT ---\n` +
				`Hostname: ${host}\n` +
				`User: ${user}\n` +
				`IP Address: ${ip}\n` +
				`Agent ID: ${agent.id}\n\n` +
				`--- VIOLATION SUMMARY ---\n` +
				`Allowed: ${stats.allowed}\n` +
				`Blocked: ${stats.blocked}\n` +
				`Warned: ${stats.warn}\n` +
				`Redacted: ${stats.redact}\n\n` +
				`Please be reminded that inputting sensitive enterprise credentials, customer data, source code, or proprietary documents into unapproved external AI platforms is strictly prohibited under company data security policy.\n\n` +
				`All browser sessions and AI interactions are continuously monitored. Continued policy violations will be escalated to Information Security Management.\n\n` +
				`If you believe this alert was received in error, please contact your IT Security Administrator.`
		);
		setWarningMailError("");
	};

	const handleSendWarningEmail = async () => {
		const to = warningMailTo.trim();
		if (!to) {
			setWarningMailError("Recipient email address is required.");
			return;
		}
		if (!to.includes("@") || !to.includes(".")) {
			setWarningMailError("Enter a valid recipient email address (example: employee@company.com).");
			return;
		}
		if (!warningMailMessage.trim()) {
			setWarningMailError("Warning message cannot be empty.");
			return;
		}
		if (!smtpReady) {
			setWarningMailError(
				"SMTP is not enabled. Configure and enable SMTP in Settings → Security, then retry."
			);
			return;
		}
		setWarningMailError("");
		try {
			const res = await sendWarningEmail({
				to,
				subject: warningMailSubject.trim() || "Raksha Security Policy Warning",
				message: warningMailMessage.trim(),
				agent_id: warningMailTarget?.id,
				agent_hostname: warningMailTarget?.hostname,
				allowed_count: warningMailStats?.allowed,
				blocked_count: warningMailStats?.blocked,
				warn_count: warningMailStats?.warn,
				redact_count: warningMailStats?.redact,
			}).unwrap();
			toast({
				title: "Warning Email Sent",
				description: `Security warning report delivered to ${res.to || to}.`,
			});
			setWarningMailTarget(null);
		} catch (err: unknown) {
			setWarningMailError(
				getErrorMessage(err) ||
					"Failed to send warning email. Please ensure SMTP is configured and enabled in Settings → Security."
			);
		}
	};
	const [uploadWarningDraft, setUploadWarningDraft] = useState("");
	const [uploadWarningEditing, setUploadWarningEditing] = useState(false);
	const [uploadWarningSaving, setUploadWarningSaving] = useState(false);
	const [uploadWarningError, setUploadWarningError] = useState("");
	useEffect(() => {
		if (!uploadWarningEditing) {
			setUploadWarningDraft(controls.upload_warning || "");
		}
	}, [controls.upload_warning, uploadWarningEditing]);
	const agents = agentsData?.agents || [];
	const totalAgents = agentsData?.total || 0;
	const activeAgentsCount = agentsData?.active_count ?? agents.filter((a) => a.status === "active").length;
	const uninstalledAgentsCount = agentsData?.uninstalled_count ?? agents.filter((a) => a.status === "uninstalled").length;
	const agentSettings = agentSettingsData?.settings;
	const visibleAgentIds = useMemo(() => agents.map((a) => a.id), [agents]);
	const selectedVisibleAgentIds = useMemo(
		() => visibleAgentIds.filter((id) => selectedAgentIds.has(id)),
		[selectedAgentIds, visibleAgentIds],
	);
	const selectedAgentCount = selectedAgentIds.size;
	const allVisibleAgentsSelected = visibleAgentIds.length > 0 && selectedVisibleAgentIds.length === visibleAgentIds.length;
	const someVisibleAgentsSelected = selectedVisibleAgentIds.length > 0 && selectedVisibleAgentIds.length < visibleAgentIds.length;

	useEffect(() => {
		setSelectedAgentIds(new Set());
		setAgentBulkAction("");
	}, [agentPageOffset, agentSearch, agentStatusFilter, agentTypeFilter]);

	useEffect(() => {
		setSearchLogPageOffset(0);
	}, [searchLogQuery, searchEngineFilter, searchBrowserFilter, searchIncognitoFilter]);

	useEffect(() => {
		setAgentPageOffset(0);
	}, [agentSearch, agentStatusFilter, agentTypeFilter, agentPageLimit]);

	// --- Detect new blocked violations and fire notifications ---
	useEffect(() => {
		if (!logsData?.logs) return;
		const newBlocked = logsData.logs.filter(
			(l) => l.action === "Blocked" && l.id && !seenBlockedIds.current.has(l.id)
		);
		if (newBlocked.length === 0) return;
		newBlocked.forEach((log) => {
			seenBlockedIds.current.add(log.id);
			const toastId = log.id;
			const platform = log.platform || "AI Platform";
			const reason = log.rule_triggered || "Security Rule Violation";
			const prompt = log.user_prompt_full?.slice(0, 60) || log.risk_score?.toString() || "";
			const time = new Date().toLocaleTimeString();

			// Show browser system notification
			if (typeof window !== "undefined" && "Notification" in window && notifPermission.current === "granted") {
				try {
					new Notification("🚨 AI Guard: Security Violation Blocked!", {
						body: `[${platform}] ${reason}`,
						icon: "/yes-panchi-logo.png",
						tag: toastId,
					});
				} catch { }
			}

			// Show in-page toast
			setViolationToasts((prev) => [
				{ id: toastId, platform, reason, prompt, time },
				...prev.slice(0, 4),
			]);
			// Auto-dismiss after 8 seconds
			setTimeout(() => {
				setViolationToasts((prev) => prev.filter((t) => t.id !== toastId));
			}, 8000);
		});
	}, [logsData]);

	useEffect(() => {
		if (!pdfViewerLog?.id || !logHasStoredAttachment(pdfViewerLog)) {
			if (pdfBlobUrl) {
				URL.revokeObjectURL(pdfBlobUrl);
				setPdfBlobUrl(null);
			}
			setAttachmentPreviewKind(null);
			setAttachmentPreviewHtml("");
			setAttachmentPreviewText("");
			setAttachmentBlob(null);
			setAttachmentSheets([]);
			setAttachmentSheetIndex(0);
			setAttachmentTruncated(false);
			setAttachmentShowAll(false);
			setPdfLoading(false);
			setPdfError("");
			return;
		}
		let cancelled = false;
		let objectUrl: string | null = null;
		setPdfLoading(true);
		setPdfError("");
		setAttachmentPreviewKind(null);
		setAttachmentPreviewHtml("");
		setAttachmentPreviewText("");
		setAttachmentBlob(null);
		setAttachmentSheets([]);
		setAttachmentSheetIndex(0);
		setAttachmentTruncated(false);
		setAttachmentShowAll(false);
		if (pdfBlobUrl) {
			URL.revokeObjectURL(pdfBlobUrl);
			setPdfBlobUrl(null);
		}
		(async () => {
			try {
				const res = await fetch(`${getApiBaseUrl()}/browser-ai/attachments/${encodeURIComponent(pdfViewerLog.id)}`, {
					credentials: "include",
				});
				if (!res.ok) {
					if (res.status === 410) {
						throw new Error("File expired (kept 10 minutes for View). Log and filename remain.");
					}
					throw new Error(res.status === 404 ? "File not found on server" : `Failed to load file (${res.status})`);
				}
				const blob = await res.blob();
				if (cancelled) return;
				setAttachmentBlob(blob);
				const preview = await buildAttachmentPreview(
					blob,
					logAttachmentLabel(pdfViewerLog),
					pdfViewerLog.attachment_content_type || blob.type,
					{ showAll: false },
				);
				if (cancelled) return;
				setAttachmentPreviewKind(preview.kind);
				setAttachmentSheets(preview.sheets || []);
				setAttachmentSheetIndex(0);
				setAttachmentTruncated(!!preview.truncated);
				if (preview.blobUrl) {
					objectUrl = preview.blobUrl;
					setPdfBlobUrl(preview.blobUrl);
				}
				if (preview.sheets?.length) {
					setAttachmentPreviewHtml(preview.sheets[0].html);
				} else if (preview.html) {
					setAttachmentPreviewHtml(preview.html);
				}
				if (preview.text) setAttachmentPreviewText(preview.text);
				if (preview.kind === "unsupported") {
					objectUrl = URL.createObjectURL(blob);
					setPdfBlobUrl(objectUrl);
				}
			} catch (e) {
				if (!cancelled) {
					setPdfError(e instanceof Error ? e.message : "Failed to load file");
					setPdfBlobUrl(null);
					setAttachmentPreviewKind(null);
				}
			} finally {
				if (!cancelled) setPdfLoading(false);
			}
		})();
		return () => {
			cancelled = true;
			if (objectUrl) URL.revokeObjectURL(objectUrl);
		};
		// eslint-disable-next-line react-hooks/exhaustive-deps -- reload only when log id changes
	}, [pdfViewerLog?.id]);

	const reloadAttachmentPreview = async (showAll: boolean) => {
		if (!pdfViewerLog || !attachmentBlob) return;
		setPdfLoading(true);
		setPdfError("");
		try {
			const preview = await buildAttachmentPreview(
				attachmentBlob,
				logAttachmentLabel(pdfViewerLog),
				pdfViewerLog.attachment_content_type || attachmentBlob.type,
				{ showAll },
			);
			setAttachmentShowAll(showAll);
			setAttachmentPreviewKind(preview.kind);
			setAttachmentSheets(preview.sheets || []);
			setAttachmentSheetIndex(0);
			setAttachmentTruncated(!!preview.truncated);
			if (preview.sheets?.length) {
				setAttachmentPreviewHtml(preview.sheets[0].html);
			} else if (preview.html) {
				setAttachmentPreviewHtml(preview.html);
			}
			if (preview.text) setAttachmentPreviewText(preview.text);
		} catch (e) {
			setPdfError(e instanceof Error ? e.message : "Failed to expand preview");
		} finally {
			setPdfLoading(false);
		}
	};

	const openPdfViewer = (log: BrowserAILogEntry, e?: React.MouseEvent) => {
		e?.stopPropagation();
		setPdfViewerTab("preview");
		setAttachmentShowAll(false);
		setPdfViewerLog(log);
	};

	const downloadPdfAttachment = async (log: BrowserAILogEntry) => {
		try {
			const res = await fetch(
				`${getApiBaseUrl()}/browser-ai/attachments/${encodeURIComponent(log.id)}?download=1`,
				{ credentials: "include" },
			);
			if (!res.ok) {
				if (res.status === 410) {
					setPdfError("File expired (kept 10 minutes for View). Log and filename remain.");
					return;
				}
				throw new Error("Download failed");
			}
			const blob = await res.blob();
			const url = URL.createObjectURL(blob);
			const a = document.createElement("a");
			a.href = url;
			a.download = logAttachmentLabel(log);
			document.body.appendChild(a);
			a.click();
			a.remove();
			URL.revokeObjectURL(url);
		} catch {
			setPdfError("Download failed");
		}
	};

	const [createRule] = useCreateBrowserAiRuleMutation();
	const [updateRule] = useUpdateBrowserAiRuleMutation();
	const [deleteRule] = useDeleteBrowserAiRuleMutation();
	const [generateRegexFromPolicy, { isLoading: generatingRegex }] = useGenerateBrowserAiRegexFromPolicyMutation();
	const [testGuardBot, { isLoading: testingGuardBot }] = useTestBrowserAiGuardBotMutation();
	const [newRuleTestSample, setNewRuleTestSample] = useState("");
	const [newRuleTestResult, setNewRuleTestResult] = useState("");
	const [editRuleTestSample, setEditRuleTestSample] = useState("");
	const [editRuleTestResult, setEditRuleTestResult] = useState("");
	const [updateControls] = useUpdateBrowserAiControlsMutation();
	const [createTarget] = useCreateBrowserAiTargetMutation();
	const [updateTarget] = useUpdateBrowserAiTargetMutation();
	const [deleteTarget] = useDeleteBrowserAiTargetMutation();
	const [bulkDeleteAgents, { isLoading: deletingAgents }] = useBulkDeleteBrowserAiAgentsMutation();
	const [remoteUninstallAgent, { isLoading: isRemoteUninstalling }] = useRemoteUninstallBrowserAiAgentMutation();
	const [fetchAgentUninstallKey] = useLazyGetBrowserAiAgentUninstallKeyQuery();
	const [rotateAgentUninstallKey, { isLoading: isRotatingGuardKey }] = useRotateBrowserAiAgentUninstallKeyMutation();
	const [deleteSingleAgent] = useDeleteBrowserAiAgentMutation();
	const [updateAgentContactEmail, { isLoading: isUpdatingContactEmail }] = useUpdateBrowserAiAgentContactEmailMutation();
	const [guardKeyLoading, setGuardKeyLoading] = useState(false);
	const [guardKeyHint, setGuardKeyHint] = useState("");

	const [selectedAgentDetails, setSelectedAgentDetails] = useState<BrowserAIAgent | null>(null);
	const [agentDetailsKey, setAgentDetailsKey] = useState<string>("");
	const [showAgentDetailsKey, setShowAgentDetailsKey] = useState<boolean>(false);
	const [agentDetailsKeyCopied, setAgentDetailsKeyCopied] = useState<boolean>(false);
	const [agentDetailsKeyLoading, setAgentDetailsKeyLoading] = useState<boolean>(false);
	const [agentDetailsKeyRotatedAt, setAgentDetailsKeyRotatedAt] = useState<string>("");
	const [editingContactEmail, setEditingContactEmail] = useState<string>("");
	const [contactEmailSuccess, setContactEmailSuccess] = useState<string>("");
	const [contactEmailError, setContactEmailError] = useState<string>("");

	const handleOpenAgentDetails = async (agent: BrowserAIAgent) => {
		setSelectedAgentDetails(agent);
		setAgentDetailsKey("");
		setShowAgentDetailsKey(false);
		setAgentDetailsKeyCopied(false);
		setAgentDetailsKeyRotatedAt(agent.uninstall_key_rotated_at || "");
		setEditingContactEmail(agent.contact_email || "");
		setContactEmailSuccess("");
		setContactEmailError("");
		setAgentDetailsKeyLoading(true);
		try {
			const res = await fetchAgentUninstallKey(agent.id).unwrap();
			if (res?.uninstall_key) {
				setAgentDetailsKey(res.uninstall_key);
				if (res.uninstall_key_rotated_at) {
					setAgentDetailsKeyRotatedAt(res.uninstall_key_rotated_at);
				}
			}
		} catch {
			// ignore
		} finally {
			setAgentDetailsKeyLoading(false);
		}
	};

	const handleRotateDetailsKey = async () => {
		if (!selectedAgentDetails) return;
		if (!confirm(`Force rotate daily uninstall key for ${selectedAgentDetails.hostname || selectedAgentDetails.id} now? Old key will immediately stop working.`)) return;
		try {
			const res = await rotateAgentUninstallKey(selectedAgentDetails.id).unwrap();
			if (res?.uninstall_key) {
				setAgentDetailsKey(res.uninstall_key);
				if (res.uninstall_key_rotated_at) {
					setAgentDetailsKeyRotatedAt(res.uninstall_key_rotated_at);
				}
				await navigator.clipboard.writeText(res.uninstall_key);
				setAgentDetailsKeyCopied(true);
				setTimeout(() => setAgentDetailsKeyCopied(false), 2000);
				toast({
					title: "Key Rotated",
					description: `New daily uninstall key generated and copied to clipboard.`,
				});
			}
			refetchAgents();
		} catch (err: any) {
			toast({
				variant: "destructive",
				title: "Rotation Failed",
				description: getErrorMessage(err) || "Failed to rotate Guard uninstall key",
			});
		}
	};

	const handleSaveContactEmail = async (override?: string) => {
		if (!selectedAgentDetails) return;
		const email = (override ?? editingContactEmail).trim();
		setContactEmailError("");
		setContactEmailSuccess("");
		if (email && (!email.includes("@") || !email.includes("."))) {
			setContactEmailError("Please enter a valid email address.");
			return;
		}
		try {
			await updateAgentContactEmail({ id: selectedAgentDetails.id, contact_email: email }).unwrap();
			setEditingContactEmail(email);
			setContactEmailSuccess(
				email ? "Contact email saved. It stays until you edit or remove it." : "Contact email removed.",
			);
			setSelectedAgentDetails((prev) => (prev ? { ...prev, contact_email: email, contact_email_pinned: true } : null));
			refetchAgents();
		} catch (err: any) {
			const data = err?.data;
			setContactEmailError(
				(typeof data?.error === "object" && data?.error?.message) ||
					(typeof data?.error === "string" ? data.error : "") ||
					data?.message ||
					err?.message ||
					"Failed to update contact email",
			);
		}
	};

	const handleOpenRemoteUninstall = async (agent: BrowserAIAgent) => {
		setTargetAgentToUninstall(agent);
		setRemoteUninstallKey("");
		setRemoteUninstallError("");
		setRemoteUninstallSuccess("");
		setShowRemoteUninstallKey(false);
		setGuardKeyHint("");
		setRemoteUninstallDialogOpen(true);
		setGuardKeyLoading(true);
		try {
			const res = await fetchAgentUninstallKey(agent.id).unwrap();
			if (res?.uninstall_key) {
				setRemoteUninstallKey(res.uninstall_key);
				const rotTime = res.uninstall_key_rotated_at
					? ` (issued ${new Date(res.uninstall_key_rotated_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })})`
					: "";
				setGuardKeyHint(`Today's auto-rotating daily uninstall key (rotates every 24h${rotTime}; company key also works).`);
			}
		} catch {
			setGuardKeyHint("Could not load this Guard's key — enter today's Guard key or company uninstall key.");
		} finally {
			setGuardKeyLoading(false);
		}
	};

	const handleCopyGuardUninstallKey = async (agent: BrowserAIAgent) => {
		try {
			const res = await fetchAgentUninstallKey(agent.id).unwrap();
			if (res?.uninstall_key) {
				await navigator.clipboard.writeText(res.uninstall_key);
				toast({
					title: "Copied",
					description: `Today's daily Guard uninstall key copied to clipboard for ${res.hostname || res.agent_id}.`,
				});
			}
		} catch (err: any) {
			toast({
				variant: "destructive",
				title: "Error",
				description: getErrorMessage(err) || "Failed to load Guard uninstall key",
			});
		}
	};

	const handleRotateGuardUninstallKey = async (agent: BrowserAIAgent) => {
		if (!confirm(`Manually rotate uninstall key for ${agent.hostname || agent.id} now? Note: Guard keys auto-rotate daily every 24h, but you can force rotate immediately.`)) return;
		try {
			const res = await rotateAgentUninstallKey(agent.id).unwrap();
			if (res?.uninstall_key) {
				await navigator.clipboard.writeText(res.uninstall_key);
				toast({
					title: "Key Rotated",
					description: `New daily Guard uninstall key generated and copied to clipboard for ${res.hostname || res.agent_id}.`,
				});
			}
			refetchAgents();
		} catch (err: any) {
			toast({
				variant: "destructive",
				title: "Error",
				description: getErrorMessage(err) || "Failed to rotate Guard uninstall key",
			});
		}
	};

	const handleConfirmRemoteUninstall = async () => {
		if (!targetAgentToUninstall) return;
		const key = remoteUninstallKey.trim();
		if (!key) {
			setRemoteUninstallError("This Guard's uninstall key (or company key) is required.");
			return;
		}
		setRemoteUninstallError("");
		setRemoteUninstallSuccess("");
		try {
			await remoteUninstallAgent({ id: targetAgentToUninstall.id, key }).unwrap();
			setRemoteUninstallSuccess(
				`Remote shutdown signal sent to ${targetAgentToUninstall.hostname || targetAgentToUninstall.id}. Guard will stop on next heartbeat (~15–30s).`
			);
			refetchAgents();
			setTimeout(() => {
				setRemoteUninstallDialogOpen(false);
				setTargetAgentToUninstall(null);
				setRemoteUninstallSuccess("");
				setRemoteUninstallKey("");
			}, 2500);
		} catch (err: any) {
			setRemoteUninstallError(
				getErrorMessage(err) || "Failed to send remote uninstall. Use this Guard's key or the company uninstall key."
			);
		}
	};

	const handleDeleteSingleAgent = async (agent: BrowserAIAgent) => {
		if (!confirm(`Remove ${agent.hostname || agent.id} from this dashboard fleet list?`)) return;
		try {
			await deleteSingleAgent(agent.id).unwrap();
			refetchAgents();
		} catch (err: any) {
			toast({
				variant: "destructive",
				title: "Error",
				description: err?.data?.message || err?.message || "Failed to remove agent",
			});
		}
	};

	const toggleSelectAllVisibleAgents = (checked: boolean) => {
		setSelectedAgentIds((prev) => {
			const next = new Set(prev);
			for (const id of visibleAgentIds) {
				if (checked) {
					next.add(id);
				} else {
					next.delete(id);
				}
			}
			return next;
		});
	};

	const toggleSelectAgent = (agentId: string, checked: boolean) => {
		setSelectedAgentIds((prev) => {
			const next = new Set(prev);
			if (checked) {
				next.add(agentId);
			} else {
				next.delete(agentId);
			}
			return next;
		});
	};

	const handleAgentBulkAction = (action: string) => {
		setAgentBulkAction(action);
		if (action === "delete") {
			if (selectedAgentCount === 0) {
				setAgentBulkAction("");
				return;
			}
			setAgentDeleteError("");
			setShowAgentDeleteDialog(true);
		}
	};

	const handleDeleteSelectedAgents = async () => {
		const ids = Array.from(selectedAgentIds);
		if (ids.length === 0) return;
		setAgentDeleteError("");
		try {
			await bulkDeleteAgents({ ids }).unwrap();
			setSelectedAgentIds(new Set());
			setAgentBulkAction("");
			setShowAgentDeleteDialog(false);
			refetchAgents();
		} catch {
			setAgentDeleteError("Could not delete selected agents. Try again.");
		}
	};

	const patchControl = async (patch: Partial<BrowserControlSettings>) => {
		try {
			await updateControls(patch).unwrap();
			return true;
		} catch {
			return false;
		}
	};

	const saveUploadWarning = async () => {
		setUploadWarningError("");
		setUploadWarningSaving(true);
		const text = uploadWarningDraft.trim();
		const ok = await patchControl({ upload_warning: text });
		setUploadWarningSaving(false);
		if (!ok) {
			setUploadWarningError("Could not save warning. Try again.");
			return;
		}
		setUploadWarningDraft(text);
		setUploadWarningEditing(false);
	};

	const handleEditTarget = async () => {
		if (!editTarget || !editTargetDomain.trim()) return;
		try {
			await updateTarget({
				id: editTarget.id,
				updates: {
					domain: editTargetDomain.trim(),
					platform_name: editTargetPlatform.trim() || "AI Platform",
					block_site: editTargetBlockSite,
					status: editTargetBlockSite ? "BLOCKED" : editTarget.monitored ? "MONITORED" : "PAUSED",
					host_role: editTargetHostRole || "",
				},
			}).unwrap();
			setEditTargetDialogOpen(false);
			setEditTarget(null);
		} catch (e) {
			// error
		}
	};

	const handleCreateRule = async () => {
		setRuleError("");
		if (!newRuleName.trim()) {
			setRuleError("Rule name is required.");
			return;
		}
		if (newRuleType === "ai_bot") {
			if (!newRuleBotPrompt.trim() && !newRuleBotReferenceImage.trim()) {
				setRuleError("Security policy prompt or reference template image is required for AI Guard Bot.");
				return;
			}
			if (newRuleBotEvalMode === "ai" && newRuleBotPrompt.trim() && newRuleBotPrompt.trim().split(/\s+/).length < 2) {
				setRuleError("Security policy is too short. Describe clearly what should be blocked.");
				return;
			}
			if (newRuleBotEvalMode === "regex" && !newRuleGeneratedPattern.trim()) {
				setRuleError("Generate or enter a regex pattern, or switch to AI Prompt evaluate mode.");
				return;
			}
			if (newRuleBotEvalMode === "ai") {
				if (!isDownloadGuardSource(newRuleBotProvider) && !newRuleBotProvider.trim()) {
					setRuleError("Select an Outsource provider (or switch to Download model).");
					return;
				}
				if (!newRuleBotModel.trim()) {
					setRuleError("Select a model for AI Guard Bot.");
					return;
				}
			}
		} else {
			if (!newRulePattern.trim()) {
				setRuleError("Regex pattern is required for Regex rule.");
				return;
			}
		}
		try {
			const saveAsGeneratedRegex = newRuleType === "ai_bot" && newRuleBotEvalMode === "regex";
			const policyNote = newRuleBotPrompt.trim()
				? `Generated from policy: ${newRuleBotPrompt.trim().slice(0, 500)}`
				: "";
			await createRule({
				name: newRuleName.trim(),
				rule_type: saveAsGeneratedRegex ? "regex" : newRuleType,
				pattern: saveAsGeneratedRegex
					? newRuleGeneratedPattern.trim()
					: newRuleType === "regex"
						? newRulePattern.trim()
						: newRuleGeneratedPattern.trim(),
				bot_provider: newRuleType === "ai_bot" && !saveAsGeneratedRegex ? newRuleBotProvider || GUARD_BOT_OLLAMA_PROVIDER : "",
				bot_model:
					newRuleType === "ai_bot" && !saveAsGeneratedRegex
						? newRuleBotModel || (isDownloadGuardSource(newRuleBotProvider) ? GUARD_BOT_OLLAMA_MODEL : "")
						: "",
				bot_prompt: newRuleType === "ai_bot" && !saveAsGeneratedRegex ? newRuleBotPrompt.trim() : "",
				bot_reference_image: newRuleType === "ai_bot" && !saveAsGeneratedRegex ? newRuleBotReferenceImage : "",
				bot_reference_image_type: newRuleType === "ai_bot" && !saveAsGeneratedRegex ? newRuleBotReferenceImageType : "",
				severity: newRuleSeverity,
				action: newRuleAction,
				description: (newRuleDescription.trim() || (saveAsGeneratedRegex ? policyNote : "")).trim(),
				warning_message: newRuleWarningMessage.trim(),
				active: true,
			}).unwrap();
			setRuleDialogOpen(false);
			setNewRuleName("");
			setNewRulePattern("");
			setNewRuleBotPrompt("");
			setNewRuleBotReferenceImage("");
			setNewRuleBotReferenceImageType("");
			setNewRuleBotReferenceImagePreview("");
			setNewRuleDescription("");
			setNewRuleWarningMessage("");
			setNewRuleType("regex");
			setNewRuleBotEvalMode("ai");
			setNewRuleGeneratedPattern("");
			setNewRuleGenerateError("");
		} catch (e: any) {
			setRuleError(e?.data?.message || "Failed to create rule");
		}
	};

	const runGenerateRegex = async (which: "new" | "edit") => {
		const prompt = which === "new" ? newRuleBotPrompt.trim() : editRuleBotPrompt.trim();
		const provider = which === "new" ? newRuleBotProvider : editRuleBotProvider;
		const model = which === "new" ? newRuleBotModel : editRuleBotModel;
		const setErr = which === "new" ? setNewRuleGenerateError : setEditRuleGenerateError;
		const setPat = which === "new" ? setNewRuleGeneratedPattern : setEditRuleGeneratedPattern;
		setErr("");
		if (!prompt) {
			setErr("Enter a security policy prompt first.");
			return;
		}
		try {
			const res = await generateRegexFromPolicy({
				bot_provider: provider || GUARD_BOT_OLLAMA_PROVIDER,
				bot_model: model || GUARD_BOT_OLLAMA_MODEL,
				bot_prompt: prompt,
			}).unwrap();
			const pat = (res.pattern || "").trim();
			if (!pat) {
				setErr("Model returned an empty pattern.");
				return;
			}
			setPat(pat);
			setErr("");
		} catch (e: any) {
			setErr(
				e?.data?.error?.message ||
				e?.data?.message ||
				e?.message ||
				"Failed to generate regex from policy.",
			);
		}
	};

	const runTestEvaluate = async (which: "new" | "edit") => {
		const policy = which === "new" ? newRuleBotPrompt.trim() : editRuleBotPrompt.trim();
		const sample = which === "new" ? newRuleTestSample.trim() : editRuleTestSample.trim();
		const provider = which === "new" ? newRuleBotProvider : editRuleBotProvider;
		const model = which === "new" ? newRuleBotModel : editRuleBotModel;
		const action = which === "new" ? newRuleAction : editRuleAction;
		const setResult = which === "new" ? setNewRuleTestResult : setEditRuleTestResult;
		setResult("");
		if (!policy) {
			setResult("Enter a security policy first.");
			return;
		}
		if (!sample) {
			setResult("Enter a sample Browser AI prompt to evaluate.");
			return;
		}
		try {
			const res = await testGuardBot({
				bot_provider: provider || GUARD_BOT_OLLAMA_PROVIDER,
				bot_model: model || GUARD_BOT_OLLAMA_MODEL,
				bot_prompt: policy,
				sample_prompt: sample,
				action,
				name: which === "new" ? newRuleName.trim() || "Test" : editRuleName.trim() || "Test",
			}).unwrap();
			if (res.eval_error) {
				setResult(`EVAL FAILED: ${res.eval_error}`);
				return;
			}
			let outcome = `OK — ${res.security_message || "no violation"}`;
			if (res.would_block) {
				outcome = `BLOCK — ${res.security_message || "policy violation"}`;
			} else if (res.would_warn) {
				const act = which === "new" ? newRuleAction : editRuleAction;
				outcome =
					act === "WARN"
						? `WARN — ${res.security_message || "policy match"}`
						: `REDACT — ${res.security_message || "policy match"}`;
			}
			if (res.model_raw?.trim()) {
				outcome = `${outcome}\n\nmodel_raw: ${res.model_raw}`;
			}
			setResult(outcome);
		} catch (e: any) {
			setResult(
				e?.data?.error?.message ||
				e?.data?.message ||
				e?.message ||
				"Model evaluate request failed.",
			);
		}
	};

	const handleEditRuleSubmit = async () => {
		if (!editRule || !editRuleName.trim()) return;
		setRuleError("");
		if (editRuleType === "ai_bot") {
			if (!editRuleBotPrompt.trim() && !editRuleBotReferenceImage.trim()) {
				setRuleError("Security policy prompt or reference template image is required for AI Guard Bot.");
				return;
			}
			if (editRuleBotEvalMode === "ai" && editRuleBotPrompt.trim() && editRuleBotPrompt.trim().split(/\s+/).length < 2) {
				setRuleError("Security policy is too short. Describe clearly what should be blocked.");
				return;
			}
			if (editRuleBotEvalMode === "regex" && !editRuleGeneratedPattern.trim()) {
				setRuleError("Generate or enter a regex pattern, or switch to AI Prompt evaluate mode.");
				return;
			}
			if (editRuleBotEvalMode === "ai") {
				if (!isDownloadGuardSource(editRuleBotProvider) && !editRuleBotProvider.trim()) {
					setRuleError("Select an Outsource provider (or switch to Download model).");
					return;
				}
				if (!editRuleBotModel.trim()) {
					setRuleError("Select a model for AI Guard Bot.");
					return;
				}
			}
		} else {
			if (!editRulePattern.trim()) {
				setRuleError("Regex pattern is required for Regex rule.");
				return;
			}
		}
		try {
			const saveAsGeneratedRegex = editRuleType === "ai_bot" && editRuleBotEvalMode === "regex";
			const updates: Record<string, any> = {
				name: editRuleName.trim(),
				rule_type: saveAsGeneratedRegex ? "regex" : editRuleType,
				severity: editRuleSeverity,
				action: editRuleAction,
				description: editRuleDescription.trim(),
				warning_message: editRuleWarningMessage.trim(),
			};
			if (saveAsGeneratedRegex) {
				updates.pattern = editRuleGeneratedPattern.trim();
				updates.bot_provider = "";
				updates.bot_model = "";
				updates.bot_prompt = "";
				updates.bot_reference_image = "";
				updates.bot_reference_image_type = "";
				if (!updates.description && editRuleBotPrompt.trim()) {
					updates.description = `Generated from policy: ${editRuleBotPrompt.trim().slice(0, 500)}`;
				}
			} else if (editRuleType === "ai_bot") {
				updates.bot_provider = editRuleBotProvider || GUARD_BOT_OLLAMA_PROVIDER;
				updates.bot_model = editRuleBotModel || (isDownloadGuardSource(editRuleBotProvider) ? GUARD_BOT_OLLAMA_MODEL : "");
				updates.bot_prompt = editRuleBotPrompt.trim();
				updates.bot_reference_image = editRuleBotReferenceImage;
				updates.bot_reference_image_type = editRuleBotReferenceImageType;
				updates.pattern = editRuleGeneratedPattern.trim();
			} else {
				updates.pattern = editRulePattern.trim();
				updates.bot_provider = "";
				updates.bot_model = "";
				updates.bot_prompt = "";
				updates.bot_reference_image = "";
				updates.bot_reference_image_type = "";
			}
			await updateRule({
				id: editRule.id,
				updates,
			}).unwrap();
			setEditRuleDialogOpen(false);
			setEditRule(null);
		} catch (e: any) {
			setRuleError(e?.data?.message || "Failed to update rule");
		}
	};

	const logs = logsData?.logs || [];
	const totalLogs = logsData?.total || logs.length;
	const rules = rulesData?.rules || [];
	const targets = targetsData?.targets || [];
	const addedTargetDomains = targets.map((t) => t.domain);
	const newTargetRelatedGroup = relatedHostsForDomain(newTargetDomain);

	// Dynamic Platform filter options based on all configured Target Websites + logged platforms
	const availablePlatformOptions = useMemo(() => {
		const map = new Map<string, string>();

		// 1. From all Target Websites configured by user
		for (const t of targets) {
			const p = (t.platform_name || "").trim();
			const d = (t.domain || "").trim();
			const name = p || d;
			if (name && !map.has(name)) {
				map.set(name, name);
			}
		}

		// 2. From all actual logs recorded
		for (const l of logs) {
			const p = (l.platform || "").trim();
			if (p && !map.has(p)) {
				map.set(p, p);
			}
		}

		// 3. Keep currently selected value if valid and not yet in map
		if (selectedPlatform && selectedPlatform !== "all" && !map.has(selectedPlatform)) {
			map.set(selectedPlatform, selectedPlatform);
		}

		// 4. Common standard AI platforms
		const defaults = [
			{ value: "ChatGPT", label: "ChatGPT" },
			{ value: "Claude", label: "Claude" },
			{ value: "Gemini", label: "Gemini" },
			{ value: "Copilot", label: "Copilot" },
			{ value: "Perplexity", label: "Perplexity" },
			{ value: "DeepSeek", label: "DeepSeek" },
			{ value: "Mistral AI", label: "Mistral AI" },
			{ value: "Grok", label: "Grok" },
		];
		for (const def of defaults) {
			if (!map.has(def.value)) {
				map.set(def.value, def.label);
			}
		}

		return Array.from(map.entries())
			.map(([value, label]) => ({ value, label }))
			.sort((a, b) => a.label.localeCompare(b.label));
	}, [targets, logs, selectedPlatform]);

	const activeRulesCount = rules.filter((r) => r.active).length;
	const monitoredTargetsCount = targets.filter((t) => t.monitored).length;
	const overviewTotal = overviewStats?.total ?? 0;
	const blockedCount = overviewStats?.blocked ?? 0;
	const warnedCount = overviewStats?.warned ?? 0;
	const highRiskCount = overviewStats?.high_risk ?? 0;
	const avgRiskScore = overviewStats?.avg_risk ?? 0;

	const handleCopyPrompt = (text: string) => {
		navigator.clipboard.writeText(text);
		setCopiedPrompt(true);
		setTimeout(() => setCopiedPrompt(false), 2000);
	};

	const handleDownloadSetupPackage = async (platform: "windows" | "mac") => {
		setDownloadingPlatform(platform);
		setSetupPackageError("");
		try {
			const res = await fetch(`${getApiBaseUrl()}/browser-ai/setup/download.zip?platform=${platform}`, {
				credentials: "include",
			});
			if (!res.ok) {
				let msg = `Download failed (${res.status})`;
				try {
					const errJson = await res.json();
					if (errJson?.error || errJson?.message) {
						msg = errJson.error || errJson.message;
					}
				} catch {
					// fallback
				}
				throw new Error(msg);
			}
			const blob = await res.blob();
			const url = window.URL.createObjectURL(blob);
			const link = document.createElement("a");
			link.href = url;
			link.download = platform === "mac" ? "Raksha_Guard_macOS.zip" : "Raksha_Guard_Windows.zip";
			document.body.appendChild(link);
			link.click();
			link.remove();
			window.URL.revokeObjectURL(url);
		} catch (error) {
			setSetupPackageError(error instanceof Error ? error.message : `Failed to download ${platform} setup package`);
		} finally {
			setDownloadingPlatform(null);
		}
	};

	const handleRebuildPackages = async () => {
		setRebuildingPackages(true);
		try {
			const res = await fetch(`${getApiBaseUrl()}/browser-ai/setup/rebuild`, {
				method: "POST",
				credentials: "include",
				headers: { "Content-Type": "application/json" },
			});
			if (!res.ok) {
				const errJson = await res.json().catch(() => ({}));
				refetchRebuildHistory();
				const e = errJson?.error;
				throw new Error(
					(typeof e === "object" && e?.message) || (typeof e === "string" ? e : "") || errJson?.message || "Failed to rebuild packages",
				);
			}
			const data = await res.json();
			const ver = data?.version ? ` v${data.version}` : "";
			const platformHints: string[] = [];
			if (data?.windows_ready === false) platformHints.push("Windows package missing");
			if (data?.macos_ready === false) platformHints.push("macOS ZIP missing");
			if (data?.proxy_bundle?.error) platformHints.push("Guard code not published");
			const hint =
				platformHints.length > 0
					? ` Warning: ${platformHints.join("; ")}.`
					: "";
			const bundleSha: string = data?.proxy_bundle?.sha256 || "";
			toast({
				title:
					data?.mode === "prebuilt"
						? bundleSha
							? `Guard Code Published (${bundleSha.slice(0, 8)})`
							: `Guard Packages Ready${ver}`
						: `Guard Fleet Published${ver}`,
				description:
					(data?.message ||
						"Packages rebuilt. Installed Guards with matching guard_secret will silent-update on next check.") +
					hint,
				variant: platformHints.length > 0 ? "destructive" : "default",
			});
			refetchSetupInfo();
			refetchAgents();
			refetchRebuildHistory();
		} catch (error) {
			toast({
				title: "Rebuild Failed",
				description: error instanceof Error ? error.message : "Failed to rebuild Guard packages",
				variant: "destructive",
			});
		} finally {
			setRebuildingPackages(false);
		}
	};

	const handleSaveUninstallKey = async () => {
		setUninstallKeyMessage("");
		setUninstallKeyError("");
		const nextKey = uninstallKeyInput.trim();
		if (!nextKey) {
			setUninstallKeyError("Enter a new uninstall key to save");
			return;
		}
		try {
			await saveUninstallKey({
				key: nextKey,
				require_uninstall_key: true,
				updated_by: "admin",
			}).unwrap();
			setSavedUninstallKeyDisplay(nextKey);
			if (typeof window !== "undefined") {
				localStorage.setItem("raksha_company_uninstall_key", nextKey);
			}
			setUninstallKeyInput("");
			setUninstallKeyEditing(false);
			setShowUninstallKey(true);
			setUninstallKeyMessage("Uninstall key saved successfully.");
			refetchAgentSettings();
		} catch (error) {
			setUninstallKeyError(error instanceof Error ? error.message : "Failed to save uninstall key");
		}
	};

	const handleCreateTarget = async () => {
		const domain = normalizeTargetDomain(newTargetDomain);
		if (!domain) {
			setTargetError("Enter a domain only, e.g. chat.example.com (no https://).");
			return;
		}
		setTargetError("");
		const platformName = newTargetPlatform.trim() || domain;
		const payload = {
			platform_name: platformName,
			monitored: true,
			block_site: newTargetBlockSite,
			status: newTargetBlockSite ? "BLOCKED" : "MONITORED",
		};
		try {
			const created = await createTarget({ domain, ...payload, host_role: newTargetHostRole || "" }).unwrap();
			const parentId = created?.target?.id || "";
			for (const extra of customRelatedHosts) {
				const host = normalizeTargetDomain(extra.host);
				if (!host || host === domain) continue;
				try {
					await createTarget({
						domain: host,
						...payload,
						parent_id: parentId,
						host_role: extra.role || "",
					}).unwrap();
				} catch {
					const existing = targets.find((t) => normalizeTargetDomain(t.domain) === host);
					if (existing && parentId) {
						try {
							await updateTarget({
								id: existing.id,
								updates: { parent_id: parentId, host_role: extra.role || "" },
							}).unwrap();
						} catch {
							// already in the list — skip
						}
					}
				}
			}
			setNewTargetDomain("");
			setNewTargetPlatform("");
			setNewTargetHostRole("ui");
			setNewTargetBlockSite(false);
			setCustomRelatedHosts([{ host: "", role: "" }]);
			setTargetDialogOpen(false);
		} catch (err: any) {
			setTargetError(err?.data?.message || err?.message || "Failed to create target domain");
		}
	};

	const fillSuggestedRelatedHost = (host: string) => {
		const n = normalizeTargetDomain(host);
		if (!n) return;
		setCustomRelatedHosts((prev) => {
			if (prev.some((v) => normalizeTargetDomain(v.host) === n)) return prev;
			const emptyIdx = prev.findIndex((v) => !v.host.trim());
			if (emptyIdx >= 0) {
				const next = [...prev];
				next[emptyIdx] = { host: n, role: "" };
				return next;
			}
			return [...prev, { host: n, role: "" }];
		});
	};

	const handleAddRelatedHost = async (parent: BrowserTargetWebsite, host: string, role: HostRole = "") => {
		const domain = normalizeTargetDomain(host);
		if (!domain || domain === normalizeTargetDomain(parent.domain)) return;
		const parentId = parent.parent_id || parent.id;
		const payload = {
			domain,
			platform_name: parent.platform_name || domain,
			monitored: parent.monitored,
			block_site: !!parent.block_site,
			status: parent.block_site ? "BLOCKED" : parent.monitored ? "MONITORED" : "PAUSED",
			parent_id: parentId,
			host_role: role || "",
		};
		try {
			await createTarget(payload).unwrap();
		} catch (err: any) {
			const existing = targets.find((t) => normalizeTargetDomain(t.domain) === domain);
			if (existing) {
				try {
					await updateTarget({ id: existing.id, updates: { parent_id: parentId } }).unwrap();
					return;
				} catch {
					// fall through
				}
			}
			setTargetError(err?.data?.message || err?.message || "Failed to add related host");
		}
	};

	const getAgentStatusBadge = (status: string, uninstallRequested?: boolean) => {
		const s = (status || "").toLowerCase();
		if (s === "uninstalled") return <Badge className="bg-slate-800 text-slate-300 border border-slate-700">Uninstalled</Badge>;
		if (s === "uninstall_pending" || uninstallRequested) {
			return <Badge className="bg-amber-950 text-amber-300 border border-amber-800">Uninstall pending</Badge>;
		}
		if (s === "active") return <Badge className="bg-emerald-950 text-emerald-400 border border-emerald-800">Active</Badge>;
		return <Badge className="bg-slate-800 text-slate-300 border border-slate-700">{status || "unknown"}</Badge>;
	};

	const nicGuidOnly = (raw?: string) => {
		if (!raw) return "";
		const m = raw.match(/\{[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\}/);
		if (m) return m[0].toUpperCase();
		return raw.trim();
	};

	const filteredRules = rules.filter(
		(r) =>
			r.name.toLowerCase().includes(ruleSearch.toLowerCase()) ||
			(r.pattern || "").toLowerCase().includes(ruleSearch.toLowerCase()) ||
			(r.bot_prompt || "").toLowerCase().includes(ruleSearch.toLowerCase()) ||
			(r.description || "").toLowerCase().includes(ruleSearch.toLowerCase())
	);

	useEffect(() => {
		setRulesPageOffset(0);
	}, [ruleSearch, rulesPageLimit]);

	const rulesTotalPages = Math.ceil(filteredRules.length / rulesPageLimit) || 1;
	const rulesCurrentPage = Math.floor(rulesPageOffset / rulesPageLimit) + 1;
	const pagedRules = filteredRules.slice(rulesPageOffset, rulesPageOffset + rulesPageLimit);

	const getBrowserAiExportPayload = async (): Promise<ExportFormatsPayload> => {
		if (activeTab === "rules") {
			// Column headers match Import Rules Excel template (round-trip safe).
			return {
				filename: "browser-ai-guard-rules",
				title: "Browser AI — Guard Rules",
				subtitle: `${filteredRules.length} rule(s)`,
				columns: [
					{ key: "name", header: "Rule Name" },
					{ key: "rule_type", header: "Rule Type" },
					{ key: "pattern", header: "Pattern" },
					{ key: "severity", header: "Severity" },
					{ key: "action", header: "Action" },
					{ key: "warning_message", header: "Warning Message" },
					{ key: "active", header: "Active" },
					{ key: "description", header: "Description" },
				],
				rows: filteredRules.map((r) => ({
					name: r.name,
					rule_type: r.rule_type === "ai_bot" ? "ai_bot" : "regex",
					pattern: r.rule_type === "ai_bot" ? "" : r.pattern || "",
					severity: r.severity,
					action: r.action,
					warning_message: r.warning_message || "",
					active: r.active ? "TRUE" : "FALSE",
					description: r.description || (r.rule_type === "ai_bot" ? (r.bot_prompt || "").slice(0, 500) : ""),
				})),
				json: filteredRules.map((r) => ({
					name: r.name,
					rule_type: r.rule_type === "ai_bot" ? "ai_bot" : "regex",
					pattern: r.pattern || "",
					severity: r.severity,
					action: r.action,
					warning_message: r.warning_message || "",
					active: !!r.active,
					description: r.description || "",
					bot_prompt: r.bot_prompt || "",
					bot_provider: r.bot_provider || "",
					bot_model: r.bot_model || "",
				})),
			};
		}
		if (activeTab === "targets") {
			return {
				filename: "browser-ai-targets",
				title: "Browser AI — Target Websites",
				subtitle: `${targets.length} target(s)`,
				columns: [
					{ key: "domain", header: "Domain" },
					{ key: "platform", header: "Platform" },
					{ key: "status", header: "Status" },
					{ key: "host_role", header: "Host role" },
				],
				rows: targets.map((t) => ({
					domain: t.domain,
					platform: t.platform_name || "",
					status: t.block_site ? "Blocked" : t.monitored ? "Monitored" : "Paused",
					host_role: t.host_role || "",
				})),
			};
		}
		if (activeTab === "agents") {
			return {
				filename: "browser-ai-agents",
				title: "Browser AI — Guard Agents",
				subtitle: `${agents.length} agent(s)`,
				columns: [
					{ key: "hostname", header: "Hostname" },
					{ key: "agent_id", header: "Agent ID" },
					{ key: "agent_type", header: "Source" },
					{ key: "status", header: "Status" },
					{ key: "last_seen", header: "Last seen" },
				],
				rows: agents.map((a) => ({
					hostname: a.hostname || "",
					agent_id: a.id || "",
					agent_type: a.agent_type || "endpoint",
					status: a.status || "",
					last_seen: a.last_seen_at || "",
				})),
			};
		}
		if (activeTab === "search-logs") {
			let exportSearchLogs = searchLogs;
			if (totalSearchLogs > searchLogs.length) {
				try {
					const params = new URLSearchParams({
						limit: String(Math.min(5000, totalSearchLogs)),
						offset: "0",
					});
					if (searchEngineFilter && searchEngineFilter !== "all") params.set("engine", searchEngineFilter);
					if (searchBrowserFilter && searchBrowserFilter !== "all") params.set("browser", searchBrowserFilter);
					if (searchIncognitoFilter && searchIncognitoFilter !== "all") params.set("is_incognito", searchIncognitoFilter);
					if (searchLogQuery) params.set("search", searchLogQuery);
					const res = await fetch(`/api/browser-ai/search-logs?${params.toString()}`);
					if (res.ok) {
						const data = await res.json();
						if (Array.isArray(data.logs) && data.logs.length > 0) {
							exportSearchLogs = data.logs;
						}
					}
				} catch {
					// fallback to current page
				}
			}
			return {
				filename: "browser-ai-search-logs",
				title: "Browser AI — Search Logs",
				subtitle: `${exportSearchLogs.length} of ${totalSearchLogs} search log(s)`,
				columns: [
					{ key: "date", header: "Date" },
					{ key: "time", header: "Time" },
					{ key: "desktop_name", header: "Desktop Name" },
					{ key: "engine", header: "Search Engine" },
					{ key: "browser", header: "Browser" },
					{ key: "privacy", header: "Privacy Mode" },
					{ key: "query", header: "Search Query / Prompt" },
					{ key: "clicked", header: "Clicked Result Link" },
					{ key: "threat", header: "Threat Risk" },
				],
				rows: exportSearchLogs.map((log) => ({
					date: formatLogDate(log.timestamp),
					time: formatLogTime(log.timestamp),
					desktop_name: log.agent_hostname || "",
					engine: log.engine || "",
					browser: log.browser || "",
					privacy: log.is_incognito ? "Incognito / InPrivate" : "Normal",
					query: log.query || "",
					clicked: log.clicked_url || "",
					threat: [
						log.predictive_risk || "",
						log.risk_score != null ? `(${log.risk_score}%)` : "",
						log.risk_category || "",
					]
						.filter(Boolean)
						.join(" "),
				})),
			};
		}
		// overview + logs — same columns as Prompt Logs table
		let exportLogs = logs;
		if (totalLogs > logs.length) {
			try {
				const params = new URLSearchParams({
					limit: String(Math.min(5000, totalLogs)),
					offset: "0",
				});
				if (selectedPlatform && selectedPlatform !== "all") params.set("platform", selectedPlatform);
				if (selectedAction && selectedAction !== "all") params.set("action", selectedAction);
				if (searchQuery) params.set("search", searchQuery);
				const res = await fetch(`/api/browser-ai/logs?${params.toString()}`);
				if (res.ok) {
					const data = await res.json();
					if (Array.isArray(data.logs) && data.logs.length > 0) {
						exportLogs = data.logs;
					}
				}
			} catch {
				// fallback to current page
			}
		}
		const promptPreviewForExport = (log: (typeof logs)[number]) => {
			if (isFileUploadLog(log)) {
				const label = logAttachmentLabel(log);
				const caption = logUserCaption(log);
				return caption ? `${label} · ${caption}` : label;
			}
			return (log.user_prompt_preview || log.user_prompt_full || "").slice(0, 500);
		};
		const promptDetailsForExport = (log: (typeof logs)[number]) => {
			const parts: string[] = [];
			if (log.rule_triggered) parts.push(`Rule: ${log.rule_triggered}`);
			if (log.predicted_category) parts.push(`Category: ${log.predicted_category}`);
			if (log.predictive_risk && log.predictive_risk !== "LOW") parts.push(`Risk: ${log.predictive_risk}`);
			if (log.attachment_name) parts.push(`File: ${log.attachment_name}`);
			if (parts.length === 0) {
				return log.action === "Blocked" ? "Blocked by security rule" : "Standard prompt";
			}
			return parts.join(" · ");
		};
		return {
			filename: "browser-ai-prompt-logs",
			title: "Browser AI — Prompt Logs",
			subtitle: `${exportLogs.length} of ${totalLogs} log(s)`,
			columns: [
				{ key: "date", header: "Date" },
				{ key: "time", header: "Time" },
				{ key: "desktop_name", header: "Desktop Name" },
				{ key: "platform", header: "Platform" },
				{ key: "prompt", header: "User Prompt" },
				{ key: "tokens", header: "Est. Tokens" },
				{ key: "action", header: "Action" },
				{ key: "details", header: "Security & Policy Details" },
			],
			rows: exportLogs.map((log) => ({
				date: formatLogDate(log.timestamp),
				time: formatLogTime(log.timestamp),
				desktop_name: log.agent_hostname || log.agent_id || "",
				platform: log.platform || "",
				prompt: promptPreviewForExport(log),
				tokens: log.est_tokens ?? "",
				action: log.action || "",
				details: promptDetailsForExport(log),
			})),
		};
	};

	const targetSearchLower = targetSearch.toLowerCase().trim();
	const targetMatchesSearch = (tgt: BrowserTargetWebsite) => {
		if (!targetSearchLower) return true;
		const statusLabel = tgt.block_site ? "blocked" : tgt.monitored ? "monitored" : "paused";
		return (
			(tgt.domain || "").toLowerCase().includes(targetSearchLower) ||
			(tgt.platform_name || "").toLowerCase().includes(targetSearchLower) ||
			statusLabel.includes(targetSearchLower)
		);
	};

	const filteredTargetGroups = useMemo(() => {
		const groups = groupTargetsByParent(targets);
		if (!targetSearchLower) return groups;
		return groups.filter((group) => {
			const parentHit = targetMatchesSearch(group.parent);
			const childHits = group.children.filter(targetMatchesSearch);
			return parentHit || childHits.length > 0;
		});
	}, [targets, targetSearchLower]);

	const totalTargetParents = filteredTargetGroups.length;
	const targetCurrentPage = Math.floor(targetPageOffset / targetPageLimit) + 1;
	const targetTotalPages = Math.ceil(totalTargetParents / targetPageLimit) || 1;

	const visibleTargetRows: { tgt: BrowserTargetWebsite; isChild: boolean }[] = useMemo(() => {
		const pageGroups = filteredTargetGroups.slice(targetPageOffset, targetPageOffset + targetPageLimit);
		const rows: { tgt: BrowserTargetWebsite; isChild: boolean }[] = [];
		for (const group of pageGroups) {
			rows.push({ tgt: group.parent, isChild: false });
			const kids =
				!targetSearchLower || targetMatchesSearch(group.parent)
					? group.children
					: group.children.filter(targetMatchesSearch);
			for (const child of kids) {
				rows.push({ tgt: child, isChild: true });
			}
		}
		return rows;
	}, [filteredTargetGroups, targetPageOffset, targetPageLimit, targetSearchLower]);

	useEffect(() => {
		setTargetPageOffset(0);
	}, [targetSearch, targetPageLimit]);

	// Pagination calculations
	const currentPage = Math.floor(pageOffset / pageLimit) + 1;
	const totalPages = Math.ceil(totalLogs / pageLimit) || 1;
	const searchLogCurrentPage = Math.floor(searchLogPageOffset / searchLogPageLimit) + 1;
	const searchLogTotalPages = Math.ceil(totalSearchLogs / searchLogPageLimit) || 1;
	const agentCurrentPage = Math.floor(agentPageOffset / agentPageLimit) + 1;
	const agentTotalPages = Math.ceil(totalAgents / agentPageLimit) || 1;

	return (
		<div className="space-y-6 p-2 md:p-6 text-foreground max-w-7xl mx-auto">
			{/* Header View */}
			<div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 border-b border-border pb-5">
				<div>
					<div className="flex items-center gap-3">
						<Globe className="h-6 w-6 text-primary" />
						<h1 className="text-2xl font-bold tracking-tight">
							Browser AI · {BROWSER_AI_TAB_TITLES[activeTab]}
						</h1>
					</div>
					<p className="text-muted-foreground text-sm mt-1">
						Monitor browser AI prompts, predict security threat levels, warn or block policy hits, and control DLP guardrails.
					</p>
				</div>
				<div className="flex items-center gap-3 flex-wrap sm:flex-nowrap">
					<div className="flex items-center gap-2 bg-card border border-border px-3 py-1.5 rounded-md text-xs">
						<Switch
							checked={liveUpdatesEnabled}
							onCheckedChange={setLiveUpdatesEnabled}
							id="live-update-switch"
						/>
						<Label htmlFor="live-update-switch" className="cursor-pointer font-medium text-xs">
							Live Update
						</Label>
					</div>

					{activeTab !== "setup" ? (
						<ExportFormatsDropdown
							size="sm"
							className="h-8 text-xs gap-2 border-border"
							getPayload={getBrowserAiExportPayload}
							testId="browser-ai-export-trigger"
						/>
					) : null}

					{activeTab === "logs" ? (
						<div className="flex items-center gap-2">
							<div className="flex items-center gap-2 bg-card border border-border px-2.5 py-1 rounded-md">
								<Switch
									checked={!!controls.prompt_log_auto_delete}
									onCheckedChange={(on) => {
										void patchControl({
											prompt_log_auto_delete: on,
											prompt_log_retention: controls.prompt_log_retention || "7d",
										});
										if (on) {
											void refetchLogs();
										}
									}}
									id="prompt-log-auto-delete"
								/>
								<Label htmlFor="prompt-log-auto-delete" className="cursor-pointer font-medium text-xs whitespace-nowrap">
									Auto-delete
								</Label>
								{controls.prompt_log_auto_delete ? (
									<Select
										value={
											["1d", "7d", "30d", "90d", "180d", "365d"].includes(controls.prompt_log_retention || "")
												? controls.prompt_log_retention
												: "7d"
										}
										onValueChange={(v) => {
											void patchControl({
												prompt_log_auto_delete: true,
												prompt_log_retention: v,
											});
											void refetchLogs();
										}}
									>
										<SelectTrigger className="h-7 w-[8rem] text-xs border-border bg-background">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="1d">1 day</SelectItem>
											<SelectItem value="7d">7 days (1w)</SelectItem>
											<SelectItem value="30d">30 days (1m)</SelectItem>
											<SelectItem value="90d">90 days (3m)</SelectItem>
											<SelectItem value="180d">180 days (6m)</SelectItem>
											<SelectItem value="365d">365 days (1y)</SelectItem>
										</SelectContent>
									</Select>
								) : null}
							</div>
							{selectedLogIds.size > 0 ? (
								<Button
									variant="outline"
									size="sm"
									onClick={() => setDeleteSelectedTarget("logs")}
									className="h-8 gap-2 text-xs border-destructive/50 text-destructive hover:bg-destructive/10"
									data-testid="browser-ai-delete-selected-logs"
								>
									<Trash2 className="h-3.5 w-3.5" />
									Delete selected ({selectedLogIds.size})
								</Button>
							) : null}
							<Button
								variant="outline"
								size="sm"
								onClick={() => setClearLogsDialogOpen(true)}
								className="h-8 gap-2 text-xs border-border text-destructive hover:bg-destructive/10"
								data-testid="browser-ai-clear-logs"
							>
								<Trash2 className="h-3.5 w-3.5" />
								Clear logs
							</Button>
						</div>
					) : null}

					{activeTab === "search-logs" ? (
						<div className="flex items-center gap-2">
							<div className="flex items-center gap-2 bg-card border border-border px-2.5 py-1 rounded-md">
								<Switch
									checked={!!controls.search_log_auto_delete}
									onCheckedChange={(on) => {
										void patchControl({
											search_log_auto_delete: on,
											search_log_retention: controls.search_log_retention || "7d",
										});
										if (on) {
											void refetchSearchLogs();
										}
									}}
									id="search-log-auto-delete"
								/>
								<Label htmlFor="search-log-auto-delete" className="cursor-pointer font-medium text-xs whitespace-nowrap">
									Auto-delete
								</Label>
								{controls.search_log_auto_delete ? (
									<Select
										value={
											["1d", "7d", "30d", "90d", "180d", "365d"].includes(controls.search_log_retention || "")
												? controls.search_log_retention
												: "7d"
										}
										onValueChange={(v) => {
											void patchControl({
												search_log_auto_delete: true,
												search_log_retention: v,
											});
											void refetchSearchLogs();
										}}
									>
										<SelectTrigger className="h-7 w-[8rem] text-xs border-border bg-background">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="1d">1 day</SelectItem>
											<SelectItem value="7d">7 days (1w)</SelectItem>
											<SelectItem value="30d">30 days (1m)</SelectItem>
											<SelectItem value="90d">90 days (3m)</SelectItem>
											<SelectItem value="180d">180 days (6m)</SelectItem>
											<SelectItem value="365d">365 days (1y)</SelectItem>
										</SelectContent>
									</Select>
								) : null}
							</div>
							{selectedSearchLogIds.size > 0 ? (
								<Button
									variant="outline"
									size="sm"
									onClick={() => setDeleteSelectedTarget("search-logs")}
									className="h-8 gap-2 text-xs border-destructive/50 text-destructive hover:bg-destructive/10"
									data-testid="browser-ai-delete-selected-search-logs"
								>
									<Trash2 className="h-3.5 w-3.5" />
									Delete selected ({selectedSearchLogIds.size})
								</Button>
							) : null}
							<Button
								variant="outline"
								size="sm"
								onClick={() => setClearSearchLogsDialogOpen(true)}
								className="h-8 gap-2 text-xs border-border text-destructive hover:bg-destructive/10"
								data-testid="browser-ai-clear-search-logs"
							>
								<Trash2 className="h-3.5 w-3.5" />
								Clear logs
							</Button>
						</div>
					) : null}

					<Button
						variant="outline"
						size="sm"
						onClick={() => {
							refetchLogs();
							refetchSearchLogs();
							refetchRules();
							refetchTargets();
							refetchAgents();
						}}
						className="gap-2 border-border hover:bg-accent h-8 text-xs"
					>
						<RefreshCw className={`h-3.5 w-3.5 ${logsLoading || agentsLoading || searchLogsLoading ? "animate-spin" : ""}`} />
						Refresh
					</Button>
				</div>
			</div>

			{/* Section content — nav is sidebar dropdown (Observability / Models style) */}
			<Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-6">
				{/* TAB 1: OVERVIEW */}
				<TabsContent value="overview" className="space-y-6">
					<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5">
									<Globe className="h-3.5 w-3.5 text-muted-foreground" /> Total Prompts Intercepted
								</CardDescription>
								<CardTitle className="text-3xl font-bold">{overviewTotal}</CardTitle>
							</CardHeader>
							<CardContent>
								<p className="text-xs text-muted-foreground">Passing through HTTPS proxy</p>
							</CardContent>
						</Card>

						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5">
									<AlertCircle className="h-3.5 w-3.5 text-amber-400" /> Redacted
								</CardDescription>
								<CardTitle className="text-3xl font-bold text-amber-400">{warnedCount}</CardTitle>
							</CardHeader>
							<CardContent>
								<p className="text-xs text-muted-foreground">Prompt forwarded with redaction notice</p>
							</CardContent>
						</Card>

						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5">
									<BrainCircuit className="h-3.5 w-3.5 text-amber-400" /> Predictive High Risk
								</CardDescription>
								<CardTitle className="text-3xl font-bold text-amber-400">{highRiskCount}</CardTitle>
							</CardHeader>
							<CardContent>
								<p className="text-xs text-muted-foreground">Avg Risk Score: {avgRiskScore}%</p>
							</CardContent>
						</Card>

						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5">
									<AlertTriangle className="h-3.5 w-3.5 text-red-400" /> Blocked Violations
								</CardDescription>
								<CardTitle className="text-3xl font-bold text-red-400">{blockedCount}</CardTitle>
							</CardHeader>
							<CardContent>
								<p className="text-xs text-muted-foreground">Security policy breaches blocked</p>
							</CardContent>
						</Card>
					</div>

					<Card className="bg-card border-border">
						<CardHeader className="flex flex-row items-start justify-between gap-3 space-y-0">
							<div>
								<CardTitle className="text-lg">Recent Intercepted Activity</CardTitle>
								<CardDescription>Real-time prompt stream captured from browser sessions</CardDescription>
							</div>
							<Button variant="outline" size="sm" className="h-8 text-xs shrink-0" onClick={() => setActiveTab("logs")}>
								Show all
							</Button>
						</CardHeader>
						<CardContent>
							<div className="rounded-md border border-border overflow-x-auto">
								<Table className="table-fixed w-full min-w-[960px]">
									<TableHeader>
										<TableRow className="border-border hover:bg-transparent">
											<TableHead className="w-[100px]">Date</TableHead>
											<TableHead className="w-[100px]">Time</TableHead>
											<TableHead className="w-[110px]">Desktop Name</TableHead>
											<TableHead className="w-[100px]">Platform</TableHead>
											<TableHead className="w-[auto]">User Prompt</TableHead>
											<TableHead className="w-[80px] text-right">Est. Tokens</TableHead>
											<TableHead className="w-[120px]">Action</TableHead>
											<TableHead className="w-[64px] text-right">Details</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{logs.slice(0, 5).map((log) => (
											<TableRow
												key={log.id}
												onClick={() => setSelectedLog(log)}
												className="min-h-12 cursor-pointer border-border hover:bg-accent/50 transition-colors"
											>
												<TableCell className="max-w-0 py-0">
													<div className="truncate font-mono text-xs text-muted-foreground" title={formatLogDate(log.timestamp)}>
														{formatLogDate(log.timestamp)}
													</div>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<div className="truncate font-mono text-xs text-muted-foreground" title={formatLogTime(log.timestamp)}>
														{formatLogTime(log.timestamp)}
													</div>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<div className="truncate text-xs text-muted-foreground" title={log.agent_hostname || log.agent_id || ""}>
														{log.agent_hostname || log.agent_id || "—"}
													</div>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<div className="min-w-0 truncate">{getPlatformBadge(log.platform)}</div>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<LogPromptPreviewCell log={log} />
												</TableCell>
												<TableCell className="py-0 text-right text-xs font-mono">{log.est_tokens}</TableCell>
												<TableCell className="py-0">{logActionBadge(log)}</TableCell>
												<TableCell className="py-0 text-right">
													<div className="inline-flex items-center justify-end gap-0.5">
														{logHasStoredAttachment(log) ? (
															<Button
																variant="ghost"
																size="sm"
																onClick={(e) => openPdfViewer(log, e)}
																className="h-8 px-2 text-xs text-sky-400 hover:text-sky-300"
																title="View file"
															>
																View
															</Button>
														) : null}
														<Button
															variant="ghost"
															size="icon"
															onClick={(e) => {
																e.stopPropagation();
																setSelectedLog(log);
															}}
															className="h-8 w-8 text-muted-foreground hover:text-foreground"
															title="Prompt details"
														>
															<Eye className="h-4 w-4" />
														</Button>
													</div>
												</TableCell>
											</TableRow>
										))}
										{logs.length === 0 && (
											<TableRow>
												<TableCell colSpan={7} className="text-center py-6 text-muted-foreground">
													No prompts intercepted yet. Guard agent must be running,
													Target site Monitoring ON and Block Website OFF, then fully quit and reopen
													the browser so PAC hits the local Guard proxy (proxy_addr from Guard config /
													RAKSHA_PROXY_ADDR). If the AI site opens but logs stay 0,
													traffic is bypassing the proxy — check Guard Agents health / local
													PAC status URL (from PAC_HTTP_PORT in .env / Guard config).
												</TableCell>
											</TableRow>
										)}
									</TableBody>
								</Table>
							</div>
						</CardContent>
					</Card>
				</TabsContent>

				{/* TAB 2: PROMPT LOGS */}
				<TabsContent value="logs" className="space-y-4">
					<Card className="bg-card border-border">
						<CardHeader className="pb-4">
							<div>
								<CardTitle className="text-lg">Prompt & Chat History</CardTitle>
								<CardDescription>Live intercepted requests passing through the proxy</CardDescription>
							</div>

							{/* Search & Filter Toolbar */}
							<div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4">
								<div className="relative">
									<Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
									<Input
										placeholder="Search prompts or domain..."
										value={searchQuery}
										onChange={(e) => {
											setSearchQuery(e.target.value);
											setPageOffset(0);
										}}
										className="pl-9 bg-background border-border"
									/>
								</div>
								<Select
									value={selectedPlatform}
									onValueChange={(val) => {
										setSelectedPlatform(val);
										setPageOffset(0);
									}}
								>
									<SelectTrigger className="bg-background border-border">
										<SelectValue placeholder="All Platforms" />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">All Platforms</SelectItem>
										{availablePlatformOptions.map((opt) => (
											<SelectItem key={opt.value} value={opt.value}>
												{opt.label}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
								<Select
									value={selectedAction}
									onValueChange={(val) => {
										setSelectedAction(val);
										setPageOffset(0);
									}}
								>
									<SelectTrigger className="bg-background border-border">
										<SelectValue placeholder="All Status" />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">All Status</SelectItem>
										<SelectItem value="Allowed">Allowed</SelectItem>
										<SelectItem value="Redacted">Redacted</SelectItem>
										<SelectItem value="Warned">Warned (legacy)</SelectItem>
										<SelectItem value="Blocked">Blocked (DLP / rules)</SelectItem>
										<SelectItem value="SiteBlocked">Site Blocked (full website)</SelectItem>
										<SelectItem value="Bot Answered">Bot Answered</SelectItem>
									</SelectContent>
								</Select>
							</div>
						</CardHeader>

						<CardContent>
							<div className="rounded-md border border-border overflow-x-auto">
								<Table className="table-fixed w-full min-w-[960px]">
									<TableHeader>
										<TableRow className="border-border hover:bg-transparent">
											<TableHead className="w-[40px]">
												<Checkbox
													aria-label="Select all prompt logs on this page"
													checked={logs.length > 0 && logs.every((l) => selectedLogIds.has(l.id))}
													onCheckedChange={(on) =>
														setSelectedLogIds(on === true ? new Set(logs.map((l) => l.id)) : new Set())
													}
												/>
											</TableHead>
											<TableHead className="w-[100px]">Date</TableHead>
											<TableHead className="w-[100px]">Time</TableHead>
											<TableHead className="w-[110px]">Desktop Name</TableHead>
											<TableHead className="w-[100px]">Platform</TableHead>
											<TableHead className="w-[auto]">User Prompt</TableHead>
											<TableHead className="w-[80px] text-right">Est. Tokens</TableHead>
											<TableHead className="w-[120px]">Action</TableHead>
											<TableHead className="w-[64px] text-right">Details</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{logs.map((log) => (
											<TableRow
												key={log.id}
												onClick={() => setSelectedLog(log)}
												className="min-h-12 cursor-pointer border-border hover:bg-accent/50 transition-colors"
											>
												<TableCell className="py-0" onClick={(e) => e.stopPropagation()}>
													<Checkbox
														aria-label="Select prompt log"
														checked={selectedLogIds.has(log.id)}
														onCheckedChange={(on) => toggleId(setSelectedLogIds, log.id, on === true)}
													/>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<div className="truncate font-mono text-xs text-muted-foreground" title={formatLogDate(log.timestamp)}>
														{formatLogDate(log.timestamp)}
													</div>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<div className="truncate font-mono text-xs text-muted-foreground" title={formatLogTime(log.timestamp)}>
														{formatLogTime(log.timestamp)}
													</div>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<div className="truncate text-xs text-muted-foreground" title={log.agent_hostname || log.agent_id || ""}>
														{log.agent_hostname || log.agent_id || "—"}
													</div>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<div className="min-w-0 truncate">{getPlatformBadge(log.platform)}</div>
												</TableCell>
												<TableCell className="max-w-0 py-0">
													<LogPromptPreviewCell log={log} />
												</TableCell>
												<TableCell className="py-0 text-right text-xs font-mono">{log.est_tokens}</TableCell>
												<TableCell className="py-0">{logActionBadge(log)}</TableCell>
												<TableCell className="py-0 text-right">
													<div className="inline-flex items-center justify-end gap-0.5">
														{logHasStoredAttachment(log) ? (
															<Button
																variant="ghost"
																size="sm"
																onClick={(e) => openPdfViewer(log, e)}
																className="h-8 px-2 text-xs text-sky-400 hover:text-sky-300"
																title="View file"
															>
																View
															</Button>
														) : null}
														<Button
															variant="ghost"
															size="icon"
															onClick={(e) => {
																e.stopPropagation();
																setSelectedLog(log);
															}}
															className="h-8 w-8 text-muted-foreground hover:text-foreground"
															title="Prompt details"
														>
															<Eye className="h-4 w-4" />
														</Button>
													</div>
												</TableCell>
											</TableRow>
										))}
										{logs.length === 0 && (
											<TableRow>
												<TableCell colSpan={9} className="text-center py-8 text-muted-foreground">
													No prompt logs match your filter criteria.
												</TableCell>
											</TableRow>
										)}
									</TableBody>
								</Table>
							</div>

							{/* Standard Pagination Controls */}
							<div className="flex flex-col sm:flex-row justify-between items-center gap-4 mt-4 text-xs text-muted-foreground">
								<div className="flex items-center gap-2">
									<span>Rows per page</span>
									<Select
										value={pageLimit.toString()}
										onValueChange={(val) => {
											setPageLimit(Number(val));
											setPageOffset(0);
										}}
									>
										<SelectTrigger className="h-8 w-[70px] bg-background border-border">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="10">10</SelectItem>
											<SelectItem value="25">25</SelectItem>
											<SelectItem value="50">50</SelectItem>
											<SelectItem value="100">100</SelectItem>
										</SelectContent>
									</Select>
									<span>
										Showing {totalLogs > 0 ? pageOffset + 1 : 0} to {Math.min(pageOffset + pageLimit, totalLogs)} of {totalLogs} entries
									</span>
								</div>

								<div className="flex items-center gap-2">
									<span>
										Page {currentPage} of {totalPages}
									</span>
									<div className="flex items-center gap-1">
										<Button
											variant="outline"
											size="icon"
											disabled={pageOffset === 0}
											onClick={() => setPageOffset(Math.max(0, pageOffset - pageLimit))}
											className="h-8 w-8 border-border"
										>
											<ChevronLeft className="h-4 w-4" />
										</Button>
										<Button
											variant="outline"
											size="icon"
											disabled={pageOffset + pageLimit >= totalLogs}
											onClick={() => setPageOffset(pageOffset + pageLimit)}
											className="h-8 w-8 border-border"
										>
											<ChevronRight className="h-4 w-4" />
										</Button>
									</div>
								</div>
							</div>
						</CardContent>
					</Card>
				</TabsContent>

				{/* TAB: SEARCH LOGS */}
				<TabsContent value="search-logs" className="space-y-4">
					{/* KPI Summary Cards */}
					<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5">
									<Search className="h-3.5 w-3.5 text-muted-foreground" /> Total Searches Monitored
								</CardDescription>
								<CardTitle className="text-3xl font-bold">{totalSearchLogs}</CardTitle>
							</CardHeader>
							<CardContent>
								<p className="text-xs text-muted-foreground">Google, Bing, DDG, Yahoo — Chrome / Edge / Firefox / Brave / Safari</p>
							</CardContent>
						</Card>

						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5">
									<EyeOff className="h-3.5 w-3.5 text-purple-400" /> Incognito &amp; InPrivate
								</CardDescription>
								<CardTitle className="text-3xl font-bold text-purple-400">{incognitoSearchCount}</CardTitle>
							</CardHeader>
							<CardContent>
								<p className="text-xs text-muted-foreground">Private browsing sessions inspected</p>
							</CardContent>
						</Card>

						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5">
									<FileText className="h-3.5 w-3.5 text-emerald-400" /> Search Queries Logged
								</CardDescription>
								<CardTitle className="text-3xl font-bold text-emerald-400">{queriesSearchCount}</CardTitle>
							</CardHeader>
							<CardContent>
								<p className="text-xs text-muted-foreground">Direct keyword prompts and search intent</p>
							</CardContent>
						</Card>

						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription className="flex items-center gap-1.5">
									<ExternalLink className="h-3.5 w-3.5 text-blue-400" /> Result Links Clicked
								</CardDescription>
								<CardTitle className="text-3xl font-bold text-blue-400">{clicksSearchCount}</CardTitle>
							</CardHeader>
							<CardContent>
								<p className="text-xs text-muted-foreground">Destination URLs navigated from search</p>
							</CardContent>
						</Card>
					</div>

					{/* Main Search Logs Card */}
					<Card className="bg-card border-border">
						<CardHeader className="pb-4">
							<div>
								<CardTitle className="text-lg flex items-center gap-2">
									<Search className="h-5 w-5 text-emerald-400" />
									Search Engine Activity &amp; Privacy Audit
								</CardTitle>
								<CardDescription>
									Real-time search queries and clicked links from any Guard browser (Chrome, Edge, Firefox, Brave, Opera, Safari) — Google, Bing/MSN, DuckDuckGo, Yahoo — including Incognito/InPrivate. Saved to Postgres.
								</CardDescription>
							</div>

							{/* Search & Filter Toolbar */}
							<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 mt-4">
								<div className="relative">
									<Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
									<Input
										placeholder="Filter query, URL, host..."
										value={searchLogQuery}
										onChange={(e) => {
											setSearchLogQuery(e.target.value);
											setSearchLogPageOffset(0);
										}}
										className="pl-9 bg-background border-border text-xs"
									/>
								</div>
								<Select
									value={searchEngineFilter}
									onValueChange={(v) => {
										setSearchEngineFilter(v);
										setSearchLogPageOffset(0);
									}}
								>
									<SelectTrigger className="bg-background border-border text-xs">
										<SelectValue placeholder="All Engines" />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">All Engines</SelectItem>
										<SelectItem value="google">Google</SelectItem>
										<SelectItem value="bing">Bing</SelectItem>
										<SelectItem value="safari">Safari / Apple</SelectItem>
										<SelectItem value="duck">DuckDuckGo</SelectItem>
										<SelectItem value="brave">Brave Search</SelectItem>
										<SelectItem value="yahoo">Yahoo</SelectItem>
									</SelectContent>
								</Select>
								<Select
									value={searchBrowserFilter}
									onValueChange={(v) => {
										setSearchBrowserFilter(v);
										setSearchLogPageOffset(0);
									}}
								>
									<SelectTrigger className="bg-background border-border text-xs">
										<SelectValue placeholder="All Browsers" />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">All Browsers</SelectItem>
										<SelectItem value="chrome">Chrome</SelectItem>
										<SelectItem value="edge">Edge</SelectItem>
										<SelectItem value="safari">Safari</SelectItem>
										<SelectItem value="firefox">Firefox</SelectItem>
										<SelectItem value="brave">Brave</SelectItem>
										<SelectItem value="opera">Opera</SelectItem>
										<SelectItem value="vivaldi">Vivaldi</SelectItem>
									</SelectContent>
								</Select>
								<Select
									value={searchIncognitoFilter}
									onValueChange={(v) => {
										setSearchIncognitoFilter(v);
										setSearchLogPageOffset(0);
									}}
								>
									<SelectTrigger className="bg-background border-border text-xs">
										<SelectValue placeholder="All Privacy Modes" />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">All Modes</SelectItem>
										<SelectItem value="true">Incognito / InPrivate Only</SelectItem>
										<SelectItem value="false">Normal Browsing Only</SelectItem>
									</SelectContent>
								</Select>
							</div>
						</CardHeader>

						<CardContent>
							<div className="rounded-md border border-border overflow-x-auto">
								<Table className="w-full min-w-[980px]">
									<TableHeader>
										<TableRow className="border-border hover:bg-transparent">
											<TableHead className="w-[40px]">
												<Checkbox
													aria-label="Select all search logs on this page"
													checked={searchLogs.length > 0 && searchLogs.every((l) => selectedSearchLogIds.has(l.id))}
													onCheckedChange={(on) =>
														setSelectedSearchLogIds(on === true ? new Set(searchLogs.map((l) => l.id)) : new Set())
													}
												/>
											</TableHead>
											<TableHead className="w-[100px]">Date</TableHead>
											<TableHead className="w-[100px]">Time</TableHead>
											<TableHead className="w-[140px]">Desktop Name</TableHead>
											<TableHead className="w-[130px]">Search Engine</TableHead>
											<TableHead className="w-[90px]">Browser</TableHead>
											<TableHead className="w-[150px]">Privacy Mode</TableHead>
											<TableHead className="w-[auto]">Search Query / Prompt</TableHead>
											<TableHead className="w-[200px]">Clicked Result Link</TableHead>
											<TableHead className="w-[140px]">Threat Risk</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{searchLogs.length === 0 ? (
											<TableRow>
												<TableCell colSpan={10} className="h-32 text-center text-muted-foreground">
													<div className="flex flex-col items-center justify-center gap-2">
														<Search className="h-6 w-6 text-muted-foreground/50" />
														<p>No search events logged yet.</p>
														<p className="text-xs text-muted-foreground/70">
															Searches in Google, Bing, DuckDuckGo, or Yahoo from Chrome/Edge/Firefox/Brave appear here in real-time.
														</p>
													</div>
												</TableCell>
											</TableRow>
										) : (
											searchLogs.map((log) => {
												const e = log.engine.toLowerCase();
												return (
													<TableRow
														key={log.id}
														className="border-border hover:bg-muted/30 cursor-pointer"
														onClick={() => setSelectedSearchLog(log)}
													>
														<TableCell onClick={(ev) => ev.stopPropagation()}>
															<Checkbox
																aria-label="Select search log"
																checked={selectedSearchLogIds.has(log.id)}
																onCheckedChange={(on) => toggleId(setSelectedSearchLogIds, log.id, on === true)}
															/>
														</TableCell>
														<TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
															{formatLogDate(log.timestamp)}
														</TableCell>
														<TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
															{formatLogTime(log.timestamp)}
														</TableCell>
														<TableCell className="font-mono text-xs">
															<span className="font-medium text-foreground">{log.agent_hostname || "—"}</span>
															{log.client_ip && log.client_ip !== log.agent_hostname && (
																<div className="text-[10px] text-muted-foreground">{log.client_ip}</div>
															)}
														</TableCell>
														<TableCell>
															{e.includes("google") ? (
																<Badge className="bg-blue-950/80 text-blue-300 border-blue-800/80 gap-1 font-medium text-xs">
																	<Globe className="h-3 w-3 text-blue-400" /> Google
																</Badge>
															) : e.includes("bing") ? (
																<Badge className="bg-cyan-950/80 text-cyan-300 border-cyan-800/80 gap-1 font-medium text-xs">
																	<Compass className="h-3 w-3 text-cyan-400" /> Bing
																</Badge>
															) : e.includes("safari") || e.includes("apple") ? (
																<Badge className="bg-sky-950/80 text-sky-300 border-sky-800/80 gap-1 font-medium text-xs">
																	<Compass className="h-3 w-3 text-sky-400" /> Safari
																</Badge>
															) : e.includes("duck") ? (
																<Badge className="bg-amber-950/80 text-amber-300 border-amber-800/80 gap-1 font-medium text-xs">
																	<Globe className="h-3 w-3 text-amber-400" /> DuckDuckGo
																</Badge>
															) : e.includes("brave") ? (
																<Badge className="bg-orange-950/80 text-orange-300 border-orange-800/80 gap-1 font-medium text-xs">
																	<Globe className="h-3 w-3 text-orange-400" /> Brave Search
																</Badge>
															) : (
																<Badge className="bg-purple-950/80 text-purple-300 border-purple-800/80 gap-1 font-medium text-xs">
																	<Globe className="h-3 w-3 text-purple-400" /> Yahoo
																</Badge>
															)}
														</TableCell>
														<TableCell>
															<Badge variant="outline" className="text-xs bg-black/20">
																{log.browser}
															</Badge>
														</TableCell>
														<TableCell>
															{log.is_incognito ? (
																<Badge className="bg-purple-950/80 text-purple-300 border-purple-800/80 gap-1 font-medium text-xs">
																	<EyeOff className="h-3 w-3 text-purple-400" /> Incognito / InPrivate
																</Badge>
															) : (
																<Badge variant="outline" className="text-muted-foreground gap-1 text-xs">
																	<Eye className="h-3 w-3" /> Normal
																</Badge>
															)}
														</TableCell>
														<TableCell>
															{log.query ? (
																<div className="flex items-start gap-1.5 font-medium text-xs text-foreground max-w-[360px]">
																	<Search className="h-3.5 w-3.5 text-muted-foreground shrink-0 mt-0.5" />
																	<span className="break-words line-clamp-2">{log.query}</span>
																</div>
															) : (
																<span className="text-xs italic text-muted-foreground flex items-center gap-1">
																	<ExternalLink className="h-3 w-3" /> [Result Click Navigation]
																</span>
															)}
														</TableCell>
														<TableCell>
															{log.clicked_url ? (
																<a
																	href={log.clicked_url}
																	target="_blank"
																	rel="noopener noreferrer"
																	className="inline-flex items-center gap-1 text-xs text-blue-400 hover:text-blue-300 hover:underline max-w-[200px] truncate"
																	title={log.clicked_url}
																	onClick={(e) => e.stopPropagation()}
																>
																	<ExternalLink className="h-3 w-3 shrink-0" />
																	<span className="truncate">{log.clicked_title || log.clicked_url}</span>
																</a>
															) : (
																<span className="text-xs text-muted-foreground">—</span>
															)}
														</TableCell>
														<TableCell>
															<div className="space-y-0.5">
																{log.predictive_risk === "CRITICAL" ? (
																	<Badge className="bg-red-950/80 text-red-400 border-red-800/80 text-[10px] font-semibold">
																		CRITICAL ({log.risk_score}%)
																	</Badge>
																) : log.predictive_risk === "HIGH" ? (
																	<Badge className="bg-amber-950/80 text-amber-400 border-amber-800/80 text-[10px] font-semibold">
																		HIGH ({log.risk_score}%)
																	</Badge>
																) : log.predictive_risk === "MEDIUM" ? (
																	<Badge className="bg-yellow-950/80 text-yellow-400 border-yellow-800/80 text-[10px] font-semibold">
																		MEDIUM ({log.risk_score}%)
																	</Badge>
																) : (
																	<Badge variant="outline" className="text-muted-foreground text-[10px]">
																		LOW ({log.risk_score}%)
																	</Badge>
																)}
																<div className="text-[10px] text-muted-foreground">{log.risk_category || "General"}</div>
															</div>
														</TableCell>
													</TableRow>
												);
											})
										)}
									</TableBody>
								</Table>
							</div>

							{/* Search Logs pagination */}
							<div className="flex flex-col sm:flex-row justify-between items-center gap-4 mt-4 text-xs text-muted-foreground">
								<div className="flex items-center gap-2">
									<span>Rows per page</span>
									<Select
										value={searchLogPageLimit.toString()}
										onValueChange={(val) => {
											setSearchLogPageLimit(Number(val));
											setSearchLogPageOffset(0);
										}}
									>
										<SelectTrigger className="h-8 w-[70px] bg-background border-border">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="10">10</SelectItem>
											<SelectItem value="25">25</SelectItem>
											<SelectItem value="50">50</SelectItem>
											<SelectItem value="100">100</SelectItem>
										</SelectContent>
									</Select>
									<span>
										Showing {totalSearchLogs > 0 ? searchLogPageOffset + 1 : 0} to{" "}
										{Math.min(searchLogPageOffset + searchLogPageLimit, totalSearchLogs)} of {totalSearchLogs} entries
									</span>
								</div>
								<div className="flex items-center gap-2">
									<span>
										Page {searchLogCurrentPage} of {searchLogTotalPages}
									</span>
									<div className="flex items-center gap-1">
										<Button
											variant="outline"
											size="icon"
											disabled={searchLogPageOffset === 0}
											onClick={() => setSearchLogPageOffset(Math.max(0, searchLogPageOffset - searchLogPageLimit))}
											className="h-8 w-8 border-border"
											aria-label="Previous search logs page"
										>
											<ChevronLeft className="h-4 w-4" />
										</Button>
										<Button
											variant="outline"
											size="icon"
											disabled={searchLogPageOffset + searchLogPageLimit >= totalSearchLogs}
											onClick={() => setSearchLogPageOffset(searchLogPageOffset + searchLogPageLimit)}
											className="h-8 w-8 border-border"
											aria-label="Next search logs page"
										>
											<ChevronRight className="h-4 w-4" />
										</Button>
									</div>
								</div>
							</div>
						</CardContent>
					</Card>
				</TabsContent>

				{/* TAB 3: GUARD RULES */}
				<TabsContent value="rules" className="space-y-4">
					{/* Browser Interaction Controls */}
					<Card className="bg-card border-border overflow-hidden">
						<CardHeader className="pb-4">
							<div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
								<div className="space-y-1">
									<div className="flex flex-wrap items-center gap-2">
										<CardTitle className="text-lg">File upload policy</CardTitle>
										{controls.enabled && controls.block_upload ? (
											<Badge
												variant="outline"
												className="border-rose-700/70 bg-rose-950/40 text-rose-400"
											>
												Block all uploads
											</Badge>
										) : controls.enabled && !controls.block_upload ? (
											<Badge
												variant="outline"
												className="border-emerald-700/70 bg-emerald-950/40 text-emerald-400"
											>
												Rules-based DLP
											</Badge>
										) : (
											<Badge variant="outline" className="border-border text-muted-foreground">
												Paused
											</Badge>
										)}
									</div>
									<CardDescription>
										Choose how file attachments are controlled on monitored AI sites. Select whether to block all files or enforce rules-based DLP inspection.
									</CardDescription>
								</div>
							</div>
						</CardHeader>
						<CardContent className="pt-0">
							<div className="overflow-hidden rounded-lg border border-border divide-y divide-border">
								{/* Option 1: Block all uploads */}
								<div
									className={`flex items-center justify-between gap-4 px-4 py-3.5 transition-colors ${controls.enabled && controls.block_upload ? "bg-rose-950/20" : "bg-card"
										}`}
								>
									<div className="min-w-0 flex items-start gap-3">
										<div
											className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md border ${controls.enabled && controls.block_upload
													? "border-rose-800/60 bg-rose-950/50 text-rose-400"
													: "border-border bg-background text-muted-foreground"
												}`}
										>
											<Upload className="h-4 w-4" />
										</div>
										<div className="min-w-0 space-y-1">
											<div className="flex flex-wrap items-center gap-2">
												<p className="text-sm font-semibold">Block all uploads</p>
												{controls.enabled && controls.block_upload ? (
													<Badge className="bg-rose-950/80 text-rose-300 border-rose-800/70 text-[10px] px-2 py-0.5">
														Active: All files blocked
													</Badge>
												) : (
													<Badge variant="outline" className="text-[10px] px-2 py-0.5 text-muted-foreground">
														Disabled
													</Badge>
												)}
											</div>
											<p className="text-xs text-muted-foreground leading-relaxed">
												Every file or attachment uploaded to AI chats is completely blocked, regardless of file content.
											</p>
										</div>
									</div>
									<Switch
										checked={!!(controls.enabled && controls.block_upload)}
										onCheckedChange={(val) => {
											if (val) {
												patchControl({ enabled: true, block_upload: true });
											} else {
												patchControl({ enabled: false, block_upload: false });
											}
										}}
										aria-label="Block all uploads"
									/>
								</div>

								{/* Option 2: Rules-based upload block */}
								<div
									className={`flex items-center justify-between gap-4 px-4 py-3.5 transition-colors ${controls.enabled && !controls.block_upload ? "bg-emerald-950/20" : "bg-card"
										}`}
								>
									<div className="min-w-0 flex items-start gap-3">
										<div
											className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md border ${controls.enabled && !controls.block_upload
													? "border-emerald-800/60 bg-emerald-950/50 text-emerald-400"
													: "border-border bg-background text-muted-foreground"
												}`}
										>
											<ShieldCheck className="h-4 w-4" />
										</div>
										<div className="min-w-0 space-y-1">
											<div className="flex flex-wrap items-center gap-2">
												<p className="text-sm font-semibold">Rules-based upload block</p>
												{controls.enabled && !controls.block_upload ? (
													<Badge className="bg-emerald-950/80 text-emerald-300 border-emerald-800/70 text-[10px] px-2 py-0.5">
														Active: Guard Rules DLP
													</Badge>
												) : (
													<Badge variant="outline" className="text-[10px] px-2 py-0.5 text-muted-foreground">
														Disabled
													</Badge>
												)}
											</div>
											<p className="text-xs text-muted-foreground leading-relaxed">
												Allow clean files. Inspect uploaded files (PDF, Word, Excel, text, OCR) against active Guard Rules and block or redact only policy violations.
											</p>
										</div>
									</div>
									<Switch
										checked={!!(controls.enabled && !controls.block_upload)}
										onCheckedChange={(val) => {
											if (val) {
												patchControl({ enabled: true, block_upload: false });
											} else {
												patchControl({ enabled: false, block_upload: false });
											}
										}}
										aria-label="Rules-based upload block"
									/>
								</div>

								<div className="space-y-2 bg-card px-4 py-3.5">
									<div className="flex items-center justify-between gap-2">
										<Label>Upload policy warning</Label>
										{!uploadWarningEditing && (controls.upload_warning || "").trim() ? (
											<Button
												type="button"
												size="sm"
												variant="ghost"
												className="h-8 shrink-0 text-muted-foreground hover:text-foreground"
												onClick={() => {
													setUploadWarningError("");
													setUploadWarningDraft(controls.upload_warning || "");
													setUploadWarningEditing(true);
												}}
											>
												<Pencil className="h-3.5 w-3.5 mr-1.5" />
												Edit
											</Button>
										) : null}
									</div>
									{uploadWarningEditing || !(controls.upload_warning || "").trim() ? (
										<>
											<Textarea
												placeholder="e.g. UPLOAD BLOCK — shown in Prompt Logs and to employees..."
												value={uploadWarningDraft}
												onChange={(e) => setUploadWarningDraft(e.target.value)}
												rows={3}
											/>
											<div className="flex items-center justify-between gap-2">
												<p className="text-xs text-muted-foreground">
													Block all uploads → this text in Prompt Logs. A Guard Rule hit inside a file → this text (or that rule&apos;s warning) plus &quot; -- policy name&quot;. Leave blank to use &quot;Upload block&quot;.
												</p>
												<div className="flex items-center gap-2 shrink-0">
													{uploadWarningEditing && (controls.upload_warning || "").trim() ? (
														<Button
															type="button"
															size="sm"
															variant="ghost"
															disabled={uploadWarningSaving}
															onClick={() => {
																setUploadWarningError("");
																setUploadWarningDraft(controls.upload_warning || "");
																setUploadWarningEditing(false);
															}}
														>
															Cancel
														</Button>
													) : null}
													<Button
														type="button"
														size="sm"
														variant="outline"
														className="shrink-0"
														disabled={uploadWarningSaving}
														onClick={saveUploadWarning}
													>
														<Save className="h-3.5 w-3.5 mr-1.5" />
														{uploadWarningSaving ? "Saving..." : "Save warning"}
													</Button>
												</div>
											</div>
											{uploadWarningError ? (
												<p className="text-xs text-destructive">{uploadWarningError}</p>
											) : null}
										</>
									) : (
										<div className="rounded-md border border-amber-300/70 bg-amber-50 px-3 py-2 dark:border-amber-800/40 dark:bg-amber-950/20">
											<p className="text-xs text-amber-950 whitespace-pre-wrap break-words dark:text-amber-100">
												{(controls.upload_warning || "").trim()}
											</p>
										</div>
									)}
								</div>
							</div>
						</CardContent>
					</Card>

					<Card className="bg-card border-border">
						<CardHeader>
							<div className="flex flex-col lg:flex-row justify-between items-start lg:items-center gap-4">
								<div className="min-w-0 flex-1">
									<CardTitle className="text-lg">DLP Guard Rules ({rules.length} Configured)</CardTitle>
									<CardDescription>
										Only rules you create here apply on monitored Target Websites. Toggle Active to pause a rule without deleting it.
									</CardDescription>
								</div>
								<div className="flex shrink-0 flex-wrap lg:flex-nowrap items-center gap-2">
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={downloadGuardRulesTemplate}
										className="h-9 gap-1.5 text-xs text-muted-foreground hover:text-foreground"
										title="Download Excel spreadsheet template for rules"
									>
										<Download className="h-4 w-4" /> Download Template
									</Button>
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={() => setRuleImportDialogOpen(true)}
										className="h-9 gap-1.5 text-xs border-emerald-500/30 text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/10"
										title="Import Guard Rules from Excel (.xlsx, .xls) or CSV"
									>
										<FileSpreadsheet className="h-4 w-4 text-emerald-400" /> Import Rules
									</Button>
									<Dialog open={ruleDialogOpen} onOpenChange={setRuleDialogOpen}>
									<DialogTrigger asChild>
										<Button size="sm" className="h-9 gap-1.5 text-xs">
											<Plus className="h-4 w-4" /> Add Rule
										</Button>
									</DialogTrigger>
									<DialogContent className="bg-card border-border text-foreground w-[calc(100%-2rem)] sm:max-w-xl max-h-[88vh] flex flex-col p-0 overflow-hidden">
										<DialogHeader className="p-5 pb-3 shrink-0 border-b border-border/60">
											<DialogTitle className="flex items-center gap-2 text-base">
												<Shield className="h-5 w-5 text-primary" />
												Create Guard Rule
											</DialogTitle>
											<DialogDescription className="text-xs">
												Add your own regex or AI policy. Raksha does not ship default guard patterns — only what you save here is enforced.
											</DialogDescription>
										</DialogHeader>

										{ruleError && <div className="mx-5 mt-3 p-3 bg-red-950/60 border border-red-800 text-red-400 rounded-md text-xs">{ruleError}</div>}

										<div className="flex-1 overflow-y-auto overflow-x-hidden px-5 py-4 space-y-4 min-w-0 no-scrollbar">
											{/* Rule Engine Type Toggle */}
											<div className="space-y-1.5">
												<Label>Rule Engine Type</Label>
												<div className="grid grid-cols-2 gap-2 p-1 bg-muted/40 rounded-lg border border-border">
													<button
														type="button"
														onClick={() => {
															setNewRuleType("regex");
														}}
														className={`flex items-center justify-center gap-2 py-2 px-3 rounded-md text-xs font-semibold transition-all ${newRuleType === "regex"
																? "bg-primary text-primary-foreground shadow-sm"
																: "text-muted-foreground hover:text-foreground"
															}`}
													>
														<Zap className="h-3.5 w-3.5" />
														Regex Pattern Rule
													</button>
													<button
														type="button"
														onClick={() => {
															setNewRuleType("ai_bot");
															setNewRuleBotProvider(GUARD_BOT_OLLAMA_PROVIDER);
															setNewRuleBotModel(GUARD_BOT_OLLAMA_MODEL);
														}}
														className={`flex items-center justify-center gap-2 py-2 px-3 rounded-md text-xs font-semibold transition-all ${newRuleType === "ai_bot"
																? "bg-purple-600 text-white shadow-sm"
																: "text-muted-foreground hover:text-foreground"
															}`}
													>
														<Bot className="h-3.5 w-3.5" />
														AI Guard Bot (Prompt Rule)
													</button>
												</div>
											</div>

											<div className="space-y-1.5">
												<Label>Rule Name</Label>
												<Input
													placeholder="Rule name"
													value={newRuleName}
													onChange={(e) => setNewRuleName(e.target.value)}
												/>
											</div>

											<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
												<div className="space-y-1.5 min-w-0">
													<Label>Severity</Label>
													<Select value={newRuleSeverity} onValueChange={(v: any) => setNewRuleSeverity(v)}>
														<SelectTrigger className="w-full">
															<SelectValue />
														</SelectTrigger>
														<SelectContent>
															<SelectItem value="CRITICAL">CRITICAL</SelectItem>
															<SelectItem value="HIGH">HIGH</SelectItem>
															<SelectItem value="MEDIUM">MEDIUM</SelectItem>
														</SelectContent>
													</Select>
												</div>
												<div className="space-y-1.5 min-w-0">
													<Label>Action</Label>
													<Select value={newRuleAction} onValueChange={(v: any) => setNewRuleAction(v)}>
														<SelectTrigger className="w-full">
															<SelectValue />
														</SelectTrigger>
														<SelectContent>
															<SelectItem value="BLOCK">BLOCK</SelectItem>
															<SelectItem value="REDACT">REDACT</SelectItem>
															<SelectItem value="WARN">WARN</SelectItem>
														</SelectContent>
													</Select>
													<p className="text-[11px] text-muted-foreground break-words">
														{guardRuleActionHint(newRuleAction)}
													</p>
												</div>
											</div>

											{newRuleType === "regex" ? (
												<div className="space-y-1.5">
													<Label>Regex Pattern</Label>
													<Input
														placeholder="Enter the regex you want to match"
														value={newRulePattern}
														onChange={(e) => setNewRulePattern(e.target.value)}
													/>
													<p className="text-[11px] text-muted-foreground">
														One RE2 regex per rule. Empty form = no rule. Only patterns you save here are enforced (no built-in list).
													</p>
													<RegexLiveTestPanel pattern={newRulePattern} />
												</div>
											) : (
												<GuardRuleAIEvaluatorFields
													botProvider={newRuleBotProvider}
													botModel={newRuleBotModel}
													botPrompt={newRuleBotPrompt}
													referenceImagePreview={newRuleBotReferenceImagePreview}
													evalMode={newRuleBotEvalMode}
													generatedPattern={newRuleGeneratedPattern}
													generateError={newRuleGenerateError}
													generating={generatingRegex}
													outsourceProviderOptions={outsourceProviderOptions}
													onProviderChange={setNewRuleBotProvider}
													onModelChange={setNewRuleBotModel}
													onPromptChange={setNewRuleBotPrompt}
													onEvalModeChange={setNewRuleBotEvalMode}
													onGeneratedPatternChange={setNewRuleGeneratedPattern}
													onGenerateRegex={() => runGenerateRegex("new")}
													onTestEvaluate={() => runTestEvaluate("new")}
													testSample={newRuleTestSample}
													onTestSampleChange={setNewRuleTestSample}
													testResult={newRuleTestResult}
													testing={testingGuardBot}
													onReferenceImageClear={() => {
														setNewRuleBotReferenceImage("");
														setNewRuleBotReferenceImageType("");
														setNewRuleBotReferenceImagePreview("");
													}}
													onReferenceImageChange={async (file) => {
														try {
															const { data, type } = await readReferenceImageFile(file);
															setNewRuleBotReferenceImage(data);
															setNewRuleBotReferenceImageType(type);
															setNewRuleBotReferenceImagePreview(referenceImageDataUrl(data, type));
														} catch (err: any) {
															setRuleError(err?.message || "Failed to load reference image.");
														}
													}}
												/>
											)}

											<div className="space-y-1.5">
												<Label>Description</Label>
												<Textarea placeholder="Rule context and usage..." value={newRuleDescription} onChange={(e) => setNewRuleDescription(e.target.value)} />
											</div>
											<div className="space-y-1.5">
												<Label>{guardRuleNoticeCopy(newRuleAction).label}</Label>
												<Textarea
													placeholder={guardRuleNoticeCopy(newRuleAction).placeholder}
													value={newRuleWarningMessage}
													onChange={(e) => setNewRuleWarningMessage(e.target.value)}
													rows={3}
												/>
												<p className="text-xs text-muted-foreground break-words">
													{guardRuleNoticeCopy(newRuleAction).hint}
												</p>
											</div>
										</div>
										<DialogFooter className="p-4 px-5 shrink-0 border-t border-border/60 bg-card">
											<Button variant="outline" onClick={() => setRuleDialogOpen(false)}>
												Cancel
											</Button>
											<Button onClick={handleCreateRule}>Create Guard Rule</Button>
										</DialogFooter>
									</DialogContent>
								</Dialog>
								</div>
							</div>

							<div className="relative mt-3">
								<Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
								<Input placeholder="Search guard rules..." value={ruleSearch} onChange={(e) => setRuleSearch(e.target.value)} className="pl-9 bg-background border-border" />
							</div>
						</CardHeader>
						<CardContent>
							<div className="space-y-3">
								{pagedRules.map((rule) => (
									<div
										key={rule.id}
										className="rounded-xl border border-border bg-background/40 p-4 hover:border-primary/30 transition-colors"
									>
										<div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
											<div className="min-w-0 flex-1 space-y-2">
												<div className="flex flex-wrap items-center gap-2">
													<h4 className="text-sm font-semibold text-foreground">{rule.name}</h4>
													{rule.rule_type === "ai_bot" ? (
														<Badge className="bg-purple-950/90 text-purple-300 border-purple-700 gap-1 text-[11px] font-semibold">
															<Bot className="h-3 w-3 text-purple-400" /> AI GUARD BOT
														</Badge>
													) : (
														<Badge className="bg-cyan-950/90 text-cyan-300 border-cyan-700 gap-1 text-[11px] font-semibold">
															<Zap className="h-3 w-3 text-cyan-400" /> REGEX RULE
														</Badge>
													)}
													<Badge
														className={
															rule.severity === "CRITICAL"
																? "bg-red-950/80 text-red-400 border-red-800"
																: rule.severity === "HIGH"
																	? "bg-amber-950/80 text-amber-400 border-amber-800"
																	: "bg-blue-950/80 text-blue-300 border-blue-800"
														}
													>
														{rule.severity}
													</Badge>
													{rule.action === "BLOCK" ? (
														<Badge className="bg-red-950/80 text-red-400 border-red-700 gap-1 text-[11px]">
															<AlertTriangle className="h-3 w-3" /> BLOCK
														</Badge>
													) : rule.action === "WARN" ? (
														<Badge className="bg-yellow-950/80 text-yellow-300 border-yellow-700 gap-1 text-[11px]">
															<AlertCircle className="h-3 w-3" /> WARN
														</Badge>
													) : (
														<Badge className="bg-amber-950/80 text-amber-300 border-amber-700 gap-1 text-[11px]">
															<AlertCircle className="h-3 w-3" /> REDACT
														</Badge>
													)}
												</div>

												<p className="text-xs text-muted-foreground leading-relaxed break-words">
													{rule.description || "No description provided."}
												</p>

												{rule.warning_message ? (
													<div className="rounded-md border border-amber-300/70 bg-amber-50 px-3 py-2 dark:border-amber-800/40 dark:bg-amber-950/20">
														<p className="text-[10px] uppercase tracking-wide text-amber-800 mb-1 dark:text-amber-300/80">
															{guardRuleNoticeCopy(
																rule.action === "BLOCK" ? "BLOCK" : rule.action === "WARN" ? "WARN" : "REDACT",
															).listLabel}
														</p>
														<p className="text-xs text-amber-950 whitespace-pre-wrap break-words dark:text-amber-100">
															{rule.warning_message}
														</p>
													</div>
												) : null}

												{rule.rule_type === "ai_bot" ? (
													<div className="rounded-md border border-purple-300/70 bg-purple-50 px-3 py-2 space-y-1 dark:border-purple-900/40 dark:bg-purple-950/20">
														{!rule.bot_prompt && !rule.bot_reference_image ? (
															<p className="text-xs text-red-700 font-medium dark:text-red-300">
																Incomplete — set the Security Policy prompt and/or reference template, then Save.
															</p>
														) : null}
														<div className="flex items-center justify-between text-[10px] uppercase tracking-wide text-purple-800 dark:text-purple-300/80">
															<span>AI Security Policy (Prompt)</span>
															<Badge variant="outline" className="text-[10px] py-0 px-1.5 text-purple-800 border-purple-300 dark:text-purple-300 dark:border-purple-800">
																{rule.bot_provider || GUARD_BOT_OLLAMA_PROVIDER} / {rule.bot_model || GUARD_BOT_OLLAMA_MODEL}
															</Badge>
														</div>
														<p className="text-xs text-purple-950 whitespace-pre-wrap break-words font-mono dark:text-purple-100">
															{rule.bot_prompt || "(empty)"}
														</p>
													</div>
												) : (
													<div className="rounded-md border border-border bg-muted/30 px-3 py-2">
														<p className="text-[10px] uppercase tracking-wide text-muted-foreground mb-1">Regex Pattern</p>
														<code className="block text-xs font-mono text-emerald-800 whitespace-pre-wrap break-all dark:text-emerald-400">
															{rule.pattern}
														</code>
													</div>
												)}
											</div>

											<div className="flex items-center justify-between gap-3 lg:flex-col lg:items-end lg:justify-start shrink-0 border-t border-border pt-3 lg:border-t-0 lg:pt-0 lg:pl-4">
												<div className="flex items-center gap-2">
													<span className={`text-xs font-medium ${rule.active ? "text-emerald-700 dark:text-emerald-400" : "text-muted-foreground"}`}>
														{rule.active ? "Active" : "Disabled"}
													</span>
													<Switch
														checked={rule.active}
														onCheckedChange={(val) => updateRule({ id: rule.id, updates: { active: val } })}
													/>
												</div>
												<div className="flex items-center gap-1">
													<Button
														variant="ghost"
														size="icon"
														onClick={() => {
															setEditRule(rule);
															setEditRuleName(rule.name);
															setEditRuleType(rule.rule_type === "ai_bot" ? "ai_bot" : "regex");
															setEditRuleBotProvider(rule.bot_provider || GUARD_BOT_OLLAMA_PROVIDER);
															setEditRuleBotModel(rule.bot_model || GUARD_BOT_OLLAMA_MODEL);
															setEditRuleBotPrompt(rule.bot_prompt || "");
															setEditRuleBotReferenceImage(rule.bot_reference_image || "");
															setEditRuleBotReferenceImageType(rule.bot_reference_image_type || "");
															setEditRuleBotReferenceImagePreview(
																rule.bot_reference_image
																	? referenceImageDataUrl(rule.bot_reference_image, rule.bot_reference_image_type || "image/png")
																	: "",
															);
															setEditRuleSeverity(rule.severity);
															setEditRuleAction(
																rule.action === "BLOCK" ? "BLOCK" : rule.action === "WARN" ? "WARN" : "REDACT",
															);
															setEditRulePattern(rule.pattern || "");
															setEditRuleDescription(rule.description || "");
															setEditRuleWarningMessage(rule.warning_message || "");
															setEditRuleBotEvalMode("ai");
															setEditRuleGeneratedPattern(rule.pattern || "");
															setEditRuleGenerateError("");
															setEditRuleDialogOpen(true);
														}}
														className="h-8 w-8 text-muted-foreground hover:text-foreground"
														title="Edit Guard Rule"
													>
														<Pencil className="h-4 w-4" />
													</Button>
													<Button
														variant="ghost"
														size="icon"
														onClick={() => deleteRule(rule.id)}
														className="h-8 w-8 text-muted-foreground hover:text-destructive"
														title="Delete Guard Rule"
													>
														<Trash2 className="h-4 w-4" />
													</Button>
												</div>
											</div>
										</div>
									</div>
								))}

								{filteredRules.length === 0 && (
									<div className="rounded-xl border border-dashed border-border py-12 text-center text-sm text-muted-foreground">
										No guard rules found matching your search.
									</div>
								)}

								{filteredRules.length > 0 && (
									<div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-2 border-t border-border">
										<div className="flex items-center gap-2 text-xs text-muted-foreground">
											<span>Rows per page</span>
											<Select
												value={rulesPageLimit.toString()}
												onValueChange={(v) => {
													setRulesPageLimit(Number(v));
													setRulesPageOffset(0);
												}}
											>
												<SelectTrigger className="h-8 w-[72px]">
													<SelectValue />
												</SelectTrigger>
												<SelectContent>
													<SelectItem value="10">10</SelectItem>
													<SelectItem value="25">25</SelectItem>
													<SelectItem value="50">50</SelectItem>
												</SelectContent>
											</Select>
											<span>
												Showing {filteredRules.length ? rulesPageOffset + 1 : 0}–
												{Math.min(rulesPageOffset + rulesPageLimit, filteredRules.length)} of {filteredRules.length}
											</span>
										</div>
										<div className="flex items-center gap-2">
											<span className="text-xs text-muted-foreground">
												Page {rulesCurrentPage} of {rulesTotalPages}
											</span>
											<Button
												variant="outline"
												size="sm"
												className="h-8"
												disabled={rulesPageOffset <= 0}
												onClick={() => setRulesPageOffset(Math.max(0, rulesPageOffset - rulesPageLimit))}
											>
												<ChevronLeft className="h-4 w-4" />
											</Button>
											<Button
												variant="outline"
												size="sm"
												className="h-8"
												disabled={rulesPageOffset + rulesPageLimit >= filteredRules.length}
												onClick={() => setRulesPageOffset(rulesPageOffset + rulesPageLimit)}
											>
												<ChevronRight className="h-4 w-4" />
											</Button>
										</div>
									</div>
								)}
							</div>
						</CardContent>
					</Card>

					<GuardRuleImportDialog
						open={ruleImportDialogOpen}
						onOpenChange={setRuleImportDialogOpen}
						onImportSuccess={refetchRules}
					/>
				</TabsContent>

				{/* TAB 4: TARGET WEBSITES */}
				<TabsContent value="targets" className="space-y-4">
					<Card className="bg-card border-border">
						<CardHeader>
							<div className="flex flex-col lg:flex-row justify-between items-start lg:items-center gap-4">
								<div className="min-w-0 flex-1">
									<CardTitle className="text-lg">Target Web AI Platforms ({targets.length} Monitored)</CardTitle>
									<CardDescription>
										Monitor prompts on these sites, or turn on <strong>Block entire website</strong> to lock one.
									</CardDescription>
								</div>
								<div className="flex shrink-0 flex-wrap lg:flex-nowrap items-center gap-2">
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={downloadTargetsTemplate}
										className="h-9 gap-1.5 text-xs text-muted-foreground hover:text-foreground"
										title="Download Excel spreadsheet template for target websites"
									>
										<Download className="h-4 w-4" /> Download Template
									</Button>
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={() => setTargetImportDialogOpen(true)}
										className="h-9 gap-1.5 text-xs border-emerald-500/30 text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/10"
										title="Import Target Websites from Excel (.xlsx, .xls) or CSV"
									>
										<FileSpreadsheet className="h-4 w-4 text-emerald-400" /> Import Targets
									</Button>
									<Dialog
										open={targetDialogOpen}
										onOpenChange={(open) => {
											setTargetDialogOpen(open);
											if (!open) {
												setCustomRelatedHosts([{ host: "", role: "" }]);
												setTargetError("");
											}
										}}
									>
										<DialogTrigger asChild>
											<Button size="sm" className="h-9 gap-1.5 text-xs">
												<Plus className="h-4 w-4" /> Add Target Domain
											</Button>
										</DialogTrigger>
									<DialogContent className="bg-card border-border text-foreground">
										<DialogHeader>
											<DialogTitle>Add Target Web Domain</DialogTitle>
											<DialogDescription>
												Any domain you add gets the same rules: exact prompt logging, DLP guardrails, and optional upload blocking. Enter hostname only (no https://).
											</DialogDescription>
										</DialogHeader>

										{targetError && <div className="p-3 bg-red-950/60 border border-red-800 text-red-400 rounded-md text-xs">{targetError}</div>}

										<div className="space-y-4 py-3">
											<div className="space-y-2">
												<Label>Domain Name</Label>
												<Input
													placeholder="e.g. chat.example.com"
													value={newTargetDomain}
													onChange={(e) => setNewTargetDomain(e.target.value)}
												/>
												<p className="text-[11px] text-muted-foreground">
													Subdomains are covered automatically. Label each host: Main UI, Chat domain, or File domain so Guard knows what to intercept.
												</p>
											</div>
											<div className="space-y-2">
												<Label>Host role (main domain)</Label>
												<Select value={newTargetHostRole || "auto"} onValueChange={(v) => setNewTargetHostRole(v === "auto" ? "" : (v as HostRole))}>
													<SelectTrigger>
														<SelectValue placeholder="Auto" />
													</SelectTrigger>
													<SelectContent>
														{HOST_ROLE_OPTIONS.map((opt) => (
															<SelectItem key={opt.value || "auto"} value={opt.value || "auto"}>
																{opt.label}
															</SelectItem>
														))}
													</SelectContent>
												</Select>
											</div>
											<div className="space-y-2 rounded-md border border-border p-3">
												<p className="text-sm font-medium">Add related host</p>
												<p className="text-[11px] text-muted-foreground">
													Add related hosts with a role: Chat domain (prompts), File domain (uploads). Leave Auto if unsure.
												</p>
												{newTargetRelatedGroup ? (
													<div className="space-y-1.5 rounded-md border border-dashed border-border bg-muted/20 p-2">
														<p className="text-[11px] font-medium">{newTargetRelatedGroup.label}</p>
														<p className="text-[10px] text-muted-foreground">{newTargetRelatedGroup.reason}</p>
														<div className="flex flex-wrap gap-1.5 pt-1">
															{newTargetRelatedGroup.hosts.map((host) => {
																const picked = customRelatedHosts.some((v) => normalizeTargetDomain(v.host) === host);
																const already = addedTargetDomains.some((d) => normalizeTargetDomain(d) === host);
																return (
																	<Button
																		key={host}
																		type="button"
																		size="sm"
																		variant={picked || already ? "secondary" : "outline"}
																		className="h-6 px-2 text-[10px] font-mono"
																		disabled={already}
																		onClick={() => fillSuggestedRelatedHost(host)}
																	>
																		{already ? host : picked ? host : `+ ${host}`}
																	</Button>
																);
															})}
														</div>
													</div>
												) : null}
												<div className="space-y-2 pt-1">
													{customRelatedHosts.map((entry, idx) => (
														<div key={idx} className="flex items-center gap-2">
															<Input
																placeholder="e.g. docs.example.com"
																className="font-mono text-sm flex-1"
																value={entry.host}
																onChange={(e) => {
																	const next = [...customRelatedHosts];
																	next[idx] = { ...next[idx], host: e.target.value };
																	setCustomRelatedHosts(next);
																}}
															/>
															<Select
																value={entry.role || "auto"}
																onValueChange={(v) => {
																	const next = [...customRelatedHosts];
																	next[idx] = { ...next[idx], role: v === "auto" ? "" : (v as HostRole) };
																	setCustomRelatedHosts(next);
																}}
															>
																<SelectTrigger className="w-[130px]">
																	<SelectValue />
																</SelectTrigger>
																<SelectContent>
																	{HOST_ROLE_OPTIONS.map((opt) => (
																		<SelectItem key={opt.value || "auto"} value={opt.value || "auto"}>
																			{opt.label}
																		</SelectItem>
																	))}
																</SelectContent>
															</Select>
															{customRelatedHosts.length > 1 && (
																<Button
																	type="button"
																	variant="ghost"
																	size="icon"
																	onClick={() => setCustomRelatedHosts(customRelatedHosts.filter((_, i) => i !== idx))}
																>
																	<X className="h-4 w-4" />
																</Button>
															)}
														</div>
													))}
													<Button
														type="button"
														variant="outline"
														size="sm"
														className="gap-1"
														onClick={() => setCustomRelatedHosts([...customRelatedHosts, { host: "", role: "" }])}
													>
														<Plus className="h-3.5 w-3.5" /> Add related host
													</Button>
												</div>
											</div>
											<div className="space-y-2">
												<Label>Platform Name</Label>
												<Input placeholder="e.g. Gemini" value={newTargetPlatform} onChange={(e) => setNewTargetPlatform(e.target.value)} />
											</div>
											<div className="flex items-center justify-between gap-4 rounded-md border border-border p-3">
												<div>
													<p className="text-sm font-medium">Block entire website</p>
													<p className="text-xs text-muted-foreground">
														ON = employees cannot open this domain at all. OFF = only filter/block prompts (current Guard mode).
													</p>
												</div>
												<Switch checked={newTargetBlockSite} onCheckedChange={setNewTargetBlockSite} />
											</div>
										</div>
										<DialogFooter>
											<Button variant="outline" onClick={() => setTargetDialogOpen(false)}>
												Cancel
											</Button>
											<Button onClick={handleCreateTarget}>Add Domain</Button>
										</DialogFooter>
									</DialogContent>
								</Dialog>
								</div>
							</div>

							<div className="relative mt-3">
								<Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
								<Input
									placeholder="Search target websites..."
									value={targetSearch}
									onChange={(e) => setTargetSearch(e.target.value)}
									className="pl-9 bg-background border-border"
								/>
							</div>
						</CardHeader>
						<CardContent>
							<div className="rounded-md border border-border overflow-x-auto">
								<Table className="table-fixed min-w-[980px]">
									<TableHeader>
										<TableRow className="border-border hover:bg-transparent">
											<TableHead className="w-[320px]">Domain</TableHead>
											<TableHead className="w-[120px]">Platform Name</TableHead>
											<TableHead className="w-[110px]">Intercepted</TableHead>
											<TableHead className="w-[100px]">Status</TableHead>
											<TableHead className="w-[120px]">Monitoring</TableHead>
											<TableHead className="w-[130px]">Block Website</TableHead>
											<TableHead className="w-[90px] text-right">Actions</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{visibleTargetRows.map(({ tgt, isChild }) => (
											<TableRow key={tgt.id} className={`border-border transition-colors ${isChild ? "bg-muted/15" : "hover:bg-accent/50"}`}>
												<TableCell className="align-top whitespace-normal">
													<div className={isChild ? "pl-5 space-y-1" : "space-y-1.5"}>
														<div className="flex items-center gap-1.5 font-semibold text-sm font-mono min-w-0">
															{isChild ? (
																<CornerDownRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
															) : null}
															<span className="truncate" title={tgt.domain}>{tgt.domain}</span>
															<ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
														</div>
														{isChild ? (
															<p className="text-[10px] text-muted-foreground pl-5">{hostRoleLabel(tgt.host_role)}</p>
														) : (
															<div className="space-y-1.5 pt-0.5">
																{tgt.host_role ? (
																	<p className="text-[10px] text-muted-foreground">{hostRoleLabel(tgt.host_role)}</p>
																) : null}
																{(() => {
																	const leftover = relatedHostOptions(tgt.domain, addedTargetDomains);
																	if (leftover.length === 0) return null;
																	return (
																		<div className="flex flex-wrap gap-1">
																			{leftover.map((host) => (
																				<Button
																					key={host}
																					type="button"
																					variant="outline"
																					size="sm"
																					className="h-6 px-2 text-[10px] font-mono"
																					onClick={() => handleAddRelatedHost(tgt, host)}
																				>
																					+ {host}
																				</Button>
																			))}
																		</div>
																	);
																})()}
																<p className="text-[10px] font-medium text-muted-foreground">Add subdomain / related host</p>
																<div className="flex items-center gap-1 flex-wrap">
																	<Select
																		value={extraHostRoleDrafts[tgt.id] || "auto"}
																		onValueChange={(v) =>
																			setExtraHostRoleDrafts((prev) => ({
																				...prev,
																				[tgt.id]: v === "auto" ? "" : (v as HostRole),
																			}))
																		}
																	>
																		<SelectTrigger className="h-7 text-[10px] w-[110px]">
																			<SelectValue />
																		</SelectTrigger>
																		<SelectContent>
																			{HOST_ROLE_OPTIONS.map((opt) => (
																				<SelectItem key={opt.value || "auto"} value={opt.value || "auto"}>
																					{opt.label}
																				</SelectItem>
																			))}
																		</SelectContent>
																	</Select>
																	<Input
																		id={`subdomain-input-${tgt.id}`}
																		placeholder="e.g. clients6.google.com"
																		className="h-7 text-[10px] font-mono max-w-[200px]"
																		value={extraHostDrafts[tgt.id] || ""}
																		onChange={(e) => setExtraHostDrafts((prev) => ({ ...prev, [tgt.id]: e.target.value }))}
																		onKeyDown={(e) => {
																			if (e.key === "Enter") {
																				e.preventDefault();
																				const host = extraHostDrafts[tgt.id];
																				if (host?.trim()) {
																					handleAddRelatedHost(tgt, host, extraHostRoleDrafts[tgt.id] || "");
																					setExtraHostDrafts((prev) => ({ ...prev, [tgt.id]: "" }));
																				}
																			}
																		}}
																	/>
																	<Button
																		type="button"
																		variant="secondary"
																		size="sm"
																		className="h-7 px-2 text-[10px] gap-1"
																		onClick={() => {
																			const host = extraHostDrafts[tgt.id];
																			if (host?.trim()) {
																				handleAddRelatedHost(tgt, host, extraHostRoleDrafts[tgt.id] || "");
																				setExtraHostDrafts((prev) => ({ ...prev, [tgt.id]: "" }));
																			}
																		}}
																	>
																		<Plus className="h-3 w-3" />
																		Add
																	</Button>
																</div>
															</div>
														)}
													</div>
												</TableCell>
												<TableCell className="text-sm text-muted-foreground truncate" title={tgt.platform_name || ""}>{tgt.platform_name}</TableCell>
												<TableCell className="font-mono text-sm truncate">{tgt.intercepted_count} requests</TableCell>
												<TableCell className="overflow-hidden">
													<Badge
														className={
															tgt.block_site
																? "bg-red-950/80 text-red-400 border-red-800 text-[11px]"
																: tgt.monitored
																	? "bg-emerald-950/80 text-emerald-400 border-emerald-800 text-[11px]"
																	: "bg-slate-800 text-slate-400 border-slate-700 text-[11px]"
														}
													>
														{tgt.block_site ? "BLOCKED" : tgt.monitored ? "MONITORED" : "PAUSED"}
													</Badge>
												</TableCell>
												<TableCell className="overflow-hidden">
													<div className="flex items-center gap-2 min-w-0">
														<Switch
															checked={tgt.monitored}
															onCheckedChange={(val) =>
																updateTarget({
																	id: tgt.id,
																	updates: {
																		monitored: val,
																		status: tgt.block_site ? "BLOCKED" : val ? "MONITORED" : "PAUSED",
																	},
																})
															}
														/>
														<span className="text-xs text-muted-foreground truncate">{tgt.monitored ? "Active" : "Paused"}</span>
													</div>
												</TableCell>
												<TableCell className="overflow-hidden">
													<div className="flex items-center gap-2 min-w-0">
														<Switch
															checked={!!tgt.block_site}
															onCheckedChange={(val) =>
																updateTarget({
																	id: tgt.id,
																	updates: {
																		block_site: val,
																		status: val ? "BLOCKED" : tgt.monitored ? "MONITORED" : "PAUSED",
																	},
																})
															}
														/>
														<span className={`text-xs truncate ${tgt.block_site ? "text-red-400" : "text-muted-foreground"}`}>
															{tgt.block_site ? "Locked" : "Off"}
														</span>
													</div>
												</TableCell>
												<TableCell className="text-right">
													<div className="flex items-center justify-end gap-1">
														{!isChild ? (
															<Button
																variant="ghost"
																size="icon"
																onClick={() => {
																	const el = document.getElementById(`subdomain-input-${tgt.id}`) as HTMLInputElement | null;
																	el?.focus();
																	el?.scrollIntoView({ behavior: "smooth", block: "nearest" });
																}}
																className="h-8 w-8 text-muted-foreground hover:text-foreground"
																title="Add subdomain / related host"
															>
																<Plus className="h-4 w-4" />
															</Button>
														) : null}
														<Button
															variant="ghost"
															size="icon"
															onClick={() => {
																setEditTarget(tgt);
																setEditTargetDomain(tgt.domain);
																setEditTargetPlatform(tgt.platform_name);
																setEditTargetBlockSite(!!tgt.block_site);
																setEditTargetHostRole((tgt.host_role as HostRole) || "");
																setEditTargetDialogOpen(true);
															}}
															className="h-8 w-8 text-muted-foreground hover:text-foreground"
															title="Edit Target Domain"
														>
															<Pencil className="h-4 w-4" />
														</Button>
														<Button
															variant="ghost"
															size="icon"
															onClick={() => deleteTarget(tgt.id)}
															className="h-8 w-8 text-muted-foreground hover:text-destructive"
															title="Delete Target Domain"
														>
															<Trash2 className="h-4 w-4" />
														</Button>
													</div>
												</TableCell>
											</TableRow>
										))}
										{visibleTargetRows.length === 0 && (
											<TableRow>
												<TableCell colSpan={7} className="text-center py-8 text-muted-foreground">
													{targets.length === 0
														? "No target web domains configured."
														: "No target websites found matching your search."}
												</TableCell>
											</TableRow>
										)}
									</TableBody>
								</Table>
							</div>

							{targets.length > 0 ? (
								<div className="flex flex-col sm:flex-row items-center justify-between gap-4 pt-4 border-t border-border mt-4 text-xs text-muted-foreground">
									<div className="flex items-center gap-2">
										<span className="whitespace-nowrap">Parent domains per page</span>
										<Select
											value={String(targetPageLimit)}
											onValueChange={(v) => setTargetPageLimit(Number(v))}
										>
											<SelectTrigger className="h-8 w-[72px] bg-background border-border">
												<SelectValue />
											</SelectTrigger>
											<SelectContent>
												<SelectItem value="5">5</SelectItem>
												<SelectItem value="10">10</SelectItem>
												<SelectItem value="25">25</SelectItem>
												<SelectItem value="50">50</SelectItem>
											</SelectContent>
										</Select>
										<span>
											Showing {totalTargetParents > 0 ? targetPageOffset + 1 : 0} to{" "}
											{Math.min(targetPageOffset + targetPageLimit, totalTargetParents)} of {totalTargetParents} parent domains
										</span>
									</div>
									<div className="flex items-center gap-2">
										<span>
											Page {targetCurrentPage} of {targetTotalPages}
										</span>
										<div className="flex items-center gap-1">
											<Button
												variant="outline"
												size="icon"
												disabled={targetPageOffset === 0}
												onClick={() => setTargetPageOffset(Math.max(0, targetPageOffset - targetPageLimit))}
												className="h-8 w-8 border-border"
											>
												<ChevronLeft className="h-4 w-4" />
											</Button>
											<Button
												variant="outline"
												size="icon"
												disabled={targetPageOffset + targetPageLimit >= totalTargetParents}
												onClick={() => setTargetPageOffset(targetPageOffset + targetPageLimit)}
												className="h-8 w-8 border-border"
											>
												<ChevronRight className="h-4 w-4" />
											</Button>
										</div>
									</div>
								</div>
							) : null}
						</CardContent>
					</Card>

					<TargetImportDialog
						open={targetImportDialogOpen}
						onOpenChange={setTargetImportDialogOpen}
						onImportSuccess={refetchTargets}
					/>
				</TabsContent>

				{/* EDIT TARGET DIALOG */}
				<Dialog open={editTargetDialogOpen} onOpenChange={setEditTargetDialogOpen}>
					<DialogContent className="bg-card border-border text-foreground">
						<DialogHeader>
							<DialogTitle>Edit Target Web Domain</DialogTitle>
							<DialogDescription>Modify domain, platform name, or full-site lock.</DialogDescription>
						</DialogHeader>
						<div className="space-y-4 py-3">
							<div className="space-y-2">
								<Label>Domain Name</Label>
								<Input value={editTargetDomain} onChange={(e) => setEditTargetDomain(e.target.value)} />
							</div>
							<div className="space-y-2">
								<Label>Platform Name</Label>
								<Input value={editTargetPlatform} onChange={(e) => setEditTargetPlatform(e.target.value)} />
							</div>
							<div className="space-y-2">
								<Label>Host role</Label>
								<Select value={editTargetHostRole || "auto"} onValueChange={(v) => setEditTargetHostRole(v === "auto" ? "" : (v as HostRole))}>
									<SelectTrigger>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{HOST_ROLE_OPTIONS.map((opt) => (
											<SelectItem key={opt.value || "auto"} value={opt.value || "auto"}>
												{opt.label}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
							<div className="flex items-center justify-between gap-4 rounded-md border border-border p-3">
								<div>
									<p className="text-sm font-medium">Block entire website</p>
									<p className="text-xs text-muted-foreground">
										ON = cannot open this domain. OFF = prompt Guard only.
									</p>
								</div>
								<Switch checked={editTargetBlockSite} onCheckedChange={setEditTargetBlockSite} />
							</div>
						</div>
						<DialogFooter>
							<Button variant="outline" onClick={() => setEditTargetDialogOpen(false)}>
								Cancel
							</Button>
							<Button onClick={handleEditTarget}>Save Changes</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>

				{/* EDIT RULE DIALOG */}
				<Dialog open={editRuleDialogOpen} onOpenChange={setEditRuleDialogOpen}>
					<DialogContent className="bg-card border-border text-foreground w-[calc(100%-2rem)] sm:max-w-xl max-h-[88vh] flex flex-col p-0 overflow-hidden">
						<DialogHeader className="p-5 pb-3 shrink-0 border-b border-border/60">
							<DialogTitle className="flex items-center gap-2 text-base">
								<Pencil className="h-5 w-5 text-primary" />
								Edit Guard Rule
							</DialogTitle>
							<DialogDescription className="text-xs">Modify rule engine parameters, action, and notification messages.</DialogDescription>
						</DialogHeader>

						{ruleError && <div className="mx-5 mt-3 p-3 bg-red-950/60 border border-red-800 text-red-400 rounded-md text-xs">{ruleError}</div>}

						<div className="flex-1 overflow-y-auto overflow-x-hidden px-5 py-4 space-y-4 min-w-0 no-scrollbar">
							{/* Rule Engine Type Toggle */}
							<div className="space-y-1.5">
								<Label>Rule Engine Type</Label>
								<div className="grid grid-cols-2 gap-2 p-1 bg-muted/40 rounded-lg border border-border">
									<button
										type="button"
										onClick={() => {
											setEditRuleType("regex");
										}}
										className={`flex items-center justify-center gap-2 py-2 px-3 rounded-md text-xs font-semibold transition-all ${editRuleType === "regex"
												? "bg-primary text-primary-foreground shadow-sm"
												: "text-muted-foreground hover:text-foreground"
											}`}
									>
										<Zap className="h-3.5 w-3.5" />
										Regex Pattern Rule
									</button>
									<button
										type="button"
										onClick={() => {
											setEditRuleType("ai_bot");
											setEditRuleBotProvider(GUARD_BOT_OLLAMA_PROVIDER);
											setEditRuleBotModel(GUARD_BOT_OLLAMA_MODEL);
										}}
										className={`flex items-center justify-center gap-2 py-2 px-3 rounded-md text-xs font-semibold transition-all ${editRuleType === "ai_bot"
												? "bg-purple-600 text-white shadow-sm"
												: "text-muted-foreground hover:text-foreground"
											}`}
									>
										<Bot className="h-3.5 w-3.5" />
										AI Guard Bot (Prompt Rule)
									</button>
								</div>
							</div>

							<div className="space-y-1.5">
								<Label>Rule Name</Label>
								<Input value={editRuleName} onChange={(e) => setEditRuleName(e.target.value)} />
							</div>

							<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div className="space-y-1.5 min-w-0">
									<Label>Severity</Label>
									<Select value={editRuleSeverity} onValueChange={(v: any) => setEditRuleSeverity(v)}>
										<SelectTrigger className="w-full">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="CRITICAL">CRITICAL</SelectItem>
											<SelectItem value="HIGH">HIGH</SelectItem>
											<SelectItem value="MEDIUM">MEDIUM</SelectItem>
										</SelectContent>
									</Select>
								</div>
								<div className="space-y-1.5 min-w-0">
									<Label>Action</Label>
									<Select value={editRuleAction} onValueChange={(v: any) => setEditRuleAction(v)}>
										<SelectTrigger className="w-full">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="BLOCK">BLOCK</SelectItem>
											<SelectItem value="REDACT">REDACT</SelectItem>
											<SelectItem value="WARN">WARN</SelectItem>
										</SelectContent>
									</Select>
									<p className="text-[11px] text-muted-foreground break-words">
										{guardRuleActionHint(editRuleAction)}
									</p>
								</div>
							</div>

							{editRuleType === "regex" ? (
								<div className="space-y-1.5">
									<Label>Regex Pattern</Label>
									<Input value={editRulePattern} onChange={(e) => setEditRulePattern(e.target.value)} />
									<p className="text-[11px] text-muted-foreground">
										Evaluated in microseconds using Golang RE2 regular expressions. BLOCK wins over REDACT, REDACT wins over WARN if both match.
									</p>
									<RegexLiveTestPanel pattern={editRulePattern} />
								</div>
							) : (
								<GuardRuleAIEvaluatorFields
									botProvider={editRuleBotProvider}
									botModel={editRuleBotModel}
									botPrompt={editRuleBotPrompt}
									referenceImagePreview={editRuleBotReferenceImagePreview}
									evalMode={editRuleBotEvalMode}
									generatedPattern={editRuleGeneratedPattern}
									generateError={editRuleGenerateError}
									generating={generatingRegex}
									outsourceProviderOptions={outsourceProviderOptions}
									onProviderChange={setEditRuleBotProvider}
									onModelChange={setEditRuleBotModel}
									onPromptChange={setEditRuleBotPrompt}
									onEvalModeChange={setEditRuleBotEvalMode}
									onGeneratedPatternChange={setEditRuleGeneratedPattern}
									onGenerateRegex={() => runGenerateRegex("edit")}
									onTestEvaluate={() => runTestEvaluate("edit")}
									testSample={editRuleTestSample}
									onTestSampleChange={setEditRuleTestSample}
									testResult={editRuleTestResult}
									testing={testingGuardBot}
									onReferenceImageClear={() => {
										setEditRuleBotReferenceImage("");
										setEditRuleBotReferenceImageType("");
										setEditRuleBotReferenceImagePreview("");
									}}
									onReferenceImageChange={async (file) => {
										try {
											const { data, type } = await readReferenceImageFile(file);
											setEditRuleBotReferenceImage(data);
											setEditRuleBotReferenceImageType(type);
											setEditRuleBotReferenceImagePreview(referenceImageDataUrl(data, type));
										} catch (err: any) {
											setRuleError(err?.message || "Failed to load reference image.");
										}
									}}
								/>
							)}

							<div className="space-y-1.5">
								<Label>Description</Label>
								<Textarea value={editRuleDescription} onChange={(e) => setEditRuleDescription(e.target.value)} />
							</div>
							<div className="space-y-1.5">
								<Label>{guardRuleNoticeCopy(editRuleAction).label}</Label>
								<Textarea
									value={editRuleWarningMessage}
									onChange={(e) => setEditRuleWarningMessage(e.target.value)}
									placeholder={guardRuleNoticeCopy(editRuleAction).placeholder}
									rows={3}
								/>
								<p className="text-xs text-muted-foreground break-words">
									{guardRuleNoticeCopy(editRuleAction).hint}
								</p>
							</div>
						</div>
						<DialogFooter className="p-4 px-5 shrink-0 border-t border-border/60 bg-card">
							<Button variant="outline" onClick={() => setEditRuleDialogOpen(false)}>
								Cancel
							</Button>
							<Button onClick={handleEditRuleSubmit}>Save Changes</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>

				{/* TAB 5: GUARD AGENTS */}
				<TabsContent value="agents" className="space-y-6">
					<div className="flex flex-col sm:flex-row gap-3">
						<div className="relative flex-1 max-w-md">
							<Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
							<Input
								placeholder="Search hostname, user, IP, MAC, agent id..."
								className="pl-9"
								value={agentSearch}
								onChange={(e) => {
									setAgentSearch(e.target.value);
									setAgentPageOffset(0);
								}}
							/>
						</div>
						<Select
							value={agentStatusFilter}
							onValueChange={(v) => {
								setAgentStatusFilter(v);
								setAgentPageOffset(0);
							}}
						>
							<SelectTrigger className="w-[160px]">
								<SelectValue placeholder="Status" />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="all">All statuses</SelectItem>
								<SelectItem value="active">Active</SelectItem>
								<SelectItem value="uninstall_pending">Uninstall pending</SelectItem>
								<SelectItem value="uninstalled">Uninstalled</SelectItem>
							</SelectContent>
						</Select>
						<Select
							value={agentTypeFilter}
							onValueChange={(v) => {
								setAgentTypeFilter(v);
								setAgentPageOffset(0);
							}}
						>
							<SelectTrigger className="w-[160px]">
								<SelectValue placeholder="Source" />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="all">All sources</SelectItem>
								<SelectItem value="endpoint">Laptop Guard</SelectItem>
								<SelectItem value="network">Network / server</SelectItem>
							</SelectContent>
						</Select>
					</div>

					<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription>Total registered</CardDescription>
								<CardTitle className="text-2xl">{totalAgents}</CardTitle>
							</CardHeader>
						</Card>
						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription>Active</CardDescription>
								<CardTitle className="text-2xl text-emerald-400">{activeAgentsCount}</CardTitle>
							</CardHeader>
						</Card>
						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription>Uninstalled</CardDescription>
								<CardTitle className="text-2xl text-slate-400">{uninstalledAgentsCount}</CardTitle>
							</CardHeader>
						</Card>
						<Card className="bg-card border-border">
							<CardHeader className="pb-2">
								<CardDescription>Uninstall key</CardDescription>
								<CardTitle className="text-lg">
									{agentSettings?.key_configured ? "Configured · Always required" : "Not set · Set key first"}
								</CardTitle>
							</CardHeader>
						</Card>
					</div>

					<Card className="bg-card border-border">
						<CardHeader>
							<div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
								<div>
									<CardTitle className="text-lg">Guard Agents (laptop + network)</CardTitle>
									<CardDescription>
										Same Browser AI dashboard for laptop Guard EXE and shared server/network proxy. Rules and Prompt Logs are shared.
									</CardDescription>
								</div>
								<div className="flex items-center gap-2">
									{selectedAgentCount > 0 ? (
										<span className="text-xs text-muted-foreground whitespace-nowrap">
											{selectedAgentCount} selected
										</span>
									) : null}
									<Select
										value={agentBulkAction}
										onValueChange={handleAgentBulkAction}
										disabled={selectedAgentCount === 0 || deletingAgents}
									>
										<SelectTrigger className="w-[160px]">
											<SelectValue placeholder="Choose option" />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="delete" className="text-destructive focus:text-destructive">
												Delete
											</SelectItem>
										</SelectContent>
									</Select>
								</div>
							</div>
						</CardHeader>
						<CardContent className="p-0">
							<Table className="table-fixed min-w-[1400px]">
								<TableHeader>
									<TableRow className="hover:bg-transparent border-border">
										<TableHead className="w-[44px]">
											<Checkbox
												checked={allVisibleAgentsSelected || (someVisibleAgentsSelected ? "indeterminate" : false)}
												onCheckedChange={(checked) => toggleSelectAllVisibleAgents(checked === true)}
												aria-label="Select all Guard agents on this page"
												disabled={agents.length === 0}
											/>
										</TableHead>
										<TableHead className="w-[180px]">Host</TableHead>
										<TableHead className="w-[90px]">Source</TableHead>
										<TableHead className="w-[110px]">User</TableHead>
										<TableHead className="w-[130px]">IP</TableHead>
										<TableHead className="w-[200px] pr-4">Physical address (MAC)</TableHead>
										<TableHead className="w-[220px] px-4">Transport name</TableHead>
										<TableHead className="w-[170px]">Version</TableHead>
										<TableHead className="w-[120px]">Status</TableHead>
										<TableHead className="w-[160px]">Last seen</TableHead>
										<TableHead className="w-[160px]">Installed</TableHead>
										<TableHead className="w-[60px] text-right">Actions</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{agents.map((agent) => (
										<TableRow key={agent.id} className="border-border">
											<TableCell>
												<Checkbox
													checked={selectedAgentIds.has(agent.id)}
													onCheckedChange={(checked) => toggleSelectAgent(agent.id, checked === true)}
													aria-label={`Select ${agent.hostname || agent.id}`}
												/>
											</TableCell>
											<TableCell className="align-top whitespace-normal">
												<button
													type="button"
													onClick={() => handleOpenAgentDetails(agent)}
													className="font-medium text-sm truncate text-left hover:underline hover:text-primary transition-colors block w-full"
													title={`Click to view device details & daily uninstall key for ${agent.hostname || agent.id}`}
												>
													{agent.hostname || "—"}
												</button>
												<div className="text-[11px] text-muted-foreground font-mono truncate" title={agent.id}>
													{agent.id}
												</div>
												{agent.has_uninstall_key && (
													<button
														type="button"
														onClick={() => handleOpenAgentDetails(agent)}
														className="text-[10px] text-emerald-400 hover:text-emerald-300 flex items-center gap-1 mt-0.5 transition-colors cursor-pointer"
														title="Click to view today's daily auto-rotating key"
													>
														<KeyRound className="h-3 w-3" />
														<span>Daily Key Active</span>
													</button>
												)}
											</TableCell>
											<TableCell className="text-sm">
												{(agent.agent_type || "endpoint") === "network" ? "Network" : "Laptop"}
											</TableCell>
											<TableCell className="text-sm truncate">{agent.username || "—"}</TableCell>
											<TableCell className="text-xs font-mono truncate">{agent.ip_address || "—"}</TableCell>
											<TableCell className="text-xs font-mono truncate pr-4" data-testid="guard-agent-mac-cell" title={agent.mac_address || ""}>
												{agent.mac_address || "—"}
											</TableCell>
											<TableCell className="text-[11px] font-mono text-muted-foreground truncate px-4" data-testid="guard-agent-transport-cell" title={nicGuidOnly(agent.transport_name) || ""}>
												{nicGuidOnly(agent.transport_name) || "—"}
											</TableCell>
											<TableCell className="text-xs align-top whitespace-normal">
												{(() => {
													const isMac = /mac|darwin/i.test(agent.os_version || "");
													const latest = isMac ? latestMacVersion : latestWinVersion;
													const current = agent.agent_version || "";
													const outdated = !!current && !!latest && compareGuardVersions(current, latest) < 0;
													return (
														<>
															<div className="font-medium font-mono">{current ? `v${current}` : "—"}</div>
															{current && latest ? (
																<Badge
																	variant="outline"
																	className={`mt-0.5 px-1.5 py-0 text-[10px] ${outdated ? "border-amber-500/40 text-amber-400" : "border-emerald-500/40 text-emerald-400"}`}
																	title={outdated ? `Server has v${latest}; Guard updates within a few minutes while online` : "Matches the server package"}
																>
																	{outdated ? `Update pending → v${latest}` : "Up to date"}
																</Badge>
															) : null}
															{agent.version_updated_at ? (
																<div className="mt-0.5 text-[10px] text-muted-foreground" title="When this version first reported in (install or auto-update)">
																	since {new Date(agent.version_updated_at).toLocaleString()}
																</div>
															) : null}
															{(() => {
																const bundle = setupInfo?.proxy_bundle;
																if (!bundle || !current || !bundle.guard_versions?.includes(current)) return null;
																if (compareGuardVersions(current, PROXY_BUNDLE_MIN_GUARD) < 0) return null;
																const running = agent.proxy_bundle_sha === bundle.sha256;
																return (
																	<div
																		className={`mt-0.5 text-[10px] ${running ? "text-emerald-400" : "text-amber-400"}`}
																		title={
																			running
																				? `Running Guard code ${bundle.sha256.slice(0, 8)} published ${new Date(bundle.published_at).toLocaleString()}`
																				: "Guard downloads, self-tests and restarts on the new code at its next heartbeat while online"
																		}
																	>
																		{running ? `Code ${bundle.sha256.slice(0, 8)} ✓` : `Code → ${bundle.sha256.slice(0, 8)} pending`}
																	</div>
																);
															})()}
														</>
													);
												})()}
											</TableCell>
											<TableCell>{getAgentStatusBadge(agent.status, agent.uninstall_requested)}</TableCell>
											<TableCell className="text-xs text-muted-foreground truncate">
												{agent.last_seen_at ? new Date(agent.last_seen_at).toLocaleString() : "—"}
											</TableCell>
											<TableCell className="text-xs text-muted-foreground truncate">
												{agent.installed_at ? new Date(agent.installed_at).toLocaleString() : "—"}
											</TableCell>
											<TableCell className="text-right">
												<DropdownMenu>
													<DropdownMenuTrigger asChild>
														<Button variant="ghost" size="icon" className="h-8 w-8 p-0 text-muted-foreground hover:text-foreground">
															<MoreHorizontal className="h-4 w-4" />
															<span className="sr-only">Actions</span>
														</Button>
													</DropdownMenuTrigger>
													<DropdownMenuContent align="end" className="w-56 bg-card border-border">
														<DropdownMenuItem
															onClick={() => handleOpenAgentDetails(agent)}
															className="cursor-pointer gap-2 font-medium"
														>
															<KeyRound className="h-4 w-4 text-emerald-400" />
															View Device &amp; Daily Key
														</DropdownMenuItem>
														<DropdownMenuItem
															onClick={() => handleOpenRemoteUninstall(agent)}
															className="text-red-400 focus:text-red-400 focus:bg-red-950/40 cursor-pointer gap-2"
														>
															<PowerOff className="h-4 w-4 text-red-400" />
															Turn Off / Uninstall Guard
														</DropdownMenuItem>
														<DropdownMenuItem
															onClick={() => handleCopyGuardUninstallKey(agent)}
															className="cursor-pointer gap-2"
														>
															<KeyRound className="h-4 w-4" />
															Copy Today&apos;s Key (Daily)
														</DropdownMenuItem>
														<DropdownMenuItem
															onClick={() => handleRotateGuardUninstallKey(agent)}
															disabled={isRotatingGuardKey}
															className="cursor-pointer gap-2"
														>
															<RefreshCw className="h-4 w-4" />
															Rotate Guard Key Now
														</DropdownMenuItem>
														<DropdownMenuItem
															onClick={() => {
																navigator.clipboard.writeText(agent.id);
															}}
															className="cursor-pointer gap-2"
														>
															<Copy className="h-4 w-4" />
															Copy Agent ID
														</DropdownMenuItem>
														<DropdownMenuSeparator />
														<DropdownMenuItem
															onClick={() => handleDeleteSingleAgent(agent)}
															className="text-muted-foreground focus:text-destructive cursor-pointer gap-2"
														>
															<Trash2 className="h-4 w-4" />
															Remove from Fleet
														</DropdownMenuItem>
													</DropdownMenuContent>
												</DropdownMenu>
											</TableCell>
										</TableRow>
									))}
									{agents.length === 0 && (
										<TableRow>
											<TableCell colSpan={12} className="text-center py-10 text-muted-foreground text-sm">
												No Guard agents yet. Install Raksha_Guard_Setup.exe (Windows) or Raksha_Guard_macOS.zip (Mac) on laptops and/or run the network proxy: docker compose --profile network-proxy up -d raksha_browser_ai_proxy (or Guard with server_mode). Same dashboard for both.
											</TableCell>
										</TableRow>
									)}
								</TableBody>
							</Table>
						</CardContent>
					</Card>

					{/* Guard Agents pagination — always visible (Prompt Logs style) */}
					<div className="flex flex-col sm:flex-row justify-between items-center gap-4 text-xs text-muted-foreground">
						<div className="flex items-center gap-2">
							<span>Rows per page</span>
							<Select
								value={agentPageLimit.toString()}
								onValueChange={(val) => {
									setAgentPageLimit(Number(val));
								}}
							>
								<SelectTrigger className="h-8 w-[70px] bg-background border-border">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="10">10</SelectItem>
									<SelectItem value="25">25</SelectItem>
									<SelectItem value="50">50</SelectItem>
									<SelectItem value="100">100</SelectItem>
								</SelectContent>
							</Select>
							<span>
								Showing {totalAgents > 0 ? agentPageOffset + 1 : 0} to{" "}
								{Math.min(agentPageOffset + agentPageLimit, totalAgents)} of {totalAgents} entries
							</span>
						</div>
						<div className="flex items-center gap-2">
							<span>
								Page {agentCurrentPage} of {agentTotalPages}
							</span>
							<div className="flex items-center gap-1">
								<Button
									variant="outline"
									size="icon"
									disabled={agentPageOffset === 0}
									onClick={() => setAgentPageOffset(Math.max(0, agentPageOffset - agentPageLimit))}
									className="h-8 w-8 border-border"
									aria-label="Previous agents page"
								>
									<ChevronLeft className="h-4 w-4" />
								</Button>
								<Button
									variant="outline"
									size="icon"
									disabled={agentPageOffset + agentPageLimit >= totalAgents}
									onClick={() => setAgentPageOffset(agentPageOffset + agentPageLimit)}
									className="h-8 w-8 border-border"
									aria-label="Next agents page"
								>
									<ChevronRight className="h-4 w-4" />
								</Button>
							</div>
						</div>
					</div>

					<AlertDialog
						open={showAgentDeleteDialog}
						onOpenChange={(open) => {
							setShowAgentDeleteDialog(open);
							if (!open) {
								setAgentBulkAction("");
								setAgentDeleteError("");
							}
						}}
					>
						<AlertDialogContent>
							<AlertDialogHeader>
								<AlertDialogTitle>Delete selected Guard agents?</AlertDialogTitle>
								<AlertDialogDescription>
									This removes {selectedAgentCount} selected {selectedAgentCount === 1 ? "record" : "records"} from the Guard Agents list.
									Installed agents on employee laptops are not uninstalled automatically.
								</AlertDialogDescription>
							</AlertDialogHeader>
							{agentDeleteError ? <p className="text-sm text-red-400">{agentDeleteError}</p> : null}
							<AlertDialogFooter>
								<AlertDialogCancel disabled={deletingAgents}>Cancel</AlertDialogCancel>
								<AlertDialogAction
									onClick={(e) => {
										e.preventDefault();
										void handleDeleteSelectedAgents();
									}}
									disabled={deletingAgents || selectedAgentCount === 0}
									className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
								>
									{deletingAgents ? "Deleting..." : "Delete"}
								</AlertDialogAction>
							</AlertDialogFooter>
						</AlertDialogContent>
					</AlertDialog>
				</TabsContent>

				{/* TAB 6: SETUP */}
				<TabsContent value="setup" className="space-y-6">
					<Card className="bg-card border-border">
						<CardHeader>
							<div className="flex items-center gap-3">
								<FileKey className="h-6 w-6 text-amber-400" />
								<div>
									<CardTitle className="text-lg">Uninstall Key</CardTitle>
									<CardDescription>
										Employees and admins can remove Guard only with this company key. Key is stored hashed.
									</CardDescription>
								</div>
							</div>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="rounded-md border border-amber-800/50 bg-amber-950/20 p-3">
								<p className="text-sm font-medium text-amber-200">Uninstall always requires this key</p>
								<p className="text-xs text-muted-foreground mt-1">
									Windows Settings / Start Menu / CLI / Mac uninstall / admin remote uninstall — all need the matching key. You cannot turn this off.
								</p>
							</div>
							<div className="space-y-3">
								{agentSettings?.key_configured && !uninstallKeyEditing ? (
									<div className="space-y-2 rounded-md border border-border p-3">
										<div className="flex items-center justify-between gap-2">
											<Label>Saved uninstall key</Label>
											<p className="text-xs text-muted-foreground">
												{agentSettings?.updated_at ? `Updated ${new Date(agentSettings.updated_at).toLocaleString()}` : "Configured"}
											</p>
										</div>
										<div className="flex flex-col sm:flex-row gap-2">
											<div className="relative min-w-0 flex-1">
												<Input
													readOnly
													type={showUninstallKey ? "text" : "password"}
													value={
														showUninstallKey
															? (savedUninstallKeyDisplay || agentSettingsData?.uninstall_key || (typeof window !== "undefined" ? localStorage.getItem("raksha_company_uninstall_key") : "") || "12345678")
															: "••••••••••••••••••••"
													}
													className="pr-10 font-mono"
												/>
												<Button
													type="button"
													variant="ghost"
													size="icon"
													className="absolute right-1 top-1/2 h-8 w-8 -translate-y-1/2 text-muted-foreground hover:text-foreground"
													onClick={async () => {
														let currentKey = savedUninstallKeyDisplay || agentSettingsData?.uninstall_key;
														if (!currentKey && typeof window !== "undefined") {
															currentKey = localStorage.getItem("raksha_company_uninstall_key") || "";
														}
														if (!currentKey) {
															try {
																const res = await fetch("/api/browser-ai/agents/uninstall-key");
																if (res.ok) {
																	const json = await res.json();
																	if (json.uninstall_key) {
																		currentKey = json.uninstall_key;
																		setSavedUninstallKeyDisplay(json.uninstall_key);
																		if (typeof window !== "undefined") {
																			localStorage.setItem("raksha_company_uninstall_key", json.uninstall_key);
																		}
																	}
																}
															} catch {}
														}
														if (!currentKey && agentSettings?.key_configured) {
															currentKey = "12345678";
															setSavedUninstallKeyDisplay(currentKey);
														}
														if (currentKey) {
															setSavedUninstallKeyDisplay(currentKey);
															setUninstallKeyMessage("");
														}
														setShowUninstallKey((v) => !v);
													}}
													title={showUninstallKey ? "Hide key" : "Show key"}
												>
													{showUninstallKey ? (
														<EyeOff className="h-4 w-4" />
													) : (
														<Eye className="h-4 w-4" />
													)}
												</Button>
											</div>
											<Button
												variant="outline"
												className="gap-2 shrink-0"
												onClick={() => {
													setUninstallKeyEditing(true);
													setUninstallKeyInput("");
													setUninstallKeyMessage("");
													setUninstallKeyError("");
													setShowUninstallKey(false);
												}}
											>
												<Pencil className="h-4 w-4" />
												Edit
											</Button>
										</div>
										<p className="text-xs text-muted-foreground">
											Click the eye icon to view or hide the company uninstall key. Use Edit to rotate.
										</p>
									</div>
								) : (
									<div className="space-y-2">
										<Label>{agentSettings?.key_configured ? "Edit / rotate uninstall key" : "Set uninstall key"}</Label>
										<div className="flex flex-col sm:flex-row gap-2">
											<div className="relative min-w-0 flex-1">
												<Input
													type={showUninstallKey ? "text" : "password"}
													placeholder={agentSettings?.key_configured ? "Enter new key to rotate…" : "Enter company uninstall key…"}
													value={uninstallKeyInput}
													onChange={(e) => setUninstallKeyInput(e.target.value)}
													autoComplete="new-password"
													className="pr-10 font-mono"
												/>
												<Button
													type="button"
													variant="ghost"
													size="icon"
													className="absolute right-1 top-1/2 h-8 w-8 -translate-y-1/2 text-muted-foreground hover:text-foreground"
													onClick={() => setShowUninstallKey((v) => !v)}
													title={showUninstallKey ? "Hide key" : "Show key"}
												>
													{showUninstallKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</Button>
											</div>
											<Button onClick={handleSaveUninstallKey} disabled={savingUninstallKey || !uninstallKeyInput.trim()} className="gap-2 shrink-0">
												<Save className="h-4 w-4" />
												{savingUninstallKey ? "Saving…" : "Save"}
											</Button>
											{agentSettings?.key_configured ? (
												<Button
													type="button"
													variant="outline"
													className="shrink-0"
													onClick={() => {
														setUninstallKeyEditing(false);
														setUninstallKeyInput("");
														setUninstallKeyError("");
													}}
													disabled={savingUninstallKey}
												>
													Cancel
												</Button>
											) : null}
										</div>
									</div>
								)}
								{uninstallKeyMessage ? <p className="text-sm text-emerald-400">{uninstallKeyMessage}</p> : null}
								{uninstallKeyError ? <p className="text-sm text-red-400">{uninstallKeyError}</p> : null}
							</div>
						</CardContent>
					</Card>

					<Card className="bg-card border-border">
						<CardHeader>
							<div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
								<div className="flex min-w-0 flex-1 items-start gap-3">
									<CheckCircle2 className="mt-0.5 h-5 w-5 text-emerald-400 shrink-0" />
									<div className="min-w-0">
										<CardTitle className="text-lg">Employee Setup Packages</CardTitle>
										<CardDescription>
											Installers for employee laptops. After Rebuild &amp; Publish, online Guards switch to the latest Guard code within minutes — no reinstall.
										</CardDescription>
									</div>
								</div>
								<div className="flex shrink-0 flex-wrap lg:flex-nowrap items-center gap-2">
									<Button
										onClick={handleRebuildPackages}
										disabled={rebuildingPackages}
										variant="outline"
										size="sm"
										className="h-9 gap-1.5 text-xs border-emerald-500/40 hover:border-emerald-500 hover:bg-emerald-500/10 text-emerald-400 transition-colors"
									>
										<RefreshCw className={`h-4 w-4 ${rebuildingPackages ? "animate-spin" : ""}`} />
										{rebuildingPackages ? "Rebuilding..." : "Rebuild & Publish"}
									</Button>
									<Button
										onClick={() => handleDownloadSetupPackage("windows")}
										disabled={downloadingPlatform !== null}
										variant="outline"
										size="sm"
										className="h-9 gap-1.5 text-xs border-border hover:border-sky-500/60 hover:bg-sky-500/10 transition-colors"
									>
										<svg className="h-4 w-4 fill-current text-sky-400" viewBox="0 0 24 24">
											<path d="M0 3.449L9.75 2.1v9.451H0m10.949-9.602L24 0v11.4H10.949M0 12.6h9.75v9.451L0 20.699M10.949 12.6H24V24l-12.949-1.95" />
										</svg>
										{downloadingPlatform === "windows" ? "Preparing..." : "Windows"}
									</Button>
									<Button
										onClick={() => handleDownloadSetupPackage("mac")}
										disabled={downloadingPlatform !== null}
										size="sm"
										className="h-9 gap-1.5 text-xs bg-primary hover:bg-primary/90 text-primary-foreground"
									>
										<svg className="h-4 w-4 fill-current" viewBox="0 0 170 170">
											<path d="M150.37 130.25c-2.45 5.66-5.35 10.87-8.71 15.66-4.58 6.53-8.33 11.05-11.22 13.56-4.48 4.12-9.28 6.23-14.42 6.35-3.69 0-8.14-1.05-13.32-3.18-5.19-2.12-9.97-3.17-14.34-3.17-4.58 0-9.49 1.05-14.75 3.17-5.26 2.13-9.5 3.24-12.74 3.35-4.35.13-9.16-1.9-14.42-6.08-3.7-3.04-7.7-7.9-11.99-14.57-6.09-9.46-10.9-20.2-14.42-32.22-3.52-12.01-5.28-23.23-5.28-33.64 0-14.78 3.82-27.17 11.45-37.19 7.63-10.01 17.1-15.13 28.4-15.35 4.35 0 9.29 1.14 14.81 3.42 5.53 2.29 9.38 3.48 11.56 3.59 1.74 0 5.86-1.25 12.38-3.76 6.52-2.5 12.16-3.6 16.92-3.3 12.51.98 22.37 5.76 29.57 14.34-11.09 6.74-16.53 16.09-16.32 28.05.22 9.57 3.91 17.61 11.09 24.13 7.18 6.52 15.66 10.11 25.44 10.76-2.28 7.07-5.22 14.67-8.81 22.8zM119.22 31.84c0-7.18 2.61-13.91 7.83-20.19 5.22-6.28 11.52-10.22 18.91-11.83 1.09 6.74-.22 13.48-3.91 20.22-3.7 6.74-9.35 11.3-16.96 13.7-1.09-.76-2.93-1.3-5.52-1.63-.22-.11-.35-.27-.35-.27z" />
										</svg>
										{downloadingPlatform === "mac" ? "Preparing..." : "macOS"}
									</Button>
								</div>
							</div>
						</CardHeader>
						<CardContent className="pt-0 space-y-3">
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
								{[
									{
										label: "Windows",
										file: "Raksha_Guard_Windows.zip",
										contents: "Setup.exe · auto-start · proxy routing",
										dot: "bg-sky-500",
										ready: setupInfo?.windows_ready,
										version: latestWinVersion,
										builtAt: setupInfo?.windows_built_at,
									},
									{
										label: "macOS",
										file: "Raksha_Guard_macOS.zip",
										contents: "Raksha_Guard.app · Install .command",
										dot: "bg-primary",
										ready: setupInfo?.macos_ready,
										version: latestMacVersion,
										builtAt: setupInfo?.macos_built_at,
									},
								].map((pkg) => (
									<div key={pkg.label} className="flex items-center justify-between gap-3 rounded-md border border-border bg-card px-3 py-2.5">
										<div className="min-w-0">
											<div className="flex items-center gap-2 text-xs font-medium text-foreground">
												<span className={`h-2 w-2 shrink-0 rounded-full ${pkg.dot}`} />
												{pkg.label}
												<code className="truncate rounded border border-border bg-muted px-1.5 py-0.5 text-[11px]">{pkg.file}</code>
											</div>
											<p className="mt-1 truncate text-[11px] text-muted-foreground">
												{pkg.contents}
												{pkg.builtAt ? ` · built ${new Date(pkg.builtAt).toLocaleString()}` : ""}
											</p>
										</div>
										{pkg.ready === false ? (
											<Badge variant="outline" className="shrink-0 border-red-500/40 text-red-400">
												Missing
											</Badge>
										) : (
											<Badge variant="outline" className="shrink-0 border-emerald-500/40 font-mono text-emerald-400">
												{pkg.version ? `v${pkg.version}` : "—"}
											</Badge>
										)}
									</div>
								))}
							</div>
							<div className="flex items-center justify-between gap-3 rounded-md border border-border bg-card px-3 py-2.5">
								<div className="min-w-0">
									<div className="flex items-center gap-2 text-xs font-medium text-foreground">
										<span className="h-2 w-2 shrink-0 rounded-full bg-emerald-500" />
										Guard code (hot-update)
									</div>
									<p className="mt-1 truncate text-[11px] text-muted-foreground">
										{setupInfo?.proxy_bundle
											? `Published ${new Date(setupInfo.proxy_bundle.published_at).toLocaleString()} for Guard v${setupInfo.proxy_bundle.guard_versions.join(" / v")} · applied by installed Guards within about a minute`
											: setupInfo?.proxy_source_available === false
												? "Guard source not found on the server — mount apps/browser-guard/proxy and /agent at /app/guard-proxy and /app/guard-agent"
												: "Not published yet — press Rebuild & Publish to push the current Guard code to installed Guards"}
									</p>
								</div>
								{setupInfo?.proxy_bundle ? (
									<Badge variant="outline" className="shrink-0 border-emerald-500/40 font-mono text-emerald-400" title={setupInfo.proxy_bundle.sha256}>
										{setupInfo.proxy_bundle.sha256.slice(0, 8)}
									</Badge>
								) : (
									<Badge variant="outline" className="shrink-0 border-amber-500/40 text-amber-400">
										Not published
									</Badge>
								)}
							</div>
							{setupInfo && !setupInfo.can_rebuild ? (
								<p className="text-[11px] text-muted-foreground">
									Rebuild publishes the server&apos;s current Guard code (agent + proxy) to installed Guards (v{PROXY_BUNDLE_MIN_GUARD}+),
									no reinstall needed. Only new Python packages or installer changes need a new version built on the build machine.
								</p>
							) : null}
							{setupPackageError ? <p className="text-sm text-destructive">{setupPackageError}</p> : null}
							<details className="rounded-md border border-border bg-card px-3 py-2.5">
								<summary className="cursor-pointer select-none text-xs font-medium text-foreground">
									Rebuild history ({rebuildHistory.length}
									{rebuildHistory.length >= 20 ? "+" : ""}) — saved in the database
								</summary>
								{rebuildHistory.length === 0 ? (
									<p className="mt-2 text-[11px] text-muted-foreground">No Rebuild &amp; Publish yet.</p>
								) : (
									<div className="mt-2 max-h-72 overflow-y-auto no-scrollbar">
										<table className="w-full text-left text-[11px]">
											<thead className="sticky top-0 bg-card text-muted-foreground">
												<tr>
													<th className="py-1 pr-2 font-medium">When</th>
													<th className="py-1 pr-2 font-medium">Result</th>
													<th className="py-1 pr-2 font-medium">Guard code</th>
													<th className="py-1 pr-2 font-medium">Installers</th>
													<th className="py-1 pr-2 font-medium">By</th>
												</tr>
											</thead>
											<tbody className="divide-y divide-border/60">
												{rebuildHistory.map((row) => (
													<tr key={row.id} title={row.message}>
														<td className="py-1.5 pr-2 whitespace-nowrap">{new Date(row.created_at).toLocaleString()}</td>
														<td className="py-1.5 pr-2">
															<Badge
																variant="outline"
																className={
																	row.status === "success"
																		? "border-emerald-500/40 text-emerald-400"
																		: "border-red-500/40 text-red-400"
																}
															>
																{row.status === "success" ? "Success" : "Failed"}
															</Badge>
														</td>
														<td className="py-1.5 pr-2 font-mono">
															{row.bundle_sha ? row.bundle_sha.slice(0, 8) : "—"}
															{row.guard_versions ? (
																<span className="ml-1 font-sans text-muted-foreground">for v{row.guard_versions.split(",").join(" / v")}</span>
															) : null}
														</td>
														<td className="py-1.5 pr-2 whitespace-nowrap">
															{row.mode === "rebuilt" ? "Rebuilt" : "Served"}
															{row.version ? ` · Win v${row.version}` : ""}
															{row.mac_version ? ` · Mac v${row.mac_version}` : ""}
														</td>
														<td className="py-1.5 pr-2 truncate max-w-[120px]">{row.requested_by || "—"}</td>
													</tr>
												))}
											</tbody>
										</table>
									</div>
								)}
							</details>
						</CardContent>
					</Card>

					<Card className="bg-card border-border">
						<CardHeader>
							<CardTitle className="text-lg">Install Steps</CardTitle>
							<CardDescription>Download the package for your OS and install Guard on Windows or Mac laptops.</CardDescription>
						</CardHeader>
						<CardContent className="space-y-6">
							<div className="space-y-4">
								<div className="flex items-center gap-2 font-semibold text-foreground">
									<span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs">1</span>
									<span>Download the package for your OS</span>
								</div>
								<p className="text-sm text-foreground/80 pl-8 leading-relaxed">
									Click <strong>Download for Windows</strong> or <strong>Download for Mac</strong> above based on your device.
								</p>
							</div>

							<div className="space-y-4">
								<div className="flex items-center gap-2 font-semibold text-foreground">
									<span className="flex h-6 min-w-6 px-1.5 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs">Win</span>
									<span>Windows</span>
								</div>
								<p className="text-sm text-foreground/80 pl-8 leading-relaxed">
									Run{" "}
									<code className="rounded border border-border bg-muted px-1.5 py-0.5 text-foreground">
										Raksha_Guard_Setup.exe
									</code>
									. Keep autostart enabled so Guard starts at Windows login. To turn OFF / uninstall: Windows Settings → Apps →
									Raksha Guard → Uninstall (company uninstall key).
								</p>
							</div>

							<div className="space-y-4">
								<div className="flex items-center gap-2 font-semibold text-foreground">
									<span className="flex h-6 min-w-6 px-1.5 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs">Mac</span>
									<span>Mac</span>
								</div>
								<p className="text-sm text-foreground/80 pl-8 leading-relaxed">
									Unzip{" "}
									<code className="rounded border border-border bg-muted px-1.5 py-0.5 text-foreground">
										Raksha_Guard_macOS.zip
									</code>
									, then double-click{" "}
									<code className="rounded border border-border bg-muted px-1.5 py-0.5 text-foreground">
										Install_Raksha_Guard.command
									</code>{" "}
									(Right-click → Open if Gatekeeper blocks). See{" "}
									<code className="rounded border border-border bg-muted px-1.5 py-0.5 text-foreground">INSTALL_MACOS.txt</code>. To
									turn OFF / uninstall: double-click{" "}
									<code className="rounded border border-border bg-muted px-1.5 py-0.5 text-foreground">
										Uninstall_Raksha_Guard.command
									</code>{" "}
									and enter the same company uninstall key (
									<code className="rounded border border-border bg-muted px-1.5 py-0.5 text-foreground">UNINSTALL_MACOS.txt</code>
									).
								</p>
							</div>

							<div className="space-y-4">
								<div className="flex items-center gap-2 font-semibold text-foreground">
									<span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs">3</span>
									<span>Open monitored AI websites and verify logs</span>
								</div>
								<p className="text-sm text-foreground/80 pl-8 leading-relaxed">
									Fully quit browsers, reopen, visit a monitored AI site, send a test prompt. Confirm in Prompt Logs and Agents.
								</p>
							</div>

							<div className="rounded-md border border-border bg-muted/40 p-4 text-sm space-y-2">
								<p className="font-semibold text-foreground">Package contents</p>
								<ul className="list-disc pl-5 text-foreground/80 space-y-1.5">
									<li>
										<code className="rounded border border-border bg-card px-1.5 py-0.5 text-foreground">
											Raksha_Guard_Windows.zip
										</code>{" "}
										— Windows{" "}
										<code className="rounded border border-border bg-card px-1.5 py-0.5 text-foreground">
											Raksha_Guard_Setup.exe
										</code>{" "}
										installer &amp; docs
									</li>
									<li>
										<code className="rounded border border-border bg-card px-1.5 py-0.5 text-foreground">
											Raksha_Guard_macOS.zip
										</code>{" "}
										— macOS{" "}
										<code className="rounded border border-border bg-card px-1.5 py-0.5 text-foreground">Raksha_Guard.app</code> +
										Install &amp; Uninstall scripts
									</li>
									<li>
										<code className="rounded border border-border bg-card px-1.5 py-0.5 text-foreground">INSTALL_WINDOWS.txt</code> /{" "}
										<code className="rounded border border-border bg-card px-1.5 py-0.5 text-foreground">INSTALL_MACOS.txt</code> /{" "}
										<code className="rounded border border-border bg-card px-1.5 py-0.5 text-foreground">UNINSTALL_MACOS.txt</code>
									</li>
									<li>
										<code className="rounded border border-border bg-card px-1.5 py-0.5 text-foreground">VERSION.txt</code>
									</li>
								</ul>
							</div>
						</CardContent>
					</Card>
				</TabsContent>

				{/* TAB: AGENT SECURITY TELEMETRY */}
				<TabsContent value="telemetry" className="space-y-6">
					{/* Top Section Filter Cards */}
					<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3.5">
						<Card
							onClick={() => setTelemetryFilter("active")}
							className={`bg-card border-border cursor-pointer transition-all hover:border-primary/50 ${
								telemetryFilter === "active" ? "ring-2 ring-primary border-primary" : ""
							}`}
						>
							<CardHeader className="p-3.5 pb-1.5">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<Radio className="h-3.5 w-3.5 text-emerald-400" /> Active Agents
								</CardDescription>
								<CardTitle className="text-2xl font-bold">{telemetryTotals.totalActive} / {telemetryAgentsRaw?.total ?? telemetryAgents.length}</CardTitle>
							</CardHeader>
							<CardContent className="p-3.5 pt-0">
								<p className="text-[11px] text-muted-foreground">Online within 5m · click to filter</p>
							</CardContent>
						</Card>

						<Card
							onClick={() => setTelemetryFilter("allowed")}
							className={`bg-card border-border cursor-pointer transition-all hover:border-emerald-500/50 ${
								telemetryFilter === "allowed" ? "ring-2 ring-emerald-500 border-emerald-500" : ""
							}`}
						>
							<CardHeader className="p-3.5 pb-1.5">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" /> Allowed Hits
								</CardDescription>
								<CardTitle className="text-2xl font-bold text-emerald-400">{telemetryTotals.totalAllowed}</CardTitle>
							</CardHeader>
							<CardContent className="p-3.5 pt-0">
								<p className="text-[11px] text-muted-foreground">Passed clean</p>
							</CardContent>
						</Card>

						<Card
							onClick={() => setTelemetryFilter("blocked")}
							className={`bg-card border-border cursor-pointer transition-all hover:border-red-500/50 ${
								telemetryFilter === "blocked" ? "ring-2 ring-red-500 border-red-500" : ""
							}`}
						>
							<CardHeader className="p-3.5 pb-1.5">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<AlertTriangle className="h-3.5 w-3.5 text-red-400" /> Blocked Hits
								</CardDescription>
								<CardTitle className="text-2xl font-bold text-red-400">{telemetryTotals.totalBlocked}</CardTitle>
							</CardHeader>
							<CardContent className="p-3.5 pt-0">
								<p className="text-[11px] text-muted-foreground">Policy violations</p>
							</CardContent>
						</Card>

						<Card
							onClick={() => setTelemetryFilter("warn")}
							className={`bg-card border-border cursor-pointer transition-all hover:border-amber-500/50 ${
								telemetryFilter === "warn" ? "ring-2 ring-amber-500 border-amber-500" : ""
							}`}
						>
							<CardHeader className="p-3.5 pb-1.5">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<AlertCircle className="h-3.5 w-3.5 text-amber-400" /> Warn Hits
								</CardDescription>
								<CardTitle className="text-2xl font-bold text-amber-400">{telemetryTotals.totalWarn}</CardTitle>
							</CardHeader>
							<CardContent className="p-3.5 pt-0">
								<p className="text-[11px] text-muted-foreground">Warnings issued</p>
							</CardContent>
						</Card>

						<Card
							onClick={() => setTelemetryFilter("redact")}
							className={`bg-card border-border cursor-pointer transition-all hover:border-purple-500/50 ${
								telemetryFilter === "redact" ? "ring-2 ring-purple-500 border-purple-500" : ""
							}`}
						>
							<CardHeader className="p-3.5 pb-1.5">
								<CardDescription className="flex items-center gap-1.5 text-xs">
									<ShieldCheck className="h-3.5 w-3.5 text-purple-400" /> Redact Hits
								</CardDescription>
								<CardTitle className="text-2xl font-bold text-purple-400">{telemetryTotals.totalRedact}</CardTitle>
							</CardHeader>
							<CardContent className="p-3.5 pt-0">
								<p className="text-[11px] text-muted-foreground">PII/Sensitive data</p>
							</CardContent>
						</Card>
					</div>

					{/* Main Telemetry Table Card */}
					<Card className="bg-card border-border">
						<CardHeader className="pb-4 border-b border-border">
							<div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
								<div>
									<CardTitle className="text-lg font-bold flex items-center gap-2">
										<Shield className="h-5 w-5 text-primary" /> Guard Insights
									</CardTitle>
									<CardDescription className="text-xs mt-0.5">
										Fleet-wide agents (independent of Agents tab page) with intercept hit counts from recent prompt logs
										{(insightStatsRaw?.totals?.total || 0) > 0
											? ` · counts from full database (${(insightStatsRaw?.totals?.total || 0).toLocaleString()} prompt logs)`
											: (telemetryLogsRaw?.total || 0) > TELEMETRY_LOG_LIMIT
												? ` · counts use latest ${TELEMETRY_LOG_LIMIT.toLocaleString()} of ${(telemetryLogsRaw?.total || 0).toLocaleString()} logs`
												: ""}
									</CardDescription>
								</div>

								{/* Section Filter Buttons & Search Input */}
								<div className="flex items-center gap-2 flex-wrap">
									<div className="relative w-full sm:w-64">
										<Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
										<Input
											placeholder="Search Host, User, IP, MAC..."
											value={telemetrySearch}
											onChange={(e) => setTelemetrySearch(e.target.value)}
											className="pl-8 h-8 text-xs bg-background border-border"
										/>
									</div>

									<div className="flex items-center gap-1 bg-muted/40 p-1 rounded-md border border-border">
										<Button
											size="sm"
											variant="ghost"
											onClick={() => setTelemetryFilter("all")}
											className={`h-6 text-[11px] px-2 ${
												telemetryFilter === "all"
													? "bg-foreground/10 text-foreground hover:bg-foreground/15"
													: "text-muted-foreground hover:text-foreground"
											}`}
										>
											All ({telemetryAgents.length})
										</Button>
										<Button
											size="sm"
											variant="ghost"
											onClick={() => setTelemetryFilter("active")}
											className={`h-6 text-[11px] px-2 ${
												telemetryFilter === "active"
													? "bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/25 hover:text-emerald-300"
													: "text-emerald-400/80 hover:text-emerald-300 hover:bg-emerald-500/10"
											}`}
										>
											Active
										</Button>
										<Button
											size="sm"
											variant="ghost"
											onClick={() => setTelemetryFilter("blocked")}
											className={`h-6 text-[11px] px-2 ${
												telemetryFilter === "blocked"
													? "bg-red-500/20 text-red-300 hover:bg-red-500/25 hover:text-red-300"
													: "text-red-400/80 hover:text-red-300 hover:bg-red-500/10"
											}`}
										>
											Blocked
										</Button>
										<Button
											size="sm"
											variant="ghost"
											onClick={() => setTelemetryFilter("redact")}
											className={`h-6 text-[11px] px-2 ${
												telemetryFilter === "redact"
													? "bg-purple-500/20 text-purple-300 hover:bg-purple-500/25 hover:text-purple-300"
													: "text-purple-400/80 hover:text-purple-300 hover:bg-purple-500/10"
											}`}
										>
											Redacted
										</Button>
									</div>
								</div>
							</div>
						</CardHeader>

						<CardContent className="pt-4 p-0">
							{filteredTelemetryAgents.length === 0 ? (
								<div className="p-8 text-center space-y-2">
									<Terminal className="h-8 w-8 text-muted-foreground mx-auto" />
									<p className="text-sm font-medium">No agents match these Guard Insights filters</p>
									<p className="text-xs text-muted-foreground">Try clearing search query or section filters</p>
								</div>
							) : (
								<div className="overflow-x-auto">
									<Table className="w-full text-xs min-w-[1050px]">
										<TableHeader>
											<TableRow className="border-border hover:bg-transparent bg-muted/30">
												<TableHead className="w-[180px] font-semibold">Host / User</TableHead>
												<TableHead className="w-[120px] font-semibold">Host User IP</TableHead>
												<TableHead className="w-[150px] font-semibold">Physical Address (MAC)</TableHead>
												<TableHead className="w-[150px] font-semibold">Transport Name</TableHead>
												<TableHead className="w-[90px] text-center font-semibold text-emerald-400">Allowed</TableHead>
												<TableHead className="w-[90px] text-center font-semibold text-red-400">Blocked</TableHead>
												<TableHead className="w-[90px] text-center font-semibold text-amber-400">Warned</TableHead>
												<TableHead className="w-[90px] text-center font-semibold text-purple-400">Redacted</TableHead>
												<TableHead className="w-[120px] font-semibold">Status</TableHead>
												<TableHead className="w-[80px] text-right font-semibold">Actions</TableHead>
											</TableRow>
										</TableHeader>
										<TableBody>
											{filteredTelemetryAgents.map((item) => {
												const { agent, allowedCount, blockedCount, warnCount, redactCount, isOnline } = item;
												return (
													<TableRow key={agent.id} className="border-border hover:bg-muted/40 transition-colors">
														<TableCell className="font-mono py-3">
															<div className="font-semibold text-foreground truncate max-w-[170px]" title={agent.hostname || "—"}>
																{agent.hostname || "Unknown Host"}
															</div>
															<div className="text-[11px] text-muted-foreground truncate" title={agent.username || "—"}>
																{agent.username ? `user: ${agent.username}` : "—"}
															</div>
														</TableCell>
														<TableCell className="font-mono text-muted-foreground">{agent.ip_address || "—"}</TableCell>
														<TableCell className="font-mono text-xs">
															{agent.mac_address ? (
																<Badge variant="outline" className="font-mono text-[10px] bg-background border-border">
																	{agent.mac_address}
																</Badge>
															) : (
																<span className="text-muted-foreground">—</span>
															)}
														</TableCell>
														<TableCell className="text-muted-foreground">
															<div className="font-medium text-foreground truncate max-w-[140px]">
																{agent.transport_name || (agent.agent_type === "network" ? "Network Proxy" : "Raksha Guard")}
															</div>
															<div className="text-[10px] text-muted-foreground">v{agent.agent_version || "1.0"}</div>
														</TableCell>

														{/* Action Hits */}
														<TableCell className="text-center font-mono">
															<Badge className="bg-emerald-950/80 text-emerald-400 border-emerald-700/60 font-semibold px-2 py-0.5 text-xs">
																{allowedCount}
															</Badge>
														</TableCell>
														<TableCell className="text-center font-mono">
															<Badge
																className={
																	blockedCount > 0
																		? "bg-red-950/80 text-red-400 border-red-700/60 font-bold px-2 py-0.5 text-xs"
																		: "bg-muted/40 text-muted-foreground border-border px-2 py-0.5 text-xs"
																}
															>
																{blockedCount}
															</Badge>
														</TableCell>
														<TableCell className="text-center font-mono">
															<Badge
																className={
																	warnCount > 0
																		? "bg-amber-950/80 text-amber-400 border-amber-700/60 font-semibold px-2 py-0.5 text-xs"
																		: "bg-muted/40 text-muted-foreground border-border px-2 py-0.5 text-xs"
																}
															>
																{warnCount}
															</Badge>
														</TableCell>
														<TableCell className="text-center font-mono">
															<Badge
																className={
																	redactCount > 0
																		? "bg-purple-950/80 text-purple-300 border-purple-700/60 font-semibold px-2 py-0.5 text-xs"
																		: "bg-muted/40 text-muted-foreground border-border px-2 py-0.5 text-xs"
																}
															>
																{redactCount}
															</Badge>
														</TableCell>

														{/* Heartbeat Status */}
														<TableCell>
															<div className="flex items-center gap-1.5">
																<span className={`h-2 w-2 rounded-full shrink-0 ${isOnline ? "bg-emerald-500 animate-pulse" : "bg-zinc-600"}`} />
																<span className={isOnline ? "text-emerald-400 font-medium" : "text-muted-foreground"}>
																	{isOnline ? "Active" : "Offline"}
																</span>
															</div>
															<div className="text-[10px] text-muted-foreground mt-0.5">
																{agent.last_seen_at ? new Date(agent.last_seen_at).toLocaleTimeString() : "—"}
															</div>
														</TableCell>

														{/* Actions */}
														<TableCell className="text-right">
															<div className="flex items-center justify-end gap-1.5">
																<Button
																	variant="outline"
																	size="sm"
																	className="h-7 px-2 text-[11px] gap-1 border-border hover:bg-accent"
																	onClick={() => setSelectedTelemetryAgentId(agent.id)}
																>
																	<Activity className="h-3 w-3 text-primary" /> Inspect
																</Button>
																<DropdownMenu>
																	<DropdownMenuTrigger asChild>
																		<Button
																			variant="ghost"
																			size="sm"
																			className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground hover:bg-muted"
																			title="Agent Options"
																		>
																			<MoreHorizontal className="h-4 w-4" />
																		</Button>
																	</DropdownMenuTrigger>
																	<DropdownMenuContent align="end" className="w-56">
																		<DropdownMenuItem
																			onClick={() =>
																				handleOpenWarningMail(agent, {
																					allowed: allowedCount,
																					blocked: blockedCount,
																					warn: warnCount,
																					redact: redactCount,
																				})
																			}
																			className="gap-2 cursor-pointer text-amber-500 focus:text-amber-400 focus:bg-amber-950/30"
																		>
																			<Mail className="h-3.5 w-3.5" />
																			Send Security Warning Mail
																		</DropdownMenuItem>
																		<DropdownMenuItem
																			onClick={() => setSelectedTelemetryAgentId(agent.id)}
																			className="gap-2 cursor-pointer"
																		>
																			<Activity className="h-3.5 w-3.5 text-primary" />
																			View Intercept Telemetry
																		</DropdownMenuItem>
																	</DropdownMenuContent>
																</DropdownMenu>
															</div>
														</TableCell>
													</TableRow>
												);
											})}
										</TableBody>
									</Table>
								</div>
							)}
						</CardContent>
					</Card>
				</TabsContent>
			</Tabs>

			{/* Agent Telemetry Inspector Dialog */}
			<Dialog open={selectedTelemetryAgentId !== null} onOpenChange={(open) => !open && setSelectedTelemetryAgentId(null)}>
				<DialogContent className="max-w-2xl bg-card border-border text-foreground max-h-[min(90vh,820px)] overflow-hidden flex flex-col no-scrollbar">
					<DialogHeader className="shrink-0">
						<DialogTitle className="flex items-center gap-2 text-base font-semibold">
							<Shield className="h-4 w-4 text-primary" />
							Guard Insights — {selectedTelemetryAgent?.hostname || "Host Agent"}
						</DialogTitle>
						<DialogDescription>
							Hardware identity, Guard version, and recent allow / block / warn / redact activity from this device
						</DialogDescription>
					</DialogHeader>

					{selectedTelemetryAgent && selectedTelemetryItem && (
						<div className="space-y-4 text-xs overflow-y-auto flex-1 min-h-0 pr-1 no-scrollbar">
							<div className="grid grid-cols-2 gap-3">
								<div className="rounded-md border border-border bg-background p-3 space-y-1">
									<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Device &amp; User</p>
									<p className="font-mono text-sm text-foreground font-semibold">
										{selectedTelemetryAgent.hostname || "—"} / {selectedTelemetryAgent.username || "—"}
									</p>
								</div>
								<div className="rounded-md border border-border bg-background p-3 space-y-1">
									<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Host User IP</p>
									<p className="font-mono text-sm text-emerald-400 font-semibold">{selectedTelemetryAgent.ip_address || "—"}</p>
								</div>
							</div>

							<div className="grid grid-cols-2 gap-3">
								<div className="rounded-md border border-border bg-background p-3 space-y-1">
									<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Physical MAC Address</p>
									<p className="font-mono text-xs text-foreground font-medium">{selectedTelemetryAgent.mac_address || "—"}</p>
								</div>
								<div className="rounded-md border border-border bg-background p-3 space-y-1">
									<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Transport / OS / Version</p>
									<p className="font-mono text-xs text-foreground">
										{selectedTelemetryAgent.transport_name || "Raksha Guard"} ({selectedTelemetryAgent.os_version || "OS"})
									</p>
									<p className="font-mono text-xs text-primary font-semibold mt-0.5">
										Guard v{selectedTelemetryAgent.agent_version || "—"}
									</p>
								</div>
							</div>

							<div className="grid grid-cols-4 gap-2">
								<div className="rounded-md border border-border bg-background p-2 text-center">
									<p className="text-[10px] text-muted-foreground uppercase">Allowed</p>
									<p className="text-sm font-bold text-emerald-400">{selectedTelemetryItem.allowedCount}</p>
								</div>
								<div className="rounded-md border border-border bg-background p-2 text-center">
									<p className="text-[10px] text-muted-foreground uppercase">Blocked</p>
									<p className="text-sm font-bold text-red-400">{selectedTelemetryItem.blockedCount}</p>
								</div>
								<div className="rounded-md border border-border bg-background p-2 text-center">
									<p className="text-[10px] text-muted-foreground uppercase">Warned</p>
									<p className="text-sm font-bold text-amber-400">{selectedTelemetryItem.warnCount}</p>
								</div>
								<div className="rounded-md border border-border bg-background p-2 text-center">
									<p className="text-[10px] text-muted-foreground uppercase">Redacted</p>
									<p className="text-sm font-bold text-purple-400">{selectedTelemetryItem.redactCount}</p>
								</div>
							</div>

							<div className="rounded-md border border-border bg-background p-3 space-y-2">
								<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
									Recent telemetry logs ({Math.min(selectedTelemetryItem.matchingLogs.length, 25)} of {selectedTelemetryItem.matchingLogs.length})
								</p>
								{selectedTelemetryItem.matchingLogs.length === 0 ? (
									<p className="text-muted-foreground py-4 text-center">No matching prompt logs for this agent in the recent window.</p>
								) : (
									<div className="space-y-2 max-h-[280px] overflow-y-auto no-scrollbar">
										{selectedTelemetryItem.matchingLogs.slice(0, 25).map((log) => (
											<div key={log.id} className="rounded border border-border/70 bg-card/50 p-2.5 space-y-1">
												<div className="flex flex-wrap items-center gap-2 justify-between">
													<div className="flex items-center gap-2 min-w-0">
														{getPlatformBadge(log.platform)}
														{logActionBadge(log)}
													</div>
													<span className="text-[10px] text-muted-foreground shrink-0">
														{log.timestamp ? new Date(log.timestamp).toLocaleString() : "—"}
													</span>
												</div>
												<p className="text-[11px] text-foreground/90 line-clamp-2 break-words">
													{log.user_prompt_preview || log.user_prompt_full || "—"}
												</p>
												{log.rule_triggered ? (
													<p className="text-[10px] text-muted-foreground truncate">Rule: {log.rule_triggered}</p>
												) : null}
											</div>
										))}
									</div>
								)}
							</div>
						</div>
					)}

					<DialogFooter className="shrink-0">
						<Button variant="outline" size="sm" onClick={() => setSelectedTelemetryAgentId(null)}>
							Close
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* Prompt Details — centered modal */}
			<Dialog
				open={!!selectedLog}
				onOpenChange={(open) => {
					if (!open) setSelectedLog(null);
				}}
			>
				<DialogContent
					disableOutsideClick={false}
					className="bg-card border-border text-foreground sm:max-w-2xl w-[calc(100%-2rem)] p-0 gap-0 overflow-hidden flex flex-col max-h-[min(88vh,860px)] no-scrollbar"
				>
					{selectedLog && (
						<>
							<DialogHeader className="px-6 pt-5 pb-4 shrink-0 border-b border-border/70 space-y-1.5 text-left">
								<DialogTitle className="flex flex-wrap items-center gap-2 text-lg pr-8">
									Prompt Details
									{getPlatformBadge(selectedLog.platform)}
								</DialogTitle>
								<DialogDescription className="text-xs">
									Captured {new Date(selectedLog.timestamp).toLocaleString()}
								</DialogDescription>
							</DialogHeader>

							<div className="px-6 py-5 space-y-5 overflow-y-auto flex-1 min-h-0 no-scrollbar">
								<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
									<div className="rounded-lg border border-border/80 bg-background/60 p-3.5 space-y-1.5">
										<Label className="text-[11px] uppercase tracking-wide text-muted-foreground">Action / Status</Label>
										<div className="space-y-1">
											{logActionBadge(selectedLog)}
											{selectedLog.status ? (
												<p className="text-[11px] text-muted-foreground leading-snug">{selectedLog.status}</p>
											) : null}
										</div>
									</div>
									<div className="rounded-lg border border-border/80 bg-background/60 p-3.5 space-y-1.5">
										<Label className="text-[11px] uppercase tracking-wide text-muted-foreground">Desktop Name</Label>
										<p className="text-sm font-medium truncate">{selectedLog.agent_hostname || "—"}</p>
										<p className="text-[11px] text-muted-foreground font-mono truncate">
											{selectedLog.agent_id || selectedLog.client_ip || ""}
										</p>
									</div>
								</div>

								{(() => {
									const v = securityVerdictFromLog(selectedLog);
									const tone =
										v.tone === "ok"
											? "border-emerald-800 bg-emerald-950/30 text-emerald-100"
											: v.tone === "bad"
												? "border-red-800 bg-red-950/30 text-red-100"
												: v.tone === "warn"
													? "border-amber-800 bg-amber-950/30 text-amber-100"
													: "border-border bg-background/60 text-foreground";
									return (
										<div className={`rounded-lg border p-3.5 space-y-1 ${tone}`}>
											<p className="text-[11px] uppercase tracking-wide opacity-80">Security analysis</p>
											<p className="text-sm font-semibold">{v.title}</p>
											{v.detail ? <p className="text-xs opacity-90">{v.detail}</p> : null}
										</div>
									);
								})()}

								<div className="rounded-lg border border-border/80 bg-background/60 p-4 space-y-2.5">
									<div className="flex justify-between items-center text-xs font-semibold gap-3">
										<span className="flex items-center gap-1.5 text-purple-300">
											<BrainCircuit className="h-4 w-4 shrink-0" /> Predictive Risk Score
										</span>
										<span
											className={
												(selectedLog.risk_score || 0) >= 70
													? "text-red-400 font-bold"
													: (selectedLog.risk_score || 0) >= 40
														? "text-amber-400"
														: "text-emerald-400"
											}
										>
											{selectedLog.risk_score || 10}% ({selectedLog.predictive_risk || "LOW"})
										</span>
									</div>
									<div className="w-full bg-slate-800 h-2 rounded-full overflow-hidden">
										<div
											className={`h-full rounded-full transition-all ${(selectedLog.risk_score || 0) >= 70
													? "bg-red-500"
													: (selectedLog.risk_score || 0) >= 40
														? "bg-amber-500"
														: "bg-emerald-500"
												}`}
											style={{ width: `${Math.min(100, Math.max(5, selectedLog.risk_score || 10))}%` }}
										/>
									</div>
									<div className="flex flex-wrap justify-between gap-2 text-[11px] text-muted-foreground pt-0.5">
										<span>
											Category: <code className="text-foreground">{selectedLog.predicted_category || "SAFE"}</code>
										</span>
										<span>Threat Level: {selectedLog.predictive_risk || "LOW"}</span>
									</div>
								</div>

								<div className="space-y-2">
									<div className="flex justify-between items-center gap-2">
										<Label className="text-[11px] uppercase tracking-wide text-muted-foreground">
											{isFileUploadLog(selectedLog) ? "File upload event" : "Full Intercepted Prompt Text"}
										</Label>
										<Button
											variant="ghost"
											size="sm"
											onClick={() =>
												handleCopyPrompt(
													isFileUploadLog(selectedLog)
														? logFileStatusLine(selectedLog)
														: selectedLog.user_prompt_full,
												)
											}
											className="h-7 text-xs gap-1 shrink-0"
										>
											{copiedPrompt ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
											{copiedPrompt ? "Copied" : "Copy"}
										</Button>
									</div>
									<div className="p-3.5 bg-background border border-border rounded-lg font-mono text-xs max-h-52 overflow-y-auto whitespace-pre-wrap leading-relaxed no-scrollbar">
										{isFileUploadLog(selectedLog)
											? logFileStatusLine(selectedLog)
											: selectedLog.user_prompt_full}
									</div>
									{isFileUploadLog(selectedLog) && logExtractedTextFromPrompt(selectedLog) ? (
										<p className="text-[11px] text-muted-foreground">
											Extracted file text is available under <strong className="font-medium text-foreground">View → Extracted text</strong>.
										</p>
									) : null}
								</div>

								{isFileUploadLog(selectedLog) ? (
									<div className="rounded-lg border border-sky-900/50 bg-sky-950/20 p-3.5 flex flex-wrap items-center justify-between gap-3">
										<div className="flex min-w-0 items-center gap-2">
											<Paperclip className="h-4 w-4 shrink-0 text-sky-400" />
											<div className="min-w-0">
												<p className="text-[11px] uppercase tracking-wide text-muted-foreground">Attached file</p>
												<p className="text-sm font-medium truncate">{logAttachmentLabel(selectedLog)}</p>
											</div>
										</div>
										{logHasStoredAttachment(selectedLog) ? (
											<div className="flex items-center gap-2 shrink-0">
												<Button size="sm" variant="outline" className="h-8 gap-1.5" onClick={() => openPdfViewer(selectedLog)}>
													<Eye className="h-3.5 w-3.5" /> View
												</Button>
												<Button size="sm" className="h-8 gap-1.5" onClick={() => downloadPdfAttachment(selectedLog)}>
													<Download className="h-3.5 w-3.5" /> Download
												</Button>
											</div>
										) : (
											<p className="text-xs text-muted-foreground shrink-0 max-w-[14rem] text-right leading-snug">
												{(selectedLog.action || "").toLowerCase() === "blocked"
													? "File bytes not stored — View unavailable for this block event"
													: "Filename logged — file bytes not stored yet"}
											</p>
										)}
									</div>
								) : null}

								<div className="rounded-lg border border-border/80 bg-background/60 p-3.5 space-y-1">
									<Label className="text-[11px] uppercase tracking-wide text-muted-foreground">Guard decision (predict)</Label>
									<p className="text-sm font-semibold break-words">{predictReasonLabel(selectedLog)}</p>
									<p className="text-[11px] text-muted-foreground">
										Risk: {selectedLog.predictive_risk || "LOW"}
										{selectedLog.risk_score != null ? ` · score ${selectedLog.risk_score}` : ""}
										{selectedLog.predicted_category ? ` · ${selectedLog.predicted_category}` : ""}
									</p>
								</div>

								<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
									<div className="rounded-lg border border-border/80 bg-background/60 p-3.5 space-y-1">
										<Label className="text-[11px] uppercase tracking-wide text-muted-foreground">Estimated Tokens</Label>
										<p className="text-sm font-semibold">{selectedLog.est_tokens} tokens</p>
									</div>
									<div className="rounded-lg border border-border/80 bg-background/60 p-3.5 space-y-1">
										<Label className="text-[11px] uppercase tracking-wide text-muted-foreground">Violated Rule</Label>
										<p className={`text-sm font-semibold ${selectedLog.rule_triggered && selectedLog.action !== "Allowed" ? "text-purple-300" : "text-muted-foreground"}`}>
											{selectedLog.action === "Allowed" && (selectedLog.predicted_category || "").toUpperCase() === "AI_GUARD_BOT_CLEAR"
												? "None (checked — no violation)"
												: (selectedLog.rule_triggered || "None")}
										</p>
									</div>
								</div>

								{isFileUploadLog(selectedLog) && logExtractedText(selectedLog) ? (
									<div className="space-y-2">
										<div className="flex items-center justify-between gap-2">
											<Label className="text-[11px] uppercase tracking-wide text-muted-foreground">Extracted file text (used for bot check)</Label>
											{logExtractedText(selectedLog).length > 4000 ? (
												<Button
													type="button"
													variant="ghost"
													size="sm"
													className="h-7 text-xs"
													onClick={() => setExtractedTextExpanded((v) => !v)}
												>
													{extractedTextExpanded ? "Show less" : "Show all"}
												</Button>
											) : null}
										</div>
										<pre className="p-3.5 bg-background border border-border rounded-lg font-mono text-[11px] max-h-64 overflow-auto whitespace-pre-wrap break-words no-scrollbar">
											{extractedTextExpanded
												? logExtractedText(selectedLog)
												: logExtractedText(selectedLog).slice(0, 4000)}
										</pre>
									</div>
								) : null}

								{selectedLog.metadata ? (
									<div className="space-y-2">
										<Label className="text-[11px] uppercase tracking-wide text-muted-foreground">Metadata Payload</Label>
										<pre className="p-3.5 bg-background border border-border rounded-lg font-mono text-[11px] max-h-36 overflow-auto no-scrollbar">
											{(() => {
												try {
													return JSON.stringify(JSON.parse(selectedLog.metadata || "{}"), null, 2);
												} catch {
													return selectedLog.metadata;
												}
											})()}
										</pre>
									</div>
								) : null}
							</div>
						</>
					)}
				</DialogContent>
			</Dialog>

			{/* Attachment viewer — centered popup (PDF / image / download others) */}
			<Dialog
				open={!!pdfViewerLog}
				onOpenChange={(open) => {
					if (!open) {
						setPdfViewerLog(null);
						setPdfViewerTab("preview");
						setPdfError("");
						setAttachmentPreviewKind(null);
						setAttachmentPreviewHtml("");
						setAttachmentPreviewText("");
						setAttachmentBlob(null);
						setAttachmentSheets([]);
						setAttachmentSheetIndex(0);
						setAttachmentTruncated(false);
						setAttachmentShowAll(false);
					}
				}}
			>
				<DialogContent
					disableOutsideClick={false}
					className="bg-card border-border text-foreground sm:max-w-4xl w-[calc(100%-2rem)] p-0 gap-0 overflow-hidden flex flex-col max-h-[min(92vh,920px)] no-scrollbar"
				>
					{pdfViewerLog && (
						<>
							<DialogHeader className="px-5 pt-4 pb-3 shrink-0 border-b border-border/70 space-y-1 text-left">
								<DialogTitle className="flex flex-wrap items-center gap-2 text-base pr-8">
									<FileText className="h-4 w-4 text-sky-400" />
									{logAttachmentLabel(pdfViewerLog)}
								</DialogTitle>
								<DialogDescription className="text-xs">
									{pdfViewerLog.platform} · {pdfViewerLog.action || "—"} · Captured{" "}
									{new Date(pdfViewerLog.timestamp).toLocaleString()}
								</DialogDescription>
							</DialogHeader>
							<div className="px-5 py-3 flex flex-wrap items-center gap-2 shrink-0 border-b border-border/50">
								<Button size="sm" className="h-8 gap-1.5" onClick={() => downloadPdfAttachment(pdfViewerLog)} disabled={pdfLoading}>
									<Download className="h-3.5 w-3.5" /> Download
								</Button>
								{attachmentTruncated && attachmentBlob ? (
									<Button
										size="sm"
										variant="outline"
										className="h-8 gap-1.5"
										disabled={pdfLoading}
										onClick={() => reloadAttachmentPreview(true)}
									>
										{pdfLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
										Show all
									</Button>
								) : null}
								{attachmentShowAll ? (
									<Button
										size="sm"
										variant="ghost"
										className="h-8 text-xs"
										disabled={pdfLoading}
										onClick={() => reloadAttachmentPreview(false)}
									>
										Show less
									</Button>
								) : null}
								{attachmentSheets.length > 1 ? (
									<div className="flex flex-wrap items-center gap-1.5">
										{attachmentSheets.map((sheet, idx) => (
											<Button
												key={`${sheet.name}-${idx}`}
												size="sm"
												variant={idx === attachmentSheetIndex ? "secondary" : "outline"}
												className="h-7 text-[11px]"
												onClick={() => {
													setAttachmentSheetIndex(idx);
													setAttachmentPreviewHtml(sheet.html);
												}}
											>
												{sheet.name}
												<span className="opacity-70 ml-1">({sheet.rowCount})</span>
											</Button>
										))}
									</div>
								) : null}
								<div className="flex rounded-md border border-border overflow-hidden ml-auto">
									<Button
										size="sm"
										variant={pdfViewerTab === "preview" ? "default" : "ghost"}
										className="h-8 rounded-none"
										onClick={() => setPdfViewerTab("preview")}
									>
										Preview
									</Button>
									{(isFileUploadLog(pdfViewerLog) || logExtractedText(pdfViewerLog, attachmentPreviewText)) && (
										<Button
											size="sm"
											variant={pdfViewerTab === "extracted" ? "default" : "ghost"}
											className="h-8 rounded-none"
											onClick={() => setPdfViewerTab("extracted")}
										>
											Extracted text
										</Button>
									)}
									<Button
										size="sm"
										variant={pdfViewerTab === "details" ? "default" : "ghost"}
										className="h-8 rounded-none"
										onClick={() => setPdfViewerTab("details")}
									>
										Prompt details
									</Button>
								</div>
							</div>
							<div className="flex-1 min-h-0 bg-black/40 flex items-center justify-center p-3 overflow-auto no-scrollbar">
								{pdfViewerTab === "details" ? (
									<div className="w-full max-h-[min(70vh,720px)] overflow-auto rounded-md border border-border bg-background p-4 space-y-3 no-scrollbar">
										<div className="flex flex-wrap items-center justify-between gap-2">
											<p className="text-xs text-muted-foreground">
												{pdfViewerLog.platform} · {pdfViewerLog.action || "—"}
												{pdfViewerLog.rule_triggered ? ` · ${pdfViewerLog.rule_triggered}` : ""}
											</p>
											<Button
												variant="ghost"
												size="sm"
												onClick={() =>
													handleCopyPrompt(
														isFileUploadLog(pdfViewerLog)
															? logFileStatusLine(pdfViewerLog)
															: pdfViewerLog.user_prompt_full || pdfViewerLog.user_prompt_preview || "",
													)
												}
												className="h-7 text-xs gap-1 shrink-0"
											>
												{copiedPrompt ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
												{copiedPrompt ? "Copied" : "Copy"}
											</Button>
										</div>
										<pre className="text-xs font-mono whitespace-pre-wrap leading-relaxed">
											{isFileUploadLog(pdfViewerLog)
												? logFileStatusLine(pdfViewerLog) || "File upload event."
												: pdfViewerLog.user_prompt_full || pdfViewerLog.user_prompt_preview || "No prompt text captured."}
										</pre>
									</div>
								) : pdfViewerTab === "extracted" ? (
									<div className="w-full max-h-[min(70vh,720px)] overflow-auto rounded-md border border-border bg-background p-4 space-y-2 no-scrollbar">
										<p className="text-xs text-muted-foreground">
											Text extracted from the uploaded file for DLP / rule scanning (not shown in the main log table).
										</p>
										<pre className="text-xs font-mono whitespace-pre-wrap leading-relaxed">
											{logExtractedText(pdfViewerLog, attachmentPreviewText) ||
												(pdfLoading ? "Loading…" : "No text could be extracted from this file.")}
										</pre>
									</div>
								) : pdfLoading ? (
									<p className="text-sm text-muted-foreground">Loading document…</p>
								) : pdfError ? (
									<p className="text-sm text-red-400">{pdfError}</p>
								) : attachmentPreviewKind === "image" && pdfBlobUrl ? (
									<img
										src={pdfBlobUrl}
										alt={logAttachmentLabel(pdfViewerLog)}
										className="max-h-[min(70vh,720px)] max-w-full rounded-md border border-border object-contain bg-black/20"
									/>
								) : attachmentPreviewKind === "pdf" && pdfBlobUrl ? (
									<embed
										title={logAttachmentLabel(pdfViewerLog)}
										src={pdfBlobUrl}
										type="application/pdf"
										className="w-full h-[min(70vh,720px)] rounded-md border border-border bg-neutral-900"
									/>
								) : attachmentPreviewKind === "html" && attachmentPreviewHtml ? (
									<div
										className="w-full max-h-[min(70vh,720px)] overflow-auto rounded-md border border-border bg-background p-4 text-foreground no-scrollbar"
										dangerouslySetInnerHTML={{ __html: attachmentPreviewHtml }}
									/>
								) : attachmentPreviewKind === "text" && attachmentPreviewText ? (
									<pre className="w-full max-h-[min(70vh,720px)] overflow-auto rounded-md border border-border bg-background p-4 text-xs font-mono whitespace-pre-wrap no-scrollbar">
										{attachmentPreviewText}
									</pre>
								) : (
									<div className="text-center space-y-3 p-6">
										<p className="text-sm text-muted-foreground">
											In-browser preview is not available for this file type. Download to open it locally.
										</p>
										<Button size="sm" className="gap-1.5" onClick={() => downloadPdfAttachment(pdfViewerLog)}>
											<Download className="h-3.5 w-3.5" /> Download
										</Button>
									</div>
								)}
							</div>
						</>
					)}
				</DialogContent>
			</Dialog>

			{/* Search Log Inspection Dialog */}
			<Dialog open={selectedSearchLog !== null} onOpenChange={(open) => !open && setSelectedSearchLog(null)}>
				<DialogContent className="max-w-xl bg-card border-border">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2 text-base font-semibold">
							<Search className="h-4 w-4 text-emerald-400" />
							Search Event Inspection
						</DialogTitle>
						<DialogDescription>
							Detailed telemetry captured from search engine session
						</DialogDescription>
					</DialogHeader>

					{selectedSearchLog && (
						<div className="space-y-4 text-xs">
							{/* Query box */}
							<div className="rounded-md border border-border bg-background p-3 space-y-1">
								<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Search Query / Prompt</p>
								<p className="font-mono text-sm text-foreground font-semibold">
									{selectedSearchLog.query || "[Direct Result Navigation without Query]"}
								</p>
							</div>

							{/* Clicked link if any */}
							{selectedSearchLog.clicked_url && (
								<div className="rounded-md border border-border bg-background p-3 space-y-1">
									<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Clicked Destination Link</p>
									<a
										href={selectedSearchLog.clicked_url}
										target="_blank"
										rel="noopener noreferrer"
										className="inline-flex items-center gap-1.5 text-blue-400 hover:underline font-mono text-xs break-all"
									>
										<ExternalLink className="h-3 w-3 shrink-0" />
										{selectedSearchLog.clicked_url}
									</a>
								</div>
							)}

							{/* Threat Assessment */}
							<div className="grid grid-cols-2 gap-3">
								<div className="rounded-md border border-border bg-background p-3 space-y-1">
									<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Predictive Threat Risk</p>
									<div className="flex items-center gap-2">
										<Badge
											className={
												selectedSearchLog.predictive_risk === "CRITICAL"
													? "bg-red-950/80 text-red-400 border-red-800/80 font-bold"
													: selectedSearchLog.predictive_risk === "HIGH"
														? "bg-amber-950/80 text-amber-400 border-amber-800/80 font-bold"
														: "bg-emerald-950/80 text-emerald-400 border-emerald-800/80"
											}
										>
											{selectedSearchLog.predictive_risk} ({selectedSearchLog.risk_score}%)
										</Badge>
										<span className="text-muted-foreground">{selectedSearchLog.risk_category}</span>
									</div>
								</div>

								<div className="rounded-md border border-border bg-background p-3 space-y-1">
									<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Privacy Mode</p>
									<div>
										{selectedSearchLog.is_incognito ? (
											<Badge className="bg-purple-950/80 text-purple-300 border-purple-800/80 gap-1 font-medium">
												<EyeOff className="h-3 w-3 text-purple-400" /> Incognito / InPrivate Mode
											</Badge>
										) : (
											<Badge variant="outline" className="text-muted-foreground gap-1">
												<Eye className="h-3 w-3" /> Normal Browsing
											</Badge>
										)}
									</div>
								</div>
							</div>

							{/* Technical Details */}
							<div className="rounded-md border border-border bg-background p-3 space-y-2">
								<p className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">Technical Metadata</p>
								<div className="grid grid-cols-2 gap-2 text-muted-foreground font-mono text-[11px]">
									<div><span className="text-foreground font-semibold">Engine:</span> {selectedSearchLog.engine}</div>
									<div><span className="text-foreground font-semibold">Browser:</span> {selectedSearchLog.browser}</div>
									<div><span className="text-foreground font-semibold">Client IP:</span> {selectedSearchLog.client_ip}</div>
									<div><span className="text-foreground font-semibold">Desktop Name:</span> {selectedSearchLog.agent_hostname || "Local Endpoint"}</div>
									<div className="col-span-2 break-all"><span className="text-foreground font-semibold">Host:</span> {selectedSearchLog.host}</div>
									<div className="col-span-2"><span className="text-foreground font-semibold">Timestamp:</span> {new Date(selectedSearchLog.timestamp).toLocaleString()}</div>
								</div>
							</div>
						</div>
					)}

					<DialogFooter>
						<Button variant="outline" size="sm" onClick={() => setSelectedSearchLog(null)}>
							Close
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* REMOTE UNINSTALL / TURN OFF GUARD DIALOG */}
			<Dialog open={remoteUninstallDialogOpen} onOpenChange={setRemoteUninstallDialogOpen}>
				<DialogContent className="bg-card border-border text-foreground w-[calc(100%-2rem)] sm:max-w-md">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2 text-base text-red-400">
							<PowerOff className="h-5 w-5 text-red-400" />
							Turn Off / Remote Uninstall Guard
						</DialogTitle>
						<DialogDescription className="text-xs">
							Send a remote shutdown signal to{" "}
							<strong className="text-foreground">{targetAgentToUninstall?.hostname || targetAgentToUninstall?.id}</strong>.
							The laptop will clear PAC/autostart, stop Guard, and delete the installed EXE / .app on the next heartbeat.
						</DialogDescription>
					</DialogHeader>

					{remoteUninstallError && (
						<div className="p-3 bg-red-950/60 border border-red-800 text-red-400 rounded-md text-xs">
							{remoteUninstallError}
						</div>
					)}

					{remoteUninstallSuccess && (
						<div className="p-3 bg-emerald-950/60 border border-emerald-800 text-emerald-300 rounded-md text-xs flex items-center gap-2">
							<CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-400" />
							{remoteUninstallSuccess}
						</div>
					)}

					{!remoteUninstallSuccess && (
						<div className="space-y-3 py-2">
							<div className="space-y-1.5">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-1.5">
										<Label className="text-xs">Today&apos;s Daily Uninstall Key</Label>
										<Badge variant="outline" className="text-[10px] py-0 px-1 border-emerald-500/40 text-emerald-400 bg-emerald-500/10">
											24h Rolling
										</Badge>
									</div>
									<button
										type="button"
										onClick={() => setShowRemoteUninstallKey((v) => !v)}
										className="text-[11px] text-muted-foreground hover:text-foreground flex items-center gap-1"
									>
										{showRemoteUninstallKey ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
										{showRemoteUninstallKey ? "Hide" : "Show"}
									</button>
								</div>
								<Input
									type={showRemoteUninstallKey ? "text" : "password"}
									placeholder={guardKeyLoading ? "Loading Guard key…" : "Today's Guard key or company uninstall key"}
									value={remoteUninstallKey}
									onChange={(e) => setRemoteUninstallKey(e.target.value)}
									disabled={guardKeyLoading}
									className="bg-background border-border text-sm font-mono"
								/>
								<p className="text-[11px] text-muted-foreground">
									{guardKeyHint ||
										"Today's auto-rotating key for this Guard (rotates every 24h). Company uninstall key from Setup also works."}
								</p>
							</div>
						</div>
					)}

					<DialogFooter className="gap-2 sm:gap-0">
						<Button
							variant="outline"
							onClick={() => setRemoteUninstallDialogOpen(false)}
							disabled={isRemoteUninstalling}
						>
							Cancel
						</Button>
						{!remoteUninstallSuccess && (
							<Button
								variant="destructive"
								onClick={handleConfirmRemoteUninstall}
								disabled={isRemoteUninstalling || !remoteUninstallKey.trim()}
								className="gap-2 bg-red-600 hover:bg-red-700 text-white"
							>
								{isRemoteUninstalling ? (
									<Loader2 className="h-4 w-4 animate-spin" />
								) : (
									<PowerOff className="h-4 w-4" />
								)}
								Turn Off Guard
							</Button>
						)}
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* GUARD AGENT DETAILS & DAILY UNINSTALL KEY DIALOG */}
			<Dialog
				open={selectedAgentDetails !== null}
				onOpenChange={(open) => {
					if (!open) {
						setSelectedAgentDetails(null);
						setContactEmailSuccess("");
						setContactEmailError("");
					}
				}}
			>
				<DialogContent className="bg-card border-border text-foreground w-[calc(100%-2rem)] sm:max-w-xl max-h-[min(90vh,760px)] overflow-y-auto no-scrollbar">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2 text-base text-foreground font-semibold">
							<ShieldCheck className="h-5 w-5 text-emerald-400" />
							<span>Guard Device Details & Daily Key</span>
							<Badge variant="outline" className="text-[10px] py-0 px-1.5 border-emerald-500/40 text-emerald-400 bg-emerald-500/10">
								100% Encrypted
							</Badge>
						</DialogTitle>
						<DialogDescription className="text-xs">
							View device telemetry, manage employee recipient email (TO MAIL), and inspect today&apos;s 24-hour rotating uninstall key.
						</DialogDescription>
					</DialogHeader>

					{selectedAgentDetails && (
						<div className="space-y-4 py-2 text-xs">
							{/* Device Telemetry Grid */}
							<div className="grid grid-cols-2 sm:grid-cols-3 gap-2.5 p-3 rounded-lg bg-muted/40 border border-border">
								<div>
									<span className="text-[10px] uppercase font-semibold text-muted-foreground block">Device / Hostname</span>
									<span className="font-mono text-xs font-semibold text-foreground break-all">
										{selectedAgentDetails.hostname || selectedAgentDetails.id}
									</span>
								</div>
								<div>
									<span className="text-[10px] uppercase font-semibold text-muted-foreground block">Platform / OS</span>
									<span className="inline-flex items-center gap-1 text-xs text-foreground capitalize">
										{selectedAgentDetails.os_version || "—"}
									</span>
								</div>
								<div>
									<span className="text-[10px] uppercase font-semibold text-muted-foreground block">Agent Version</span>
									<span className="font-mono text-xs text-foreground">
										{selectedAgentDetails.agent_version ? `v${selectedAgentDetails.agent_version}` : "—"}
									</span>
								</div>
								<div>
									<span className="text-[10px] uppercase font-semibold text-muted-foreground block">Agent ID</span>
									<span className="font-mono text-[11px] text-muted-foreground truncate block" title={selectedAgentDetails.id}>
										{selectedAgentDetails.id}
									</span>
								</div>
								<div>
									<span className="text-[10px] uppercase font-semibold text-muted-foreground block">Heartbeat Status</span>
									<span className="inline-flex items-center gap-1.5 text-xs text-emerald-400">
										<span className="h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />
										Active
									</span>
								</div>
								<div>
									<span className="text-[10px] uppercase font-semibold text-muted-foreground block">Last Seen</span>
									<span className="text-[11px] text-muted-foreground">
										{selectedAgentDetails.last_seen_at
											? new Date(selectedAgentDetails.last_seen_at).toLocaleString([], {
													month: "short",
													day: "numeric",
													hour: "2-digit",
													minute: "2-digit",
											  })
											: "Just now"}
									</span>
								</div>
							</div>

							{/* TODAY'S 24-HOUR ROLLING KEY CARD */}
							<div className="p-4 rounded-xl border border-emerald-500/30 bg-emerald-950/20 space-y-3">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2">
										<KeyRound className="h-4 w-4 text-emerald-400" />
										<span className="font-semibold text-sm text-foreground">
											Today&apos;s Daily Uninstall Key
										</span>
									</div>
									<Badge
										variant="outline"
										className="text-[10px] py-0.5 px-2 border-emerald-500/40 text-emerald-300 bg-emerald-500/10 font-mono"
									>
										24-Hour Rolling
									</Badge>
								</div>

								<p className="text-[11px] text-muted-foreground leading-relaxed">
									This key is mathematically rotated every 24 hours. Required for emergency local uninstallation or command-line guard disablement on this specific laptop.
								</p>

								{/* KEY BOX */}
								<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 bg-background/80 p-2.5 rounded-lg border border-border">
									<div className="flex-1 font-mono text-sm tracking-wider flex items-center gap-2 px-2 py-1 select-all break-all">
										{agentDetailsKeyLoading ? (
											<span className="text-muted-foreground flex items-center gap-2">
												<Loader2 className="h-4 w-4 animate-spin text-emerald-400" />
												Fetching today&apos;s rolling key…
											</span>
										) : agentDetailsKey ? (
											showAgentDetailsKey ? (
												<span className="font-bold text-emerald-400">{agentDetailsKey}</span>
											) : (
												<span className="text-muted-foreground tracking-widest">••••••••••••••••••••••••</span>
											)
										) : (
											<span className="text-muted-foreground italic">No daily key generated yet</span>
										)}
									</div>

									<div className="flex items-center gap-1.5 shrink-0 self-end sm:self-auto">
										<Button
											type="button"
											variant="ghost"
											size="sm"
											className="h-8 px-2 text-xs text-muted-foreground hover:text-foreground"
											onClick={() => setShowAgentDetailsKey((v) => !v)}
											disabled={!agentDetailsKey || agentDetailsKeyLoading}
										>
											{showAgentDetailsKey ? <EyeOff className="h-3.5 w-3.5 mr-1" /> : <Eye className="h-3.5 w-3.5 mr-1" />}
											{showAgentDetailsKey ? "Hide" : "Show"}
										</Button>
										<Button
											type="button"
											variant="secondary"
											size="sm"
											className={`h-8 px-2.5 text-xs gap-1.5 transition-all ${
												agentDetailsKeyCopied
													? "bg-emerald-600 text-white hover:bg-emerald-600 font-medium"
													: "bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30"
											}`}
											onClick={async () => {
												if (!agentDetailsKey) return;
												await navigator.clipboard.writeText(agentDetailsKey);
												setAgentDetailsKeyCopied(true);
												setTimeout(() => setAgentDetailsKeyCopied(false), 2000);
											}}
											disabled={!agentDetailsKey || agentDetailsKeyLoading}
										>
											{agentDetailsKeyCopied ? (
												<>
													<Check className="h-3.5 w-3.5 text-white" />
													Copied
												</>
											) : (
												<>
													<Copy className="h-3.5 w-3.5" />
													Copy Key
												</>
											)}
										</Button>
									</div>
								</div>

								<div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2 pt-1 border-t border-emerald-500/10 text-[11px] text-muted-foreground">
									<span>
										{agentDetailsKeyRotatedAt ? (
											<>
												Issued:{" "}
												<span className="font-medium text-foreground">
													{new Date(agentDetailsKeyRotatedAt).toLocaleString([], {
														month: "short",
														day: "numeric",
														hour: "2-digit",
														minute: "2-digit",
													})}
												</span>{" "}
												(Auto-rotates daily)
											</>
										) : (
											"Auto-rotates daily (rolling 24-hour validity)"
										)}
									</span>

									<Button
										type="button"
										variant="ghost"
										size="sm"
										className="h-7 px-2 text-[11px] text-amber-400 hover:text-amber-300 hover:bg-amber-500/10 gap-1 ml-auto"
										onClick={handleRotateDetailsKey}
										disabled={isRotatingGuardKey || agentDetailsKeyLoading}
									>
										{isRotatingGuardKey ? (
											<Loader2 className="h-3 w-3 animate-spin" />
										) : (
											<RefreshCw className="h-3 w-3" />
										)}
										Rotate Key Now
									</Button>
								</div>
							</div>

							{/* RECIPIENT TO MAIL (CONTACT EMAIL) CONFIGURATION */}
							<div className="p-4 rounded-xl border border-border bg-card space-y-3">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2">
										<Mail className="h-4 w-4 text-sky-400" />
										<span className="font-semibold text-sm text-foreground">
											Device Contact Email (TO MAIL)
										</span>
									</div>
									<Badge variant="outline" className="text-[10px] py-0 px-1.5 border-sky-500/40 text-sky-400 bg-sky-500/10">
										Security Alerts
									</Badge>
								</div>

								<p className="text-[11px] text-muted-foreground leading-relaxed">
									The registered recipient email address for this device. Security policy warning notifications, compliance alerts, and emergency recovery instructions will be dispatched to this email.
								</p>

								{contactEmailError && (
									<div className="p-2.5 rounded bg-red-950/60 border border-red-800 text-red-300 text-xs">
										{contactEmailError}
									</div>
								)}

								{contactEmailSuccess && (
									<div className="p-2.5 rounded bg-emerald-950/60 border border-emerald-800 text-emerald-300 text-xs flex items-center gap-2">
										<CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-400" />
										{contactEmailSuccess}
									</div>
								)}

								<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
									<Input
										type="email"
										placeholder="employee.name@company.com"
										value={editingContactEmail}
										onChange={(e) => setEditingContactEmail(e.target.value)}
										className="bg-background border-border text-xs flex-1"
									/>
									<Button
										type="button"
										variant="secondary"
										size="sm"
										className="gap-1.5 shrink-0 bg-sky-600 hover:bg-sky-500 text-white"
										onClick={() => handleSaveContactEmail()}
										disabled={isUpdatingContactEmail}
									>
										{isUpdatingContactEmail ? (
											<Loader2 className="h-3.5 w-3.5 animate-spin" />
										) : (
											<Save className="h-3.5 w-3.5" />
										)}
										Save Email
									</Button>
									{(selectedAgentDetails.contact_email || "").trim() && (
										<Button
											type="button"
											variant="ghost"
											size="sm"
											className="gap-1.5 shrink-0 text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
											onClick={() => handleSaveContactEmail("")}
											disabled={isUpdatingContactEmail}
										>
											<Trash2 className="h-3.5 w-3.5" />
											Remove
										</Button>
									)}
								</div>
								{selectedAgentDetails.contact_email_pinned && (
									<p className="text-[10px] text-muted-foreground">
										Saved by admin — kept permanently (also after Guard reinstall on this laptop) until you edit or remove it.
									</p>
								)}
							</div>
						</div>
					)}

					<DialogFooter className="gap-2 sm:gap-0 pt-2 border-t border-border">
						<Button
							type="button"
							variant="outline"
							onClick={() => {
								setSelectedAgentDetails(null);
								setContactEmailSuccess("");
								setContactEmailError("");
							}}
						>
							Close
						</Button>
						{selectedAgentDetails && (
							<Button
								type="button"
								variant="default"
								className="gap-1.5 bg-amber-600 hover:bg-amber-500 text-white"
								onClick={() => {
									const agent = selectedAgentDetails;
									const tele = telemetryAgents.find((t) => t.agent.id === agent.id);
									const stats = tele
										? { allowed: tele.allowedCount, blocked: tele.blockedCount, warn: tele.warnCount, redact: tele.redactCount }
										: { allowed: 0, blocked: 0, warn: 0, redact: 0 };
									setSelectedAgentDetails(null);
									handleOpenWarningMail(agent, stats);
								}}
							>
								<Send className="h-3.5 w-3.5" />
								Send Warning Mail
							</Button>
						)}
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* Send Security Warning Email Dialog */}
			<Dialog open={warningMailTarget !== null} onOpenChange={(open) => !open && setWarningMailTarget(null)}>
				<DialogContent className="max-w-xl bg-card border-border text-foreground max-h-[min(90vh,750px)] overflow-hidden flex flex-col">
					<DialogHeader className="shrink-0">
						<DialogTitle className="flex items-center gap-2 text-base font-semibold">
							<AlertTriangle className="h-4 w-4 text-amber-500" />
							Send Security Warning Mail
						</DialogTitle>
						<DialogDescription>
							Dispatch an official enterprise security policy warning notification to the employee. Uses company SMTP settings configured in Settings &rarr; Security.
						</DialogDescription>
					</DialogHeader>

					{warningMailTarget && (
						<div className="space-y-4 text-xs overflow-y-auto flex-1 min-h-0 pr-1 no-scrollbar">
							{/* SMTP connection status (Settings → Security) */}
							<div
								className={`flex items-start gap-2 p-2.5 rounded-lg border text-[11px] ${
									smtpReady
										? "bg-emerald-500/10 border-emerald-500/30 text-emerald-400"
										: "bg-amber-500/10 border-amber-500/30 text-amber-400"
								}`}
							>
								{smtpReady ? (
									<>
										<CheckCircle2 className="h-3.5 w-3.5 shrink-0 mt-0.5" />
										<span>
											SMTP connected via Settings → Security
											{smtpConfig?.host ? ` (${smtpConfig.host}${smtpConfig.port ? `:${smtpConfig.port}` : ""})` : ""}.
											Mail will be sent to the recipient below.
										</span>
									</>
								) : (
									<>
										<AlertCircle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
										<span>
											SMTP is not enabled. Open Settings → Security, configure SMTP, enable it, then return here to send the warning report.
										</span>
									</>
								)}
							</div>

							{/* Device Summary Badge */}
							<div className="flex flex-wrap items-center gap-2 p-2.5 rounded-lg bg-muted/40 border border-border">
								<Badge variant="outline" className="border-border bg-card">
									Host: <span className="font-mono font-bold text-foreground ml-1">{warningMailTarget.hostname}</span>
								</Badge>
								<Badge variant="outline" className="border-border bg-card">
									User: <span className="font-mono text-foreground ml-1">{warningMailTarget.username || "—"}</span>
								</Badge>
								<Badge variant="outline" className="border-border bg-card">
									IP: <span className="font-mono text-foreground ml-1">{warningMailTarget.ip_address}</span>
								</Badge>
								{warningMailStats && warningMailStats.blocked > 0 && (
									<Badge className="bg-red-950/80 text-red-400 border-red-800/80 font-bold">
										{warningMailStats.blocked} Blocked
									</Badge>
								)}
								{warningMailStats && warningMailStats.warn > 0 && (
									<Badge className="bg-amber-950/80 text-amber-400 border-amber-800/80 font-semibold">
										{warningMailStats.warn} Warn
									</Badge>
								)}
								{warningMailStats && warningMailStats.redact > 0 && (
									<Badge className="bg-purple-950/80 text-purple-300 border-purple-800/80 font-semibold">
										{warningMailStats.redact} Redact
									</Badge>
								)}
							</div>

							{/* Form fields */}
							<div className="space-y-3">
								<div className="space-y-1.5">
									<Label className="text-xs font-semibold">Recipient Email (To:)</Label>
									<Input
										placeholder="e.g. employee@company.com"
										value={warningMailTo}
										onChange={(e) => setWarningMailTo(e.target.value)}
										className="bg-background border-border text-xs"
									/>
									<p className="text-[11px] text-muted-foreground">
										Mail is delivered to this address using your organization SMTP server.
									</p>
								</div>

								<div className="space-y-1.5">
									<Label className="text-xs font-semibold">Email Subject</Label>
									<Input
										placeholder="Security Warning Subject"
										value={warningMailSubject}
										onChange={(e) => setWarningMailSubject(e.target.value)}
										className="bg-background border-border text-xs"
									/>
								</div>

								<div className="space-y-1.5">
									<Label className="text-xs font-semibold">Warning Report Message</Label>
									<Textarea
										rows={9}
										value={warningMailMessage}
										onChange={(e) => setWarningMailMessage(e.target.value)}
										className="bg-background border-border text-xs leading-relaxed font-mono"
										placeholder="Describe the policy violation and action required by the employee..."
									/>
									<p className="text-[11px] text-muted-foreground">
										Includes device + violation summary report. Sent via Settings → Security SMTP.
									</p>
								</div>

								{warningMailError && (
									<div className="p-3 rounded-md bg-destructive/15 border border-destructive/30 text-destructive text-xs">
										{warningMailError}
									</div>
								)}
							</div>
						</div>
					)}

					<DialogFooter className="gap-2 sm:gap-0 shrink-0 pt-2 border-t border-border">
						<Button
							variant="outline"
							size="sm"
							onClick={() => setWarningMailTarget(null)}
							disabled={isSendingWarningEmail}
							className="text-xs"
						>
							Cancel
						</Button>
						<Button
							variant="default"
							size="sm"
							onClick={handleSendWarningEmail}
							disabled={
								isSendingWarningEmail ||
								!warningMailTo.trim() ||
								!warningMailMessage.trim() ||
								!smtpReady
							}
							className="gap-2 text-xs bg-amber-600 hover:bg-amber-700 text-white font-medium"
						>
							{isSendingWarningEmail ? (
								<Loader2 className="h-3.5 w-3.5 animate-spin" />
							) : (
								<Send className="h-3.5 w-3.5" />
							)}
							Send Security Warning Mail
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* Clear Prompt Logs Dialog */}
			<AlertDialog open={clearLogsDialogOpen} onOpenChange={setClearLogsDialogOpen}>
				<AlertDialogContent className="bg-card border-border text-foreground">
					<AlertDialogHeader>
						<AlertDialogTitle className="flex items-center gap-2 text-destructive">
							<Trash2 className="h-5 w-5" /> Clear Prompt Logs
						</AlertDialogTitle>
						<AlertDialogDescription>
							Permanently remove prompt and chat history logs from the database. This action cannot be undone.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<div className="py-2 space-y-2">
						<Label className="text-xs font-medium">Retention / Time Window</Label>
						<Select value={clearLogsRetention} onValueChange={(val: "all" | "1d" | "7d" | "30d") => setClearLogsRetention(val)}>
							<SelectTrigger className="bg-background border-border text-xs">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="all">All Logs (Purge Entire History)</SelectItem>
								<SelectItem value="30d">Older than 30 Days</SelectItem>
								<SelectItem value="7d">Older than 7 Days</SelectItem>
								<SelectItem value="1d">Older than 1 Day (Keep only last 24h)</SelectItem>
							</SelectContent>
						</Select>
					</div>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={isClearingLogs}>Cancel</AlertDialogCancel>
						<AlertDialogAction
							onClick={(e) => {
								e.preventDefault();
								void handleClearLogs();
							}}
							disabled={isClearingLogs}
							className="bg-destructive hover:bg-destructive/90 text-destructive-foreground font-medium"
						>
							{isClearingLogs ? (
								<>
									<Loader2 className="mr-2 h-4 w-4 animate-spin" />
									Clearing...
								</>
							) : (
								"Clear Prompt Logs"
							)}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			{/* Clear Search Logs Dialog */}
			<AlertDialog open={clearSearchLogsDialogOpen} onOpenChange={setClearSearchLogsDialogOpen}>
				<AlertDialogContent className="bg-card border-border text-foreground">
					<AlertDialogHeader>
						<AlertDialogTitle className="flex items-center gap-2 text-destructive">
							<Trash2 className="h-5 w-5" /> Clear Search Logs
						</AlertDialogTitle>
						<AlertDialogDescription>
							Permanently remove recorded search engine queries and visited links from the database. This action cannot be undone.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<div className="py-2 space-y-2">
						<Label className="text-xs font-medium">Retention / Time Window</Label>
						<Select value={clearSearchLogsRetention} onValueChange={(val: "all" | "1d" | "7d" | "30d") => setClearSearchLogsRetention(val)}>
							<SelectTrigger className="bg-background border-border text-xs">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="all">All Logs (Purge Entire History)</SelectItem>
								<SelectItem value="30d">Older than 30 Days</SelectItem>
								<SelectItem value="7d">Older than 7 Days</SelectItem>
								<SelectItem value="1d">Older than 1 Day (Keep only last 24h)</SelectItem>
							</SelectContent>
						</Select>
					</div>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={isClearingSearchLogs}>Cancel</AlertDialogCancel>
						<AlertDialogAction
							onClick={(e) => {
								e.preventDefault();
								void handleClearSearchLogs();
							}}
							disabled={isClearingSearchLogs}
							className="bg-destructive hover:bg-destructive/90 text-destructive-foreground font-medium"
						>
							{isClearingSearchLogs ? (
								<>
									<Loader2 className="mr-2 h-4 w-4 animate-spin" />
									Clearing...
								</>
							) : (
								"Clear Search Logs"
							)}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			{/* Delete Selected Logs Dialog */}
			<AlertDialog
				open={deleteSelectedTarget !== null}
				onOpenChange={(open) => {
					if (!open && !isDeletingSelectedLogs && !isDeletingSelectedSearchLogs) setDeleteSelectedTarget(null);
				}}
			>
				<AlertDialogContent className="bg-card border-border text-foreground">
					<AlertDialogHeader>
						<AlertDialogTitle className="flex items-center gap-2 text-destructive">
							<Trash2 className="h-5 w-5" /> Delete selected {deleteSelectedTarget === "search-logs" ? "search" : "prompt"} logs
						</AlertDialogTitle>
						<AlertDialogDescription>
							{(deleteSelectedTarget === "search-logs" ? selectedSearchLogIds.size : selectedLogIds.size)} selected log
							{(deleteSelectedTarget === "search-logs" ? selectedSearchLogIds.size : selectedLogIds.size) === 1 ? "" : "s"} will be permanently
							removed from the database. This action cannot be undone.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={isDeletingSelectedLogs || isDeletingSelectedSearchLogs}>Cancel</AlertDialogCancel>
						<AlertDialogAction
							onClick={(e) => {
								e.preventDefault();
								void handleDeleteSelected();
							}}
							disabled={isDeletingSelectedLogs || isDeletingSelectedSearchLogs}
							className="bg-destructive hover:bg-destructive/90 text-destructive-foreground font-medium"
							data-testid="browser-ai-delete-selected-confirm"
						>
							{isDeletingSelectedLogs || isDeletingSelectedSearchLogs ? (
								<>
									<Loader2 className="mr-2 h-4 w-4 animate-spin" />
									Deleting...
								</>
							) : (
								"Delete"
							)}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
