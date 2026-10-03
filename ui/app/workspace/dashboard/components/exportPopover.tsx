import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import { buildCSV, downloadCSV } from "@/lib/utils/csv";
import { downloadDocTable, downloadExcelTable } from "@/lib/utils/tableExport";
import { Download, FileSpreadsheet, FileText, Loader2 } from "lucide-react";
import { useCallback, useState } from "react";
import { toast } from "sonner";
import { type DashboardData, getCSVSections } from "../utils/exportUtils";

/** Must match the section order returned by the page's onPdfExport. */
const PDF_TAB_LABELS = [
	"Overview",
	"Provider Usage",
	"Model Rankings",
	"MCP Usage",
	"Team Rankings",
	"Customer Rankings",
	"Business Unit Rankings",
	"User Rankings",
	"Virtual Key Rankings",
];

interface ExportPopoverProps {
	/** Fetches every tab's data (including tabs never opened) for the current filters. */
	onLoadData: () => Promise<DashboardData>;
	onPdfExport: () => Promise<HTMLElement[]>;
	onPdfExportDone: () => void;
}

function flattenDashboardRows(data: DashboardData) {
	const sections = getCSVSections(data, "all");
	const width = Math.max(1, ...sections.map((s) => s.csv.headers.length));
	const columns = [
		{ key: "section", header: "Section" },
		...Array.from({ length: width }, (_, i) => ({ key: `c${i + 1}`, header: `Col ${i + 1}` })),
	];
	const rows: Record<string, string>[] = [];
	for (const section of sections) {
		if (section.csv.rows.length === 0) continue;
		const header: Record<string, string> = { section: section.name };
		section.csv.headers.forEach((h, i) => {
			header[`c${i + 1}`] = String(h ?? "");
		});
		rows.push(header);
		for (const row of section.csv.rows) {
			const out: Record<string, string> = { section: section.name };
			for (let i = 0; i < width; i++) out[`c${i + 1}`] = String(row[i] ?? "");
			rows.push(out);
		}
	}
	return { columns, rows };
}

function exportErrorMessage(err: unknown) {
	return err instanceof Error && err.message ? `Export failed: ${err.message}` : "Export failed";
}

export function ExportPopover({ onLoadData, onPdfExport, onPdfExportDone }: ExportPopoverProps) {
	const [exporting, setExporting] = useState(false);

	const handleCsvExport = useCallback(async () => {
		setExporting(true);
		try {
			const sections = getCSVSections(await onLoadData(), "all");
			const parts: string[] = [];
			for (const section of sections) {
				if (section.csv.rows.length === 0) continue;
				parts.push(`# ${section.name}`);
				parts.push(buildCSV(section.csv.headers, section.csv.rows));
				parts.push("");
			}
			if (parts.length === 0) {
				toast.info("Nothing to export for the selected filters");
				return;
			}
			downloadCSV(parts.join("\n"), "dashboard-export");
		} catch (err) {
			toast.error(exportErrorMessage(err));
		} finally {
			setExporting(false);
		}
	}, [onLoadData]);

	const handleTableExport = useCallback(
		async (format: "excel" | "doc") => {
			setExporting(true);
			try {
				const { columns, rows } = flattenDashboardRows(await onLoadData());
				if (rows.length === 0) {
					toast.info("Nothing to export for the selected filters");
					return;
				}
				if (format === "excel") {
					await downloadExcelTable({ filename: "dashboard-export", sheetName: "Dashboard", columns, rows });
				} else {
					await downloadDocTable({
						filename: "dashboard-export",
						title: "Raksha Dashboard Export",
						subtitle: "Usage and ranking snapshot",
						columns,
						rows,
						logoSrc: "/yes-panchi-logo.png",
					});
				}
			} catch (err) {
				toast.error(exportErrorMessage(err));
			} finally {
				setExporting(false);
			}
		},
		[onLoadData],
	);

	const handlePdfExport = useCallback(async () => {
		setExporting(true);

		await new Promise((r) => requestAnimationFrame(r));

		try {
			const { generatePdf } = await import("@/lib/utils/pdf");

			const elements = await onPdfExport();

			const sections = elements.map((element) => ({
				element,
				label: PDF_TAB_LABELS[Number(element.dataset.pdfIndex ?? -1)] ?? element.dataset.pdfLabel ?? "",
			}));

			await generatePdf(sections, "dashboard-export", {
				branding: {
					logoSrc: "/yes-panchi-logo.png",
					text: "Powered by",
				},
			});
		} catch (err) {
			toast.error(exportErrorMessage(err));
		} finally {
			onPdfExportDone();
			setExporting(false);
		}
	}, [onPdfExport, onPdfExportDone]);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" size="default" disabled={exporting} data-testid="dashboard-export-trigger">
					{exporting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
					{exporting ? "Exporting..." : "Export"}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem onClick={handlePdfExport} data-testid="export-pdf-item">
					<FileText className="h-4 w-4" />
					PDF
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => void handleTableExport("excel")} data-testid="export-excel-item">
					<FileSpreadsheet className="h-4 w-4" />
					Excel
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => void handleTableExport("doc")} data-testid="export-doc-item">
					<FileText className="h-4 w-4" />
					DOC
				</DropdownMenuItem>
				<DropdownMenuItem onClick={handleCsvExport} data-testid="export-csv-item">
					<FileSpreadsheet className="h-4 w-4" />
					CSV
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
