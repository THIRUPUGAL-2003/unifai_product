import React, { useState, useRef } from "react";
import * as XLSX from "xlsx";
import {
	Upload,
	FileSpreadsheet,
	CheckCircle2,
	AlertCircle,
	Trash2,
	RefreshCw,
	HelpCircle,
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
import { useImportBrowserAiRulesMutation } from "@/lib/store/apis/browserAiApi";

export interface ParsedImportRule {
	name: string;
	rule_type: string;
	pattern: string;
	severity: "CRITICAL" | "HIGH" | "MEDIUM";
	action: "BLOCK" | "REDACT" | "WARN";
	warning_message: string;
	description: string;
	active: boolean;
	valid: boolean;
	errors: string[];
}

interface GuardRuleImportDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onImportSuccess: () => void;
	getApiBaseUrl?: () => string;
}

/** Canonical Excel / CSV columns for Guard Rules import (also used by Export on Rules tab). */
export const GUARD_RULES_IMPORT_HEADERS = [
	"Rule Name",
	"Rule Type",
	"Pattern",
	"Severity",
	"Action",
	"Warning Message",
	"Active",
	"Description",
] as const;

export function downloadGuardRulesTemplate() {
	const headers = [...GUARD_RULES_IMPORT_HEADERS];
	const sampleRows = [
		[
			"Block Credit Card Numbers",
			"regex",
			"\\b(?:4[0-9]{12}(?:[0-9]{3})?)\\b",
			"CRITICAL",
			"BLOCK",
			"Credit card numbers are prohibited by corporate policy.",
			"TRUE",
			"PCI — block card PANs in AI prompts",
		],
		[
			"Redact Salary & Compensation",
			"regex",
			"\\b(?:salary|payroll|bonus|compensation|ctc)\\b",
			"MEDIUM",
			"REDACT",
			"Employee compensation details redacted.",
			"TRUE",
			"HR confidentiality",
		],
		[
			"Warn on Secret Keys",
			"regex",
			"\\b(?:api[_-]?key|secret[_-]?token|bearer[\\s]+[a-zA-Z0-9_\\-]{16,})\\b",
			"HIGH",
			"WARN",
			"Potential API key detected. Please verify before sending.",
			"TRUE",
			"Secrets hygiene",
		],
		[
			"Block Aadhaar Numbers",
			"regex",
			"\\b[2-9]{1}[0-9]{3}\\s?[0-9]{4}\\s?[0-9]{4}\\b",
			"CRITICAL",
			"BLOCK",
			"Aadhaar numbers are not allowed in AI chats.",
			"TRUE",
			"India PII",
		],
	];

	const ws = XLSX.utils.aoa_to_sheet([headers, ...sampleRows]);

	ws["!cols"] = [
		{ wch: 30 }, // Rule Name
		{ wch: 12 }, // Rule Type
		{ wch: 50 }, // Pattern
		{ wch: 12 }, // Severity
		{ wch: 10 }, // Action
		{ wch: 55 }, // Warning Message
		{ wch: 10 }, // Active
		{ wch: 28 }, // Description
	];

	const wb = XLSX.utils.book_new();
	XLSX.utils.book_append_sheet(wb, ws, "Guard Rules");
	XLSX.writeFile(wb, "guard_rules_template.xlsx");
}

export function normalizePatternToRegex(rawPattern: string): string {
	const trimmed = (rawPattern || "").trim();
	if (!trimmed) return "";
	// Already an advanced regex starting with boundary, anchor, or group
	if (/^(\\b|\^|\(\?)/.test(trimmed)) {
		return trimmed;
	}
	// Real regexes ({3,5}, classes, token alternation) must not be split on , or |.
	// $ ^ . are excluded: they appear in plain keywords (e.g. "$secret", "api.key").
	if (/[\\[\]{}()*+?]/.test(trimmed)) {
		try {
			new RegExp(trimmed);
			return trimmed;
		} catch {
			// Not a valid regex (e.g. "c++, c#") — treat as keyword list below.
		}
	}
	// If it contains commas, semicolons, newlines, or pipes, split into clean keyword alternations
	if (/[,;\n\r|]/.test(trimmed)) {
		const tokens = trimmed
			.split(/[,;\n\r|]+/)
			.map((t) => t.trim())
			.filter(Boolean);
		if (tokens.length > 1) {
			const allWordBounded = tokens.every((t) => /^\w/.test(t) && /\w$/.test(t));
			if (allWordBounded) {
				const escaped = tokens.map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
				return `\\b(?:${escaped.join("|")})\\b`;
			}
			const bounded = tokens.map((t) => {
				const prefix = /^\w/.test(t) ? "\\b" : "";
				const suffix = /\w$/.test(t) ? "\\b" : "";
				return `${prefix}${t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}${suffix}`;
			});
			return `(?:${bounded.join("|")})`;
		} else if (tokens.length === 1) {
			const t = tokens[0];
			const prefix = /^\w/.test(t) ? "\\b" : "";
			const suffix = /\w$/.test(t) ? "\\b" : "";
			return `${prefix}${t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}${suffix}`;
		}
	}
	// Plain single word or phrase without regex boundary
	if (/^[a-zA-Z0-9_\-\s]+$/.test(trimmed)) {
		return `\\b${trimmed.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`;
	}
	return trimmed;
}

export function GuardRuleImportDialog({
	open,
	onOpenChange,
	onImportSuccess,
	getApiBaseUrl: _getApiBaseUrl,
}: GuardRuleImportDialogProps) {
	const { toast } = useToast();
	const fileInputRef = useRef<HTMLInputElement>(null);
	const [importRulesMutation, { isLoading: isSubmitting }] = useImportBrowserAiRulesMutation();

	const [fileName, setFileName] = useState<string>("");
	const [parsedRules, setParsedRules] = useState<ParsedImportRule[]>([]);
	const [overwriteExisting, setOverwriteExisting] = useState<boolean>(false);
	const [isDragging, setIsDragging] = useState<boolean>(false);

	const resetState = () => {
		setFileName("");
		setParsedRules([]);
		setOverwriteExisting(false);
		if (fileInputRef.current) fileInputRef.current.value = "";
	};

	const normalizeAction = (val: any): "BLOCK" | "REDACT" | "WARN" => {
		const s = String(val || "").trim().toUpperCase();
		if (s === "REDACT") return "REDACT";
		if (s === "WARN" || s === "ALERT") return "WARN";
		return "BLOCK";
	};

	const normalizeSeverity = (val: any): "CRITICAL" | "HIGH" | "MEDIUM" => {
		const s = String(val || "").trim().toUpperCase();
		if (s === "CRITICAL") return "CRITICAL";
		if (s === "MEDIUM" || s === "LOW") return "MEDIUM";
		return "HIGH";
	};

	const normalizeActive = (val: any): boolean => {
		if (typeof val === "boolean") return val;
		const s = String(val || "").trim().toLowerCase();
		return s === "true" || s === "yes" || s === "1" || s === "active" || s === "";
	};

	const processWorkbook = (wb: XLSX.WorkBook, name: string) => {
		try {
			const sheetName = wb.SheetNames[0];
			const ws = wb.Sheets[sheetName];
			const jsonData = XLSX.utils.sheet_to_json<Record<string, any>>(ws, { defval: "" });

			if (!jsonData || jsonData.length === 0) {
				toast({
					title: "Empty Spreadsheet",
					description: "The uploaded file does not contain any rule rows.",
					variant: "destructive",
				});
				return;
			}

			const results: ParsedImportRule[] = jsonData.map((row) => {
				// Flexible header lookup (template + Export + common aliases).
				// Strip UTF-8 BOM — Excel/CSV often prefixes the first column as "\ufeffRule Name".
				const normalizeHeader = (h: string) =>
					h.replace(/^\uFEFF/, "").toLowerCase().replace(/\s+/g, " ").trim();

				const getField = (keys: string[]): string => {
					for (const k of keys) {
						const want = normalizeHeader(k);
						for (const rowKey of Object.keys(row)) {
							const have = normalizeHeader(rowKey);
							if (have === want) {
								return String(row[rowKey] ?? "").trim();
							}
						}
					}
					return "";
				};

				const ruleName = getField(["rule name", "name", "title", "rulename"]);
				const patternRaw = getField([
					"pattern",
					"regex",
					"keyword",
					"expression",
					"pattern / policy",
					"pattern/policy",
				]);
				const pattern = normalizePatternToRegex(patternRaw);
				const actionRaw = getField(["action", "decision"]);
				const severityRaw = getField(["severity", "level"]);
				const warningMessage = getField([
					"warning message",
					"warning_message",
					"message",
					"warning",
					"notice",
				]);
				const description = getField(["description", "desc", "notes", "note"]);
				const ruleTypeRaw = getField(["rule type", "rule_type", "type", "ruletype"]).toLowerCase();
				const activeRaw = getField(["active", "status", "enabled"]);

				let ruleType = "regex";
				if (
					ruleTypeRaw === "ai_bot" ||
					ruleTypeRaw === "ai guard bot" ||
					ruleTypeRaw === "aibot" ||
					ruleTypeRaw.includes("bot")
				) {
					ruleType = "ai_bot";
				} else if (ruleTypeRaw === "" || ruleTypeRaw === "regex" || ruleTypeRaw === "dlp") {
					ruleType = "regex";
				}

				const action = normalizeAction(actionRaw);
				const severity = normalizeSeverity(severityRaw);
				const active = normalizeActive(activeRaw);

				const errors: string[] = [];
				if (!ruleName) {
					errors.push("Missing rule name");
				}

				if (ruleType === "ai_bot") {
					errors.push("AI Guard Bot rules must be created in the UI (Excel import supports regex only)");
				} else if (!pattern) {
					errors.push("Missing regex pattern");
				} else if (/\(\?<?[=!]|\\[1-9]/.test(pattern)) {
					errors.push("Lookahead/lookbehind/backreferences are not supported by the gateway regex engine");
				} else {
					try {
						new RegExp(pattern);
					} catch (e: any) {
						errors.push(`Invalid regex: ${e.message}`);
					}
				}

				return {
					name: ruleName,
					rule_type: ruleType,
					pattern,
					severity,
					action,
					warning_message: warningMessage || description,
					description,
					active,
					valid: errors.length === 0,
					errors,
				};
			});

			const seenNames = new Set<string>();
			const seenPatterns = new Set<string>();
			for (const r of results) {
				const key = r.name.trim().toLowerCase();
				const pat = (r.pattern || "").trim();
				if (!key) continue;
				if (seenNames.has(key)) {
					r.errors.push("Duplicate rule name in file — will be skipped");
					r.valid = false;
				} else if (pat && seenPatterns.has(pat)) {
					r.errors.push("Same pattern as an earlier row — will be skipped");
					r.valid = false;
				}
				seenNames.add(key);
				if (pat) seenPatterns.add(pat);
			}

			setFileName(name);
			setParsedRules(results);
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
				processWorkbook(wb, file.name);
			} catch (err: any) {
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

	const handleConfirmImport = async () => {
		const validRules = parsedRules.filter((r) => r.valid);
		if (validRules.length === 0) {
			toast({
				title: "No Valid Rules",
				description: "None of the rows passed validation. Please correct the errors in the spreadsheet.",
				variant: "destructive",
			});
			return;
		}

		try {
			const data = await importRulesMutation({
				rules: validRules.map((r) => ({
					name: r.name,
					rule_type: "regex",
					pattern: r.pattern,
					severity: r.severity,
					action: r.action,
					warning_message: r.warning_message,
					description: r.description || r.warning_message,
					active: r.active,
				})),
				overwrite: overwriteExisting,
			}).unwrap();

			const imported = data.imported || 0;
			const updated = data.updated || 0;
			const skipped = data.skipped || 0;
			const serverErrors = data.errors || [];
			const skippedExisting = data.already_exists ?? Math.max(0, skipped - serverErrors.length);
			const fileDupes =
				(data.duplicates_in_file || 0) + parsedRules.filter((r) => r.errors.some((e) => e.endsWith("will be skipped"))).length;
			let msg = `Imported ${imported} rule(s)${updated ? `, updated ${updated}` : ""}${skipped ? `, skipped ${skipped}` : ""}.`;
			if (skippedExisting > 0) {
				msg += ` ${skippedExisting} already exist (same name or pattern, skipped)${overwriteExisting ? "" : ` — tick "Overwrite existing rules" to update same-name rules`}.`;
			}
			if (fileDupes > 0) {
				msg += ` ${fileDupes} duplicate row(s) in the file skipped.`;
			}
			if (serverErrors.length > 0) {
				msg += ` ${serverErrors.slice(0, 3).join(" · ")}${serverErrors.length > 3 ? ` (+${serverErrors.length - 3} more)` : ""}`;
			}

			if (imported + updated === 0) {
				toast({ title: "Nothing Imported", description: msg, variant: "destructive" });
				return;
			}
			toast({ title: "Rules Imported", description: msg });
			onImportSuccess();
			onOpenChange(false);
			resetState();
		} catch (err: unknown) {
			const anyErr = err as { data?: { error?: { message?: string } | string; message?: string }; message?: string };
			const raw = anyErr?.data?.error;
			const msg =
				(typeof raw === "object" && raw?.message) ||
				(typeof raw === "string" ? raw : "") ||
				anyErr?.data?.message ||
				anyErr?.message ||
				"Failed to submit imported rules.";
			toast({
				title: "Import Error",
				description: msg,
				variant: "destructive",
			});
		}
	};

	const validCount = parsedRules.filter((r) => r.valid).length;
	const invalidCount = parsedRules.filter((r) => !r.valid).length;

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
						<FileSpreadsheet className="h-5 w-5 shrink-0 text-emerald-500" />
						Import DLP Guard Rules from Excel / CSV
					</DialogTitle>
					<DialogDescription className="text-xs text-muted-foreground mt-1">
						Upload Excel (.xlsx / .xls) or CSV. Import supports <b>regex</b> DLP rules (AI Guard Bot rules → create in UI).
					</DialogDescription>
				</DialogHeader>

				<div className="flex-1 overflow-y-auto p-5 space-y-4">
					{/* Dropzone Area */}
					{parsedRules.length === 0 ? (
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
								<p className="text-sm font-medium text-foreground">
									Click to browse or drag & drop spreadsheet
								</p>
								<p className="text-xs text-muted-foreground mt-1">
									Supports Excel (.xlsx, .xls) or Comma-Separated Values (.csv)
								</p>
							</div>
							<div className="flex items-center gap-2 text-[11px] text-muted-foreground bg-muted/50 px-3 py-1.5 rounded-full border border-border/60">
								<HelpCircle className="h-3 w-3 text-emerald-400" />
								<span>
									Required: <b>Rule Name</b>, <b>Pattern</b> · Optional: Severity, Action, Warning Message, Active, Description
								</span>
							</div>
						</div>
					) : (
						/* File Loaded & Preview */
						<div className="space-y-4">
							<div className="flex flex-wrap items-center justify-between gap-3 p-3 bg-muted/40 rounded-lg border border-border">
								<div className="flex items-center gap-2 min-w-0">
									<FileSpreadsheet className="h-5 w-5 text-emerald-400 shrink-0" />
									<div className="min-w-0">
										<p className="text-xs font-semibold text-foreground truncate">{fileName}</p>
										<div className="flex items-center gap-2 mt-0.5 text-[11px]">
											<span className="text-emerald-400 font-medium">{validCount} valid</span>
											{invalidCount > 0 && (
												<span className="text-rose-400 font-medium">· {invalidCount} invalid</span>
											)}
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

							{/* Preview Table */}
							<div className="rounded-lg border border-border overflow-hidden">
								<div className="max-h-[320px] overflow-y-auto">
									<table className="w-full text-left text-xs border-collapse">
										<thead className="bg-muted/70 text-muted-foreground sticky top-0 border-b border-border z-10">
											<tr>
												<th className="py-2.5 px-3 font-medium">Status</th>
												<th className="py-2.5 px-3 font-medium">Rule Name</th>
												<th className="py-2.5 px-3 font-medium">Pattern</th>
												<th className="py-2.5 px-3 font-medium">Severity</th>
												<th className="py-2.5 px-3 font-medium">Action</th>
												<th className="py-2.5 px-3 font-medium">Warning Message</th>
											</tr>
										</thead>
										<tbody className="divide-y divide-border/60">
											{parsedRules.map((r, i) => (
												<tr
													key={i}
													className={
														r.valid
															? "hover:bg-muted/20"
															: "bg-rose-500/5 hover:bg-rose-500/10"
													}
												>
													<td className="py-2 px-3 align-top whitespace-nowrap">
														{r.valid ? (
															<Badge
																variant="outline"
																className="border-emerald-500/30 text-emerald-400 bg-emerald-500/10 text-[10px] gap-1 px-1.5 py-0.5"
															>
																<CheckCircle2 className="h-2.5 w-2.5" />
																Valid
															</Badge>
														) : (
															<div className="space-y-1">
																<Badge
																	variant="destructive"
																	className="text-[10px] gap-1 px-1.5 py-0.5"
																>
																	<AlertCircle className="h-2.5 w-2.5" />
																	Invalid
																</Badge>
																<p className="text-[10px] text-rose-400 break-words max-w-[120px]">
																	{r.errors.join(", ")}
																</p>
															</div>
														)}
													</td>
													<td className="py-2 px-3 align-top font-medium text-foreground max-w-[160px] truncate">
														{r.name || <span className="text-muted-foreground italic">&lt;empty&gt;</span>}
													</td>
													<td className="py-2 px-3 align-top font-mono text-[11px] text-muted-foreground max-w-[200px] truncate">
														{r.pattern || <span className="text-muted-foreground italic">&lt;empty&gt;</span>}
													</td>
													<td className="py-2 px-3 align-top whitespace-nowrap">
														<span
															className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold ${
																r.severity === "CRITICAL"
																	? "bg-rose-500/15 text-rose-400 border border-rose-500/30"
																	: r.severity === "HIGH"
																	? "bg-amber-500/15 text-amber-400 border border-amber-500/30"
																	: "bg-sky-500/15 text-sky-400 border border-sky-500/30"
															}`}
														>
															{r.severity}
														</span>
													</td>
													<td className="py-2 px-3 align-top whitespace-nowrap">
														<span
															className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold ${
																r.action === "BLOCK"
																	? "bg-red-500/15 text-red-400 border border-red-500/30"
																	: r.action === "REDACT"
																	? "bg-purple-500/15 text-purple-400 border border-purple-500/30"
																	: "bg-yellow-500/15 text-yellow-400 border border-yellow-500/30"
															}`}
														>
															{r.action}
														</span>
													</td>
													<td className="py-2 px-3 align-top text-muted-foreground max-w-[180px] truncate text-[11px]">
														{r.warning_message || "—"}
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</div>

							{/* Import Options */}
							<div className="flex items-center space-x-2 pt-1">
								<Checkbox
									id="overwriteExisting"
									checked={overwriteExisting}
									onCheckedChange={(checked) => setOverwriteExisting(Boolean(checked))}
								/>
								<label
									htmlFor="overwriteExisting"
									className="text-xs text-foreground cursor-pointer select-none font-medium"
								>
									Overwrite existing rules if a rule with the same name already exists
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

					{parsedRules.length > 0 && (
						<Button
							type="button"
							onClick={handleConfirmImport}
							disabled={isSubmitting || validCount === 0}
							className="gap-2 bg-emerald-600 hover:bg-emerald-500 text-white text-xs"
						>
							{isSubmitting ? (
								<>
									<RefreshCw className="h-3.5 w-3.5 animate-spin" />
									Importing...
								</>
							) : (
								<>
									<CheckCircle2 className="h-3.5 w-3.5" />
									Confirm & Import {validCount} Rule{validCount === 1 ? "" : "s"}
								</>
							)}
						</Button>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
