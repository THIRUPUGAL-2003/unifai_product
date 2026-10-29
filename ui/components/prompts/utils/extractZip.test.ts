import { beforeEach, describe, expect, it, vi } from "vitest";

const { loadAsync } = vi.hoisted(() => ({ loadAsync: vi.fn() }));
vi.mock("jszip", () => ({ default: { loadAsync } }));

import { extractZipInnerFiles } from "./extractZip";

describe("ZIP attachment extraction limits", () => {
	beforeEach(() => loadAsync.mockReset());

	it("rejects an oversized archive before reading it into memory", async () => {
		const file = {
			name: "large.zip",
			size: 50 * 1024 * 1024 + 1,
			arrayBuffer: vi.fn(),
		} as unknown as File;

		const result = await extractZipInnerFiles(file);
		expect(result.files).toEqual([]);
		expect(result.errors[0]).toContain("50 MB safe archive limit");
		expect(file.arrayBuffer).not.toHaveBeenCalled();
		expect(loadAsync).not.toHaveBeenCalled();
	});

	it("checks each entry's declared expanded size before decompressing it", async () => {
		const async = vi.fn();
		loadAsync.mockResolvedValue({
			files: {
				"docs/large.txt": {
					dir: false,
					_data: { uncompressedSize: 50 * 1024 * 1024 + 1 },
					async,
				},
			},
		});

		const result = await extractZipInnerFiles(new File(["zip"], "docs.zip"));
		expect(result.files).toEqual([]);
		expect(result.errors[0]).toContain("per-file archive limit");
		expect(async).not.toHaveBeenCalled();
	});

	it("imports a normal small ZIP entry", async () => {
		loadAsync.mockResolvedValue({
			files: {
				"folder/readme.md": {
					dir: false,
					_data: { uncompressedSize: 12 },
					async: vi.fn().mockResolvedValue(new Blob(["Hello archive"])),
				},
			},
		});

		const result = await extractZipInnerFiles(new File(["zip"], "docs.zip"));
		expect(result.files).toHaveLength(1);
		expect(result.files[0].name).toBe("readme.md");
		expect(await result.files[0].text()).toBe("Hello archive");
	});
});
