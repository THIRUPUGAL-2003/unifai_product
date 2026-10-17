import { MessageContent } from "@/lib/message";
import { getAttachmentDisplayName } from "../../utils/attachment";
import { AttachmentPreviewModal, getFileTypeBadge } from "./attachmentPreviewModal";
import { Eye, FileIcon, Mic, Video, XIcon } from "lucide-react";
import { useState } from "react";

/** Helper to extract extension */
function getFileExtension(name: string): string {
	if (!name) return "";
	const parts = name.split(".");
	return parts.length > 1 ? parts.pop()?.toLowerCase() || "" : "";
}

/**
 * Renders a compact, interactive badge for a single attachment with:
 * - Color-coded category tag (PDF, XLSX, DOCX, PY, IMG, ZIP, etc.)
 * - Click-to-preview lightbox modal (ChatGPT style)
 * - Inline remove control
 */
export function AttachmentBadge({
	attachment,
	onRemove,
}: {
	attachment: MessageContent;
	onRemove: () => void;
}) {
	const [previewOpen, setPreviewOpen] = useState(false);
	const isImage = attachment.type === "image_url";
	const isAudio = attachment.type === "input_audio";
	const name = getAttachmentDisplayName(attachment);
	const ext = getFileExtension(name);
	const badge = getFileTypeBadge(ext, attachment.type, name);
	const isVideo = attachment.type === "video_url" || !!attachment.video_url || badge.label === "VIDEO";
	const BadgeIcon = badge.icon;

	return (
		<>
			<div
				onClick={() => setPreviewOpen(true)}
				title="Click to preview file content"
				className="group/att bg-card hover:bg-accent/40 hover:border-primary/40 relative flex items-center gap-2 rounded-md border border-border/70 px-2 py-1 text-xs shadow-xs transition-all cursor-pointer select-none"
			>
				{isImage && attachment.image_url?.url ? (
					<>
						<div className="relative h-7 w-7 rounded overflow-hidden shrink-0 border border-border/50 bg-muted">
							{/* eslint-disable-next-line @next/next/no-img-element */}
							<img src={attachment.image_url.url} alt="attachment" className="h-full w-full object-cover" />
						</div>
						<div className="flex flex-col min-w-0">
							<span className="text-foreground font-medium max-w-[130px] truncate text-[11px]">
								{attachment.image_url.filename || "Image"}
							</span>
							<span className="text-[10px] text-muted-foreground flex items-center gap-1">
								<span className="text-sky-500 font-semibold">IMG</span> • Click to view
							</span>
						</div>
					</>
				) : isVideo ? (
					<>
						<div className="h-7 w-7 rounded flex items-center justify-center shrink-0 bg-violet-500/10 text-violet-600 dark:text-violet-400 border border-violet-500/20">
							<Video className="size-3.5" />
						</div>
						<div className="flex flex-col min-w-0">
							<span className="text-foreground font-medium max-w-[130px] truncate text-[11px]" title={name}>
								{name || "Video"}
							</span>
							<span className="text-[10px] text-muted-foreground flex items-center gap-1">
								<span className="text-violet-500 font-semibold">VIDEO</span> • Click to play
							</span>
						</div>
					</>
				) : isAudio ? (
					<>
						<div className="h-7 w-7 rounded flex items-center justify-center shrink-0 bg-rose-500/10 text-rose-500 border border-rose-500/20">
							<Mic className="size-3.5" />
						</div>
						<div className="flex flex-col min-w-0">
							<span className="text-foreground font-medium max-w-[130px] truncate text-[11px]">
								{name || "Voice recording"}
							</span>
							<span className="text-[10px] text-muted-foreground flex items-center gap-1">
								<span className="text-rose-500 font-semibold">VOICE</span> • Audio transcript
							</span>
						</div>
					</>
				) : (
					<>
						<div className={`h-7 w-7 rounded flex items-center justify-center shrink-0 border ${badge.bg}`}>
							<BadgeIcon className="size-3.5" />
						</div>
						<div className="flex flex-col min-w-0">
							<span className="text-foreground font-medium max-w-[140px] truncate text-[11px]" title={name}>
								{name}
							</span>
							<span className="text-[10px] text-muted-foreground flex items-center gap-1">
								<span className="font-semibold opacity-90">{badge.label}</span> • Click to inspect
							</span>
						</div>
					</>
				)}

				<div className="flex items-center gap-0.5 ml-1">
					<button
						onClick={(e) => {
							e.stopPropagation();
							onRemove();
						}}
						title="Remove attachment"
						className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive cursor-pointer rounded-full p-1 transition-colors"
						type="button"
					>
						<XIcon className="size-3" />
					</button>
				</div>
			</div>

			<AttachmentPreviewModal
				attachment={attachment}
				open={previewOpen}
				onOpenChange={setPreviewOpen}
				onRemove={onRemove}
			/>
		</>
	);
}

/**
 * Renders a list of attachment previews for sent messages in the chat history.
 * Every card is interactive: clicking it opens the full Lightbox Preview Modal.
 */
export function AttachmentDisplay({
	attachments,
	editable,
	onRemoveAttachment,
}: {
	attachments: MessageContent[];
	editable?: boolean;
	onRemoveAttachment?: (index: number) => void;
}) {
	const [activePreview, setActivePreview] = useState<MessageContent | null>(null);

	if (attachments.length === 0) return null;

	return (
		<>
			<div className="mt-2 flex flex-wrap gap-2">
				{attachments.map((att, i) => {
					const name = getAttachmentDisplayName(att);
					const ext = getFileExtension(name);
					const badge = getFileTypeBadge(ext, att.type);
					const BadgeIcon = badge.icon;

					if (att.type === "image_url" && att.image_url?.url) {
						return (
							<div
								key={i}
								onClick={() => setActivePreview(att)}
								className="group/att relative max-w-full cursor-pointer select-none"
							>
								{/* eslint-disable-next-line jsx-a11y/alt-text */}
								<img
									src={att.image_url.url}
									alt={att.image_url.filename || "attached image"}
									title="Click to view full image"
									className="max-h-56 max-w-full rounded-md border border-border object-contain sm:max-w-xs shadow-xs bg-muted/20 hover:ring-2 hover:ring-primary/40 transition-all"
								/>
								<div className="flex items-center justify-between text-[11px] text-muted-foreground truncate max-w-xs mt-1 px-1">
									<span className="truncate">{att.image_url.filename || "Attached Image"}</span>
									<span className="text-[10px] text-sky-500 hover:underline flex items-center gap-0.5 shrink-0">
										<Eye className="size-2.5" /> View
									</span>
								</div>
								{editable && onRemoveAttachment && (
									<button
										onClick={(e) => {
											e.stopPropagation();
											onRemoveAttachment(i);
										}}
										className="bg-background/90 text-muted-foreground hover:bg-card hover:text-destructive absolute -top-1.5 -right-1.5 cursor-pointer rounded-full border p-1 opacity-0 transition-opacity group-hover/att:opacity-100 shadow-xs"
									>
										<XIcon className="size-3" />
									</button>
								)}
							</div>
						);
					}

					if ((att.type === "video_url" || att.video_url) && att.video_url?.url) {
						return (
							<div
								key={i}
								className="group/att relative max-w-sm rounded-lg overflow-hidden border border-border shadow-xs bg-black"
							>
								<video
									src={att.video_url.url}
									controls
									playsInline
									preload="metadata"
									className="max-h-56 w-full object-contain bg-black"
								/>
								<div className="flex items-center justify-between text-[11px] text-muted-foreground p-2 bg-card/90 border-t border-border/40">
									<span className="truncate font-medium text-foreground max-w-[180px]">{att.video_url.filename || "Video"}</span>
									<button
										onClick={() => setActivePreview(att)}
										className="text-[10px] text-violet-500 hover:underline flex items-center gap-1 shrink-0 cursor-pointer"
										title="View video details and transcript"
									>
										<Eye className="size-2.5" /> Details
									</button>
								</div>
								{editable && onRemoveAttachment && (
									<button
										onClick={(e) => {
											e.stopPropagation();
											onRemoveAttachment(i);
										}}
										className="bg-background/90 text-muted-foreground hover:bg-card hover:text-destructive absolute top-1.5 right-1.5 cursor-pointer rounded-full border p-1 opacity-0 transition-opacity group-hover/att:opacity-100 shadow-xs z-10"
									>
										<XIcon className="size-3" />
									</button>
								)}
							</div>
						);
					}

					if (att.type === "input_audio") {
						const format = att.input_audio?.format || "wav";
						const dataUrl = `data:audio/${format};base64,${att.input_audio?.data || ""}`;
						return (
							<div
								key={i}
								className="group/att bg-card relative flex w-full max-w-md items-center gap-2 rounded-md border border-border px-3 py-2 shadow-xs"
							>
								<audio controls className="h-8 w-full min-w-0 grow">
									<source src={dataUrl} type={`audio/${format}`} />
								</audio>
								<button
									onClick={() => setActivePreview(att)}
									title="View Transcript"
									className="text-xs text-muted-foreground hover:text-primary px-2 py-1 rounded border hover:bg-muted/50 shrink-0"
								>
									Transcript
								</button>
								{editable && onRemoveAttachment && (
									<button
										onClick={(e) => {
											e.stopPropagation();
											onRemoveAttachment(i);
										}}
										className="bg-background text-muted-foreground hover:text-destructive rounded-full border p-1 shadow-xs"
									>
										<XIcon className="size-3" />
									</button>
								)}
							</div>
						);
					}

					return (
						<div
							key={i}
							onClick={() => setActivePreview(att)}
							title="Click to preview extracted content"
							className="group/att bg-card hover:bg-accent/40 hover:border-primary/40 relative flex max-w-full items-center gap-2.5 rounded-md border border-border px-3 py-2 text-xs shadow-xs cursor-pointer select-none transition-all"
						>
							<div className={`h-6 w-6 rounded flex items-center justify-center shrink-0 border ${badge.bg}`}>
								<BadgeIcon className="size-3" />
							</div>
							<div className="flex flex-col min-w-0">
								<span className="font-medium text-foreground truncate max-w-[160px] sm:max-w-[220px]" title={name}>
									{name}
								</span>
								<span className="text-[10px] text-muted-foreground flex items-center gap-1">
									<span className="font-semibold">{badge.label}</span> • Click to inspect
								</span>
							</div>
							{editable && onRemoveAttachment && (
								<button
									onClick={(e) => {
										e.stopPropagation();
										onRemoveAttachment(i);
									}}
									className="bg-background text-muted-foreground hover:text-destructive ml-1 rounded-full border p-1 opacity-0 transition-opacity group-hover/att:opacity-100 shadow-xs"
								>
									<XIcon className="size-3" />
								</button>
							)}
						</div>
					);
				})}
			</div>

			<AttachmentPreviewModal
				attachment={activePreview}
				open={!!activePreview}
				onOpenChange={(isOpen) => {
					if (!isOpen) setActivePreview(null);
				}}
			/>
		</>
	);
}