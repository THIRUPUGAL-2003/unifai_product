import { PRODUCT_NAME } from "@/lib/constants/config";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { COMPANY_LOGO, COMPANY_NAME, DEFAULT_FOOTER_TEXT } from "@/lib/constants/config";
import { useBranding } from "@/lib/hooks/useBranding";
import { getApiBaseUrl } from "@/lib/utils/port";
import { Link, useNavigate } from "@tanstack/react-router";
import {
	ArrowLeft,
	ArrowRight,
	Check,
	CheckCircle,
	Clock,
	Cpu,
	Eye,
	EyeOff,
	KeyRound,
	Loader2,
	Lock,
	Mail,
	Shield,
	ShieldAlert,
	Sparkles,
	User,
} from "lucide-react";
import { useEffect, useState } from "react";

// Must match the server's per-account resend cooldown; a resend inside it is silently dropped.
const CODE_RESEND_COOLDOWN_SECONDS = 120;

export default function SignupPage() {
	const branding = useBranding();
	const [username, setUsername] = useState("");
	const [email, setEmail] = useState("");
	const [password, setPassword] = useState("");
	const [confirmPassword, setConfirmPassword] = useState("");
	const [showPassword, setShowPassword] = useState(false);
	const [showConfirmPassword, setShowConfirmPassword] = useState(false);
	const [errorMessage, setErrorMessage] = useState("");
	const [isSubmitted, setIsSubmitted] = useState(false);
	const [isLoading, setIsLoading] = useState(false);
	const [needsCode, setNeedsCode] = useState(false);
	const [code, setCode] = useState("");
	const [codeInfo, setCodeInfo] = useState("");
	const [resendCooldown, setResendCooldown] = useState(0);
	const [isResending, setIsResending] = useState(false);
	const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

	const clearFieldError = (field: string) => {
		setFieldErrors((prev) => {
			if (!prev[field]) return prev;
			const next = { ...prev };
			delete next[field];
			return next;
		});
	};
	const navigate = useNavigate();

	useEffect(() => {
		if (resendCooldown <= 0) return;
		const timer = setTimeout(() => {
			setResendCooldown((prev) => prev - 1);
		}, 1000);
		return () => clearTimeout(timer);
	}, [resendCooldown]);

	const apiError = (data: any, fallback: string) => data?.error?.message || data?.message || data?.error || fallback;

	const has8Chars = password.length >= 8;
	const hasUpper = /[A-Z]/.test(password);
	const hasLower = /[a-z]/.test(password);
	const hasDigit = /[0-9]/.test(password);
	const hasSpecial = /[^A-Za-z0-9]/.test(password);
	const passwordsMatch = password.length > 0 && password === confirmPassword;

	const handleVerifyCode = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		setErrorMessage("");
		const cleanCode = code.trim();
		if (!cleanCode) {
			setFieldErrors({ code: "Please fill in this field" });
			setErrorMessage("Please enter the 6-digit verification code");
			return;
		}
		if (!/^\d{6}$/.test(cleanCode)) {
			setFieldErrors({ code: "Code must be 6 digits" });
			setErrorMessage("Enter the 6-digit code from your email");
			return;
		}
		setIsLoading(true);
		try {
			const res = await fetch(`${getApiBaseUrl()}/session/register/verify`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ username: username.trim(), otp: code.trim() }),
			});
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				setErrorMessage(apiError(data, "Verification failed"));
				return;
			}
			setNeedsCode(false);
			setIsSubmitted(true);
		} catch {
			setErrorMessage("Could not reach the server. Please try again.");
		} finally {
			setIsLoading(false);
		}
	};

	const handleResendCode = async () => {
		if (isResending || resendCooldown > 0) return;
		setErrorMessage("");
		setIsResending(true);
		try {
			const res = await fetch(`${getApiBaseUrl()}/session/register/resend`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ username: username.trim() }),
			});
			const data = await res.json().catch(() => ({}));
			if (res.ok) {
				setCodeInfo("If your sign-up is waiting for verification, a new code was sent.");
				setResendCooldown(CODE_RESEND_COOLDOWN_SECONDS);
			} else {
				setErrorMessage(apiError(data, "Could not resend the code"));
			}
		} catch {
			setErrorMessage("Could not reach the server. Please try again.");
		} finally {
			setIsResending(false);
		}
	};

	const handleSignup = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();
		setErrorMessage("");

		const newErrors: Record<string, string> = {};
		if (!username.trim()) {
			newErrors.username = "Please fill in this field";
		}
		if (!email.trim()) {
			newErrors.email = "Please fill in this field";
		}
		if (!password) {
			newErrors.password = "Please fill in this field";
		}
		if (!confirmPassword) {
			newErrors.confirmPassword = "Please fill in this field";
		}
		if (Object.keys(newErrors).length > 0) {
			setFieldErrors(newErrors);
			setErrorMessage("Please fill in all required fields");
			return;
		}

		if (password !== confirmPassword) {
			setFieldErrors({ confirmPassword: "Passwords do not match" });
			setErrorMessage("Passwords do not match");
			return;
		}

		const policyFails: string[] = [];
		if (password.length < 8) policyFails.push("at least 8 characters");
		if (!/[A-Z]/.test(password)) policyFails.push("one uppercase letter");
		if (!/[a-z]/.test(password)) policyFails.push("one lowercase letter");
		if (!/\d/.test(password)) policyFails.push("one number");
		if (!/[^A-Za-z0-9]/.test(password)) policyFails.push("one special character");
		if (policyFails.length > 0) {
			setErrorMessage("Password must include " + policyFails.join(", "));
			return;
		}

		setIsLoading(true);
		try {
			const res = await fetch(`${getApiBaseUrl()}/session/register`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					username: username.trim(),
					email: email.trim(),
					password,
					role: "user",
				}),
			});
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				const message = apiError(data, "Registration failed");
				if (res.status === 409 && /waiting for email verification/i.test(message)) {
					setCodeInfo(message);
					setNeedsCode(true);
					return;
				}
				setErrorMessage(message);
				return;
			}
			if (data?.verification_required) {
				setCodeInfo(data.message || "We emailed you a 6-digit code.");
				setResendCooldown(CODE_RESEND_COOLDOWN_SECONDS);
				setNeedsCode(true);
				return;
			}
			setIsSubmitted(true);
		} catch {
			setErrorMessage("Could not reach the server. Please try again.");
		} finally {
			setIsLoading(false);
		}
	};

	return (
		<div className="relative flex min-h-screen flex-col overflow-x-hidden bg-[#f6f8fd] font-sans text-slate-600 selection:bg-blue-500/20 selection:text-[#0b1b3a]">
			<div className="pointer-events-none absolute inset-x-0 top-0 h-[420px] bg-[radial-gradient(ellipse_70%_80%_at_50%_-20%,#d9e6ff_0%,rgba(246,248,253,0)_70%)]" />

			{/* Fixed Top Navigation Header */}
			<header className="relative z-20 flex-shrink-0 mx-auto flex w-full max-w-[1400px] items-center justify-between px-6 py-5">
				<div className="flex items-center gap-4">
					<button
						type="button"
						onClick={() => navigate({ to: "/" })}
						className="group inline-flex items-center gap-2 rounded-full border border-[#e3ebf8] bg-white px-4 py-1.5 text-xs font-medium text-[#0b1b3a] transition-all hover:border-[#2f6bff]/40 hover:text-[#2f6bff] cursor-pointer shadow-sm"
						title="Return to Landing Page"
					>
						<ArrowLeft className="h-3.5 w-3.5 transition-transform group-hover:-translate-x-1 text-[#2f6bff]" />
						<span>Back to Home</span>
					</button>

					<div className="hidden h-5 w-px bg-[#dbe4f5] sm:block" />

					<div className="hidden sm:flex items-center gap-2.5">
						<img src={branding.companyLogo} alt={branding.companyName} className="h-7 w-auto object-contain" />
						<span className="text-sm font-bold tracking-tight text-[#0b1b3a]">{branding.companyName}</span>
					</div>
				</div>

				<div className="flex items-center gap-3">
					<span className="hidden text-xs text-slate-400 md:inline">Already have an account?</span>
					<Button
						type="button"
						size="sm"
						onClick={() => navigate({ to: "/login" })}
						className="h-8.5 rounded-full border-[#2f6bff] bg-[#2f6bff] px-4 text-xs font-semibold text-white hover:bg-[#2457d6] transition-all cursor-pointer shadow-sm"
					>
						Sign In
						<ArrowRight className="ml-1.5 h-3.5 w-3.5" />
					</Button>
				</div>
			</header>

			{/* Main Center Content - only this area scrolls */}
			<main className="relative z-10 flex-1">
			<div className="mx-auto flex w-full max-w-[1440px] items-start justify-center px-6 lg:px-10 py-8">
					<div className="grid w-full items-start gap-10 lg:grid-cols-2 lg:gap-12">
					<section className="hidden flex-col justify-center space-y-5 lg:flex">
						<div className="space-y-4">
							<div className="inline-flex items-center gap-2 rounded-full border border-[#d5e2ff] bg-white px-3.5 py-1 text-xs font-semibold tracking-wide text-[#2f6bff] shadow-sm">
								<span className="h-2 w-2 rounded-full bg-[#2f6bff]" />
								<span>Join the Modern AI Gateway</span>
							</div>

							<h1 className="text-4xl font-extrabold tracking-tight text-[#0b1b3a] sm:text-5xl leading-[1.08]">
								Get Started with{" "}
								<span className="bg-gradient-to-r from-[#2f6bff] to-[#7a5cff] bg-clip-text text-transparent">
									{PRODUCT_NAME} Platform.
								</span>
							</h1>

							<p className="max-w-lg text-sm text-slate-400 leading-relaxed">
								Deploy unified LLM access, versioned Prompt Repositories with ChatGPT-style vision support, team workspace governance, and spending budgets in minutes.
							</p>
						</div>

						{/* Benefits List */}
						<div className="space-y-3.5 max-w-lg">
							<div className="flex items-start gap-3.5 rounded-xl border border-[#e6edf8] bg-white p-4 shadow-sm transition-colors hover:border-[#2f6bff]/30">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#eef3ff] text-[#2f6bff]">
									<Cpu className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-[#0b1b3a]">Unified OpenAI-Compatible Endpoint</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Plug-and-play with your existing OpenAI SDK, LangChain, or curl scripts with a 1-line base URL switch.
									</p>
								</div>
							</div>

							<div className="flex items-start gap-3.5 rounded-xl border border-[#e6edf8] bg-white p-4 shadow-sm transition-colors hover:border-[#2f6bff]/30">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-400">
									<Sparkles className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-[#0b1b3a]">Prompt Repository & Team Folders</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Collaborate on prompts, run live chat experiments with multimodal image attachments, and track audit history.
									</p>
								</div>
							</div>

							<div className="flex items-start gap-3.5 rounded-xl border border-[#e6edf8] bg-white p-4 shadow-sm transition-colors hover:border-[#2f6bff]/30">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-indigo-500/10 text-indigo-400">
									<Shield className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-[#0b1b3a]">Zero-Telemetry & Privacy</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Self-hosted architecture with local database storage, RBAC governance, and user budget waterfalls.
									</p>
								</div>
							</div>
						</div>
					</section>

					{/* Right Column: Sign Up Form Card */}
					<section className="flex items-start justify-center">
						<div className="w-full max-w-[480px] rounded-3xl border border-[#e6edf8] bg-white p-6 sm:p-7 shadow-[0_24px_60px_rgba(47,107,255,0.10)]">
							{needsCode ? (
								/* Email OTP Verification Step */
								<div className="space-y-6">
									<div className="flex items-center justify-between">
										<button
											type="button"
											onClick={() => {
												setNeedsCode(false);
												setErrorMessage("");
												setCode("");
											}}
											className="group inline-flex items-center gap-1.5 text-xs font-medium text-slate-400 hover:text-[#2f6bff] transition-colors cursor-pointer"
										>
											<ArrowLeft className="h-3.5 w-3.5 transition-transform group-hover:-translate-x-1" />
											<span>Back to Form</span>
										</button>
										<Link to="/login" className="text-xs text-[#2f6bff] hover:underline font-medium">
											Sign In
										</Link>
									</div>

									<div className="space-y-1.5">
										<h2 className="text-2xl font-bold tracking-tight text-[#0b1b3a]">Verify Your Email</h2>
										<p className="text-xs text-slate-400 leading-relaxed">{codeInfo || "Enter the 6-digit code sent to your email."}</p>
									</div>

									{errorMessage && (
										<div className="flex items-center gap-2.5 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300">
											<ShieldAlert className="h-4 w-4 shrink-0 text-rose-400" />
											<span>{errorMessage}</span>
										</div>
									)}

									<form onSubmit={handleVerifyCode} noValidate className="space-y-4">
										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="signup-code" className="text-xs font-medium text-slate-700">
													Verification Code <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
												</Label>
												{fieldErrors.code && (
													<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
														<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
														{fieldErrors.code}
													</span>
												)}
											</div>
											<div className="relative">
												<KeyRound className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="signup-code"
													inputMode="numeric"
													autoComplete="one-time-code"
													maxLength={6}
													placeholder="6-digit code"
													value={code}
													onChange={(e) => {
														setCode(e.target.value.replace(/\D/g, ""));
														clearFieldError("code");
													}}
													className={`h-11 pl-10 rounded-xl bg-[#f7f9ff] font-mono tracking-[0.3em] text-sm text-[#0b1b3a] placeholder:tracking-normal transition-all ${
														fieldErrors.code
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-[#e3ebf8] focus:border-[#2f6bff]"
													}`}
												/>
											</div>
										</div>

										<Button
											type="submit"
											disabled={isLoading}
											className="h-11 w-full rounded-xl bg-[#2f6bff] hover:bg-[#2457d6] font-bold text-white shadow-[0_0_24px_rgba(69,243,255,0.25)] hover:brightness-105 transition-all cursor-pointer disabled:opacity-40"
										>
											{isLoading ? "Verifying..." : "Verify & Complete"}
										</Button>
									</form>

									<div className="flex items-center justify-between pt-1 text-xs">
										<button
											type="button"
											onClick={() => {
												setNeedsCode(false);
												setErrorMessage("");
												setCode("");
											}}
											className="text-slate-500 hover:text-[#2f6bff] transition-colors cursor-pointer"
										>
											Edit details
										</button>

										<button
											type="button"
											onClick={handleResendCode}
											disabled={isResending || resendCooldown > 0}
											className="text-[#2f6bff] hover:underline disabled:text-slate-500 disabled:no-underline disabled:cursor-not-allowed cursor-pointer inline-flex items-center gap-1.5"
										>
											{isResending ? (
												<>
													<Loader2 className="h-3 w-3 animate-spin" />
													Resending...
												</>
											) : resendCooldown > 0 ? (
												`Resend in ${resendCooldown}s`
											) : (
												"Resend Code"
											)}
										</button>
									</div>
								</div>
							) : !isSubmitted ? (
								/* Standard Signup Form */
								<div className="space-y-6">
									<div className="space-y-1.5">
										<h2 className="text-2xl font-bold tracking-tight text-[#0b1b3a]">Create Account</h2>
										<p className="text-xs text-slate-400 leading-relaxed">
											Request a user account. An existing admin will review and approve your account.
										</p>
									</div>

									{errorMessage && (
										<div className="flex items-center gap-2.5 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300">
											<ShieldAlert className="h-4 w-4 shrink-0 text-rose-400" />
											<span>{errorMessage}</span>
										</div>
									)}

									<form onSubmit={handleSignup} noValidate className="space-y-3.5">
										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="username" className="text-xs font-medium text-slate-700">
													Username <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
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
													placeholder="e.g. john.doe"
													value={username}
													onChange={(e) => {
														setUsername(e.target.value);
														clearFieldError("username");
													}}
													className={`h-10 pl-10 rounded-xl bg-[#f7f9ff] text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all ${
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
												<Label htmlFor="email" className="text-xs font-medium text-slate-700">
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
													id="email"
													type="email"
													placeholder="e.g. user@company.com"
													value={email}
													onChange={(e) => {
														setEmail(e.target.value);
														clearFieldError("email");
													}}
													className={`h-10 pl-10 rounded-xl bg-[#f7f9ff] text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all ${
														fieldErrors.email
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-[#e3ebf8] focus:border-[#2f6bff] focus:ring-1 focus:ring-[#2f6bff]/25"
													}`}
													autoComplete="email"
												/>
											</div>
										</div>

										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="password" className="text-xs font-medium text-slate-700">
													Password <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
												</Label>
												{fieldErrors.password && (
													<span className="text-[11px] font-medium text-rose-400 animate-in fade-in flex items-center gap-1">
														<span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
														{fieldErrors.password}
													</span>
												)}
											</div>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="password"
													type={showPassword ? "text" : "password"}
													placeholder="Create a strong password"
													value={password}
													onChange={(e) => {
														setPassword(e.target.value);
														clearFieldError("password");
													}}
													className={`h-10 pl-10 pr-10 rounded-xl bg-[#f7f9ff] text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all ${
														fieldErrors.password
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-[#e3ebf8] focus:border-[#2f6bff]"
													}`}
													autoComplete="new-password"
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

										<div className="space-y-1.5">
											<div className="flex items-center justify-between">
												<Label htmlFor="confirm-password" className="text-xs font-medium text-slate-700">
													Confirm Password <span className="text-rose-400 font-bold ml-0.5" aria-hidden="true">*</span>
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
													id="confirm-password"
													type={showConfirmPassword ? "text" : "password"}
													placeholder="Repeat your password"
													value={confirmPassword}
													onChange={(e) => {
														setConfirmPassword(e.target.value);
														clearFieldError("confirmPassword");
													}}
													className={`h-10 pl-10 pr-10 rounded-xl bg-[#f7f9ff] text-sm text-[#0b1b3a] placeholder:text-slate-400 transition-all ${
														fieldErrors.confirmPassword
															? "border-rose-500/80 ring-1 ring-rose-500/30 focus:border-rose-500 focus:ring-rose-500/40"
															: "border-[#e3ebf8] focus:border-[#2f6bff]"
													}`}
													autoComplete="new-password"
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

										{/* Password requirements checklist - single compact row */}
										<div className="flex flex-wrap gap-x-3 gap-y-2 rounded-xl border border-[#e3ebf8] bg-[#f7f9ff] px-3 py-2.5 text-[11px]">
											<span className={has8Chars ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-normal" : "text-slate-500 flex items-center gap-1 whitespace-normal"}>
												<Check className="h-3 w-3 shrink-0" /> 8+ chars
											</span>
											<span className={hasUpper ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-normal" : "text-slate-500 flex items-center gap-1 whitespace-normal"}>
												<Check className="h-3 w-3 shrink-0" /> A-Z
											</span>
											<span className={hasLower ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-normal" : "text-slate-500 flex items-center gap-1 whitespace-normal"}>
												<Check className="h-3 w-3 shrink-0" /> a-z
											</span>
											<span className={hasDigit ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-normal" : "text-slate-500 flex items-center gap-1 whitespace-normal"}>
												<Check className="h-3 w-3 shrink-0" /> 0-9
											</span>
											<span className={hasSpecial ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-normal" : "text-slate-500 flex items-center gap-1 whitespace-normal"}>
												<Check className="h-3 w-3 shrink-0" /> Symbol
											</span>
											<span className={passwordsMatch ? "text-emerald-400 font-medium flex items-center gap-1 whitespace-normal" : "text-slate-500 flex items-center gap-1 whitespace-normal"}>
												<Check className="h-3 w-3 shrink-0" /> Match
											</span>
										</div>

										<Button
											type="submit"
											disabled={isLoading}
											className="mt-2 h-11 w-full rounded-xl bg-[#2f6bff] hover:bg-[#2457d6] font-bold text-white shadow-[0_0_24px_rgba(69,243,255,0.25)] hover:shadow-[0_0_32px_rgba(69,243,255,0.4)] hover:brightness-105 transition-all cursor-pointer disabled:opacity-40"
										>
											{isLoading ? "Submitting Request..." : "Request Account Access"}
										</Button>
									</form>

									<div className="pt-1 text-center text-xs text-slate-400">
										<span>Already have an account? </span>
										<button
											type="button"
											onClick={() => navigate({ to: "/login" })}
											className="font-medium text-[#2f6bff] hover:underline cursor-pointer"
										>
											Sign In
										</button>
									</div>
								</div>
							) : (
								/* Request Submitted Success Step */
								<div className="space-y-6 text-center animate-in fade-in duration-300">
									<div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-amber-500/10 text-amber-400 border border-amber-500/20 shadow-[0_0_24px_rgba(245,158,11,0.15)]">
										<Clock className="h-8 w-8" />
									</div>

									<div className="space-y-2">
										<h2 className="text-2xl font-bold tracking-tight text-[#0b1b3a]">Submitted for Admin Approval</h2>
										<p className="mx-auto max-w-sm text-xs leading-relaxed text-slate-400">
											Your request for user <span className="font-semibold text-[#0b1b3a]">{username}</span> was successfully received. An administrator must approve your account on the Users Governance tab before you can sign in.
										</p>
									</div>

									<Button
										onClick={() => navigate({ to: "/login" })}
										className="h-11 px-6 rounded-xl bg-[#2f6bff] hover:bg-[#2457d6] font-bold text-white shadow-lg shadow-[#45f3ff]/20 hover:brightness-105 transition-all cursor-pointer inline-flex items-center gap-2"
									>
										<span>Continue to Sign In</span>
										<ArrowRight className="h-4 w-4" />
									</Button>
								</div>
							)}
						</div>
					</section>
				</div>
			</div>
			</main>

			{/* Minimal Sleek Footer */}
			<footer className="relative z-10 mt-auto w-full border-t border-[#e6edf8] bg-[#f6f8fd] px-6 py-4 text-center text-xs text-slate-500">
				<span className="mx-auto block max-w-[1440px]">{branding.footerText}</span>
			</footer>
		</div>
	);
}
