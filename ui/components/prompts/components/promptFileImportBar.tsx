import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { MessageContent } from "@/lib/message";
import { Check, Mic, Paperclip, Square, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { fileToAttachment, filesToAttachments, PROMPT_FILE_ACCEPT, PROMPT_FILE_ACCEPT_LABEL } from "../utils/attachment";
import { transcribeAudioFile, voiceTranscriptAttachment } from "../utils/transcribeAudio";

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

function formatDuration(seconds: number): string {
	const mins = Math.floor(seconds / 60);
	const secs = seconds % 60;
	return `${mins.toString().padStart(2, "0")}:${secs.toString().padStart(2, "0")}`;
}

interface PromptFileImportBarProps {
	disabled?: boolean;
	onAttachmentsAdded: (attachments: MessageContent[]) => void;
	onTextExtracted?: (text: string) => void;
	className?: string;
}

export function PromptFileImportBar({ disabled, onAttachmentsAdded, onTextExtracted, className = "" }: PromptFileImportBarProps) {
	const fileInputRef = useRef<HTMLInputElement>(null);
	const [isRecording, setIsRecording] = useState(false);
	const [recordingSeconds, setRecordingSeconds] = useState(0);
	const [frequencyData, setFrequencyData] = useState<number[]>([20, 45, 65, 80, 65, 45, 30, 20]);
	const [liveTranscript, setLiveTranscript] = useState("");

	const mediaRecorderRef = useRef<MediaRecorder | null>(null);
	const mediaStreamRef = useRef<MediaStream | null>(null);
	const audioChunksRef = useRef<Blob[]>([]);
	const timerRef = useRef<number | null>(null);

	// Web Audio Analyser refs
	const audioCtxRef = useRef<AudioContext | null>(null);
	const analyserRef = useRef<AnalyserNode | null>(null);
	const animFrameRef = useRef<number | null>(null);

	// Web Speech Recognition ref
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const recognitionRef = useRef<any>(null);
	const transcriptRef = useRef<string>("");

	const addFiles = useCallback(
		async (files: FileList | File[]) => {
			const attachments = await filesToAttachments(files);
			if (attachments.length > 0) {
				onAttachmentsAdded(attachments);
			}
		},
		[onAttachmentsAdded],
	);

	const cleanupStream = useCallback(() => {
		mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
		mediaStreamRef.current = null;
	}, []);

	const cleanupAudioVisualizer = useCallback(() => {
		if (animFrameRef.current) {
			cancelAnimationFrame(animFrameRef.current);
			animFrameRef.current = null;
		}
		if (audioCtxRef.current && audioCtxRef.current.state !== "closed") {
			void audioCtxRef.current.close().catch(() => {});
			audioCtxRef.current = null;
		}
		analyserRef.current = null;
		setFrequencyData([20, 45, 65, 80, 65, 45, 30, 20]);
	}, []);

	const cleanupTimer = useCallback(() => {
		if (timerRef.current) {
			clearInterval(timerRef.current);
			timerRef.current = null;
		}
		setRecordingSeconds(0);
	}, []);

	useEffect(() => {
		return () => {
			if (mediaRecorderRef.current?.state !== "inactive") {
				try {
					mediaRecorderRef.current?.stop();
				} catch {}
			}
			cleanupStream();
			cleanupAudioVisualizer();
			cleanupTimer();
		};
	}, [cleanupAudioVisualizer, cleanupStream, cleanupTimer]);

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
				console.warn("Advanced audio constraints rejected by driver, falling back to basic audio: true", constraintErr);
				stream = await navigator.mediaDevices.getUserMedia({ audio: true });
			}
			mediaStreamRef.current = stream;

			// 1. Setup Audio Frequency Analyser for real-time waveform visualization
			try {
				const AudioContextClass =
					window.AudioContext ||
					(window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
				if (AudioContextClass) {
					const audioCtx = new AudioContextClass();
					if (audioCtx.state === "suspended") {
						void audioCtx.resume();
					}
					const analyser = audioCtx.createAnalyser();
					analyser.fftSize = 64;
					analyser.smoothingTimeConstant = 0.65;
					const source = audioCtx.createMediaStreamSource(stream);
					source.connect(analyser);

					audioCtxRef.current = audioCtx;
					analyserRef.current = analyser;

					const bufferLength = analyser.frequencyBinCount;
					const dataArray = new Uint8Array(bufferLength);

					const updateFrequencies = () => {
						if (!analyserRef.current) return;
						analyserRef.current.getByteFrequencyData(dataArray);

						const sampleCount = 8;
						const step = Math.max(1, Math.floor(bufferLength / sampleCount));
						const sampled: number[] = [];
						for (let i = 0; i < sampleCount; i++) {
							const val = dataArray[i * step] || 0;
							// Scale 0..255 to percentage 15%..100%
							const pct = Math.max(15, Math.min(100, Math.round((val / 255) * 100)));
							sampled.push(pct);
						}
						setFrequencyData(sampled);
						animFrameRef.current = requestAnimationFrame(updateFrequencies);
					};
					updateFrequencies();
				}
			} catch (audioCtxErr) {
				console.warn("Web Audio API visualizer setup failed:", audioCtxErr);
			}

			// 2. Setup Web Speech Recognition for instant Speech-To-Text transcription
			transcriptRef.current = "";
			setLiveTranscript("");
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const SpeechRecognitionClass =
				(window as unknown as { SpeechRecognition?: any; webkitSpeechRecognition?: any }).SpeechRecognition ||
				(window as unknown as { SpeechRecognition?: any; webkitSpeechRecognition?: any }).webkitSpeechRecognition;

			if (SpeechRecognitionClass) {
				try {
					const recognition = new SpeechRecognitionClass();
					recognition.continuous = true;
					recognition.interimResults = true;
					recognition.lang = typeof navigator !== "undefined" && navigator.language ? navigator.language : "en-US";
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					recognition.onresult = (event: any) => {
						let fullText = "";
						for (let i = 0; i < event.results.length; i++) {
							fullText += event.results[i][0].transcript;
						}
						transcriptRef.current = fullText;
						setLiveTranscript(fullText);
					};
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					recognition.onerror = (event: any) => {
						console.warn("Speech recognition notice:", event.error);
					};
					recognition.start();
					recognitionRef.current = recognition;
				} catch (speechErr) {
					console.warn("Web Speech API failed to start:", speechErr);
				}
			}

			// 3. Start MediaRecorder
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
				cleanupAudioVisualizer();
				cleanupTimer();
				setIsRecording(false);
				mediaRecorderRef.current = null;
				toast.error("Recording failed. Please try again.");
			};

			recorder.onstop = async () => {
				cleanupStream();
				cleanupAudioVisualizer();
				cleanupTimer();
				setIsRecording(false);
				mediaRecorderRef.current = null;

				const capturedLiveText = transcriptRef.current.trim();

				// Option A: Live Speech Recognition provided the text
				if (capturedLiveText) {
					if (onTextExtracted) {
						onTextExtracted(capturedLiveText);
						toast.success(`Voice transcribed: "${capturedLiveText.slice(0, 50)}${capturedLiveText.length > 50 ? "..." : ""}"`);
					} else {
						const attachment = voiceTranscriptAttachment(`voice-${Date.now()}.wav`, capturedLiveText);
						onAttachmentsAdded([attachment]);
						toast.success("Voice transcript attached");
					}
					return;
				}

				// Option B: Fallback to recorded audio blob + Whisper transcription
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

				toast.message("Transcribing voice audio…");
				const whisperResult = await transcribeAudioFile(file);
				if (whisperResult?.text?.trim()) {
					const text = whisperResult.text.trim();
					if (onTextExtracted) {
						onTextExtracted(text);
						toast.success(`Voice transcribed: "${text.slice(0, 50)}${text.length > 50 ? "..." : ""}"`);
					} else {
						const attachment = voiceTranscriptAttachment(file.name, text);
						onAttachmentsAdded([attachment]);
						toast.success("Voice transcript attached");
					}
					return;
				}

				// Option C: Raw audio attachment fallback
				const attachment = await fileToAttachment(file);
				if (attachment) {
					onAttachmentsAdded([attachment]);
					toast.success("Voice recording attached");
				}
			};

			recorder.start(250);
			mediaRecorderRef.current = recorder;
			setIsRecording(true);
			setRecordingSeconds(0);
			timerRef.current = window.setInterval(() => {
				setRecordingSeconds((prev) => prev + 1);
			}, 1000);
		} catch (error) {
			cleanupStream();
			cleanupAudioVisualizer();
			cleanupTimer();
			setIsRecording(false);
			mediaRecorderRef.current = null;
			toast.error(getMicrophoneErrorMessage(error));
		}
	}, [cleanupAudioVisualizer, cleanupStream, cleanupTimer, onAttachmentsAdded, onTextExtracted]);

	const stopAndExtract = useCallback(() => {
		if (recognitionRef.current) {
			try {
				recognitionRef.current.stop();
			} catch {}
			recognitionRef.current = null;
		}
		if (mediaRecorderRef.current && mediaRecorderRef.current.state !== "inactive") {
			mediaRecorderRef.current.stop();
		}
		mediaRecorderRef.current = null;
	}, []);

	const cancelRecording = useCallback(() => {
		if (recognitionRef.current) {
			try {
				recognitionRef.current.abort();
			} catch {}
			recognitionRef.current = null;
		}
		if (mediaRecorderRef.current && mediaRecorderRef.current.state !== "inactive") {
			mediaRecorderRef.current.stop();
		}
		mediaRecorderRef.current = null;
		audioChunksRef.current = [];
		transcriptRef.current = "";
		setLiveTranscript("");
		cleanupStream();
		cleanupAudioVisualizer();
		cleanupTimer();
		setIsRecording(false);
		toast.message("Voice recording discarded");
	}, [cleanupAudioVisualizer, cleanupStream, cleanupTimer]);

	if (isRecording) {
		return (
			<div className={`flex items-center gap-2 rounded-full border border-rose-500/30 bg-rose-500/10 px-3 py-1 text-xs shadow-sm ${className}`}>
				{/* Live Pulsing Dot */}
				<span className="relative flex h-2 w-2 shrink-0">
					<span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-rose-400 opacity-75" />
					<span className="relative inline-flex h-2 w-2 rounded-full bg-rose-500" />
				</span>

				{/* Timer */}
				<span className="font-mono text-[11px] font-semibold text-rose-600 dark:text-rose-400 shrink-0">
					{formatDuration(recordingSeconds)}
				</span>

				{/* Animated Audio Frequency Bars */}
				<div className="flex items-center gap-[2px] h-4 px-1" title="Voice frequency waveform">
					{frequencyData.map((val, idx) => (
						<div
							key={idx}
							className="w-[3px] rounded-full bg-gradient-to-t from-rose-500 via-amber-400 to-teal-400 transition-[height] duration-75"
							style={{ height: `${Math.max(15, val)}%` }}
						/>
					))}
				</div>

				{/* Live Transcript / Status */}
				{liveTranscript ? (
					<span className="text-[11px] text-foreground font-medium max-w-[130px] truncate" title={liveTranscript}>
						&ldquo;{liveTranscript}&rdquo;
					</span>
				) : (
					<span className="text-[10px] text-muted-foreground italic shrink-0">Listening...</span>
				)}

				{/* Stop & Extract Button */}
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							type="button"
							size="icon"
							variant="destructive"
							onClick={stopAndExtract}
							className="h-6 w-6 rounded-full"
							aria-label="Stop & extract voice to text"
							data-testid="prompt-record-stop-button"
						>
							<Square className="h-2.5 w-2.5 fill-current" />
						</Button>
					</TooltipTrigger>
					<TooltipContent side="top">Stop & extract voice to text</TooltipContent>
				</Tooltip>

				{/* Cancel Button */}
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							type="button"
							size="icon"
							variant="ghost"
							onClick={cancelRecording}
							className="h-6 w-6 rounded-full text-muted-foreground hover:text-foreground"
							aria-label="Discard recording"
						>
							<X className="h-3 w-3" />
						</Button>
					</TooltipTrigger>
					<TooltipContent side="top">Discard</TooltipContent>
				</Tooltip>
			</div>
		);
	}

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
						variant="ghost"
						size="icon"
						disabled={disabled}
						onClick={startRecording}
						className="text-muted-foreground hover:text-foreground h-7 w-7"
						aria-label="Record voice"
						data-testid="prompt-record-voice-button"
					>
						<Mic className="h-3.5 w-3.5" />
					</Button>
				</TooltipTrigger>
				<TooltipContent side="top">
					Record voice & extract text
				</TooltipContent>
			</Tooltip>
		</div>
	);
}
