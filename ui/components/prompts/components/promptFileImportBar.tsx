import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { MessageContent } from "@/lib/message";
import { Mic, Paperclip, Square } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { fileToAttachment, filesToAttachments, PROMPT_FILE_ACCEPT, PROMPT_FILE_ACCEPT_LABEL } from "../utils/attachment";

function getSupportedAudioMimeType(): string {
	const candidates = ["audio/webm;codecs=opus", "audio/webm", "audio/ogg;codecs=opus", "audio/mp4", "audio/wav"];
	for (const type of candidates) {
		if (typeof MediaRecorder !== "undefined" && MediaRecorder.isTypeSupported(type)) {
			return type;
		}
	}
	return "";
}

function getMicrophoneErrorMessage(error: unknown): string {
	if (error instanceof DOMException) {
		switch (error.name) {
			case "NotAllowedError":
			case "PermissionDeniedError":
				return "Microphone permission denied. Allow microphone access in your browser settings and try again.";
			case "NotFoundError":
			case "DevicesNotFoundError":
				return "No microphone found. Connect a microphone and try again.";
			case "NotReadableError":
			case "TrackStartError":
				return "Microphone is in use by another app. Close other apps using the mic and try again.";
			case "SecurityError":
				return "Microphone blocked on insecure (HTTP) pages. Open the app via HTTPS or localhost.";
			default:
				break;
		}
	}
	if (typeof window !== "undefined" && !window.isSecureContext) {
		return "Microphone requires a secure connection (HTTPS or localhost).";
	}
	return "Microphone access denied or unavailable.";
}

interface PromptFileImportBarProps {
	disabled?: boolean;
	onAttachmentsAdded: (attachments: MessageContent[]) => void;
	className?: string;
}

export function PromptFileImportBar({ disabled, onAttachmentsAdded, className = "" }: PromptFileImportBarProps) {
	const fileInputRef = useRef<HTMLInputElement>(null);
	const [isRecording, setIsRecording] = useState(false);
	const mediaRecorderRef = useRef<MediaRecorder | null>(null);
	const mediaStreamRef = useRef<MediaStream | null>(null);
	const audioChunksRef = useRef<Blob[]>([]);

	const addFiles = useCallback(
		async (files: FileList | File[]) => {
			const attachments = await filesToAttachments(files);
			if (attachments.length > 0) {
				onAttachmentsAdded(attachments);
			}
		},
		[onAttachmentsAdded],
	);

	const stopRecording = useCallback(() => {
		mediaRecorderRef.current?.stop();
		mediaRecorderRef.current = null;
	}, []);

	const cleanupStream = useCallback(() => {
		mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
		mediaStreamRef.current = null;
	}, []);

	useEffect(() => {
		return () => {
			if (mediaRecorderRef.current?.state !== "inactive") {
				mediaRecorderRef.current?.stop();
			}
			cleanupStream();
		};
	}, [cleanupStream]);

	const startRecording = useCallback(async () => {
		if (typeof window !== "undefined" && !window.isSecureContext) {
			toast.error("Microphone requires HTTPS or localhost", {
				description: "Open this page with https:// or use localhost to record voice.",
			});
			return;
		}
		if (!navigator.mediaDevices?.getUserMedia) {
			toast.error("Voice recording is not supported in this browser");
			return;
		}
		if (typeof MediaRecorder === "undefined") {
			toast.error("Audio recording is not supported in this browser");
			return;
		}

		const mimeType = getSupportedAudioMimeType();
		if (!mimeType) {
			toast.error("No supported audio format found in this browser");
			return;
		}

		let stream: MediaStream;
		try {
			try {
				stream = await navigator.mediaDevices.getUserMedia({
					audio: {
						echoCancellation: true,
						noiseSuppression: true,
						autoGainControl: true,
					},
				});
			} catch (constraintErr) {
				// If advanced audio constraints failed (common with Conexant / Realtek laptop drivers), fallback to basic audio: true
				console.warn("Advanced audio constraints rejected by driver, falling back to basic audio: true", constraintErr);
				stream = await navigator.mediaDevices.getUserMedia({ audio: true });
			}
			mediaStreamRef.current = stream;

			let recorder: MediaRecorder;
			try {
				recorder = mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream);
			} catch (recErr) {
				console.warn("MediaRecorder with specified mimeType failed, falling back to browser default:", recErr);
				recorder = new MediaRecorder(stream);
			}
			audioChunksRef.current = [];

			recorder.ondataavailable = (event) => {
				if (event.data.size > 0) {
					audioChunksRef.current.push(event.data);
				}
			};

			recorder.onerror = () => {
				cleanupStream();
				setIsRecording(false);
				mediaRecorderRef.current = null;
				toast.error("Recording failed. Please try again.");
			};

			recorder.onstop = async () => {
				cleanupStream();
				setIsRecording(false);
				mediaRecorderRef.current = null;

				const recordedMimeType = recorder.mimeType || mimeType;
				const blob = new Blob(audioChunksRef.current, { type: recordedMimeType });
				audioChunksRef.current = [];
				if (blob.size === 0) {
					toast.error("No audio captured. Try recording again.");
					return;
				}

				const ext = recordedMimeType.includes("webm")
					? "webm"
					: recordedMimeType.includes("ogg")
						? "ogg"
						: recordedMimeType.includes("mp4")
							? "m4a"
							: "wav";
				const file = new File([blob], `voice-${Date.now()}.${ext}`, { type: recordedMimeType });
				const attachment = await fileToAttachment(file);
				if (attachment) {
					onAttachmentsAdded([attachment]);
					toast.success("Voice recording attached");
				}
			};

			recorder.start(250);
			mediaRecorderRef.current = recorder;
			setIsRecording(true);
			toast.message("Recording...", { description: "Click Stop when finished." });
		} catch (error) {
			cleanupStream();
			setIsRecording(false);
			mediaRecorderRef.current = null;
			toast.error(getMicrophoneErrorMessage(error));
		}
	}, [cleanupStream, onAttachmentsAdded]);

	const toggleRecording = useCallback(() => {
		if (disabled) return;
		if (isRecording) {
			stopRecording();
			return;
		}
		void startRecording();
	}, [disabled, isRecording, startRecording, stopRecording]);

	return (
		<div className={`flex flex-wrap items-center justify-end gap-1.5 ${className}`}>
			<input
				ref={fileInputRef}
				type="file"
				multiple
				accept={PROMPT_FILE_ACCEPT}
				className="hidden"
				onChange={(e) => {
					const files = e.target.files;
					if (files && files.length > 0) {
						void addFiles(files);
					}
					e.target.value = "";
				}}
			/>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						type="button"
						variant="ghost"
						size="icon"
						disabled={disabled}
						onClick={() => fileInputRef.current?.click()}
						className="text-muted-foreground hover:text-foreground h-7 w-7"
						aria-label="Import files"
						data-testid="prompt-import-files-button"
					>
						<Paperclip className="h-3.5 w-3.5" />
					</Button>
				</TooltipTrigger>
				<TooltipContent side="top" className="max-w-xs">
					<p className="font-medium">Import files</p>
					<p className="text-muted-foreground text-xs">{PROMPT_FILE_ACCEPT_LABEL}</p>
				</TooltipContent>
			</Tooltip>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						type="button"
						variant={isRecording ? "destructive" : "ghost"}
						size="icon"
						disabled={disabled}
						onClick={toggleRecording}
						className={
							isRecording
								? "h-7 w-7"
								: "text-muted-foreground hover:text-foreground h-7 w-7"
						}
						aria-label={isRecording ? "Stop recording" : "Record voice"}
						data-testid="prompt-record-voice-button"
					>
						{isRecording ? <Square className="h-3 w-3 fill-current" /> : <Mic className="h-3.5 w-3.5" />}
					</Button>
				</TooltipTrigger>
				<TooltipContent side="top">
					{isRecording ? "Stop recording" : "Record voice"}
				</TooltipContent>
			</Tooltip>
		</div>
	);
}
