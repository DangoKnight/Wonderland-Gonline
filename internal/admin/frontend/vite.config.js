import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

export default defineConfig({
	root: fileURLToPath(new URL(".", import.meta.url)),
	plugins: [vue(), tailwindcss()],
	build: {
		outDir: "../web",
		emptyOutDir: true,
		assetsDir: "assets",
		// Fixed entry names let the Go embedding tests verify the distributable.
		rollupOptions: {
			output: { entryFileNames: "assets/admin.js", assetFileNames: "assets/admin.[ext]" },
		},
	},
	server: {
		proxy: {
			"/api": "http://127.0.0.1:8080",
			"/healthz": "http://127.0.0.1:8080",
			"/register": {
				target: "http://127.0.0.1:8080",
				bypass(req) {
					if (req.method === "GET") return "/index.html";
				},
			},
		},
	},
	test: {
		environment: "jsdom",
		include: ["internal/admin/frontend/**/*.test.js", "src/**/*.test.js"],
	},
});
