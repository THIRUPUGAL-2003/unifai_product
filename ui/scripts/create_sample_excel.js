const path = require("path");
const fs = require("fs");
const XLSX = require("xlsx");

// Destination directories
const projectRoot = path.resolve(__dirname, "../../");
const desktopRoot = path.resolve(projectRoot, "../");

console.log("Generating sample Excel and CSV files...");

// -------------------------------------------------------------
// 1. Target Websites Sample Excel & CSV
// -------------------------------------------------------------
const targetHeaders = [
	"Domain",
	"Platform Name",
	"Host Role",
	"Monitored",
	"Block Entire Website",
];

const targetRows = [
	// ChatGPT (Main UI & Subdomains)
	["chatgpt.com", "ChatGPT", "ui", "TRUE", "FALSE"],
	["ab.chatgpt.com", "ChatGPT", "chat", "TRUE", "FALSE"],
	["chat.openai.com", "ChatGPT", "chat", "TRUE", "FALSE"],
	["api.openai.com", "ChatGPT", "chat", "TRUE", "FALSE"],
	["files.oaiusercontent.com", "ChatGPT", "file", "TRUE", "FALSE"],
	["oaiusercontent.com", "ChatGPT", "file", "TRUE", "FALSE"],
	["oaistatic.com", "ChatGPT", "file", "TRUE", "FALSE"],
	["cdn.oaistatic.com", "ChatGPT", "file", "TRUE", "FALSE"],
	["ws.chatgpt.com", "ChatGPT", "chat", "TRUE", "FALSE"],
	["ws-api.chatgpt.com", "ChatGPT", "chat", "TRUE", "FALSE"],
	["realtime.chatgpt.com", "ChatGPT", "chat", "TRUE", "FALSE"],
	["openai.com", "ChatGPT", "chat", "TRUE", "FALSE"],
	["browser-gateway-openai.livekit.cloud", "ChatGPT", "chat", "TRUE", "FALSE"],

	// Claude (Main UI & Subdomains)
	["claude.ai", "Claude", "ui", "TRUE", "FALSE"],
	["api.anthropic.com", "Claude", "chat", "TRUE", "FALSE"],
	["files.claudeusercontent.com", "Claude", "file", "TRUE", "FALSE"],
	["cdn.claude.ai", "Claude", "file", "TRUE", "FALSE"],
	["anthropic.com", "Claude", "ui", "TRUE", "FALSE"],

	// Gemini (Main UI & Subdomains)
	["gemini.google.com", "Gemini", "ui", "TRUE", "FALSE"],
	["clients6.google.com", "Gemini", "chat", "TRUE", "FALSE"],
	["generativelanguage.googleapis.com", "Gemini", "chat", "TRUE", "FALSE"],
	["aistudio.google.com", "Gemini", "ui", "TRUE", "FALSE"],
	["bard.google.com", "Gemini", "ui", "TRUE", "FALSE"],
];

const targetWs = XLSX.utils.aoa_to_sheet([targetHeaders, ...targetRows]);
targetWs["!cols"] = [
	{ wch: 30 }, // Domain
	{ wch: 24 }, // Platform Name
	{ wch: 18 }, // Host Role
	{ wch: 14 }, // Monitored
	{ wch: 26 }, // Block Entire Website
];

const targetWb = XLSX.utils.book_new();
XLSX.utils.book_append_sheet(targetWb, targetWs, "Target Websites");

// -------------------------------------------------------------
// 2. Guard Rules Sample Excel & CSV
// -------------------------------------------------------------
const ruleHeaders = [
	"Rule Name",
	"Rule Type",
	"Pattern",
	"Severity",
	"Action",
	"Warning Message",
	"Active",
	"Description",
];

const ruleRows = [
	[
		"Block Credit Card Numbers",
		"regex",
		"\\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14})\\b",
		"CRITICAL",
		"BLOCK",
		"Credit card numbers are prohibited by corporate security policy.",
		"TRUE",
		"PCI-DSS — block card PANs in AI prompts",
	],
	[
		"Redact Salary & Compensation",
		"regex",
		"\\b(?:salary|payroll|bonus|compensation|ctc|appraisal)\\b",
		"MEDIUM",
		"REDACT",
		"Employee compensation details redacted.",
		"TRUE",
		"HR confidentiality",
	],
	[
		"Block Aadhaar Numbers",
		"regex",
		"\\b[2-9]{1}[0-9]{3}\\s?[0-9]{4}\\s?[0-9]{4}\\b",
		"CRITICAL",
		"BLOCK",
		"Aadhaar numbers are strictly prohibited in AI chats.",
		"TRUE",
		"India PII protection",
	],
	[
		"Warn on API & Secret Keys",
		"regex",
		"\\b(?:api[_-]?key|secret[_-]?token|bearer[\\s]+[a-zA-Z0-9_\\-]{16,})\\b",
		"HIGH",
		"WARN",
		"Potential API key detected. Please verify before sending.",
		"TRUE",
		"Secrets hygiene",
	],
	[
		"Block Private Key Leakage",
		"regex",
		"\\b(?:BEGIN RSA PRIVATE KEY|ssh-rsa|PRIVATE KEY-----)\\b",
		"CRITICAL",
		"BLOCK",
		"Private keys and credentials cannot be shared with external AI.",
		"TRUE",
		"Security credentials",
	],
	[
		"Redact Internal Project Codename",
		"regex",
		"\\b(?:ProjectTitan|ProjectApex|SecretSauce)\\b",
		"HIGH",
		"REDACT",
		"Internal project codename redacted.",
		"TRUE",
		"Intellectual Property protection",
	],
];

const ruleWs = XLSX.utils.aoa_to_sheet([ruleHeaders, ...ruleRows]);
ruleWs["!cols"] = [
	{ wch: 32 }, // Rule Name
	{ wch: 12 }, // Rule Type
	{ wch: 55 }, // Pattern
	{ wch: 12 }, // Severity
	{ wch: 10 }, // Action
	{ wch: 60 }, // Warning Message
	{ wch: 10 }, // Active
	{ wch: 35 }, // Description
];

const ruleWb = XLSX.utils.book_new();
XLSX.utils.book_append_sheet(ruleWb, ruleWs, "Guard Rules");

// Write files to project root
const targetDestinations = [
	path.join(projectRoot, "target_websites_sample.xlsx"),
];

const targetCsvDestinations = [
	path.join(projectRoot, "target_websites_sample.csv"),
];

const ruleDestinations = [
	path.join(projectRoot, "guard_rules_sample.xlsx"),
];

const ruleCsvDestinations = [
	path.join(projectRoot, "guard_rules_sample.csv"),
];

for (const dest of targetDestinations) {
	try {
		XLSX.writeFile(targetWb, dest);
		console.log(`Saved Target Websites Excel: ${dest}`);
	} catch (e) {
		console.error(`Failed to write to ${dest}:`, e.message);
	}
}

for (const dest of targetCsvDestinations) {
	try {
		const csvContent = XLSX.utils.sheet_to_csv(targetWs);
		fs.writeFileSync(dest, csvContent, "utf8");
		console.log(`Saved Target Websites CSV: ${dest}`);
	} catch (e) {
		console.error(`Failed to write to ${dest}:`, e.message);
	}
}

for (const dest of ruleDestinations) {
	try {
		XLSX.writeFile(ruleWb, dest);
		console.log(`Saved Guard Rules Excel: ${dest}`);
	} catch (e) {
		console.error(`Failed to write to ${dest}:`, e.message);
	}
}

for (const dest of ruleCsvDestinations) {
	try {
		const csvContent = XLSX.utils.sheet_to_csv(ruleWs);
		fs.writeFileSync(dest, csvContent, "utf8");
		console.log(`Saved Guard Rules CSV: ${dest}`);
	} catch (e) {
		console.error(`Failed to write to ${dest}:`, e.message);
	}
}

console.log("All sample files generated successfully!");
