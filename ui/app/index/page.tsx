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
	EyeOff,
	Sparkles
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
		python: `from openai import OpenAI

# Route seamlessly through UnifAI (1-line change)
client = OpenAI(
    base_url="https://unifai.yourcompany.com/v1",
    api_key="unifai_vk_live_enterprise"
)

response = client.chat.completions.create(
    model="claude-3-7-sonnet", # or gpt-4o, grok-2, gemini-2.0-flash
    messages=[{"role": "user", "content": "Analyze quarterly performance report"}]
)

print(response.choices[0].message.content)`,
		curl: `curl -X POST https://unifai.yourcompany.com/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer unifai_vk_live_enterprise" \\
  -d '{
    "model": "gpt-4o",
    "messages": [
      {
        "role": "user",
        "content": "Evaluate our security guardrails and semantic caching"
      }
    ]
  }'`,
		node: `import OpenAI from "openai";

// Drop-in compatible with standard OpenAI SDK
const openai = new OpenAI({
  baseURL: "https://unifai.yourcompany.com/v1",
  apiKey: "unifai_vk_live_enterprise"
});

const response = await openai.chat.completions.create({
  model: "claude-3-7-sonnet",
  messages: [{ role: "user", content: "Optimize our multi-model LLM architecture" }]
});

console.log(response.choices[0].message.content);`
	};

	const faqs = [
		{
			q: "How does Browser Guard protect corporate data in web AI tools?",
			a: "UnifAI Browser Guard runs as a lightweight background agent on macOS and Windows. When employees use web AI tools like ChatGPT, Claude, or Grok, Browser Guard intercepts prompts and attached documents in-flight, automatically redacting credentials, customer PII, and sensitive source code before the request leaves the workstation."
		},
		{
			q: "Does Browser Guard affect employee internet browsing speed?",
			a: "No. Browser Guard operates via local loopback routing and only processes traffic destined for designated AI target websites. Interception overhead is under 1.2 milliseconds, and standard web browsing remains entirely untouched at full native network speeds."
		},
		{
			q: "How does the AI Gateway cut API costs with Semantic Caching?",
			a: "The gateway maintains a vectorized semantic cache. When an incoming query is semantically equivalent to a previously answered request, UnifAI immediately delivers the cached response with zero model token charges and near-zero latency, reducing vendor API spend by up to 85%."
		},
		{
			q: "Can UnifAI be deployed on-premise or in a private cloud VPC?",
			a: "Yes. UnifAI is distributed as a self-contained Go binary with standard PostgreSQL storage. It can run in air-gapped environments, Kubernetes clusters, or private AWS/Azure VPCs so that corporate data never touches external infrastructure."
		},
		{
			q: "How do we roll out the desktop agent to employee computers?",
			a: "UnifAI provides pre-packaged silent installers for Windows (MSI/EXE) and macOS (PKG/ZIP) designed for automated deployment via Microsoft Intune, Jamf Pro, or Active Directory GPO."
		}
	];

	const companyLogoSrc = COMPANY_LOGO;
	const productName = "UnifAI";
	const companyFullName = COMPANY_NAME;

	return (
		<div className="bg-[#090a0f] text-[#cbd5e1] min-h-screen font-sans selection:bg-sky-500/20 selection:text-white antialiased overflow-x-hidden no-scrollbar">
			{/* Subtle Ambient Glow */}
			<div className="fixed inset-0 pointer-events-none overflow-hidden z-0">
				<div className="absolute -top-[30%] left-1/2 -translate-x-1/2 w-[1100px] h-[600px] bg-gradient-to-b from-sky-500/[0.07] via-indigo-500/[0.04] to-transparent rounded-full blur-[140px]" />
			</div>

			{/* Navigation */}
			<header className="sticky top-0 z-50 border-b border-white/[0.07] bg-[#090a0f]/80 backdrop-blur-xl">
				<div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
					{/* Brand Logo */}
					<a href="/" className="flex items-center gap-3 group" aria-label={productName}>
						<img
							src={companyLogoSrc}
							alt={companyFullName}
							className="h-8 w-auto max-w-[160px] object-contain shrink-0"
							onError={(e) => {
								const target = e.currentTarget;
								if (!target.src.endsWith("/yes-panchi-logo.png")) {
									target.src = "/yes-panchi-logo.png";
								}
							}}
						/>
						<span className="text-lg font-semibold tracking-tight text-white group-hover:text-sky-400 transition-colors">
							{productName}
						</span>
					</a>

					{/* Navigation Links */}
					<nav className="hidden md:flex items-center gap-8 text-sm font-medium text-slate-300">
						<a href="#products" className="hover:text-white transition-colors">Products</a>
						<a href="#features" className="hover:text-white transition-colors">Capabilities</a>
						<a href="#quickstart" className="hover:text-white transition-colors">Quickstart</a>
						<a href="#faq" className="hover:text-white transition-colors">FAQ</a>
					</nav>

					{/* Actions */}
					<div className="hidden md:flex items-center gap-3">
						<button
							onClick={() => setIsDemoModalOpen(true)}
							className="text-xs font-semibold text-slate-300 hover:text-white border border-white/[0.12] hover:border-white/[0.25] bg-white/[0.03] hover:bg-white/[0.08] px-3.5 py-1.5 rounded-lg transition-all cursor-pointer"
						>
							Book a Demo
						</button>

						{isLoggedIn ? (
							<Button
								onClick={() => navigate({ to: "/workspace" })}
								className="bg-white hover:bg-slate-100 text-[#090a0f] font-semibold text-xs h-8 px-4 rounded-lg transition-all"
							>
								Dashboard
								<ArrowRight className="h-3.5 w-3.5 ml-1" />
							</Button>
						) : (
							<>
								<Link
									to="/login"
									className="text-xs font-semibold text-slate-300 hover:text-white px-2.5 py-1.5 transition-colors"
								>
									Sign In
								</Link>
								<Button
									onClick={() => navigate({ to: "/signup" })}
									className="bg-white hover:bg-slate-100 text-[#090a0f] font-semibold text-xs h-8 px-3.5 rounded-lg transition-all shadow-sm"
								>
									Get Started
								</Button>
							</>
						)}
					</div>

					{/* Mobile Menu Button */}
					<button
						onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
						className="md:hidden text-slate-300 hover:text-white p-1.5 rounded-lg"
						aria-label="Toggle menu"
					>
						{mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
					</button>
				</div>

				{/* Mobile Menu Dropdown */}
				{mobileMenuOpen && (
					<div className="md:hidden border-b border-white/[0.08] bg-[#090a0f]/95 px-6 py-5 space-y-3">
						<a href="#products" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">Products</a>
						<a href="#features" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">Capabilities</a>
						<a href="#quickstart" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">Quickstart</a>
						<a href="#faq" onClick={() => setMobileMenuOpen(false)} className="block py-1.5 text-sm text-slate-300 hover:text-white">FAQ</a>
						
						<div className="pt-3 border-t border-white/[0.08] flex flex-col gap-2.5">
							<button
								onClick={() => { setMobileMenuOpen(false); setIsDemoModalOpen(true); }}
								className="w-full py-2 rounded-lg border border-white/[0.15] text-slate-200 font-semibold text-xs text-center cursor-pointer"
							>
								Book a Demo
							</button>
							{isLoggedIn ? (
								<Button
									onClick={() => { setMobileMenuOpen(false); navigate({ to: "/workspace" }); }}
									className="w-full bg-white text-[#090a0f] font-semibold text-xs h-9"
								>
									Go to Workspace
								</Button>
							) : (
								<div className="grid grid-cols-2 gap-2">
									<Link
										to="/login"
										onClick={() => setMobileMenuOpen(false)}
										className="border border-white/[0.1] text-center font-semibold text-slate-200 text-xs py-2 rounded-lg"
									>
										Sign In
									</Link>
									<Button
										onClick={() => { setMobileMenuOpen(false); navigate({ to: "/signup" }); }}
										className="bg-white text-[#090a0f] font-semibold text-xs h-8 rounded-lg"
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
				<section className="pt-20 pb-20 md:pt-28 md:pb-28 px-6 max-w-5xl mx-auto text-center">
					<div className="inline-flex items-center gap-2 px-3 py-1 rounded-full border border-white/[0.1] bg-white/[0.03] text-xs font-medium text-slate-300 mb-8">
						<span className="w-1.5 h-1.5 rounded-full bg-emerald-400" />
						<span>Unified AI Security & Gateway Platform</span>
					</div>

					<h1 className="text-4xl sm:text-6xl md:text-7xl font-bold tracking-tight text-white mb-6 leading-[1.12]">
						Secure, Govern, and Accelerate <br className="hidden sm:inline" />
						<span className="bg-gradient-to-r from-slate-100 via-slate-200 to-slate-400 bg-clip-text text-transparent">
							AI Across Your Organization
						</span>
					</h1>

					<p className="text-base sm:text-lg text-slate-400 max-w-2xl mx-auto mb-10 leading-relaxed font-normal">
						UnifAI provides real-time DLP protection for employee AI browser use while delivering a unified, high-performance gateway for developer LLM APIs.
					</p>

					<div className="flex flex-col sm:flex-row items-center justify-center gap-3.5 max-w-md mx-auto">
						<button
							onClick={() => setIsDemoModalOpen(true)}
							className="w-full sm:w-auto h-11 px-6 bg-white hover:bg-slate-100 text-[#090a0f] font-semibold rounded-lg text-sm transition-all shadow-sm flex items-center justify-center gap-2 cursor-pointer"
						>
							<Calendar className="h-4 w-4 text-slate-700" />
							Book a Demo
						</button>

						{isLoggedIn ? (
							<Button
								onClick={() => navigate({ to: "/workspace" })}
								className="w-full sm:w-auto h-11 px-6 bg-white/[0.06] hover:bg-white/[0.1] text-white border border-white/[0.12] font-semibold rounded-lg text-sm transition-all"
							>
								Open Dashboard
								<ArrowRight className="h-4 w-4 ml-1.5 text-slate-400" />
							</Button>
						) : (
							<Button
								onClick={() => navigate({ to: "/signup" })}
								className="w-full sm:w-auto h-11 px-6 bg-white/[0.06] hover:bg-white/[0.1] text-white border border-white/[0.12] font-semibold rounded-lg text-sm transition-all"
							>
								Start Free Trial
								<ArrowRight className="h-4 w-4 ml-1.5 text-slate-400" />
							</Button>
						)}
					</div>

					{/* Clean Architecture Diagram Card */}
					<div className="mt-16 sm:mt-20 border border-white/[0.08] rounded-2xl bg-[#0f111a]/80 backdrop-blur-xl p-6 sm:p-8 text-left shadow-2xl">
						<div className="flex items-center justify-between pb-6 border-b border-white/[0.06]">
							<div className="flex items-center gap-3">
								<div className="h-2.5 w-2.5 rounded-full bg-emerald-400" />
								<span className="text-xs font-mono font-medium text-slate-300">UnifAI Control Plane Active</span>
							</div>
							<span className="text-xs text-slate-500 font-mono">Real-time Policy Enforcement</span>
						</div>

						<div className="grid grid-cols-1 md:grid-cols-2 gap-6 pt-6">
							{/* Component 1: Browser Guard */}
							<div className="border border-white/[0.06] rounded-xl bg-white/[0.02] p-5 space-y-4">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2.5">
										<div className="h-8 w-8 rounded-lg bg-sky-500/10 border border-sky-500/20 flex items-center justify-center text-sky-400">
											<Shield className="h-4 w-4" />
										</div>
										<div>
											<h3 className="text-sm font-semibold text-white">Browser Guard DLP</h3>
											<p className="text-xs text-slate-400">Endpoint Agent (Win / macOS)</p>
										</div>
									</div>
									<span className="text-[11px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
										Active
									</span>
								</div>

								<div className="text-xs text-slate-300 space-y-2 border-t border-white/[0.04] pt-3 font-mono">
									<div className="flex items-center justify-between text-slate-400">
										<span>Target AI Sites:</span>
										<span className="text-white">ChatGPT, Grok, Claude</span>
									</div>
									<div className="flex items-center justify-between text-slate-400">
										<span>In-Flight Action:</span>
										<span className="text-amber-300">Auto-Redact Credentials & PII</span>
									</div>
									<div className="flex items-center justify-between text-slate-400">
										<span>Network Latency:</span>
										<span className="text-emerald-400">&lt; 1.2 ms</span>
									</div>
								</div>
							</div>

							{/* Component 2: AI Gateway */}
							<div className="border border-white/[0.06] rounded-xl bg-white/[0.02] p-5 space-y-4">
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2.5">
										<div className="h-8 w-8 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
											<Zap className="h-4 w-4" />
										</div>
										<div>
											<h3 className="text-sm font-semibold text-white">Universal AI Gateway</h3>
											<p className="text-xs text-slate-400">Developer & Service Proxy</p>
										</div>
									</div>
									<span className="text-[11px] font-mono px-2 py-0.5 rounded bg-sky-500/10 text-sky-400 border border-sky-500/20">
										Connected
									</span>
								</div>

								<div className="text-xs text-slate-300 space-y-2 border-t border-white/[0.04] pt-3 font-mono">
									<div className="flex items-center justify-between text-slate-400">
										<span>Models Unified:</span>
										<span className="text-white">100+ Providers</span>
									</div>
									<div className="flex items-center justify-between text-slate-400">
										<span>Semantic Cache:</span>
										<span className="text-emerald-400">Enabled (Cost -85%)</span>
									</div>
									<div className="flex items-center justify-between text-slate-400">
										<span>API Standard:</span>
										<span className="text-white">100% OpenAI Drop-in</span>
									</div>
								</div>
							</div>
						</div>
					</div>
				</section>

				{/* Trust & Compliance Strip */}
				<section className="border-y border-white/[0.07] bg-[#0c0e14]/60 py-10 px-6">
					<div className="max-w-6xl mx-auto flex flex-wrap items-center justify-around gap-8 text-center">
						<div className="space-y-1">
							<div className="text-2xl sm:text-3xl font-bold text-white tracking-tight">50M+</div>
							<div className="text-xs text-slate-400">Prompts Inspected</div>
						</div>
						<div className="space-y-1">
							<div className="text-2xl sm:text-3xl font-bold text-white tracking-tight">&lt; 1.2ms</div>
							<div className="text-xs text-slate-400">Endpoint Overhead</div>
						</div>
						<div className="space-y-1">
							<div className="text-2xl sm:text-3xl font-bold text-white tracking-tight">85%</div>
							<div className="text-xs text-slate-400">API Cost Saved via Cache</div>
						</div>
						<div className="space-y-1">
							<div className="text-2xl sm:text-3xl font-bold text-white tracking-tight">99.99%</div>
							<div className="text-xs text-slate-400">Platform Uptime</div>
						</div>
					</div>

					<div className="max-w-4xl mx-auto mt-8 pt-6 border-t border-white/[0.05] flex flex-wrap items-center justify-center gap-6 text-xs text-slate-400">
						<span className="flex items-center gap-1.5"><CheckCircle className="h-3.5 w-3.5 text-slate-300" /> SOC 2 Type II Ready</span>
						<span className="flex items-center gap-1.5"><CheckCircle className="h-3.5 w-3.5 text-slate-300" /> HIPAA Compliant</span>
						<span className="flex items-center gap-1.5"><CheckCircle className="h-3.5 w-3.5 text-slate-300" /> GDPR Ready</span>
						<span className="flex items-center gap-1.5"><CheckCircle className="h-3.5 w-3.5 text-slate-300" /> Zero-Trust Architecture</span>
					</div>
				</section>

				{/* Products Section (Two Pillars) */}
				<section id="products" className="py-24 px-6 max-w-6xl mx-auto">
					<div className="text-center max-w-2xl mx-auto mb-16">
						<h2 className="text-3xl sm:text-4xl font-bold tracking-tight text-white mb-4">
							Two Products. One Architecture.
						</h2>
						<p className="text-slate-400 text-sm sm:text-base">
							Designed to give enterprise leaders complete security over consumer web AI tools while providing engineering teams with a resilient, cost-effective LLM gateway.
						</p>
					</div>

					<div className="grid grid-cols-1 md:grid-cols-2 gap-8">
						{/* Product 1: Browser Guard */}
						<div className="border border-white/[0.08] rounded-2xl bg-[#0f111a]/70 p-8 space-y-6 hover:border-white/[0.15] transition-all">
							<div className="h-10 w-10 rounded-xl bg-sky-500/10 border border-sky-500/20 flex items-center justify-center text-sky-400">
								<Shield className="h-5 w-5" />
							</div>

							<div className="space-y-2">
								<h3 className="text-2xl font-bold text-white">UnifAI Browser Guard</h3>
								<p className="text-slate-400 text-sm leading-relaxed">
									Client-side DLP agent for macOS and Windows. Automatically protects sensitive corporate IP whenever employees interact with ChatGPT, Claude, Grok, or custom AI interfaces.
								</p>
							</div>

							<div className="space-y-3 pt-2">
								{[
									"Zero-latency prompt redaction and blocking",
									"Multi-file document upload and OCR inspection",
									"Full coverage for Grok, ChatGPT, Claude, and Gemini",
									"Silent enterprise deployment via Jamf, Intune, or GPO"
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
									className="text-xs font-semibold text-sky-400 hover:text-sky-300 flex items-center gap-1 cursor-pointer"
								>
									Learn about Browser Guard <ArrowRight className="h-3.5 w-3.5" />
								</button>
							</div>
						</div>

						{/* Product 2: AI Gateway */}
						<div className="border border-white/[0.08] rounded-2xl bg-[#0f111a]/70 p-8 space-y-6 hover:border-white/[0.15] transition-all">
							<div className="h-10 w-10 rounded-xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
								<Zap className="h-5 w-5" />
							</div>

							<div className="space-y-2">
								<h3 className="text-2xl font-bold text-white">UnifAI AI Gateway</h3>
								<p className="text-slate-400 text-sm leading-relaxed">
									A centralized reverse proxy for backend applications. Unifies model routing, semantic vector caching, rate limits, and team access tokens behind a single drop-in API.
								</p>
							</div>

							<div className="space-y-3 pt-2">
								{[
									"1-line OpenAI SDK drop-in replacement",
									"pgvector semantic caching cuts LLM costs by up to 85%",
									"Virtual API keys with team-level spend quotas",
									"Automatic model failover during provider outages"
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
									className="text-xs font-semibold text-indigo-400 hover:text-indigo-300 flex items-center gap-1 cursor-pointer"
								>
									Learn about AI Gateway <ArrowRight className="h-3.5 w-3.5" />
								</button>
							</div>
						</div>
					</div>
				</section>

				{/* Capabilities Grid */}
				<section id="features" className="py-20 px-6 border-t border-white/[0.07] bg-[#0c0e14]/40">
					<div className="max-w-6xl mx-auto">
						<div className="text-center max-w-2xl mx-auto mb-16">
							<h2 className="text-3xl sm:text-4xl font-bold tracking-tight text-white mb-4">
								Enterprise Grade from Day One
							</h2>
							<p className="text-slate-400 text-sm sm:text-base">
								Everything required to satisfy enterprise InfoSec, compliance, and developer productivity standards.
							</p>
						</div>

						<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
							{[
								{
									icon: <Lock className="h-5 w-5 text-sky-400" />,
									title: "In-Flight Data Redaction",
									desc: "API credentials, customer PII, and sensitive source code are sanitized before requests reach third-party AI models."
								},
								{
									icon: <Layers className="h-5 w-5 text-sky-400" />,
									title: "Semantic Vector Caching",
									desc: "High-performance vector embeddings catch semantically identical questions, answering instantly at zero cost."
								},
								{
									icon: <Key className="h-5 w-5 text-sky-400" />,
									title: "Virtual Keys & Quotas",
									desc: "Provision scoped virtual API keys for teams and microservices with strict spend limits and token budgets."
								},
								{
									icon: <Activity className="h-5 w-5 text-sky-400" />,
									title: "Audit Logs & Tracing",
									desc: "Full observability into prompt metadata, token usage, latency metrics, and policy trigger events across your company."
								},
								{
									icon: <Laptop className="h-5 w-5 text-sky-400" />,
									title: "Cross-Platform Agents",
									desc: "Lightweight, silent installers for macOS and Windows, manageable through existing MDM fleets."
								},
								{
									icon: <Cpu className="h-5 w-5 text-sky-400" />,
									title: "Model Fallbacks & Routing",
									desc: "Configure multi-model failovers so applications stay online even when individual model providers experience downtime."
								}
							].map((feature, idx) => (
								<div
									key={idx}
									className="border border-white/[0.06] rounded-xl bg-[#0f111a]/60 p-6 space-y-3"
								>
									<div className="h-9 w-9 rounded-lg bg-white/[0.04] border border-white/[0.08] flex items-center justify-center">
										{feature.icon}
									</div>
									<h3 className="text-base font-semibold text-white">{feature.title}</h3>
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
							<h2 className="text-3xl sm:text-4xl font-bold tracking-tight text-white leading-tight">
								One Line of Code to Integrate
							</h2>
							<p className="text-slate-400 text-sm sm:text-base leading-relaxed">
								UnifAI is fully compatible with the standard OpenAI API specification. Update your connection endpoint and immediately gain semantic caching, virtual key governance, and multi-model routing.
							</p>

							<div className="space-y-2.5 pt-2">
								{[
									"Zero SDK changes or refactoring necessary",
									"Compatible with Python, Node.js, Go, and cURL",
									"Seamless fallback between OpenAI, Anthropic, and Google"
								].map((item, idx) => (
									<div key={idx} className="flex items-center gap-2 text-xs text-slate-300">
										<Check className="h-4 w-4 text-emerald-400 shrink-0" />
										<span>{item}</span>
									</div>
								))}
							</div>
						</div>

						{/* Code Box */}
						<div className="border border-white/[0.08] rounded-xl bg-[#0b0d14] overflow-hidden shadow-2xl">
							<div className="flex bg-white/[0.02] border-b border-white/[0.06] px-4 py-2.5 justify-between items-center">
								<div className="flex gap-1.5">
									<button
										onClick={() => setCodeTab("python")}
										className={`text-xs font-semibold px-3 py-1 rounded-md transition-colors cursor-pointer ${
											codeTab === "python" ? "bg-white/[0.08] text-white" : "text-slate-400 hover:text-white"
										}`}
									>
										Python
									</button>
									<button
										onClick={() => setCodeTab("node")}
										className={`text-xs font-semibold px-3 py-1 rounded-md transition-colors cursor-pointer ${
											codeTab === "node" ? "bg-white/[0.08] text-white" : "text-slate-400 hover:text-white"
										}`}
									>
										Node.js
									</button>
									<button
										onClick={() => setCodeTab("curl")}
										className={`text-xs font-semibold px-3 py-1 rounded-md transition-colors cursor-pointer ${
											codeTab === "curl" ? "bg-white/[0.08] text-white" : "text-slate-400 hover:text-white"
										}`}
									>
										cURL
									</button>
								</div>

								<button
									onClick={() => handleCopyCode(codeExamples[codeTab])}
									className="text-xs text-slate-400 hover:text-white flex items-center gap-1 font-mono p-1 cursor-pointer"
								>
									{copiedCode ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
									<span>{copiedCode ? "Copied" : "Copy"}</span>
								</button>
							</div>

							<div className="p-5 overflow-x-auto text-xs font-mono leading-relaxed bg-[#08090e] text-slate-300 max-h-[300px]">
								<pre className="whitespace-pre">{codeExamples[codeTab]}</pre>
							</div>
						</div>
					</div>
				</section>

				{/* FAQ Section */}
				<section id="faq" className="py-20 px-6 border-t border-white/[0.07] bg-[#0c0e14]/40">
					<div className="max-w-3xl mx-auto">
						<div className="text-center mb-12">
							<h2 className="text-3xl font-bold tracking-tight text-white mb-3">
								Frequently Asked Questions
							</h2>
							<p className="text-slate-400 text-sm">
								Answers to common enterprise security and deployment inquiries.
							</p>
						</div>

						<div className="space-y-3">
							{faqs.map((faq, idx) => (
								<div
									key={idx}
									className="border border-white/[0.06] rounded-xl bg-[#0f111a]/60 overflow-hidden"
								>
									<button
										onClick={() => setOpenFaq(openFaq === idx ? null : idx)}
										className="w-full p-4 sm:p-5 text-left font-medium text-sm text-white flex items-center justify-between gap-4 hover:text-slate-200 transition-colors cursor-pointer"
									>
										<span>{faq.q}</span>
										<ChevronDown className={`h-4 w-4 text-slate-500 shrink-0 transition-transform ${openFaq === idx ? "rotate-180" : ""}`} />
									</button>
									{openFaq === idx && (
										<div className="px-4 sm:px-5 pb-5 text-xs sm:text-sm text-slate-400 leading-relaxed border-t border-white/[0.04] pt-3">
											{faq.a}
										</div>
									)}
								</div>
							))}
						</div>
					</div>
				</section>

				{/* Clean Bottom CTA */}
				<section className="py-24 px-6 text-center max-w-4xl mx-auto">
					<h2 className="text-3xl sm:text-4xl font-bold tracking-tight text-white mb-4">
						Ready to Standardize Your AI Stack?
					</h2>
					<p className="text-slate-400 text-sm sm:text-base max-w-xl mx-auto mb-8">
						Connect with our team for a personalized architecture walkthrough or begin with our self-hosted deployment.
					</p>
					<div className="flex flex-col sm:flex-row items-center justify-center gap-3">
						<button
							onClick={() => setIsDemoModalOpen(true)}
							className="w-full sm:w-auto h-11 px-6 bg-white hover:bg-slate-100 text-[#090a0f] font-semibold rounded-lg text-sm transition-all shadow-sm flex items-center justify-center gap-2 cursor-pointer"
						>
							<Calendar className="h-4 w-4" />
							Schedule a Demo
						</button>
						{isLoggedIn ? (
							<Button
								onClick={() => navigate({ to: "/workspace" })}
								className="w-full sm:w-auto h-11 px-6 bg-white/[0.06] hover:bg-white/[0.1] text-white border border-white/[0.12] font-semibold rounded-lg text-sm"
							>
								Dashboard
							</Button>
						) : (
							<Button
								onClick={() => navigate({ to: "/signup" })}
								className="w-full sm:w-auto h-11 px-6 bg-white/[0.06] hover:bg-white/[0.1] text-white border border-white/[0.12] font-semibold rounded-lg text-sm"
							>
								Get Started Free
							</Button>
						)}
					</div>
				</section>
			</main>

			{/* Minimalist Footer */}
			<footer className="border-t border-white/[0.07] bg-[#07080c] py-12 px-6">
				<div className="max-w-6xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-6 text-xs text-slate-500">
					<div className="flex items-center gap-3">
						<img src={companyLogoSrc} alt={companyFullName} className="h-6 w-auto object-contain opacity-80" />
						<span className="font-semibold text-slate-300">{companyFullName}</span>
					</div>

					<div className="flex items-center gap-6">
						<a href="#products" className="hover:text-slate-300 transition-colors">Products</a>
						<button onClick={() => setIsDemoModalOpen(true)} className="hover:text-slate-300 transition-colors cursor-pointer">Demo</button>
						<a href={`mailto:support@${companyFullName.toLowerCase().replace(/[^a-z0-9]/g, '')}.com`} className="hover:text-slate-300 transition-colors">Contact</a>
					</div>

					<div>&copy; 2026 {companyFullName}. All rights reserved.</div>
				</div>
			</footer>

			{/* Clean "Book a Demo" Modal */}
			<Dialog open={isDemoModalOpen} onOpenChange={setIsDemoModalOpen}>
				<DialogContent className="bg-[#0f111a] border border-white/[0.1] text-slate-200 sm:max-w-lg p-6 shadow-2xl">
					<DialogHeader>
						<DialogTitle className="text-lg font-bold text-white flex items-center gap-2">
							<Calendar className="h-4 w-4 text-sky-400" />
							Schedule a Platform Demo
						</DialogTitle>
						<DialogDescription className="text-slate-400 text-xs">
							Our engineering team will provide a tailored 20-minute demonstration of UnifAI Browser Guard and AI Gateway for your architecture.
						</DialogDescription>
					</DialogHeader>

					{demoSubmitted ? (
						<div className="py-8 text-center space-y-3">
							<div className="h-12 w-12 rounded-full bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400 mx-auto">
								<CheckCircle2 className="h-6 w-6" />
							</div>
							<h3 className="text-base font-bold text-white">Demo Request Received</h3>
							<p className="text-xs text-slate-400 max-w-sm mx-auto leading-relaxed">
								Thank you, <strong className="text-white">{demoForm.fullName}</strong>. We have sent a calendar invitation to <strong className="text-white">{demoForm.email}</strong>.
							</p>
							<div className="pt-2">
								<Button
									onClick={resetDemoModal}
									className="bg-white text-[#090a0f] font-semibold text-xs h-8 px-4"
								>
									Done
								</Button>
							</div>
						</div>
					) : (
						<form onSubmit={handleDemoSubmit} className="space-y-4 mt-2">
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div className="space-y-1">
									<label className="text-xs font-medium text-slate-300">Full Name *</label>
									<Input
										required
										placeholder="Jane Doe"
										value={demoForm.fullName}
										onChange={(e) => setDemoForm({ ...demoForm, fullName: e.target.value })}
										className="bg-white/[0.03] border-white/[0.1] text-white text-xs h-9 focus:border-white/[0.3]"
									/>
								</div>

								<div className="space-y-1">
									<label className="text-xs font-medium text-slate-300">Work Email *</label>
									<Input
										type="email"
										required
										placeholder="jane@company.com"
										value={demoForm.email}
										onChange={(e) => setDemoForm({ ...demoForm, email: e.target.value })}
										className="bg-white/[0.03] border-white/[0.1] text-white text-xs h-9 focus:border-white/[0.3]"
									/>
								</div>
							</div>

							<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
								<div className="space-y-1">
									<label className="text-xs font-medium text-slate-300">Company</label>
									<Input
										placeholder="Acme Inc."
										value={demoForm.company}
										onChange={(e) => setDemoForm({ ...demoForm, company: e.target.value })}
										className="bg-white/[0.03] border-white/[0.1] text-white text-xs h-9 focus:border-white/[0.3]"
									/>
								</div>

								<div className="space-y-1">
									<label className="text-xs font-medium text-slate-300">Team Size</label>
									<select
										value={demoForm.teamSize}
										onChange={(e) => setDemoForm({ ...demoForm, teamSize: e.target.value })}
										className="w-full bg-[#0f111a] border border-white/[0.1] text-white text-xs h-9 rounded-md px-2.5 focus:outline-none focus:border-white/[0.3]"
									>
										<option value="1-50">1 - 50 employees</option>
										<option value="50-250">50 - 250 employees</option>
										<option value="250-1000">250 - 1,000 employees</option>
										<option value="1000+">1,000+ employees</option>
									</select>
								</div>
							</div>

							<div className="space-y-1">
								<label className="text-xs font-medium text-slate-300">Primary Focus</label>
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
													? "border-white/[0.4] bg-white/[0.1] text-white"
													: "border-white/[0.08] bg-white/[0.02] text-slate-400 hover:text-white"
											}`}
										>
											{opt.label}
										</button>
									))}
								</div>
							</div>

							<div className="space-y-1">
								<label className="text-xs font-medium text-slate-300">Requirements or Questions (Optional)</label>
								<textarea
									rows={2}
									placeholder="Tell us about your current AI use cases or security needs..."
									value={demoForm.notes}
									onChange={(e) => setDemoForm({ ...demoForm, notes: e.target.value })}
									className="w-full bg-white/[0.03] border border-white/[0.1] rounded-lg p-2.5 text-white text-xs focus:outline-none focus:border-white/[0.3] resize-none"
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
									className="bg-white hover:bg-slate-100 text-[#090a0f] font-semibold text-xs h-8 px-4 shadow-sm"
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
