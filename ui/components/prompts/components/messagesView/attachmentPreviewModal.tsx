import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { MessageContent } from "@/lib/message";
import { getAttachmentDisplayName } from "../../utils/attachment";
import {
	Archive,
	Check,
	Copy,
	Download,
	ExternalLink,
	FileCode,
	FileIcon,
	FileSpreadsheet,
	FileText,
	Headphones,
	Image as ImageIcon,
	Mic,
	ZoomIn,
} from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

export interface AttachmentPreviewModalProps {
	attachment: MessageContent | null;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onRemove?: () => void;
}

/** Helper to extract file extension from filename or attachment text */
function getFileExtension(name: string): string {
	if (!name) return "";
	const parts = name.split(".");
	return parts.length > 1 ? parts.pop()?.toLowerCase() || "" : "";
}

/** Formatter helper for badge colors based on file category */
export function getFileTypeBadge(ext: string, type: string, name?: string) {
	const e = ext.toLowerCase();
	if (name?.startsWith("OCR:") || e === "ocr") {
		return { label: "OCR", bg: "bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/30", icon: FileText };
	}
	if (["pdf"].includes(e)) {
		return { label: "PDF", bg: "bg-red-500/10 text-red-600 dark:text-red-400 border-red-500/30", icon: FileText };
	}
	if (["xlsx", "xls", "csv", "tsv", "ods"].includes(e)) {
		return { label: e.toUpperCase(), bg: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/30", icon: FileSpreadsheet };
	}
	if (["docx", "doc", "dot", "rtf", "odt"].includes(e)) {
		return { label: e.toUpperCase(), bg: "bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/30", icon: FileText };
	}
	if (["pptx", "ppt", "odp", "key"].includes(e)) {
		return { label: e.toUpperCase(), bg: "bg-orange-500/10 text-orange-600 dark:text-orange-400 border-orange-500/30", icon: FileText };
	}
	if (["py", "js", "ts", "jsx", "tsx", "java", "cpp", "c", "cs", "go", "rs", "php", "rb", "swift", "kt", "sql", "sh", "ps1", "html", "css", "json", "yaml", "yml", "ipynb"].includes(e)) {
		return { label: e.toUpperCase(), bg: "bg-purple-500/10 text-purple-600 dark:text-purple-400 border-purple-500/30", icon: FileCode };
	}
	if (["zip", "tar", "gz", "tgz", "bz2", "xz", "7z", "rar"].includes(e)) {
		return { label: "ZIP", bg: "bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/30", icon: Archive };
	}
	if (type === "image_url" || ["png", "jpg", "jpeg", "webp", "gif", "svg", "bmp"].includes(e)) {
		return { label: "IMG", bg: "bg-sky-500/10 text-sky-600 dark:text-sky-400 border-sky-500/30", icon: ImageIcon };
	}
	if (type === "input_audio" || ["mp3", "wav", "m4a", "ogg", "flac"].includes(e)) {
		return { label: "VOICE", bg: "bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/30", icon: Mic };
	}
	return { label: e ? e.toUpperCase() : "FILE", bg: "bg-muted text-muted-foreground border-border", icon: FileIcon };
}

export function AttachmentPreviewModal({
	attachment,
	open,
	onOpenChange,
	onRemove,
}: AttachmentPreviewModalProps) {
	const [copied, setCopied] = useState(false);

	const parsed = useMemo(() => {
		if (!attachment) return null;
		const name = getAttachmentDisplayName(attachment) || "Attachment";
		const ext = getFileExtension(name);
		const badge = getFileTypeBadge(ext, attachment.type);

		let rawExtractedText = "";
		let isVoice = false;
		let isImage = attachment.type === "image_url";
		let isAudio = attachment.type === "input_audio";

		if (attachment.type === "image_url" && attachment.image_url?.extracted_text) {
			rawExtractedText = attachment.image_url.extracted_text.trim();
		}

		if (attachment.type === "text" && attachment.text) {
			const t = attachment.text;
			if (t.startsWith("Voice transcript")) {
				isVoice = true;
				rawExtractedText = t.replace(/^Voice transcript\s*\([^)]*\):\s*/i, "").trim();
			} else if (t.includes("--- extracted content ---")) {
				const parts = t.split("--- extracted content ---");
				rawExtractedText = (parts[1] || parts[0] || "").trim();
			} else if (t.includes("--- OCR extracted content ---")) {
				const parts = t.split("--- OCR extracted content ---");
				rawExtractedText = (parts[1] || parts[0] || "").trim();
			} else {
				rawExtractedText = t.trim();
			}
		}

		const charCount = rawExtractedText.length;
		const lineCount = rawExtractedText ? rawExtractedText.split("\n").length : 0;
		const wordCount = rawExtractedText ? (rawExtractedText.match(/\S+/g) || []).length : 0;

		return {
			name,
			ext,
			badge,
			rawExtractedText,
			isVoice,
			isImage,
			isAudio,
			charCount,
			lineCount,
			wordCount,
			imageUrl: attachment.image_url?.url || "",
			imageExtractedText: attachment.image_url?.extracted_text || "",
			audioFormat: attachment.input_audio?.format || "wav",
			audioDataUrl: attachment.input_audio?.data ? `data:audio/${attachment.input_audio.format || "wav"};base64,${attachment.input_audio.data}` : "",
		};
	}, [attachment]);

	if (!attachment || !parsed) return null;

	const handleCopyText = async () => {
		if (!parsed.rawExtractedText) return;
		try {
			await navigator.clipboard.writeText(parsed.rawExtractedText);
			setCopied(true);
			toast.success("Extracted text copied to clipboard");
			setTimeout(() => setCopied(false), 2000);
		} catch {
			toast.error("Failed to copy to clipboard");
		}
	};

	const handleDownload = () => {
		if (parsed.isImage && parsed.imageUrl) {
			const a = document.createElement("a");
			a.href = parsed.imageUrl;
			a.download = parsed.name || "attachment-image.png";
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			toast.success(`Downloaded ${parsed.name}`);
			return;
		}
		if (parsed.rawExtractedText) {
			const blob = new Blob([parsed.rawExtractedText], { type: "text/plain;charset=utf-8" });
			const url = URL.createObjectURL(blob);
			const a = document.createElement("a");
			a.href = url;
			a.download = `${parsed.name.replace(/\.[^.]+$/, "")}_extracted.txt`;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			URL.revokeObjectURL(url);
			toast.success(`Saved extracted text for ${parsed.name}`);
		}
	};

	const BadgeIcon = parsed.badge.icon;

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				className="sm:max-w-3xl max-h-[85vh] flex flex-col p-0 gap-0 overflow-hidden bg-background border shadow-2xl rounded-xl"
				disableOutsideClick={false}
			>
				{/* Top Bar Header */}
				<DialogHeader className="px-5 py-3.5 border-b flex flex-row items-center justify-between gap-3 bg-muted/30">
					<div className="flex items-center gap-2.5 min-w-0">
						<span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-semibold border ${parsed.badge.bg}`}>
							<BadgeIcon className="h-3 w-3" />
							{parsed.badge.label}
						</span>
						<div className="min-w-0">
							<DialogTitle className="text-sm font-semibold truncate text-foreground" title={parsed.name}>
								{parsed.name}
							</DialogTitle>
							<DialogDescription className="text-[11px] text-muted-foreground flex items-center gap-2 mt-0.5">
								{parsed.isImage ? (
									<span>Visual Image Attachment for Multimodal AI</span>
								) : parsed.isVoice || parsed.isAudio ? (
									<span>Voice Recording & Speech Transcript</span>
								) : (
									<span>
										{parsed.lineCount > 0 ? `${parsed.lineCount.toLocaleString()} lines • ` : ""}
										{parsed.charCount.toLocaleString()} chars • {parsed.wordCount.toLocaleString()} words extracted
									</span>
								)}
							</DialogDescription>
						</div>
					</div>

					<div className="flex items-center gap-1.5 shrink-0 pr-6">
						{parsed.rawExtractedText && (
							<Button
								variant="outline"
								size="sm"
								onClick={handleCopyText}
								className="h-8 gap-1.5 text-xs font-normal"
								title="Copy extracted text to clipboard"
							>
								{copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
								<span>{copied ? "Copied" : "Copy Text"}</span>
							</Button>
						)}
						<Button
							variant="outline"
							size="sm"
							onClick={handleDownload}
							className="h-8 gap-1.5 text-xs font-normal"
							title="Download file or extracted content"
						>
							<Download className="h-3.5 w-3.5" />
							<span>Download</span>
						</Button>
					</div>
				</DialogHeader>

				{/* Preview Body */}
				<div className="flex-1 overflow-y-auto p-5 custom-scrollbar min-h-[220px]">
					{parsed.isImage && parsed.imageUrl ? (
						<div className="flex flex-col items-center justify-center p-4 bg-muted/20 rounded-lg border">
							{/* eslint-disable-next-line @next/next/no-img-element */}
							<img
								src={parsed.imageUrl}
								alt={parsed.name}
								className="max-h-[60vh] max-w-full rounded-md object-contain shadow-md"
							/>
							<div className="mt-3 text-xs text-muted-foreground flex items-center gap-2">
								<ZoomIn className="h-3.5 w-3.5" />
								<span>Full multimodal resolution sent to Vision-capable models</span>
							</div>
							{parsed.imageExtractedText ? (
								<div className="mt-4 w-full text-left">
									<h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
										Extracted text
									</h4>
									<div className="p-4 rounded-lg border bg-card text-foreground font-mono text-xs leading-relaxed whitespace-pre-wrap overflow-x-auto max-h-[40vh] select-text">
										{parsed.imageExtractedText}
									</div>
								</div>
							) : null}
						</div>
					) : parsed.isAudio && parsed.audioDataUrl ? (
						<div className="flex flex-col gap-4">
							<div className="p-4 rounded-lg border bg-muted/20 flex flex-col items-center gap-3">
								<Headphones className="h-8 w-8 text-rose-500 animate-pulse" />
								<audio controls className="w-full max-w-md">
									<source src={parsed.audioDataUrl} type={`audio/${parsed.audioFormat}`} />
								</audio>
							</div>
							{parsed.rawExtractedText && (
								<div>
									<h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
										Whisper Speech-To-Text Transcript
									</h4>
									<div className="p-4 rounded-lg border bg-card text-foreground text-sm font-sans leading-relaxed whitespace-pre-wrap select-text">
										{parsed.rawExtractedText}
									</div>
								</div>
							)}
						</div>
					) : parsed.rawExtractedText ? (
						<div className="flex flex-col gap-2">
							<div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
								<span className="font-medium text-foreground">Extracted Content Preview:</span>
								<span>Sent to AI model inside unified context</span>
							</div>
							<div className="p-4 rounded-lg border bg-muted/20 text-foreground font-mono text-xs leading-relaxed whitespace-pre-wrap overflow-x-auto max-h-[55vh] select-text">
								{parsed.rawExtractedText}
							</div>
						</div>
					) : (
						<div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
							<FileIcon className="h-10 w-10 mb-2 opacity-50" />
							<p className="text-sm">No preview content available for this attachment.</p>
						</div>
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
}
