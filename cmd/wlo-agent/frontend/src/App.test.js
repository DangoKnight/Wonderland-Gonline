import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.vue";
import { createClientHost } from "./clientHost.js";

vi.mock("./clientHost.js", () => ({ createClientHost: vi.fn() }));

describe("agent host UI", () => {
	let lifecycle;
	beforeEach(() => {
		vi.clearAllMocks();
		lifecycle = { start: vi.fn().mockResolvedValue(), dispose: vi.fn() };
		createClientHost.mockReturnValue(lifecycle);
	});

	it("shows startup progress and removes the overlay when the client is ready", async () => {
		const wrapper = mount(App);
		await flushPromises();
		expect(lifecycle.start).toHaveBeenCalledOnce();
		expect(wrapper.get('[role="status"]').text()).toBe("Loading assets...");
		const callbacks = createClientHost.mock.calls[0][0];
		callbacks.onStatus("Checking world.zip...");
		await flushPromises();
		expect(wrapper.text()).toBe("Checking world.zip...");
		callbacks.onReady();
		await flushPromises();
		expect(wrapper.find("#status").exists()).toBe(false);
		wrapper.unmount();
		expect(lifecycle.dispose).toHaveBeenCalledOnce();
	});

	it("shows startup or runtime errors as escaped text even after readiness", async () => {
		const wrapper = mount(App);
		const callbacks = createClientHost.mock.calls[0][0];
		callbacks.onReady();
		callbacks.onError(new Error("<script>failure</script>"));
		await flushPromises();
		expect(wrapper.get('[role="alert"]').text()).toBe("<script>failure</script>");
		expect(wrapper.find("script").exists()).toBe(false);
		wrapper.unmount();
	});
});
