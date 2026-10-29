import { describe, expect, it, vi } from "vitest";
import { extractPromptFileText, MAX_TEXT_EXTRACTION_BYTES } from "./extractFileText";

describe("prompt attachment text extraction", () => {
	it("extracts source code and CSV text without losing their content", async () => {
		const source = new File(["const answer = 42;"], "answer.ts", { type: "text/typescript" });
		const csv = new File(['Name,Note\n"Ada","Research, design"'], "people.csv", { type: "text/csv" });

		expect(await extractPromptFileText(source, source.type)).toContain("const answer = 42;");
		expect(await extractPromptFileText(csv, csv.type)).toContain('"Ada","Research, design"');
	});

	it("extracts readable text from DOCX and XLSX packages", async () => {
		const JSZipMod = await import("jszip");
		const JSZip = JSZipMod.default ?? JSZipMod;
		const docxZip = new JSZip();
		docxZip.file(
			"word/document.xml",
			'<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Quarterly report content</w:t></w:r></w:p></w:body></w:document>',
		);
		const docxBytes = await docxZip.generateAsync({ type: "uint8array" });
		const docxBuffer = docxBytes.slice().buffer as ArrayBuffer;
		const docx = new File([docxBuffer], "report.docx", {
			type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		});

		const XLSXMod = await import("xlsx");
		const XLSX = XLSXMod.default ?? XLSXMod;
		const workbook = XLSX.utils.book_new();
		XLSX.utils.book_append_sheet(
			workbook,
			XLSX.utils.aoa_to_sheet([
				["Name", "Score"],
				["Ada", 98],
			]),
			"Results",
		);
		const xlsxBytes = XLSX.write(workbook, { bookType: "xlsx", type: "array" }) as ArrayBuffer;
		const xlsx = new File([xlsxBytes], "results.xlsx", {
			type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		});

		expect(await extractPromptFileText(docx, docx.type)).toContain("Quarterly report content");
		const extractedSheet = await extractPromptFileText(xlsx, xlsx.type);
		expect(extractedSheet).toContain("Ada");
		expect(extractedSheet).toContain("98");
	});

	it("refuses an oversized document before allocating its contents", async () => {
		const arrayBuffer = vi.fn();
		const file = {
			name: "large-report.pdf",
			size: MAX_TEXT_EXTRACTION_BYTES + 1,
			arrayBuffer,
		} as unknown as File;

		const result = await extractPromptFileText(file, "application/pdf");
		expect(result).toContain("Extraction skipped:");
		expect(result).toContain("50 MB safe in-browser text extraction limit");
		expect(arrayBuffer).not.toHaveBeenCalled();
	});

	it("streams oversized text/code files without allocating the whole file", async () => {
		const arrayBuffer = vi.fn();
		const stream = new ReadableStream<Uint8Array>({
			start(controller) {
				controller.enqueue(new TextEncoder().encode("large source file content"));
				controller.close();
			},
		});
		const file = {
			name: "large.log",
			size: MAX_TEXT_EXTRACTION_BYTES + 1,
			arrayBuffer,
			stream: vi.fn(() => stream),
		} as unknown as File;

		expect(await extractPromptFileText(file, "text/plain")).toContain("large source file content");
		expect(file.stream).toHaveBeenCalledOnce();
		expect(arrayBuffer).not.toHaveBeenCalled();
	});

	it("returns a readable error stub if the browser cannot read the file", async () => {
		const file = {
			name: "report.txt",
			size: 100,
			arrayBuffer: vi.fn().mockRejectedValue(new Error("read denied")),
		} as unknown as File;

		expect(await extractPromptFileText(file, "text/plain")).toContain("Extraction error: read denied");
	});
});
