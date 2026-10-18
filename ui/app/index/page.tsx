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
	Loader2
} from "lucide-react";
import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/utils/port";
import { COMPANY_LOGO, COMPANY_NAME, DEFAULT_FOOTER_TEXT } from "@/lib/constants/config";
import { useBranding } from "@/lib/hooks/useBranding";

export default function LandingPage() {
	const branding = useBranding();
	const [isLoggedIn, setIsLoggedIn] = useState(false);
	const [codeTab, setCodeTab] = useState<"python" | "curl" | "node">("python");
	const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
	const [copiedCode, setCopiedCode] = useState(false);
	const [openFaq, setOpenFaq] = useState<number | null>(0);
	const navigate = useNavigate();

	// Demo Booking Modal State
	const [isDemoModalOpen, setIsDemoModalOpen] = useState(false);
	const [demoSubmitted, setDemoSubmitted] = useState(false);
	const [isSubmittingDemo, setIsSubmittingDemo] = useState(false);
	const [demoError, setDemoError] = useState("");
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

	// Smooth scrolling helper for nav links with top bar offset
	const scrollToSection = (id: string) => {
		setMobileMenuOpen(false);
		const targetId = id === "capabilities" ? "features" : id === "developers" ? "quickstart" : id;
		const element = document.getElementById(targetId) || document.getElementById(id);
		if (!element) return;
		const top = element.getBoundingClientRect().top + window.scrollY - 64;
		window.scrollTo({ top: Math.max(0, top), behavior: "smooth" });
	};

	const handleCopyCode = (text: string) => {
		navigator.clipboard.writeText(text);
		setCopiedCode(true);
		setTimeout(() => setCopiedCode(false), 2000);
	};

	const handleDemoSubmit = async (e: React.FormEvent) => {
		e.preventDefault();
		if (!demoForm.fullName || !demoForm.email) return;
		setIsSubmittingDemo(true);
		setDemoError("");
		try {
			const res = await fetch(`${getApiBaseUrl()}/session/schedule-demo`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					full_name: demoForm.fullName.trim(),
					email: demoForm.email.trim(),
					company: demoForm.company.trim(),
					team_size: demoForm.teamSize,
					interest: demoForm.interest,
					notes: demoForm.notes.trim(),
				}),
			});
			const data = await res.json();
			if (!res.ok) {
				throw new Error(data?.error?.message || data?.message || "Failed to schedule demonstration. Please try again.");
			}
			setDemoSubmitted(true);
		} catch (err: any) {
			setDemoError(err?.message || "Unable to send demo schedule. Please check your network and try again.");
		} finally {
			setIsSubmittingDemo(false);
		}
	};

	const resetDemoModal = () => {
		setDemoSubmitted(false);
		setIsDemoModalOpen(false);
		setIsSubmittingDemo(false);
		setDemoError("");
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
    base_url="https://gateway.yourcompany.com/v1",
    api_key="gateway_vk_live_enterprise_99x"
)

# Unified routing across OpenAI, Claude, Gemini, DeepSeek
response = client.chat.completions.create(
    model="claude-3-7-sonnet", # or gpt-4o, gemini-2.0-flash, deepseek-r1
    messages=[{"role": "user", "content": "Analyze quarterly enterprise security report"}]
)

print(response.choices[0].message.content)`,
		curl: `# Universal OpenAI-compatible cURL endpoint
curl -X POST https://gateway.yourcompany.com/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer gateway_vk_live_enterprise_99x" \\
  -d '{
    "model": "gpt-4o",
    "messages": [
      {
        "role": "user",
        "content": "Verify DLP redaction and semantic cache hit rate"
      }
    ]
  }'`,
		node: `// Drop-in compatible with standard Node OpenAI SDK
import OpenAI from "openai";

const openai = new OpenAI({
  baseURL: "https://gateway.yourcompany.com/v1",
  apiKey: "gateway_vk_live_enterprise_99x"
});

const response = await openai.chat.completions.create({
  model: "claude-3-7-sonnet",
  messages: [{ role: "user", content: "Optimize multi-tenant virtual key rate limits" }]
});

console.log(response.choices[0].message.content);`
	};

	const supportedModels = [
		{ name: "OpenAI GPT-4o", color: "bg-emerald-400", note: "Chat, tools, and vision on one route." },
		{ name: "Anthropic Claude 3.7", color: "bg-amber-400", note: "Long context and careful reasoning." },
		{ name: "Google Gemini 2.0", color: "bg-sky-400", note: "Multimodal requests on the same API." },
		{ name: "DeepSeek R1", color: "bg-blue-400", note: "Reasoning models without a second client." },
		{ name: "Meta Llama 3.3", color: "bg-purple-400", note: "Open models behind the same virtual key." },
		{ name: "Mistral Large", color: "bg-orange-400", note: "Additional providers on the same gateway." },
	];

	const faqs = [
		{
			q: "How does Browser Guard protect corporate data in employee AI web tools?",
			a: "GATEWAY Browser Guard runs as a lightweight, zero-latency endpoint daemon on macOS and Windows. When staff interact with ChatGPT, Claude, or Grok, Browser Guard intercepts prompts and document attachments in-flight, immediately sanitizing API keys, passwords, customer PII, and proprietary code before packets exit the machine."
		},
		{
			q: "Does Browser Guard slow down employee network connection or browsing?",
			a: "Not at all. Browser Guard uses local loopback inspection dedicated strictly to configured AI domain destinations. Interception overhead benchmarks under 0.8ms, and standard internet browsing stays 100% native with zero speed penalty."
		},
		{
			q: "How does the Universal AI Gateway save API spend with Semantic Caching?",
			a: "The gateway maintains a pgvector vectorized semantic memory. When incoming prompts are semantically equivalent to previously answered queries, GATEWAY serves the response directly with sub-10ms latency and 0 provider token cost, routinely cutting corporate LLM bills by 70% to 85%."
		},
		{
			q: "Can GATEWAY run in an air-gapped on-premise datacenter or private VPC?",
			a: "Yes. GATEWAY compiles to a standalone high-performance Go binary with PostgreSQL. You can deploy it seamlessly across Kubernetes, Docker, AWS ECS/EKS, Azure, or air-gapped bare metal so that sensitive data never leaves your internal cloud perimeter."
		},
		{
			q: "How do we deploy the desktop agent across hundreds of employee workstations?",
			a: "GATEWAY ships with pre-configured silent installers for Windows (MSI/EXE) and macOS (PKG/ZIP). IT administrators can roll it out in minutes via Microsoft Intune, Jamf Pro, Kandji, or Active Directory Group Policy."
		}
	];

	const companyLogoSrc = branding.companyLogo;
	// Home page keeps the Gateway brand mark; workspace/emails use productName (Gateway).
	const productName = branding.brandName;
	const productFullName = branding.productFullName;
	const companyFullName = branding.companyName;

	return (
		<div className="bg-[#f6f8fd] text-slate-600 min-h-screen font-sans selection:bg-blue-500/20 selection:text-[#0b1b3a] antialiased relative w-full overflow-y-visible">
			<div className="fixed inset-0 pointer-events-none overflow-hidden z-0">
				<div className="absolute inset-x-0 top-0 h-[640px] bg-[radial-gradient(ellipse_80%_60%_at_50%_-10%,#d9e6ff_0%,rgba(255,255,255,0)_70%)]" />
				<div className="absolute top-28 right-[8%] w-56 h-56 rounded-full bg-[#e7e2ff]/70 blur-3xl" />
				<div className="absolute top-40 left-[6%] w-48 h-48 rounded-full bg-[#d7e8ff]/80 blur-3xl" />
			</div>

			<header className="fixed top-0 left-0 right-0 z-50 w-full border-b border-[#dbe4f5] bg-white/85 backdrop-blur-xl shadow-[0_8px_30px_rgba(47,107,255,0.06)]">
				<div className="max-w-[1440px] mx-auto px-6 lg:px-10 xl:px-14 h-16 flex items-center justify-between">
					{/* Brand Logo & Tag */}
					<a href="/" className="flex items-center gap-3.5 group cursor-pointer" aria-label={productName}>
						<div className="flex items-center justify-center p-1.5 rounded-xl bg-white border border-[#e3ebf8] group-hover:border-[#2f6bff]/40 transition-all shadow-sm">
							<img
								src={companyLogoSrc}
								alt={companyFullName}
								className="h-8 w-auto max-w-[150px] object-contain shrink-0"
								onError={(e) => {
									const target = e.currentTarget;
									if (!target.src.endsWith("/yes-panchi-logo.png")) {
										target.src = "/yes-panchi-logo.png";
									}
								}}
							/>
						</div>
						<div className="flex flex-col">
							<div className="flex items-center gap-2">
								<span className="text-xl font-extrabold tracking-tight text-[#0b1b3a] group-hover:text-[#2f6bff] transition-colors">
									{productName}
								</span>
								<span className="flex h-2 w-2 relative">
									<span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
									<span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
								</span>
							</div>
						</div>
					</a>

					{/* Navigation Links with Smooth Scroll Click */}
					<nav className="hidden md:flex items-center gap-1 text-sm font-semibold text-slate-600 bg-white px-2 py-1.5 rounded-full border border-[#e3ebf8] shadow-sm">
						<button
							type="button"
							onClick={() => scrollToSection("products")}
							className="px-3.5 py-1.5 rounded-full text-slate-600 hover:text-[#2f6bff] hover:bg-[#eef3ff] transition-all cursor-pointer"
						>
							Products
						</button>
						<button
							type="button"
							onClick={() => scrollToSection("features")}
							className="px-3.5 py-1.5 rounded-full text-slate-600 hover:text-[#2f6bff] hover:bg-[#eef3ff] transition-all cursor-pointer"
						>
							Capabilities
						</button>
						<button
							type="button"
							onClick={() => scrollToSection("quickstart")}
							className="px-3.5 py-1.5 rounded-full text-slate-600 hover:text-[#2f6bff] hover:bg-[#eef3ff] transition-all cursor-pointer"
						>
							Developers
						</button>
						<button
							type="button"
							onClick={() => scrollToSection("models")}
							className="px-3.5 py-1.5 rounded-full text-slate-600 hover:text-[#2f6bff] hover:bg-[#eef3ff] transition-all cursor-pointer"
						>
							Models
						</button>
						<button
							type="button"
							onClick={() => scrollToSection("faq")}
							className="px-3.5 py-1.5 rounded-full text-slate-600 hover:text-[#2f6bff] hover:bg-[#eef3ff] transition-all cursor-pointer"
						>
							FAQ
						</button>
					</nav>

					{/* Action Buttons */}
					<div className="hidden md:flex items-center gap-3">
						<button
							onClick={() => setIsDemoModalOpen(true)}
							className="text-xs font-bold text-[#0b1b3a] hover:text-[#2f6bff] border border-[#d5e0f2] hover:border-[#2f6bff]/40 bg-white hover:bg-[#f4f7ff] px-4 py-2 rounded-xl transition-all shadow-sm flex items-center gap-1.5 cursor-pointer"
						>
							<Calendar className="w-3.5 h-3.5 text-[#2f6bff]" />
							Book a Demo
						</button>

						{isLoggedIn ? (
							<Button
								onClick={() => navigate({ to: "/workspace" })}
								className="bg-[#2f6bff] hover:bg-[#2457d6] text-white font-bold text-xs h-9 px-5 rounded-xl transition-all shadow-[0_8px_20px_rgba(47,107,255,0.28)] cursor-pointer"
							>
								Dashboard
								<ArrowRight className="h-3.5 w-3.5 ml-1.5" />
							</Button>
						) : (
							<div className="flex items-center gap-2.5">
								<Link
									to="/login"
									className="text-xs font-semibold text-slate-600 hover:text-[#2f6bff] px-3.5 py-2 rounded-xl hover:bg-[#eef3ff] transition-all cursor-pointer"
								>
									Sign In
								</Link>
								<Button
									onClick={() => navigate({ to: "/signup" })}
									className="bg-[#2f6bff] hover:bg-[#2457d6] text-white font-bold text-xs h-9 px-5 rounded-xl transition-all shadow-[0_8px_20px_rgba(47,107,255,0.28)] cursor-pointer"
								>
									Get Started
								</Button>
							</div>
						)}
					</div>

					{/* Mobile Menu Toggle */}
					<button
						onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
						className="md:hidden text-[#0b1b3a] hover:text-[#2f6bff] p-2 rounded-xl bg-white border border-[#dbe4f5] cursor-pointer"
						aria-label="Toggle menu"
					>
						{mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
					</button>
				</div>

				{/* Mobile Menu Dropdown */}
				{mobileMenuOpen && (
					<div className="md:hidden border-b border-[#dbe4f5] bg-white/98 backdrop-blur-xl px-6 py-5 space-y-3">
						<button onClick={() => scrollToSection("products")} className="block w-full text-left py-2 text-sm font-semibold text-[#0b1b3a] hover:text-[#2f6bff]">Products</button>
						<button onClick={() => scrollToSection("features")} className="block w-full text-left py-2 text-sm font-semibold text-[#0b1b3a] hover:text-[#2f6bff]">Capabilities</button>
						<button onClick={() => scrollToSection("quickstart")} className="block w-full text-left py-2 text-sm font-semibold text-[#0b1b3a] hover:text-[#2f6bff]">Developers</button>
						<button onClick={() => scrollToSection("models")} className="block w-full text-left py-2 text-sm font-semibold text-[#0b1b3a] hover:text-[#2f6bff]">Models</button>
						<button onClick={() => scrollToSection("faq")} className="block w-full text-left py-2 text-sm font-semibold text-[#0b1b3a] hover:text-[#2f6bff]">FAQ</button>
						
						<div className="pt-4 border-t border-[#e3ebf8] flex flex-col gap-2.5">
							<button
								onClick={() => { setMobileMenuOpen(false); setIsDemoModalOpen(true); }}
								className="w-full py-2.5 rounded-xl border border-[#d5e0f2] bg-white text-[#0b1b3a] font-bold text-xs text-center cursor-pointer flex items-center justify-center gap-2"
							>
								<Calendar className="w-3.5 h-3.5 text-[#2f6bff]" />
								Book a Demo
							</button>
							{isLoggedIn ? (
								<Button
									onClick={() => { setMobileMenuOpen(false); navigate({ to: "/workspace" }); }}
									className="w-full bg-[#2f6bff] text-white font-bold text-xs h-9 rounded-xl"
								>
									Go to Workspace
								</Button>
							) : (
								<div className="grid grid-cols-2 gap-2">
									<Link
										to="/login"
										onClick={() => setMobileMenuOpen(false)}
										className="border border-[#d5e0f2] bg-white text-center font-bold text-[#0b1b3a] text-xs py-2.5 rounded-xl"
									>
										Sign In
									</Link>
									<Button
										onClick={() => { setMobileMenuOpen(false); navigate({ to: "/signup" }); }}
										className="bg-[#2f6bff] text-white font-bold text-xs h-9 rounded-xl shadow-sm"
									>
										Get Started
									</Button>
								</div>
							)}
						</div>
					</div>
				)}
			</header>

			{/* MAIN BODY CONTENT (SCROLLABLE UNDER FIXED HEADER) */}
			<main className="relative z-10 w-full pt-16">
				<section className="px-6 lg:px-10 xl:px-14 min-h-[calc(100vh-4rem)] flex items-center py-6">
					<div className="max-w-[1440px] mx-auto w-full grid grid-cols-1 lg:grid-cols-2 gap-8 xl:gap-14 items-center">
					<div className="text-left">
					<div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full border border-[#d5e2ff] bg-white text-[13px] font-semibold text-[#2f6bff] mb-4 shadow-[0_6px_18px_rgba(47,107,255,0.08)]">
						<Sparkles className="w-3.5 h-3.5" />
						<span>{productName} — {productFullName}</span>
					</div>

					<h1 className="text-4xl sm:text-5xl font-extrabold tracking-[-0.03em] text-[#0b1b3a] leading-[1.08]">
						Secure, govern, and
						<span className="block bg-gradient-to-r from-[#2f6bff] to-[#7a5cff] bg-clip-text text-transparent">
							accelerate AI
						</span>
					</h1>
					<p className="mt-4 text-base text-slate-500 max-w-xl leading-relaxed">
						In-flight protection for employee AI sessions, and a drop-in gateway for production LLM APIs. One control plane for both.
					</p>

					<div className="mt-6 flex flex-col sm:flex-row items-start sm:items-center gap-3">
						<button
							onClick={() => setIsDemoModalOpen(true)}
							className="w-full sm:w-auto h-12 px-7 bg-[#2f6bff] hover:bg-[#2457d6] text-white font-bold rounded-full text-sm transition-all shadow-[0_10px_24px_rgba(47,107,255,0.28)] flex items-center justify-center gap-2 cursor-pointer"
						>
							Schedule Demo
							<ArrowRight className="h-4 w-4" />
						</button>
						{isLoggedIn ? (
							<Button
								onClick={() => navigate({ to: "/workspace" })}
								className="w-full sm:w-auto h-12 px-7 bg-white hover:bg-[#f4f7ff] text-[#0b1b3a] border border-[#d7e0f0] font-bold rounded-full text-sm shadow-sm"
							>
								Open Control Plane
							</Button>
						) : (
							<Button
								onClick={() => navigate({ to: "/signup" })}
								className="w-full sm:w-auto h-12 px-7 bg-white hover:bg-[#f4f7ff] text-[#0b1b3a] border border-[#d7e0f0] font-bold rounded-full text-sm shadow-sm"
							>
								Start Free Trial
							</Button>
						)}
					</div>
					<div className="mt-5 flex flex-wrap gap-x-4 gap-y-2 text-xs font-medium text-slate-600">
						<span className="inline-flex items-center gap-1.5"><Check className="h-3.5 w-3.5 text-emerald-600" /> Browser and desktop apps</span>
						<span className="inline-flex items-center gap-1.5"><Check className="h-3.5 w-3.5 text-emerald-600" /> One virtual key</span>
						<span className="inline-flex items-center gap-1.5"><Check className="h-3.5 w-3.5 text-emerald-600" /> Under 0.8 ms overhead</span>
					</div>
					<div className="mt-4 flex flex-wrap items-center gap-2">
						{supportedModels.map((m, idx) => (
							<div key={idx} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full border border-[#e6edf8] bg-white text-xs font-semibold text-slate-600">
								<span className={`w-1.5 h-1.5 rounded-full ${m.color}`} />
								<span>{m.name}</span>
							</div>
						))}
					</div>
					</div>

					<div className="relative w-full">
					<div className="relative border border-[#e6edf8] rounded-[28px] bg-white p-4 sm:p-5 text-left shadow-[0_24px_60px_rgba(47,107,255,0.10)]">
						<div className="flex items-center justify-between pb-3 border-b border-[#e8eef8]">
							<div className="flex items-center gap-3">
								<div className="flex items-center gap-1.5">
									<div className="h-3 w-3 rounded-full bg-red-400" />
									<div className="h-3 w-3 rounded-full bg-amber-400" />
									<div className="h-3 w-3 rounded-full bg-emerald-400" />
								</div>
								<span className="text-xs font-semibold text-slate-500 ml-2">
									Control plane
								</span>
							</div>
							<div className="flex items-center gap-3 text-xs font-mono">
								<span className="hidden sm:inline text-slate-400">Node: v2.4-edge</span>
								<span className="flex items-center gap-1.5 text-emerald-600 font-semibold">
									<span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
									Live
								</span>
							</div>
						</div>

						<div className="grid grid-cols-4 gap-2 py-3 border-b border-[#e8eef8] text-xs">
							<div className="p-2.5 rounded-xl bg-[#f4f7ff] border border-[#e3ebf8]">
								<div className="text-slate-500 text-[10px] font-bold tracking-wide">OVERHEAD</div>
								<div className="text-emerald-600 font-extrabold text-base mt-0.5">&lt; 0.8 ms</div>
							</div>
							<div className="p-2.5 rounded-xl bg-[#f4f7ff] border border-[#e3ebf8]">
								<div className="text-slate-500 text-[10px] font-bold tracking-wide">CACHE</div>
								<div className="text-[#2f6bff] font-extrabold text-base mt-0.5">86.4%</div>
							</div>
							<div className="p-2.5 rounded-xl bg-[#f4f7ff] border border-[#e3ebf8]">
								<div className="text-slate-500 text-[10px] font-bold tracking-wide">BLOCKED</div>
								<div className="text-amber-600 font-extrabold text-base mt-0.5">14,290</div>
							</div>
							<div className="p-2.5 rounded-xl bg-[#f4f7ff] border border-[#e3ebf8]">
								<div className="text-slate-500 text-[10px] font-bold tracking-wide">SDK</div>
								<div className="text-indigo-600 font-extrabold text-base mt-0.5">100%</div>
							</div>
						</div>

						{/* Two Connected Core Engines */}
						<div className="grid grid-cols-1 gap-3 pt-3">
							<div className="border border-[#d7e4ff] rounded-2xl bg-[#f7f9ff] px-4 py-3">
								<div className="flex items-center justify-between gap-3">
									<div className="flex items-center gap-3 min-w-0">
										<div className="h-9 w-9 shrink-0 rounded-xl bg-[#2f6bff] flex items-center justify-center text-white">
											<Shield className="h-4 w-4" />
										</div>
										<div className="min-w-0">
											<h3 className="text-sm font-bold text-[#0b1b3a]">Browser Guard</h3>
											<p className="text-xs text-slate-500 truncate">ChatGPT, Claude, Grok, Gemini · PII and file scan</p>
										</div>
									</div>
									<span className="text-[10px] font-mono px-2.5 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 font-bold shrink-0">
										ACTIVE
									</span>
								</div>
							</div>

							<div className="border border-[#e4e0ff] rounded-2xl bg-[#f8f7ff] px-4 py-3">
								<div className="flex items-center justify-between gap-3">
									<div className="flex items-center gap-3 min-w-0">
										<div className="h-9 w-9 shrink-0 rounded-xl bg-[#6d5ef7] flex items-center justify-center text-white">
											<Zap className="h-4 w-4" />
										</div>
										<div className="min-w-0">
											<h3 className="text-sm font-bold text-[#0b1b3a]">Universal Gateway</h3>
											<p className="text-xs text-slate-500 truncate">100+ models · cache · virtual keys</p>
										</div>
									</div>
									<span className="text-[10px] font-mono px-2.5 py-0.5 rounded-full bg-blue-50 text-[#2f6bff] border border-blue-200 font-bold shrink-0">
										CONNECTED
									</span>
								</div>
							</div>
						</div>
					</div>
					</div>
					</div>
				</section>

				{/* TRUST & COMPLIANCE METRICS STRIP */}
				<section className="px-6 lg:px-10 xl:px-14 py-3">
					<div className="max-w-[1440px] mx-auto rounded-[28px] border border-[#e6edf8] bg-white shadow-[0_12px_40px_rgba(15,23,42,0.04)] py-6 px-4 sm:px-8">
					<div className="grid grid-cols-2 md:grid-cols-4 gap-6 md:gap-0 md:divide-x md:divide-[#e8eef8] text-center">
						<div className="space-y-1 px-3">
							<div className="text-3xl sm:text-4xl font-extrabold text-[#2f6bff] tracking-tight">50M+</div>
							<div className="text-xs text-slate-500 font-medium">Prompts protected</div>
						</div>
						<div className="space-y-1 px-3">
							<div className="text-3xl sm:text-4xl font-extrabold text-[#0b1b3a] tracking-tight">&lt; 0.8ms</div>
							<div className="text-xs text-slate-500 font-medium">Inspection overhead</div>
						</div>
						<div className="space-y-1 px-3">
							<div className="text-3xl sm:text-4xl font-extrabold text-[#0b1b3a] tracking-tight">85%</div>
							<div className="text-xs text-slate-500 font-medium">API spend saved</div>
						</div>
						<div className="space-y-1 px-3">
							<div className="text-3xl sm:text-4xl font-extrabold text-[#0b1b3a] tracking-tight">99.99%</div>
							<div className="text-xs text-slate-500 font-medium">Enterprise availability</div>
						</div>
					</div>

					<div className="max-w-4xl mx-auto mt-8 pt-6 border-t border-[#e8eef8] flex flex-wrap items-center justify-center gap-3 text-xs text-slate-600 font-medium">
						<span className="flex items-center gap-1.5 rounded-full bg-[#f4f7ff] border border-[#e3ebf8] px-3 py-1.5"><CheckCircle className="h-4 w-4 text-emerald-500" /> SOC 2 Type II Certified</span>
						<span className="flex items-center gap-1.5 rounded-full bg-[#f4f7ff] border border-[#e3ebf8] px-3 py-1.5"><CheckCircle className="h-4 w-4 text-emerald-500" /> HIPAA & GDPR Compliant</span>
						<span className="flex items-center gap-1.5 rounded-full bg-[#f4f7ff] border border-[#e3ebf8] px-3 py-1.5"><CheckCircle className="h-4 w-4 text-emerald-500" /> Zero-Trust Architecture</span>
						<span className="flex items-center gap-1.5 rounded-full bg-[#f4f7ff] border border-[#e3ebf8] px-3 py-1.5"><CheckCircle className="h-4 w-4 text-emerald-500" /> ISO/IEC 27001 Aligned</span>
					</div>
					</div>
				</section>

				{/* PRODUCTS SECTION (TWO PILLARS) */}
				<section id="products" className="flex min-h-[calc(100dvh-4rem)] items-start bg-white/70 px-6 py-12 lg:px-10 xl:px-14 scroll-mt-16">
					<div className="mx-auto w-full max-w-[1440px]">
					<div className="text-center max-w-2xl mx-auto mb-8">
						<div className="text-[11px] font-bold uppercase tracking-[0.16em] text-[#2f6bff] mb-3">Platform</div>
						<h2 className="text-3xl sm:text-5xl font-extrabold tracking-[-0.03em] text-[#0b1b3a] mb-4">
							Two products. One architecture.
						</h2>
						<p className="text-slate-600 text-sm sm:text-base leading-relaxed">
							Arm enterprise security teams with real-time DLP over consumer web AI, while giving developers a blisteringly fast LLM gateway with strict budget governance.
						</p>
					</div>

					<div className="grid grid-cols-1 md:grid-cols-2 gap-8">
						{/* Product 1: Browser Guard */}
						<div className="group border border-[#e3ebf8] hover:border-[#2f6bff]/40 rounded-3xl bg-white p-8 space-y-6 transition-all duration-300 shadow-[0_18px_50px_rgba(47,107,255,0.08)] hover:-translate-y-1">
							<div className="h-12 w-12 rounded-2xl bg-[#2f6bff] flex items-center justify-center text-white group-hover:scale-105 transition-transform shadow-[0_10px_20px_rgba(47,107,255,0.25)]">
								<Shield className="h-6 w-6" />
							</div>

							<div className="space-y-2">
								<h3 className="text-2xl font-bold text-[#0b1b3a]">GATEWAY Browser Guard</h3>
								<p className="text-slate-600 text-sm leading-relaxed">
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
									<div key={idx} className="flex items-center gap-2.5 text-xs text-slate-700 font-medium">
										<CheckCircle2 className="h-4 w-4 text-[#2f6bff] shrink-0" />
										<span>{item}</span>
									</div>
								))}
							</div>

							<div className="pt-2">
								<button
									onClick={() => setIsDemoModalOpen(true)}
									className="text-xs font-bold text-[#2f6bff] hover:text-[#2457d6] flex items-center gap-1.5 cursor-pointer"
								>
									Explore Browser Guard Specs <ArrowRight className="h-3.5 w-3.5" />
								</button>
							</div>
						</div>

						{/* Product 2: AI Gateway */}
						<div className="group border border-[#e4e0ff] hover:border-[#6d5ef7]/40 rounded-3xl bg-white p-8 space-y-6 transition-all duration-300 shadow-[0_18px_50px_rgba(109,94,247,0.08)] hover:-translate-y-1">
							<div className="h-12 w-12 rounded-2xl bg-[#6d5ef7] flex items-center justify-center text-white group-hover:scale-105 transition-transform shadow-[0_10px_20px_rgba(109,94,247,0.25)]">
								<Zap className="h-6 w-6" />
							</div>

							<div className="space-y-2">
								<h3 className="text-2xl font-bold text-[#0b1b3a]">GATEWAY Universal Gateway</h3>
								<p className="text-slate-600 text-sm leading-relaxed">
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
									<div key={idx} className="flex items-center gap-2.5 text-xs text-slate-700 font-medium">
										<CheckCircle2 className="h-4 w-4 text-[#6d5ef7] shrink-0" />
										<span>{item}</span>
									</div>
								))}
							</div>

							<div className="pt-2">
								<button
									onClick={() => setIsDemoModalOpen(true)}
									className="text-xs font-bold text-[#6d5ef7] hover:text-[#5648d6] flex items-center gap-1.5 cursor-pointer"
								>
									Explore Gateway Specs <ArrowRight className="h-3.5 w-3.5" />
								</button>
							</div>
						</div>
					</div>
					</div>
				</section>

				{/* CAPABILITIES SECTION */}
				<section id="features" className="flex min-h-[calc(100dvh-4rem)] items-start px-6 py-12 lg:px-10 xl:px-14 scroll-mt-16">
					<div className="mx-auto w-full max-w-[1440px]">
						<div className="text-center max-w-2xl mx-auto mb-6">
							<div className="text-[11px] font-bold uppercase tracking-[0.16em] text-[#2f6bff] mb-2">Capabilities</div>
							<h2 className="text-3xl sm:text-4xl font-extrabold tracking-[-0.03em] text-[#0b1b3a] mb-3">
								Enterprise grade from day one
							</h2>
							<p className="text-slate-600 text-sm sm:text-base">
								Everything required to satisfy stringent enterprise InfoSec, compliance, and developer productivity demands.
							</p>
						</div>

						<div className="grid grid-cols-1 md:grid-cols-3 gap-4">
							{[
								{
									icon: <Lock className="h-5 w-5 text-[#2f6bff]" />,
									title: "In-Flight Data Redaction",
									desc: "API credentials, customer PII, and sensitive source code are sanitized before requests ever leave your perimeter."
								},
								{
									icon: <Layers className="h-5 w-5 text-[#2f6bff]" />,
									title: "Semantic Vector Caching",
									desc: "High-performance vector embeddings catch semantically identical questions, answering in < 10ms at 0 token cost."
								},
								{
									icon: <Key className="h-5 w-5 text-[#2f6bff]" />,
									title: "Virtual Keys & Quotas",
									desc: "Provision scoped virtual API keys for teams and microservices with hard spend limits and budget double-locks."
								},
								{
									icon: <Activity className="h-5 w-5 text-[#2f6bff]" />,
									title: "Audit Logs & Tracing",
									desc: "Full observability into prompt metadata, token usage, latency metrics, and policy trigger events across your company."
								},
								{
									icon: <Laptop className="h-5 w-5 text-[#2f6bff]" />,
									title: "Cross-Platform Agents",
									desc: "Lightweight, silent installers for macOS and Windows, manageable through existing MDM fleets."
								},
								{
									icon: <Cpu className="h-5 w-5 text-[#2f6bff]" />,
									title: "Automatic Model Fallbacks",
									desc: "Configure automated multi-model failover chains so applications remain resilient during third-party AI outages."
								}
							].map((feature, idx) => (
								<div
									key={idx}
									className="group border border-[#e3ebf8] hover:border-[#2f6bff]/30 rounded-2xl bg-white p-4 space-y-2 transition-all shadow-[0_12px_30px_rgba(47,107,255,0.06)] hover:-translate-y-0.5"
								>
									<div className="h-11 w-11 rounded-xl bg-[#eef3ff] border border-[#d7e4ff] flex items-center justify-center group-hover:scale-105 transition-transform">
										{feature.icon}
									</div>
									<h3 className="text-base font-bold text-[#0b1b3a]">{feature.title}</h3>
									<p className="text-xs text-slate-600 leading-relaxed font-normal">{feature.desc}</p>
								</div>
							))}
						</div>
					</div>
				</section>

				{/* DEVELOPER QUICKSTART CODE SECTION */}
				<section id="quickstart" className="flex min-h-[calc(100dvh-4rem)] items-start bg-white/70 px-6 py-12 lg:px-10 xl:px-14 scroll-mt-16">
					<div className="mx-auto w-full max-w-[1440px]">
					<div id="developers" className="scroll-mt-24" />
					<div id="code-preview" className="scroll-mt-24" />
					<div className="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
						<div className="space-y-6">
							<div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full border border-[#d7d2ff] bg-white text-xs font-semibold text-[#6d5ef7] shadow-sm">
								<Terminal className="w-3.5 h-3.5" />
								<span>Zero SDK Refactoring</span>
							</div>

							<h2 className="text-3xl sm:text-5xl font-extrabold tracking-[-0.03em] text-[#0b1b3a] leading-tight">
								One line of code to route every model
							</h2>
							<p className="text-slate-600 text-sm sm:text-base leading-relaxed">
								GATEWAY is 100% compliant with standard OpenAI SDKs and specs. Simply update your connection base URL to instantly unlock semantic caching, virtual key quotas, and automatic multi-model failover.
							</p>

							<div className="space-y-3 pt-2">
								{[
									"Zero SDK changes or refactoring necessary",
									"Compatible with Python, Node.js, Go, LangChain, and cURL",
									"Seamless fallback between OpenAI, Anthropic, Gemini, and DeepSeek"
								].map((item, idx) => (
									<div key={idx} className="flex items-center gap-2 text-xs text-slate-700 font-medium">
										<Check className="h-4 w-4 text-emerald-600 shrink-0" />
										<span>{item}</span>
									</div>
								))}
							</div>
						</div>

						{/* Terminal Code Box with macOS controls */}
						<div className="border border-[#1e293b] rounded-2xl bg-[#0f172a] overflow-hidden shadow-[0_24px_50px_rgba(15,23,42,0.18)]">
							<div className="flex bg-[#111c33] border-b border-white/10 px-4 py-3 justify-between items-center">
								<div className="flex items-center gap-2">
									<div className="flex gap-1.5 mr-2">
										<div className="w-3 h-3 rounded-full bg-red-500/80" />
										<div className="w-3 h-3 rounded-full bg-amber-500/80" />
										<div className="w-3 h-3 rounded-full bg-emerald-500/80" />
									</div>

									<button
										onClick={() => setCodeTab("python")}
										className={`text-xs font-bold px-3 py-1 rounded-lg transition-colors cursor-pointer ${
											codeTab === "python" ? "bg-white/[0.15] text-white shadow-sm" : "text-slate-400 hover:text-white"
										}`}
									>
										Python
									</button>
									<button
										onClick={() => setCodeTab("node")}
										className={`text-xs font-bold px-3 py-1 rounded-lg transition-colors cursor-pointer ${
											codeTab === "node" ? "bg-white/[0.15] text-white shadow-sm" : "text-slate-400 hover:text-white"
										}`}
									>
										Node.js
									</button>
									<button
										onClick={() => setCodeTab("curl")}
										className={`text-xs font-bold px-3 py-1 rounded-lg transition-colors cursor-pointer ${
											codeTab === "curl" ? "bg-white/[0.15] text-white shadow-sm" : "text-slate-400 hover:text-white"
										}`}
									>
										cURL
									</button>
								</div>

								<button
									onClick={() => handleCopyCode(codeExamples[codeTab])}
									className="text-xs text-slate-300 hover:text-white flex items-center gap-1.5 font-mono px-2.5 py-1 rounded-lg hover:bg-white/[0.08] transition-colors cursor-pointer"
								>
									{copiedCode ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
									<span>{copiedCode ? "Copied!" : "Copy"}</span>
								</button>
							</div>

							<div className="p-5 overflow-x-auto text-xs font-mono leading-relaxed bg-[#050609] text-slate-300">
								<pre className="whitespace-pre">{codeExamples[codeTab]}</pre>
							</div>
						</div>
					</div>
					</div>
				</section>

				<section id="models" className="flex min-h-[calc(100dvh-4rem)] items-start px-6 py-12 lg:px-10 xl:px-14 scroll-mt-16">
					<div className="mx-auto w-full max-w-[1440px]">
						<div className="text-center max-w-2xl mx-auto mb-6">
							<div className="text-[11px] font-bold uppercase tracking-[0.16em] text-[#2f6bff] mb-3">Models</div>
							<h2 className="text-3xl sm:text-5xl font-extrabold tracking-[-0.03em] text-[#0b1b3a] mb-4">
								One gateway. Every model.
							</h2>
							<p className="text-slate-600 text-sm sm:text-base">
								The same API key reaches each provider. No separate console for each model.
							</p>
						</div>
						<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
							{supportedModels.map((m) => (
								<div key={m.name} className="rounded-2xl border border-[#e6edf8] bg-white p-5 shadow-[0_12px_30px_rgba(47,107,255,0.06)]">
									<div className="mb-3 flex items-center gap-2">
										<span className={`h-2.5 w-2.5 rounded-full ${m.color}`} />
										<h3 className="text-base font-bold text-[#0b1b3a]">{m.name}</h3>
									</div>
									<p className="text-sm leading-relaxed text-slate-600">{m.note}</p>
								</div>
							))}
						</div>
						<div className="mt-6 grid grid-cols-1 gap-4 md:grid-cols-3">
							{[
								{ title: "One virtual key", text: "Every provider is reached with the same key. Teams do not keep a separate console for each model." },
								{ title: "Automatic failover", text: "If a model is unavailable, the gateway moves the request to the next model in the chain." },
								{ title: "Same SDK", text: "Keep the OpenAI client. Change only the base URL to send traffic through the gateway." },
							].map((item) => (
								<div key={item.title} className="rounded-2xl border border-[#e3ebf8] bg-white px-5 py-4">
									<h3 className="text-sm font-bold text-[#0b1b3a]">{item.title}</h3>
									<p className="mt-1.5 text-sm leading-relaxed text-slate-600">{item.text}</p>
								</div>
							))}
						</div>
					</div>
				</section>

				{/* FAQ SECTION */}
				<section id="faq" className="flex min-h-[calc(100dvh-4rem)] items-start bg-white/70 px-6 py-12 lg:px-10 xl:px-14 scroll-mt-16">
					<div className="mx-auto w-full max-w-[1440px]">
						<div className="text-center mb-12">
							<div className="text-[11px] font-bold uppercase tracking-[0.16em] text-[#2f6bff] mb-3">FAQ</div>
							<h2 className="text-3xl sm:text-5xl font-extrabold tracking-[-0.03em] text-[#0b1b3a] mb-3">
								Questions teams ask first
							</h2>
							<p className="text-slate-600 text-sm">
								Answers to common enterprise security, performance, and deployment inquiries.
							</p>
						</div>

						<div className="grid grid-cols-1 items-start gap-3.5 lg:grid-cols-2">
							{[0, 1].map((column) => (
								<div key={column} className="flex flex-col gap-3.5">
									{faqs.map((faq, idx) => idx % 2 === column ? (
										<div
											key={idx}
											className="border border-[#e3ebf8] rounded-2xl bg-white overflow-hidden shadow-[0_10px_24px_rgba(47,107,255,0.05)]"
										>
											<button
												onClick={() => setOpenFaq(openFaq === idx ? null : idx)}
												className="w-full p-5 text-left font-bold text-sm text-[#0b1b3a] flex items-center justify-between gap-4 hover:text-[#2f6bff] transition-colors cursor-pointer"
											>
												<span>{faq.q}</span>
												<ChevronDown className={`h-4 w-4 text-[#2f6bff] shrink-0 transition-transform duration-200 ${openFaq === idx ? "rotate-180" : ""}`} />
											</button>
											{openFaq === idx && (
												<div className="px-5 pb-5 text-xs sm:text-sm text-slate-600 leading-relaxed border-t border-[#e8eef8] pt-3.5">
													{faq.a}
												</div>
											)}
										</div>
									) : null)}
								</div>
							))}
						</div>
					</div>
				</section>

				{/* BOTTOM HIGH-CONVERTING CTA BANNER */}
				<section className="py-16 px-6 lg:px-10 xl:px-14">
					<div className="max-w-[1440px] mx-auto text-center">
					<div className="p-8 sm:p-14 rounded-3xl bg-gradient-to-br from-[#2f6bff] via-[#4d6bff] to-[#6d5ef7] shadow-[0_24px_60px_rgba(47,107,255,0.28)] relative overflow-hidden">
						<div className="absolute -top-16 right-0 w-64 h-64 rounded-full bg-white/15 blur-2xl pointer-events-none" />
						
						<h2 className="text-3xl sm:text-5xl font-extrabold tracking-[-0.03em] text-white mb-4 relative z-10">
							Ready to standardize your AI perimeter?
						</h2>
						<p className="text-blue-50 text-sm sm:text-base max-w-xl mx-auto mb-8 relative z-10 leading-relaxed">
							Connect with our enterprise security team for a customized deployment plan or start immediately in our cloud sandbox.
						</p>
						<div className="flex flex-col sm:flex-row items-center justify-center gap-4 relative z-10">
							<button
								onClick={() => setIsDemoModalOpen(true)}
								className="w-full sm:w-auto h-12 px-8 bg-white hover:bg-blue-50 text-[#2f6bff] font-extrabold rounded-xl text-sm transition-all shadow-sm flex items-center justify-center gap-2 cursor-pointer"
							>
								<Calendar className="h-4 w-4" />
								Schedule a Demo
							</button>
							{isLoggedIn ? (
								<Button
									onClick={() => navigate({ to: "/workspace" })}
									className="w-full sm:w-auto h-12 px-8 bg-white/15 hover:bg-white/25 text-white border border-white/30 font-bold rounded-xl text-sm shadow-sm cursor-pointer"
								>
									Go to Workspace
								</Button>
							) : (
								<Button
									onClick={() => navigate({ to: "/signup" })}
									className="w-full sm:w-auto h-12 px-8 bg-white/15 hover:bg-white/25 text-white border border-white/30 font-bold rounded-xl text-sm shadow-sm cursor-pointer"
								>
									Get Started Free
								</Button>
							)}
						</div>
					</div>
					</div>
				</section>
			</main>

			{/* HIGH-END FOOTER - PERFECT SINGLE-LINE ALIGNMENT */}
			<footer className="border-t border-[#e3ebf8] bg-white py-8 px-6 w-full">
				<div className="max-w-[1400px] mx-auto flex flex-col lg:flex-row items-center justify-between gap-6 text-xs text-slate-500">
					{/* Brand Logo & Company */}
					<div className="flex items-center gap-3 shrink-0">
						<img src={companyLogoSrc} alt={companyFullName} className="h-6 w-auto object-contain opacity-90 shrink-0" />
						<span className="font-bold text-[#0b1b3a] whitespace-nowrap">{companyFullName}</span>
					</div>

					{/* Navigation Links - Perfect single line, never wrap or split words */}
					<nav aria-label="Footer Navigation" className="flex items-center gap-6 sm:gap-8 whitespace-nowrap shrink-0">
						<button type="button" onClick={() => scrollToSection("products")} className="hover:text-[#2f6bff] transition-colors cursor-pointer whitespace-nowrap">Products</button>
						<button type="button" onClick={() => scrollToSection("features")} className="hover:text-[#2f6bff] transition-colors cursor-pointer whitespace-nowrap">Capabilities</button>
						<button type="button" onClick={() => setIsDemoModalOpen(true)} className="hover:text-[#2f6bff] transition-colors cursor-pointer whitespace-nowrap">Book Demo</button>
						<Link to="/login" className="hover:text-[#2f6bff] transition-colors cursor-pointer whitespace-nowrap">Sign In</Link>
						<Link to="/signup" className="hover:text-[#2f6bff] transition-colors cursor-pointer whitespace-nowrap">Sign Up</Link>
					</nav>

					{/* Copyright Text */}
					<div className="text-slate-500 text-[11px] sm:text-xs text-center lg:text-right shrink-0 whitespace-nowrap">
						{branding.footerText}
					</div>
				</div>
			</footer>

			{/* BOOK A DEMO MODAL */}
			<Dialog open={isDemoModalOpen} onOpenChange={setIsDemoModalOpen}>
				<DialogContent className="bg-white border border-[#e3ebf8] text-slate-700 sm:max-w-lg p-6 shadow-2xl rounded-2xl">
					<DialogHeader>
						<DialogTitle className="text-lg font-bold text-[#0b1b3a] flex items-center gap-2">
							<Calendar className="h-4 w-4 text-[#2f6bff]" />
							Schedule an Enterprise Demo
						</DialogTitle>
						<DialogDescription className="text-slate-500 text-xs">
							Our engineering team will provide a tailored 20-minute architecture demonstration of Browser Guard and the Universal AI Gateway.
						</DialogDescription>
					</DialogHeader>

					{demoSubmitted ? (
						<div className="py-8 text-center space-y-3">
							<div className="h-12 w-12 rounded-full bg-emerald-500/15 border border-emerald-500/30 flex items-center justify-center text-emerald-400 mx-auto">
								<CheckCircle2 className="h-6 w-6" />
							</div>
							<h3 className="text-base font-bold text-[#0b1b3a]">Demo Request Received</h3>
							<p className="text-xs text-slate-600 max-w-sm mx-auto leading-relaxed">
								Thank you, <strong className="text-[#0b1b3a]">{demoForm.fullName}</strong>. A technical architect has sent a calendar invitation to <strong className="text-[#0b1b3a]">{demoForm.email}</strong>.
							</p>
							<div className="pt-2">
								<Button
									onClick={resetDemoModal}
									className="bg-[#2f6bff] text-white font-bold text-xs h-8 px-5 rounded-lg cursor-pointer"
								>
									Done
								</Button>
							</div>
						</div>
					) : (
						<form onSubmit={handleDemoSubmit} className="space-y-4 mt-2">
							{demoError && (
								<div className="rounded-lg border border-rose-500/30 bg-rose-500/10 p-2.5 text-xs text-rose-300">
									{demoError}
								</div>
							)}
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div className="space-y-1">
									<label className="text-xs font-semibold text-slate-700">Full Name *</label>
									<Input
										required
										placeholder="Jane Doe"
										value={demoForm.fullName}
										onChange={(e) => setDemoForm({ ...demoForm, fullName: e.target.value })}
										className="bg-[#f7f9ff] border-[#e3ebf8] text-[#0b1b3a] text-xs h-9 focus:border-[#2f6bff] rounded-lg"
									/>
								</div>

								<div className="space-y-1">
									<label className="text-xs font-semibold text-slate-700">Work Email *</label>
									<Input
										type="email"
										required
										placeholder="jane@company.com"
										value={demoForm.email}
										onChange={(e) => setDemoForm({ ...demoForm, email: e.target.value })}
										className="bg-[#f7f9ff] border-[#e3ebf8] text-[#0b1b3a] text-xs h-9 focus:border-[#2f6bff] rounded-lg"
									/>
								</div>
							</div>

							<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div className="space-y-1">
									<label className="text-xs font-semibold text-slate-700">Company</label>
									<Input
										placeholder="Acme Corp"
										value={demoForm.company}
										onChange={(e) => setDemoForm({ ...demoForm, company: e.target.value })}
										className="bg-[#f7f9ff] border-[#e3ebf8] text-[#0b1b3a] text-xs h-9 focus:border-[#2f6bff] rounded-lg"
									/>
								</div>

								<div className="space-y-1">
									<label className="text-xs font-semibold text-slate-700">Team Size</label>
									<select
										value={demoForm.teamSize}
										onChange={(e) => setDemoForm({ ...demoForm, teamSize: e.target.value })}
										className="w-full bg-[#f7f9ff] border border-[#e3ebf8] text-[#0b1b3a] text-xs h-9 rounded-lg px-2.5 focus:outline-none focus:border-[#2f6bff]"
									>
										<option value="1-50">1 - 50 employees</option>
										<option value="50-250">50 - 250 employees</option>
										<option value="250-1000">250 - 1,000 employees</option>
										<option value="1000+">1,000+ employees</option>
									</select>
								</div>
							</div>

							<div className="space-y-1">
								<label className="text-xs font-semibold text-slate-700">Primary Focus</label>
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
													? "border-[#2f6bff] bg-[#eef3ff] text-[#2f6bff] font-bold"
													: "border-[#e3ebf8] bg-white text-slate-500 hover:text-[#0b1b3a]"
											}`}
										>
											{opt.label}
										</button>
									))}
								</div>
							</div>

							<div className="space-y-1">
								<label className="text-xs font-semibold text-slate-700">Architecture Requirements (Optional)</label>
								<textarea
									rows={2}
									placeholder="Tell us about your current AI models, team size, or security needs..."
									value={demoForm.notes}
									onChange={(e) => setDemoForm({ ...demoForm, notes: e.target.value })}
									className="w-full bg-[#f7f9ff] border border-[#e3ebf8] rounded-lg p-2.5 text-[#0b1b3a] text-xs focus:outline-none focus:border-[#2f6bff] resize-none"
								/>
							</div>

							<div className="pt-2 flex items-center justify-end gap-2">
								<Button
									type="button"
									variant="ghost"
									disabled={isSubmittingDemo}
									onClick={() => setIsDemoModalOpen(false)}
									className="text-xs h-8 text-slate-500 hover:text-[#0b1b3a] cursor-pointer"
								>
									Cancel
								</Button>
								<Button
									type="submit"
									disabled={isSubmittingDemo}
									className="bg-[#2f6bff] hover:bg-[#2457d6] text-white font-bold text-xs h-8 px-5 rounded-lg shadow-sm cursor-pointer disabled:opacity-50"
								>
									{isSubmittingDemo ? (
										<span className="flex items-center gap-1.5">
											<Loader2 className="h-3.5 w-3.5 animate-spin" />
											Scheduling...
										</span>
									) : (
										"Submit Request"
									)}
								</Button>
							</div>
						</form>
					)}
				</DialogContent>
			</Dialog>
		</div>
	);
}
