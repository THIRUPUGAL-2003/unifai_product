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
	Check,
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
			if (params.get("mode") === "force_change_password") {
				return "force_change_password";
			}
		}
		return "login";
	});
	const [username, setUsername] = useState("");
	const [email, setEmail] = useState("");
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

	// Active identifier being typed/used across modes
	const activeInputIdentifier = (
		mode === "login"
			? username
			: email || username
	).trim().toLowerCase();

	// Lockout applies when the entered username/email matches the blocked account across ANY mode
	const isCurrentAccountLocked = Boolean(
		lockedAccount &&
		now < lockedAccount.endsAt &&
		(
			(activeInputIdentifier && activeInputIdentifier === lockedAccount.identifier) ||
			(username.trim().toLowerCase() && username.trim().toLowerCase() === lockedAccount.identifier) ||
			(email.trim().toLowerCase() && email.trim().toLowerCase() === lockedAccount.identifier)
		)
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
			(email.trim().toLowerCase() === lockedAccount.identifier || username.trim().toLowerCase() === lockedAccount.identifier) &&
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
				(cleanInput.toLowerCase() === lockedAccount.identifier || username.trim().toLowerCase() === lockedAccount.identifier) &&
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
				const targetId = (mode === "login" ? username : email || username).trim().toLowerCase();
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
		<div className="relative flex h-screen flex-col overflow-hidden bg-[#090b12] font-sans text-slate-100 selection:bg-[#45f3ff] selection:text-black">
			{/* Ambient Gradient Glows */}
			<div className="pointer-events-none absolute -top-40 left-1/2 -translate-x-1/2 h-[500px] w-[800px] rounded-full bg-gradient-to-tr from-indigo-500/15 via-[#45f3ff]/15 to-transparent blur-[140px]" />
			<div className="pointer-events-none absolute right-[-5%] bottom-[-10%] h-[420px] w-[420px] rounded-full bg-blue-600/10 blur-[130px]" />
			<div className="pointer-events-none absolute inset-0 bg-[radial-gradient(#ffffff08_1px,transparent_1px)] [background-size:24px_24px] opacity-70" />

			{/* Fixed Top Navigation Header */}
			<header className="relative z-20 flex-shrink-0 mx-auto flex w-full max-w-[1400px] items-center justify-between px-6 py-5">
				<div className="flex items-center gap-4">
					<button
						type="button"
						onClick={() => window.location.assign("/")}
						className="group inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-4 py-1.5 text-xs font-medium text-slate-300 backdrop-blur-md transition-all hover:border-white/20 hover:bg-white/10 hover:text-white cursor-pointer shadow-sm"
						title="Return to Landing Page"
					>
						<ArrowLeft className="h-3.5 w-3.5 transition-transform group-hover:-translate-x-1 text-[#45f3ff]" />
						<span>Back to Home</span>
					</button>

					<div className="hidden h-5 w-px bg-white/15 sm:block" />

					<div className="hidden sm:flex items-center gap-2.5">
						<img src={branding.companyLogo} alt={branding.companyName} className="h-7 w-auto object-contain" />
						<span className="text-sm font-bold tracking-tight text-white">{branding.companyName}</span>
					</div>
				</div>

				<div className="flex items-center gap-3">
					<span className="hidden text-xs text-slate-400 md:inline">Don't have an account?</span>
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={() => window.location.assign("/signup")}
						className="h-8.5 rounded-lg border-white/15 bg-white/[0.04] px-4 text-xs font-semibold text-white hover:border-[#45f3ff]/60 hover:bg-[#45f3ff]/10 hover:text-[#45f3ff] transition-all cursor-pointer shadow-sm"
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
							<div className="inline-flex items-center gap-2 rounded-full border border-[#45f3ff]/20 bg-[#45f3ff]/10 px-3.5 py-1 text-xs font-semibold tracking-wide text-[#89f7ff]">
								<span className="h-2 w-2 rounded-full bg-emerald-400 shadow-[0_0_8px_#34d399]" />
								<span>Gateway Active • Zero-Trust Control Plane</span>
							</div>

							<h1 className="text-4xl font-extrabold tracking-tight text-white sm:text-5xl leading-[1.12]">
								Enterprise AI,{" "}
								<span className="bg-gradient-to-r from-[#45f3ff] via-[#7dd3fc] to-white bg-clip-text text-transparent">
									Governed & Unified.
								</span>
							</h1>

							<p className="max-w-lg text-sm text-slate-400 leading-relaxed">
								Access your centralized AI control plane. Orchestrate 100+ LLMs, manage Prompt Repositories with multimodal image support, enforce token budgets, and monitor live audit logs.
							</p>
						</div>

						{/* Feature Benefit Cards */}
						<div className="space-y-3.5 max-w-lg">
							<div className="flex items-start gap-3.5 rounded-xl border border-white/8 bg-white/[0.02] p-4 backdrop-blur-md transition-colors hover:border-white/15 hover:bg-white/[0.04]">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#45f3ff]/10 text-[#45f3ff]">
									<Cpu className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-white">Unified LLM Gateway</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Route dynamically through OpenAI, Claude, Gemini, DeepSeek, and local models with a single API key.
									</p>
								</div>
							</div>

							<div className="flex items-start gap-3.5 rounded-xl border border-white/8 bg-white/[0.02] p-4 backdrop-blur-md transition-colors hover:border-white/15 hover:bg-white/[0.04]">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-400">
									<Sparkles className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-white">Prompt Repository & Vision</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										ChatGPT-style multimodal image support, folder-based team access, and persistent chat sessions.
									</p>
								</div>
							</div>

							<div className="flex items-start gap-3.5 rounded-xl border border-white/8 bg-white/[0.02] p-4 backdrop-blur-md transition-colors hover:border-white/15 hover:bg-white/[0.04]">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-indigo-500/10 text-indigo-400">
									<Shield className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-white">Zero-Trust Guardrails & Budgets</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Enforce personal user and team spending limits, real-time PII redaction, and compliance auditing.
									</p>
								</div>
							</div>
						</div>
					</section>

					{/* Right Column: Sleek Authentication Card */}
					<section className="flex items-center justify-center lg:col-span-6">
						<div className="w-full max-w-[440px] rounded-2xl border border-white/10 bg-[#0f121d]/90 p-8 shadow-[0_25px_70px_rgba(0,0,0,0.65)] backdrop-blur-2xl">
							{mode !== "login" && mode !== "force_change_password" && (
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
									}}
									className="group mb-4 inline-flex items-center gap-1.5 text-xs font-medium text-slate-400 hover:text-[#45f3ff] transition-colors cursor-pointer"
								>
									<ArrowLeft className="h-3.5 w-3.5 transition-transform group-hover:-translate-x-1" />
									<span>{mode === "reset" ? "Back to forgot password" : "Back to Sign In"}</span>
								</button>
							)}

							<div className="mb-6 space-y-1.5">
								<h2 className="text-2xl font-bold tracking-tight text-white">
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
								<div className="mb-4 flex flex-col gap-1.5 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300 shadow-sm animate-in fade-in duration-150">
									<div className="flex items-center gap-2 font-semibold text-rose-200">
										<ShieldAlert className="h-4 w-4 shrink-0 text-rose-400" />
										<span>{lockedAccount?.reason || "Account Locked Out"}</span>
									</div>
									<p className="text-slate-300 leading-relaxed text-[11px]">
										Too many failed attempts for <span className="font-semibold text-white">"{activeInputIdentifier || lockedAccount?.identifier}"</span>. Please wait{" "}
										<span className="font-mono font-bold text-amber-300">{lockoutCountdown}</span> before trying again.
									</p>
								</div>
							) : errorMessage ? (
								<div className="mb-4 flex items-center gap-2.5 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300">
									<ShieldAlert className="h-4 w-4 shrink-0 text-rose-400" />
									<span>{errorMessage}</span>
								</div>
							) : null}

							{infoMessage && (
								<div className="mb-4 flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-200">
									<CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-400" />
									<span>{infoMessage}</span>
								</div>
							)}

							<form onSubmit={handleSubmit} noValidate className="space-y-4">
								{/* Login Mode Fields */}
								{mode === "login" && (
									<>
										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="username" className="text-xs font-medium text-slate-200">
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
														// Clear previous error if switching away from the locked account
														if (errorMessage && (!lockedAccount || val.trim().toLowerCase() !== lockedAccount.identifier)) {
															setErrorMessage("");
														}
													}}
													className={`h-10.5 pl-10 rounded-xl bg-black/40 text-sm text-white placeholder:text-slate-500 transition-all ${
														fieldErrors.username
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-white/10 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40"
													}`}
													autoComplete="username"
												/>
											</div>
										</div>

										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="password" className="text-xs font-medium text-slate-200">
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
														className="text-[11px] font-medium text-[#45f3ff] hover:underline cursor-pointer"
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
													}}
													className={`h-10.5 pl-10 pr-10 rounded-xl bg-black/40 text-sm text-white placeholder:text-slate-500 transition-all ${
														fieldErrors.password
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-white/10 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40"
													}`}
													autoComplete="current-password"
												/>
												<button
													type="button"
													onClick={() => setShowPassword(!showPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-slate-500 hover:text-white cursor-pointer"
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
											<Label htmlFor="email" className="text-xs font-medium text-slate-200">
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
													setEmail(e.target.value);
													clearFieldError("email");
												}}
												className={`h-10.5 pl-10 rounded-xl bg-black/40 text-sm text-white placeholder:text-slate-500 transition-all ${
													fieldErrors.email
														? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
														: "border-white/10 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40"
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
											<Label htmlFor="forgot-user-email" className="text-xs font-medium text-slate-200">
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
													setEmail(e.target.value);
													clearFieldError("email");
												}}
												className={`h-10.5 pl-10 rounded-xl bg-black/40 text-sm text-white placeholder:text-slate-500 transition-all ${
													fieldErrors.email
														? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
														: "border-white/10 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40"
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
													<Label className="text-xs font-medium text-slate-200">Target Account</Label>
													<div className="flex items-center justify-between rounded-xl border border-white/10 bg-black/30 px-3.5 py-2 text-xs text-slate-300">
														<span className="truncate">{email || "Account"}</span>
														<button
															type="button"
															onClick={() => {
																setMode("forgot");
																setErrorMessage("");
																setInfoMessage("");
																setFieldErrors({});
															}}
															className="text-xs font-semibold text-[#45f3ff] hover:underline cursor-pointer"
														>
															Change
														</button>
													</div>
												</div>

												<div className="space-y-1.5">
													<div className="flex items-center justify-between">
														<Label htmlFor="otp" className="text-xs font-medium text-slate-200">
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
																<span className="flex items-center gap-1 text-[11px] font-semibold text-emerald-400">
																	<CheckCircle2 className="h-3.5 w-3.5" /> Verified
																</span>
															) : (
																<button
																	type="button"
														disabled={isSendingOtp || resendCooldown > 0}
														onClick={handleResendOtp}
														className="text-[11px] font-medium text-[#45f3ff] hover:underline disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
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
																className={`h-10.5 pl-10 rounded-xl bg-black/40 font-mono tracking-widest text-sm text-white placeholder:tracking-normal transition-all disabled:opacity-50 ${
																	fieldErrors.otp
																		? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
																		: "border-white/10 focus:border-[#45f3ff]"
																}`}
															/>
														</div>
														{!isOtpVerified && (
															<Button
																type="button"
																disabled={isVerifyingOtp}
																onClick={handleVerifyOtp}
																className="h-10.5 px-4 rounded-xl bg-white/10 text-xs font-semibold text-white hover:bg-white/20 border border-white/15 cursor-pointer"
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
												<Label htmlFor="newPassword" className="text-xs font-medium text-slate-200">
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
													className={`h-10.5 pl-10 pr-10 rounded-xl bg-black/40 text-sm text-white placeholder:text-slate-500 transition-all ${
														fieldErrors.newPassword
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-white/10 focus:border-[#45f3ff]"
													}`}
												/>
												<button
													type="button"
													onClick={() => setShowNewPassword(!showNewPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-slate-500 hover:text-white cursor-pointer"
												>
													{showNewPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</button>
											</div>
										</div>

										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="confirmPassword" className="text-xs font-medium text-slate-200">
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
													className={`h-10.5 pl-10 pr-10 rounded-xl bg-black/40 text-sm text-white placeholder:text-slate-500 transition-all ${
														fieldErrors.confirmPassword
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-white/10 focus:border-[#45f3ff]"
													}`}
												/>
												<button
													type="button"
													onClick={() => setShowConfirmPassword(!showConfirmPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-slate-500 hover:text-white cursor-pointer"
												>
													{showConfirmPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</button>
											</div>
										</div>

										{/* Password requirements checklist - single compact row */}
										<div className="flex items-center justify-between rounded-xl border border-white/5 bg-black/20 px-3 py-2 text-[11px]">
											<span className={has8Chars ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-nowrap" : "text-slate-500 flex items-center gap-1 whitespace-nowrap"}>
												<Check className="h-3 w-3 shrink-0" /> 8+ chars
											</span>
											<span className={hasUpper ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-nowrap" : "text-slate-500 flex items-center gap-1 whitespace-nowrap"}>
												<Check className="h-3 w-3 shrink-0" /> A-Z
											</span>
											<span className={hasLower ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-nowrap" : "text-slate-500 flex items-center gap-1 whitespace-nowrap"}>
												<Check className="h-3 w-3 shrink-0" /> a-z
											</span>
											<span className={hasDigit ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-nowrap" : "text-slate-500 flex items-center gap-1 whitespace-nowrap"}>
												<Check className="h-3 w-3 shrink-0" /> 0-9
											</span>
											<span className={hasSpecial ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-nowrap" : "text-slate-500 flex items-center gap-1 whitespace-nowrap"}>
												<Check className="h-3 w-3 shrink-0" /> Symbol
											</span>
											<span className={passwordsMatch ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-nowrap" : "text-slate-500 flex items-center gap-1 whitespace-nowrap"}>
												<Check className="h-3 w-3 shrink-0" /> Match
											</span>
										</div>
									</>
								)}

								<Button
									type="submit"
									className="mt-2 h-11 w-full rounded-xl bg-gradient-to-r from-[#45f3ff] to-[#38bdf8] font-bold text-[#090b12] shadow-[0_0_24px_rgba(69,243,255,0.25)] hover:shadow-[0_0_32px_rgba(69,243,255,0.4)] hover:brightness-105 transition-all cursor-pointer disabled:opacity-40"
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
									<div className="pt-2 text-center text-xs text-slate-400">
										<button
											type="button"
											className="hover:text-slate-200 transition-colors cursor-pointer"
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
			<footer className="relative z-10 mx-auto w-full max-w-[1400px] border-t border-white/5 px-6 py-4 text-center text-xs text-slate-500">
				<span>{branding.footerText}</span>
			</footer>
		</div>
	);
}
