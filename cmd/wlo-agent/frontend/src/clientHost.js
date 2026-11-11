const partialContentStatus = 206;
const clientReadyPollMilliseconds = 100;

// The Vue application owns the startup UI; Go owns the game canvas and API.
// Keep the existing bridge names so the browser client and automation callers
// work unchanged. Pack bytes are fetched only when Go requests a bounded range.
export function createClientHost(callbacks, environment = window) {
	const controller = new environment.AbortController();
	const fetch = environment.fetch.bind(environment);
	let disposed = false;
	let readinessTimer;

	function stopPolling() {
		if (readinessTimer !== undefined) environment.clearInterval(readinessTimer);
		readinessTimer = undefined;
	}

	function fail(error) {
		if (disposed) return;
		stopPolling();
		callbacks.onError(error instanceof Error ? error : new Error(String(error)));
	}

	function status(message) {
		if (!disposed) callbacks.onStatus(message);
	}

	async function start() {
		try {
			const manifest = await fetch("/packs.json", { signal: controller.signal });
			if (!manifest.ok) throw new Error("Asset manifest unavailable");
			const names = await manifest.json();
			if (!Array.isArray(names) || names.length === 0) {
				throw new Error("Invalid asset manifest");
			}
			const packs = Object.create(null);
			for (const name of names) {
				status("Checking " + name + "...");
				const response = await fetch("/assets/" + name, {
					method: "HEAD",
					signal: controller.signal,
				});
				const size = Number(response.headers.get("Content-Length"));
				if (!response.ok || !Number.isSafeInteger(size) || size <= 0) {
					throw new Error("Invalid asset pack " + name);
				}
				packs[name] = { size };
			}
			if (disposed) return;
			environment.wloPacks = packs;
			environment.wloReadBlob = async (name, offset, length) => {
				const pack = packs[name];
				if (
					!pack ||
					!Number.isSafeInteger(offset) ||
					!Number.isSafeInteger(length) ||
					offset < 0 ||
					length < 0 ||
					offset + length > pack.size
				) {
					throw new Error("Invalid asset range for " + name);
				}
				if (length === 0) return new ArrayBuffer(0);
				const end = offset + length - 1;
				const response = await fetch("/assets/" + name, {
					headers: { Range: "bytes=" + offset + "-" + end },
					cache: "no-store",
					signal: controller.signal,
				});
				if (
					response.status !== partialContentStatus ||
					response.headers.get("Content-Range") !==
						"bytes " + offset + "-" + end + "/" + pack.size
				) {
					throw new Error("Asset server did not honor the requested range for " + name);
				}
				const bytes = await response.arrayBuffer();
				if (bytes.byteLength !== length)
					throw new Error("Truncated asset range for " + name);
				return bytes;
			};

			status("Starting client...");
			const runtime = new environment.Go();
			const response = await fetch("/wonderland-client.wasm", { signal: controller.signal });
			if (!response.ok) throw new Error("Client WebAssembly unavailable");
			const bytes = await response.arrayBuffer();
			const result = await environment.WebAssembly.instantiate(bytes, runtime.importObject);
			if (disposed) return;
			// Go.run remains pending for the lifetime of the client. Observe its
			// failures without blocking Vue's mounted hook or readiness updates.
			Promise.resolve(runtime.run(result.instance)).catch(fail);
			readinessTimer = environment.setInterval(() => {
				try {
					if (environment.wlo?.state?.().status === "running") {
						stopPolling();
						callbacks.onReady();
					}
				} catch (error) {
					fail(error);
				}
			}, clientReadyPollMilliseconds);
		} catch (error) {
			fail(error);
		}
	}

	function dispose() {
		disposed = true;
		controller.abort();
		stopPolling();
	}

	return { start, dispose };
}
