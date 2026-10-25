import React, { useState, useEffect, useMemo } from "react";
import {
	Building2,
	Globe,
	Plus,
	X,
	Save,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useToast } from "@/hooks/use-toast";
import { BrowserControlSettings, BrowserTargetWebsite } from "@/lib/store/apis/browserAiApi";
import { PolicyTargetWebsitesSelector } from "./PolicyTargetWebsitesSelector";

const DEFAULT_BLOCK_MESSAGE =
	"Personal accounts are restricted by corporate security policy. Please log in with your company account.";

export interface CorporateAIWorkspaceCardProps {
	controls: BrowserControlSettings;
	targets: BrowserTargetWebsite[];
	patchControl: (patch: Partial<BrowserControlSettings>) => Promise<boolean>;
}

export function CorporateAIWorkspaceCard({
	controls,
	targets,
	patchControl,
}: CorporateAIWorkspaceCardProps) {
	const { toast } = useToast();

	// Local input states
	const [corporateDomainInput, setCorporateDomainInput] = useState("");
	const [workspaceIdInput, setWorkspaceIdInput] = useState("");
	const [corporateBlockMsgInput, setCorporateBlockMsgInput] = useState(
		controls.corporate_block_message || DEFAULT_BLOCK_MESSAGE
	);

	// Sync message from backend controls
	useEffect(() => {
		if (controls.corporate_block_message) {
			setCorporateBlockMsgInput(controls.corporate_block_message);
		}
	}, [controls.corporate_block_message]);

	// Configured domains list
	const configuredCorporateDomains = useMemo(() => {
		const raw = controls.corporate_email_domains || "";
		return raw
			.split(/[,\s;\n\r]+/)
			.map((d) => d.trim().toLowerCase().replace(/^@+/, "").replace(/^\.+/, ""))
			.filter(Boolean);
	}, [controls.corporate_email_domains]);

	const configuredWorkspaceIds = useMemo(() => {
		return (controls.allowed_workspace_ids || "")
			.split(/[ ,\s;\n\r]+/)
			.map((id) => id.trim())
			.filter(Boolean);
	}, [controls.allowed_workspace_ids]);

	// Toggle Enforcement
	const handleToggleEnforcement = async (checked: boolean) => {
		const ok = await patchControl({ enforce_corporate_domains: checked });
		if (ok) {
			toast({
				title: checked ? "Corporate Enforcement Activated" : "Corporate Enforcement Deactivated",
				description: checked
					? "Corporate account checks are active for selected AI websites when the extension detects an account."
					: "Corporate domain restrictions are now paused.",
			});
		} else {
			toast({ title: "Could not update corporate domain enforcement", variant: "destructive" });
		}
	};

	// Add Domain
	const handleAddDomain = async (domainValue?: string) => {
		const raw = (domainValue || corporateDomainInput)
			.trim()
			.toLowerCase()
			.replace(/^@+/, "")
			.replace(/^\.+/, "")
			.replace(/\.$/, "");
		if (!raw) return;
		if (!/^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*$/.test(raw)) {
			toast({
				title: "Invalid email domain",
				description: "Enter a domain such as company.com, without a URL or email address.",
				variant: "destructive",
			});
			return;
		}
		if (configuredCorporateDomains.includes(raw)) {
			toast({
				title: "Domain already added",
				description: `@${raw} is already in the authorized list.`,
			});
			setCorporateDomainInput("");
			return;
		}
		const updated = [...configuredCorporateDomains, raw];
		const ok = await patchControl({ corporate_email_domains: updated.join(", ") });
		if (ok) {
			toast({
				title: "Corporate domain added",
				description: `@${raw} added to authorized domains.`,
			});
			setCorporateDomainInput("");
		} else {
			toast({
				title: "Failed to add domain",
				description: "Could not save corporate domain setting.",
				variant: "destructive",
			});
		}
	};

	const handleAddWorkspaceId = async () => {
		const id = workspaceIdInput.trim();
		if (!id) return;
		if (/[\s,;]/.test(id)) {
			toast({ title: "Enter one workspace ID", description: "Add one workspace or tenant ID at a time.", variant: "destructive" });
			return;
		}
		if (configuredWorkspaceIds.some((item) => item.toLowerCase() === id.toLowerCase())) {
			toast({ title: "Workspace ID already added" });
			setWorkspaceIdInput("");
			return;
		}
		const updated = [...configuredWorkspaceIds, id];
		const ok = await patchControl({ allowed_workspace_ids: updated.join(", ") });
		if (ok) {
			setWorkspaceIdInput("");
			toast({ title: "Workspace ID added", description: "The corporate policy now requires a matching workspace ID." });
		} else {
			toast({ title: "Failed to add workspace ID", description: "Could not save the workspace restriction.", variant: "destructive" });
		}
	};

	const handleRemoveWorkspaceId = async (id: string) => {
		const updated = configuredWorkspaceIds.filter((item) => item.toLowerCase() !== id.toLowerCase());
		const ok = await patchControl({ allowed_workspace_ids: updated.join(", ") });
		if (!ok) {
			toast({ title: "Failed to remove workspace ID", description: "Could not save the workspace restriction.", variant: "destructive" });
		}
	};

	// Remove Domain
	const handleRemoveDomain = async (domainToRemove: string) => {
		const updated = configuredCorporateDomains.filter((d) => d !== domainToRemove);
		const ok = await patchControl({ corporate_email_domains: updated.join(", ") });
		if (ok) {
			toast({
				title: "Corporate domain removed",
				description: `@${domainToRemove} removed from authorized list.`,
			});
		} else {
			toast({ title: "Failed to remove domain", description: "Could not save corporate domain setting.", variant: "destructive" });
		}
	};

	// Save Custom Block Message
	const handleSaveBlockMessage = async () => {
		const msg = corporateBlockMsgInput.trim();
		const ok = await patchControl({ corporate_block_message: msg });
		if (ok) {
			toast({
				title: "Block message saved",
				description: "Custom warning message updated successfully.",
			});
		} else {
			toast({ title: "Could not save block message", variant: "destructive" });
		}
	};

	const handleResetBlockMessage = async () => {
		const ok = await patchControl({ corporate_block_message: DEFAULT_BLOCK_MESSAGE });
		if (ok) {
			setCorporateBlockMsgInput(DEFAULT_BLOCK_MESSAGE);
			toast({ title: "Block message reset" });
		} else {
			toast({ title: "Could not reset block message", variant: "destructive" });
		}
	};

	const handleTargetWebsitesChange = async (value: string) => {
		if (!(await patchControl({ corporate_target_websites: value }))) {
			toast({ title: "Could not save target websites", variant: "destructive" });
		}
	};

	return (
		<Card className="border border-border bg-card overflow-hidden shadow-sm">
			<CardHeader className="p-4 pb-3">
				<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
					<div className="flex items-start gap-3.5 min-w-0">
						<div className="p-2.5 rounded-lg border border-border bg-muted/40 text-muted-foreground shrink-0 mt-0.5">
							<Building2 className="h-5 w-5" />
						</div>
						<div>
							<div className="flex items-center gap-2 flex-wrap">
								<CardTitle className="text-sm font-semibold">
									Corporate AI Workspace &amp; Domain Authorization
								</CardTitle>
								{controls.enforce_corporate_domains ? (
									<Badge
										variant="outline"
										className="text-[10px] px-2 py-0.5 border-emerald-500/50 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 font-medium"
									>
										Active
									</Badge>
								) : (
									<Badge
										variant="outline"
										className="text-[10px] px-2 py-0.5 border-border text-muted-foreground font-medium"
									>
										Disabled
									</Badge>
								)}
							</div>
							<CardDescription className="text-xs text-muted-foreground mt-0.5">
								Restrict access across ChatGPT, Claude, Gemini, Copilot, DeepSeek, Mistral, and monitored AI tools to corporate accounts. Personal email accounts (@gmail.com, @yahoo.com, etc.) are blocked from sending prompts.
							</CardDescription>
						</div>
					</div>
					<div className="flex items-center gap-3 shrink-0 self-start sm:self-center">
						<div className="text-right hidden sm:block">
							<p className="text-xs font-medium text-foreground">Enforce Policy</p>
							<p className="text-[11px] text-muted-foreground">Block non-corporate accounts</p>
						</div>
						<Switch
							checked={!!controls.enforce_corporate_domains}
							onCheckedChange={handleToggleEnforcement}
							aria-label="Toggle corporate domain enforcement"
						/>
					</div>
				</div>
			</CardHeader>

			<CardContent className="p-4 pt-1 space-y-4 border-t border-border/60">
				{/* 1. Allowed Corporate Email Domains Section */}
				<div className="space-y-2 pt-2">
					<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1">
						<Label className="text-xs font-semibold flex items-center gap-1.5 text-foreground">
							<Globe className="h-3.5 w-3.5 text-muted-foreground" />
							Allowed Corporate Email Domains (@company.com)
						</Label>
						<span className="text-[11px] text-muted-foreground">
							Only users logged in with these email domains will be allowed to send prompts
						</span>
					</div>

					{/* Configured Domains Chips */}
					<div className="min-h-[38px] p-2 rounded-md border border-border bg-muted/20 flex flex-wrap items-center gap-1.5">
						{configuredCorporateDomains.length === 0 ? (
							<span className="text-xs text-muted-foreground italic px-1">
								No corporate domains configured yet. Add your company email domain below (e.g. company.com).
							</span>
						) : (
							configuredCorporateDomains.map((dom) => (
								<Badge
									key={dom}
									variant="secondary"
									className="pl-2.5 pr-1.5 py-0.5 text-xs font-mono bg-muted border border-border text-foreground flex items-center gap-1.5 font-normal"
								>
									<span>@{dom}</span>
									<button
										type="button"
										onClick={() => handleRemoveDomain(dom)}
										className="h-3.5 w-3.5 rounded-full inline-flex items-center justify-center hover:bg-muted-foreground/20 text-muted-foreground hover:text-foreground transition-colors"
										title={`Remove @${dom}`}
									>
										<X className="h-3 w-3" />
									</button>
								</Badge>
							))
						)}
					</div>

					{/* Add Domain Input Form */}
					<div className="flex flex-col sm:flex-row gap-2">
						<div className="relative flex-1">
							<span className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground font-mono text-xs">
								@
							</span>
							<Input
								value={corporateDomainInput}
								onChange={(e) => setCorporateDomainInput(e.target.value)}
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										e.preventDefault();
										void handleAddDomain();
									}
								}}
								placeholder="company.com or subsidiary.org (press Enter to add)"
								className="pl-6 font-mono text-xs h-8"
							/>
						</div>
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={() => handleAddDomain()}
							disabled={!corporateDomainInput.trim()}
							className="h-8 text-xs gap-1.5 shrink-0"
						>
							<Plus className="h-3.5 w-3.5" />
							Add Domain
						</Button>
					</div>
				</div>

				{/* 2. Optional Approved Workspace IDs */}
				<div className="space-y-2 pt-3 border-t border-border/40">
					<div className="space-y-0.5">
						<Label className="text-xs font-semibold text-foreground">Optional Approved Workspace / Tenant IDs</Label>
						<p className="text-[11px] text-muted-foreground">When configured, the session must provide one of these IDs as well as an allowed email domain. Sessions without a detected ID are blocked.</p>
					</div>
					<div className="min-h-[34px] p-2 rounded-md border border-border bg-muted/20 flex flex-wrap items-center gap-1.5">
						{configuredWorkspaceIds.length === 0 ? (
							<span className="text-xs text-muted-foreground italic px-1">No workspace restriction configured.</span>
						) : configuredWorkspaceIds.map((id) => (
							<Badge key={id.toLowerCase()} variant="secondary" className="pl-2.5 pr-1.5 py-0.5 text-xs font-mono bg-muted border border-border text-foreground flex items-center gap-1.5 font-normal">
								<span>{id}</span>
								<button type="button" onClick={() => void handleRemoveWorkspaceId(id)} className="h-3.5 w-3.5 rounded-full inline-flex items-center justify-center hover:bg-muted-foreground/20 text-muted-foreground hover:text-foreground" title={`Remove ${id}`}>
									<X className="h-3 w-3" />
								</button>
							</Badge>
						))}
					</div>
					<div className="flex flex-col sm:flex-row gap-2">
						<Input
							value={workspaceIdInput}
							onChange={(e) => setWorkspaceIdInput(e.target.value)}
							onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); void handleAddWorkspaceId(); } }}
							placeholder="Workspace or tenant ID (press Enter to add)"
							className="font-mono text-xs h-8"
						/>
						<Button type="button" variant="outline" size="sm" onClick={() => void handleAddWorkspaceId()} disabled={!workspaceIdInput.trim()} className="h-8 text-xs gap-1.5 shrink-0">
							<Plus className="h-3.5 w-3.5" /> Add Workspace ID
						</Button>
					</div>
				</div>

				{/* 3. Custom Block Warning Message Section */}
				<div className="space-y-2 pt-3 border-t border-border/40">
					<div className="flex items-center justify-between">
						<Label className="text-xs font-semibold text-foreground">Custom Block Warning Message</Label>
						<Button
							type="button"
							variant="ghost"
							size="sm"
							className="h-6 text-xs text-muted-foreground hover:text-foreground px-2"
							onClick={() => void handleResetBlockMessage()}
						>
							Reset to Default
						</Button>
					</div>
					<div className="flex flex-col sm:flex-row gap-2">
						<Input
							value={corporateBlockMsgInput}
							onChange={(e) => setCorporateBlockMsgInput(e.target.value)}
							placeholder={DEFAULT_BLOCK_MESSAGE}
							className="text-xs h-8"
						/>
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={handleSaveBlockMessage}
							className="h-8 text-xs gap-1.5 shrink-0"
						>
							<Save className="h-3.5 w-3.5" />
							Save Message
						</Button>
					</div>
				</div>

				{/* 4. Target Websites Selector */}
				<PolicyTargetWebsitesSelector
					value={controls.corporate_target_websites || "all"}
					targets={targets}
					onChange={(val) => void handleTargetWebsitesChange(val)}
				/>
			</CardContent>
		</Card>
	);
}
