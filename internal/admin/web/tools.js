"use strict";
const toolTitles = {
	operations: "Server operations",
	characters: "Characters",
	studio: "GM studio",
	inventory: "Inventory & equipment",
	stats: "Stats & skills",
	player_settings: "Player settings",
	friends: "Friendships",
	guilds: "Guilds",
	marriages: "Marriages",
	mail: "GM mail & gifts",
	bans: "IP bans",
	battles: "Live battles",
	portals: "Portals & destinations",
	drops: "Monster drops",
	chest: "Chest loot",
	starters: "Starter items",
	talks: "Dialogue resolver",
	assets_editor: "SQL asset editor",
	configuration: "Startup configuration",
	logs: "Live logs",
	audit: "Audit history",
	catalog: "Item mall catalog",
};
Object.assign(titles, toolTitles);
const toolViews = new Set(Object.keys(toolTitles));
function toolPanel(title, note) {
	const panel = element("section", undefined, "panel");
	panel.append(element("h2", title));
	if (note) panel.append(element("p", note));
	return panel;
}
function toolButton(label, run, danger = false) {
	const b = element("button", label, danger ? "danger" : "");
	b.type = "button";
	b.onclick = async () => {
		b.disabled = true;
		try {
			const result = await run();
			if (result?.messages?.length) message(result.messages.join("\n"));
		} catch (e) {
			message(e.message);
		} finally {
			b.disabled = false;
		}
	};
	return b;
}
function toolField(form, label, key, value, type = "text", choices) {
	const wrap = element("label", label),
		input = element(choices ? "select" : type === "textarea" ? "textarea" : "input");
	input.name = key;
	if (choices) {
		for (const choice of choices) {
			const o = element("option", choice);
			o.value = choice;
			input.append(o);
		}
	} else if (type !== "textarea") {
		input.type = type;
		if (type === "number") input.step = "any";
	}
	input.value = value ?? "";
	wrap.append(input);
	form.append(wrap);
	return input;
}
function toolForm(title, fields, submit, note) {
	const panel = toolPanel(title, note),
		form = element("form", undefined, "tool-form");
	for (const [label, key, value, type, choices] of fields)
		toolField(form, label, key, value, type, choices);
	const save = element("button", "Apply");
	form.append(save);
	form.onsubmit = async (e) => {
		e.preventDefault();
		save.disabled = true;
		try {
			const values = Object.fromEntries(new FormData(form));
			const result = await submit(values);
			message(result?.messages?.join("\n") || "Saved.");
		} catch (e) {
			message(e.message);
		} finally {
			save.disabled = false;
		}
	};
	panel.append(form);
	return panel;
}
function jsonEditor(
	title,
	value,
	save,
	note = "Changes save to the database. Refresh after another administrator changes this record.",
) {
	const dialog = element("dialog", undefined, "panel editor-dialog"),
		form = element("form"),
		input = element("textarea", undefined, "json-editor"),
		error = element("p");
	input.value = JSON.stringify(value, null, 2);
	input.spellcheck = false;
	input.readOnly = !save;
	input.setAttribute("aria-label", title);
	error.setAttribute("role", "alert");
	form.append(element("h2", title), element("p", note), input, error);
	const actions = element("div", undefined, "actions"),
		ok = element("button", "Save changes"),
		cancel = toolButton("Cancel", () => dialog.close());
	if (save) actions.append(ok);
	else cancel.textContent = "Close";
	actions.append(cancel);
	form.append(actions);
	form.onsubmit = async (e) => {
		e.preventDefault();
		ok.disabled = true;
		try {
			await save(JSON.parse(input.value));
			dialog.close();
			await refresh();
			message("Changes saved.");
		} catch (e) {
			error.textContent = e.message;
			ok.disabled = false;
		}
	};
	dialog.append(form);
	dialog.onclose = () => dialog.remove();
	document.body.append(dialog);
	dialog.showModal();
}
async function toolRequest(path, body, method = "POST") {
	return api(path, { method, body: JSON.stringify(body) });
}
async function playerAction(id, command, args = []) {
	return toolRequest("players/" + id + "/action", { command, args });
}
function playerSelect(players) {
	const label = element("label", "Online character"),
		select = element("select");
	select.append(element("option", "Select a character"));
	select.firstChild.value = "";
	for (const player of players.filter((p) => p.character_id)) {
		const option = element("option", player.character_name + " (#" + player.character_id + ")");
		option.value = player.character_id;
		select.append(option);
	}
	label.append(select);
	return [label, select];
}
async function renderAdminTool(target, id) {
	const paths = {
		operations: "operations",
		characters: "characters",
		inventory: "characters",
		stats: "characters",
		player_settings: "characters",
		studio: "sessions",
		friends: "friends",
		guilds: "guilds",
		marriages: "marriages",
		mail: "mail",
		bans: "bans",
		battles: "battles",
		portals: "assets/maps",
		talks: "assets/talks",
		assets_editor: "assets/documents",
		configuration: "configuration",
		logs: "logs",
		audit: "audit",
	};
	const data = paths[target] ? await api(paths[target]) : null;
	if (id !== requestID) return;
	content.replaceChildren();
	if (target === "operations") renderOperations(data);
	else if (["characters", "inventory", "stats", "player_settings"].includes(target))
		renderCharacters(data, target);
	else if (target === "studio") renderStudio(data);
	else if (["catalog", "starters", "chest", "drops"].includes(target))
		await renderAssetTable(target, id);
	else if (target === "portals") renderMaps(data);
	else if (target === "talks") renderTalks(data);
	else if (target === "assets_editor") renderAssetDirectory(data);
	else if (target === "configuration") renderConfiguration(data);
	else if (target === "friends")
		content.append(
			table(
				data,
				[
					["Character", "character1"],
					["Friend", "character2"],
				],
				(r) =>
					toolButton(
						"Remove friendship",
						async () => {
							if (!confirm("Remove this friendship?")) return;
							await toolRequest(
								"friends/" + r.character1 + "/" + r.character2,
								{},
								"DELETE",
							);
							await refresh();
						},
						true,
					),
			),
		);
	else if (target === "battles") renderBattles(data);
	else if (target === "bans") renderBans(data);
	else if (target === "mail") await renderMail(data, id);
	else if (target === "guilds") renderGuilds(data);
	else if (target === "marriages") renderMarriages(data);
	else if (target === "audit")
		content.append(
			table(data, [
				["Time", "at"],
				["Action", "action"],
				["Subject", "subject"],
			]),
		);
	else if (target === "logs") renderLogs(data);
}
function renderOperations(data) {
	content.append(
		toolForm(
			"Live server settings",
			[
				["Server name", "server_name", data.server_name],
				["Message of the day", "motd", data.motd, "textarea"],
				["EXP multiplier (0.01–1000)", "exp_rate", data.exp_rate, "number"],
				["Drop multiplier (0.1–100)", "drop_rate", data.drop_rate, "number"],
				[
					"Launcher traffic status",
					"status_mode",
					data.status_mode,
					"text",
					["auto", "green", "yellow", "red"],
				],
				[
					"Log level",
					"log_level",
					data.log_level,
					"text",
					["debug", "info", "warn", "error"],
				],
			],
			async (v) => {
				v.exp_rate = Number(v.exp_rate);
				v.drop_rate = Number(v.drop_rate);
				return toolRequest("operations", v, "PUT");
			},
			"Rates, traffic status and logging settings persist across restarts.",
		),
	);
	content.append(
		toolForm("Broadcast announcement", [["Message", "text", "", "textarea"]], (v) =>
			toolRequest("operations/broadcast", v),
		),
	);
	const maintenance = toolPanel("Maintenance"),
		buttons = element("div", undefined, "actions");
	for (const scope of ["quests", "mall", "drops", "gms", "all"])
		buttons.append(
			toolButton("Reload " + scope, () => toolRequest("operations/reload", { scope })),
		);
	buttons.append(toolButton("Save all characters", () => toolRequest("operations/save", {})));
	buttons.append(
		toolButton(
			"Disconnect all sessions",
			() => {
				if (confirm("Disconnect every connected session?"))
					return toolRequest("operations/kickall", {});
			},
			true,
		),
	);
	maintenance.append(buttons);
	content.append(maintenance);
	content.append(
		toolForm(
			"Schedule shutdown",
			[["Delay in seconds (1–300)", "seconds", 10, "number"]],
			(v) => {
				if (!confirm("Shut down the server after " + v.seconds + " seconds?")) return;
				return toolRequest("operations/shutdown", { seconds: Number(v.seconds) });
			},
			"The server announces the countdown, disconnects sessions and saves pending character changes.",
		),
	);
}
function renderCharacters(rows, target) {
	const panel = toolPanel("Find characters"),
		form = element("form"),
		search = toolField(form, "Name or exact character ID", "q", ""),
		button = element("button", "Search");
	form.append(button);
	panel.append(form);
	content.append(panel);
	const results = element("div");
	content.append(results);
	const render = (records) => {
		results.replaceChildren(
			table(
				records.map((r) => ({
					...r,
					id: r.state.id,
					name: r.state.name,
					level: r.state.level,
					map: r.state.map,
				})),
				[
					["ID", "id"],
					["Name", "name"],
					["Account", "account_id"],
					["Level", "level"],
					["Map", "map"],
				],
				(row) => {
					const actions = element("div", undefined, "actions");
					actions.append(
						toolButton("Inspect / edit", async () => {
							const current = await api("characters/" + row.id);
							openCharacter(current, target);
						}),
					);
					if (target === "characters")
						actions.append(
							toolButton(
								"Delete",
								async () => {
									if (!confirm("Delete character " + row.name + "?")) return;
									await toolRequest(
										"characters/" + row.id,
										{ version: row.version },
										"DELETE",
									);
									await refresh();
								},
								true,
							),
						);
					return actions;
				},
			),
		);
		if (records.length === 500)
			results.append(
				toolButton("Next 500", async () =>
					render(
						await api(
							"characters?after=" +
								records.at(-1).state.id +
								"&q=" +
								encodeURIComponent(search.value),
						),
					),
				),
			);
	};
	form.onsubmit = async (e) => {
		e.preventDefault();
		try {
			render(await api("characters?q=" + encodeURIComponent(search.value)));
		} catch (e) {
			message(e.message);
		}
	};
	render(rows);
}
function openCharacter(row, target) {
	const selected =
		target === "inventory"
			? { bag: row.state.bag, equipment: row.state.equipment, storage: row.state.storage }
			: target === "stats"
				? {
						base: row.state.base,
						skills: row.state.skills,
						level: row.state.level,
						exp: row.state.exp,
						stat_points: row.state.stat_points,
					}
				: target === "player_settings"
					? {
							settings: row.state.settings ?? {
								pk_allowed: true,
								join_allowed: true,
								trade_allowed: true,
								channels: 31,
							},
						}
					: row.state;
	jsonEditor(
		"Edit " + row.state.name,
		selected,
		async (value) => {
			const next = target === "characters" ? value : { ...row.state, ...value };
			await toolRequest(
				"characters/" + row.state.id,
				{ version: row.version, state: next },
				"PUT",
			);
		},
		"Identity fields stay fixed. An online character must be idle and will reconnect after a complete state edit. Every saved field uses the same format shown here.",
	);
}
function renderStudio(sessions) {
	const panel = toolPanel("Online player tools"),
		[label, select] = playerSelect(sessions);
	panel.append(label);
	const run = (command, args = []) => {
		if (!select.value) throw Error("Select an online character.");
		return playerAction(Number(select.value), command, args);
	};
	const quick = element("div", undefined, "actions");
	for (const [label, command, args] of [
		["Full heal + companion", "fullheal", []],
		["+10,000 gold", "gold", ["10000"]],
		["+100 stat points", "points", ["100"]],
		["All skills grade 10", "allskills", ["10"]],
		["God attributes", "god", []],
		["Max pet amity", "amity", ["100"]],
		["Pet rebirth", "rebirth", []],
		["Reset attributes", "restat", []],
		["Clear skills", "clearskills", []],
		["Repair items", "repair", []],
		["Hide / unhide", "invis", []],
		["Force battle victory", "winbattle", []],
		["Summon all here", "summonall", []],
		["Clear ground drops", "clearground", []],
		["Inspect character", "info", []],
	])
		quick.append(toolButton(label, () => run(command, args)));
	panel.append(quick);
	content.append(panel);
	for (const [title, fields, command, make] of [
		["Set level", [["Level", "level", 1, "number"]], "level", (v) => [v.level]],
		[
			"Give item",
			[
				["Item ID", "item", 32176, "number"],
				["Count", "count", 1, "number"],
			],
			"item",
			(v) => [v.item, v.count],
		],
		[
			"Delete bag stack",
			[["Bag slot (1–50)", "slot", 1, "number"]],
			"deleteitem",
			(v) => [v.slot],
		],
		[
			"Recruit companion",
			[
				["NPC template ID", "pet", 14156, "number"],
				["Optional name", "name", ""],
			],
			"pet",
			(v) => [v.pet, ...(v.name ? [v.name] : [])],
		],
		["Pet level", [["Level", "level", 1, "number"]], "petlvl", (v) => [v.level]],
		["Pet EXP", [["EXP gain", "exp", 100, "number"]], "petexp", (v) => [v.exp]],
		[
			"Warp to coordinates",
			[
				["Map ID", "map", 10017, "number"],
				["X", "x", 600, "number"],
				["Y", "y", 600, "number"],
			],
			"warp",
			(v) => [v.map, v.x, v.y],
		],
		[
			"Warp to character",
			[["Character ID or exact name", "target", ""]],
			"warp",
			(v) => [v.target],
		],
		[
			"Summon character here",
			[["Character ID or exact name", "target", ""]],
			"summon",
			(v) => [v.target],
		],
		[
			"Mall point adjustment",
			[["Points to add (negative removes)", "amount", 100, "number"]],
			"mallpoints",
			(v) => [v.amount],
		],
		["Dismount", [], "unride", () => []],
		[
			"NPC visibility / prop",
			[
				["Click ID", "click", 1, "number"],
				["Action", "action", "show", "text", ["show", "hide", "open", "close"]],
			],
			"actor",
			(v) => [v.click, v.action],
		],
		["Named town warp", [["Town alias", "town", "kelan"]], "town", (v) => [v.town]],
		[
			"Start test battle",
			[["Monster template ID", "monster", 100, "number"]],
			"battle",
			(v) => [v.monster],
		],
		[
			"Trigger map NPC event",
			[["Click ID on the character's current map", "click", 1, "number"]],
			"event",
			(v) => [v.click],
		],
	])
		content.append(toolForm(title, fields, (v) => run(command, make(v))));
	content.append(
		toolForm(
			"Mute or jail",
			[
				["Operation", "operation", "mute", "text", ["mute", "unmute", "jail", "unjail"]],
				["Minutes", "minutes", 10, "number"],
			],
			(v) => {
				if (!select.value) throw Error("Select a character.");
				const args = [select.value];
				if (v.operation === "mute" || v.operation === "jail") args.push(v.minutes);
				return run(v.operation, args);
			},
		),
	);
	const danger = toolPanel("Inventory and connection"),
		actions = element("div", undefined, "actions");
	actions.append(
		toolButton(
			"Clear inventory",
			() => {
				if (confirm("Empty this character's bag?")) return run("clearinv");
			},
			true,
		),
		toolButton("Kick character", () => run("kick", ["Disconnected by administrator."]), true),
	);
	danger.append(actions);
	content.append(danger);
}
async function editableAsset(asset) {
	return api("assets/documents/" + encodeURIComponent(asset));
}
async function saveAsset(asset, doc) {
	return toolRequest("assets/documents/" + encodeURIComponent(asset), doc, "PUT");
}
async function renderAssetTable(target, id) {
	const asset = {
		catalog: "item_mall.json",
		starters: "starter_items.json",
		chest: "chest_drops.json",
		drops: "monster_drops.txt",
	}[target];
	let doc;
	try {
		doc = await editableAsset(asset);
	} catch (e) {
		if (target !== "chest") throw e;
		const panel = toolPanel(
			"Chest pools",
			"No custom chest pools have been configured. Authored map rewards remain active.",
		);
		panel.append(
			toolButton("Create chest pool table", async () => {
				await toolRequest("assets/documents/" + asset + "/initialize", {});
				await refresh();
			}),
		);
		if (id === requestID) content.append(panel);
		return;
	}
	if (id !== requestID) return;
	const panel = toolPanel(
		toolTitles[target],
		"Saving validates the complete SQL catalog and reconnects idle characters so their asset snapshots refresh.",
	);
	panel.append(
		toolButton("Edit table JSON", () =>
			jsonEditor(toolTitles[target], doc.value.value ?? doc.value.text, (value) =>
				saveAsset(asset, {
					version: doc.version,
					value: { ...doc.value, [target === "drops" ? "text" : "value"]: value },
				}),
			),
		),
	);
	if (target === "catalog") {
		const rows = doc.value.value ?? [];
		panel.append(
			toolButton("Add item", () =>
				jsonEditor(
					"Add mall item",
					{
						item_id: 0,
						item_name: "",
						category_id: 1,
						category: "",
						point_cost: 0,
						count: 1,
						order_idx: 1,
					},
					(value) =>
						saveAsset(asset, {
							version: doc.version,
							value: { ...doc.value, value: [...rows, value] },
						}),
				),
			),
		);
		content.append(panel);
		content.append(
			table(
				rows,
				[
					["ID", "item_id"],
					["Name", "item_name"],
					["Category", "category_id"],
					["Points", "point_cost"],
					["Quantity", "count"],
					["Order", "order_idx"],
				],
				(row) => {
					const actions = element("div", undefined, "actions"),
						index = rows.indexOf(row);
					actions.append(
						toolButton("Edit", () =>
							jsonEditor("Edit mall item", row, (value) =>
								saveAsset(asset, {
									version: doc.version,
									value: {
										...doc.value,
										value: rows.map((r, i) => (i === index ? value : r)),
									},
								}),
							),
						),
					);
					for (const [label, delta] of [
						["Up", -1],
						["Down", 1],
					])
						actions.append(
							toolButton(label, async () => {
								const group = rows
									.map((r, i) => ({ r, i }))
									.filter(
										({ r }) =>
											r.category_id === row.category_id &&
											(r.is_bonus || 0) === (row.is_bonus || 0),
									);
								const position = group.findIndex(({ i }) => i === index);
								const neighbor = group[position + delta];
								if (!neighbor) return;
								const other = neighbor.i;
								const next = structuredClone(rows);
								[next[index], next[other]] = [next[other], next[index]];
								group.forEach(({ i }, rank) => {
									next[i].order_idx = rank + 1;
								});
								await saveAsset(asset, {
									version: doc.version,
									value: { ...doc.value, value: next },
								});
								await refresh();
							}),
						);
					actions.append(
						toolButton(
							"Delete",
							async () => {
								if (!confirm("Delete this mall item?")) return;
								await saveAsset(asset, {
									version: doc.version,
									value: {
										...doc.value,
										value: rows.filter((r, i) => i !== index),
									},
								});
								await refresh();
							},
							true,
						),
					);
					return actions;
				},
			),
		);
	} else if (target === "drops") {
		content.append(panel);
		const pre = element("pre", doc.value.text ?? "");
		pre.className = "asset-preview";
		content.append(pre);
		content.append(
			element(
				"p",
				"Rows use TID:<monster ID> | <item ID>,<name>,<min>,<max>,<percentage>. Add, change or remove rows in the table editor.",
			),
		);
	} else {
		content.append(panel);
		const pre = element("pre", JSON.stringify(doc.value.value, null, 2));
		pre.className = "asset-preview";
		content.append(pre);
		if (target === "chest")
			panel.append(
				toolButton("Add map pool", () =>
					jsonEditor(
						"New chest pool",
						{
							map_id: 10017,
							category: "",
							respawn_seconds: 60,
							rewards: [{ item_id: 32176, name: "", count: 1, weight: 100 }],
						},
						(value) =>
							saveAsset(asset, {
								version: doc.version,
								value: { ...doc.value, value: [...(doc.value.value ?? []), value] },
							}),
					),
				),
			);
	}
}
function renderMaps(rows) {
	const panel = toolPanel("Map NPCs, events, portals and destinations"),
		form = element("form"),
		input = toolField(form, "Map ID", "id", 10017, "number"),
		button = element("button", "Inspect map");
	form.append(button);
	panel.append(form);
	content.append(panel);
	const detail = element("div");
	content.append(detail);
	const inspect = async (id) => {
		const data = await api("assets/maps/" + id);
		detail.replaceChildren();
		const p = toolPanel("Map " + id);
		p.append(
			toolButton("Edit portals and map data", async () => {
				try {
					await editableAsset("map_overrides.json");
				} catch {
					await toolRequest("assets/documents/map_overrides.json/initialize", {});
				}
				const doc = await editableAsset("map_overrides.json");
				jsonEditor("Map " + id, data, (value) => {
					const rows = (doc.value.value ?? []).filter((m) => m.id !== data.id);
					return saveAsset("map_overrides.json", {
						version: doc.version,
						value: { ...doc.value, value: [...rows, value] },
					});
				});
			}),
		);
		detail.append(p);
		const pre = element("pre", JSON.stringify(data, null, 2));
		pre.className = "asset-preview";
		detail.append(pre);
	};
	form.onsubmit = async (e) => {
		e.preventDefault();
		try {
			await inspect(input.value);
		} catch (e) {
			message(e.message);
		}
	};
	content.append(
		table(
			rows,
			[
				["Map", "id"],
				["Scene", "scene"],
				["NPCs", "npcs"],
				["Events", "events"],
				["Portals", "warps"],
			],
			(row) => toolButton("Inspect", () => inspect(row.id)),
		),
	);
}
function renderTalks(rows) {
	const results = element("div"),
		panel = toolForm(
			"Resolve or search dialogue",
			[
				["Talk ID, byte offset, record index or text", "q", ""],
				["Player name for #n", "name", ""],
			],
			async (v) => {
				const data = await api(
					"assets/talks?q=" +
						encodeURIComponent(v.q) +
						"&name=" +
						encodeURIComponent(v.name),
				);
				render(data);
			},
		);
	content.append(panel, results);
	const render = (data) =>
		results.replaceChildren(
			table(
				data,
				[
					["ID / offset", "id"],
					["Lookup", "method"],
					["Dialogue", "text"],
				],
				(row) =>
					toolButton("Send to player", async () => {
						const target = prompt("Online character ID");
						if (target) return playerAction(Number(target), "dialogue", [row.text]);
					}),
			),
		);
	render(rows);
}
function renderAssetDirectory(rows) {
	const panel = toolPanel(
		"Authoritative SQL assets",
		"Use document editors for server tables and a record lookup for native NPC, skill and dialogue fields.",
	);
	panel.append(
		toolForm(
			"Find a native record",
			[
				[
					"Asset",
					"asset",
					"npc.dat",
					"text",
					["npc.dat", "skill.dat", "talk.dat", "mark.dat", "item.dat"],
				],
				["Game ID", "id", 10001, "number"],
			],
			async (v) => {
				const records = await api(
					"assets/documents/" +
						encodeURIComponent(v.asset) +
						"/records?id=" +
						encodeURIComponent(v.id),
				);
				if (!records.length) throw Error("No indexed record found.");
				for (const record of records)
					jsonEditor(v.asset + " #" + v.id, record.value, (value) =>
						toolRequest(
							"assets/documents/" +
								encodeURIComponent(v.asset) +
								"/records/" +
								record.ordinal +
								"?collection=" +
								encodeURIComponent(record.collection),
							{ version: record.version, value },
							"PUT",
						),
					);
			},
		),
	);
	content.append(panel);
	content.append(
		table(
			rows,
			[
				["Asset", "asset"],
				["Origin", "origin"],
				["Source", "source"],
			],
			(row) =>
				toolButton("Edit document", async () => {
					const doc = await editableAsset(row.asset);
					jsonEditor(row.asset, doc.value, (value) =>
						saveAsset(row.asset, { version: doc.version, value }),
					);
				}),
		),
	);
}
function renderConfiguration(data) {
	const panel = toolPanel(
		"Startup configuration",
		"Changes apply after restarting with " +
			data.path +
			". Database engine: " +
			data.engine +
			".",
	);
	panel.append(
		toolButton("Edit startup configuration", () =>
			jsonEditor(
				"Startup configuration",
				data.configuration,
				(value) =>
					toolRequest(
						"configuration",
						{ version: data.version, configuration: value },
						"PUT",
					),
				"Listener addresses, database paths, connection limits and idle timers apply on the next start.",
			),
		),
	);
	const pre = element("pre", JSON.stringify(data.configuration, null, 2));
	panel.append(pre);
	content.append(panel);
}
function renderBattles(rows) {
	content.append(
		table(
			rows.map((r) => ({ ...r, party: r.players.join(", ") })),
			[
				["Leader", "leader_id"],
				["Party", "party"],
				["Turn", "turn"],
				["Animating", "processing"],
			],
			(row) => {
				const actions = element("div", undefined, "actions");
				actions.append(
					toolButton("Inspect", () =>
						jsonEditor(
							"Battle " + row.leader_id,
							{ attackers: row.attackers, defenders: row.defenders },
							null,
							"Live fighter snapshot.",
						),
					),
					toolButton("Force victory", () => playerAction(row.leader_id, "winbattle")),
					toolButton(
						"Abort",
						() => toolRequest("battles/" + row.leader_id + "/abort", {}),
						true,
					),
				);
				return actions;
			},
		),
	);
}
function renderBans(rows) {
	content.append(
		toolForm(
			"Ban IP address",
			[
				["IPv4 or IPv6 address", "ip", ""],
				["Reason", "reason", ""],
			],
			async (v) => {
				await toolRequest("bans", v);
				await refresh();
			},
		),
	);
	content.append(
		table(
			rows,
			[
				["IP", "ip"],
				["Reason", "reason"],
				["Time", "at"],
			],
			(row) =>
				toolButton("Unban", async () => {
					await toolRequest("bans", { ip: row.ip, reason: "" }, "DELETE");
					await refresh();
				}),
		),
	);
}
async function renderMail(rows, id) {
	const characters = await api("characters");
	if (id !== requestID) return;
	content.append(
		toolForm(
			"Send GM mail and gifts",
			[
				["Recipient mode", "mode", "single", "text", ["single", "online", "all"]],
				["Character ID for single recipient", "recipient", "", "number"],
				["Subject", "subject", "Server gift"],
				["Message", "body", "", "textarea"],
				["Gold", "gold", 0, "number"],
				["Item ID (0 for none)", "item_id", 0, "number"],
				["Quantity (0 for none)", "count", 0, "number"],
			],
			async (v) => {
				let receivers;
				if (v.mode === "single") receivers = [Number(v.recipient)];
				else if (v.mode === "online")
					receivers = (await api("sessions"))
						.filter((p) => p.character_id)
						.map((p) => p.character_id);
				else {
					if (characters.length === 500)
						throw Error("Use explicit recipient batches for more than 500 characters.");
					receivers = characters.map((c) => c.state.id);
				}
				if (!confirm("Send this mail and gift to " + receivers.length + " character(s)?"))
					return;
				await toolRequest("mail", {
					receivers,
					message: {
						subject: v.subject,
						body: v.body,
						gold: Number(v.gold),
						item_id: Number(v.item_id),
						count: Number(v.count),
					},
				});
				await refresh();
			},
			"Gifts are saved with the mail, then claimed once when the recipient can receive them. Full inventories retain the gift for a later login.",
		),
	);
	content.append(
		table(
			rows,
			[
				["ID", "id"],
				["Recipient", "receiver_id"],
				["Subject", "subject"],
				["Gold", "gold"],
				["Item", "item_id"],
				["Count", "count"],
				["Gift claimed", "claimed"],
				["Notice delivered", "delivered"],
			],
			(row) =>
				toolButton(
					"Delete",
					async () => {
						if (confirm("Delete this mail? An already claimed gift is preserved.")) {
							await toolRequest("mail/" + row.id, {}, "DELETE");
							await refresh();
						}
					},
					true,
				),
		),
	);
}
function renderGuilds(rows) {
	const panel = toolPanel(
		"Guild administration",
		"Edit announcements, the leader and membership together. A leader must remain in the member list.",
	);
	panel.append(
		toolButton("Create guild", () =>
			jsonEditor(
				"New guild",
				{ id: 0, name: "", notice: "", leader_id: 0, members: [] },
				(value) => toolRequest("guilds", value, "PUT"),
			),
		),
	);
	content.append(
		panel,
		table(
			rows.map((r) => ({ ...r, member_count: r.members.length })),
			[
				["ID", "id"],
				["Name", "name"],
				["Leader", "leader_id"],
				["Members", "member_count"],
				["Announcement", "notice"],
			],
			(row) => {
				const actions = element("div", undefined, "actions");
				actions.append(
					toolButton("Edit / roster", () =>
						jsonEditor(
							row.name,
							{
								id: row.id,
								name: row.name,
								notice: row.notice,
								leader_id: row.leader_id,
								members: row.members,
							},
							(value) => toolRequest("guilds", value, "PUT"),
						),
					),
					toolButton(
						"Disband",
						async () => {
							if (confirm("Disband " + row.name + "?")) {
								await toolRequest("guilds/" + row.id, {}, "DELETE");
								await refresh();
							}
						},
						true,
					),
				);
				return actions;
			},
		),
	);
}
function renderMarriages(rows) {
	content.append(
		table(
			rows,
			[
				["ID", "id"],
				["Character", "character1"],
				["Spouse", "character2"],
				["Date", "at"],
			],
			(row) => {
				const actions = element("div", undefined, "actions");
				actions.append(
					toolButton("Bring spouses together", () =>
						toolRequest("marriages/" + row.id + "/summon", {}),
					),
					toolButton(
						"Annul",
						async () => {
							if (confirm("Annul this marriage record?")) {
								await toolRequest("marriages/" + row.id, {}, "DELETE");
								await refresh();
							}
						},
						true,
					),
				);
				return actions;
			},
		),
	);
}
function renderLogs(rows) {
	const panel = toolPanel(
			"Recent server logs",
			"The newest 500 entries are retained in memory. Change logging detail in Server operations.",
		),
		input = element("input");
	input.placeholder = "Filter message, level or fields";
	input.setAttribute("aria-label", "Filter server logs");
	panel.append(input);
	content.append(panel);
	const results = element("div");
	content.append(results);
	const render = () => {
		const filtered = rows
			.filter((r) => JSON.stringify(r).toLowerCase().includes(input.value.toLowerCase()))
			.reverse()
			.map((r) => ({ ...r, fields: JSON.stringify(r.fields) }));
		results.replaceChildren(
			table(filtered, [
				["Time", "at"],
				["Level", "level"],
				["Message", "message"],
				["Fields", "fields"],
			]),
		);
	};
	input.oninput = render;
	render();
}
