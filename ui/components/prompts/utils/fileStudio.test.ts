import { beforeEach, describe, expect, it, vi } from "vitest";

const { exportToPdf, triggerBlobDownload } = vi.hoisted(() => ({ exportToPdf: vi.fn(), triggerBlobDownload: vi.fn() }));
vi.mock("./generateFiles", () => ({
	exportToExcel: vi.fn(),
	exportToPdf,
	exportToWordDocx: vi.fn(),
	triggerBlobDownload,
}));

import { runDocumentStudio } from "./fileStudio";

describe("document studio reliability", () => {
	beforeEach(() => {
		exportToPdf.mockReset().mockResolvedValue(true);
		triggerBlobDownload.mockReset();
	});

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

	it("propagates cancellation instead of reporting a generation failure", async () => {
		const abortError = Object.assign(new Error("Cancelled"), { name: "AbortError" });
		const askModel = vi.fn().mockRejectedValue(abortError);

		await expect(runDocumentStudio({ kind: "pdf", instruction: "Generate a report PDF", context: "", askModel })).rejects.toBe(abortError);
		expect(exportToPdf).not.toHaveBeenCalled();
	});

	it("rejects unsafe SVG output instead of downloading it", async () => {
		const askModel = vi.fn().mockResolvedValue(
			JSON.stringify({ title: "Artwork", svg: '<svg><image onerror="alert(1)"/></svg>' }),
		);

		await expect(runDocumentStudio({ kind: "svg", instruction: "Create an SVG", context: "", askModel })).rejects.toThrow(
			"valid SVG",
		);
		expect(triggerBlobDownload).not.toHaveBeenCalled();
	});
});
