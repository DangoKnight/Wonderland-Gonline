import { afterEach, describe, expect, it, vi } from "vitest";
import { createClientHost } from "./clientHost.js";

function fixture() {
	const callbacks = { onStatus: vi.fn(), onReady: vi.fn(), onError: vi.fn() };
	let rejectRuntime;
	const runtime = {
		importObject: { gojs: {} },
		run: vi.fn(() => new Promise((_, reject) => (rejectRuntime = reject))),
	};
	const environment = {
		AbortController,
		Go: vi.fn(function () {
			return runtime;
		}),
		WebAssembly: { instantiate: vi.fn(async () => ({ instance: "wasm-instance" })) },
		setInterval,
		clearInterval,
		fetch: vi.fn(async (url, options) => {
			if (url === "/packs.json") {
				return { ok: true, json: async () => ["core.zip", "world.zip"] };
			}
			if (options.method === "HEAD") {
				return { ok: true, headers: new Headers({ "Content-Length": "10" }) };
			}
			return { ok: true, arrayBuffer: async () => new ArrayBuffer(4) };
		}),
	};
	const host = createClientHost(callbacks, environment);
	return { host, callbacks, environment, runtime, failRuntime: (error) => rejectRuntime(error) };
}

afterEach(() => vi.useRealTimers());

describe("client startup and range bridge", () => {
	it("only probes pack sizes before starting WASM, then waits for actual readiness", async () => {
		vi.useFakeTimers();
		const { host, environment, callbacks, runtime } = fixture();
		await host.start();
		expect(environment.fetch.mock.calls.map(([url, options]) => [url, options.method])).toEqual(
			[
				["/packs.json", undefined],
				["/assets/core.zip", "HEAD"],
				["/assets/world.zip", "HEAD"],
				["/wonderland-client.wasm", undefined],
			],
		);
		expect(environment.wloPacks["world.zip"]).toEqual({ size: 10 });
		expect(environment.WebAssembly.instantiate).toHaveBeenCalledWith(
			expect.any(ArrayBuffer),
			runtime.importObject,
		);
		expect(runtime.run).toHaveBeenCalledWith("wasm-instance");
		environment.wlo = { state: () => ({ status: "initializing" }) };
		await vi.advanceTimersByTimeAsync(100);
		expect(callbacks.onReady).not.toHaveBeenCalled();
		environment.wlo.state = () => ({ status: "running" });
		await vi.advanceTimersByTimeAsync(500);
		expect(callbacks.onReady).toHaveBeenCalledOnce();
		expect(vi.getTimerCount()).toBe(0);
		host.dispose();
	});

	it("requests exact ranges and rejects invalid, ignored, mismatched or truncated replies", async () => {
		const { host, environment } = fixture();
		await host.start();
		try {
			environment.fetch.mockResolvedValueOnce({
				status: 206,
				headers: new Headers({ "Content-Range": "bytes 2-5/10" }),
				arrayBuffer: async () => new Uint8Array([2, 3, 4, 5]).buffer,
			});
			expect(new Uint8Array(await environment.wloReadBlob("world.zip", 2, 4))).toEqual(
				new Uint8Array([2, 3, 4, 5]),
			);
			expect(environment.fetch).toHaveBeenLastCalledWith(
				"/assets/world.zip",
				expect.objectContaining({ headers: { Range: "bytes=2-5" }, cache: "no-store" }),
			);
			const calls = environment.fetch.mock.calls.length;
			for (const args of [
				["missing.zip", 0, 1],
				["world.zip", -1, 1],
				["world.zip", 9, 2],
				["world.zip", 0.5, 1],
			]) {
				await expect(environment.wloReadBlob(...args)).rejects.toThrow(
					"Invalid asset range",
				);
			}
			expect((await environment.wloReadBlob("world.zip", 10, 0)).byteLength).toBe(0);
			expect(environment.fetch).toHaveBeenCalledTimes(calls);
			for (const response of [
				{ status: 200, headers: new Headers() },
				{ status: 206, headers: new Headers({ "Content-Range": "bytes 0-3/10" }) },
				{
					status: 206,
					headers: new Headers({ "Content-Range": "bytes 2-5/10" }),
					arrayBuffer: async () => new ArrayBuffer(3),
				},
			]) {
				environment.fetch.mockResolvedValueOnce(response);
				await expect(environment.wloReadBlob("world.zip", 2, 4)).rejects.toThrow();
			}
		} finally {
			host.dispose();
		}
	});

	it.each(["manifest", "size", "wasm"])(
		"reports %s failures without starting Go",
		async (stage) => {
			const { host, environment, callbacks, runtime } = fixture();
			if (stage === "manifest") environment.fetch.mockResolvedValueOnce({ ok: false });
			if (stage === "size")
				environment.fetch.mockImplementation(async (url) =>
					url === "/packs.json"
						? { ok: true, json: async () => ["core.zip"] }
						: { ok: true, headers: new Headers({ "Content-Length": "0" }) },
				);
			if (stage === "wasm")
				environment.WebAssembly.instantiate.mockRejectedValue(new Error("Invalid WASM"));
			await host.start();
			expect(callbacks.onError).toHaveBeenCalledOnce();
			expect(runtime.run).not.toHaveBeenCalled();
			host.dispose();
		},
	);

	it("reports a later runtime failure and stops readiness polling", async () => {
		vi.useFakeTimers();
		const { host, callbacks, failRuntime } = fixture();
		await host.start();
		failRuntime(new Error("Runtime stopped"));
		await Promise.resolve();
		expect(callbacks.onError).toHaveBeenCalledWith(
			expect.objectContaining({ message: "Runtime stopped" }),
		);
		expect(vi.getTimerCount()).toBe(0);
		host.dispose();
	});

	it("aborts pending startup requests without updating an unmounted UI", async () => {
		const { host, environment, callbacks } = fixture();
		environment.fetch.mockImplementation(
			(_, { signal }) =>
				new Promise((_, reject) =>
					signal.addEventListener("abort", () => reject(new Error("aborted"))),
				),
		);
		const starting = host.start();
		host.dispose();
		await starting;
		expect(callbacks.onError).not.toHaveBeenCalled();
		expect(callbacks.onReady).not.toHaveBeenCalled();
	});
});
