import React, { useState, useEffect, useMemo } from "react";
import { Globe, Check, Search, Languages, ChevronDown, Sparkles } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { toast } from "sonner";
import { cn } from "@/lib/utils";

export interface Language {
	code: string;
	name: string;
	nativeName: string;
	flag: string;
	popular?: boolean;
}

export const WORLD_LANGUAGES: Language[] = [
	{ code: "en", name: "English", nativeName: "English", flag: "🇺🇸", popular: true },
	{ code: "ta", name: "Tamil", nativeName: "தமிழ்", flag: "🇮🇳", popular: true },
	{ code: "hi", name: "Hindi", nativeName: "हिन्दी", flag: "🇮🇳", popular: true },
	{ code: "es", name: "Spanish", nativeName: "Español", flag: "🇪🇸", popular: true },
	{ code: "fr", name: "French", nativeName: "Français", flag: "🇫🇷", popular: true },
	{ code: "de", name: "German", nativeName: "Deutsch", flag: "🇩🇪", popular: true },
	{ code: "zh-CN", name: "Chinese (Simplified)", nativeName: "中文 (简体)", flag: "🇨🇳", popular: true },
	{ code: "ja", name: "Japanese", nativeName: "日本語", flag: "🇯🇵", popular: true },
	{ code: "ar", name: "Arabic", nativeName: "العربية", flag: "🇸🇦", popular: true },
	{ code: "ru", name: "Russian", nativeName: "Русский", flag: "🇷🇺", popular: true },
	{ code: "pt", name: "Portuguese", nativeName: "Português", flag: "🇵🇹", popular: true },
	{ code: "it", name: "Italian", nativeName: "Italiano", flag: "🇮🇹" },
	{ code: "ko", name: "Korean", nativeName: "한국어", flag: "🇰🇷" },
	{ code: "te", name: "Telugu", nativeName: "తెలుగు", flag: "🇮🇳" },
	{ code: "bn", name: "Bengali", nativeName: "বাংলা", flag: "🇮🇳" },
	{ code: "kn", name: "Kannada", nativeName: "ಕನ್ನಡ", flag: "🇮🇳" },
	{ code: "ml", name: "Malayalam", nativeName: "മലയാളം", flag: "🇮🇳" },
	{ code: "mr", name: "Marathi", nativeName: "मराठी", flag: "🇮🇳" },
	{ code: "gu", name: "Gujarati", nativeName: "ગુજરાતી", flag: "🇮🇳" },
	{ code: "pa", name: "Punjabi", nativeName: "ਪੰਜਾਬੀ", flag: "🇮🇳" },
	{ code: "ur", name: "Urdu", nativeName: "اردو", flag: "🇵🇰" },
	{ code: "tr", name: "Turkish", nativeName: "Türkçe", flag: "🇹🇷" },
	{ code: "vi", name: "Vietnamese", nativeName: "Tiếng Việt", flag: "🇻🇳" },
	{ code: "id", name: "Indonesian", nativeName: "Bahasa Indonesia", flag: "🇮🇩" },
	{ code: "th", name: "Thai", nativeName: "ไทย", flag: "🇹🇭" },
	{ code: "nl", name: "Dutch", nativeName: "Nederlands", flag: "🇳🇱" },
	{ code: "pl", name: "Polish", nativeName: "Polski", flag: "🇵🇱" },
	{ code: "uk", name: "Ukrainian", nativeName: "Українська", flag: "🇺🇦" },
	{ code: "fa", name: "Persian", nativeName: "فارسی", flag: "🇮🇷" },
	{ code: "he", name: "Hebrew", nativeName: "עברית", flag: "🇮🇱" },
	{ code: "sv", name: "Swedish", nativeName: "Svenska", flag: "🇸🇪" },
	{ code: "el", name: "Greek", nativeName: "Ελληνικά", flag: "🇬🇷" },
];

function getStoredLanguage(): string {
	if (typeof window === "undefined") return "en";
	try {
		// 1. Check localStorage
		const local = localStorage.getItem("unifai_language");
		if (local) return local;

		// 2. Check googtrans cookie
		const cookies = document.cookie.split(";");
		for (const cookie of cookies) {
			const [name, val] = cookie.trim().split("=");
			if (name === "googtrans" && val) {
				const parts = val.split("/");
				if (parts.length >= 3 && parts[2]) {
					return parts[2];
				}
			}
		}
	} catch {
		// ignore
	}
	return "en";
}

function setGoogleTransCookie(targetCode: string) {
	if (typeof window === "undefined") return;
	const isEn = targetCode === "en";
	const cookieVal = isEn ? "" : `/en/${targetCode}`;
	const expires = isEn ? "Thu, 01 Jan 1970 00:00:00 UTC" : new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toUTCString();
	const domain = window.location.hostname;

	document.cookie = `googtrans=${cookieVal}; path=/; expires=${expires};`;
	document.cookie = `googtrans=${cookieVal}; path=/; domain=${domain}; expires=${expires};`;
	if (domain && domain.includes(".")) {
		const parts = domain.split(".");
		if (parts.length >= 2) {
			const root = parts.slice(-2).join(".");
			document.cookie = `googtrans=${cookieVal}; path=/; domain=.${root}; expires=${expires};`;
		}
	}
}

export function ensureGoogleTranslateScript() {
	if (typeof window === "undefined") return;
	if (document.getElementById("google-translate-script")) return;

	// Add container if not present
	if (!document.getElementById("google_translate_element")) {
		const div = document.createElement("div");
		div.id = "google_translate_element";
		div.style.display = "none";
		document.body.appendChild(div);
	}

	// Define global init handler
	(window as unknown as { googleTranslateElementInit: () => void }).googleTranslateElementInit = function () {
		const googleObj = (window as unknown as { google?: { translate?: { TranslateElement: new (opts: unknown, id: string) => void } } }).google;
		if (googleObj && googleObj.translate && googleObj.translate.TranslateElement) {
			new googleObj.translate.TranslateElement(
				{
					pageLanguage: "en",
					autoDisplay: false,
				},
				"google_translate_element",
			);
		}
	};

	const script = document.createElement("script");
	script.id = "google-translate-script";
	script.type = "text/javascript";
	script.src = "//translate.google.com/translate_a/element.js?cb=googleTranslateElementInit";
	script.async = true;
	document.head.appendChild(script);
}

interface LanguageSelectorProps {
	compact?: boolean;
	className?: string;
}

export function LanguageSelector({ compact = false, className }: LanguageSelectorProps) {
	const [selectedCode, setSelectedCode] = useState<string>("en");
	const [open, setOpen] = useState(false);
	const [searchQuery, setSearchQuery] = useState("");

	useEffect(() => {
		setSelectedCode(getStoredLanguage());
		ensureGoogleTranslateScript();
	}, []);

	const currentLanguage = useMemo(() => {
		return WORLD_LANGUAGES.find((l) => l.code === selectedCode) || WORLD_LANGUAGES[0];
	}, [selectedCode]);

	const filteredLanguages = useMemo(() => {
		if (!searchQuery.trim()) return WORLD_LANGUAGES;
		const q = searchQuery.toLowerCase().trim();
		return WORLD_LANGUAGES.filter(
			(l) => l.name.toLowerCase().includes(q) || l.nativeName.toLowerCase().includes(q) || l.code.toLowerCase().includes(q),
		);
	}, [searchQuery]);

	const handleSelectLanguage = (lang: Language) => {
		const targetCode = lang.code;
		setSelectedCode(targetCode);
		setOpen(false);

		try {
			localStorage.setItem("unifai_language", targetCode);
			setGoogleTransCookie(targetCode);

			// Trigger Google Translate select element if present
			const combo = document.querySelector<HTMLSelectElement>(".goog-te-combo");
			if (combo) {
				combo.value = targetCode;
				combo.dispatchEvent(new Event("change"));
				toast.success(`Language changed to ${lang.name} (${lang.nativeName})`);
			} else {
				// Script still initializing or first switch: reload to apply cookie cleanly
				toast.info(`Applying ${lang.name} (${lang.nativeName})...`);
				setTimeout(() => {
					window.location.reload();
				}, 300);
			}
		} catch (err) {
			console.error("Language switch error:", err);
			window.location.reload();
		}
	};

	const triggerButton = compact ? (
		<button
			type="button"
			className={cn(
				"hover:text-primary text-muted-foreground hover:bg-accent/50 flex h-8 w-8 cursor-pointer items-center justify-center rounded-md p-1.5 transition-colors",
				selectedCode !== "en" && "text-primary bg-primary/10 font-medium",
				className,
			)}
			aria-label="Change Language"
		>
			<Languages className="h-4 w-4" />
		</button>
	) : (
		<Button
			variant="outline"
			size="sm"
			className={cn(
				"h-8 gap-1.5 rounded-full border-border/80 bg-background/80 px-2.5 text-xs font-normal shadow-xs backdrop-blur-sm hover:border-primary/50 hover:bg-accent/60 transition-all",
				selectedCode !== "en" && "border-primary/50 bg-primary/5 font-medium text-primary",
				className,
			)}
			type="button"
		>
			<span className="text-sm leading-none">{currentLanguage.flag}</span>
			<span className="max-w-[80px] truncate">{currentLanguage.nativeName}</span>
			<ChevronDown className="h-3 w-3 opacity-60 ml-0.5" />
		</Button>
	);

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<Tooltip>
				<TooltipTrigger asChild>
					<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
				</TooltipTrigger>
				<TooltipContent side="bottom" className="text-xs">
					Change Language / மொழி மாற்ற / भाषा बदलें
				</TooltipContent>
			</Tooltip>

			<PopoverContent
				side="bottom"
				align="end"
				className="w-80 p-0 shadow-xl border-border bg-card/95 backdrop-blur-md rounded-xl overflow-hidden z-[100]"
			>
				<div className="p-3 border-b border-border/60 bg-muted/30">
					<div className="flex items-center justify-between mb-2">
						<div className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
							<Globe className="h-3.5 w-3.5 text-primary" />
							<span>Language / மொழி / भाषा</span>
						</div>
						{selectedCode !== "en" && (
							<Badge
								variant="secondary"
								className="text-[10px] px-1.5 py-0 cursor-pointer hover:bg-muted"
								onClick={() => handleSelectLanguage(WORLD_LANGUAGES[0])}
							>
								Reset to English
							</Badge>
						)}
					</div>

					<div className="relative">
						<Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
						<Input
							placeholder="Search language / மொழி தேட..."
							value={searchQuery}
							onChange={(e) => setSearchQuery(e.target.value)}
							className="h-8 pl-8 text-xs bg-background/80 border-border"
							autoFocus
						/>
					</div>
				</div>

				<div className="max-h-[300px] overflow-y-auto p-1.5 custom-scrollbar divide-y divide-border/20">
					{/* Popular Languages shortcuts when search is empty */}
					{!searchQuery.trim() && (
						<div className="pb-1.5 mb-1.5">
							<div className="px-2 py-1 text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
								Popular Languages
							</div>
							<div className="grid grid-cols-2 gap-1 px-1">
								{WORLD_LANGUAGES.filter((l) => l.popular).map((lang) => {
									const isSelected = lang.code === selectedCode;
									return (
										<button
											key={lang.code}
											type="button"
											onClick={() => handleSelectLanguage(lang)}
											className={cn(
												"flex items-center gap-2 px-2 py-1.5 text-xs rounded-md text-left transition-colors cursor-pointer",
												isSelected
													? "bg-primary text-primary-foreground font-semibold shadow-xs"
													: "hover:bg-accent text-foreground hover:text-accent-foreground",
											)}
										>
											<span className="text-sm leading-none">{lang.flag}</span>
											<span className="truncate">{lang.nativeName}</span>
											{isSelected && <Check className="h-3 w-3 ml-auto shrink-0" />}
										</button>
									);
								})}
							</div>
						</div>
					)}

					{/* All filtered languages */}
					<div className="pt-1">
						{!searchQuery.trim() && (
							<div className="px-2 py-1 text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
								All World Languages ({filteredLanguages.length})
							</div>
						)}
						{filteredLanguages.length === 0 ? (
							<div className="py-6 text-center text-xs text-muted-foreground">
								No languages found matching &ldquo;{searchQuery}&rdquo;
							</div>
						) : (
							<div className="space-y-0.5">
								{filteredLanguages.map((lang) => {
									const isSelected = lang.code === selectedCode;
									return (
										<button
											key={lang.code}
											type="button"
											onClick={() => handleSelectLanguage(lang)}
											className={cn(
												"w-full flex items-center justify-between px-2.5 py-1.5 text-xs rounded-md text-left transition-colors cursor-pointer group",
												isSelected
													? "bg-primary/10 text-primary font-semibold"
													: "hover:bg-accent text-foreground hover:text-accent-foreground",
											)}
										>
											<div className="flex items-center gap-2 truncate">
												<span className="text-sm leading-none">{lang.flag}</span>
												<span className="font-medium">{lang.nativeName}</span>
												<span className="text-muted-foreground text-[11px] truncate">({lang.name})</span>
											</div>
											{isSelected && <Check className="h-3.5 w-3.5 text-primary shrink-0" />}
										</button>
									);
								})}
							</div>
						)}
					</div>
				</div>

				<div className="p-2 border-t border-border/60 bg-muted/20 flex items-center justify-between text-[10px] text-muted-foreground">
					<span className="flex items-center gap-1">
						<Sparkles className="h-3 w-3 text-amber-500" />
						Instant translation across all pages
					</span>
					<span>Powered by UnifAI</span>
				</div>
			</PopoverContent>
		</Popover>
	);
}

export default LanguageSelector;
