import { toast } from "sonner";

export interface CodeBlockInfo {
	language: string;
	filename: string;
	code: string;
}

export interface TableData {
	title?: string;
	headers: string[];
	rows: string[][];
}

const EXT_MAP: Record<string, string> = {
	python: "py",
	py: "py",
	javascript: "js",
	js: "js",
	typescript: "ts",
	ts: "ts",
	tsx: "tsx",
	jsx: "jsx",
	html: "html",
	css: "css",
	scss: "scss",
	json: "json",
	jsonc: "json",
	yaml: "yaml",
	yml: "yaml",
	sql: "sql",
	sh: "sh",
	bash: "sh",
	shell: "sh",
	powershell: "ps1",
	ps1: "ps1",
	go: "go",
	golang: "go",
	rust: "rs",
	rs: "rs",
	c: "c",
	cpp: "cpp",
	csharp: "cs",
	cs: "cs",
	java: "java",
	kotlin: "kt",
	kt: "kt",
	swift: "swift",
	php: "php",
	ruby: "rb",
	rb: "rb",
	r: "r",
	markdown: "md",
	md: "md",
	csv: "csv",
	tsv: "tsv",
	xml: "xml",
	svg: "svg",
	dockerfile: "dockerfile",
};

/** Trigger browser file download from a Blob */
export function triggerBlobDownload(blob: Blob, filename: string): void {
	const url = URL.createObjectURL(blob);
	const a = document.createElement("a");
	a.href = url;
	a.download = filename;
	document.body.appendChild(a);
	a.click();
	document.body.removeChild(a);
	setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Sanitize file base name */
function sanitizeFileName(name: string, defaultName: string): string {
	const clean = name.replace(/[\\/:*?"<>|]+/g, "_").trim();
	return clean.length > 0 ? clean.slice(0, 50) : defaultName;
}

/** Guess a title from the first heading or first line of markdown */
function extractDocumentTitle(content: string): string {
	const lines = content.split("\n");
	for (const line of lines) {
		const trimmed = line.trim();
		if (trimmed.startsWith("#")) {
			return trimmed.replace(/^#+\s*/, "").replace(/[*_~`]/g, "").trim();
		}
	}
	for (const line of lines) {
		const trimmed = line.trim();
		if (trimmed.length > 3 && !trimmed.startsWith("|") && !trimmed.startsWith("```")) {
			return trimmed.replace(/[*_~`]/g, "").slice(0, 40).trim();
		}
	}
	return "document";
}

/** Parse markdown tables from text */
export function parseMarkdownTables(content: string): TableData[] {
	const tables: TableData[] = [];
	const lines = content.split("\n");
	let i = 0;

	while (i < lines.length) {
		const line = lines[i].trim();
		// Look for table header line: "| col 1 | col 2 |"
		if (line.startsWith("|") && line.endsWith("|") && i + 1 < lines.length) {
			const nextLine = lines[i + 1].trim();
			// Check if next line is separator line: "| --- | --- |"
			if (nextLine.startsWith("|") && /^\|[\s-:]+(\|[\s-:]+)+\|$/.test(nextLine)) {
				const headers = line
					.slice(1, -1)
					.split("|")
					.map((h) => h.trim());
				const rows: string[][] = [];
				let j = i + 2;
				while (j < lines.length && lines[j].trim().startsWith("|") && lines[j].trim().endsWith("|")) {
					const rowCells = lines[j]
						.trim()
						.slice(1, -1)
						.split("|")
						.map((c) => c.trim());
					rows.push(rowCells);
					j++;
				}
				if (headers.length > 0 && rows.length > 0) {
					// Check preceding line for potential title
					let title: string | undefined;
					if (i > 0 && lines[i - 1].trim()) {
						title = lines[i - 1].replace(/^#+\s*/, "").replace(/[*_]/g, "").trim();
					}
					tables.push({ title, headers, rows });
				}
				i = j;
				continue;
			}
		}

		// Also look for CSV blocks
		if (line.startsWith("```csv") || line.startsWith("```tsv")) {
			const isTsv = line.startsWith("```tsv");
			const sep = isTsv ? "\t" : ",";
			let j = i + 1;
			const csvLines: string[] = [];
			while (j < lines.length && !lines[j].trim().startsWith("```")) {
				if (lines[j].trim()) csvLines.push(lines[j]);
				j++;
			}
			if (csvLines.length >= 2) {
				const headers = csvLines[0].split(sep).map((s) => s.trim().replace(/^["']|["']$/g, ""));
				const rows = csvLines.slice(1).map((r) => r.split(sep).map((s) => s.trim().replace(/^["']|["']$/g, "")));
				tables.push({ title: "CSV Data", headers, rows });
			}
			i = j + 1;
			continue;
		}

		i++;
	}

	return tables;
}

/** Parse code blocks from content */
export function parseCodeBlocks(content: string): CodeBlockInfo[] {
	const blocks: CodeBlockInfo[] = [];
	const regex = /```([a-zA-Z0-9_-]*)\s*\n([\s\S]*?)```/g;
	let match: RegExpExecArray | null;
	let counter = 1;

	while ((match = regex.exec(content)) !== null) {
		const rawLang = (match[1] || "").toLowerCase().trim();
		const code = match[2];
		if (!code || !code.trim()) continue;

		const ext = EXT_MAP[rawLang] || rawLang || "txt";

		// Try finding embedded filename comment in the first 2 lines
		let filename = "";
		const firstLines = code.split("\n", 2);
		for (const fl of firstLines) {
			const nameMatch = fl.match(/(?:filename|file|name)[:=]\s*([a-zA-Z0-9_.-]+)/i) ||
				fl.match(/(?:\/\/|#|\/\*|<!--)\s*([a-zA-Z0-9_-]+\.[a-zA-Z0-9]+)/);
			if (nameMatch && nameMatch[1]) {
				filename = nameMatch[1].trim();
				break;
			}
		}

		if (!filename) {
			filename = `code_${counter}.${ext}`;
		}

		blocks.push({
			language: rawLang || ext,
			filename,
			code: code.trim(),
		});
		counter++;
	}

	return blocks;
}

/** Detect what file formats can be generated from content */
export function detectGeneratableFiles(content: string) {
	if (!content || !content.trim()) {
		return { canExcel: false, canPdf: false, canDocx: false, canCode: false, codeBlocks: [] };
	}
	const tables = parseMarkdownTables(content);
	const codeBlocks = parseCodeBlocks(content);
	const hasText = content.trim().length > 10;

	return {
		canExcel: tables.length > 0 || content.includes("|") || content.includes("```csv") || content.includes("```json"),
		canPdf: hasText,
		canDocx: hasText,
		canCode: codeBlocks.length > 0,
		codeBlocks,
		tablesCount: tables.length,
	};
}

/** Export assistant content / data to a genuine Excel (.xlsx) file */
export async function exportToExcel(content: string, customTitle?: string): Promise<boolean> {
	try {
		toast.loading("Generating Excel spreadsheet (.xlsx)...", { id: "excel-export" });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const XLSXmod: any = await import("xlsx");
		const XLSX = XLSXmod.default ?? XLSXmod;

		const wb = XLSX.utils.book_new();
		const tables = parseMarkdownTables(content);

		if (tables.length > 0) {
			tables.forEach((table, idx) => {
				const sheetData = [table.headers, ...table.rows];
				const ws = XLSX.utils.aoa_to_sheet(sheetData);
				const sheetName = sanitizeFileName(table.title || `Sheet${idx + 1}`, `Sheet${idx + 1}`).slice(0, 31);
				XLSX.utils.book_append_sheet(wb, ws, sheetName);
			});
		} else {
			// If JSON is present
			let jsonParsed = false;
			const jsonMatch = content.match(/```json\s*\n([\s\S]*?)```/);
			if (jsonMatch) {
				try {
					const parsed = JSON.parse(jsonMatch[1]);
					if (Array.isArray(parsed) && parsed.length > 0 && typeof parsed[0] === "object") {
						const ws = XLSX.utils.json_to_sheet(parsed);
						XLSX.utils.book_append_sheet(wb, ws, "Data");
						jsonParsed = true;
					}
				} catch {}
			}

			if (!jsonParsed) {
				// Convert plain lines / key-value lines into a structured sheet
				const rows: string[][] = [];
				const lines = content.split("\n");
				for (const line of lines) {
					const t = line.trim();
					if (!t) continue;
					if (t.includes(":") && !t.startsWith("http")) {
						const parts = t.split(":");
						rows.push([parts[0].replace(/^[-*#\s]+/, "").trim(), parts.slice(1).join(":").trim()]);
					} else {
						rows.push([t.replace(/^[-*#\s]+/, "").trim()]);
					}
				}
				const ws = XLSX.utils.aoa_to_sheet(rows.length > 0 ? rows : [["Content", content]]);
				XLSX.utils.book_append_sheet(wb, ws, "Sheet1");
			}
		}

		const docTitle = customTitle || extractDocumentTitle(content);
		const filename = `${sanitizeFileName(docTitle, "spreadsheet")}.xlsx`;
		XLSX.writeFile(wb, filename);

		toast.success(`Generated and downloaded "${filename}"!`, { id: "excel-export" });
		return true;
	} catch (err) {
		console.error("Excel generation error:", err);
		toast.error("Failed to generate Excel file", {
			id: "excel-export",
			description: err instanceof Error ? err.message : String(err),
		});
		return false;
	}
}

/** Export assistant content to a styled Word Document (.docx / .doc) */
export async function exportToWordDocx(content: string, customTitle?: string): Promise<boolean> {
	try {
		toast.loading("Generating Word document (.docx)...", { id: "word-export" });
		const docTitle = customTitle || extractDocumentTitle(content);

		// Format Markdown into clean HTML for Microsoft Word
		const paragraphs = content.split("\n\n");
		let bodyHtml = "";

		for (const p of paragraphs) {
			const trimmed = p.trim();
			if (!trimmed) continue;

			if (trimmed.startsWith("# ")) {
				bodyHtml += `<h1 style="color:#1e293b; font-family:'Segoe UI', Calibri, sans-serif; font-size:24pt; border-bottom:2px solid #3b82f6; padding-bottom:6px; margin-top:20px;">${escapeHtml(trimmed.slice(2))}</h1>`;
			} else if (trimmed.startsWith("## ")) {
				bodyHtml += `<h2 style="color:#334155; font-family:'Segoe UI', Calibri, sans-serif; font-size:18pt; border-bottom:1px solid #e2e8f0; padding-bottom:4px; margin-top:16px;">${escapeHtml(trimmed.slice(3))}</h2>`;
			} else if (trimmed.startsWith("### ")) {
				bodyHtml += `<h3 style="color:#475569; font-family:'Segoe UI', Calibri, sans-serif; font-size:14pt; margin-top:12px;">${escapeHtml(trimmed.slice(4))}</h3>`;
			} else if (trimmed.startsWith("```")) {
				const codeLines = trimmed.split("\n");
				const codeText = codeLines.slice(1, codeLines[codeLines.length - 1].startsWith("```") ? -1 : undefined).join("\n");
				bodyHtml += `<pre style="background-color:#f8fafc; border:1px solid #cbd5e1; border-radius:4px; padding:12px; font-family:Consolas, 'Courier New', monospace; font-size:9.5pt; color:#0f172a; white-space:pre-wrap; mso-line-height-rule:exactly;">${escapeHtml(codeText)}</pre>`;
			} else if (trimmed.startsWith("|") && trimmed.includes("\n|")) {
				// Table
				const tLines = trimmed.split("\n");
				const headerCells = tLines[0].slice(1, -1).split("|").map((c) => c.trim());
				let rowsHtml = "";
				for (let r = 2; r < tLines.length; r++) {
					if (!tLines[r].trim().startsWith("|")) continue;
					const cells = tLines[r].trim().slice(1, -1).split("|").map((c) => c.trim());
					rowsHtml += `<tr>${cells.map((c) => `<td style="border:1px solid #cbd5e1; padding:8px 10px; font-size:10pt;">${escapeHtml(c)}</td>`).join("")}</tr>`;
				}
				bodyHtml += `<table style="border-collapse:collapse; width:100%; margin:16px 0; font-family:'Segoe UI', Calibri, sans-serif;">
					<thead>
						<tr style="background-color:#f1f5f9; color:#0f172a; font-weight:bold;">
							${headerCells.map((h) => `<th style="border:1px solid #94a3b8; padding:8px 10px; font-size:10pt; text-align:left;">${escapeHtml(h)}</th>`).join("")}
						</tr>
					</thead>
					<tbody>${rowsHtml}</tbody>
				</table>`;
			} else if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
				const items = trimmed.split("\n").filter((l) => l.trim().startsWith("- ") || l.trim().startsWith("* "));
				bodyHtml += `<ul style="font-family:'Segoe UI', Calibri, sans-serif; font-size:11pt; color:#1e293b; line-height:1.6;">${items.map((it) => `<li>${escapeHtml(it.replace(/^[-*]\s*/, ""))}</li>`).join("")}</ul>`;
			} else {
				bodyHtml += `<p style="font-family:'Segoe UI', Calibri, sans-serif; font-size:11pt; color:#1e293b; line-height:1.6; margin-bottom:10px;">${escapeHtml(trimmed)}</p>`;
			}
		}

		const fullHtml = `<!DOCTYPE html>
<html xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:w="urn:schemas-microsoft-com:office:word" xmlns="http://www.w3.org/TR/REC-html40">
<head>
	<meta charset="utf-8">
	<title>${escapeHtml(docTitle)}</title>
	<!--[if gte mso 9]>
	<xml>
		<w:WordDocument>
			<w:View>Print</w:View>
			<w:Zoom>100</w:Zoom>
			<w:DoNotOptimizeForBrowser/>
		</w:WordDocument>
	</xml>
	<![endif]-->
	<style>
		@page {
			size: 8.5in 11.0in;
			margin: 1.0in 1.0in 1.0in 1.0in;
			mso-header-margin: .5in;
			mso-footer-margin: .5in;
		}
		body {
			font-family: 'Segoe UI', Calibri, Arial, sans-serif;
			font-size: 11pt;
			color: #1e293b;
		}
	</style>
</head>
<body>
	<div style="margin-bottom:20px;">
		<h1 style="font-size:26pt; font-weight:bold; color:#0f172a; margin-bottom:4px; font-family:'Segoe UI', sans-serif;">${escapeHtml(docTitle)}</h1>
		<p style="font-size:9pt; color:#64748b; font-style:italic;">Generated via Unifai Platform • ${new Date().toLocaleDateString(undefined, { dateStyle: "long" })}</p>
	</div>
	<hr style="border:none; border-top:1px solid #cbd5e1; margin-bottom:24px;" />
	${bodyHtml}
</body>
</html>`;

		const blob = new Blob(["\ufeff", fullHtml], {
			type: "application/msword;charset=utf-8",
		});
		const filename = `${sanitizeFileName(docTitle, "document")}.docx`;
		triggerBlobDownload(blob, filename);

		toast.success(`Generated and downloaded "${filename}"!`, { id: "word-export" });
		return true;
	} catch (err) {
		console.error("Word document generation error:", err);
		toast.error("Failed to generate Word document", {
			id: "word-export",
			description: err instanceof Error ? err.message : String(err),
		});
		return false;
	}
}

/** Export assistant content to a clean, formatted PDF document */
export async function exportToPdf(content: string, customTitle?: string): Promise<boolean> {
	try {
		toast.loading("Compiling formatted PDF document...", { id: "pdf-export" });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const jsPdfMod: any = await import("jspdf");
		const jsPDF = jsPdfMod.jsPDF ?? jsPdfMod.default ?? jsPdfMod;

		const doc = new jsPDF({
			orientation: "portrait",
			unit: "pt",
			format: "a4",
		});

		const pageWidth = doc.internal.pageSize.getWidth();
		const pageHeight = doc.internal.pageSize.getHeight();
		const margin = 40;
		const maxLineWidth = pageWidth - margin * 2;
		let y = margin + 10;

		const checkPageBreak = (neededHeight: number) => {
			if (y + neededHeight > pageHeight - margin) {
				doc.addPage();
				y = margin + 10;
			}
		};

		const docTitle = customTitle || extractDocumentTitle(content);

		// Header accent bar
		doc.setFillColor(59, 130, 246); // Blue accent
		doc.rect(margin, y, maxLineWidth, 4, "F");
		y += 18;

		// Title
		doc.setFont("helvetica", "bold");
		doc.setFontSize(20);
		doc.setTextColor(15, 23, 42); // slate-900
		const titleLines = doc.splitTextToSize(docTitle, maxLineWidth);
		doc.text(titleLines, margin, y);
		y += titleLines.length * 24 + 4;

		// Subtitle timestamp
		doc.setFont("helvetica", "normal");
		doc.setFontSize(9);
		doc.setTextColor(100, 116, 139); // slate-500
		doc.text(`Generated by Unifai • ${new Date().toLocaleString()}`, margin, y);
		y += 18;

		// Divider
		doc.setDrawColor(226, 232, 240); // slate-200
		doc.setLineWidth(1);
		doc.line(margin, y, pageWidth - margin, y);
		y += 18;

		// Process content blocks
		const blocks = content.split("\n\n");
		for (const block of blocks) {
			const trimmed = block.trim();
			if (!trimmed) continue;

			if (trimmed.startsWith("# ")) {
				checkPageBreak(35);
				doc.setFont("helvetica", "bold");
				doc.setFontSize(16);
				doc.setTextColor(30, 41, 59);
				const hLines = doc.splitTextToSize(trimmed.slice(2), maxLineWidth);
				doc.text(hLines, margin, y);
				y += hLines.length * 20 + 8;
			} else if (trimmed.startsWith("## ")) {
				checkPageBreak(30);
				doc.setFont("helvetica", "bold");
				doc.setFontSize(13);
				doc.setTextColor(51, 65, 85);
				const hLines = doc.splitTextToSize(trimmed.slice(3), maxLineWidth);
				doc.text(hLines, margin, y);
				y += hLines.length * 16 + 6;
			} else if (trimmed.startsWith("### ")) {
				checkPageBreak(25);
				doc.setFont("helvetica", "bold");
				doc.setFontSize(11);
				doc.setTextColor(71, 85, 105);
				const hLines = doc.splitTextToSize(trimmed.slice(4), maxLineWidth);
				doc.text(hLines, margin, y);
				y += hLines.length * 14 + 6;
			} else if (trimmed.startsWith("```")) {
				// Code Block
				const codeLines = trimmed.split("\n");
				const rawCode = codeLines.slice(1, codeLines[codeLines.length - 1].startsWith("```") ? -1 : undefined).join("\n");
				doc.setFont("courier", "normal");
				doc.setFontSize(8.5);
				const splitCode = doc.splitTextToSize(rawCode, maxLineWidth - 16);
				const boxHeight = splitCode.length * 11 + 14;

				checkPageBreak(Math.min(boxHeight, 150));
				doc.setFillColor(248, 250, 252);
				doc.setDrawColor(203, 213, 225);
				doc.roundedRect(margin, y, maxLineWidth, boxHeight, 3, 3, "FD");
				doc.setTextColor(15, 23, 42);
				doc.text(splitCode, margin + 8, y + 14);
				y += boxHeight + 12;
			} else if (trimmed.startsWith("|") && trimmed.includes("\n|")) {
				// Table block
				const tLines = trimmed.split("\n").filter((l) => l.trim().startsWith("|"));
				if (tLines.length >= 2) {
					const headers = tLines[0].slice(1, -1).split("|").map((c) => c.trim());
					const colWidth = maxLineWidth / Math.max(1, headers.length);
					checkPageBreak(25);

					// Table Header
					doc.setFillColor(241, 245, 249);
					doc.setDrawColor(148, 163, 184);
					doc.rect(margin, y, maxLineWidth, 18, "FD");
					doc.setFont("helvetica", "bold");
					doc.setFontSize(9);
					doc.setTextColor(15, 23, 42);
					headers.forEach((h, idx) => {
						doc.text(doc.splitTextToSize(h, colWidth - 6), margin + idx * colWidth + 4, y + 12);
					});
					y += 18;

					// Table Rows
					doc.setFont("helvetica", "normal");
					doc.setFontSize(8.5);
					for (let r = 2; r < tLines.length; r++) {
						checkPageBreak(16);
						const cells = tLines[r].trim().slice(1, -1).split("|").map((c) => c.trim());
						doc.setDrawColor(226, 232, 240);
						doc.rect(margin, y, maxLineWidth, 16);
						cells.forEach((c, idx) => {
							if (idx < headers.length) {
								doc.text(doc.splitTextToSize(c, colWidth - 6), margin + idx * colWidth + 4, y + 11);
							}
						});
						y += 16;
					}
					y += 12;
				}
			} else {
				// Paragraph
				doc.setFont("helvetica", "normal");
				doc.setFontSize(10);
				doc.setTextColor(30, 41, 59);
				const pLines = doc.splitTextToSize(trimmed, maxLineWidth);
				checkPageBreak(pLines.length * 13 + 6);
				doc.text(pLines, margin, y);
				y += pLines.length * 13 + 8;
			}
		}

		// Add page numbers in footer
		const totalPages = doc.getNumberOfPages();
		for (let p = 1; p <= totalPages; p++) {
			doc.setPage(p);
			doc.setFont("helvetica", "normal");
			doc.setFontSize(8);
			doc.setTextColor(148, 163, 184);
			doc.text(`Page ${p} of ${totalPages}`, pageWidth / 2, pageHeight - 20, { align: "center" });
		}

		const filename = `${sanitizeFileName(docTitle, "document")}.pdf`;
		doc.save(filename);
		toast.success(`Generated and downloaded "${filename}"!`, { id: "pdf-export" });
		return true;
	} catch (err) {
		console.error("PDF generation error:", err);
		toast.error("Failed to generate PDF", {
			id: "pdf-export",
			description: err instanceof Error ? err.message : String(err),
		});
		return false;
	}
}

/** Download single code file */
export function exportCodeFile(block: CodeBlockInfo): void {
	try {
		const blob = new Blob([block.code], { type: "text/plain;charset=utf-8" });
		triggerBlobDownload(blob, block.filename);
		toast.success(`Downloaded "${block.filename}"`);
	} catch (err) {
		toast.error(`Failed to download ${block.filename}`);
	}
}

/** Bundle and download all code blocks as a single Project ZIP */
export async function exportAllCodeFilesZip(blocks: CodeBlockInfo[], projectName?: string): Promise<boolean> {
	try {
		toast.loading("Packaging code files into ZIP...", { id: "zip-export" });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const JSZipMod: any = await import("jszip");
		const JSZip = JSZipMod.default ?? JSZipMod;
		const zip = new JSZip();

		blocks.forEach((block, idx) => {
			const name = block.filename || `code_${idx + 1}.${block.language || "txt"}`;
			zip.file(name, block.code);
		});

		const blob = await zip.generateAsync({ type: "blob" });
		const cleanName = `${sanitizeFileName(projectName || "project_code", "project_code")}.zip`;
		triggerBlobDownload(blob, cleanName);

		toast.success(`Generated and downloaded "${cleanName}" with ${blocks.length} files!`, { id: "zip-export" });
		return true;
	} catch (err) {
		console.error("ZIP export error:", err);
		toast.error("Failed to generate ZIP", {
			id: "zip-export",
			description: err instanceof Error ? err.message : String(err),
		});
		return false;
	}
}

function escapeHtml(text: string): string {
	return text
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;")
		.replace(/'/g, "&#039;");
}
