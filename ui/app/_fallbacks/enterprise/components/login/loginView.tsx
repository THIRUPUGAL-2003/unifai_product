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
import { Activity, Check, CheckCircle2, Eye, EyeOff, Globe, Lock, Shield, ShieldAlert, Upload } from "lucide-react";
import { useEffect, useState } from "react";

type AuthMode = "login" | "forgot" | "reset" | "forgot_username";

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
		// U4: Guard against double-click sending duplicate OTP requests
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
			setResendCooldown(60);
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

		// Early-return validation (before setting isLoading so button never stays stuck)
		if (mode === "forgot" && !email.trim()) {
			setErrorMessage("Email address is required");
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
				setResendCooldown(60);
				return;
			}
			if (mode === "forgot_username") {
				const result = await forgotUsername({ email: email.trim() }).unwrap();
				setInfoMessage(result.message || "If an account matches, your username was sent to your email.");
				return;
			}
			// Reset password mode
			const cleanInput = email.trim();
			const result = await resetPassword({
				email: cleanInput,
				username: cleanInput,
				reset_token: resetToken.trim(),
				new_password: newPassword,
			}).unwrap();
			setInfoMessage(result.message || "Password updated. Sign in with your new password.");
			setMode("login");
			// U3: Clear all form state after successful password reset
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
			}
		} finally {
			setIsLoading(false);
		}
	};

	return (
		<div className="relative flex min-h-screen flex-col overflow-hidden bg-[#07080c] font-sans text-[#c5c6c7]">
			<div className="ug-login-grid pointer-events-none absolute inset-0" />
			<div className="pointer-events-none absolute -top-32 left-[-8%] h-[520px] w-[520px] rounded-full bg-[#45f3ff]/12 blur-[140px]" />
			<div className="pointer-events-none absolute right-[-10%] bottom-[-18%] h-[480px] w-[480px] rounded-full bg-[#3b82f6]/16 blur-[150px]" />

			<div className="relative z-10 mx-auto flex w-full max-w-[1440px] flex-1 flex-col px-5 py-6 lg:px-10 lg:py-8">
				<header className="flex items-center justify-between">
					<div className="flex min-w-0 items-center gap-4">
						<img
							src={COMPANY_LOGO}
							alt={COMPANY_NAME}
							className="h-14 sm:h-[4.25rem] w-auto max-w-[min(56vw,280px)] shrink-0 object-contain object-left"
						/>
						<div className="hidden h-10 w-px shrink-0 bg-white/15 sm:block" />
						<div className="min-w-0">
							<p className="text-xl font-bold tracking-tight text-white">UnifAI Guard</p>
							<p className="text-[11px] tracking-[0.16em] text-[#7d8896] uppercase">{COMPANY_NAME}</p>
						</div>
					</div>
					<div className="flex items-center gap-3">
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={() => window.location.assign("/signup")}
							className="h-8 border-[#45f3ff]/40 bg-[#45f3ff]/10 px-3.5 text-xs font-semibold text-[#45f3ff] hover:bg-[#45f3ff]/20 hover:border-[#45f3ff] cursor-pointer"
						>
							Sign Up
						</Button>
						<div className="hidden items-center gap-2 rounded-full border border-[#1f2833] bg-[#12141c]/70 px-3 py-1.5 text-xs text-[#8b949e] sm:flex">
							<span className="h-1.5 w-1.5 rounded-full bg-[#45f3ff] shadow-[0_0_8px_#45f3ff]" />
							Live policy sync
						</div>
					</div>
				</header>

				<main className="grid flex-1 items-center gap-10 pt-8 lg:grid-cols-12 lg:gap-8 lg:pt-4">
					<section className="lg:col-span-7">
						<div className="mb-8 max-w-xl space-y-4">
							<div className="inline-flex items-center rounded-full border border-[#45f3ff]/20 bg-[#45f3ff]/10 px-3 py-1 text-[11px] font-semibold tracking-[0.16em] text-[#89f7ff] uppercase">
								Browser AI security
							</div>
							<h1 className="text-4xl leading-[1.08] font-extrabold text-white sm:text-5xl lg:text-[56px]">
								See every risk.
								<br />
								<span className="bg-gradient-to-r from-[#45f3ff] to-[#7dd3fc] bg-clip-text text-transparent">
									Block it in real time.
								</span>
							</h1>
							<p className="max-w-lg text-sm leading-7 text-[#8b949e] sm:text-base">
								One dashboard for employee AI chats, website locks, upload policy, and Guard agents — no restart, no extra
								tools.
							</p>
						</div>

						<div className="ug-login-stage relative mx-auto h-[360px] w-full max-w-[640px] sm:h-[420px] lg:mx-0">
							<div className="ug-login-glow pointer-events-none absolute inset-x-10 bottom-6 h-24 rounded-full bg-[#45f3ff]/20 blur-3xl" />

							<div className="ug-login-plane relative h-full w-full">
								<div className="ug-login-glass absolute inset-x-6 top-10 overflow-hidden rounded-3xl sm:inset-x-10">
									<div className="flex items-center gap-2 border-b border-white/5 px-4 py-3">
										<span className="h-2.5 w-2.5 rounded-full bg-[#ff5f57]" />
										<span className="h-2.5 w-2.5 rounded-full bg-[#febc2e]" />
										<span className="h-2.5 w-2.5 rounded-full bg-[#28c840]" />
										<span className="ml-3 text-xs font-medium text-white/80">Browser AI Control</span>
									</div>
									<div className="grid grid-cols-3 gap-3 p-4">
										<div className="rounded-2xl border border-white/10 bg-white/5 p-3">
											<p className="text-[10px] tracking-wider text-[#8b949e] uppercase">Targets</p>
											<p className="mt-1 text-xl font-bold text-white">Any domain</p>
										</div>
										<div className="rounded-2xl border border-rose-500/20 bg-rose-500/10 p-3">
											<p className="text-[10px] tracking-wider text-rose-300 uppercase">Locks</p>
											<p className="mt-1 text-xl font-bold text-white">Full site</p>
										</div>
										<div className="rounded-2xl border border-[#45f3ff]/20 bg-[#45f3ff]/10 p-3">
											<p className="text-[10px] tracking-wider text-[#89f7ff] uppercase">Uploads</p>
											<p className="mt-1 text-xl font-bold text-white">Policy on</p>
										</div>
									</div>
									<div className="space-y-2 px-4 pb-5">
										<div className="flex items-center justify-between rounded-xl border border-white/10 bg-[#0b0c10]/70 px-3 py-2.5">
											<div className="flex items-center gap-2 text-sm text-white">
												<Globe className="h-3.5 w-3.5 text-[#45f3ff]" />
												chat.example.com
											</div>
											<span className="text-[10px] font-semibold tracking-wider text-emerald-400 uppercase">Monitored</span>
										</div>
										<div className="flex items-center justify-between rounded-xl border border-rose-500/20 bg-rose-950/40 px-3 py-2.5">
											<div className="flex items-center gap-2 text-sm text-white">
												<Lock className="h-3.5 w-3.5 text-rose-400" />
												social.example.com
											</div>
											<span className="text-[10px] font-semibold tracking-wider text-rose-300 uppercase">Blocked</span>
										</div>
										<div className="flex items-center justify-between rounded-xl border border-amber-500/20 bg-amber-950/30 px-3 py-2.5">
											<div className="flex items-center gap-2 text-sm text-white">
												<Upload className="h-3.5 w-3.5 text-amber-300" />
												file.pdf
											</div>
											<span className="text-[10px] font-semibold tracking-wider text-amber-200 uppercase">Upload block</span>
										</div>
									</div>
								</div>

								<div className="ug-login-chip absolute top-4 left-0 rounded-2xl border border-[#45f3ff]/25 bg-[#0b0c10]/80 px-3 py-2 shadow-[0_12px_40px_rgba(0,0,0,0.45)] backdrop-blur-md">
									<div className="flex items-center gap-2">
										<Shield className="h-3.5 w-3.5 text-[#45f3ff]" />
										<div>
											<p className="text-[11px] font-semibold text-white">Prompt Guard</p>
											<p className="text-[10px] text-[#8b949e]">Secrets blocked live</p>
										</div>
									</div>
								</div>

								<div className="ug-login-chip ug-login-chip-delay absolute top-16 right-0 rounded-2xl border border-white/10 bg-[#0b0c10]/80 px-3 py-2 shadow-[0_12px_40px_rgba(0,0,0,0.45)] backdrop-blur-md">
									<div className="flex items-center gap-2">
										<Activity className="h-3.5 w-3.5 text-[#45f3ff]" />
										<div>
											<p className="text-[11px] font-semibold text-white">12 agents online</p>
											<p className="text-[10px] text-[#8b949e]">No restart required</p>
										</div>
									</div>
								</div>
							</div>
						</div>
					</section>

					<section className="flex items-center justify-center lg:col-span-5">
						<div className="w-full max-w-md rounded-[28px] border border-white/10 bg-[#10131c]/80 p-8 shadow-[0_30px_80px_rgba(0,0,0,0.55)] backdrop-blur-2xl">
							<div className="mb-6 space-y-2">
								<h2 className="text-3xl font-bold text-white">
									{mode === "login"
										? "Sign In"
										: mode === "forgot"
											? "Forgot password"
											: mode === "forgot_username"
												? "Forgot username"
												: "Reset password"}
								</h2>
								<p className="text-sm leading-6 text-[#8b949e]">
									{mode === "login"
										? "Access the Guard control plane for your organization."
										: mode === "forgot"
											? "Enter your registered email ID or username (e.g. admin). We will email you a one-time verification code."
											: mode === "forgot_username"
												? "Enter your registered email ID and we will send your username to your inbox."
												: "Enter the OTP from your email, verify it, and choose a new password."}
								</p>
							</div>

							{errorMessage && (
								<div className="bg-destructive/10 border-destructive/20 text-destructive mb-4 flex items-center gap-2.5 rounded-lg border p-3 text-sm">
									<ShieldAlert className="h-4 w-4 shrink-0" />
									<span>
										{isLockedOut ? `${lockoutReason} Try again in ${lockoutCountdown}.` : errorMessage}
									</span>
								</div>
							)}
							{infoMessage && (
								<div className="mb-4 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-200">
									{infoMessage}
								</div>
							)}

							<form onSubmit={handleSubmit} className="space-y-4">
								{/* Login Mode Fields */}
								{mode === "login" && (
									<>
										<div className="space-y-2">
											<Label htmlFor="username" className="text-xs font-semibold tracking-wider text-white uppercase">
												Username
											</Label>
											<Input
												id="username"
												type="text"
												placeholder="Enter your username"
												value={username}
												onChange={(e) => {
													setUsername(e.target.value);
													setLockoutEndsAt(null);
												}}
												required
												className="h-11 border-[#1f2833]/80 bg-[#07080c]/80 text-sm text-white focus:border-[#45f3ff]"
												autoComplete="username"
											/>
										</div>
										<div className="space-y-2">
											<Label htmlFor="password" className="text-xs font-semibold tracking-wider text-white uppercase">
												Password
											</Label>
											<div className="relative">
												<Input
													id="password"
													type={showPassword ? "text" : "password"}
													placeholder="Enter your password"
													value={password}
													onChange={(e) => setPassword(e.target.value)}
													required
													className="h-11 border-[#1f2833]/80 bg-[#07080c]/80 pr-10 text-sm text-white focus:border-[#45f3ff]"
													autoComplete="current-password"
												/>
												<button
													type="button"
													onClick={() => setShowPassword(!showPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-gray-500 hover:text-white"
												>
													{showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</button>
											</div>
										</div>
									</>
								)}

								{/* Forgot Password Mode */}
								{mode === "forgot" && (
									<div className="space-y-2">
										<Label htmlFor="email" className="text-xs font-semibold tracking-wider text-white uppercase">
											Email ID or Username
										</Label>
										<Input
											id="email"
											type="text"
											placeholder="name@company.com or admin"
											value={email}
											onChange={(e) => setEmail(e.target.value)}
											required
											className="h-11 border-[#1f2833]/80 bg-[#07080c]/80 text-sm text-white focus:border-[#45f3ff]"
											autoComplete="username email"
										/>
									</div>
								)}

								{/* Forgot Username Mode */}
								{mode === "forgot_username" && (
									<div className="space-y-2">
										<Label htmlFor="forgot-user-email" className="text-xs font-semibold tracking-wider text-white uppercase">
											Email ID
										</Label>
										<Input
											id="forgot-user-email"
											type="email"
											placeholder="name@company.com"
											value={email}
											onChange={(e) => setEmail(e.target.value)}
											required
											className="h-11 border-[#1f2833]/80 bg-[#07080c]/80 text-sm text-white focus:border-[#45f3ff]"
											autoComplete="email"
										/>
									</div>
								)}

								{/* Reset Password Mode */}
								{mode === "reset" && (
									<>
										<div className="space-y-1.5">
											<Label className="text-xs font-semibold tracking-wider text-white uppercase">
												Email ID
											</Label>
											<div className="flex items-center justify-between rounded-lg border border-[#1f2833]/80 bg-[#07080c]/60 px-3 py-2 text-sm text-slate-300">
												<span className="truncate">{email || "No email specified"}</span>
												<button
													type="button"
													onClick={() => {
														setMode("forgot");
														setErrorMessage("");
														setInfoMessage("");
													}}
													className="text-xs font-medium text-[#45f3ff] hover:underline"
												>
													Change
												</button>
											</div>
										</div>

										<div className="space-y-2">
											<div className="flex items-center justify-between">
												<Label htmlFor="otp" className="text-xs font-semibold tracking-wider text-white uppercase">
													OTP Code
												</Label>
												{isOtpVerified ? (
													<span className="flex items-center gap-1 text-xs font-semibold text-emerald-400">
														<CheckCircle2 className="h-3.5 w-3.5" /> Verified
													</span>
												) : (
													<button
														type="button"
														disabled={isSendingOtp || resendCooldown > 0}
														onClick={handleResendOtp}
														className="text-xs font-medium text-[#45f3ff] hover:underline disabled:opacity-50 disabled:cursor-not-allowed"
													>
														{resendCooldown > 0 ? `Resend in ${resendCooldown}s` : isSendingOtp ? "Sending..." : "Resend OTP"}
													</button>
												)}
											</div>
											<div className="flex gap-2">
												<Input
													id="otp"
													type="text"
													inputMode="numeric"
													maxLength={6}
													placeholder="6-digit code"
													autoComplete="one-time-code"
													value={otp}
													onChange={(e) => {
														setOtp(e.target.value.replace(/\D/g, ""));
														if (isOtpVerified) setIsOtpVerified(false);
													}}
													disabled={isOtpVerified}
													required
													className="h-11 flex-1 border-[#1f2833]/80 bg-[#07080c]/80 text-sm font-mono tracking-widest text-white focus:border-[#45f3ff] disabled:opacity-85 disabled:border-emerald-500/50"
												/>
												{!isOtpVerified && (
													<Button
														type="button"
														onClick={handleVerifyOtp}
														disabled={otp.trim().length < 6 || isVerifyingOtp}
														className="h-11 px-4 bg-[#45f3ff]/20 text-[#45f3ff] border border-[#45f3ff]/40 hover:bg-[#45f3ff]/30 font-semibold cursor-pointer"
													>
														{isVerifyingOtp ? "Verifying..." : "Verify OTP"}
													</Button>
												)}
											</div>
										</div>

										<div className="space-y-2">
											<Label htmlFor="new-password" className="text-xs font-semibold tracking-wider text-white uppercase">
												New password
											</Label>
											<div className="relative">
												<Input
													id="new-password"
													type={showNewPassword ? "text" : "password"}
													placeholder="New password"
													value={newPassword}
													onChange={(e) => setNewPassword(e.target.value)}
													disabled={!isOtpVerified}
													required
													className="h-11 border-[#1f2833]/80 bg-[#07080c]/80 pr-10 text-sm text-white focus:border-[#45f3ff] disabled:opacity-50"
													autoComplete="new-password"
												/>
												<button
													type="button"
													onClick={() => setShowNewPassword(!showNewPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-gray-500 hover:text-white"
												>
													{showNewPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</button>
											</div>
										</div>

										<div className="space-y-2">
											<Label htmlFor="confirm-password" className="text-xs font-semibold tracking-wider text-white uppercase">
												Confirm password
											</Label>
											<div className="relative">
												<Input
													id="confirm-password"
													type={showConfirmPassword ? "text" : "password"}
													placeholder="Re-enter new password"
													value={confirmPassword}
													onChange={(e) => setConfirmPassword(e.target.value)}
													disabled={!isOtpVerified}
													required
													className="h-11 border-[#1f2833]/80 bg-[#07080c]/80 pr-10 text-sm text-white focus:border-[#45f3ff] disabled:opacity-50"
													autoComplete="new-password"
												/>
												<button
													type="button"
													onClick={() => setShowConfirmPassword(!showConfirmPassword)}
													className="absolute top-1/2 right-3 -translate-y-1/2 text-gray-500 hover:text-white"
												>
													{showConfirmPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
												</button>
											</div>
										</div>

										{/* Password requirements checklist — must match backend getPasswordPolicyFailures() */}
										<div className="rounded-lg border border-white/5 bg-white/[0.02] p-3 text-xs space-y-1.5">
											<p className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider">Password requirements:</p>
											<div className="grid grid-cols-2 gap-2 text-[11px]">
												<span className={has8Chars ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
													<Check className="h-3 w-3" /> 8+ characters
												</span>
												<span className={hasUpper ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
													<Check className="h-3 w-3" /> 1+ capital letter
												</span>
												<span className={hasLower ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
													<Check className="h-3 w-3" /> 1+ lowercase letter
												</span>
												<span className={hasDigit ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
													<Check className="h-3 w-3" /> 1+ number
												</span>
												<span className={hasSpecial ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
													<Check className="h-3 w-3" /> 1+ symbol (!@#...)
												</span>
												<span className={passwordsMatch ? "text-emerald-400 font-medium flex items-center gap-1" : "text-slate-500 flex items-center gap-1"}>
													<Check className="h-3 w-3" /> Passwords match
												</span>
											</div>
										</div>
									</>
								)}

								<Button
									type="submit"
									className="mt-2 h-11 w-full bg-[#45f3ff] font-bold text-[#0b0c10] shadow-[0_0_24px_rgba(69,243,255,0.28)] hover:bg-[#45f3ff]/90 disabled:opacity-40 cursor-pointer"
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
												: "Send OTP"
											: mode === "forgot_username"
												? isRetrievingUsername
													? "Sending username..."
													: "Retrieve username"
												: isResetting
													? "Updating password..."
													: "Update password"}
								</Button>

								{mode === "login" && (
									<Button
										type="button"
										variant="outline"
										onClick={() => window.location.assign("/signup")}
										className="mt-2.5 h-11 w-full border-white/15 bg-white/[0.04] text-sm font-semibold text-white hover:bg-[#45f3ff]/10 hover:border-[#45f3ff]/60 hover:text-[#45f3ff] cursor-pointer transition-all"
									>
										Sign Up
									</Button>
								)}

								<div className="pt-2 text-xs text-[#8b949e]">
									{mode === "login" ? (
										<div className="space-y-3">
											<div className="flex items-center justify-between">
												<button
													type="button"
													className="text-[#45f3ff] underline-offset-2 hover:underline cursor-pointer"
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
												<button
													type="button"
													className="text-[#45f3ff] underline-offset-2 hover:underline cursor-pointer"
													onClick={() => {
														setMode("forgot_username");
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
													Forgot username?
												</button>
											</div>
										</div>
									) : (
										<div className="text-center">
											<button
												type="button"
												className="text-[#45f3ff] underline-offset-2 hover:underline cursor-pointer"
												onClick={() => {
													setMode("login");
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
												Back to sign in
											</button>
										</div>
									)}
								</div>
							</form>
						</div>
					</section>
				</main>
			</div>

			<footer className="relative z-10 mt-auto border-t border-[#1f2833]/80 bg-[#07080c]/85 backdrop-blur-md">
				<p className="px-5 py-3.5 text-center text-[11px] font-medium tracking-[0.18em] text-[#7d8896] uppercase">
					{COMPANY_NAME}
				</p>
			</footer>
		</div>
	);
}
