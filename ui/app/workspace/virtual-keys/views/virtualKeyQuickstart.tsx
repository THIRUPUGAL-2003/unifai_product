import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { VirtualKey } from "@/lib/types/governance";
import { Check, Copy, Terminal, Code2, Play, Loader2, CheckCircle2, AlertCircle, Zap } from "lucide-react";
import { useMemo, useState } from "react";

interface VirtualKeyQuickstartProps {
	virtualKey: VirtualKey;
}

interface TestResult {
	success: boolean;
	status: number;
	latency: number;
	model?: string;
	content?: string;
	error?: string;
	isCacheHit?: boolean;
}

export default function VirtualKeyQuickstart({ virtualKey }: VirtualKeyQuickstartProps) {
	const [activeTab, setActiveTab] = useState<"python" | "curl" | "node">("python");
	const { copy, copied } = useCopyToClipboard({ successMessage: "Code snippet copied" });

	const [isTesting, setIsTesting] = useState(false);
	const [testResult, setTestResult] = useState<TestResult | null>(null);

	const baseUrl = useMemo(() => {
		if (typeof window !== "undefined" && window.location.origin) {
			return `${window.location.origin.replace(/\/+$/, "")}/v1`;
		}
		return "<YOUR_UNIFAI_URL>/v1";
	}, []);

	const keySecret = virtualKey.value || "sk-uf-your-virtual-key";

	const recommendedModel = useMemo(() => {
		if (virtualKey.provider_configs && virtualKey.provider_configs.length > 0) {
			for (const pc of virtualKey.provider_configs) {
				if (pc.allowed_models && pc.allowed_models.length > 0) {
					const specific = pc.allowed_models.find((m) => m !== "*");
					if (specific) return specific;
					if (pc.provider === "mistral") return "mistral/mistral-tiny";
					if (pc.provider === "cohere") return "cohere/command-r7b-12-2024";
					if (pc.provider === "openrouter") return "openrouter/meta-llama/llama-3-8b-instruct";
					return `${pc.provider}/default`;
				}
			}
		}
		return "mistral/mistral-tiny";
	}, [virtualKey]);

	const snippets = useMemo(() => {
		const python = `from openai import OpenAI

client = OpenAI(
    base_url="${baseUrl}",
    api_key="${keySecret}"
)

response = client.chat.completions.create(
    model="${recommendedModel}",
    messages=[
        {"role": "user", "content": "Hello from UniFAI!"}
    ]
)

print(response.choices[0].message.content)`;

		const curl = `curl -X POST "${baseUrl}/chat/completions" \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${keySecret}" \\
  -d '{
    "model": "${recommendedModel}",
    "messages": [
      {"role": "user", "content": "Hello from UniFAI!"}
    ]
  }'`;

		const node = `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${baseUrl}",
  apiKey: "${keySecret}",
});

async function main() {
  const completion = await client.chat.completions.create({
    model: "${recommendedModel}",
    messages: [
      { role: "user", content: "Hello from UniFAI!" }
    ],
  });

  console.log(completion.choices[0].message.content);
}

main();`;

		return { python, curl, node };
	}, [baseUrl, keySecret, recommendedModel]);

	const currentCode = snippets[activeTab];

	const handleSendTestPing = async () => {
		if (!virtualKey.value) return;
		setIsTesting(true);
		setTestResult(null);

		const startTime = performance.now();
		try {
			const res = await fetch(`${baseUrl}/chat/completions`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					Authorization: `Bearer ${virtualKey.value}`,
				},
				body: JSON.stringify({
					model: recommendedModel,
					messages: [{ role: "user", content: "Hello UniFAI! Test ping." }],
					max_tokens: 30,
				}),
			});

			const latency = Math.round(performance.now() - startTime);
			const isCache = res.headers.get("x-uf-cache") === "hit";
			const data = await res.json().catch(() => ({}));

			if (res.ok) {
				const content = data?.choices?.[0]?.message?.content || "Connected successfully.";
				setTestResult({
					success: true,
					status: res.status,
					latency,
					model: data?.model || recommendedModel,
					content,
					isCacheHit: isCache,
				});
			} else {
				const errMsg = data?.error?.message || data?.message || `HTTP ${res.status} Error`;
				setTestResult({
					success: false,
					status: res.status,
					latency,
					error: errMsg,
				});
			}
		} catch (err: unknown) {
			const latency = Math.round(performance.now() - startTime);
			setTestResult({
				success: false,
				status: 0,
				latency,
				error: err instanceof Error ? err.message : "Network error reaching endpoint",
			});
		} finally {
			setIsTesting(false);
		}
	};

	return (
		<div className="space-y-4">
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-2">
					<Code2 className="h-4 w-4 text-primary" />
					<h3 className="text-sm font-semibold">Integration Snippet</h3>
				</div>
				<div className="flex items-center gap-2">
					<Button
						variant="secondary"
						size="sm"
						className="h-8 gap-1.5 text-xs font-medium"
						onClick={handleSendTestPing}
						disabled={isTesting || !virtualKey.value}
						data-testid="vk-test-ping-btn"
					>
						{isTesting ? (
							<>
								<Loader2 className="h-3.5 w-3.5 animate-spin" />
								<span>Pinging...</span>
							</>
						) : (
							<>
								<Play className="h-3 w-3 fill-current text-primary" />
								<span>Live Test Ping</span>
							</>
						)}
					</Button>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="sm"
								className="h-8 gap-1.5 text-xs"
								onClick={() => void copy(currentCode)}
							>
								{copied ? <Check className="h-3.5 w-3.5 text-green-600" /> : <Copy className="h-3.5 w-3.5" />}
								<span>{copied ? "Copied" : "Copy Code"}</span>
							</Button>
						</TooltipTrigger>
						<TooltipContent>Copy {activeTab.toUpperCase()} code with this key</TooltipContent>
					</Tooltip>
				</div>
			</div>

			{/* Live Test Result Banner */}
			{testResult && (
				<div
					className={`rounded-md border p-3 text-xs transition-all ${
						testResult.success
							? "border-emerald-500/30 bg-emerald-500/10 text-emerald-900 dark:text-emerald-100"
							: "border-red-500/30 bg-red-500/10 text-red-900 dark:text-red-100"
					}`}
					data-testid="vk-test-result-banner"
				>
					<div className="flex items-center justify-between">
						<div className="flex items-center gap-2 font-semibold">
							{testResult.success ? (
								<>
									<CheckCircle2 className="h-4 w-4 text-emerald-500" />
									<span>HTTP {testResult.status} OK — Virtual Key Operational</span>
								</>
							) : (
								<>
									<AlertCircle className="h-4 w-4 text-red-500" />
									<span>HTTP {testResult.status} Error — Test Failed</span>
								</>
							)}
						</div>
						<div className="flex items-center gap-2">
							{testResult.isCacheHit && (
								<span className="flex items-center gap-1 rounded bg-blue-500/20 px-1.5 py-0.5 text-[10px] font-medium text-blue-600 dark:text-blue-300">
									<Zap className="h-3 w-3" /> Cache Hit
								</span>
							)}
							<span className="rounded bg-background/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
								{testResult.latency}ms
							</span>
						</div>
					</div>

					{testResult.success && testResult.content && (
						<div className="mt-2 rounded bg-background/40 p-2 font-mono text-[11px] leading-relaxed text-foreground">
							{testResult.content}
						</div>
					)}

					{!testResult.success && testResult.error && (
						<div className="mt-2 rounded bg-background/40 p-2 font-mono text-[11px] text-red-600 dark:text-red-300">
							{testResult.error}
						</div>
					)}
				</div>
			)}

			<Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as "python" | "curl" | "node")}>
				<TabsList className="h-8 w-full justify-start rounded-md bg-muted/60 p-1">
					<TabsTrigger value="python" className="h-6 text-xs">
						Python (OpenAI SDK)
					</TabsTrigger>
					<TabsTrigger value="curl" className="h-6 text-xs">
						cURL
					</TabsTrigger>
					<TabsTrigger value="node" className="h-6 text-xs">
						Node.js / TS
					</TabsTrigger>
				</TabsList>

				<div className="relative mt-2 overflow-hidden rounded-md border bg-[#0d1117] text-slate-100 shadow-sm">
					<div className="flex items-center justify-between border-b border-white/10 px-3 py-1.5 text-[11px] text-slate-400">
						<div className="flex items-center gap-1.5">
							<Terminal className="h-3.5 w-3.5 text-slate-400" />
							<span>{activeTab === "python" ? "python script.py" : activeTab === "curl" ? "bash / terminal" : "index.ts"}</span>
						</div>
						<span className="font-mono text-[10px] text-slate-500">{baseUrl}</span>
					</div>
					<TabsContent value="python" className="m-0">
						<pre className="overflow-x-auto p-3.5 font-mono text-xs leading-5 text-slate-200">
							<code>{snippets.python}</code>
						</pre>
					</TabsContent>
					<TabsContent value="curl" className="m-0">
						<pre className="overflow-x-auto p-3.5 font-mono text-xs leading-5 text-slate-200">
							<code>{snippets.curl}</code>
						</pre>
					</TabsContent>
					<TabsContent value="node" className="m-0">
						<pre className="overflow-x-auto p-3.5 font-mono text-xs leading-5 text-slate-200">
							<code>{snippets.node}</code>
						</pre>
					</TabsContent>
				</div>
			</Tabs>
		</div>
	);
}
