import React, { useState, useRef } from "react";
import * as XLSX from "xlsx";
import {
	Upload,
	FileSpreadsheet,
	CheckCircle2,
	AlertCircle,
	Trash2,
	Globe,
	Loader2,
	HelpCircle,
	ShieldAlert,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import { useImportBrowserAiTargetsMutation, type BrowserTargetWebsite } from "@/lib/store/apis/browserAiApi";
import { normalizeTargetDomain, HOST_ROLE_OPTIONS, type HostRole } from "./relatedHosts";

export interface ParsedImportTarget {
	domain: string;
	platform_name: string;
	host_role: HostRole;
	monitored: boolean;
	block_site: boolean;
	valid: boolean;
	errors: string[];
}

interface TargetImportDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onImportSuccess: () => void;
}

const DUPLICATE_IN_FILE = "Duplicate domain in file — will be skipped";

/** Canonical Excel / CSV columns for Target Websites import */
export const TARGETS_IMPORT_HEADERS = [
	"Domain",
	"Platform Name",
	"Host Role",
	"Monitored",
	"Block Entire Website",
] as const;

export function downloadTargetsTemplate() {
	const headers = [...TARGETS_IMPORT_HEADERS];
	const sampleRows = [
		["chatgpt.com", "ChatGPT", "ui", "TRUE", "FALSE"],
		["ab.chatgpt.com", "ChatGPT", "chat", "TRUE", "FALSE"],
		["files.oaiusercontent.com", "ChatGPT", "file", "TRUE", "FALSE"],
		["claude.ai", "Claude", "ui", "TRUE", "FALSE"],
		["api.anthropic.com", "Claude", "chat", "TRUE", "FALSE"],
		["files.claudeusercontent.com", "Claude", "file", "TRUE", "FALSE"],
		["gemini.google.com", "Gemini", "ui", "TRUE", "FALSE"],
		["clients6.google.com", "Gemini", "chat", "TRUE", "FALSE"],
		["generativelanguage.googleapis.com", "Gemini", "chat", "TRUE", "FALSE"],
	];

	const ws = XLSX.utils.aoa_to_sheet([headers, ...sampleRows]);

	ws["!cols"] = [
		{ wch: 30 }, // Domain
		{ wch: 22 }, // Platform Name
		{ wch: 18 }, // Host Role
		{ wch: 14 }, // Monitored
		{ wch: 24 }, // Block Entire Website
	];

	const wb = XLSX.utils.book_new();
	XLSX.utils.book_append_sheet(wb, ws, "Target Websites");
	XLSX.writeFile(wb, "target_websites_template.xlsx");
}

export function TargetImportDialog({
	open,
	onOpenChange,
	onImportSuccess,
}: TargetImportDialogProps) {
	const { toast } = useToast();
	const fileInputRef = useRef<HTMLInputElement>(null);
	const [importTargetsMutation, { isLoading: isSubmitting }] = useImportBrowserAiTargetsMutation();

	const [fileName, setFileName] = useState<string>("");
	const [parsedTargets, setParsedTargets] = useState<ParsedImportTarget[]>([]);
	const [overwriteExisting, setOverwriteExisting] = useState<boolean>(false);
	const [isDragging, setIsDragging] = useState<boolean>(false);

	const resetState = () => {
		setFileName("");
		setParsedTargets([]);
		setOverwriteExisting(false);
		if (fileInputRef.current) fileInputRef.current.value = "";
	};

	const normalizeBooleanValue = (val: any, defaultVal = true): boolean => {
		if (typeof val === "boolean") return val;
		const s = String(val || "").trim().toLowerCase();
		if (s === "") return defaultVal;
		return s === "true" || s === "yes" || s === "1" || s === "monitored" || s === "active" || s === "blocked";
	};

	const normalizeHostRoleValue = (val: any): HostRole => {
		const s = String(val || "").trim().toLowerCase();
		if (s === "ui" || s.includes("main") || s.includes("website")) return "ui";
		if (s === "chat" || s.includes("prompt")) return "chat";
		if (s === "file" || s.includes("upload") || s.includes("attachment")) return "file";
		return "";
	};

	const parseWorkbook = (wb: XLSX.WorkBook, originalFileName: string) => {
		try {
			const sheetName = wb.SheetNames[0];
			const ws = wb.Sheets[sheetName];
			const rawRows: Record<string, any>[] = XLSX.utils.sheet_to_json(ws, { defval: "" });

			if (!rawRows || rawRows.length === 0) {
				toast({
					title: "Empty Spreadsheet",
					description: "The uploaded file does not contain any target website rows.",
					variant: "destructive",
				});
				return;
			}

			const targets: ParsedImportTarget[] = rawRows.map((row) => {
				const normalizeKey = (k: string) =>
					k.replace(/^\uFEFF/, "").toLowerCase().replace(/\s+/g, " ").trim();

				const getVal = (possibleHeaders: string[]): string => {
					for (const h of possibleHeaders) {
						const nh = normalizeKey(h);
						for (const key of Object.keys(row)) {
							if (normalizeKey(key) === nh) return String(row[key] ?? "").trim();
						}
					}
					return "";
				};

				const rawDomain = getVal(["domain", "domain name", "target domain", "website", "host", "url"]);
				const rawPlatform = getVal(["platform name", "platform", "platform_name", "name", "service"]);
				const rawRole = getVal(["host role", "host_role", "role", "type"]);
				const rawMonitored = getVal(["monitored", "active", "enabled", "status"]);
				const rawBlockSite = getVal(["block entire website", "block website", "block_site", "blocked", "block"]);

				const domain = normalizeTargetDomain(rawDomain);
				const hostRole = normalizeHostRoleValue(rawRole);
				const monitored = normalizeBooleanValue(rawMonitored, true);
				const blockSite = normalizeBooleanValue(rawBlockSite, false);
				const platformName = rawPlatform || domain;

				const errors: string[] = [];
				if (!rawDomain) {
					errors.push("Missing domain name");
				} else if (!domain) {
					errors.push("Invalid domain format");
				} else if (domain.includes(" ") || !domain.includes(".")) {
					errors.push("Domain must include a valid TLD (e.g. domain.com)");
				}

				return {
					domain,
					platform_name: platformName,
					host_role: hostRole,
					monitored,
					block_site: blockSite,
					valid: errors.length === 0,
					errors,
				};
			});

			// Check for in-file duplicate domains
			const seenDomains = new Set<string>();
			for (const t of targets) {
				const key = t.domain.toLowerCase();
				if (key) {
					if (seenDomains.has(key)) {
						t.errors.push(DUPLICATE_IN_FILE);
						t.valid = false;
					}
					seenDomains.add(key);
				}
			}

			setFileName(originalFileName);
			setParsedTargets(targets);
		} catch (err: any) {
			toast({
				title: "Parsing Error",
				description: err?.message || "Failed to parse spreadsheet file.",
				variant: "destructive",
			});
		}
	};

	const handleFileSelect = (file: File) => {
		if (!file) return;
		const reader = new FileReader();
		reader.onload = (e) => {
			const data = e.target?.result;
			if (!data || !(data instanceof ArrayBuffer)) return;
			try {
				const wb = XLSX.read(new Uint8Array(data), { type: "array" });
				parseWorkbook(wb, file.name);
			} catch {
				toast({
					title: "Failed to Read File",
					description: "Please upload a valid .xlsx, .xls, or .csv file.",
					variant: "destructive",
				});
			}
		};
		reader.readAsArrayBuffer(file);
	};

	const handleDrop = (e: React.DragEvent) => {
		e.preventDefault();
		setIsDragging(false);
		if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
			handleFileSelect(e.dataTransfer.files[0]);
		}
	};

	const handleImport = async () => {
		const validList = parsedTargets.filter((t) => t.valid);
		if (validList.length === 0) {
			toast({
				title: "No Valid Targets",
				description: "None of the rows passed validation. Please correct the errors in the spreadsheet.",
				variant: "destructive",
			});
			return;
		}

		try {
			const res = await importTargetsMutation({
				targets: validList.map((t) => ({
					domain: t.domain,
					platform_name: t.platform_name,
					host_role: t.host_role,
					monitored: t.monitored,
					block_site: t.block_site,
					status: t.block_site ? "BLOCKED" : !t.monitored ? "PAUSED" : "MONITORED",
				})),
				overwrite: overwriteExisting,
			}).unwrap();

			const imp = res.imported || 0;
			const upd = res.updated || 0;
			const skp = res.skipped || 0;
			const errs = res.errors || [];
			const alreadyExist = res.already_exists ?? Math.max(0, skp - errs.length);

			const nested = res.nested || 0;
			const fileDupes = (res.duplicates_in_file || 0) + parsedTargets.filter((t) => t.errors.includes(DUPLICATE_IN_FILE)).length;

			let msg = `Imported ${imp} target(s)${upd ? `, updated ${upd}` : ""}${skp ? `, skipped ${skp}` : ""}.`;
			if (alreadyExist > 0 && !overwriteExisting) {
				msg += ` ${alreadyExist} already exist (skipped) — tick "Overwrite existing targets" to update them.`;
			}
			if (fileDupes > 0) {
				msg += ` ${fileDupes} duplicate row(s) in the file skipped.`;
			}
			if (nested > 0) {
				msg += ` ${nested} existing subdomain(s) moved under their main domain.`;
			}
			if (errs.length > 0) {
				msg += ` ${errs.slice(0, 3).join(" · ")}${errs.length > 3 ? ` (+${errs.length - 3} more)` : ""}`;
			}

			if (imp + upd + nested === 0) {
				toast({
					title: "Nothing Imported",
					description: msg,
					variant: "destructive",
				});
				return;
			}

			toast({
				title: "Targets Imported",
				description: msg,
			});
			onImportSuccess();
			onOpenChange(false);
			resetState();
		} catch (err: any) {
			const data = err?.data;
			const errMsg =
				(typeof data?.error === "object" && data?.error?.message) ||
				(typeof data?.error === "string" ? data.error : "") ||
				data?.message ||
				err?.message ||
				"Failed to submit imported targets.";
			toast({
				title: "Import Error",
				description: errMsg,
				variant: "destructive",
			});
		}
	};

	const validCount = parsedTargets.filter((t) => t.valid).length;
	const invalidCount = parsedTargets.filter((t) => !t.valid).length;

	return (
		<Dialog
			open={open}
			onOpenChange={(v) => {
				onOpenChange(v);
				if (!v) resetState();
			}}
		>
			<DialogContent className="bg-card border-border text-foreground w-[calc(100%-2rem)] sm:max-w-3xl max-h-[90vh] flex flex-col p-0 overflow-hidden">
				<DialogHeader className="p-5 pb-3 pr-12 shrink-0 border-b border-border/60">
					<DialogTitle className="flex min-w-0 items-center gap-2 text-base font-semibold">
						<Globe className="h-5 w-5 shrink-0 text-emerald-500" />
						Import Target Websites from Excel / CSV
					</DialogTitle>
					<DialogDescription className="text-xs text-muted-foreground mt-1">
						Bulk add AI platform domains, host roles, and website blocking rules via Excel (.xlsx / .xls) or CSV.
					</DialogDescription>
				</DialogHeader>

				<div className="flex-1 overflow-y-auto p-5 space-y-4 no-scrollbar">
					{parsedTargets.length === 0 ? (
						<div
							onDragOver={(e) => {
								e.preventDefault();
								setIsDragging(true);
							}}
							onDragLeave={() => setIsDragging(false)}
							onDrop={handleDrop}
							onClick={() => fileInputRef.current?.click()}
							className={`border-2 border-dashed rounded-xl p-8 text-center cursor-pointer transition-colors flex flex-col items-center justify-center gap-3 ${
								isDragging
									? "border-emerald-500 bg-emerald-500/10"
									: "border-border hover:border-emerald-500/60 hover:bg-muted/30"
							}`}
						>
							<input
								ref={fileInputRef}
								type="file"
								accept=".xlsx,.xls,.csv"
								className="hidden"
								onChange={(e) => {
									if (e.target.files && e.target.files.length > 0) {
										handleFileSelect(e.target.files[0]);
									}
								}}
							/>
							<div className="h-12 w-12 rounded-full bg-emerald-500/10 flex items-center justify-center text-emerald-400">
								<Upload className="h-6 w-6" />
							</div>
							<div>
								<p className="text-sm font-medium text-foreground">Click to browse or drag &amp; drop spreadsheet</p>
								<p className="text-xs text-muted-foreground mt-1">Supports Excel (.xlsx, .xls) or Comma-Separated Values (.csv)</p>
							</div>
							<div className="flex items-center gap-2 text-[11px] text-muted-foreground bg-muted/50 px-3 py-1.5 rounded-full border border-border/60">
								<HelpCircle className="h-3 w-3 text-emerald-400" />
								<span>
									Required: <b>Domain</b> · Optional: Platform Name, Host Role (ui/chat/file), Monitored, Block Entire Website
								</span>
							</div>
						</div>
					) : (
						<div className="space-y-4">
							<div className="flex flex-wrap items-center justify-between gap-3 p-3 bg-muted/40 rounded-lg border border-border">
								<div className="flex items-center gap-2 min-w-0">
									<Globe className="h-5 w-5 text-emerald-400 shrink-0" />
									<div className="min-w-0">
										<p className="text-xs font-semibold text-foreground truncate">{fileName}</p>
										<div className="flex items-center gap-2 mt-0.5 text-[11px]">
											<span className="text-emerald-400 font-medium">{validCount} valid</span>
											{invalidCount > 0 && <span className="text-rose-400 font-medium">· {invalidCount} invalid</span>}
										</div>
									</div>
								</div>
								<div className="flex items-center gap-2">
									<Button
										type="button"
										variant="ghost"
										size="sm"
										onClick={resetState}
										className="h-8 gap-1 text-xs text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
									>
										<Trash2 className="h-3.5 w-3.5" />
										Choose Another
									</Button>
								</div>
							</div>

							<div className="rounded-lg border border-border overflow-hidden">
								<div className="max-h-[320px] overflow-y-auto no-scrollbar">
									<table className="w-full text-left text-xs border-collapse">
										<thead className="bg-muted/70 text-muted-foreground sticky top-0 border-b border-border z-10">
											<tr>
												<th className="py-2.5 px-3 font-medium">Status</th>
												<th className="py-2.5 px-3 font-medium">Domain</th>
												<th className="py-2.5 px-3 font-medium">Platform Name</th>
												<th className="py-2.5 px-3 font-medium">Host Role</th>
												<th className="py-2.5 px-3 font-medium">Monitoring</th>
												<th className="py-2.5 px-3 font-medium">Website Lock</th>
											</tr>
										</thead>
										<tbody className="divide-y divide-border/60">
											{parsedTargets.map((row, idx) => (
												<tr key={idx} className={row.valid ? "hover:bg-muted/20" : "bg-rose-500/5 hover:bg-rose-500/10"}>
													<td className="py-2 px-3 align-top whitespace-nowrap">
														{row.valid ? (
															<Badge
																variant="outline"
																className="border-emerald-500/30 text-emerald-400 bg-emerald-500/10 text-[10px] gap-1 px-1.5 py-0.5"
															>
																<CheckCircle2 className="h-2.5 w-2.5" />
																Valid
															</Badge>
														) : (
															<div className="space-y-1">
																<Badge variant="destructive" className="text-[10px] gap-1 px-1.5 py-0.5">
																	<AlertCircle className="h-2.5 w-2.5" />
																	Invalid
																</Badge>
																<p className="text-[10px] text-rose-400 break-words max-w-[120px]">
																	{row.errors.join(", ")}
																</p>
															</div>
														)}
													</td>
													<td className="py-2 px-3 align-top font-mono text-[11px] font-semibold text-foreground max-w-[200px] truncate">
														{row.domain || <span className="text-muted-foreground italic">&lt;empty&gt;</span>}
													</td>
													<td className="py-2 px-3 align-top font-medium text-foreground max-w-[150px] truncate">
														{row.platform_name || "—"}
													</td>
													<td className="py-2 px-3 align-top whitespace-nowrap">
														<span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-medium bg-muted/60 text-muted-foreground border border-border">
															{row.host_role === "ui"
																? "UI"
																: row.host_role === "chat"
																	? "Chat"
																	: row.host_role === "file"
																		? "File"
																		: "Auto"}
														</span>
													</td>
													<td className="py-2 px-3 align-top whitespace-nowrap">
														<Badge
															variant="outline"
															className={`text-[10px] px-1.5 py-0.5 ${
																row.monitored
																	? "border-emerald-500/30 text-emerald-400 bg-emerald-500/10"
																	: "border-border text-muted-foreground"
															}`}
														>
															{row.monitored ? "Monitored" : "Paused"}
														</Badge>
													</td>
													<td className="py-2 px-3 align-top whitespace-nowrap">
														{row.block_site ? (
															<Badge className="bg-rose-950/80 text-rose-300 border-rose-800 text-[10px] gap-1 px-1.5 py-0.5">
																<ShieldAlert className="h-2.5 w-2.5" /> Blocked
															</Badge>
														) : (
															<span className="text-[10px] text-muted-foreground">Allowed</span>
														)}
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</div>

							<div className="flex items-center space-x-2 pt-1">
								<Checkbox
									id="overwriteExistingTargets"
									checked={overwriteExisting}
									onCheckedChange={(checked) => setOverwriteExisting(!!checked)}
								/>
								<label
									htmlFor="overwriteExistingTargets"
									className="text-xs text-foreground cursor-pointer select-none font-medium"
								>
									Overwrite existing target websites if domain already exists
								</label>
							</div>
						</div>
					)}
				</div>

				<DialogFooter className="p-4 shrink-0 border-t border-border/60 flex items-center justify-between sm:justify-between bg-muted/20">
					<Button
						type="button"
						variant="ghost"
						onClick={() => {
							onOpenChange(false);
							resetState();
						}}
						disabled={isSubmitting}
						className="text-xs"
					>
						Cancel
					</Button>
					{parsedTargets.length > 0 && (
						<Button
							type="button"
							onClick={handleImport}
							disabled={isSubmitting || validCount === 0}
							className="gap-2 bg-emerald-600 hover:bg-emerald-500 text-white text-xs"
						>
							{isSubmitting ? (
								<>
									<Loader2 className="h-3.5 w-3.5 animate-spin" />
									Importing...
								</>
							) : (
								<>
									<CheckCircle2 className="h-3.5 w-3.5" />
									Confirm &amp; Import {validCount} Target{validCount === 1 ? "" : "s"}
								</>
							)}
						</Button>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
