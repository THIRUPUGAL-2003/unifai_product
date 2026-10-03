import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { COMPANY_LOGO, COMPANY_NAME } from "@/lib/constants/config";
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
		if (!/^\d{6}$/.test(code.trim())) {
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

		if (password !== confirmPassword) {
			setErrorMessage("Passwords do not match");
			return;
		}
		if (!email.trim()) {
			setErrorMessage("Email is required");
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
		<div className="relative flex min-h-screen flex-col justify-between overflow-hidden bg-[#090b12] font-sans text-slate-100 selection:bg-[#45f3ff] selection:text-black">
			{/* Ambient Gradient Glows */}
			<div className="pointer-events-none absolute -top-40 left-1/2 -translate-x-1/2 h-[500px] w-[800px] rounded-full bg-gradient-to-tr from-indigo-500/15 via-[#45f3ff]/15 to-transparent blur-[140px]" />
			<div className="pointer-events-none absolute right-[-5%] bottom-[-10%] h-[420px] w-[420px] rounded-full bg-blue-600/10 blur-[130px]" />
			<div className="pointer-events-none absolute inset-0 bg-[radial-gradient(#ffffff08_1px,transparent_1px)] [background-size:24px_24px] opacity-70" />

			{/* Floating Top Navigation Header */}
			<header className="relative z-20 mx-auto flex w-full max-w-[1400px] items-center justify-between px-6 py-5">
				<div className="flex items-center gap-4">
					<button
						type="button"
						onClick={() => navigate({ to: "/" })}
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
					<span className="hidden text-xs text-slate-400 md:inline">Already have an account?</span>
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={() => navigate({ to: "/login" })}
						className="h-8.5 rounded-lg border-white/15 bg-white/[0.04] px-4 text-xs font-semibold text-white hover:border-[#45f3ff]/60 hover:bg-[#45f3ff]/10 hover:text-[#45f3ff] transition-all cursor-pointer shadow-sm"
					>
						Sign In
						<ArrowRight className="ml-1.5 h-3.5 w-3.5" />
					</Button>
				</div>
			</header>

			{/* Main Center Content */}
			<main className="relative z-10 mx-auto flex w-full max-w-[1300px] flex-1 items-center justify-center px-6 py-8">
				<div className="grid w-full items-center gap-12 lg:grid-cols-12 lg:gap-14">
					{/* Left Column: Platform Value Props */}
					<section className="hidden flex-col justify-center space-y-8 lg:col-span-6 lg:flex">
						<div className="space-y-4">
							<div className="inline-flex items-center gap-2 rounded-full border border-[#45f3ff]/20 bg-[#45f3ff]/10 px-3.5 py-1 text-xs font-semibold tracking-wide text-[#89f7ff]">
								<span className="h-2 w-2 rounded-full bg-[#45f3ff] shadow-[0_0_8px_#45f3ff]" />
								<span>Join the Modern AI Gateway</span>
							</div>

							<h1 className="text-4xl font-extrabold tracking-tight text-white sm:text-5xl leading-[1.12]">
								Get Started with{" "}
								<span className="bg-gradient-to-r from-[#45f3ff] via-[#7dd3fc] to-white bg-clip-text text-transparent">
									Raksha Platform.
								</span>
							</h1>

							<p className="max-w-lg text-sm text-slate-400 leading-relaxed">
								Deploy unified LLM access, versioned Prompt Repositories with ChatGPT-style vision support, team workspace governance, and spending budgets in minutes.
							</p>
						</div>

						{/* Benefits List */}
						<div className="space-y-3.5 max-w-lg">
							<div className="flex items-start gap-3.5 rounded-xl border border-white/8 bg-white/[0.02] p-4 backdrop-blur-md transition-colors hover:border-white/15 hover:bg-white/[0.04]">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#45f3ff]/10 text-[#45f3ff]">
									<Cpu className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-white">Unified OpenAI-Compatible Endpoint</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Plug-and-play with your existing OpenAI SDK, LangChain, or curl scripts with a 1-line base URL switch.
									</p>
								</div>
							</div>

							<div className="flex items-start gap-3.5 rounded-xl border border-white/8 bg-white/[0.02] p-4 backdrop-blur-md transition-colors hover:border-white/15 hover:bg-white/[0.04]">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-400">
									<Sparkles className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-white">Prompt Repository & Team Folders</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Collaborate on prompts, run live chat experiments with multimodal image attachments, and track audit history.
									</p>
								</div>
							</div>

							<div className="flex items-start gap-3.5 rounded-xl border border-white/8 bg-white/[0.02] p-4 backdrop-blur-md transition-colors hover:border-white/15 hover:bg-white/[0.04]">
								<div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-indigo-500/10 text-indigo-400">
									<Shield className="h-5 w-5" />
								</div>
								<div>
									<h3 className="text-sm font-semibold text-white">Zero-Telemetry & Privacy</h3>
									<p className="text-xs text-slate-400 leading-relaxed mt-0.5">
										Self-hosted architecture with local database storage, RBAC governance, and user budget waterfalls.
									</p>
								</div>
							</div>
						</div>
					</section>

					{/* Right Column: Sign Up Form Card */}
					<section className="flex items-center justify-center lg:col-span-6">
						<div className="w-full max-w-[460px] rounded-2xl border border-white/10 bg-[#0f121d]/90 p-8 shadow-[0_25px_70px_rgba(0,0,0,0.65)] backdrop-blur-2xl">
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
											className="group inline-flex items-center gap-1.5 text-xs font-medium text-slate-400 hover:text-[#45f3ff] transition-colors cursor-pointer"
										>
											<ArrowLeft className="h-3.5 w-3.5 transition-transform group-hover:-translate-x-1" />
											<span>Back to Form</span>
										</button>
										<Link to="/login" className="text-xs text-[#45f3ff] hover:underline font-medium">
											Sign In
										</Link>
									</div>

									<div className="space-y-1.5">
										<h2 className="text-2xl font-bold tracking-tight text-white">Verify Your Email</h2>
										<p className="text-xs text-slate-400 leading-relaxed">{codeInfo || "Enter the 6-digit code sent to your email."}</p>
									</div>

									{errorMessage && (
										<div className="flex items-center gap-2.5 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300">
											<ShieldAlert className="h-4 w-4 shrink-0 text-rose-400" />
											<span>{errorMessage}</span>
										</div>
									)}

									<form onSubmit={handleVerifyCode} className="space-y-4">
										<div className="space-y-1.5">
											<Label htmlFor="signup-code" className="text-xs font-medium text-slate-200">
												Verification Code
											</Label>
											<div className="relative">
												<KeyRound className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="signup-code"
													inputMode="numeric"
													autoComplete="one-time-code"
													maxLength={6}
													placeholder="6-digit code"
													value={code}
													onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
													required
													className="h-11 pl-10 rounded-xl border-white/10 bg-black/40 font-mono tracking-[0.3em] text-sm text-white placeholder:tracking-normal focus:border-[#45f3ff]"
												/>
											</div>
										</div>

										<Button
											type="submit"
											disabled={isLoading || code.trim().length < 6}
											className="h-11 w-full rounded-xl bg-gradient-to-r from-[#45f3ff] to-[#38bdf8] font-bold text-[#090b12] shadow-[0_0_24px_rgba(69,243,255,0.25)] hover:brightness-105 transition-all cursor-pointer disabled:opacity-40"
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
											className="text-slate-400 hover:text-white transition-colors cursor-pointer"
										>
											Edit details
										</button>

										<button
											type="button"
											onClick={handleResendCode}
											disabled={isResending || resendCooldown > 0}
											className="text-[#45f3ff] hover:underline disabled:text-slate-500 disabled:no-underline disabled:cursor-not-allowed cursor-pointer inline-flex items-center gap-1.5"
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
										<h2 className="text-2xl font-bold tracking-tight text-white">Create Account</h2>
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

									<form onSubmit={handleSignup} className="space-y-3.5">
										<div className="space-y-1.5">
											<Label htmlFor="username" className="text-xs font-medium text-slate-200">
												Username
											</Label>
											<div className="relative">
												<User className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="username"
													type="text"
													placeholder="e.g. john.doe"
													value={username}
													onChange={(e) => setUsername(e.target.value)}
													required
													className="h-10 pl-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40"
													autoComplete="username"
												/>
											</div>
										</div>

										<div className="space-y-1.5">
											<Label htmlFor="email" className="text-xs font-medium text-slate-200">
												Email Address
											</Label>
											<div className="relative">
												<Mail className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="email"
													type="email"
													placeholder="e.g. user@company.com"
													value={email}
													onChange={(e) => setEmail(e.target.value)}
													required
													className="h-10 pl-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff] focus:ring-1 focus:ring-[#45f3ff]/40"
													autoComplete="email"
												/>
											</div>
										</div>

										<div className="space-y-1.5">
											<Label htmlFor="password" className="text-xs font-medium text-slate-200">
												Password
											</Label>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="password"
													type={showPassword ? "text" : "password"}
													placeholder="Create a strong password"
													value={password}
													onChange={(e) => setPassword(e.target.value)}
													required
													className="h-10 pl-10 pr-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff]"
													autoComplete="new-password"
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

										<div className="space-y-1.5">
											<Label htmlFor="confirm-password" className="text-xs font-medium text-slate-200">
												Confirm Password
											</Label>
											<div className="relative">
												<Lock className="absolute top-1/2 left-3.5 -translate-y-1/2 h-4 w-4 text-slate-500" />
												<Input
													id="confirm-password"
													type={showConfirmPassword ? "text" : "password"}
													placeholder="Repeat your password"
													value={confirmPassword}
													onChange={(e) => setConfirmPassword(e.target.value)}
													required
													className="h-10 pl-10 pr-10 rounded-xl border-white/10 bg-black/40 text-sm text-white placeholder:text-slate-500 focus:border-[#45f3ff]"
													autoComplete="new-password"
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

										<Button
											type="submit"
											disabled={isLoading}
											className="mt-2 h-11 w-full rounded-xl bg-gradient-to-r from-[#45f3ff] to-[#38bdf8] font-bold text-[#090b12] shadow-[0_0_24px_rgba(69,243,255,0.25)] hover:shadow-[0_0_32px_rgba(69,243,255,0.4)] hover:brightness-105 transition-all cursor-pointer disabled:opacity-40"
										>
											{isLoading ? "Submitting Request..." : "Request Account Access"}
										</Button>
									</form>

									<div className="pt-1 text-center text-xs text-slate-400">
										<span>Already have an account? </span>
										<button
											type="button"
											onClick={() => navigate({ to: "/login" })}
											className="font-medium text-[#45f3ff] hover:underline cursor-pointer"
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
										<h2 className="text-2xl font-bold tracking-tight text-white">Submitted for Admin Approval</h2>
										<p className="mx-auto max-w-sm text-xs leading-relaxed text-slate-400">
											Your request for user <span className="font-semibold text-white">{username}</span> was successfully received. An administrator must approve your account on the Users Governance tab before you can sign in.
										</p>
									</div>

									<Button
										onClick={() => navigate({ to: "/login" })}
										className="h-11 px-6 rounded-xl bg-gradient-to-r from-[#45f3ff] to-[#38bdf8] font-bold text-[#090b12] shadow-lg shadow-[#45f3ff]/20 hover:brightness-105 transition-all cursor-pointer inline-flex items-center gap-2"
									>
										<span>Continue to Sign In</span>
										<ArrowRight className="h-4 w-4" />
									</Button>
								</div>
							)}
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
