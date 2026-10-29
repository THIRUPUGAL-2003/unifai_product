import { useEffect, useRef, useState } from "react";
import { Code2, Eye, X } from "lucide-react";
import { Markdown } from "@/components/ui/markdown";
import { parseCsvRows, parseMarkdownTables } from "../utils/generateFiles";
import type { StudioEvent, StudioFileKind } from "../utils/fileStudio";

type StudioView = "preview" | "source";

function SpreadsheetPreview({ tables }: { tables: Array<{ title: string; rows: string[][] }> }) {
	const [activeSheet, setActiveSheet] = useState(0);
	const sheet = tables[Math.min(activeSheet, tables.length - 1)];
	const rows = sheet.rows;
	return (
		<div className="flex h-full min-h-0 flex-col bg-white text-zinc-900">
			{tables.length > 1 && (
				<div className="flex shrink-0 gap-1 overflow-x-auto border-b border-zinc-200 px-3 pt-2">
					{tables.map((table, index) => (
						<button
							type="button"
							className={`rounded-t px-3 py-1.5 text-xs ${activeSheet === index ? "border border-zinc-200 border-b-white bg-white font-medium" : "text-zinc-500 hover:bg-zinc-50"}`}
							key={index}
							onClick={() => setActiveSheet(index)}
						>
							{table.title || `Sheet ${index + 1}`}
						</button>
					))}
				</div>
			)}
			<div className="custom-scrollbar min-h-0 flex-1 overflow-auto p-4">
				<div className="mx-auto w-fit min-w-full overflow-hidden rounded border border-zinc-200 shadow-sm">
					<table className="min-w-full border-collapse text-left text-xs">
						<thead className="sticky top-0 bg-emerald-50">
							<tr>
								{rows[0].map((cell, i) => (
									<th className="border border-zinc-200 px-3 py-2 font-semibold" key={i}>
										{cell}
									</th>
								))}
							</tr>
						</thead>
						<tbody>
							{rows.slice(1).map((row, rowIndex) => (
								<tr className={rowIndex % 2 ? "bg-zinc-50" : "bg-white"} key={rowIndex}>
									{rows[0].map((_, colIndex) => (
										<td className="border border-zinc-200 px-3 py-2 align-top" key={colIndex}>
											{row[colIndex] || ""}
										</td>
									))}
								</tr>
							))}
						</tbody>
					</table>
				</div>
			</div>
		</div>
	);
}

function FilePreview({ event }: { event: StudioEvent }) {
	const kind = event.kind;
	if (kind === "pdf" && event.previewUrl) {
		return <iframe className="h-full w-full bg-zinc-200" src={event.previewUrl} title="Generated PDF preview" />;
	}
	if (kind === "xlsx" || kind === "csv") {
		const content = event.previewCsv || event.previewMarkdown || "";
		const csvRows = parseCsvRows(content);
		const markdownTables = parseMarkdownTables(content);
		const tables = csvRows
			? [{ title: "Sheet1", rows: csvRows }]
			: markdownTables.map((table, index) => ({ title: table.title || `Sheet ${index + 1}`, rows: [table.headers, ...table.rows] }));
		if (tables.length) return <SpreadsheetPreview tables={tables} />;
	}
	if (kind === "html" && event.previewHtml) {
		return <iframe className="h-full w-full bg-white" sandbox="" srcDoc={event.previewHtml} title="Generated HTML preview" />;
	}
	if (kind === "svg" && event.previewSvg) {
		return <iframe className="h-full w-full bg-white" sandbox="" srcDoc={event.previewSvg} title="Generated SVG preview" />;
	}
	const content = event.previewMarkdown || event.previewCsv || "";
	return (
		<div className="custom-scrollbar h-full overflow-auto bg-zinc-100 p-5 sm:p-8">
			<article className="mx-auto min-h-full max-w-[720px] bg-white px-7 py-8 text-zinc-900 shadow-md sm:px-12 sm:py-12">
				<div className="mb-5 border-b border-zinc-200 pb-4">
					<p className="text-[10px] font-semibold tracking-[0.16em] text-zinc-400 uppercase">{kindLabel(kind)}</p>
					<h1 className="mt-2 text-2xl leading-tight font-semibold">{event.title || "Generated file"}</h1>
				</div>
				<Markdown content={content} controls={false} className="prose prose-sm max-w-none text-zinc-800" />
			</article>
		</div>
	);
}

function kindLabel(kind?: StudioFileKind): string {
	if (!kind) return "File preview";
	if (kind === "docx") return "Word document";
	if (kind === "xlsx") return "Excel spreadsheet";
	return `${kind.toUpperCase()} preview`;
}

export function FileStudioPanel() {
	const [event, setEvent] = useState<StudioEvent | null>(null);
	const [view, setView] = useState<StudioView>("preview");
	const pdfPreviewUrlRef = useRef<string | null>(null);

	useEffect(() => {
		const revokePdfPreview = () => {
			if (pdfPreviewUrlRef.current) {
				try {
					URL.revokeObjectURL(pdfPreviewUrlRef.current);
				} catch {
					// Ignore stale URL cleanup errors.
				}
			}
			pdfPreviewUrlRef.current = null;
		};
		const onStudio = (ev: Event) => {
			const detail = (ev as CustomEvent<StudioEvent>).detail;
			if (!detail) return;
			if (detail.phase === "close") {
				revokePdfPreview();
				setEvent(null);
				return;
			}
			if (detail.phase === "open") {
				revokePdfPreview();
				setView("preview");
			}
			setEvent((prev) => ({ ...prev, ...detail }));
		};
		const onPdfPreview = (ev: Event) => {
			const detail = (ev as CustomEvent<{ blob?: Blob; filename?: string }>).detail;
			if (!detail?.blob) return;
			revokePdfPreview();
			let previewUrl: string;
			try {
				previewUrl = URL.createObjectURL(detail.blob);
			} catch (error) {
				setEvent((prev) => ({
					...prev,
					phase: "done",
					kind: "pdf",
					title: detail.filename || "Generated PDF",
					previewUrl: undefined,
					log: `PDF is ready; inline preview failed: ${error instanceof Error ? error.message : String(error)}`,
				}));
				return;
			}
			pdfPreviewUrlRef.current = previewUrl;
			setView("preview");
			setEvent((prev) => ({
				...prev,
				phase: "done",
				kind: "pdf",
				title: detail.filename || "Generated PDF",
				previewUrl,
				log: "PDF generated and ready to preview",
			}));
		};
		window.addEventListener("prompt-repo:studio", onStudio);
		window.addEventListener("prompt-repo:pdf-preview", onPdfPreview);
		return () => {
			window.removeEventListener("prompt-repo:studio", onStudio);
			window.removeEventListener("prompt-repo:pdf-preview", onPdfPreview);
			revokePdfPreview();
		};
	}, []);

	if (!event || event.phase === "close") return null;

	return (
		<aside className="absolute top-0 right-0 bottom-0 z-40 flex w-[min(440px,48%)] flex-col border-l border-zinc-800 bg-zinc-950 text-zinc-100 shadow-2xl">
			<div className="flex items-center justify-between border-b border-zinc-800 px-3 py-2">
				<div className="min-w-0">
					<p className="text-[11px] tracking-wide text-zinc-400 uppercase">{event.kind ? kindLabel(event.kind) : "File studio"}</p>
					<p className="truncate text-sm font-medium">{event.title || "File studio"}</p>
				</div>
				<button
					type="button"
					className="rounded p-1 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
					onClick={() => {
						if (pdfPreviewUrlRef.current) {
							try {
								URL.revokeObjectURL(pdfPreviewUrlRef.current);
							} catch {
								// Ignore stale URL cleanup errors.
							}
						}
						pdfPreviewUrlRef.current = null;
						setEvent(null);
					}}
					aria-label="Close file studio"
				>
					<X className="size-4" />
				</button>
			</div>
			{event.kind && (
				<div className="flex gap-1 border-b border-zinc-800 px-3 py-2">
					<button
						type="button"
						className={`rounded px-2 py-1 text-xs ${view === "preview" ? "bg-zinc-800 text-white" : "text-zinc-400 hover:text-white"}`}
						onClick={() => setView("preview")}
					>
						<Eye className="mr-1 inline size-3.5" />
						Preview
					</button>
					<button
						type="button"
						className={`rounded px-2 py-1 text-xs ${view === "source" ? "bg-zinc-800 text-white" : "text-zinc-400 hover:text-white"}`}
						onClick={() => setView("source")}
					>
						<Code2 className="mr-1 inline size-3.5" />
						Content
					</button>
				</div>
			)}
			{view === "preview" && event.kind ? (
				<div className="min-h-0 flex-1 overflow-hidden">
					<FilePreview event={event} />
				</div>
			) : (
				<pre className="custom-scrollbar flex-1 overflow-auto px-3 py-3 font-mono text-[11px] leading-5 whitespace-pre-wrap text-emerald-200">
					{event.kind
						? event.previewCsv || event.previewSvg || event.previewHtml || event.previewMarkdown || event.code || ""
						: event.code || ""}
				</pre>
			)}
			<div className="border-t border-zinc-800 px-3 py-2 text-[11px] text-zinc-300">
				{event.phase === "open"
					? "Preparing…"
					: event.phase === "code"
						? "Writing…"
						: event.phase === "run"
							? "Generating…"
							: event.phase === "error"
								? "Stopped"
								: "Finished"}
				{event.log ? ` · ${event.log}` : ""}
			</div>
		</aside>
	);
}
