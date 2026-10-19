import { toast } from "sonner";
import { COMPANY_LOGO, PRODUCT_NAME } from "@/lib/constants/config";
import { loadLogoAsset, type LogoAsset } from "@/lib/utils/tableExport";

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
		canExcel: hasText,
		canPdf: hasText,
		canDocx: hasText,
		canCode: codeBlocks.length > 0,
		codeBlocks,
		tablesCount: tables.length,
	};
}

const LOGO_SRC = COMPANY_LOGO || "/yes-panchi-logo.png";

function logoDrawSize(logo: LogoAsset, maxW: number, maxH: number): { w: number; h: number } {
	const aspect = (logo.width || 1) / (logo.height || 1);
	let w = maxW;
	let h = w / aspect;
	if (h > maxH) {
		h = maxH;
		w = h * aspect;
	}
	return { w, h };
}

function plain(text: string): string {
	return text
		.replace(/\*\*(.*?)\*\*/g, "$1")
		.replace(/__(.*?)__/g, "$1")
		.replace(/(^|\s)\*(.*?)\*(?=\s|$)/g, "$1$2")
		.replace(/`([^`]+)`/g, "$1");
}

function parseCsv(text: string): string[][] | null {
	const lines = text
		.split("\n")
		.map((line) => line.trim())
		.filter(Boolean);
	if (lines.length < 2 || !lines.every((line) => line.includes(","))) return null;
	return lines.map((line) => line.split(",").map((cell) => plain(cell.trim())));
}

/** Walk markdown into blocks so exports keep every line instead of clipping a paragraph. */
function markdownBlocks(content: string): Array<{ kind: string; text: string; rows?: string[][] }> {
	const lines = content.replace(/\r\n/g, "\n").split("\n");
	const blocks: Array<{ kind: string; text: string; rows?: string[][] }> = [];
	let i = 0;
	while (i < lines.length) {
		const line = lines[i];
		const trimmed = line.trim();
		if (!trimmed) {
			i++;
			continue;
		}
		if (trimmed.startsWith("```")) {
			const code: string[] = [];
			i++;
			while (i < lines.length && !lines[i].trim().startsWith("```")) {
				code.push(lines[i]);
				i++;
			}
			if (i < lines.length) i++;
			blocks.push({ kind: "code", text: code.join("\n") });
			continue;
		}
		if (trimmed.startsWith("|") && trimmed.endsWith("|")) {
			const tableLines: string[] = [];
			while (i < lines.length && lines[i].trim().startsWith("|") && lines[i].trim().endsWith("|")) {
				tableLines.push(lines[i].trim());
				i++;
			}
			const rows = tableLines
				.filter((l) => !/^\|[\s-:|]+\|$/.test(l))
				.map((l) => l.slice(1, -1).split("|").map((c) => plain(c.trim())));
			blocks.push({ kind: "table", text: "", rows });
			continue;
		}
		if (/^#{1,3}\s+/.test(trimmed)) {
			const level = trimmed.match(/^#+/)?.[0].length || 1;
			blocks.push({ kind: `h${level}`, text: plain(trimmed.replace(/^#+\s*/, "")) });
			i++;
			continue;
		}
		if (/^[-*]\s+/.test(trimmed) || /^\d+\.\s+/.test(trimmed)) {
			const ordered = /^\d+\.\s+/.test(trimmed);
			const items: string[] = [];
			while (i < lines.length) {
				const t = lines[i].trim();
				if (ordered ? /^\d+\.\s+/.test(t) : /^[-*]\s+/.test(t)) {
					items.push(t.replace(/^[-*]\s+/, "").replace(/^\d+\.\s+/, ""));
					i++;
				} else break;
			}
			blocks.push({ kind: ordered ? "ol" : "ul", text: items.map(plain).join("\n") });
			continue;
		}
		const para: string[] = [trimmed];
		i++;
		while (i < lines.length) {
			const t = lines[i].trim();
			if (!t || t.startsWith("```") || t.startsWith("|") || /^#{1,3}\s+/.test(t) || /^[-*]\s+/.test(t) || /^\d+\.\s+/.test(t)) break;
			para.push(t);
			i++;
		}
		blocks.push({ kind: "p", text: para.map(plain).join("\n") });
	}
	return blocks;
}

async function embedExcelLogo(xlsxData: ArrayBuffer, logo: LogoAsset): Promise<ArrayBuffer> {
	const JSZipMod = await import("jszip");
	const JSZip = JSZipMod.default ?? JSZipMod;
	const zip = await JSZip.loadAsync(xlsxData);
	const png = logo.dataUrl.split(",")[1] || "";
	if (!png) return xlsxData;
	zip.file("xl/media/logo.png", png, { base64: true });
	const { w, h } = logoDrawSize(logo, 180, 48);
	const cx = Math.round(w * 9525);
	const cy = Math.round(h * 9525);
	zip.file(
		"xl/drawings/drawing1.xml",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <xdr:oneCellAnchor>
    <xdr:from><xdr:col>0</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>0</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:from>
    <xdr:ext cx="${cx}" cy="${cy}"/>
    <xdr:pic>
      <xdr:nvPicPr><xdr:cNvPr id="2" name="Logo"/><xdr:cNvPicPr/></xdr:nvPicPr>
      <xdr:blipFill><a:blip xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:embed="rId1"/><a:stretch><a:fillRect/></a:stretch></xdr:blipFill>
      <xdr:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="${cx}" cy="${cy}"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></xdr:spPr>
    </xdr:pic>
    <xdr:clientData/>
  </xdr:oneCellAnchor>
</xdr:wsDr>`,
	);
	zip.file(
		"xl/drawings/_rels/drawing1.xml.rels",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/logo.png"/>
</Relationships>`,
	);
	const typesPath = "[Content_Types].xml";
	let types = await zip.file(typesPath)?.async("string");
	if (types) {
		if (!types.includes('Extension="png"')) {
			types = types.replace("</Types>", `<Default Extension="png" ContentType="image/png"/></Types>`);
		}
		if (!types.includes("/xl/drawings/drawing1.xml")) {
			types = types.replace(
				"</Types>",
				`<Override PartName="/xl/drawings/drawing1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawing+xml"/></Types>`,
			);
		}
		zip.file(typesPath, types);
	}
	const sheetPath = "xl/worksheets/sheet1.xml";
	let sheet = await zip.file(sheetPath)?.async("string");
	if (sheet && !sheet.includes("<drawing ")) {
		if (!sheet.includes("xmlns:r=")) {
			sheet = sheet.replace("<worksheet ", `<worksheet xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" `);
		}
		sheet = sheet.replace("</worksheet>", `<drawing r:id="rIdLogo"/></worksheet>`);
		zip.file(sheetPath, sheet);
	}
	const relPath = "xl/worksheets/_rels/sheet1.xml.rels";
	let rels = await zip.file(relPath)?.async("string");
	const drawingRel = `<Relationship Id="rIdLogo" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing" Target="../drawings/drawing1.xml"/>`;
	if (rels) {
		if (!rels.includes("drawing1.xml")) {
			rels = rels.replace("</Relationships>", `${drawingRel}</Relationships>`);
			zip.file(relPath, rels);
		}
	} else {
		zip.file(
			relPath,
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${drawingRel}</Relationships>`,
		);
	}
	return await zip.generateAsync({ type: "arraybuffer" });
}

/** Export assistant content / data to a genuine Excel (.xlsx) file */
export async function exportToExcel(content: string, customTitle?: string): Promise<boolean> {
	try {
		toast.loading("Generating Excel spreadsheet (.xlsx)...", { id: "excel-export" });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const XLSXmod: any = await import("xlsx");
		const XLSX = XLSXmod.default ?? XLSXmod;

		const wb = XLSX.utils.book_new();
		const docTitle = customTitle || extractDocumentTitle(content);
		const logo = await loadLogoAsset(LOGO_SRC);
		const csvRows = parseCsv(content);
		if (csvRows) {
			const ws = XLSX.utils.aoa_to_sheet(csvRows);
			ws["!cols"] = csvRows[0].map(() => ({ wch: 28 }));
			XLSX.utils.book_append_sheet(wb, ws, "Sheet1");
			const filename = `${sanitizeFileName(docTitle, "spreadsheet")}.xlsx`;
			let out = XLSX.write(wb, { bookType: "xlsx", type: "array" }) as ArrayBuffer;
			if (logo) {
				try {
					out = await embedExcelLogo(out, logo);
				} catch (logoErr) {
					console.error("Excel logo embed failed:", logoErr);
				}
			}
			triggerBlobDownload(
				new Blob([out], { type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" }),
				filename,
			);
			toast.success(`Generated and downloaded "${filename}"!`, { id: "excel-export" });
			return true;
		}
		const contentRows: string[][] = [
			[""],
			[""],
			[""],
			[PRODUCT_NAME],
			[docTitle],
			[`Exported ${new Date().toLocaleString()}`],
			[""],
		];
		for (const block of markdownBlocks(content)) {
			if (block.kind === "table" && block.rows) {
				for (const row of block.rows) contentRows.push(row);
				contentRows.push([""]);
				continue;
			}
			for (const line of block.text.split("\n")) {
				contentRows.push([line]);
			}
			contentRows.push([""]);
		}
		const ws = XLSX.utils.aoa_to_sheet(contentRows);
		ws["!cols"] = [{ wch: 120 }];
		XLSX.utils.book_append_sheet(wb, ws, "Content");

		const tables = parseMarkdownTables(content);
		tables.forEach((table, idx) => {
			const sheetData = [table.headers, ...table.rows];
			const tableSheet = XLSX.utils.aoa_to_sheet(sheetData);
			const sheetName = sanitizeFileName(table.title || `Table${idx + 1}`, `Table${idx + 1}`).slice(0, 31);
			XLSX.utils.book_append_sheet(wb, tableSheet, sheetName || `Table${idx + 1}`);
		});

		const filename = `${sanitizeFileName(docTitle, "spreadsheet")}.xlsx`;
		let out = XLSX.write(wb, { bookType: "xlsx", type: "array" }) as ArrayBuffer;
		if (logo) {
			try {
				out = await embedExcelLogo(out, logo);
			} catch (logoErr) {
				console.error("Excel logo embed failed:", logoErr);
			}
		}
		triggerBlobDownload(
			new Blob([out], { type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" }),
			filename,
		);

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
		const logo = await loadLogoAsset(LOGO_SRC);
		let logoHtml = "";
		if (logo) {
			const size = logoDrawSize(logo, 180, 48);
			logoHtml = `<p style="margin:0 0 12pt 0;"><img src="${logo.dataUrl}" alt="Logo" width="${Math.round(size.w)}" height="${Math.round(size.h)}" /></p>`;
		}

		let bodyHtml = "";
		for (const block of markdownBlocks(content)) {
			if (block.kind === "h1" || block.kind === "h2" || block.kind === "h3") {
				const tag = block.kind;
				bodyHtml += `<${tag} style="font-family:'Segoe UI', Calibri, sans-serif; color:#1e293b;">${escapeHtml(block.text)}</${tag}>`;
			} else if (block.kind === "code") {
				bodyHtml += `<pre style="background-color:#f8fafc; border:1px solid #cbd5e1; padding:12px; font-family:Consolas, 'Courier New', monospace; font-size:9.5pt; color:#0f172a; white-space:pre-wrap;">${escapeHtml(block.text)}</pre>`;
			} else if (block.kind === "table" && block.rows) {
				const [header, ...rest] = block.rows;
				bodyHtml += `<table style="border-collapse:collapse; width:100%; margin:12px 0;"><thead><tr>${(header || []).map((h) => `<th style="border:1px solid #94a3b8; padding:8px;">${escapeHtml(h)}</th>`).join("")}</tr></thead><tbody>${rest.map((cells) => `<tr>${cells.map((c) => `<td style="border:1px solid #cbd5e1; padding:8px;">${escapeHtml(c)}</td>`).join("")}</tr>`).join("")}</tbody></table>`;
			} else if (block.kind === "ul" || block.kind === "ol") {
				const tag = block.kind;
				bodyHtml += `<${tag} style="font-family:'Segoe UI', Calibri, sans-serif; font-size:11pt; line-height:1.6;">${block.text.split("\n").map((it) => `<li>${escapeHtml(it)}</li>`).join("")}</${tag}>`;
			} else {
				bodyHtml += `<p style="font-family:'Segoe UI', Calibri, sans-serif; font-size:11pt; color:#1e293b; line-height:1.6; margin-bottom:10px;">${block.text.split("\n").map((line) => escapeHtml(line)).join("<br/>")}</p>`;
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
	${logoHtml}
	<div style="margin-bottom:20px;">
		<h1 style="font-size:26pt; font-weight:bold; color:#0f172a; margin-bottom:4px; font-family:'Segoe UI', sans-serif;">${escapeHtml(docTitle)}</h1>
		<p style="font-size:9pt; color:#64748b; font-style:italic;">${escapeHtml(PRODUCT_NAME)} • ${new Date().toLocaleDateString(undefined, { dateStyle: "long" })}</p>
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
		const footerGap = 36;
		const maxLineWidth = pageWidth - margin * 2;
		let y = margin;

		const ensure = (needed: number) => {
			if (y + needed > pageHeight - footerGap) {
				doc.addPage();
				y = margin;
			}
		};

		const writeWrapped = (text: string, lineH: number, indent = 0) => {
			const lines = doc.splitTextToSize(text || " ", Math.max(40, maxLineWidth - indent));
			for (const line of lines) {
				ensure(lineH);
				doc.text(String(line), margin + indent, y);
				y += lineH;
			}
		};

		const docTitle = customTitle || extractDocumentTitle(content);
		const logo = await loadLogoAsset(LOGO_SRC);
		if (logo) {
			const size = logoDrawSize(logo, 160, 42);
			ensure(size.h + 8);
			doc.addImage(logo.dataUrl, "PNG", margin, y, size.w, size.h);
			y += size.h + 12;
		}

		doc.setFont("helvetica", "bold");
		doc.setFontSize(18);
		doc.setTextColor(15, 23, 42);
		writeWrapped(docTitle, 22);
		y += 4;

		doc.setFont("helvetica", "normal");
		doc.setFontSize(9);
		doc.setTextColor(100, 116, 139);
		writeWrapped(`${PRODUCT_NAME} • ${new Date().toLocaleString()}`, 12);
		y += 8;
		ensure(8);
		doc.setDrawColor(226, 232, 240);
		doc.setLineWidth(1);
		doc.line(margin, y, pageWidth - margin, y);
		y += 16;

		for (const block of markdownBlocks(content)) {
			if (block.kind === "h1" || block.kind === "h2" || block.kind === "h3") {
				const size = block.kind === "h1" ? 16 : block.kind === "h2" ? 13 : 11;
				doc.setFont("helvetica", "bold");
				doc.setFontSize(size);
				doc.setTextColor(30, 41, 59);
				writeWrapped(block.text, size + 4);
				y += 4;
			} else if (block.kind === "code") {
				doc.setFont("courier", "normal");
				doc.setFontSize(8.5);
				doc.setTextColor(15, 23, 42);
				for (const rawLine of block.text.split("\n")) {
					const wrapped = doc.splitTextToSize(rawLine || " ", maxLineWidth - 16);
					for (const line of wrapped) {
						ensure(12);
						doc.setFillColor(248, 250, 252);
						doc.rect(margin, y - 9, maxLineWidth, 12, "F");
						doc.text(String(line), margin + 8, y);
						y += 12;
					}
				}
				y += 8;
			} else if (block.kind === "table" && block.rows && block.rows.length) {
				const colCount = Math.max(...block.rows.map((r) => r.length), 1);
				const colWidth = maxLineWidth / colCount;
				block.rows.forEach((cells, rowIdx) => {
					doc.setFont("helvetica", rowIdx === 0 ? "bold" : "normal");
					doc.setFontSize(8.5);
					const wrappedCells = cells.map((c) => doc.splitTextToSize(c || " ", colWidth - 8));
					const rowH = Math.max(16, ...wrappedCells.map((w) => w.length * 11)) + 6;
					ensure(rowH);
					doc.setDrawColor(226, 232, 240);
					if (rowIdx === 0) doc.setFillColor(241, 245, 249);
					doc.rect(margin, y, maxLineWidth, rowH, rowIdx === 0 ? "FD" : "S");
					doc.setTextColor(15, 23, 42);
					wrappedCells.forEach((w, idx) => {
						doc.text(w, margin + idx * colWidth + 4, y + 12);
					});
					y += rowH;
				});
				y += 10;
			} else if (block.kind === "ul" || block.kind === "ol") {
				doc.setFont("helvetica", "normal");
				doc.setFontSize(10);
				doc.setTextColor(30, 41, 59);
				block.text.split("\n").forEach((item, idx) => {
					const prefix = block.kind === "ol" ? `${idx + 1}. ` : "• ";
					writeWrapped(prefix + item, 14, 8);
				});
				y += 6;
			} else {
				doc.setFont("helvetica", "normal");
				doc.setFontSize(10);
				doc.setTextColor(30, 41, 59);
				for (const line of block.text.split("\n")) {
					writeWrapped(line, 14);
				}
				y += 8;
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
