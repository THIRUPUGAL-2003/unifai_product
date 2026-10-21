import { ScrollArea } from "@/components/ui/scrollArea";
import { MessagesView } from "../components/messagesView/rootMessageView";
import { FileStudioPanel } from "./fileStudioPanel";
import { NewMessageInputView } from "../components/newMessageInputView";
import { UploadCloud } from "lucide-react";
import { useCallback, useRef, useState } from "react";

export function PlaygroundPanel() {
	const [isDragging, setIsDragging] = useState(false);
	const dragCounterRef = useRef(0);

	const handleDragEnter = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
		dragCounterRef.current++;
		if (e.dataTransfer.types.includes("Files")) {
			setIsDragging(true);
		}
	}, []);

	const handleDragLeave = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
		dragCounterRef.current--;
		if (dragCounterRef.current <= 0) {
			dragCounterRef.current = 0;
			setIsDragging(false);
		}
	}, []);

	const handleDragOver = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
	}, []);

	const handleDrop = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
		dragCounterRef.current = 0;
		setIsDragging(false);

		const files = e.dataTransfer.files;
		if (files && files.length > 0) {
			window.dispatchEvent(
				new CustomEvent("prompt-repo:attach-files", {
					detail: Array.from(files),
				})
			);
		}
	}, []);

	return (
		<div
			className="custom-scrollbar relative flex h-full min-h-0 flex-1 flex-col overflow-hidden overscroll-none"
			onDragEnter={handleDragEnter}
			onDragLeave={handleDragLeave}
			onDragOver={handleDragOver}
			onDrop={handleDrop}
		>
			{isDragging && (
				<div className="absolute inset-0 z-50 flex flex-col items-center justify-center bg-background/85 backdrop-blur-md border-2 border-dashed border-primary animate-in fade-in-0 duration-200 pointer-events-none select-none">
					<div className="flex flex-col items-center gap-3 p-8 text-center max-w-md rounded-2xl bg-card/90 border border-primary/20 shadow-2xl">
						<div className="p-4 rounded-full bg-primary/10 text-primary border border-primary/30 shadow-md">
							<UploadCloud className="h-10 w-10 animate-bounce" />
						</div>
						<div>
							<h3 className="text-base font-semibold text-foreground">Drop files anywhere to attach</h3>
							<p className="text-xs text-muted-foreground mt-1 leading-relaxed">
								Supports Code, Images, PDFs, Word, Excel, PowerPoint, Audio, and Zip archives
							</p>
						</div>
					</div>
				</div>
			)}
			<FileStudioPanel />
			<ScrollArea className="flex-1 min-h-0 scroll-mb-12 overflow-y-auto" viewportClassName="no-table">
				<MessagesView />
			</ScrollArea>
			<NewMessageInputView />
		</div>
	);
}
