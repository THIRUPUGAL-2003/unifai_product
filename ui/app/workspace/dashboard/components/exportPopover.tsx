import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import { buildCSV, downloadCSV } from "@/lib/utils/csv";
import { downloadDocTable, downloadExcelTable } from "@/lib/utils/tableExport";
import { Download, FileSpreadsheet, FileText, Loader2 } from "lucide-react";
import { useCallback, useState } from "react";
import { type DashboardData, getCSVSections } from "../utils/exportUtils";

const PDF_TAB_LABELS = ["Overview", "Provider Usage", "Model Rankings", "MCP Usage"];

interface ExportPopoverProps {
	getData: () => DashboardData;
	onPreloadData: () => Promise<void>;
	onPdfExport: () => Promise<HTMLElement[]>;
	onPdfExportDone: () => void;
}

function flattenDashboardRows(data: DashboardData) {
	const sections = getCSVSections(data, "all");
	const columns = [
		{ key: "section", header: "Section" },
		{ key: "c1", header: "Col 1" },
		{ key: "c2", header: "Col 2" },
		{ key: "c3", header: "Col 3" },
		{ key: "c4", header: "Col 4" },
		{ key: "c5", header: "Col 5" },
		{ key: "c6", header: "Col 6" },
	];
	const rows: Record<string, string>[] = [];
	for (const section of sections) {
		for (const row of section.csv.rows) {
			rows.push({
				section: section.name,
				c1: String(row[0] ?? ""),
				c2: String(row[1] ?? ""),
				c3: String(row[2] ?? ""),
				c4: String(row[3] ?? ""),
				c5: String(row[4] ?? ""),
				c6: String(row[5] ?? ""),
			});
		}
	}
	return { columns, rows };
}

export function ExportPopover({ getData, onPreloadData, onPdfExport, onPdfExportDone }: ExportPopoverProps) {
	const [exporting, setExporting] = useState(false);

	const handleCsvExport = useCallback(async () => {
		setExporting(true);
		try {
			await onPreloadData();
			const sections = getCSVSections(getData(), "all");
			const parts: string[] = [];
			for (const section of sections) {
				if (section.csv.rows.length === 0) continue;
				parts.push(`# ${section.name}`);
				parts.push(buildCSV(section.csv.headers, section.csv.rows));
				parts.push("");
			}
			if (parts.length > 0) {
				downloadCSV(parts.join("\n"), "dashboard-export");
			}
		} finally {
			setExporting(false);
		}
	}, [getData, onPreloadData]);

	const handleExcelExport = useCallback(async () => {
		setExporting(true);
		try {
			await onPreloadData();
			const { columns, rows } = flattenDashboardRows(getData());
			await downloadExcelTable({
				filename: "dashboard-export",
				sheetName: "Dashboard",
				columns,
				rows,
			});
		} finally {
			setExporting(false);
		}
	}, [getData, onPreloadData]);

	const handleDocExport = useCallback(async () => {
		setExporting(true);
		try {
			await onPreloadData();
			const { columns, rows } = flattenDashboardRows(getData());
			await downloadDocTable({
				filename: "dashboard-export",
				title: "UnifAI Dashboard Export",
				subtitle: "Usage and ranking snapshot",
				columns,
				rows,
				logoSrc: "/yes-panchi-logo.png",
			});
		} finally {
			setExporting(false);
		}
	}, [getData, onPreloadData]);

	const handlePdfExport = useCallback(async () => {
		setExporting(true);

		await new Promise((r) => requestAnimationFrame(r));

		try {
			const { generatePdf } = await import("@/lib/utils/pdf");

			const elements = await onPdfExport();

			const sections = elements.map((element, i) => ({
				element,
				label: PDF_TAB_LABELS[i],
			}));

			await generatePdf(sections, "dashboard-export", {
				branding: {
					logoSrc: "/yes-panchi-logo.png",
					text: "Powered by",
				},
			});
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
				<DropdownMenuItem onClick={handleExcelExport} data-testid="export-excel-item">
					<FileSpreadsheet className="h-4 w-4" />
					Excel
				</DropdownMenuItem>
				<DropdownMenuItem onClick={handleDocExport} data-testid="export-doc-item">
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
