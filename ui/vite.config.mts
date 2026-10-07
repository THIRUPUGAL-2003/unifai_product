import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv } from "vite";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const isEnterpriseBuild = fs.existsSync(path.join(__dirname, "app", "enterprise"));

/** Prefer GATEWAY_* then legacy GATEWAY_* from .env (loadEnv + process.env). */
function gatewayEnv(env: Record<string, string>, key: string): string {
	const k = key.replace(/^(GATEWAY_|GATEWAY_)/, "");
	return String(
		env[`GATEWAY_${k}`] ||
			env[`GATEWAY_${k}`] ||
			process.env[`GATEWAY_${k}`] ||
			process.env[`GATEWAY_${k}`] ||
			"",
	).trim();
}

export default defineConfig(({ mode }) => {
	const rootDir = path.resolve(__dirname, "..");
	const env = loadEnv(mode, rootDir, "");

	const backendPort = env.APP_PORT || gatewayEnv(env, "PORT") || "8001";
	const backendTarget = gatewayEnv(env, "BACKEND_URL") || `http://localhost:${backendPort}`;
	const uiPort = Number(env.UI_PORT || process.env.UI_PORT || 3000);
	const companyName = gatewayEnv(env, "COMPANY_NAME") || "YesPanchi Group of Companies";
	const companyShortName = gatewayEnv(env, "COMPANY_SHORT_NAME") || "YesPanchi";
	const companyLogo = gatewayEnv(env, "COMPANY_LOGO") || "/yes-panchi-logo.png";
	const productName = gatewayEnv(env, "PRODUCT_NAME") || env.PRODUCT_NAME || "Gateway";
	const productFullName =
		gatewayEnv(env, "PRODUCT_FULL_NAME") ||
		gatewayEnv(env, "PRODUCT_SUBTITLE") ||
		"Real-time AI Knowledge Screening & Hazard Audit";
	const footerCopyright = gatewayEnv(env, "FOOTER_COPYRIGHT") || "";
	const footerSubtitle =
		gatewayEnv(env, "FOOTER_SUBTITLE") || `${productName} - Real-time AI Knowledge Screening & Hazard Audit`;
	const isEnterprise = gatewayEnv(env, "IS_ENTERPRISE");
	const disableProfiler = gatewayEnv(env, "DISABLE_PROFILER");
	const trialExpiry =
		gatewayEnv(env, "ENTERPRISE_TRIAL_EXPIRY") || String(process.env.ENTERPRISE_TRIAL_EXPIRY ?? "").trim();

	return {
		plugins: [
		tanstackRouter({
			target: "react",
			routesDirectory: "./app",
			generatedRouteTree: "./app/routeTree.gen.ts",
			// All routes live in layout.tsx files. page.tsx files are pure view
			// components imported by their sibling layout.tsx (Next-style mental
			// model preserved for content, but routing config lives in one place).
			routeToken: "layout",
			// Treat ONLY layout.tsx / __root.tsx as routes; everything else under app/
			// (page.tsx, views, components, helpers) is ignored.
			// Directory entries have no extension and are not matched, so recursion still works.
			routeFileIgnorePattern: "^(?!layout\\.tsx$|__root\\.tsx$).+\\.(tsx|ts|jsx|js)$",
			autoCodeSplitting: true,
		}),
		react(),
		tailwindcss(),
	],
	resolve: {
		// Enterprise UI source is symlinked into ./app/enterprise; preserving symlinks
		// keeps module resolution rooted here so deps like zod / @phosphor-icons/react
		// resolve against this ui/node_modules rather than the symlink target's tree.
		preserveSymlinks: true,
		alias: {
			"@": path.resolve(__dirname),
			"@enterprise": isEnterpriseBuild
				? path.resolve(__dirname, "app", "enterprise")
				: path.resolve(__dirname, "app", "_fallbacks", "enterprise"),
			"@schemas": isEnterpriseBuild
				? path.resolve(__dirname, "app", "enterprise", "lib", "schemas")
				: path.resolve(__dirname, "app", "_fallbacks", "enterprise", "lib", "schemas"),
		},
	},
	define: {
		"process.env.NODE_ENV": JSON.stringify(process.env.NODE_ENV ?? "production"),
		"process.env.GATEWAY_IS_ENTERPRISE": JSON.stringify(isEnterpriseBuild ? "true" : isEnterprise || "false"),
		"process.env.GATEWAY_IS_ENTERPRISE": JSON.stringify(isEnterpriseBuild ? "true" : isEnterprise || "false"),
		"process.env.GATEWAY_DISABLE_PROFILER": JSON.stringify(disableProfiler),
		"process.env.GATEWAY_DISABLE_PROFILER": JSON.stringify(disableProfiler),
		"process.env.GATEWAY_ENTERPRISE_TRIAL_EXPIRY": JSON.stringify(trialExpiry),
		"process.env.GATEWAY_ENTERPRISE_TRIAL_EXPIRY": JSON.stringify(trialExpiry),
		"process.env.GATEWAY_PRODUCT_NAME": JSON.stringify(productName),
		"process.env.GATEWAY_PRODUCT_NAME": JSON.stringify(productName),
		"process.env.GATEWAY_PRODUCT_FULL_NAME": JSON.stringify(productFullName),
		"process.env.GATEWAY_PRODUCT_FULL_NAME": JSON.stringify(productFullName),
		"process.env.GATEWAY_COMPANY_NAME": JSON.stringify(companyName),
		"process.env.GATEWAY_COMPANY_NAME": JSON.stringify(companyName),
		"process.env.GATEWAY_COMPANY_SHORT_NAME": JSON.stringify(companyShortName),
		"process.env.GATEWAY_COMPANY_SHORT_NAME": JSON.stringify(companyShortName),
		"process.env.GATEWAY_COMPANY_LOGO": JSON.stringify(companyLogo),
		"process.env.GATEWAY_COMPANY_LOGO": JSON.stringify(companyLogo),
		"process.env.GATEWAY_FOOTER_COPYRIGHT": JSON.stringify(footerCopyright),
		"process.env.GATEWAY_FOOTER_COPYRIGHT": JSON.stringify(footerCopyright),
		"process.env.GATEWAY_FOOTER_SUBTITLE": JSON.stringify(footerSubtitle),
		"process.env.GATEWAY_FOOTER_SUBTITLE": JSON.stringify(footerSubtitle),
		"process.env.GATEWAY_PORT": JSON.stringify(backendPort),
		"process.env.GATEWAY_PORT": JSON.stringify(backendPort),
		"process.env.GATEWAY_BACKEND_URL": JSON.stringify(backendTarget),
		"process.env.GATEWAY_BACKEND_URL": JSON.stringify(backendTarget),
	},
	server: {
		port: uiPort,
		proxy: {
			"/api": {
				target: backendTarget,
				changeOrigin: true,
			},
			"/v1": {
				target: backendTarget,
				changeOrigin: true,
			},
		},
	},
	build: {
		outDir: "out",
		emptyOutDir: true,
	},
	};
});
