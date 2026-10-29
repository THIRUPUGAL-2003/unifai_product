import { afterEach, describe, expect, it, vi } from "vitest";
import { ocrDocument } from "./ocrImage";

describe("image OCR requests", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("parses multi-page OCR output and labels the requested pages", async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({ pages: [{ markdown: "Invoice 123" }, { markdown: "Total: $42" }] }),
		});
		vi.stubGlobal("fetch", fetchMock);

		const result = await ocrDocument(
			{ type: "document_url", document_url: "data:application/pdf;base64,JVBERiQ=" },
			{ models: ["mistral-ocr-latest"], pages: [2, 4] },
		);

		expect(result).toEqual({ text: "[Page 2]\nInvoice 123\n\n[Page 4]\nTotal: $42", model: "mistral-ocr-latest" });
		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toMatchObject({
			model: "mistral-ocr-latest",
			document: { type: "document_url", document_url: "data:application/pdf;base64,JVBERiQ=" },
			pages: [2, 4],
		});
	});

	it("stops retrying on a permanent image request error", async () => {
		const fetchMock = vi.fn().mockResolvedValue({
			ok: false,
			status: 413,
			json: async () => ({ error: { message: "Image payload is too large" } }),
		});
		vi.stubGlobal("fetch", fetchMock);
		const warn = vi.spyOn(console, "warn").mockImplementation(() => {});

		const result = await ocrDocument(
			{ type: "image_url", image_url: "data:image/tiff;base64,AA==" },
			{ models: ["model-a", "model-b", "model-c"] },
		);

		expect(result).toBeNull();
		expect(fetchMock).toHaveBeenCalledTimes(1);
		warn.mockRestore();
	});

	it("propagates cancellation without trying another model alias", async () => {
		const abortError = Object.assign(new Error("Request aborted"), { name: "AbortError" });
		const fetchMock = vi.fn().mockRejectedValue(abortError);
		vi.stubGlobal("fetch", fetchMock);

		await expect(
			ocrDocument({ type: "image_url", image_url: "data:image/png;base64,iVBORw0KGgo=" }, { models: ["model-a", "model-b"] }),
		).rejects.toBe(abortError);
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});
});
