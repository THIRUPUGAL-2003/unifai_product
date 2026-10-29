import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), loading: vi.fn(), success: vi.fn() } }));
vi.mock("@/lib/constants/config", () => ({ COMPANY_LOGO: "", PRODUCT_NAME: "Test" }));
vi.mock("@/lib/utils/tableExport", () => ({ loadLogoAsset: vi.fn().mockResolvedValue(null) }));

import { exportToWordDocx, parseCsvRows } from "./generateFiles";

describe("generated file helpers", () => {
	afterEach(() => vi.unstubAllGlobals());

	it("parses quoted CSV cells, commas, quotes, and embedded newlines", () => {
		expect(parseCsvRows('Name,Note\n"Chennai","First, second"\n"Quote ""yes""","line one\nline two"')).toEqual([
			["Name", "Note"],
			["Chennai", "First, second"],
			['Quote "yes"', "line one\nline two"],
		]);
	});

	it("returns a real OOXML DOCX package and preserves non-English text", async () => {
		let generatedBlob: Blob | undefined;
		const anchor = { href: "", download: "", click: vi.fn() };
		vi.stubGlobal("document", {
			createElement: vi.fn(() => anchor),
			body: { appendChild: vi.fn(), removeChild: vi.fn() },
		});
		vi.stubGlobal("URL", {
			createObjectURL: vi.fn((blob: Blob) => {
				generatedBlob = blob;
				return "blob:generated-docx";
			}),
			revokeObjectURL: vi.fn(),
		});

		const content = "# அறிக்கை\n\nவணக்கம் உலகம்.\n\n| பெயர் | தொகை |\n| --- | --- |\n| சென்னை | 42 |";
		await expect(exportToWordDocx(content, "அறிக்கை")).resolves.toBe(true);

		expect(generatedBlob?.type).toBe("application/vnd.openxmlformats-officedocument.wordprocessingml.document");
		const JSZipMod = await import("jszip");
		const JSZip = JSZipMod.default ?? JSZipMod;
		const packageZip = await JSZip.loadAsync(await (generatedBlob as Blob).arrayBuffer());
		const documentXml = await packageZip.file("word/document.xml")?.async("string");
		expect(documentXml).toContain("<w:document");
		expect(documentXml).toContain("வணக்கம் உலகம்");
		expect(documentXml).toContain("<w:tbl>");
		expect(packageZip.file("[Content_Types].xml")).toBeTruthy();
	});
});
