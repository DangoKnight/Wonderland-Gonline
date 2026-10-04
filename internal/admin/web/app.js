"use strict";
let token = "",
	view = "overview",
	requestID = 0;
const $ = (s) => document.querySelector(s),
	content = $("#content");
const titles = {
	overview: "Overview",
	sessions: "Sessions",
	accounts: "Accounts",
	npcs: "NPC library",
	maps: "World maps",
	catalog: "Item mall catalog",
	settings: "Settings",
};
function element(tag, text, cls) {
	const e = document.createElement(tag);
	if (text !== undefined) e.textContent = text;
	if (cls) e.className = cls;
	return e;
}
async function api(path, options = {}) {
	const r = await fetch("/api/" + path, {
		...options,
		headers: { Authorization: "Bearer " + token, "Content-Type": "application/json" },
	});
	const data = await r.json();
	if (!r.ok) throw Error(data.error || "Request failed");
	return data;
}
function message(text) {
	$("#message").textContent = text;
}
function table(rows, columns, action) {
	const wrapper = element("div", undefined, "panel table-wrap");
	if (!rows.length) {
		wrapper.append(element("p", "No records to show.", "empty"));
		return wrapper;
	}
	const t = element("table"),
		head = element("tr");
	for (const [label] of columns) head.append(element("th", label));
	if (action) head.append(element("th", "Actions"));
	const thead = element("thead");
	thead.append(head);
	t.append(thead);
	const body = element("tbody");
	for (const row of rows) {
		const tr = element("tr");
		for (const [, key] of columns) tr.append(element("td", row[key]));
		if (action) {
			const td = element("td");
			td.append(action(row));
			tr.append(td);
		}
		body.append(tr);
	}
	t.append(body);
	wrapper.append(t);
	return wrapper;
}
async function refresh() {
	if (!token) return false;
	const id = ++requestID,
		target = view;
	message("");
	try {
		if (typeof renderAdminTool === "function" && toolViews.has(target)) {
			await renderAdminTool(target, id);
			return true;
		}
		const path =
			{ overview: "status", npcs: "assets/npcs", maps: "assets/maps" }[target] || target;
		const [data, sessions] = await Promise.all([
			api(path),
			target === "accounts" ? api("sessions") : Promise.resolve([]),
		]);
		if (target === "accounts") {
			const online = new Map(sessions.filter((s) => s.username).map((s) => [s.username, s]));
			for (const row of data) {
				row.session = online.get(row.username);
				row.connection = row.session ? "Online" : "Offline";
			}
		}
		if (id !== requestID) return false;
		content.replaceChildren();
		if (target === "overview") {
			content.append($("#overview").content.cloneNode(true));
			for (const [label, value] of [
				["Connections", data.connections],
				["Uptime", Math.floor(data.uptime_seconds / 60) + " min"],
				["Packets received", data.packets_received],
				["Unported packets", data.unsupported_packets],
			]) {
				const e = element("div", undefined, "metric");
				e.append(element("span", label), element("strong", value));
				$(".metrics").append(e);
			}
			for (const [key, label] of [
				["npcs", "NPC templates"],
				["items", "Item definitions"],
				["skills", "Skills"],
				["dialogues", "Dialogues"],
				["maps", "Maps"],
				["map_npcs", "Map NPCs"],
				["events", "Events"],
				["warps", "Warps"],
				["ground_items", "Ground items"],
				["mall_items", "Mall items"],
				["gacha_packs", "Gacha pools"],
			]) {
				const row = element("div", undefined, "row");
				row.append(element("span", label), element("span", data.assets[key]));
				$("#assets").append(row);
			}
			for (const [label, items] of [
				["Available", data.implemented],
				["Remaining", data.pending],
			]) {
				$("#coverage").append(element("strong", label));
				const ul = element("ul");
				for (const item of items) ul.append(element("li", item));
				$("#coverage").append(ul);
			}
			for (const warning of data.assets.warnings) $("#assets").append(element("p", warning));
		} else if (target === "sessions") {
			content.append(
				table(
					data,
					[
						["ID", "id"],
						["Service", "service"],
						["Address", "remote"],
						["Account", "username"],
						["Character", "character_name"],
						["Map", "map"],
					],
					(row) => {
						const b = element("button", "Terminate session");
						b.onclick = () => terminateSession(row, b);
						return b;
					},
				),
			);
		} else if (target === "accounts") {
			content.append(
				table(
					data,
					[
						["ID", "id"],
						["Account", "username"],
						["Email", "email"],
						["Session", "connection"],
						["Banned", "banned"],
						["GM level", "gm_level"],
						["Mall points", "im"],
						["Bonus mall points", "im_bonus"],
					],
					(row) => {
						const b = element("button", row.banned ? "Unban" : "Ban");
						b.onclick = () =>
							act("accounts/" + row.id + "/ban", { banned: !row.banned });
						const gm = element("button", row.gm_level ? "Revoke GM" : "Grant GM");
						gm.onclick = () =>
							act("accounts/" + row.id + "/gm", { gm_level: row.gm_level ? 0 : 1 });
						const actions = element("div");
						actions.className = "actions";
						const reset = element("button", "Reset password");
						reset.onclick = () => resetPassword(row);
						const remove = element("button", "Delete");
						remove.onclick = async () => {
							if (
								!confirm(
									"Delete account " +
										row.username +
										" and all its characters? This cannot be undone.",
								)
							)
								return;
							remove.disabled = true;
							try {
								await api("accounts/" + row.id, { method: "DELETE" });
								await refresh();
								message("Account deleted.");
							} catch (e) {
								message(e.message);
								remove.disabled = false;
							}
						};
						if (row.session) {
							const terminate = element("button", "Terminate session");
							terminate.onclick = () => terminateSession(row.session, terminate);
							actions.append(terminate);
						}
						const mall = element("button", "Adjust mall points");
						mall.onclick = () => adjustMallPoints(row);
						actions.append(b, gm, mall, reset, remove);
						return actions;
					},
				),
			);
		} else if (target === "npcs") {
			const label = element("label", "Search NPC name or ID");
			const input = element("input");
			input.className = "search";
			input.placeholder = "e.g. Roca or 10001";
			label.append(input);
			content.append(label);
			const results = element("div");
			content.append(results);
			const render = (rows) =>
				results.replaceChildren(
					table(
						rows,
						[
							["ID", "id"],
							["Name", "name"],
							["Level", "level"],
							["HP", "hp"],
							["Element", "element"],
						],
						(row) =>
							toolButton("Inspect", () =>
								jsonEditor(
									"NPC " + row.id,
									row,
									null,
									"NPC template from the current asset catalog.",
								),
							),
					),
				);
			render(data);
			let timer,
				searchID = 0;
			input.oninput = () => {
				clearTimeout(timer);
				const sid = ++searchID;
				timer = setTimeout(async () => {
					try {
						const rows = await api("assets/npcs?q=" + encodeURIComponent(input.value));
						if (sid === searchID && view === "npcs") render(rows);
					} catch (e) {
						message(e.message);
					}
				}, 250);
			};
		} else if (target === "maps") {
			content.append(
				table(data, [
					["Map", "id"],
					["Scene", "scene"],
					["NPCs", "npcs"],
					["Events", "events"],
					["Warps", "warps"],
				]),
			);
		} else if (target === "catalog") {
			content.append(
				table(data, [
					["ID", "item_id"],
					["Item", "item_name"],
					["Category", "category"],
					["Points", "point_cost"],
					["Quantity", "count"],
				]),
			);
		} else if (target === "settings") {
			const panel = element("section", undefined, "panel"),
				form = element("form");
			for (const [key, label] of [
				["server_name", "Server name"],
				["motd", "Message of the day"],
			]) {
				const l = element("label", label),
					input = element(key === "motd" ? "textarea" : "input");
				input.name = key;
				input.value = data[key] || "";
				input.maxLength = key === "motd" ? 1000 : 200;
				if (key === "server_name") input.required = true;
				l.append(input);
				form.append(l);
			}
			form.append(element("button", "Save settings"));
			form.onsubmit = async (e) => {
				e.preventDefault();
				try {
					await api("settings", {
						method: "PUT",
						body: JSON.stringify(Object.fromEntries(new FormData(form))),
					});
					message("Settings saved.");
				} catch (e) {
					message(e.message);
				}
			};
			panel.append(form);
			content.append(panel);
		}
		return true;
	} catch (e) {
		message(e.message);
		return false;
	}
}
async function act(path, body = {}) {
	try {
		await api(path, { method: "POST", body: JSON.stringify(body) });
		await refresh();
	} catch (e) {
		message(e.message);
	}
}
$("#login-form").onsubmit = async (e) => {
	e.preventDefault();
	token = $("#token").value;
	try {
		await api("status");
		$("#token").value = "";
		$("#login").hidden = true;
		content.hidden = false;
		await refresh();
	} catch (e) {
		token = "";
		message(e.message);
	}
};
for (const button of document.querySelectorAll("[data-view]"))
	button.onclick = () => {
		view = button.dataset.view;
		$("#heading").textContent = titles[view];
		for (const b of document.querySelectorAll("[data-view]"))
			b.classList.toggle("active", b === button);
		refresh();
	};
$("#refresh").onclick = refresh;

function resetPassword(row) {
	const dialog = element("dialog", undefined, "panel"),
		form = element("form");
	form.append(element("h2", "Reset password for " + row.username));
	form.append(
		element(
			"p",
			"Use 4–14 printable ASCII characters. Connected sessions will be disconnected.",
		),
	);
	const fields = [];
	for (const title of ["New password", "Confirm password"]) {
		const label = element("label", title),
			input = element("input");
		input.type = "password";
		input.autocomplete = "new-password";
		input.required = true;
		input.minLength = 4;
		input.maxLength = 14;
		input.pattern = "[ -~]{4,14}";
		label.append(input);
		form.append(label);
		fields.push(input);
	}
	const [password, confirmation] = fields;
	const validate = () =>
		confirmation.setCustomValidity(
			password.value === confirmation.value ? "" : "Passwords do not match.",
		);
	password.oninput = confirmation.oninput = validate;
	const error = element("p");
	error.setAttribute("role", "alert");
	form.append(error);
	const buttons = element("div", undefined, "actions"),
		save = element("button", "Reset password"),
		cancel = element("button", "Cancel");
	save.type = "submit";
	cancel.type = "button";
	cancel.onclick = () => dialog.close();
	buttons.append(save, cancel);
	form.append(buttons);
	form.onsubmit = async (e) => {
		e.preventDefault();
		validate();
		if (!form.reportValidity()) return;
		save.disabled = true;
		error.textContent = "";
		try {
			await api("accounts/" + row.id + "/password", {
				method: "POST",
				body: JSON.stringify({ password: password.value }),
			});
			password.value = confirmation.value = "";
			dialog.close();
			await refresh();
			message("Password reset. Connected sessions were disconnected.");
		} catch (e) {
			error.textContent = e.message;
			save.disabled = false;
		}
	};
	dialog.onclose = () => {
		password.value = confirmation.value = "";
		dialog.remove();
	};
	dialog.append(form);
	document.body.append(dialog);
	dialog.showModal();
	password.focus();
}

async function terminateSession(session, button) {
	button.disabled = true;
	try {
		await api("sessions/" + session.id + "/kick", { method: "POST", body: "{}" });
		await refresh();
		message("Session terminated" + (session.username ? " for " + session.username : "") + ".");
	} catch (e) {
		message(e.message);
		button.disabled = false;
	}
}

// Native mall adjustments and balances use signed 32-bit ranges.
const mallAdjustmentMinimum = -2147483648,
	mallAdjustmentMaximum = 2147483647;

function adjustMallPoints(row) {
	const dialog = element("dialog", undefined, "panel"),
		form = element("form");
	form.append(element("h2", "Adjust mall points for " + row.username));
	form.append(
		element(
			"p",
			"Enter a positive amount to add points or a negative amount to deduct. Deductions stop at zero. Each change is recorded.",
		),
	);
	const label = element("label", "Balance"),
		currency = element("select");
	for (const [value, title] of [
		["points", "Mall points (current: " + row.im + ")"],
		["bonus", "Bonus points (current: " + row.im_bonus + ")"],
	]) {
		const option = element("option", title);
		option.value = value;
		currency.append(option);
	}
	label.append(currency);
	form.append(label);
	const amountLabel = element("label", "Amount to add or deduct"),
		amount = element("input");
	amount.type = "number";
	amount.required = true;
	amount.step = "1";
	amount.min = String(mallAdjustmentMinimum);
	amount.max = String(mallAdjustmentMaximum);
	amount.oninput = () =>
		amount.setCustomValidity(Number(amount.value) === 0 ? "Enter a nonzero amount." : "");
	amountLabel.append(amount);
	form.append(amountLabel);
	const error = element("p");
	error.setAttribute("role", "alert");
	form.append(error);
	const buttons = element("div", undefined, "actions"),
		save = element("button", "Apply adjustment"),
		cancel = element("button", "Cancel");
	save.type = "submit";
	cancel.type = "button";
	cancel.onclick = () => dialog.close();
	buttons.append(save, cancel);
	form.append(buttons);
	form.onsubmit = async (e) => {
		e.preventDefault();
		if (save.disabled) return;
		amount.oninput();
		if (!form.reportValidity()) return;
		save.disabled = true;
		cancel.disabled = true;
		error.textContent = "";
		let data;
		try {
			data = await api("accounts/" + row.id + "/mall", {
				method: "POST",
				body: JSON.stringify({ currency: currency.value, delta: Number(amount.value) }),
			});
			dialog.close();
		} catch (e) {
			error.textContent = e.message;
			save.disabled = false;
			cancel.disabled = false;
			return;
		}
		// A refresh failure must not invite another submission of a saved grant.
		if (await refresh()) {
			message(
				"Adjustment saved. Mall points: " +
					data.balances.points +
					"; bonus points: " +
					data.balances.bonus +
					".",
			);
		} else {
			message("Adjustment saved. Refresh the Accounts table to see current balances.");
		}
	};
	dialog.oncancel = (e) => {
		if (save.disabled) e.preventDefault();
	};
	dialog.onclose = () => dialog.remove();
	dialog.append(form);
	document.body.append(dialog);
	dialog.showModal();
	amount.focus();
}
