import { h } from "vue";
import { mount } from "@vue/test-utils";
import { describe, it, expect, vi } from "vitest";
import { loadView, titles } from "./views.js";
function fixture(path) {
	if (path === "status")
		return {
			connections: 1,
			assets: { npcs: 2, warnings: [] },
			implemented: ["Login"],
			pending: [],
		};
	if (path === "configuration")
		return { path: "config.json", engine: "sqlite", version: "1", configuration: {} };
	if (path === "settings" || path === "operations")
		return {
			server_name: "Wonderland",
			motd: "Welcome",
			exp_rate: 1,
			drop_rate: 1,
			status_mode: "auto",
			log_level: "info",
		};
	if (path === "characters")
		return [
			{
				account_id: 1,
				version: "1",
				state: { id: 10001, name: "Player", level: 1, map: 12000 },
			},
		];
	if (path === "accounts")
		return [{ id: 1, username: "tester", im: 10, im_bonus: 0, gm_level: 0 }];
	if (path === "sessions")
		return [{ id: 1, username: "tester", character_id: 10001, character_name: "Player" }];
	if (path === "guilds") return [{ id: 1, name: "Guild", leader_id: 10001, members: [10001] }];
	if (path === "battles") return [{ leader_id: 10001, players: [10001] }];
	if (path === "tent-craft")
		return {
			formulas: [
				{
					formula_id: 511,
					item_id: 38027,
					name: "Coconut Basin",
					duration_seconds: 0,
					tent_output: true,
				},
			],
			max_duration_seconds: 604800,
		};
	if (path.startsWith("assets/definitions/"))
		return { version: "1", value: path.endsWith("QuestDefinitions") ? {} : [] };
	return [];
}
describe("feature parity", () => {
	for (const [key, label] of Object.entries(titles))
		it(`renders ${label}`, async () => {
			const api = vi.fn(async (path) => fixture(path));
			const nodes = await loadView(key, {
				api,
				request: vi.fn(),
				notify: vi.fn(),
				refresh: vi.fn(),
				edit: vi.fn(),
			});
			const wrapper = mount({ render: () => h("div", nodes) });
			expect(wrapper.find("section").exists()).toBe(true);
			wrapper.unmount();
		});
	it("retains version and scope when editing character inventory", async () => {
		const current = {
			version: "saved-version",
			state: { id: 10001, name: "Player", bag: [], equipment: [], storage: [], level: 1 },
		};
		let model;
		const request = vi.fn();
		const nodes = await loadView("inventory", {
			api: async (path) =>
				path === "characters" ? [{ account_id: 1, ...current }] : current,
			request,
			notify: vi.fn(),
			refresh: vi.fn(),
			edit: (m) => (model = m),
		});
		const wrapper = mount({ render: () => h("div", nodes) });
		await wrapper
			.findAll("button")
			.find((b) => b.text() === "Inspect / edit")
			.trigger("click");
		await new Promise((r) => setTimeout(r, 0));
		await model.save({ bag: [{ id: 1 }], equipment: [], storage: [] });
		expect(request).toHaveBeenCalledWith(
			"characters/10001",
			expect.objectContaining({
				version: "saved-version",
				scope: "inventory",
				state: expect.objectContaining({ id: 10001, level: 1, bag: [{ id: 1 }] }),
			}),
			"PUT",
		);
		wrapper.unmount();
	});
});
