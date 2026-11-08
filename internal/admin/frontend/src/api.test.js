import { describe, it, expect, vi, afterEach } from "vitest";
import { createAPI } from "./api.js";
afterEach(() => vi.unstubAllGlobals());
describe("admin API transport", () => {
	it("uses the in-memory token and preserves the request body", async () => {
		const fetch = vi.fn().mockResolvedValue(new Response('{"ok":true}'));
		vi.stubGlobal("fetch", fetch);
		await createAPI(() => "test-token")("accounts/7/mall", {
			method: "POST",
			body: '{"delta":10}',
		});
		expect(fetch).toHaveBeenCalledWith(
			"/api/accounts/7/mall",
			expect.objectContaining({
				method: "POST",
				body: '{"delta":10}',
				headers: { Authorization: "Bearer test-token", "Content-Type": "application/json" },
			}),
		);
		expect(localStorage.length).toBe(0);
	});
	it("invalidates authorization on 401 and surfaces server errors", async () => {
		vi.stubGlobal(
			"fetch",
			vi
				.fn()
				.mockResolvedValue(
					new Response('{"error":"administrator token required"}', { status: 401 }),
				),
		);
		const expired = vi.fn();
		await expect(createAPI(() => "bad", expired)("accounts")).rejects.toThrow(
			"administrator token required",
		);
		expect(expired).toHaveBeenCalledOnce();
	});
	it("rejects non-JSON failure pages without rendering HTML", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(new Response("<script>bad()</script>", { status: 502 })),
		);
		await expect(createAPI(() => "")("status")).rejects.toThrow(
			"Invalid server response (502)",
		);
	});
});
