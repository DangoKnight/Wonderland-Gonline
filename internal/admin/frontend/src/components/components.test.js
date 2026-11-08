import { mount, flushPromises } from "@vue/test-utils";
import { describe, it, expect, vi, beforeAll } from "vitest";
import RecordTable from "./RecordTable.vue";
import ActionForm from "./ActionForm.vue";
import EditDialog from "./EditDialog.vue";
beforeAll(() => {
	HTMLDialogElement.prototype.showModal = function () {
		this.open = true;
	};
});
describe("administration controls", () => {
	it("filters, sorts and pages records without losing the selected row action", async () => {
		const rows = Array.from({ length: 60 }, (_, i) => ({ id: i + 1, name: `Player ${i + 1}` }));
		const action = vi.fn();
		const wrapper = mount(RecordTable, {
			props: {
				rows,
				columns: [
					["ID", "id"],
					["Name", "name"],
				],
				actions: () => [{ label: "Inspect", run: action }],
			},
		});
		expect(wrapper.findAll("tbody tr")).toHaveLength(25);
		await wrapper
			.findAll("button")
			.find((b) => b.text() === "Next")
			.trigger("click");
		expect(wrapper.find("tbody").text()).toContain("Player 26");
		await wrapper.get("input").setValue("Player 60");
		expect(wrapper.findAll("tbody tr")).toHaveLength(1);
		await wrapper
			.findAll("button")
			.find((b) => b.text() === "Inspect")
			.trigger("click");
		expect(action).toHaveBeenCalledWith(rows[59]);
		wrapper.unmount();
	});
	it("prevents duplicate form submissions while a write is pending", async () => {
		let resolve;
		const submit = vi.fn(() => new Promise((r) => (resolve = r)));
		const wrapper = mount(ActionForm, {
			props: { title: "Grant", fields: [["Amount", "delta", 1, "number"]], submit },
		});
		await wrapper.get("form").trigger("submit");
		await wrapper.get("form").trigger("submit");
		expect(submit).toHaveBeenCalledOnce();
		expect(wrapper.get("button").attributes("disabled")).toBeDefined();
		resolve({});
		await flushPromises();
		expect(wrapper.get("button").attributes("disabled")).toBeUndefined();
		wrapper.unmount();
	});
	it("keeps a conflicting editor open and closes only after a successful write", async () => {
		const save = vi
			.fn()
			.mockRejectedValueOnce(Error("version conflict"))
			.mockResolvedValueOnce({});
		const wrapper = mount(EditDialog, {
			props: { model: { title: "Definition", value: { id: 1 }, save } },
			global: { stubs: { teleport: true } },
			attachTo: document.body,
		});
		await flushPromises();
		await wrapper.get("form").trigger("submit");
		await flushPromises();
		expect(wrapper.text()).toContain("version conflict");
		expect(wrapper.emitted("saved")).toBeUndefined();
		await wrapper.get("form").trigger("submit");
		await flushPromises();
		expect(wrapper.emitted("saved")).toHaveLength(1);
		expect(wrapper.emitted("close")).toHaveLength(1);
		wrapper.unmount();
	});
});
