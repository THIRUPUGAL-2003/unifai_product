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

	const serverDomain = (env.SERVER_DOMAIN || gatewayEnv(env, "SERVER_DOMAIN") || "").trim().replace(/\/+$/, "");
	const backendPort = env.APP_PORT || gatewayEnv(env, "PORT") || "";
	const localBackend = backendPort ? `http://127.0.0.1:${backendPort}` : "http://127.0.0.1:6000";
	const backendTarget =
		mode === "development" && !env.USE_REMOTE_BACKEND
			? localBackend
			: serverDomain || gatewayEnv(env, "BACKEND_URL") || env.GATEWAY_BACKEND_URL || localBackend;
	const brandName = gatewayEnv(env, "BRAND_NAME") || env.GATEWAY_BRAND_NAME || "Raksha";
	const uiPort = Number(env.UI_PORT || process.env.UI_PORT || 3100);
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

	const appHost = env.APP_HOST || gatewayEnv(env, "HOST") || "0.0.0.0";
	const proxyPort = env.PROXY_PORT || gatewayEnv(env, "PROXY_PORT") || "18182";
	const containerName = env.CONTAINER_NAME || gatewayEnv(env, "CONTAINER_NAME") || "raksha_yespanchi";
	const gatewayProxyAddr = env.GATEWAY_PROXY_ADDR || gatewayEnv(env, "GATEWAY_PROXY_ADDR") || "127.0.0.1:18103";
	const pacHttpPort = env.PAC_HTTP_PORT || gatewayEnv(env, "PAC_HTTP_PORT") || "18195";
	const mitmWebPort = env.MITM_WEB_PORT || gatewayEnv(env, "MITM_WEB_PORT") || "18183";
	const networkProxyContainerName = env.NETWORK_PROXY_CONTAINER_NAME || gatewayEnv(env, "NETWORK_PROXY_CONTAINER_NAME") || "raksha_proxy";
	const dockerNetwork = env.DOCKER_NETWORK || gatewayEnv(env, "DOCKER_NETWORK") || "raksha-network";

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
		"process.env.SERVER_DOMAIN": JSON.stringify(serverDomain),
		"process.env.GATEWAY_BRAND_NAME": JSON.stringify(brandName),
		"process.env.GATEWAY_IS_ENTERPRISE": JSON.stringify(isEnterpriseBuild ? "true" : isEnterprise || "false"),
		"process.env.GATEWAY_DISABLE_PROFILER": JSON.stringify(disableProfiler),
		"process.env.GATEWAY_ENTERPRISE_TRIAL_EXPIRY": JSON.stringify(trialExpiry),
		"process.env.GATEWAY_PRODUCT_NAME": JSON.stringify(productName),
		"process.env.GATEWAY_PRODUCT_FULL_NAME": JSON.stringify(productFullName),
		"process.env.GATEWAY_COMPANY_NAME": JSON.stringify(companyName),
		"process.env.GATEWAY_COMPANY_SHORT_NAME": JSON.stringify(companyShortName),
		"process.env.GATEWAY_COMPANY_LOGO": JSON.stringify(companyLogo),
		"process.env.GATEWAY_FOOTER_COPYRIGHT": JSON.stringify(footerCopyright),
		"process.env.GATEWAY_FOOTER_SUBTITLE": JSON.stringify(footerSubtitle),
		"process.env.GATEWAY_PORT": JSON.stringify(backendPort),
		"process.env.APP_PORT": JSON.stringify(backendPort),
		"process.env.APP_HOST": JSON.stringify(appHost),
		"process.env.PROXY_PORT": JSON.stringify(proxyPort),
		"process.env.CONTAINER_NAME": JSON.stringify(containerName),
		"process.env.GATEWAY_PROXY_ADDR": JSON.stringify(gatewayProxyAddr),
		"process.env.PAC_HTTP_PORT": JSON.stringify(pacHttpPort),
		"process.env.MITM_WEB_PORT": JSON.stringify(mitmWebPort),
		"process.env.NETWORK_PROXY_CONTAINER_NAME": JSON.stringify(networkProxyContainerName),
		"process.env.DOCKER_NETWORK": JSON.stringify(dockerNetwork),
		"process.env.GATEWAY_BACKEND_URL": JSON.stringify(backendTarget),
		"process.env.UI_PORT": JSON.stringify(String(uiPort)),
	},
	server: {
		port: uiPort,
		proxy: {
			"/api": {
				target: backendTarget,
				changeOrigin: true,
				secure: false,
			},
			"/v1": {
				target: backendTarget,
				changeOrigin: true,
				secure: false,
			},
			"/ws": {
				target: backendTarget,
				changeOrigin: true,
				secure: false,
				ws: true,
			},
		},
	},
	build: {
		outDir: "out",
		emptyOutDir: true,
	},
	};
});
