import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv } from "vite";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const isEnterpriseBuild = fs.existsSync(path.join(__dirname, "app", "enterprise"));

export default defineConfig(({ mode }) => {
	const rootDir = path.resolve(__dirname, "..");
	const env = loadEnv(mode, rootDir, "");

	const backendPort = env.APP_PORT || env.RAKSHA_PORT || process.env.APP_PORT || process.env.RAKSHA_PORT || "8001";
	const backendTarget = env.RAKSHA_BACKEND_URL || process.env.RAKSHA_BACKEND_URL || `http://localhost:${backendPort}`;
	const uiPort = Number(env.UI_PORT || process.env.UI_PORT || 3000);
	const companyName = env.RAKSHA_COMPANY_NAME || process.env.RAKSHA_COMPANY_NAME || "YesPanchi Group of Companies";
	const companyShortName = env.RAKSHA_COMPANY_SHORT_NAME || process.env.RAKSHA_COMPANY_SHORT_NAME || "YesPanchi";
	const companyLogo = env.RAKSHA_COMPANY_LOGO || process.env.RAKSHA_COMPANY_LOGO || "/yes-panchi-logo.png";
	const productName = env.RAKSHA_PRODUCT_NAME || env.PRODUCT_NAME || process.env.RAKSHA_PRODUCT_NAME || process.env.PRODUCT_NAME || "Raksha";
	const productFullName = env.RAKSHA_PRODUCT_FULL_NAME || env.RAKSHA_PRODUCT_SUBTITLE || process.env.RAKSHA_PRODUCT_FULL_NAME || process.env.RAKSHA_PRODUCT_SUBTITLE || "Real-time AI Knowledge Screening & Hazard Audit";
	const footerCopyright = env.RAKSHA_FOOTER_COPYRIGHT || process.env.RAKSHA_FOOTER_COPYRIGHT || "";
	const footerSubtitle = env.RAKSHA_FOOTER_SUBTITLE || process.env.RAKSHA_FOOTER_SUBTITLE || "Enterprise AI Governance Platform.";

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
		"process.env.RAKSHA_IS_ENTERPRISE": JSON.stringify(isEnterpriseBuild ? "true" : "false"),
		"process.env.RAKSHA_DISABLE_PROFILER": JSON.stringify(process.env.RAKSHA_DISABLE_PROFILER ?? ""),
		"process.env.RAKSHA_ENTERPRISE_TRIAL_EXPIRY": JSON.stringify(process.env.ENTERPRISE_TRIAL_EXPIRY ?? ""),
		"process.env.RAKSHA_PRODUCT_NAME": JSON.stringify(productName),
		"process.env.RAKSHA_PRODUCT_FULL_NAME": JSON.stringify(productFullName),
		"process.env.RAKSHA_COMPANY_NAME": JSON.stringify(companyName),
		"process.env.RAKSHA_COMPANY_SHORT_NAME": JSON.stringify(companyShortName),
		"process.env.RAKSHA_COMPANY_LOGO": JSON.stringify(companyLogo),
		"process.env.RAKSHA_FOOTER_COPYRIGHT": JSON.stringify(footerCopyright),
		"process.env.RAKSHA_FOOTER_SUBTITLE": JSON.stringify(footerSubtitle),
		"process.env.RAKSHA_PORT": JSON.stringify(backendPort),
		"process.env.RAKSHA_BACKEND_URL": JSON.stringify(backendTarget),
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