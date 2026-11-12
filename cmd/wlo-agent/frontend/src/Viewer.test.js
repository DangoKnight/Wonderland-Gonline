import { mount, flushPromises } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Viewer from "./Viewer.vue";
import { createViewerConnection } from "./viewerConnection.js";

vi.mock("./viewerConnection.js", () => ({ createViewerConnection: vi.fn() }));
describe("viewer presentation", () => {
	let connection;
	beforeEach(() => {
		vi.clearAllMocks();
		sessionStorage.clear();
		history.replaceState(null, "", "/viewer/#scoped-ticket");
		connection = { takeover: vi.fn(), release: vi.fn(), input: vi.fn(), close: vi.fn() };
		createViewerConnection.mockReturnValue(connection);
	});
	it("uses the scoped ticket, exposes explicit control, and detaches on close", async () => {
		const wrapper = mount(Viewer);
		expect(location.hash).toBe("");
		expect(sessionStorage.getItem("wlo-viewer-ticket")).toBe("scoped-ticket");
		expect(createViewerConnection.mock.calls[0][1]).toBe("scoped-ticket");
		const status = createViewerConnection.mock.calls[0][3];
		expect(wrapper.findAll("button")[0].attributes("disabled")).toBeDefined();
		status({ connected: true, owns: false, control: { owner: "api", generation: 1 } });
		await flushPromises();
		await wrapper.findAll("button")[0].trigger("click");
		expect(connection.takeover).toHaveBeenCalledOnce();
		status({ connected: true, owns: true, control: { owner: "viewer", generation: 2 } });
		await flushPromises();
		await wrapper.findAll("button")[1].trigger("click");
		expect(connection.release).toHaveBeenCalledOnce();
		await wrapper.get("canvas").trigger("blur");
		expect(connection.input).toHaveBeenCalledWith({ kind: "reset" });
		wrapper.unmount();
		expect(connection.close).toHaveBeenCalledOnce();
	});
});
