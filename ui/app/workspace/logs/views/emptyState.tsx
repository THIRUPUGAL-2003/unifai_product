import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { PRODUCT_NAME } from "@/lib/constants/config";
import { getExampleBaseUrl } from "@/lib/utils/port";
import { AlertTriangle, Check, Copy } from "lucide-react";
import { useMemo, useState } from "react";

type Provider = "openai" | "anthropic" | "genai" | "litellm" | "langchain";
type Language = "python" | "typescript";

type Examples = {
	curl: string;
	sdk: {
		[P in Provider]: {
			[L in Language]: string;
		};
	};
};

interface CodeBlockProps {
	code: string;
	language: string;
	onLanguageChange?: (language: string) => void;
	showLanguageSelect?: boolean;
	readonly?: boolean;
}

function CodeBlock({ code, language, onLanguageChange, showLanguageSelect = false }: CodeBlockProps) {
	const { copy: copyToClipboard } = useCopyToClipboard();
	const [copied, setCopied] = useState(false);

	const handleCopy = () => {
		copyToClipboard(code);
		setCopied(true);
		setTimeout(() => setCopied(false), 2000);
	};

	return (
		<div className="relative overflow-hidden rounded-lg border border-border/80 bg-zinc-950/95 dark:bg-zinc-950/90 shadow-sm">
			<div className="flex items-center justify-between border-b border-border/40 bg-zinc-900/60 px-4 py-2">
				<div className="flex items-center gap-2">
					<div className="h-2.5 w-2.5 rounded-full bg-red-500/70" />
					<div className="h-2.5 w-2.5 rounded-full bg-amber-500/70" />
					<div className="h-2.5 w-2.5 rounded-full bg-emerald-500/70" />
					<span className="ml-2 font-mono text-xs text-zinc-400 capitalize">{language}</span>
				</div>
				<div className="flex items-center gap-2">
					{showLanguageSelect && onLanguageChange && (
						<Select value={language} onValueChange={onLanguageChange}>
							<SelectTrigger className="h-7 w-fit bg-zinc-800/80 border-zinc-700/60 text-zinc-200 text-xs hover:bg-zinc-800">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem className="text-xs" value="python">
									Python
								</SelectItem>
								<SelectItem className="text-xs" value="typescript">
									TypeScript
								</SelectItem>
							</SelectContent>
						</Select>
					)}
					<Button
						variant="ghost"
						size="icon"
						onClick={handleCopy}
						className="h-7 w-7 text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800/60"
						aria-label="Copy to clipboard"
					>
						{copied ? <Check className="size-3.5 text-emerald-400" /> : <Copy className="size-3.5" />}
					</Button>
				</div>
			</div>
			<div className="h-[300px] overflow-auto p-4 font-mono text-xs leading-relaxed text-zinc-200 selection:bg-primary/40">
				<pre className="whitespace-pre font-mono">
					<code>{code}</code>
				</pre>
			</div>
		</div>
	);
}

interface EmptyStateProps {
	error: string | null;
}

export function EmptyState({ error }: EmptyStateProps) {
	const [language, setLanguage] = useState<Language>("python");

	// Generate examples dynamically using the port utility
	const examples: Examples = useMemo(() => {
		const baseUrl = getExampleBaseUrl();

		return {
			curl: `curl -X POST ${baseUrl}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "openai/gpt-4o-mini",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'`,
			sdk: {
				openai: {
					python: `import openai

client = openai.OpenAI(
    base_url="${baseUrl}/openai",
    api_key="dummy-api-key" # Handled by ${PRODUCT_NAME}
)

response = client.chat.completions.create(
    model="gpt-4o-mini", # or "provider/model" for other providers (anthropic/claude-3-sonnet)
    messages=[{"role": "user", "content": "Hello!"}]
)`,
					typescript: `import OpenAI from "openai";

const openai = new OpenAI({
  baseURL: "${baseUrl}/openai",
  apiKey: "dummy-api-key", // Handled by ${PRODUCT_NAME}
});

const response = await openai.chat.completions.create({
  model: "gpt-4o-mini", // or "provider/model" for other providers (anthropic/claude-3-sonnet)
  messages: [{ role: "user", content: "Hello!" }],
});`,
				},
				anthropic: {
					python: `import anthropic

client = anthropic.Anthropic(
    base_url="${baseUrl}/anthropic",
    api_key="dummy-api-key" # Handled by ${PRODUCT_NAME}
)

response = client.messages.create(
    model="claude-3-sonnet-20240229", # or "provider/model" for other providers (openai/gpt-4o-mini)
    max_tokens=1000,
    messages=[{"role": "user", "content": "Hello!"}]
)`,
					typescript: `import Anthropic from "@anthropic-ai/sdk";

const anthropic = new Anthropic({
  baseURL: "${baseUrl}/anthropic",
  apiKey: "dummy-api-key", // Handled by ${PRODUCT_NAME}
});

const response = await anthropic.messages.create({
  model: "claude-3-sonnet-20240229", // or "provider/model" for other providers (openai/gpt-4o-mini)
  max_tokens: 1000,
  messages: [{ role: "user", content: "Hello!" }],
});`,
				},
				genai: {
					python: `from google import genai
from google.genai.types import HttpOptions

client = genai.Client(
    api_key="dummy-api-key", # Handled by ${PRODUCT_NAME}
    http_options=HttpOptions(base_url="${baseUrl}/genai")
)

response = client.models.generate_content(
    model="gemini-2.5-pro", # or "provider/model" for other providers (openai/gpt-4o-mini)
    contents="Hello!"
)`,
					typescript: `import { GoogleGenerativeAI } from "@google/generative-ai";

const genAI = new GoogleGenerativeAI("dummy-api-key", { // Handled by ${PRODUCT_NAME}
  baseUrl: "${baseUrl}/genai",
});

const model = genAI.getGenerativeModel({ model: "gemini-2.5-pro" }); // or "provider/model" for other providers (openai/gpt-4o-mini)
const response = await model.generateContent("Hello!");`,
				},
				litellm: {
					python: `import litellm

litellm.api_base = "${baseUrl}/litellm"

response = litellm.completion(
    model="openai/gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello!"}]
)`,
					typescript: `import { completion } from "litellm";

const response = await completion({
  model: "openai/gpt-4o-mini",
  messages: [{ role: "user", content: "Hello!" }],
  api_base: "${baseUrl}/litellm",
});`,
				},
				langchain: {
					python: `from langchain_openai import ChatOpenAI
from langchain_core.messages import HumanMessage
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.output_parsers import StrOutputParser

# Initialize ChatOpenAI with ${PRODUCT_NAME}
llm = ChatOpenAI(
    model="gpt-4o-mini",
    api_key="dummy-api-key",  # Handled by ${PRODUCT_NAME}
    base_url="${baseUrl}/langchain",
    max_tokens=100,
)

# Simple message
messages = [HumanMessage(content="Hello from LangChain!")]
response = llm.invoke(messages)

# Chain with prompt template
prompt = ChatPromptTemplate.from_messages([
    ("system", "You are a helpful assistant."),
    ("human", "{input}")
])

chain = prompt | llm | StrOutputParser()
result = chain.invoke({"input": "What is LangChain?"})`,
					typescript: `import { ChatOpenAI } from "@langchain/openai";
import { HumanMessage } from "@langchain/core/messages";
import { ChatPromptTemplate } from "@langchain/core/prompts";
import { StringOutputParser } from "@langchain/core/output_parsers";

// Initialize ChatOpenAI with ${PRODUCT_NAME}
const llm = new ChatOpenAI({
  model: "gpt-4o-mini",
  openAIApiKey: "dummy-api-key", // Handled by ${PRODUCT_NAME}
  clientOptions: {
    baseURL: "${baseUrl}/langchain",
  },
  maxTokens: 100,
});

// Simple message
const messages = [new HumanMessage("Hello from LangChain!")];
const response = await llm.invoke(messages);

// Chain with prompt template
const prompt = ChatPromptTemplate.fromMessages([
  ["system", "You are a helpful assistant."],
  ["human", "{input}"],
]);

const chain = prompt.pipe(llm).pipe(new StringOutputParser());
const result = await chain.invoke({ input: "What is LangChain?" });`,
				},
			},
		};
	}, []);

	const isUnexpectedError = error && error.includes("An unexpected error occurred");

	return (
		<div className="dark:bg-card flex w-full flex-col items-center justify-center space-y-8 bg-white">
			{error && (
				<Alert>
					<AlertTriangle className="h-4 w-4" />
					<AlertDescription>
						{isUnexpectedError ? "Looks like you haven't configured the log store in your config file." : error}
					</AlertDescription>
				</Alert>
			)}

			<div className="w-full space-y-6 p-4">
				<div className="flex flex-row items-center gap-2">
					<div>
						<h3 className="text-lg font-semibold">Integrate under 60 seconds</h3>
						<p className="text-muted-foreground text-sm">Send your first request to get started</p>
					</div>
				</div>

				<Tabs defaultValue="curl" className="w-full rounded-lg border">
					<TabsList className="grid h-10 w-full grid-cols-6 rounded-t-lg rounded-b-none">
						<TabsTrigger value="curl">cURL</TabsTrigger>
						<TabsTrigger value="openai">OpenAI SDK</TabsTrigger>
						<TabsTrigger value="anthropic">Anthropic SDK</TabsTrigger>
						<TabsTrigger value="genai">Google GenAI SDK</TabsTrigger>
						<TabsTrigger value="litellm">LiteLLM SDK</TabsTrigger>
						<TabsTrigger value="langchain">LangChain SDK</TabsTrigger>
					</TabsList>

					<TabsContent value="curl" className="px-4">
						<CodeBlock code={examples.curl} language="bash" readonly={false} />
					</TabsContent>

					<TabsContent value="openai" className="px-4">
						<CodeBlock
							code={examples.sdk.openai[language]}
							language={language}
							onLanguageChange={(newLang) => setLanguage(newLang as Language)}
							showLanguageSelect
						/>
					</TabsContent>

					<TabsContent value="anthropic" className="px-4">
						<CodeBlock
							code={examples.sdk.anthropic[language]}
							language={language}
							onLanguageChange={(newLang) => setLanguage(newLang as Language)}
							showLanguageSelect
						/>
					</TabsContent>

					<TabsContent value="genai" className="px-4">
						<CodeBlock
							code={examples.sdk.genai[language]}
							language={language}
							onLanguageChange={(newLang) => setLanguage(newLang as Language)}
							showLanguageSelect
						/>
					</TabsContent>

					<TabsContent value="litellm" className="px-4">
						<CodeBlock
							code={examples.sdk.litellm[language]}
							language={language}
							onLanguageChange={(newLang) => setLanguage(newLang as Language)}
							showLanguageSelect
						/>
					</TabsContent>

					<TabsContent value="langchain" className="px-4">
						<CodeBlock
							code={examples.sdk.langchain[language]}
							language={language}
							onLanguageChange={(newLang) => setLanguage(newLang as Language)}
							showLanguageSelect
						/>
					</TabsContent>
				</Tabs>
			</div>
		</div>
	);
}