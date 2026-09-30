"use client";

import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import {
	downloadDocTable,
	downloadExcelTable,
	downloadPdfTable,
	type ExportTableColumn,
	type ExportTableRow,
} from "@/lib/utils/tableExport";
import { Braces, Download, FileSpreadsheet, FileText, Loader2 } from "lucide-react";
import { useCallback, useState } from "react";

export type ExportFormatsPayload = {
	filename: string;
	title: string;
	subtitle?: string;
	columns: ExportTableColumn[];
	rows: ExportTableRow[];
	/** Optional raw JSON for the JSON export item (defaults to rows). */
	json?: unknown;
};

interface ExportFormatsDropdownProps {
	/** Resolve rows/columns when the user picks a format (can be async). */
	getPayload: () => ExportFormatsPayload | Promise<ExportFormatsPayload>;
	disabled?: boolean;
	size?: "default" | "sm" | "lg" | "icon";
	className?: string;
	testId?: string;
	/** Include JSON in the menu (default true). */
	includeJson?: boolean;
}

function downloadJsonFile(filename: string, data: unknown) {
	const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
	const url = URL.createObjectURL(blob);
	const link = document.createElement("a");
	link.href = url;
	link.download = filename.endsWith(".json") ? filename : `${filename}.json`;
	link.click();
	URL.revokeObjectURL(url);
}

export function ExportFormatsDropdown({
	getPayload,
	disabled,
	size = "default",
	className,
	testId = "export-formats-trigger",
	includeJson = true,
}: ExportFormatsDropdownProps) {
	const [exporting, setExporting] = useState(false);

	const run = useCallback(
		async (format: "pdf" | "excel" | "doc" | "json") => {
			setExporting(true);
			await new Promise((r) => requestAnimationFrame(r));
			try {
				const payload = await getPayload();
				if (format === "json") {
					downloadJsonFile(payload.filename || "export", payload.json ?? payload.rows);
					return;
				}
				if (!payload.columns.length) return;
				if (format === "pdf") {
					await downloadPdfTable(payload);
				} else if (format === "excel") {
					await downloadExcelTable(payload);
				} else {
					await downloadDocTable(payload);
				}
			} finally {
				setExporting(false);
			}
		},
		[getPayload],
	);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="outline"
					size={size}
					disabled={disabled || exporting}
					className={className}
					data-testid={testId}
				>
					{exporting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
					{exporting ? "Exporting..." : "Export"}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem onClick={() => void run("pdf")} data-testid="export-pdf-item">
					<FileText className="h-4 w-4" />
					PDF
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => void run("excel")} data-testid="export-excel-item">
					<FileSpreadsheet className="h-4 w-4" />
					Excel
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => void run("doc")} data-testid="export-doc-item">
					<FileText className="h-4 w-4" />
					DOC
				</DropdownMenuItem>
				{includeJson ? (
					<DropdownMenuItem onClick={() => void run("json")} data-testid="export-json-item">
						<Braces className="h-4 w-4" />
						JSON
					</DropdownMenuItem>
				) : null}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
