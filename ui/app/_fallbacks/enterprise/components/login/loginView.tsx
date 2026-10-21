import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { COMPANY_LOGO, COMPANY_NAME, DEFAULT_FOOTER_TEXT } from "@/lib/constants/config";
import { useBranding } from "@/lib/hooks/useBranding";
import {
	getErrorMessage,
	useChangeTemporaryPasswordMutation,
	useForgotPasswordMutation,
	useForgotUsernameMutation,
	useLoginMutation,
	useResetPasswordMutation,
	useVerifyOTPMutation,
} from "@/lib/store/apis";
import { getLoginGotoFromSearch } from "@/lib/utils/loginGoto";
import { resolvePostLoginPath } from "@/lib/utils/workspaceAccess";
import {
	ArrowLeft,
	ArrowRight,
	CheckCircle2,
	Cpu,
	Eye,
	EyeOff,
	KeyRound,
	Lock,
	Mail,
	Shield,
	ShieldAlert,
	Sparkles,
	User,
	Ban,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";

type AuthMode = "login" | "forgot" | "reset" | "forgot_username" | "force_change_password";

const LOCKOUT_STORAGE_KEY = "unifai_auth_lockout";

interface LockedAccountInfo {
	identifier: string;
	endsAt: number;
	reason: string;
}

function getStoredLockout(): LockedAccountInfo | null {
	if (typeof window === "undefined") return null;
	try {
		const raw = localStorage.getItem(LOCKOUT_STORAGE_KEY);
		if (!raw) return null;
		const parsed = JSON.parse(raw);
		if (parsed && typeof parsed.endsAt === "number" && Date.now() < parsed.endsAt) {
			return parsed;
		}
		localStorage.removeItem(LOCKOUT_STORAGE_KEY);
	} catch {}
	return null;
}

function saveStoredLockout(data: LockedAccountInfo | null) {
	if (typeof window === "undefined") return;
	try {
		if (data && data.endsAt > Date.now()) {
			localStorage.setItem(LOCKOUT_STORAGE_KEY, JSON.stringify(data));
		} else {
			localStorage.removeItem(LOCKOUT_STORAGE_KEY);
		}
	} catch {}
}

// Must match the server's per-account forgot-password cooldown; a resend inside it is
// silently dropped (generic response, no email).
const OTP_RESEND_COOLDOWN_SECONDS = 120;

export default function LoginView() {
	const branding = useBranding();
	const [mode, setMode] = useState<AuthMode>(() => {
		if (typeof window !== "undefined") {
			const params = new URLSearchParams(window.location.search);
			const m = params.get("mode");
			if (m === "force_change_password" || m === "forgot" || m === "forgot_username") {
				return m;
			}
		}
		return "login";
	});
	const [username, setUsername] = useState(() => {
		if (typeof window !== "undefined") {
			return new URLSearchParams(window.location.search).get("username") || "";
		}
		return "";
	});
	const [email, setEmail] = useState(() => {
		if (typeof window !== "undefined") {
			return new URLSearchParams(window.location.search).get("email") || "";
		}
		return "";
	});
	const [password, setPassword] = useState("");
	const [otp, setOtp] = useState("");
	const [isOtpVerified, setIsOtpVerified] = useState(false);
	const [resetToken, setResetToken] = useState("");
	const [newPassword, setNewPassword] = useState("");
	const [confirmPassword, setConfirmPassword] = useState("");
	const [showPassword, setShowPassword] = useState(false);
	const [showNewPassword, setShowNewPassword] = useState(false);
	const [showConfirmPassword, setShowConfirmPassword] = useState(false);
	const [resendCooldown, setResendCooldown] = useState(0);
	const [errorMessage, setErrorMessage] = useState("");
	const [infoMessage, setInfoMessage] = useState("");
	const [isLoading, setIsLoading] = useState(false);
	const [lockedAccount, setLockedAccountState] = useState<LockedAccountInfo | null>(getStoredLockout);

	const setLockedAccount = useCallback((data: LockedAccountInfo | null) => {
		setLockedAccountState(data);
		saveStoredLockout(data);
	}, []);

	const [now, setNow] = useState(() => Date.now());

	const [login, { isLoading: isLoggingIn }] = useLoginMutation();
	const [forgotPassword, { isLoading: isSendingOtp }] = useForgotPasswordMutation();
	const [verifyOtp, { isLoading: isVerifyingOtp }] = useVerifyOTPMutation();
	const [resetPassword, { isLoading: isResetting }] = useResetPasswordMutation();
	const [forgotUsername, { isLoading: isRetrievingUsername }] = useForgotUsernameMutation();
	const [changeTemporaryPassword, { isLoading: isChangingTempPassword }] = useChangeTemporaryPasswordMutation();
	const [pendingPostLoginTarget, setPendingPostLoginTarget] = useState("");
	const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

	const clearFieldError = (field: string) => {
		setFieldErrors((prev) => {
			if (!prev[field]) return prev;
			const next = { ...prev };
			delete next[field];
			return next;
		});
	};

	useEffect(() => {
		if (resendCooldown <= 0) return;
		const timer = setInterval(() => setResendCooldown((c) => c - 1), 1000);
		return () => clearInterval(timer);
	}, [resendCooldown]);

	useEffect(() => {
		if (mode === "force_change_password" && !infoMessage) {
			setInfoMessage("Please set a new permanent password to continue.");
		}
	}, [mode, infoMessage]);

	useEffect(() => {
		if (!lockedAccount) return;
		const tick = () => {
			const current = Date.now();
			setNow(current);
			if (current >= lockedAccount.endsAt) {
				setLockedAccount(null);
				setErrorMessage("");
			}
		};
		tick();
		const timer = setInterval(tick, 1000);
		return () => clearInterval(timer);
	}, [lockedAccount, setLockedAccount]);

	// Active identifier being typed/used in the current mode
	const activeInputIdentifier = (
		mode === "login" || mode === "force_change_password"
			? username
			: email
	).trim().toLowerCase();

	// Lockout applies strictly when the CURRENTLY entered username/email matches the locked account.
	// If the user enters a different account ID, do not block or show the waiting error for that different account.
	const isCurrentAccountLocked = Boolean(
		lockedAccount &&
		now < lockedAccount.endsAt &&
		activeInputIdentifier &&
		activeInputIdentifier === lockedAccount.identifier
	);
	const lockoutSecondsLeft = isCurrentAccountLocked && lockedAccount
		? Math.max(0, Math.ceil((lockedAccount.endsAt - now) / 1000))
		: 0;
	const isLockedOut = isCurrentAccountLocked && lockoutSecondsLeft > 0;
	const lockoutCountdown = `${Math.floor(lockoutSecondsLeft / 60)}:${String(lockoutSecondsLeft % 60).padStart(2, "0")}`;

	const has8Chars = newPassword.length >= 8;
	const hasUpper = /[A-Z]/.test(newPassword);
	const hasLower = /[a-z]/.test(newPassword);
	const hasDigit = /[0-9]/.test(newPassword);
	const hasSpecial = /[^A-Za-z0-9]/.test(newPassword);
	const passwordsMatch = newPassword.length > 0 && newPassword === confirmPassword;

	const handleResendOtp = async () => {
		if (isSendingOtp || isLockedOut) return;
		if (!email.trim()) {
			setErrorMessage("Please enter your email address first");
			return;
		}
		if (
			lockedAccount &&
			email.trim().toLowerCase() === lockedAccount.identifier &&
			now < lockedAccount.endsAt
		) {
			setErrorMessage(`This account is temporarily locked for ${lockoutCountdown}. Please wait before trying again.`);
			return;
		}
		setErrorMessage("");
		setInfoMessage("");
		try {
			const cleanInput = email.trim();
			const payload = cleanInput.includes("@") ? { email: cleanInput } : { username: cleanInput, email: cleanInput };
			const result = await forgotPassword(payload).unwrap();
			setInfoMessage(result.message || "A fresh verification code was sent to your email.");
			setResendCooldown(OTP_RESEND_COOLDOWN_SECONDS);
		} catch (error) {
			setErrorMessage(getErrorMessage(error));
		}
	};

	const handleVerifyOtp = async () => {
		const cleanOtp = otp.trim();
		if (!cleanOtp) {
			setFieldErrors((prev) => ({ ...prev, otp: "Please fill in this field" }));
			setErrorMessage("Please enter the 6-digit OTP code");
			return;
		}
		if (cleanOtp.length < 6) {
			setFieldErrors((prev) => ({ ...prev, otp: "OTP must be 6 digits" }));
			setErrorMessage("OTP must be 6 digits");
			return;
		}
		setErrorMessage("");
		setInfoMessage("");
		try {
			const cleanInput = email.trim();
			const payload = cleanInput.includes("@")
				? { email: cleanInput, otp: cleanOtp }
				: { username: cleanInput, email: cleanInput, otp: cleanOtp };
			const res = await verifyOtp(payload).unwrap();
			setIsOtpVerified(true);
			clearFieldError("otp");
			if (res.reset_token) {
				setResetToken(res.reset_token);
			}
			setInfoMessage("OTP verified successfully! Please enter your new password below.");
		} catch (error) {
			setIsOtpVerified(false);
			setErrorMessage(getErrorMessage(error));
		}
	};

	const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		if (isLockedOut) {
			setErrorMessage(`This account is temporarily locked for ${lockoutCountdown}. Please wait before trying again.`);
			return;
		}
		setErrorMessage("");
		setInfoMessage("");

		if (mode === "login") {
			const trimmedUsername = username.trim();
			const newErrors: Record<string, string> = {};
			if (!trimmedUsername) {
				newErrors.username = "Please fill in this field";
			}
			if (!password) {
				newErrors.password = "Please fill in this field";
			}
			if (Object.keys(newErrors).length > 0) {
				setFieldErrors(newErrors);
				setErrorMessage("Please fill in all required fields");
				return;
			}
		}

		if (mode === "forgot") {
			const cleanInput = email.trim();
			if (!cleanInput) {
				setFieldErrors({ email: "Please fill in this field" });
				setErrorMessage("Please fill in your email address or username");
				return;
			}
			if (
				lockedAccount &&
				cleanInput.toLowerCase() === lockedAccount.identifier &&
				now < lockedAccount.endsAt
			) {
				setErrorMessage(`This account is temporarily locked for ${lockoutCountdown}. Forgot password is not allowed during lockout.`);
				return;
			}
		}

		if (mode === "forgot_username") {
			const cleanInput = email.trim();
			if (!cleanInput) {
				setFieldErrors({ email: "Please fill in this field" });
				setErrorMessage("Please fill in your email address");
				return;
			}
			const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
			if (!emailRegex.test(cleanInput)) {
				setFieldErrors({ email: "Please enter a valid email address" });
				setErrorMessage("Please enter a valid email address");
				return;
			}
			if (
				lockedAccount &&
				cleanInput.toLowerCase() === lockedAccount.identifier &&
				now < lockedAccount.endsAt
			) {
				setErrorMessage(`This account is temporarily locked for ${lockoutCountdown}. Forgot username is not allowed during lockout.`);
				return;
			}
		}

		if (mode === "reset") {
			const newErrors: Record<string, string> = {};
			if (!otp.trim()) {
				newErrors.otp = "Please fill in this field";
			} else if (otp.trim().length < 6) {
				newErrors.otp = "OTP must be 6 digits";
			} else if (!isOtpVerified || !resetToken.trim()) {
				newErrors.otp = "Please verify code first";
			}

			if (!newPassword) {
				newErrors.newPassword = "Please fill in this field";
			} else if (!has8Chars || !hasUpper || !hasLower || !hasDigit || !hasSpecial) {
				newErrors.newPassword = "Must meet all 5 requirements";
			}

			if (!confirmPassword) {
				newErrors.confirmPassword = "Please fill in this field";
			} else if (newPassword !== confirmPassword) {
				newErrors.confirmPassword = "Passwords do not match";
			}

			if (Object.keys(newErrors).length > 0) {
				setFieldErrors(newErrors);
				if (!otp.trim() || !newPassword || !confirmPassword) {
					setErrorMessage("Please fill in all required fields");
				} else if (!isOtpVerified || !resetToken.trim()) {
					setErrorMessage("Please click 'Verify Code' to verify your code before setting password");
				} else if (newPassword !== confirmPassword) {
					setErrorMessage("Passwords do not match");
				} else {
					setErrorMessage("Password must be at least 8 characters and include uppercase, lowercase, a number, and a symbol");
				}
				return;
			}
		}

		setIsLoading(true);
		try {
			if (mode === "login") {
				const trimmedUsername = username.trim();
				const result = await login({ username: trimmedUsername, password }).unwrap();
				const goto = getLoginGotoFromSearch(window.location.search);
				const target = resolvePostLoginPath({ role: result.role, allowed_sections: result.allowed_sections }, goto);
				if (result.must_change_password) {
					setPendingPostLoginTarget(target);
					setMode("force_change_password");
					setNewPassword("");
					setConfirmPassword("");
					setErrorMessage("");
					setInfoMessage("Temporary password verified. Please set your new permanent password below.");
					return;
				}
				window.location.assign(target);
				return;
			}
			if (mode === "force_change_password") {
				const newErrors: Record<string, string> = {};
				if (!newPassword) {
					newErrors.newPassword = "Please fill in this field";
				} else if (!has8Chars || !hasUpper || !hasLower || !hasDigit || !hasSpecial) {
					newErrors.newPassword = "Must meet all 5 requirements";
				}
				if (!confirmPassword) {
					newErrors.confirmPassword = "Please fill in this field";
				} else if (newPassword !== confirmPassword) {
					newErrors.confirmPassword = "Passwords do not match";
				}
				if (Object.keys(newErrors).length > 0) {
					setFieldErrors(newErrors);
					if (!newPassword || !confirmPassword) {
						setErrorMessage("Please fill in all required fields");
					} else if (newPassword !== confirmPassword) {
						setErrorMessage("Passwords do not match");
					} else {
						setErrorMessage("Password must be at least 8 characters and include uppercase, lowercase, a number, and a symbol");
					}
					return;
				}
				const res = await changeTemporaryPassword({
					new_password: newPassword,
					confirm_password: confirmPassword,
				}).unwrap();
				setInfoMessage(res.message || "Password updated successfully! Entering dashboard...");
				setTimeout(() => {
					window.location.assign(pendingPostLoginTarget || "/workspace");
				}, 600);
				return;
			}
			if (mode === "forgot") {
				const cleanInput = email.trim();
				const payload = cleanInput.includes("@") ? { email: cleanInput } : { username: cleanInput, email: cleanInput };
				const result = await forgotPassword(payload).unwrap();
				setInfoMessage(result.message || "If an account matches, a one-time code was sent by email");
				setMode("reset");
				setIsOtpVerified(false);
				setResetToken("");
				setOtp("");
				setResendCooldown(OTP_RESEND_COOLDOWN_SECONDS);
				return;
			}
			if (mode === "forgot_username") {
				const result = await forgotUsername({ email: email.trim() }).unwrap();
				setInfoMessage(result.message || "If an account matches, your username was sent to your email.");
				return;
			}
			const cleanInput = email.trim();
			const result = await resetPassword({
				email: cleanInput,
				username: cleanInput,
				reset_token: resetToken.trim(),
				new_password: newPassword,
			}).unwrap();
			setInfoMessage(result.message || "Password updated. Sign in with your new password.");
			// The reset lifts the server-side lock, so drop the stale countdown too.
			setLockedAccount(null);
			setMode("login");
			setEmail("");
			setPassword("");
			setOtp("");
			setIsOtpVerified(false);
			setResetToken("");
			setNewPassword("");
			setConfirmPassword("");
		} catch (error) {
			const message = getErrorMessage(error);
			setErrorMessage(message);
			let retryAfterSeconds = (error as { retryAfterSeconds?: number } | undefined)?.retryAfterSeconds;
			if (!retryAfterSeconds && /about\s+(\d+)\s+minutes?/i.test(message)) {
				const match = message.match(/about\s+(\d+)\s+minutes?/i);
				if (match && match[1]) {
					retryAfterSeconds = parseInt(match[1], 10) * 60;
				}
			}
			if (retryAfterSeconds || /locked|too many failed/i.test(message)) {
				const seconds = retryAfterSeconds || 1200;
				const reason = message.split(/(?<=\.)\s/)[0] || "Too many failed attempts. Account locked";
				const targetId = (mode === "login" || mode === "force_change_password" ? username : email).trim().toLowerCase();
				if (targetId) {
					setLockedAccount({
						identifier: targetId,
						endsAt: Date.now() + seconds * 1000,
						reason,
					});
				}
			}
		} finally {
			setIsLoading(false);
		}
	};

	return (
		<div className="relative flex h-screen flex-col overflow-hidden bg-[#f6f8fd] font-sans text-slate-600 selection:bg-blue-500/20 selection:text-[#0b1b3a]">
			<div className="pointer-events-none absolute inset-x-0 top-0 h-[420px] bg-[radial-gradient(ellipse_70%_80%_at_50%_-20%,#d9e6ff_0%,rgba(246,248,253,0)_70%)]" />

			{/* Fixed Top Navigation Header */}
			<header className="relative z-20 flex-shrink-0 mx-auto flex w-full max-w-[1400px] items-center justify-between px-6 py-4">
				<div className="flex min-w-0 items-center gap-4">
					<button
						type="button"
						onClick={() => window.location.assign("/")}
						className="group inline-flex shrink-0 items-center gap-2 rounded-full border border-[#e3ebf8] bg-white px-4 py-1.5 text-xs font-medium text-[#0b1b3a] transition-all hover:border-[#2f6bff]/40 hover:text-[#2f6bff] cursor-pointer shadow-sm"
						title="Return to Landing Page"
					>
						<ArrowLeft className="h-3.5 w-3.5 transition-transform group-hover:-translate-x-1 text-[#2f6bff]" />
						<span>Back to Home</span>
					</button>
					<div className="hidden h-5 w-px shrink-0 bg-[#dbe4f5] sm:block" />
					<div className="flex min-w-0 items-center gap-2.5">
						<img
							src={branding.companyLogo}
							alt={branding.companyName}
							className="h-10 w-auto max-w-[180px] object-contain"
						/>
						<span className="truncate text-lg font-extrabold tracking-tight text-[#0b1b3a]">Raksha</span>
						<span className="relative flex h-2 w-2 shrink-0">
							<span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
							<span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
						</span>
					</div>
				</div>

				<div className="flex items-center gap-3">
					<span className="hidden text-xs text-slate-400 md:inline">Don't have an account?</span>
					<Button
						type="button"
						size="sm"
						onClick={() => window.location.assign("/signup")}
						className="h-8.5 rounded-full border-[#2f6bff] bg-[#2f6bff] px-4 text-xs font-semibold text-white hover:bg-[#2457d6] transition-all cursor-pointer shadow-sm"
					>
						Sign Up
						<ArrowRight className="ml-1.5 h-3.5 w-3.5" />
					</Button>
				</div>
			</header>

			{/* Main Center Content - only this area scrolls */}
			<main className="relative z-10 flex-1 overflow-y-auto">
				<div className="mx-auto flex w-full max-w-[1300px] min-h-full items-center justify-center px-6 py-8">
					<div className="grid w-full items-center gap-12 lg:grid-cols-12 lg:gap-14">
					{/* Left Column: Enterprise Highlights */}
					<section className="hidden flex-col justify-center space-y-8 lg:col-span-6 lg:flex">
						<div className="space-y-4">
							<div className="inline-flex items-center gap-2 rounded-full border border-[#d5e2ff] bg-white px-3.5 py-1 text-xs font-semibold tracking-wide text-[#2f6bff] shadow-sm">
								<span className="h-2 w-2 rounded-full bg-emerald-400 shadow-[0_0_8px_#34d399]" />
								<span>Gateway Active • Zero-Trust Control Plane</span>
							</div>

							<h1 className="text-4xl font-extrabold tracking-tight text-[#0b1b3a] sm:text-5xl leading-[1.08]">
								Enterprise AI,{" "}
								<span className="bg-gradient-to-r from-[#2f6bff] to-[#7a5cff] bg-clip-text text-transparent">
									Governed & Unified.
								</span>
							</h1>

							<p className="max-w-lg text-sm text-slate-400 leading-relaxed">
								Access your centralized AI control plane. Orchestrate 100+ LLMs, manage Prompt Repositories with multimodal image support, enforce token budgets, and monitor live audit logs.
							</p>
						</div>

						{/* Feature Benefit Cards */}
						<div className="space-y-3.5 max-w-lg">
							<div className="flex items-start gap-3.5 rounded-xl border border-[#e6edf8] bg-white p-4 shadow-sm transition-colors hover:border-[#2f6bff]/30">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#eef3ff] text-[#2f6bff]">
									<Cpu className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-[#0b1b3a]">Unified LLM Gateway</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Route dynamically through OpenAI, Claude, Gemini, DeepSeek, and local models with a single API key.
									</p>
								</div>
							</div>

							<div className="flex items-start gap-3.5 rounded-xl border border-[#e6edf8] bg-white p-4 shadow-sm transition-colors hover:border-[#2f6bff]/30">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-400">
									<Sparkles className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-[#0b1b3a]">Prompt Repository & Vision</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										ChatGPT-style multimodal image support, folder-based team access, and persistent chat sessions.
									</p>
								</div>
							</div>

							<div className="flex items-start gap-3.5 rounded-xl border border-[#e6edf8] bg-white p-4 shadow-sm transition-colors hover:border-[#2f6bff]/30">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-indigo-500/10 text-indigo-400">
									<Shield className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-[#0b1b3a]">Zero-Trust Guardrails & Budgets</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Enforce personal user and team spending limits, real-time PII redaction, and compliance auditing.
									</p>
								</div>
							</div>
						</div>
					</section>

					{/* Right Column: Sleek Authentication Card */}
					<section className="flex items-center justify-center lg:col-span-6">
						<div className="w-full max-w-[440px] rounded-3xl border border-[#e6edf8] bg-white p-8 shadow-[0_24px_60px_rgba(47,107,255,0.10)]">
							{mode !== "login" && (
								<button
									type="button"
									onClick={() => {
										if (mode === "reset") {
											setMode("forgot");
										} else {
											setMode("login");
										}
										setErrorMessage("");
										setInfoMessage("");
										setFieldErrors({});
										setPassword("");
										setNewPassword("");
										setConfirmPassword("");
									}}
									className="group mb-4 inline-flex items-center gap-1.5 text-xs font-medium text-slate-400 hover:text-[#2f6bff] transition-colors cursor-pointer"
								>
									<ArrowLeft className="h-3.5 w-3.5 transition-transform group-hover:-translate-x-1" />
									<span>{mode === "reset" ? "Back to forgot password" : "Back to Sign In"}</span>
								</button>
							)}

							<div className="mb-6 space-y-1.5">
								<h2 className="text-2xl font-bold tracking-tight text-[#0b1b3a]">
									{mode === "login"
										? "Sign In"
										: mode === "forgot"
											? "Forgot Password"
											: mode === "forgot_username"
												? "Forgot Username"
												: mode === "force_change_password"
													? "Set New Password"
													: "Reset Password"}
								</h2>
								<p className="text-xs leading-relaxed text-slate-400">
									{mode === "login"
										? "Enter your credentials to access your organization's AI control plane."
										: mode === "forgot"
											? "Enter your registered email or username. We'll send a one-time verification code."
											: mode === "forgot_username"
												? "Enter your registered email address to retrieve your username."
												: mode === "force_change_password"
													? "You logged in with a temporary password. Please set your new permanent password to continue."
													: "Enter the OTP sent to your email, verify it, and choose a new password."}
								</p>
							</div>

							{isLockedOut ? (
								<div className="mb-4 flex flex-col gap-1.5 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-800 shadow-sm animate-in fade-in duration-150">
									<div className="flex items-center gap-2 font-semibold text-rose-800">
										<ShieldAlert className="h-4 w-4 shrink-0 text-rose-700" />
										<span>{lockedAccount?.reason || "Account Locked Out"}</span>
									</div>
									<p className="text-rose-900 leading-relaxed text-[11px]">
										Too many failed attempts for <span className="font-semibold text-[#0b1b3a]">"{activeInputIdentifier || lockedAccount?.identifier}"</span>. Please wait{" "}
										<span className="font-mono font-bold text-amber-800">{lockoutCountdown}</span> before trying again.
									</p>
								</div>
							) : errorMessage ? (
								<div className="mb-4 flex flex-col gap-1 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-800">
									<div className="flex items-center gap-2 font-semibold text-rose-800">
										{errorMessage.toLowerCase().includes("blocked") ? (
											<Ban className="h-4 w-4 shrink-0 text-rose-700" />
										) : (
											<ShieldAlert className="h-4 w-4 shrink-0 text-rose-700" />
										)}
										<span>
											{errorMessage.toLowerCase().includes("blocked")
												? "Account Blocked"
												: mode === "forgot"
													? "Forgot Password Error"
													: mode === "reset"
														? "Reset Password Error"
														: mode === "forgot_username"
															? "Forgot Username Error"
															: mode === "force_change_password"
																? "Password Change Error"
																: "Sign In Error"}
										</span>
									</div>
									<p className="text-rose-900 leading-relaxed text-[11px] pl-6">
										{errorMessage}
									</p>
								</div>
							) : null}

							{infoMessage && (
								<div className="mb-4 flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-800">
									<CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-700" />
									<span>{infoMessage}</span>
								</div>
							)}

							<form onSubmit={handleSubmit} noValidate className="space-y-4">
								{/* Login Mode Fields */}
								{mode === "login" && (
									<>
										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="username" className="text-xs font-medium text-slate-700">
													Username or email <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
												</Label>
												{fieldErrors.username && (
													<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
														<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
														{fieldErrors.username}
													</span>
												)}
											</div>
											<div className="relative">
												<User className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="username"
													type="text"
													placeholder="Username or email"
													value={username}
													onChange={(e) => {
														const val = e.target.value;
														setUsername(val);
														clearFieldError("username");
														if (errorMessage && !isLockedOut) {
															setErrorMessage("");
														}
													}}
													onInput={() => {
														clearFieldError("username");
														if (errorMessage && !isLockedOut) {
															setErrorMessage("");
														}
													}}
													className={`h-10.5 pl-10 rounded-xl bg-white text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all dark:bg-white [&:-webkit-autofill]:shadow-[inset_0_0_0px_1000px_#ffffff] [&:-webkit-autofill]:[-webkit-text-fill-color:#0b1b3a] [&:-webkit-autofill:hover]:shadow-[inset_0_0_0px_1000px_#ffffff] [&:-webkit-autofill:focus]:shadow-[inset_0_0_0px_1000px_#ffffff] ${
														fieldErrors.username
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-[#e3ebf8] focus:border-[#2f6bff] focus:ring-1 focus:ring-[#2f6bff]/25"
													}`}
													autoComplete="username"
												/>
											</div>
										</div>

										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="password" className="text-xs font-medium text-slate-700">
													Password <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
												</Label>
												<div className="flex items-center gap-3">
													{fieldErrors.password && (
														<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
															<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
															{fieldErrors.password}
														</span>
													)}
													<button
														type="button"
														className="text-[11px] font-medium text-[#2f6bff] hover:underline cursor-pointer"
														onClick={() => {
															setMode("forgot");
															if (username.trim()) {
																setEmail(username.trim());
															}
															setNewPassword("");
															setConfirmPassword("");
															setOtp("");
															setIsOtpVerified(false);
															setResetToken("");
															setErrorMessage("");
															setInfoMessage("");
															setFieldErrors({});
														}}
													>
														Forgot password?
													</button>
												</div>
											</div>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="password"
													type={showPassword ? "text" : "password"}
													placeholder="Enter your password"
													value={password}
													onChange={(e) => {
														setPassword(e.target.value);
														clearFieldError("password");
														if (errorMessage && !isLockedOut) {
															setErrorMessage("");
														}
													}}
													onInput={() => {
														clearFieldError("password");
														if (errorMessage && !isLockedOut) {
															setErrorMessage("");
														}
													}}
													className={`h-10.5 pl-10 pr-10 rounded-xl bg-white text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all dark:bg-white [&:-webkit-autofill]:shadow-[inset_0_0_0px_1000px_#ffffff] [&:-webkit-autofill]:[-webkit-text-fill-color:#0b1b3a] [&:-webkit-autofill:hover]:shadow-[inset_0_0_0px_1000px_#ffffff] [&:-webkit-autofill:focus]:shadow-[inset_0_0_0px_1000px_#ffffff] ${
														fieldErrors.password
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-[#e3ebf8] focus:border-[#2f6bff] focus:ring-1 focus:ring-[#2f6bff]/25"
													}`}
													autoComplete="current-password"
												/>
												<button
													type="button"
													onClick={() => setShowPassword(!showPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-slate-500 hover:text-[#0b1b3a] cursor-pointer"
												>
													{showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</button>
											</div>
										</div>
									</>
								)}

								{/* Forgot Password Mode */}
								{mode === "forgot" && (
									<div className="space-y-1.5">
										<div className="flex items-center justify-between">
											<Label htmlFor="email" className="text-xs font-medium text-slate-700">
												Email Address or Username <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
											</Label>
											{fieldErrors.email && (
												<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
													<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
													{fieldErrors.email}
												</span>
											)}
										</div>
										<div className="relative">
											<Mail className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
											<Input
												id="email"
												type="text"
												placeholder="name@company.com or username"
												value={email}
												onChange={(e) => {
													const val = e.target.value;
													setEmail(val);
													clearFieldError("email");
													if (errorMessage && !isLockedOut) {
														setErrorMessage("");
													}
												}}
												onInput={() => {
													clearFieldError("email");
													if (errorMessage && !isLockedOut) {
														setErrorMessage("");
													}
												}}
												className={`h-10.5 pl-10 rounded-xl bg-[#f7f9ff] text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all ${
													fieldErrors.email
														? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
														: "border-[#e3ebf8] focus:border-[#2f6bff] focus:ring-1 focus:ring-[#2f6bff]/25"
												}`}
												autoComplete="email username"
											/>
										</div>
									</div>
								)}

								{/* Forgot Username Mode */}
								{mode === "forgot_username" && (
									<div className="space-y-1.5">
										<div className="flex items-center justify-between">
											<Label htmlFor="forgot-user-email" className="text-xs font-medium text-slate-700">
												Email Address <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
											</Label>
											{fieldErrors.email && (
												<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
													<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
													{fieldErrors.email}
												</span>
											)}
										</div>
										<div className="relative">
											<Mail className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
											<Input
												id="forgot-user-email"
												type="email"
												placeholder="name@company.com"
												value={email}
												onChange={(e) => {
													const val = e.target.value;
													setEmail(val);
													clearFieldError("email");
													if (errorMessage && !isLockedOut) {
														setErrorMessage("");
													}
												}}
												onInput={() => {
													clearFieldError("email");
													if (errorMessage && !isLockedOut) {
														setErrorMessage("");
													}
												}}
												className={`h-10.5 pl-10 rounded-xl bg-[#f7f9ff] text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all ${
													fieldErrors.email
														? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
														: "border-[#e3ebf8] focus:border-[#2f6bff] focus:ring-1 focus:ring-[#2f6bff]/25"
												}`}
												autoComplete="email"
											/>
										</div>
									</div>
								)}

								{/* Reset Password & Force Change Password Mode */}
								{(mode === "reset" || mode === "force_change_password") && (
									<>
										{mode === "reset" && (
											<>
												<div className="space-y-1.5">
													<Label className="text-xs font-medium text-slate-700">Target Account</Label>
													<div className="flex items-center justify-between rounded-xl border border-[#e3ebf8] bg-[#f7f9ff] px-3.5 py-2 text-xs text-slate-600">
														<span className="truncate">{email || "Account"}</span>
														<button
															type="button"
															onClick={() => {
																setMode("forgot");
																setErrorMessage("");
																setInfoMessage("");
																setFieldErrors({});
															}}
															className="text-xs font-semibold text-[#2f6bff] hover:underline cursor-pointer"
														>
															Change
														</button>
													</div>
												</div>

												<div className="space-y-1.5">
													<div className="flex items-center justify-between">
														<Label htmlFor="otp" className="text-xs font-medium text-slate-700">
															Verification OTP Code <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
														</Label>
														<div className="flex items-center gap-2">
															{fieldErrors.otp && (
																<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
																	<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
																	{fieldErrors.otp}
																</span>
															)}
															{isOtpVerified ? (
																<span className="flex items-center gap-1 text-[11px] font-semibold text-emerald-700">
																	<CheckCircle2 className="h-3.5 w-3.5" /> Verified
																</span>
															) : (
																<button
																	type="button"
														disabled={isSendingOtp || resendCooldown > 0}
														onClick={handleResendOtp}
														className="text-[11px] font-medium text-[#2f6bff] hover:underline disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
													>
														{resendCooldown > 0 ? `Resend in ${resendCooldown}s` : "Resend Code"}
														</button>
															)}
														</div>
													</div>
													<div className="flex gap-2">
														<div className="relative flex-1">
															<KeyRound className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
															<Input
																id="otp"
																type="text"
																placeholder="6-digit code"
																maxLength={6}
																value={otp}
																disabled={isOtpVerified}
																onChange={(e) => {
																	setOtp(e.target.value.replace(/\D/g, ""));
																	clearFieldError("otp");
																}}
																className={`h-10.5 pl-10 rounded-xl bg-[#f7f9ff] font-mono tracking-widest text-sm text-[#0b1b3a] placeholder:tracking-normal transition-all disabled:opacity-50 ${
																	fieldErrors.otp
																		? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
																		: "border-[#e3ebf8] focus:border-[#2f6bff]"
																}`}
															/>
														</div>
														{!isOtpVerified && (
															<Button
																type="button"
																disabled={isVerifyingOtp}
																onClick={handleVerifyOtp}
																className="h-10.5 px-4 rounded-xl bg-[#eef3ff] text-xs font-semibold text-[#2f6bff] hover:bg-[#e0e9ff] border border-[#d5e2ff] cursor-pointer"
															>
																{isVerifyingOtp ? "Verifying..." : "Verify Code"}
															</Button>
														)}
													</div>
												</div>
											</>
										)}

										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="newPassword" className="text-xs font-medium text-slate-700">
													New Password <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
												</Label>
												{fieldErrors.newPassword && (
													<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
														<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
														{fieldErrors.newPassword}
													</span>
												)}
											</div>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="newPassword"
													type={showNewPassword ? "text" : "password"}
													placeholder="Enter new password"
													value={newPassword}
													onChange={(e) => {
														setNewPassword(e.target.value);
														clearFieldError("newPassword");
													}}
													className={`h-10.5 pl-10 pr-10 rounded-xl bg-[#f7f9ff] text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all ${
														fieldErrors.newPassword
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-[#e3ebf8] focus:border-[#2f6bff]"
													}`}
												/>
												<button
													type="button"
													onClick={() => setShowNewPassword(!showNewPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-slate-500 hover:text-[#0b1b3a] cursor-pointer"
												>
													{showNewPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</button>
											</div>
										</div>

										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="confirmPassword" className="text-xs font-medium text-slate-700">
													Confirm New Password <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
												</Label>
												{fieldErrors.confirmPassword && (
													<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
														<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
														{fieldErrors.confirmPassword}
													</span>
												)}
											</div>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="confirmPassword"
													type={showConfirmPassword ? "text" : "password"}
													placeholder="Re-enter new password"
													value={confirmPassword}
													onChange={(e) => {
														setConfirmPassword(e.target.value);
														clearFieldError("confirmPassword");
													}}
													className={`h-10.5 pl-10 pr-10 rounded-xl bg-[#f7f9ff] text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all ${
														fieldErrors.confirmPassword
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-[#e3ebf8] focus:border-[#2f6bff]"
													}`}
												/>
												<button
													type="button"
													onClick={() => setShowConfirmPassword(!showConfirmPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-slate-500 hover:text-[#0b1b3a] cursor-pointer"
												>
													{showConfirmPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</button>
											</div>
										</div>

									</>
								)}

								<Button
									type="submit"
									className="mt-2 h-11 w-full rounded-full bg-[#2f6bff] hover:bg-[#2457d6] font-bold text-white shadow-[0_10px_24px_rgba(47,107,255,0.28)] transition-all cursor-pointer disabled:opacity-40"
									disabled={
										isLoading ||
										isLoggingIn ||
										isLockedOut ||
										isSendingOtp ||
										isRetrievingUsername ||
										(mode === "reset" && isResetting) ||
										(mode === "force_change_password" && isChangingTempPassword)
									}
								>
									{isLockedOut
										? `Try again in ${lockoutCountdown}`
										: mode === "login"
											? isLoading || isLoggingIn
												? "Signing in..."
												: "Sign In"
											: mode === "forgot"
												? isSendingOtp
													? "Sending OTP..."
													: "Send Verification Code"
												: mode === "forgot_username"
													? isRetrievingUsername
														? "Retrieving..."
														: "Retrieve Username"
													: mode === "force_change_password"
														? isChangingTempPassword
															? "Setting Password..."
															: "Set Password & Continue"
														: isResetting
															? "Updating Password..."
															: "Update Password"}
								</Button>

								{mode === "login" && (
									<div className="pt-2 text-center text-xs">
										<button
											type="button"
											className="font-medium text-[#2f6bff] hover:underline transition-colors cursor-pointer"
											onClick={() => {
												setMode("forgot_username");
												setEmail("");
												setErrorMessage("");
												setInfoMessage("");
												setFieldErrors({});
											}}
										>
											Forgot your username?
										</button>
									</div>
								)}							</form>
						</div>
					</section>
					</div>
				</div>
			</main>

			{/* Minimal Sleek Footer */}
			<footer className="relative z-10 mx-auto w-full max-w-[1400px] border-t border-[#e6edf8] px-6 py-4 text-center text-xs text-slate-500">
				<span>{branding.footerText}</span>
			</footer>
		</div>
	);
}
