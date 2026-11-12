import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createViewerConnection } from "./viewerConnection.js";

describe("detachable viewer", () => {
	let sockets;
	beforeEach(() => {
		vi.useFakeTimers();
		sockets = [];
		vi.stubGlobal(
			"WebSocket",
			class {
				readyState = 1;
				sent = [];
				constructor() {
					sockets.push(this);
				}
				send(data) {
					this.sent.push(JSON.parse(data));
				}
				close() {
					this.readyState = 3;
					this.onclose?.();
				}
			},
		);
	});
	afterEach(() => {
		vi.unstubAllGlobals();
		vi.useRealTimers();
	});
	it("shows a server policy refusal without reconnecting", () => {
		const status = vi.fn();
		const viewer = createViewerConnection(
			"ws://localhost/viewer/ws",
			"ticket",
			vi.fn(),
			status,
		);
		sockets[0].onopen();
		sockets[0].onmessage({
			data: JSON.stringify({ error: "Server explicitly disallows agentic use" }),
		});
		expect(status).toHaveBeenLastCalledWith({
			connected: false,
			owns: false,
			error: "Server explicitly disallows agentic use",
		});
		vi.advanceTimersByTime(5000);
		expect(sockets).toHaveLength(1);
		viewer.close();
	});
	it("starts readonly, fences inputs by generation, and reconnects readonly", () => {
		const status = vi.fn(),
			viewer = createViewerConnection("ws://localhost/viewer/ws", "secret", vi.fn(), status);
		const first = sockets[0];
		first.onopen();
		viewer.input({ kind: "down" });
		expect(first.sent).toEqual([{ ticket: "secret" }]);
		first.onmessage({
			data: JSON.stringify({ viewer: "a", control: { owner: "api", generation: 1 } }),
		});
		viewer.takeover();
		expect(first.sent.at(-1)).toEqual({ operation: "takeover", generation: 1 });
		first.onmessage({
			data: JSON.stringify({
				viewer: "a",
				control: { owner: "viewer", viewer: "a", generation: 2 },
			}),
		});
		viewer.input({ kind: "key", key: 37, down: true });
		expect(first.sent.at(-1)).toMatchObject({ generation: 2, sequence: 1, operation: "input" });
		first.close();
		vi.advanceTimersByTime(1000);
		const second = sockets[1];
		second.onopen();
		viewer.input({ kind: "key", key: 37, down: true });
		expect(second.sent).toEqual([{ ticket: "secret" }]);
		second.onmessage({
			data: JSON.stringify({
				viewer: "b",
				control: { owner: "viewer", viewer: "a", generation: 2 },
			}),
		});
		viewer.release();
		expect(second.sent).toEqual([{ ticket: "secret" }]);
		viewer.close();
		vi.advanceTimersByTime(5000);
		expect(sockets).toHaveLength(2);
	});
});
