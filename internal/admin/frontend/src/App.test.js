import { mount, flushPromises } from "@vue/test-utils";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import App from "./App.vue";
import Registration from "./Registration.vue";
beforeAll(() => {
	HTMLDialogElement.prototype.showModal = function () {
		this.open = true;
	};
});
afterEach(() => {
	vi.unstubAllGlobals();
	location.hash = "";
});
describe("admin application", () => {
	it("connects, navigates and disconnects without persisting the token", async () => {
		const fetch = vi.fn(
			async (url) =>
				new Response(
					JSON.stringify(
						url === "/api/accounts"
							? [{ id: 7, username: "PreviewUser", im: 120, im_bonus: 10 }]
							: url === "/api/sessions"
								? []
								: {
										server_name: "Test",
										connections: 2,
										uptime_seconds: 60,
										services: { login: "127.0.0.1:6414" },
										assets: { npcs: 2, warnings: [] },
										implemented: [],
										pending: [],
									},
					),
				),
		);
		vi.stubGlobal("fetch", fetch);
		const wrapper = mount(App, { global: { stubs: { teleport: true } } });
		await wrapper.get("input[type=password]").setValue("test-only-administrator-token");
		await wrapper.get("form").trigger("submit");
		await flushPromises();
		expect(wrapper.text()).toContain("Native game data");
		expect(wrapper.find("input[type=password]").exists()).toBe(false);
		await wrapper.get('a[href="#accounts"]').trigger("click");
		await flushPromises();
		expect(wrapper.text()).toContain("PreviewUser");
		await wrapper
			.findAll("button")
			.find((b) => b.text() === "Adjust mall points")
			.trigger("click");
		await flushPromises();
		expect(wrapper.find("dialog").exists()).toBe(true);
		await wrapper
			.findAll("button")
			.find((b) => b.attributes("aria-label") === "Close dialog")
			.trigger("click");
		await wrapper
			.findAll("button")
			.find((b) => b.text() === "Disconnect")
			.trigger("click");
		expect(wrapper.text()).toContain("Connect to your server");
		expect(wrapper.text()).not.toContain("PreviewUser");
		expect(localStorage.length).toBe(0);
		expect(sessionStorage.length).toBe(0);
		wrapper.unmount();
	});
	it("retains registration case and validates confirmation before sending", async () => {
		const fetch = vi.fn().mockResolvedValue(new Response("{}", { status: 201 }));
		vi.stubGlobal("fetch", fetch);
		const wrapper = mount(Registration);
		const inputs = wrapper.findAll("input");
		await inputs[0].setValue("Player0001");
		await inputs[1].setValue("Secret1234");
		await inputs[2].setValue("different");
		await wrapper.get("form").trigger("submit");
		expect(fetch).not.toHaveBeenCalled();
		expect(wrapper.text()).toContain("Passwords do not match.");
		await inputs[2].setValue("Secret1234");
		await wrapper.get("form").trigger("submit");
		await flushPromises();
		expect(fetch).toHaveBeenCalledWith(
			"/register",
			expect.objectContaining({
				body: JSON.stringify({ username: "Player0001", password: "Secret1234", email: "" }),
			}),
		);
		expect(wrapper.find("form").exists()).toBe(false);
		wrapper.unmount();
	});
});
