import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

const target = process.env.WLO_AGENT_HOST_URL;
const proxy = target
	? Object.fromEntries(
			["/packs.json", "/assets/", "/wonderland-client.wasm", "/ws"].map((path) => [
				path,
				{
					target,
					changeOrigin: true,
					ws: path === "/ws",
					// The loopback Go host validates Host and Origin together.
					headers: { Origin: new URL(target).origin },
				},
			]),
		)
	: undefined;

export default defineConfig({
	root: fileURLToPath(new URL(".", import.meta.url)),
	plugins: [vue(), tailwindcss()],
	build: {
		outDir: "../host",
		emptyOutDir: true,
		// /assets/ belongs to the allowlisted game packs, not frontend files.
		assetsDir: "ui",
		rollupOptions: {
			output: { entryFileNames: "ui/host.js", assetFileNames: "ui/host.[ext]" },
		},
	},
	server: { host: "127.0.0.1", proxy },
	test: {
		environment: "jsdom",
		include: ["cmd/wlo-agent/frontend/**/*.test.js", "src/**/*.test.js"],
	},
});
