import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { COMPANY_LOGO, COMPANY_NAME } from "@/lib/constants/config";
import {
	getErrorMessage,
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
import { useEffect, useState } from "react";

type AuthMode = "login" | "forgot" | "reset" | "forgot_username";

// Must match the server's per-account forgot-password cooldown; a resend inside it is
// silently dropped (generic response, no email).
const OTP_RESEND_COOLDOWN_SECONDS = 120;

export default function LoginView() {
	const [mode, setMode] = useState<AuthMode>("login");
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
	const [lockoutEndsAt, setLockoutEndsAt] = useState<number | null>(null);
	const [lockoutReason, setLockoutReason] = useState("");
	// Account lockouts belong to one username; network lockouts (per IP) apply to every username.
	const [lockoutUsername, setLockoutUsername] = useState<string | null>(null);
	const [now, setNow] = useState(() => Date.now());

	const [login, { isLoading: isLoggingIn }] = useLoginMutation();
	const [forgotPassword, { isLoading: isSendingOtp }] = useForgotPasswordMutation();
	const [verifyOtp, { isLoading: isVerifyingOtp }] = useVerifyOTPMutation();
	const [resetPassword, { isLoading: isResetting }] = useResetPasswordMutation();
	const [forgotUsername, { isLoading: isRetrievingUsername }] = useForgotUsernameMutation();

	useEffect(() => {
		if (resendCooldown <= 0) return;
		const timer = setInterval(() => setResendCooldown((c) => c - 1), 1000);
		return () => clearInterval(timer);
	}, [resendCooldown]);

	useEffect(() => {
		if (lockoutEndsAt === null) return;
		const tick = () => {
			const current = Date.now();
			setNow(current);
			if (current >= lockoutEndsAt) {
				setLockoutEndsAt(null);
				setLockoutReason("");
				setErrorMessage("");
			}
		};
		tick();
		const timer = setInterval(tick, 1000);
		return () => clearInterval(timer);
	}, [lockoutEndsAt]);

	const lockoutSecondsLeft = lockoutEndsAt === null ? 0 : Math.max(0, Math.ceil((lockoutEndsAt - now) / 1000));
	const isLockedOut = mode === "login" && lockoutSecondsLeft > 0;
	const lockoutCountdown = `${Math.floor(lockoutSecondsLeft / 60)}:${String(lockoutSecondsLeft % 60).padStart(2, "0")}`;

	const has8Chars = newPassword.length >= 8;
	const hasUpper = /[A-Z]/.test(newPassword);
	const hasLower = /[a-z]/.test(newPassword);
	const hasDigit = /[0-9]/.test(newPassword);
	const hasSpecial = /[^A-Za-z0-9]/.test(newPassword);
	const passwordsMatch = newPassword.length > 0 && newPassword === confirmPassword;

	const handleResendOtp = async () => {
		if (isSendingOtp) return;
		if (!email.trim()) {
			setErrorMessage("Please enter your email address first");
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
		if (cleanOtp.length < 6) {
			setErrorMessage("Please enter the 6-digit OTP code");
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
		setErrorMessage("");
		setInfoMessage("");

		if (mode === "forgot" && !email.trim()) {
			setErrorMessage("Email address or username is required");
			return;
		}
		if (mode === "forgot_username" && !email.trim()) {
			setErrorMessage("Email address is required");
			return;
		}
		if (mode === "reset") {
			if (!isOtpVerified || !resetToken.trim()) {
				setErrorMessage("Please click 'Verify OTP' to verify your code before setting password");
				return;
			}
			if (newPassword !== confirmPassword) {
				setErrorMessage("Passwords do not match");
				return;
			}
			if (!has8Chars || !hasUpper || !hasLower || !hasDigit || !hasSpecial) {
				setErrorMessage("Password must be at least 8 characters and include uppercase, lowercase, a number, and a symbol");
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
				window.location.assign(target);
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
			setLockoutEndsAt(null);
			setLockoutReason("");
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
			const retryAfterSeconds = (error as { retryAfterSeconds?: number } | undefined)?.retryAfterSeconds;
			if (mode === "login" && retryAfterSeconds) {
				setLockoutReason(message.split(/(?<=\.)\s/)[0]);
				setLockoutEndsAt(Date.now() + retryAfterSeconds * 1000);
				setLockoutUsername(/from this network/i.test(message) ? null : username.trim().toLowerCase());
			}
		} finally {
			setIsLoading(false);
		}
	};

	return (
		<div className="relative flex min-h-screen flex-col justify-between overflow-x-hidden overflow-y-auto bg-[#090b12] font-sans text-slate-100 selection:bg-[#45f3ff] selection:text-black">
			{/* Ambient Gradient Glows */}
			<div className="pointer-events-none absolute -top-40 left-1/2 -translate-x-1/2 h-[500px] w-[800px] rounded-full bg-gradient-to-tr from-indigo-500/15 via-[#45f3ff]/15 to-transparent blur-[140px]" />
			<div className="pointer-events-none absolute right-[-5%] bottom-[-10%] h-[420px] w-[420px] rounded-full bg-blue-600/10 blur-[130px]" />
			<div className="pointer-events-none absolute inset-0 bg-[radial-gradient(#ffffff08_1px,transparent_1px)] [background-size:24px_24px] opacity-70" />

			{/* Floating Top Navigation Header */}
			<header className="relative z-20 mx-auto flex w-full max-w-[1400px] items-center justify-between px-6 py-5">
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
						<img src={COMPANY_LOGO} alt={COMPANY_NAME} className="h-7 w-auto object-contain" />
						<span className="text-sm font-bold tracking-tight text-white">{COMPANY_NAME}</span>
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

			{/* Main Center Content */}
			<main className="relative z-10 mx-auto flex w-full max-w-[1300px] flex-1 items-center justify-center px-6 py-8">
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
												: "Reset Password"}
								</h2>
								<p className="text-xs leading-relaxed text-slate-400">
									{mode === "login"
										? "Enter your credentials to access your organization's AI control plane."
										: mode === "forgot"
											? "Enter your registered email or username. We'll send a one-time verification code."
											: mode === "forgot_username"
												? "Enter your registered email address to retrieve your username."
												: "Enter the OTP sent to your email, verify it, and choose a new password."}
								</p>
							</div>

							{errorMessage && (
								<div className="mb-4 flex items-center gap-2.5 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300">
									<ShieldAlert className="h-4 w-4 shrink-0 text-rose-400" />
									<span>{isLockedOut ? `${lockoutReason} Try again in ${lockoutCountdown}.` : errorMessage}</span>
								</div>
							)}

							{infoMessage && (
								<div className="mb-4 flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-200">
									<CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-400" />
									<span>{infoMessage}</span>
								</div>
							)}

							<form onSubmit={handleSubmit} className="space-y-4">
								{/* Login Mode Fields */}
								{mode === "login" && (
									<>
										<div className="space-y-1.5">
											<Label htmlFor="username" className="text-xs font-medium text-slate-200">
												Username
											</Label>
											<div className="relative">
												<User className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="username"
													type="text"
													placeholder="Enter your username"
													value={username}
													onChange={(e) => {
														setUsername(e.target.value);
														if (lockoutUsername && e.target.value.trim().toLowerCase() !== lockoutUsername) {
															setLockoutEndsAt(null);
														}
													}}
													required
													className="h-10.5 pl-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40 transition-all"
													autoComplete="username"
												/>
											</div>
										</div>

										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="password" className="text-xs font-medium text-slate-200">
													Password
												</Label>
												<button
													type="button"
													className="text-[11px] font-medium text-[#45f3ff] hover:underline cursor-pointer"
													onClick={() => {
														setMode("forgot");
														setEmail("");
														setNewPassword("");
														setConfirmPassword("");
														setOtp("");
														setIsOtpVerified(false);
														setResetToken("");
														setErrorMessage("");
														setInfoMessage("");
													}}
												>
													Forgot password?
												</button>
											</div>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="password"
													type={showPassword ? "text" : "password"}
													placeholder="Enter your password"
													value={password}
													onChange={(e) => setPassword(e.target.value)}
													required
													className="h-10.5 pl-10 pr-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40 transition-all"
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
										<Label htmlFor="email" className="text-xs font-medium text-slate-200">
											Email Address or Username
										</Label>
										<div className="relative">
											<Mail className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
											<Input
												id="email"
												type="text"
												placeholder="name@company.com or username"
												value={email}
												onChange={(e) => setEmail(e.target.value)}
												required
												className="h-10.5 pl-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40 transition-all"
												autoComplete="email username"
											/>
										</div>
									</div>
								)}

								{/* Forgot Username Mode */}
								{mode === "forgot_username" && (
									<div className="space-y-1.5">
										<Label htmlFor="forgot-user-email" className="text-xs font-medium text-slate-200">
											Email Address
										</Label>
										<div className="relative">
											<Mail className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
											<Input
												id="forgot-user-email"
												type="email"
												placeholder="name@company.com"
												value={email}
												onChange={(e) => setEmail(e.target.value)}
												required
												className="h-10.5 pl-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40 transition-all"
												autoComplete="email"
											/>
										</div>
									</div>
								)}

								{/* Reset Password Mode */}
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
													Verification OTP Code
												</Label>
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
														onChange={(e) => setOtp(e.target.value.replace(/\D/g, ""))}
														className="h-10.5 pl-10 rounded-xl border-white/10 bg-black/40 font-mono tracking-widest text-sm text-white placeholder:tracking-normal focus:border-[#45f3ff] disabled:opacity-50"
													/>
												</div>
												{!isOtpVerified && (
													<Button
														type="button"
														disabled={otp.trim().length < 6 || isVerifyingOtp}
														onClick={handleVerifyOtp}
														className="h-10.5 px-4 rounded-xl bg-white/10 text-xs font-semibold text-white hover:bg-white/20 border border-white/15 cursor-pointer"
													>
														{isVerifyingOtp ? "Verifying..." : "Verify Code"}
													</Button>
												)}
											</div>
										</div>

										<div className="space-y-1.5">
											<Label htmlFor="newPassword" className="text-xs font-medium text-slate-200">
												New Password
											</Label>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="newPassword"
													type={showNewPassword ? "text" : "password"}
													placeholder="Enter new password"
													value={newPassword}
													onChange={(e) => setNewPassword(e.target.value)}
													required
													className="h-10.5 pl-10 pr-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff]"
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
											<Label htmlFor="confirmPassword" className="text-xs font-medium text-slate-200">
												Confirm New Password
											</Label>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="confirmPassword"
													type={showConfirmPassword ? "text" : "password"}
													placeholder="Re-enter new password"
													value={confirmPassword}
													onChange={(e) => setConfirmPassword(e.target.value)}
													required
													className="h-10.5 pl-10 pr-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff]"
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

										{/* Password requirements checklist */}
										<div className="flex flex-wrap items-center justify-between gap-1.5 rounded-xl border border-white/5 bg-black/20 p-2.5 text-[11px]">
											<span className={has8Chars ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
												<Check className="h-3 w-3" /> 8+ chars
											</span>
											<span className={hasUpper ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
												<Check className="h-3 w-3" /> A-Z
											</span>
											<span className={hasLower ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
												<Check className="h-3 w-3" /> a-z
											</span>
											<span className={hasDigit ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
												<Check className="h-3 w-3" /> 0-9
											</span>
											<span className={hasSpecial ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
												<Check className="h-3 w-3" /> Symbol
											</span>
											<span className={passwordsMatch ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
												<Check className="h-3 w-3" /> Match
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
										(mode === "reset" && (!isOtpVerified || !has8Chars || !hasUpper || !hasLower || !hasDigit || !hasSpecial || !passwordsMatch || isResetting))
									}
								>
									{mode === "login"
										? isLoading || isLoggingIn
											? "Signing in..."
											: isLockedOut
												? `Try again in ${lockoutCountdown}`
												: "Sign In"
										: mode === "forgot"
											? isSendingOtp
												? "Sending OTP..."
												: "Send Verification Code"
											: mode === "forgot_username"
												? isRetrievingUsername
													? "Retrieving..."
													: "Retrieve Username"
												: isResetting
													? "Updating password..."
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
											}}
										>
											Forgot your username?
										</button>
									</div>
								)}
							</form>
						</div>
					</section>
				</div>
			</main>

			{/* Minimal Sleek Footer */}
			<footer className="relative z-10 mx-auto w-full max-w-[1400px] border-t border-white/5 px-6 py-4 text-center text-xs text-slate-500">
				<span>© {new Date().getFullYear()} {COMPANY_NAME}. All rights reserved. Enterprise AI Governance Platform.</span>
			</footer>
		</div>
	);
}
