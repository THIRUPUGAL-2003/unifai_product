import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Link, useNavigate } from "@tanstack/react-router";
import {
	Shield,
	Zap,
	ArrowRight,
	Lock,
	CheckCircle,
	CheckCircle2,
	Menu,
	X,
	Laptop,
	Key,
	Cpu,
	Activity,
	Calendar,
	Copy,
	Check,
	ChevronDown,
	Layers,
	Sparkles,
	Terminal,
	Gauge,
	Sliders,
	Globe,
	Database,
	Server,
	ChevronRight
} from "lucide-react";
import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/utils/port";
import { COMPANY_LOGO, COMPANY_NAME } from "@/lib/constants/config";

export default function LandingPage() {
	const [isLoggedIn, setIsLoggedIn] = useState(false);
	const [codeTab, setCodeTab] = useState<"python" | "curl" | "node">("python");
	const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
	const [copiedCode, setCopiedCode] = useState(false);
	const [openFaq, setOpenFaq] = useState<number | null>(0);
	const navigate = useNavigate();

	// Demo Booking Modal State
	const [isDemoModalOpen, setIsDemoModalOpen] = useState(false);
	const [demoSubmitted, setDemoSubmitted] = useState(false);
	const [demoForm, setDemoForm] = useState({
		fullName: "",
		email: "",
		company: "",
		teamSize: "50-250",
		interest: "both",
		notes: "",
	});

	useEffect(() => {
		fetch(`${getApiBaseUrl()}/session/is-auth-enabled`, {
			credentials: "include",
		})
			.then((res) => (res.ok ? res.json() : null))
			.then((data) => {
				if (data && (!data.is_auth_enabled || data.has_valid_token)) {
					setIsLoggedIn(true);
				}
			})
			.catch(() => {});
	}, []);

	const handleCopyCode = (text: string) => {
		navigator.clipboard.writeText(text);
		setCopiedCode(true);
		setTimeout(() => setCopiedCode(false), 2000);
	};

	const handleDemoSubmit = (e: React.FormEvent) => {
		e.preventDefault();
		if (!demoForm.fullName || !demoForm.email) return;
		setDemoSubmitted(true);
	};

	const resetDemoModal = () => {
		setDemoSubmitted(false);
		setIsDemoModalOpen(false);
		setDemoForm({
			fullName: "",
			email: "",
			company: "",
			teamSize: "50-250",
			interest: "both",
			notes: "",
		});
	};

	const codeExamples = {
		python: `# 1-line drop-in configuration for Python
from openai import OpenAI

client = OpenAI(
    base_url="https://unifai.yourcompany.com/v1",
    api_key="unifai_vk_live_enterprise_99x"
)

# Seamless routing across any AI provider
response = client.chat.completions.create(
    model="claude-3-7-sonnet", # or gpt-4o, gemini-2.0-flash, deepseek-r1
    messages=[{"role": "user", "content": "Analyze confidential audit metrics"}]
)

print(response.choices[0].message.content)`,
		curl: `# Universal OpenAI-compatible cURL endpoint
curl -X POST https://unifai.yourcompany.com/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer unifai_vk_live_enterprise_99x" \\
  -d '{
    "model": "gpt-4o",
    "messages": [
      {
        "role": "user",
        "content": "Audit source code repository for credential leaks"
      }
    ]
  }'`,
		node: `// Drop-in compatible with standard Node OpenAI SDK
import OpenAI from "openai";

const openai = new OpenAI({
  baseURL: "https://unifai.yourcompany.com/v1",
  apiKey: "unifai_vk_live_enterprise_99x"
});

const response = await openai.chat.completions.create({
  model: "claude-3-7-sonnet",
  messages: [{ role: "user", content: "Optimize distributed semantic cache architecture" }]
});

console.log(response.choices[0].message.content);`
	};

	const supportedModels = [
		{ name: "OpenAI GPT-4o", color: "bg-emerald-500", glow: "group-hover:border-emerald-500/40" },
		{ name: "Anthropic Claude 3.7", color: "bg-amber-500", glow: "group-hover:border-amber-500/40" },
		{ name: "Google Gemini 2.0", color: "bg-sky-500", glow: "group-hover:border-sky-500/40" },
		{ name: "DeepSeek R1", color: "bg-blue-500", glow: "group-hover:border-blue-500/40" },
		{ name: "Meta Llama 3.3", color: "bg-purple-500", glow: "group-hover:border-purple-500/40" },
		{ name: "Mistral Large", color: "bg-orange-500", glow: "group-hover:border-orange-500/40" },
	];

	const faqs = [
		{
			q: "How does Browser Guard protect corporate data in employee AI web tools?",
			a: "UnifAI Browser Guard runs as a lightweight, zero-latency endpoint daemon on macOS and Windows. When staff interact with ChatGPT, Claude, or Grok, Browser Guard intercepts prompts and document attachments in-flight, immediately sanitizing API keys, passwords, customer PII, and proprietary code before packets exit the machine."
		},
		{
			q: "Does Browser Guard slow down employee network connection or browsing?",
			a: "Not at all. Browser Guard uses local loopback inspection dedicated strictly to configured AI domain destinations. Interception overhead benchmarks under 0.8ms, and standard internet browsing stays 100% native with zero speed penalty."
		},
		{
			q: "How does the Universal AI Gateway save API spend with Semantic Caching?",
			a: "The gateway maintains a pgvector vectorized semantic memory. When incoming prompts are semantically equivalent to previously answered queries, UnifAI serves the response directly with sub-10ms latency and 0 provider token cost, routinely cutting corporate LLM bills by 70% to 85%."
		},
		{
			q: "Can UnifAI run in an air-gapped on-premise datacenter or private VPC?",
			a: "Yes. UnifAI compiles to a standalone high-performance Go binary with PostgreSQL. You can deploy it seamlessly across Kubernetes, Docker, AWS ECS/EKS, Azure, or air-gapped bare metal so that sensitive data never leaves your internal cloud perimeter."
		},
		{
			q: "How do we deploy the desktop agent across hundreds of employee workstations?",
			a: "UnifAI ships with pre-configured silent installers for Windows (MSI/EXE) and macOS (PKG/ZIP). IT administrators can roll it out in minutes via Microsoft Intune, Jamf Pro, Kandji, or Active Directory Group Policy."
		}
	];

	const companyLogoSrc = COMPANY_LOGO;
	const productName = "UnifAI";
	const companyFullName = COMPANY_NAME;

	return (
		<div className="bg-[#07090e] text-[#cbd5e1] min-h-screen font-sans selection:bg-sky-500/25 selection:text-white antialiased overflow-x-hidden no-scrollbar">
			{/* Luxury Background Glow & Tech Grid */}
			<div className="fixed inset-0 pointer-events-none overflow-hidden z-0">
				{/* Radial background grid */}
				<div className="absolute inset-0 bg-[linear-gradient(to_right,#ffffff05_1px,transparent_1px),linear-gradient(to_bottom,#ffffff05_1px,transparent_1px)] bg-[size:4rem_4rem] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_0%,#000_70%,transparent_100%)]" />
				
				{/* Primary Ambient Spotlight */}
				<div className="absolute -top-[25%] left-1/2 -translate-x-1/2 w-[1200px] h-[650px] bg-gradient-to-b from-sky-500/[0.08] via-indigo-500/[0.05] to-transparent rounded-full blur-[150px]" />
				<div className="absolute top-[40%] right-[-10%] w-[600px] h-[600px] bg-purple-500/[0.03] rounded-full blur-[160px]" />
			</div>

			{/* High-Gloss Sticky Navigation */}
			<header className="sticky top-0 z-50 border-b border-white/[0.08] bg-[#07090e]/80 backdrop-blur-2xl">
				<div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
					{/* Brand Logo */}
					<a href="/" className="flex items-center gap-3 group" aria-label={productName}>
						<div className="relative flex items-center justify-center">
							<img
								src={companyLogoSrc}
								alt={companyFullName}
								className="h-8 w-auto max-w-[160px] object-contain shrink-0 drop-shadow-[0_0_12px_rgba(56,189,248,0.25)]"
								onError={(e) => {
									const target = e.currentTarget;
									if (!target.src.endsWith("/yes-panchi-logo.png")) {
										target.src = "/yes-panchi-logo.png";
									}
								}}
							/>
							<span className="absolute -top-0.5 -right-0.5 flex h-2 w-2">
								<span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-sky-400 opacity-75"></span>
								<span className="relative inline-flex rounded-full h-2 w-2 bg-sky-500"></span>
							</span>
						</div>
						<span className="text-lg font-bold tracking-tight text-white group-hover:text-sky-300 transition-colors">
							{productName}
						</span>
					</a>

					{/* Navigation Links */}
					<nav className="hidden md:flex items-center gap-8 text-sm font-medium text-slate-300">
						<a href="#products" className="hover:text-white transition-colors">Products</a>
						<a href="#features" className="hover:text-white transition-colors">Capabilities</a>
						<a href="#quickstart" className="hover:text-white transition-colors">Developers</a>
						<a href="#models" className="hover:text-white transition-colors">Models</a>
						<a href="#faq" className="hover:text-white transition-colors">FAQ</a>
					</nav>

					{/* Actions */}
					<div className="hidden md:flex items-center gap-3">
						<button
							onClick={() => setIsDemoModalOpen(true)}
							className="text-xs font-semibold text-slate-300 hover:text-white border border-white/[0.12] hover:border-white/[0.25] bg-white/[0.04] hover:bg-white/[0.08] px-3.5 py-1.5 rounded-lg transition-all shadow-sm cursor-pointer"
						>
							Book a Demo
						</button>

						{isLoggedIn ? (
							<Button
								onClick={() => navigate({ to: "/workspace" })}
								className="bg-white hover:bg-slate-100 text-[#07090e] font-bold text-xs h-8 px-4 rounded-lg transition-all shadow-[0_0_20px_rgba(255,255,255,0.2)] hover:shadow-[0_0_25px_rgba(255,255,255,0.3)]"
							>
								Dashboard
								<ArrowRight className="h-3.5 w-3.5 ml-1" />
							</Button>
						) : (
							<div className="flex items-center gap-2">
								<Link
									to="/login"
									className="text-xs font-semibold text-slate-300 hover:text-white px-3 py-1.5 rounded-lg hover:bg-white/[0.05] transition-all"
								>
									Sign In
								</Link>
								<Button
									onClick={() => navigate({ to: "/signup" })}
									className="bg-white hover:bg-slate-100 text-[#07090e] font-bold text-xs h-8 px-4 rounded-lg transition-all shadow-[0_0_20px_rgba(255,255,255,0.15)] hover:shadow-[0_0_25px_rgba(255,255,255,0.3)]"
								>
									Get Started
								</Button>
							</div>
						)}
					</div>

					{/* Mobile Menu Button */}
					<button
						onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
						className="md:hidden text-slate-300 hover:text-white p-1.5 rounded-lg bg-white/[0.04] border border-white/[0.08]"
						aria-label="Toggle menu"
					>
						{mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
					</button>
				</div>

				{/* Mobile Menu Dropdown */}
				{mobileMenuOpen && (
					<div className="md:hidden border-b border-white/[0.08] bg-[#07090e]/95 backdrop-blur-2xl px-6 py-5 space-y-3">
						<a href="#products" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">Products</a>
						<a href="#features" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">Capabilities</a>
						<a href="#quickstart" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">Developers</a>
						<a href="#models" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">Models</a>
						<a href="#faq" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">FAQ</a>
						
						<div className="pt-4 border-t border-white/[0.08] flex flex-col gap-2.5">
							<button
								onClick={() => { setMobileMenuOpen(false); setIsDemoModalOpen(true); }}
								className="w-full py-2 rounded-lg border border-white/[0.15] bg-white/[0.04] text-slate-200 font-semibold text-xs text-center cursor-pointer"
							>
								Book a Demo
							</button>
							{isLoggedIn ? (
								<Button
									onClick={() => { setMobileMenuOpen(false); navigate({ to: "/workspace" }); }}
									className="w-full bg-white text-[#07090e] font-semibold text-xs h-9"
								>
									Go to Workspace
								</Button>
							) : (
								<div className="grid grid-cols-2 gap-2">
									<Link
										to="/login"
										onClick={() => setMobileMenuOpen(false)}
										className="border border-white/[0.12] bg-white/[0.03] text-center font-semibold text-slate-200 text-xs py-2 rounded-lg"
									>
										Sign In
									</Link>
									<Button
										onClick={() => { setMobileMenuOpen(false); navigate({ to: "/signup" }); }}
										className="bg-white text-[#07090e] font-semibold text-xs h-8 rounded-lg shadow-sm"
									>
										Get Started
									</Button>
								</div>
							)}
						</div>
					</div>
				)}
			</header>

			<main className="relative z-10">
				{/* Hero Section */}
				<section className="pt-20 pb-16 md:pt-28 md:pb-24 px-6 max-w-5xl mx-auto text-center">
					{/* Glowing Announcement Pill */}
					<div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full border border-sky-500/20 bg-sky-500/[0.06] text-xs font-medium text-sky-300 mb-8 backdrop-blur-md shadow-[0_0_20px_rgba(56,189,248,0.15)]">
						<Sparkles className="w-3.5 h-3.5 text-sky-400 animate-pulse" />
						<span>Enterprise AI Security & Universal Gateway 2.0</span>
						<ChevronRight className="w-3.5 h-3.5 text-sky-400/70" />
					</div>

					<h1 className="text-4xl sm:text-6xl md:text-7xl font-extrabold tracking-tight text-white mb-6 leading-[1.08]">
						Secure, Govern, and Accelerate <br className="hidden sm:inline" />
						<span className="bg-gradient-to-r from-sky-300 via-indigo-200 to-white bg-clip-text text-transparent drop-shadow-sm">
							AI Across Your Organization
						</span>
					</h1>

					<p className="text-base sm:text-lg text-slate-400 max-w-2xl mx-auto mb-10 leading-relaxed font-normal">
						UnifAI delivers zero-latency in-flight DLP protection for employee browser AI sessions, while providing an enterprise-grade drop-in gateway for high-throughput LLM workloads.
					</p>

					{/* Action CTAs */}
					<div className="flex flex-col sm:flex-row items-center justify-center gap-4 max-w-md mx-auto">
						<button
							onClick={() => setIsDemoModalOpen(true)}
							className="w-full sm:w-auto h-11 px-6 bg-white hover:bg-slate-100 text-[#07090e] font-bold rounded-xl text-sm transition-all shadow-[0_0_25px_rgba(255,255,255,0.2)] hover:shadow-[0_0_35px_rgba(255,255,255,0.35)] flex items-center justify-center gap-2 cursor-pointer"
						>
							<Calendar className="h-4 w-4 text-slate-700" />
							Schedule Demo
						</button>

						{isLoggedIn ? (
							<Button
								onClick={() => navigate({ to: "/workspace" })}
								className="w-full sm:w-auto h-11 px-6 bg-white/[0.06] hover:bg-white/[0.12] text-white border border-white/[0.15] font-semibold rounded-xl text-sm transition-all shadow-sm"
							>
								Open Control Plane
								<ArrowRight className="h-4 w-4 ml-1.5 text-slate-300" />
							</Button>
						) : (
							<Button
								onClick={() => navigate({ to: "/signup" })}
								className="w-full sm:w-auto h-11 px-6 bg-white/[0.06] hover:bg-white/[0.12] text-white border border-white/[0.15] font-semibold rounded-xl text-sm transition-all shadow-sm"
							>
								Start Free Trial
								<ArrowRight className="h-4 w-4 ml-1.5 text-slate-300" />
							</Button>
						)}
					</div>

					{/* Supported Models Strip */}
					<div id="models" className="mt-12 flex flex-wrap items-center justify-center gap-2.5 max-w-3xl mx-auto">
						<span className="text-xs font-mono text-slate-500 mr-2 uppercase tracking-wider">Unified Provider Access:</span>
						{supportedModels.map((m, idx) => (
							<div
								key={idx}
								className={`group inline-flex items-center gap-2 px-3 py-1 rounded-lg border border-white/[0.07] bg-white/[0.02] text-xs text-slate-300 hover:text-white transition-all ${m.glow}`}
							>
								<span className={`w-1.5 h-1.5 rounded-full ${m.color}`} />
								<span className="font-medium">{m.name}</span>
							</div>
						))}
					</div>

					{/* Ultra-Luxury Control Plane Architecture Card */}
					<div className="mt-14 sm:mt-16 border border-white/[0.1] rounded-2xl bg-[#0c0e17]/80 backdrop-blur-2xl p-6 sm:p-8 text-left shadow-[0_20px_50px_rgba(0,0,0,0.6)]">
						{/* Titlebar with Mac dots */}
						<div className="flex items-center justify-between pb-6 border-b border-white/[0.08]">
							<div className="flex items-center gap-3">
								<div className="flex items-center gap-1.5">
									<div className="h-2.5 w-2.5 rounded-full bg-red-500/80" />
									<div className="h-2.5 w-2.5 rounded-full bg-amber-500/80" />
									<div className="h-2.5 w-2.5 rounded-full bg-emerald-500/80" />
								</div>
								<span className="text-xs font-mono font-medium text-slate-300 ml-2">
									unifai-perimeter :: active security enforcement
								</span>
							</div>
							<div className="flex items-center gap-4 text-xs font-mono">
								<span className="hidden sm:inline text-slate-500">Node: v2.4-edge</span>
								<span className="flex items-center gap-1.5 text-emerald-400">
									<span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
									System 100% Operational
								</span>
							</div>
						</div>

						{/* Live Telemetry Metric Strip */}
						<div className="grid grid-cols-2 sm:grid-cols-4 gap-3 py-4 border-b border-white/[0.06] text-xs font-mono">
							<div className="p-2.5 rounded-lg bg-white/[0.02] border border-white/[0.04]">
								<div className="text-slate-500 text-[10px]">IN-FLIGHT OVERHEAD</div>
								<div className="text-emerald-400 font-bold text-sm mt-0.5">&lt; 0.8 ms</div>
							</div>
							<div className="p-2.5 rounded-lg bg-white/[0.02] border border-white/[0.04]">
								<div className="text-slate-500 text-[10px]">SEMANTIC CACHE RATIO</div>
								<div className="text-sky-400 font-bold text-sm mt-0.5">86.4% Hit Rate</div>
							</div>
							<div className="p-2.5 rounded-lg bg-white/[0.02] border border-white/[0.04]">
								<div className="text-slate-500 text-[10px]">REDACTED LEAKS</div>
								<div className="text-amber-400 font-bold text-sm mt-0.5">14,290 Blocked</div>
							</div>
							<div className="p-2.5 rounded-lg bg-white/[0.02] border border-white/[0.04]">
								<div className="text-slate-500 text-[10px]">DROP-IN COMPATIBILITY</div>
								<div className="text-indigo-400 font-bold text-sm mt-0.5">OpenAI SDK 100%</div>
							</div>
						</div>

						{/* Two Connected Core Engines */}
						<div className="grid grid-cols-1 md:grid-cols-2 gap-6 pt-6">
							{/* Component 1: Browser Guard */}
							<div className="border border-white/[0.08] hover:border-sky-500/30 rounded-xl bg-gradient-to-b from-white/[0.03] to-transparent p-5 space-y-4 transition-all">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2.5">
										<div className="h-9 w-9 rounded-lg bg-sky-500/10 border border-sky-500/20 flex items-center justify-center text-sky-400">
											<Shield className="h-4 w-4" />
										</div>
										<div>
											<h3 className="text-sm font-bold text-white">Browser Guard DLP Engine</h3>
											<p className="text-xs text-slate-400">Client Agent (Windows & macOS)</p>
										</div>
									</div>
									<span className="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
										ACTIVE
									</span>
								</div>

								<div className="text-xs text-slate-300 space-y-2 border-t border-white/[0.06] pt-3 font-mono">
									<div className="flex items-center justify-between text-slate-400">
										<span>Target AI Consoles:</span>
										<span className="text-white">ChatGPT, Claude, Grok, Gemini</span>
									</div>
									<div className="flex items-center justify-between text-slate-400">
										<span>Sanitization Action:</span>
										<span className="text-amber-300">Live Token / PII Scrubbing</span>
									</div>
									<div className="flex items-center justify-between text-slate-400">
										<span>Attachment Engine:</span>
										<span className="text-sky-300">PDF, OCR, Code File OCR</span>
									</div>
								</div>
							</div>

							{/* Component 2: AI Gateway */}
							<div className="border border-white/[0.08] hover:border-indigo-500/30 rounded-xl bg-gradient-to-b from-white/[0.03] to-transparent p-5 space-y-4 transition-all">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2.5">
										<div className="h-9 w-9 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
											<Zap className="h-4 w-4" />
										</div>
										<div>
											<h3 className="text-sm font-bold text-white">Universal AI Gateway</h3>
											<p className="text-xs text-slate-400">Application Reverse Proxy</p>
										</div>
									</div>
									<span className="text-[10px] font-mono px-2 py-0.5 rounded bg-sky-500/10 text-sky-400 border border-sky-500/20">
										CONNECTED
									</span>
								</div>

								<div className="text-xs text-slate-300 space-y-2 border-t border-white/[0.06] pt-3 font-mono">
									<div className="flex items-center justify-between text-slate-400">
										<span>Model Integration:</span>
										<span className="text-white">100+ Models Unified</span>
									</div>
									<div className="flex items-center justify-between text-slate-400">
										<span>Semantic Caching:</span>
										<span className="text-emerald-400">pgvector Active (-85% Cost)</span>
									</div>
									<div className="flex items-center justify-between text-slate-400">
										<span>Governance:</span>
										<span className="text-purple-300">Virtual Keys & Hard Budgets</span>
									</div>
								</div>
							</div>
						</div>
					</div>
				</section>

				{/* Trust & Enterprise Compliance Metrics */}
				<section className="border-y border-white/[0.08] bg-[#090b14]/70 py-10 px-6 backdrop-blur-md">
					<div className="max-w-6xl mx-auto flex flex-wrap items-center justify-around gap-8 text-center">
						<div className="space-y-1">
							<div className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight">50M+</div>
							<div className="text-xs text-slate-400 font-medium">Prompts Protected</div>
						</div>
						<div className="space-y-1">
							<div className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight">&lt; 0.8ms</div>
							<div className="text-xs text-slate-400 font-medium">Interception Overhead</div>
						</div>
						<div className="space-y-1">
							<div className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight">85%</div>
							<div className="text-xs text-slate-400 font-medium">API Spend Saved via Cache</div>
						</div>
						<div className="space-y-1">
							<div className="text-3xl sm:text-4xl font-extrabold text-white tracking-tight">99.99%</div>
							<div className="text-xs text-slate-400 font-medium">Enterprise Availability</div>
						</div>
					</div>

					<div className="max-w-4xl mx-auto mt-8 pt-6 border-t border-white/[0.06] flex flex-wrap items-center justify-center gap-6 text-xs text-slate-400">
						<span className="flex items-center gap-1.5"><CheckCircle className="h-3.5 w-3.5 text-slate-300" /> SOC 2 Type II Certified</span>
						<span className="flex items-center gap-1.5"><CheckCircle className="h-3.5 w-3.5 text-slate-300" /> HIPAA & GDPR Compliant</span>
						<span className="flex items-center gap-1.5"><CheckCircle className="h-3.5 w-3.5 text-slate-300" /> Zero-Trust Architecture</span>
						<span className="flex items-center gap-1.5"><CheckCircle className="h-3.5 w-3.5 text-slate-300" /> ISO/IEC 27001 Aligned</span>
					</div>
				</section>

				{/* Products Section (Two Pillars) */}
				<section id="products" className="py-24 px-6 max-w-6xl mx-auto">
					<div className="text-center max-w-2xl mx-auto mb-16">
						<h2 className="text-3xl sm:text-5xl font-extrabold tracking-tight text-white mb-4">
							Two Products. One Architecture.
						</h2>
						<p className="text-slate-400 text-sm sm:text-base leading-relaxed">
							Arm enterprise security teams with real-time DLP over consumer web AI, while giving developers a blisteringly fast LLM gateway with strict budget governance.
						</p>
					</div>

					<div className="grid grid-cols-1 md:grid-cols-2 gap-8">
						{/* Product 1: Browser Guard */}
						<div className="group border border-white/[0.08] hover:border-sky-500/40 rounded-2xl bg-[#0c0e17]/70 p-8 space-y-6 transition-all duration-300 shadow-xl">
							<div className="h-11 w-11 rounded-xl bg-sky-500/10 border border-sky-500/25 flex items-center justify-center text-sky-400 group-hover:scale-110 transition-transform">
								<Shield className="h-5 w-5" />
							</div>

							<div className="space-y-2">
								<h3 className="text-2xl font-bold text-white">UnifAI Browser Guard</h3>
								<p className="text-slate-400 text-sm leading-relaxed">
									Endpoint DLP daemon for macOS and Windows. Automatically protects confidential IP whenever employees utilize ChatGPT, Claude, Grok, or custom AI portals.
								</p>
							</div>

							<div className="space-y-3 pt-2">
								{[
									"Zero-latency prompt redaction and enterprise policy blocking",
									"Multi-format document and source code upload inspection",
									"Complete coverage for Grok, ChatGPT, Claude, and Gemini",
									"Silent fleet rollout via Jamf, Microsoft Intune, or GPO"
								].map((item, idx) => (
									<div key={idx} className="flex items-center gap-2.5 text-xs text-slate-300">
										<CheckCircle2 className="h-4 w-4 text-sky-400 shrink-0" />
										<span>{item}</span>
									</div>
								))}
							</div>

							<div className="pt-2">
								<button
									onClick={() => setIsDemoModalOpen(true)}
									className="text-xs font-bold text-sky-400 hover:text-sky-300 flex items-center gap-1.5 cursor-pointer"
								>
									Explore Browser Guard Specs <ArrowRight className="h-3.5 w-3.5" />
								</button>
							</div>
						</div>

						{/* Product 2: AI Gateway */}
						<div className="group border border-white/[0.08] hover:border-indigo-500/40 rounded-2xl bg-[#0c0e17]/70 p-8 space-y-6 transition-all duration-300 shadow-xl">
							<div className="h-11 w-11 rounded-xl bg-indigo-500/10 border border-indigo-500/25 flex items-center justify-center text-indigo-400 group-hover:scale-110 transition-transform">
								<Zap className="h-5 w-5" />
							</div>

							<div className="space-y-2">
								<h3 className="text-2xl font-bold text-white">UnifAI Universal Gateway</h3>
								<p className="text-slate-400 text-sm leading-relaxed">
									Centralized LLM reverse proxy for production apps. Unifies model routing, semantic vector caching, rate limits, and team access tokens behind a single drop-in API.
								</p>
							</div>

							<div className="space-y-3 pt-2">
								{[
									"1-line OpenAI SDK drop-in replacement across languages",
									"pgvector semantic caching slashes LLM token costs by up to 85%",
									"Multi-entity Virtual Keys with strict team & user spend caps",
									"Automated cross-provider failover during LLM outages"
								].map((item, idx) => (
									<div key={idx} className="flex items-center gap-2.5 text-xs text-slate-300">
										<CheckCircle2 className="h-4 w-4 text-indigo-400 shrink-0" />
										<span>{item}</span>
									</div>
								))}
							</div>

							<div className="pt-2">
								<button
									onClick={() => setIsDemoModalOpen(true)}
									className="text-xs font-bold text-indigo-400 hover:text-indigo-300 flex items-center gap-1.5 cursor-pointer"
								>
									Explore Gateway Specs <ArrowRight className="h-3.5 w-3.5" />
								</button>
							</div>
						</div>
					</div>
				</section>

				{/* Capabilities Grid */}
				<section id="features" className="py-24 px-6 border-t border-white/[0.08] bg-[#090b14]/50">
					<div className="max-w-6xl mx-auto">
						<div className="text-center max-w-2xl mx-auto mb-16">
							<h2 className="text-3xl sm:text-4xl font-extrabold tracking-tight text-white mb-4">
								Enterprise Grade from Day One
							</h2>
							<p className="text-slate-400 text-sm sm:text-base">
								Everything required to satisfy stringent enterprise InfoSec, compliance, and developer productivity demands.
							</p>
						</div>

						<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
							{[
								{
									icon: <Lock className="h-5 w-5 text-sky-400" />,
									title: "In-Flight Data Redaction",
									desc: "API credentials, customer PII, and sensitive source code are sanitized before requests ever leave your perimeter."
								},
								{
									icon: <Layers className="h-5 w-5 text-indigo-400" />,
									title: "Semantic Vector Caching",
									desc: "High-performance vector embeddings catch semantically identical questions, answering in < 10ms at 0 token cost."
								},
								{
									icon: <Key className="h-5 w-5 text-amber-400" />,
									title: "Virtual Keys & Quotas",
									desc: "Provision scoped virtual API keys for teams and microservices with hard spend limits and budget double-locks."
								},
								{
									icon: <Activity className="h-5 w-5 text-emerald-400" />,
									title: "Audit Logs & Tracing",
									desc: "Full observability into prompt metadata, token usage, latency metrics, and policy trigger events across your company."
								},
								{
									icon: <Laptop className="h-5 w-5 text-cyan-400" />,
									title: "Cross-Platform Agents",
									desc: "Lightweight, silent installers for macOS and Windows, manageable through existing MDM fleets."
								},
								{
									icon: <Cpu className="h-5 w-5 text-violet-400" />,
									title: "Automatic Model Fallbacks",
									desc: "Configure automated multi-model failover chains so applications remain resilient during third-party AI outages."
								}
							].map((feature, idx) => (
								<div
									key={idx}
									className="group border border-white/[0.07] hover:border-white/[0.18] rounded-2xl bg-[#0c0e17]/70 p-6 space-y-3.5 transition-all shadow-md"
								>
									<div className="h-10 w-10 rounded-xl bg-white/[0.04] border border-white/[0.08] flex items-center justify-center group-hover:scale-105 transition-transform">
										{feature.icon}
									</div>
									<h3 className="text-base font-bold text-white">{feature.title}</h3>
									<p className="text-xs text-slate-400 leading-relaxed">{feature.desc}</p>
								</div>
							))}
						</div>
					</div>
				</section>

				{/* Quickstart Code Section */}
				<section id="quickstart" className="py-24 px-6 max-w-6xl mx-auto">
					<div className="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
						<div className="space-y-6">
							<div className="inline-flex items-center gap-2 px-3 py-1 rounded-full border border-indigo-500/20 bg-indigo-500/[0.06] text-xs font-semibold text-indigo-400">
								<Terminal className="w-3.5 h-3.5" />
								<span>Zero SDK Refactoring</span>
							</div>

							<h2 className="text-3xl sm:text-5xl font-extrabold tracking-tight text-white leading-tight">
								One Line of Code to Supercharge AI
							</h2>
							<p className="text-slate-400 text-sm sm:text-base leading-relaxed">
								UnifAI is 100% compliant with standard OpenAI SDKs and specs. Simply update your connection base URL to instantly unlock semantic caching, virtual key quotas, and automatic multi-model failover.
							</p>

							<div className="space-y-3 pt-2">
								{[
									"Zero SDK changes or refactoring necessary",
									"Compatible with Python, Node.js, Go, LangChain, and cURL",
									"Seamless fallback between OpenAI, Anthropic, Gemini, and DeepSeek"
								].map((item, idx) => (
									<div key={idx} className="flex items-center gap-2 text-xs text-slate-300 font-medium">
										<Check className="h-4 w-4 text-emerald-400 shrink-0" />
										<span>{item}</span>
									</div>
								))}
							</div>
						</div>

						{/* Code Box with macOS styling */}
						<div className="border border-white/[0.1] rounded-2xl bg-[#090b12] overflow-hidden shadow-2xl">
							<div className="flex bg-white/[0.03] border-b border-white/[0.08] px-4 py-3 justify-between items-center">
								<div className="flex items-center gap-2">
									<div className="flex gap-1.5 mr-2">
										<div className="w-2.5 h-2.5 rounded-full bg-red-500/60" />
										<div className="w-2.5 h-2.5 rounded-full bg-amber-500/60" />
										<div className="w-2.5 h-2.5 rounded-full bg-emerald-500/60" />
									</div>

									<button
										onClick={() => setCodeTab("python")}
										className={`text-xs font-semibold px-3 py-1 rounded-md transition-colors cursor-pointer ${
											codeTab === "python" ? "bg-white/[0.12] text-white shadow-sm" : "text-slate-400 hover:text-white"
										}`}
									>
										Python
									</button>
									<button
										onClick={() => setCodeTab("node")}
										className={`text-xs font-semibold px-3 py-1 rounded-md transition-colors cursor-pointer ${
											codeTab === "node" ? "bg-white/[0.12] text-white shadow-sm" : "text-slate-400 hover:text-white"
										}`}
									>
										Node.js
									</button>
									<button
										onClick={() => setCodeTab("curl")}
										className={`text-xs font-semibold px-3 py-1 rounded-md transition-colors cursor-pointer ${
											codeTab === "curl" ? "bg-white/[0.12] text-white shadow-sm" : "text-slate-400 hover:text-white"
										}`}
									>
										cURL
									</button>
								</div>

								<button
									onClick={() => handleCopyCode(codeExamples[codeTab])}
									className="text-xs text-slate-400 hover:text-white flex items-center gap-1.5 font-mono px-2 py-1 rounded-md hover:bg-white/[0.06] transition-colors cursor-pointer"
								>
									{copiedCode ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
									<span>{copiedCode ? "Copied" : "Copy"}</span>
								</button>
							</div>

							<div className="p-5 overflow-x-auto text-xs font-mono leading-relaxed bg-[#06070a] text-slate-300 max-h-[320px]">
								<pre className="whitespace-pre">{codeExamples[codeTab]}</pre>
							</div>
						</div>
					</div>
				</section>

				{/* FAQ Section */}
				<section id="faq" className="py-24 px-6 border-t border-white/[0.08] bg-[#090b14]/50">
					<div className="max-w-3xl mx-auto">
						<div className="text-center mb-12">
							<h2 className="text-3xl sm:text-4xl font-extrabold tracking-tight text-white mb-3">
								Frequently Asked Questions
							</h2>
							<p className="text-slate-400 text-sm">
								Answers to common enterprise security, performance, and deployment inquiries.
							</p>
						</div>

						<div className="space-y-3.5">
							{faqs.map((faq, idx) => (
								<div
									key={idx}
									className="border border-white/[0.08] rounded-xl bg-[#0c0e17]/70 overflow-hidden shadow-sm transition-all"
								>
									<button
										onClick={() => setOpenFaq(openFaq === idx ? null : idx)}
										className="w-full p-4 sm:p-5 text-left font-semibold text-sm text-white flex items-center justify-between gap-4 hover:text-slate-200 transition-colors cursor-pointer"
									>
										<span>{faq.q}</span>
										<ChevronDown className={`h-4 w-4 text-slate-400 shrink-0 transition-transform duration-200 ${openFaq === idx ? "rotate-180" : ""}`} />
									</button>
									{openFaq === idx && (
										<div className="px-4 sm:px-5 pb-5 text-xs sm:text-sm text-slate-400 leading-relaxed border-t border-white/[0.06] pt-3.5">
											{faq.a}
										</div>
									)}
								</div>
							))}
						</div>
					</div>
				</section>

				{/* Clean Bottom CTA */}
				<section className="py-28 px-6 text-center max-w-4xl mx-auto">
					<div className="p-8 sm:p-12 rounded-3xl border border-white/[0.1] bg-gradient-to-b from-[#0c0e1a] to-[#07080f] shadow-2xl relative overflow-hidden">
						<div className="absolute -top-[50%] left-1/2 -translate-x-1/2 w-[500px] h-[300px] bg-sky-500/[0.08] rounded-full blur-[100px] pointer-events-none" />
						
						<h2 className="text-3xl sm:text-5xl font-extrabold tracking-tight text-white mb-4 relative z-10">
							Ready to Standardize Your AI Perimeter?
						</h2>
						<p className="text-slate-400 text-sm sm:text-base max-w-xl mx-auto mb-8 relative z-10 leading-relaxed">
							Connect with our enterprise security team for a customized deployment plan or start immediately in our cloud sandbox.
						</p>
						<div className="flex flex-col sm:flex-row items-center justify-center gap-3.5 relative z-10">
							<button
								onClick={() => setIsDemoModalOpen(true)}
								className="w-full sm:w-auto h-11 px-7 bg-white hover:bg-slate-100 text-[#07090e] font-bold rounded-xl text-sm transition-all shadow-[0_0_25px_rgba(255,255,255,0.25)] flex items-center justify-center gap-2 cursor-pointer"
							>
								<Calendar className="h-4 w-4" />
								Schedule a Demo
							</button>
							{isLoggedIn ? (
								<Button
									onClick={() => navigate({ to: "/workspace" })}
									className="w-full sm:w-auto h-11 px-7 bg-white/[0.06] hover:bg-white/[0.12] text-white border border-white/[0.15] font-semibold rounded-xl text-sm shadow-sm"
								>
									Go to Workspace
								</Button>
							) : (
								<Button
									onClick={() => navigate({ to: "/signup" })}
									className="w-full sm:w-auto h-11 px-7 bg-white/[0.06] hover:bg-white/[0.12] text-white border border-white/[0.15] font-semibold rounded-xl text-sm shadow-sm"
								>
									Get Started Free
								</Button>
							)}
						</div>
					</div>
				</section>
			</main>

			{/* Minimalist High-End Footer */}
			<footer className="border-t border-white/[0.08] bg-[#05060a] py-12 px-6">
				<div className="max-w-6xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-6 text-xs text-slate-500">
					<div className="flex items-center gap-3">
						<img src={companyLogoSrc} alt={companyFullName} className="h-6 w-auto object-contain opacity-80" />
						<span className="font-semibold text-slate-300">{companyFullName}</span>
					</div>

					<div className="flex items-center gap-6">
						<a href="#products" className="hover:text-slate-300 transition-colors">Products</a>
						<a href="#features" className="hover:text-slate-300 transition-colors">Features</a>
						<button onClick={() => setIsDemoModalOpen(true)} className="hover:text-slate-300 transition-colors cursor-pointer">Book Demo</button>
						<Link to="/login" className="hover:text-slate-300 transition-colors">Sign In</Link>
						<Link to="/signup" className="hover:text-slate-300 transition-colors">Sign Up</Link>
					</div>

					<div>&copy; 2026 {companyFullName}. All rights reserved.</div>
				</div>
			</footer>

			{/* High-End "Book a Demo" Modal */}
			<Dialog open={isDemoModalOpen} onOpenChange={setIsDemoModalOpen}>
				<DialogContent className="bg-[#0c0e17] border border-white/[0.12] text-slate-200 sm:max-w-lg p-6 shadow-2xl rounded-2xl">
					<DialogHeader>
						<DialogTitle className="text-lg font-bold text-white flex items-center gap-2">
							<Calendar className="h-4 w-4 text-sky-400" />
							Schedule an Enterprise Demo
						</DialogTitle>
						<DialogDescription className="text-slate-400 text-xs">
							Our engineering team will provide a tailored 20-minute architecture demonstration of Browser Guard and the Universal AI Gateway.
						</DialogDescription>
					</DialogHeader>

					{demoSubmitted ? (
						<div className="py-8 text-center space-y-3">
							<div className="h-12 w-12 rounded-full bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400 mx-auto">
								<CheckCircle2 className="h-6 w-6" />
							</div>
							<h3 className="text-base font-bold text-white">Demo Request Received</h3>
							<p className="text-xs text-slate-400 max-w-sm mx-auto leading-relaxed">
								Thank you, <strong className="text-white">{demoForm.fullName}</strong>. A technical architect has sent a confirmation calendar invite to <strong className="text-white">{demoForm.email}</strong>.
							</p>
							<div className="pt-2">
								<Button
									onClick={resetDemoModal}
									className="bg-white text-[#07090e] font-bold text-xs h-8 px-5 rounded-lg"
								>
									Done
								</Button>
							</div>
						</div>
					) : (
						<form onSubmit={handleDemoSubmit} className="space-y-4 mt-2">
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div className="space-y-1">
									<label className="text-xs font-semibold text-slate-300">Full Name *</label>
									<Input
										required
										placeholder="Jane Doe"
										value={demoForm.fullName}
										onChange={(e) => setDemoForm({ ...demoForm, fullName: e.target.value })}
										className="bg-white/[0.04] border-white/[0.1] text-white text-xs h-9 focus:border-white/[0.3] rounded-lg"
									/>
								</div>

								<div className="space-y-1">
									<label className="text-xs font-semibold text-slate-300">Work Email *</label>
									<Input
										type="email"
										required
										placeholder="jane@company.com"
										value={demoForm.email}
										onChange={(e) => setDemoForm({ ...demoForm, email: e.target.value })}
										className="bg-white/[0.04] border-white/[0.1] text-white text-xs h-9 focus:border-white/[0.3] rounded-lg"
									/>
								</div>
							</div>

							<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div className="space-y-1">
									<label className="text-xs font-semibold text-slate-300">Company</label>
									<Input
										placeholder="Acme Corp"
										value={demoForm.company}
										onChange={(e) => setDemoForm({ ...demoForm, company: e.target.value })}
										className="bg-white/[0.04] border-white/[0.1] text-white text-xs h-9 focus:border-white/[0.3] rounded-lg"
									/>
								</div>

								<div className="space-y-1">
									<label className="text-xs font-semibold text-slate-300">Team Size</label>
									<select
										value={demoForm.teamSize}
										onChange={(e) => setDemoForm({ ...demoForm, teamSize: e.target.value })}
										className="w-full bg-[#0c0e17] border border-white/[0.1] text-white text-xs h-9 rounded-lg px-2.5 focus:outline-none focus:border-white/[0.3]"
									>
										<option value="1-50">1 - 50 employees</option>
										<option value="50-250">50 - 250 employees</option>
										<option value="250-1000">250 - 1,000 employees</option>
										<option value="1000+">1,000+ employees</option>
									</select>
								</div>
							</div>

							<div className="space-y-1">
								<label className="text-xs font-semibold text-slate-300">Primary Focus</label>
								<div className="grid grid-cols-3 gap-2 text-xs">
									{[
										{ id: "guard", label: "Browser DLP" },
										{ id: "gateway", label: "AI Gateway" },
										{ id: "both", label: "Both" },
									].map((opt) => (
										<button
											type="button"
											key={opt.id}
											onClick={() => setDemoForm({ ...demoForm, interest: opt.id })}
											className={`py-2 px-2 text-center rounded-lg border text-xs font-medium transition-all cursor-pointer ${
												demoForm.interest === opt.id
													? "border-sky-500/50 bg-sky-500/10 text-white font-bold"
													: "border-white/[0.08] bg-white/[0.02] text-slate-400 hover:text-white"
											}`}
										>
											{opt.label}
										</button>
									))}
								</div>
							</div>

							<div className="space-y-1">
								<label className="text-xs font-semibold text-slate-300">Architecture Requirements (Optional)</label>
								<textarea
									rows={2}
									placeholder="Tell us about your team's AI use cases, compliance needs, or models..."
									value={demoForm.notes}
									onChange={(e) => setDemoForm({ ...demoForm, notes: e.target.value })}
									className="w-full bg-white/[0.04] border border-white/[0.1] rounded-lg p-2.5 text-white text-xs focus:outline-none focus:border-white/[0.3] resize-none"
								/>
							</div>

							<div className="pt-2 flex items-center justify-end gap-2">
								<Button
									type="button"
									variant="ghost"
									onClick={() => setIsDemoModalOpen(false)}
									className="text-xs h-8 text-slate-400 hover:text-white"
								>
									Cancel
								</Button>
								<Button
									type="submit"
									className="bg-white hover:bg-slate-100 text-[#07090e] font-bold text-xs h-8 px-4 rounded-lg shadow-sm"
								>
									Submit Request
								</Button>
							</div>
						</form>
					)}
				</DialogContent>
			</Dialog>
		</div>
	);
}
