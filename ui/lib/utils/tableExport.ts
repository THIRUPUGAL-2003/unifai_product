/**
 * Table export helpers: PDF, Excel (.xlsx), and Word-compatible .doc with logo.
 */

export type ExportTableColumn = { key: string; header: string };
export type ExportTableRow = Record<string, string | number | boolean | null | undefined>;

import { COMPANY_LOGO } from "@/lib/constants/config";

const LOGO_SRC = COMPANY_LOGO || "/yes-panchi-logo.png";

function dateStamp(): string {
	const now = new Date();
	return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
}

function triggerDownload(blob: Blob, filename: string): void {
	const url = URL.createObjectURL(blob);
	const link = document.createElement("a");
	link.href = url;
	link.download = filename;
	document.body.appendChild(link);
	link.click();
	link.remove();
	setTimeout(() => URL.revokeObjectURL(url), 0);
}

function escapeHtml(s: string): string {
	return String(s ?? "")
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;");
}

async function loadLogoDataUrl(logoSrc = LOGO_SRC): Promise<string | null> {
	try {
		const res = await fetch(logoSrc);
		if (!res.ok) return null;
		const blob = await res.blob();
		return await new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = () => resolve(String(reader.result || "") || null);
			reader.onerror = () => reject(reader.error);
			reader.readAsDataURL(blob);
		});
	} catch {
		return null;
	}
}

function cellValue(row: ExportTableRow, key: string): string {
	const v = row[key];
	if (v === null || v === undefined) return "";
	return String(v);
}

function buildHtmlDocument(opts: {
	title: string;
	subtitle?: string;
	columns: ExportTableColumn[];
	rows: ExportTableRow[];
	logoDataUrl?: string | null;
}): string {
	const { title, subtitle, columns, rows, logoDataUrl } = opts;
	const headerCells = columns.map((c) => `<th style="border:1px solid #ccc;padding:6px 8px;background:#f3f4f6;text-align:left;font-size:11px;">${escapeHtml(c.header)}</th>`).join("");
	const bodyRows = rows
		.map((row) => {
			const cells = columns
				.map((c) => `<td style="border:1px solid #ddd;padding:5px 8px;font-size:10px;vertical-align:top;">${escapeHtml(cellValue(row, c.key))}</td>`)
				.join("");
			return `<tr>${cells}</tr>`;
		})
		.join("");

	const logoBlock = logoDataUrl
		? `<img src="${logoDataUrl}" alt="Logo" style="height:36px;width:auto;margin-bottom:8px;" />`
		: "";

	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8" />
<title>${escapeHtml(title)}</title>
</head>
<body style="font-family:Segoe UI,Arial,sans-serif;color:#111;padding:24px;">
${logoBlock}
<h1 style="font-size:18px;margin:0 0 4px 0;">${escapeHtml(title)}</h1>
${subtitle ? `<p style="font-size:11px;color:#666;margin:0 0 16px 0;">${escapeHtml(subtitle)}</p>` : ""}
<p style="font-size:10px;color:#888;margin:0 0 12px 0;">Exported ${new Date().toLocaleString()} · ${rows.length} row(s)</p>
<table style="border-collapse:collapse;width:100%;">
<thead><tr>${headerCells}</tr></thead>
<tbody>${bodyRows || `<tr><td colspan="${columns.length}" style="padding:8px;color:#666;">No data</td></tr>`}</tbody>
</table>
</body>
</html>`;
}

/** Excel (.xlsx) export via SheetJS. */
export async function downloadExcelTable(opts: {
	filename: string;
	sheetName?: string;
	columns: ExportTableColumn[];
	rows: ExportTableRow[];
}): Promise<void> {
	const XLSXmod: any = await import("xlsx");
	const XLSX = XLSXmod.default ?? XLSXmod;
	const headers = opts.columns.map((c) => c.header);
	const data = opts.rows.map((row) => opts.columns.map((c) => cellValue(row, c.key)));
	const aoa = [headers, ...data];
	const ws = XLSX.utils.aoa_to_sheet(aoa);
	const wb = XLSX.utils.book_new();
	XLSX.utils.book_append_sheet(wb, ws, (opts.sheetName || "Export").slice(0, 31));
	const out = XLSX.write(wb, { bookType: "xlsx", type: "array" });
	triggerDownload(
		new Blob([out], { type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" }),
		`${opts.filename}-${dateStamp()}.xlsx`,
	);
}

/**
 * Word-compatible .doc (HTML Word) with product logo embedded.
 * Opens cleanly in Microsoft Word / LibreOffice.
 */
export async function downloadDocTable(opts: {
	filename: string;
	title: string;
	subtitle?: string;
	columns: ExportTableColumn[];
	rows: ExportTableRow[];
	logoSrc?: string;
}): Promise<void> {
	const logoDataUrl = await loadLogoDataUrl(opts.logoSrc || LOGO_SRC);
	const html = buildHtmlDocument({
		title: opts.title,
		subtitle: opts.subtitle,
		columns: opts.columns,
		rows: opts.rows,
		logoDataUrl,
	});
	const wordHtml = `\uFEFF<html xmlns:o="urn:schemas-microsoft-com:office:office"
 xmlns:w="urn:schemas-microsoft-com:office:word"
 xmlns="http://www.w3.org/TR/REC-html40">
<head><meta charset="utf-8"><title>${escapeHtml(opts.title)}</title>
<!--[if gte mso 9]><xml><w:WordDocument><w:View>Print</w:View></w:WordDocument></xml><![endif]-->
</head>
<body>${html.replace(/^[\s\S]*<body[^>]*>/i, "").replace(/<\/body>[\s\S]*$/i, "")}</body></html>`;

	triggerDownload(
		new Blob([wordHtml], { type: "application/msword;charset=utf-8" }),
		`${opts.filename}-${dateStamp()}.doc`,
	);
}

/** Truncate for PDF cells — long regex/bot prompts must not blow page layout. */
function pdfCellText(raw: string, maxChars: number): string {
	const s = String(raw ?? "").replace(/\s+/g, " ").trim();
	if (s.length <= maxChars) return s;
	return s.slice(0, Math.max(0, maxChars - 1)) + "…";
}

/**
 * PDF table export via jsPDF text drawing (NOT html2canvas).
 *
 * html2canvas fails on large exports (2000+ Guard Rules) — canvas height limits
 * produce solid black pages. Text PDF stays readable and scales to any row count.
 */
export async function downloadPdfTable(opts: {
	filename: string;
	title: string;
	subtitle?: string;
	columns: ExportTableColumn[];
	rows: ExportTableRow[];
	logoSrc?: string;
}): Promise<void> {
	const { jsPDF } = await import("jspdf");
	const logoDataUrl = await loadLogoDataUrl(opts.logoSrc || LOGO_SRC);

	const pdf = new jsPDF({ orientation: "landscape", unit: "mm", format: "a4" });
	const pageW = pdf.internal.pageSize.getWidth();
	const pageH = pdf.internal.pageSize.getHeight();
	const margin = 10;
	const usableW = pageW - margin * 2;
	const footerH = 8;
	const cols = opts.columns;
	const colCount = Math.max(1, cols.length);

	// Weight pattern/description columns wider; keep IDs/flags narrow.
	const weights = cols.map((c) => {
		const k = c.key.toLowerCase();
		if (k.includes("pattern") || k.includes("prompt") || k.includes("description") || k.includes("policy")) return 2.4;
		if (k.includes("name") || k.includes("domain") || k.includes("platform") || k.includes("query")) return 1.4;
		if (k.includes("active") || k.includes("action") || k.includes("severity") || k.includes("type")) return 0.7;
		return 1;
	});
	const weightSum = weights.reduce((a, b) => a + b, 0) || 1;
	const colWidths = weights.map((w) => (w / weightSum) * usableW);
	const maxCharsPerCol = colWidths.map((w) => Math.max(12, Math.floor(w / 1.35)));

	const rowLineH = 3.6;
	const headerLineH = 4.2;
	const cellPadX = 1.2;

	const drawHeaderBand = (y: number) => {
		pdf.setFillColor(243, 244, 246);
		pdf.rect(margin, y - 3.2, usableW, headerLineH + 1.5, "F");
		pdf.setFont("helvetica", "bold");
		pdf.setFontSize(8);
		pdf.setTextColor(30, 30, 30);
		let x = margin;
		for (let i = 0; i < colCount; i++) {
			const label = pdfCellText(cols[i].header, maxCharsPerCol[i]);
			pdf.text(label, x + cellPadX, y);
			x += colWidths[i];
		}
		pdf.setDrawColor(200, 200, 200);
		pdf.setLineWidth(0.2);
		pdf.line(margin, y + 1.8, margin + usableW, y + 1.8);
		return y + headerLineH + 1.2;
	};

	const drawFooter = (pageNum: number, totalHint: string) => {
		pdf.setFont("helvetica", "normal");
		pdf.setFontSize(7);
		pdf.setTextColor(140, 140, 140);
		pdf.text(`Page ${pageNum} · ${totalHint}`, margin, pageH - 4);
		pdf.text("Powered by UnifAI", pageW - margin, pageH - 4, { align: "right" });
	};

	// Title block (page 1)
	let y = margin;
	if (logoDataUrl) {
		try {
			pdf.addImage(logoDataUrl, "PNG", margin, y - 2, 18, 7);
			y += 8;
		} catch {
			// continue without logo
		}
	}
	pdf.setFont("helvetica", "bold");
	pdf.setFontSize(14);
	pdf.setTextColor(17, 17, 17);
	pdf.text(opts.title || "Export", margin, y);
	y += 6;
	pdf.setFont("helvetica", "normal");
	pdf.setFontSize(9);
	pdf.setTextColor(100, 100, 100);
	const sub = [opts.subtitle, `Exported ${new Date().toLocaleString()}`, `${opts.rows.length} row(s)`]
		.filter(Boolean)
		.join(" · ");
	pdf.text(sub, margin, y);
	y += 7;

	y = drawHeaderBand(y);
	let pageNum = 1;
	const totalHint = `${opts.rows.length} row(s)`;

	pdf.setFont("helvetica", "normal");
	pdf.setFontSize(7.5);
	pdf.setTextColor(20, 20, 20);

	for (let r = 0; r < opts.rows.length; r++) {
		const row = opts.rows[r];
		const cells = cols.map((c, i) => pdfCellText(cellValue(row, c.key), maxCharsPerCol[i]));

		// Wrap long cells within column width
		const wrapped: string[][] = cells.map((text, i) => {
			const lines = pdf.splitTextToSize(text || "—", Math.max(8, colWidths[i] - cellPadX * 2));
			return Array.isArray(lines) ? lines.slice(0, 4) : [String(lines)];
		});
		const linesUsed = Math.max(1, ...wrapped.map((w) => w.length));
		const blockH = linesUsed * rowLineH + 1.2;

		if (y + blockH > pageH - margin - footerH) {
			drawFooter(pageNum, totalHint);
			pdf.addPage();
			pageNum += 1;
			y = margin;
			y = drawHeaderBand(y);
			pdf.setFont("helvetica", "normal");
			pdf.setFontSize(7.5);
			pdf.setTextColor(20, 20, 20);
		}

		if (r % 2 === 1) {
			pdf.setFillColor(249, 250, 251);
			pdf.rect(margin, y - 2.6, usableW, blockH, "F");
		}

		let x = margin;
		for (let i = 0; i < colCount; i++) {
			const lines = wrapped[i];
			for (let li = 0; li < lines.length; li++) {
				pdf.text(lines[li], x + cellPadX, y + li * rowLineH);
			}
			x += colWidths[i];
		}
		y += blockH;
	}

	drawFooter(pageNum, totalHint);
	pdf.save(`${opts.filename}-${dateStamp()}.pdf`);
}
