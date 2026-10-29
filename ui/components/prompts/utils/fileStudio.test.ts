import { describe, expect, it, vi } from "vitest";

const { exportToPdf } = vi.hoisted(() => ({ exportToPdf: vi.fn() }));
vi.mock("./generateFiles", () => ({
	exportToExcel: vi.fn(),
	exportToPdf,
	exportToWordDocx: vi.fn(),
	triggerBlobDownload: vi.fn(),
}));

import { runDocumentStudio } from "./fileStudio";

describe("document studio reliability", () => {
	it("does not turn an unusable model response into a fake file", async () => {
		const askModel = vi.fn().mockResolvedValue("I could not create that file.");

		await expect(runDocumentStudio({ kind: "pdf", instruction: "Generate a report PDF", context: "", askModel })).rejects.toThrow(
			"did not return usable file content",
		);
		expect(askModel).toHaveBeenCalledTimes(2);
		expect(exportToPdf).not.toHaveBeenCalled();
	});

	it("does not report success when the file exporter fails", async () => {
		exportToPdf.mockReset().mockResolvedValue(false);
		const askModel = vi.fn().mockResolvedValue(JSON.stringify({ title: "Report", content: "# Report\n\nA completed report." }));

		await expect(runDocumentStudio({ kind: "pdf", instruction: "Generate a report PDF", context: "", askModel })).rejects.toThrow(
			"PDF could not be generated",
		);
	});
});
