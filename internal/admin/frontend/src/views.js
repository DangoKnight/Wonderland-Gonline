import { h, shallowRef } from "vue";
import ActionForm from "./components/ActionForm.vue";
import ActionButton from "./components/ActionButton.vue";
import RecordTable from "./components/RecordTable.vue";
import SearchRecords from "./components/SearchRecords.vue";

export const navigation = [
	[
		"General",
		[
			["overview", "Overview"],
			["sessions", "Sessions"],
			["accounts", "Accounts"],
			["configuration", "Startup configuration"],
		],
	],
	[
		"Players",
		[
			["characters", "Characters"],
			["player_settings", "Player settings"],
			["stats", "Stats & skills"],
			["inventory", "Inventory & equipment"],
			["character_quests", "Character quests"],
			["friends", "Friendships"],
			["guilds", "Guilds"],
			["marriages", "Marriages"],
			["mail", "GM mail & gifts"],
			["bans", "IP bans"],
		],
	],
	[
		"Game data",
		[
			["npcs", "NPC library"],
			["maps", "World maps"],
			["portals", "Portals & destinations"],
			["catalog", "Item mall catalog"],
			["drops", "Monster drops"],
			["chest", "Chest loot"],
			["tent", "Tent craft"],
			["starters", "Starter items"],
			["quest_definitions", "Quest definitions"],
			["talks", "Dialogue resolver"],
			["assets_editor", "SQL asset editor"],
		],
	],
	[
		"Operations",
		[
			["operations", "Server operations"],
			["studio", "GM studio"],
			["battles", "Live battles"],
			["settings", "Settings"],
			["logs", "Live logs"],
			["audit", "Audit history"],
		],
	],
];
export const titles = Object.fromEntries(navigation.flatMap(([, items]) => items));
const characterViews = ["characters", "inventory", "stats", "player_settings", "character_quests"];
const assetViews = {
	catalog: "Mall",
	starters: "StarterItems",
	chest: "ChestPools",
	drops: "Drops",
	quest_definitions: "QuestDefinitions",
};
const paths = {
	npcs: "assets/npcs",
	maps: "assets/maps",
	portals: "assets/maps",
	studio: "sessions",
	talks: "assets/talks",
	tent: "tent-craft",
	assets_editor: "assets/definitions",
	inventory: "characters",
	stats: "characters",
	player_settings: "characters",
	character_quests: "characters",
};
const columns = {
	sessions: [
		["ID", "id"],
		["Service", "service"],
		["Address", "remote"],
		["Account", "username"],
		["Character", "character_name"],
		["Map", "map"],
	],
	accounts: [
		["ID", "id"],
		["Account", "username"],
		["Email", "email"],
		["Session", "connection"],
		["Banned", "banned"],
		["GM level", "gm_level"],
		["Mall points", "im"],
		["Bonus", "im_bonus"],
	],
	characters: [
		["ID", "id"],
		["Name", "name"],
		["Account", "account_id"],
		["Level", "level"],
		["Map", "map"],
	],
	npcs: [
		["ID", "id"],
		["Name", "name"],
		["Level", "level"],
		["HP", "hp"],
		["Element", "element"],
	],
	maps: [
		["Map", "id"],
		["Scene", "scene"],
		["NPCs", "npcs"],
		["Events", "events"],
		["Portals", "warps"],
	],
	friends: [
		["Character", "character1"],
		["Friend", "character2"],
	],
	guilds: [
		["ID", "id"],
		["Name", "name"],
		["Leader", "leader_id"],
		["Members", "member_count"],
		["Announcement", "notice"],
	],
	marriages: [
		["ID", "id"],
		["Character", "character1"],
		["Spouse", "character2"],
		["Date", "at"],
	],
	bans: [
		["IP", "ip"],
		["Reason", "reason"],
		["Time", "at"],
	],
	mail: [
		["ID", "id"],
		["Recipient", "receiver_id"],
		["Subject", "subject"],
		["Gold", "gold"],
		["Item", "item_id"],
		["Count", "count"],
		["Gift claimed", "claimed"],
		["Notice delivered", "delivered"],
	],
	battles: [
		["Leader", "leader_id"],
		["Party", "party"],
		["Turn", "turn"],
		["Animating", "processing"],
	],
	audit: [
		["Time", "at"],
		["Action", "action"],
		["Subject", "subject"],
	],
	logs: [
		["Time", "at"],
		["Level", "level"],
		["Message", "message"],
		["Fields", "fields"],
	],
	catalog: [
		["ID", "item_id"],
		["Name", "item_name"],
		["Category", "category_id"],
		["Points", "point_cost"],
		["Quantity", "count"],
		["Order", "order_idx"],
	],
	tent: [
		["Formula", "formula_id"],
		["Output", "name"],
		["Item ID", "item_id"],
		["Tent", "tent"],
		["Seconds", "duration_seconds"],
	],
};
export async function loadView(target, ctx) {
	const { api, request, notify, refresh, edit } = ctx;
	const button = (label, run, danger = false) => ({ label, run, danger });
	const actions = (list) =>
		h(
			"div",
			{ class: "flex flex-wrap gap-2" },
			list.map((a) => h(ActionButton, { action: a, notify })),
		);
	const section = (title, note, children = []) =>
		h("section", { class: "panel" }, [
			h("h2", title),
			note ? h("p", { class: "muted mb-5" }, note) : null,
			...children,
		]);
	const form = (title, fields, submit, note) =>
		h(ActionForm, { title, fields, submit, notify, note });
	const table = (rows, cols, rowActions, title) =>
		h(RecordTable, { rows: rows ?? [], columns: cols, actions: rowActions, notify, title });
	const inspect = (title, value, save, note) => edit({ title, value, save, note });
	const confirmAction = (text, run) => async () => {
		if (confirm(text)) return run();
		return false;
	};
	const mutation = async (path, body = {}, method = "POST") => {
		await request(path, body, method);
		await refresh();
	};
	const asset = (name) => api("assets/definitions/" + encodeURIComponent(name));
	const saveAsset = (name, doc) =>
		request("assets/definitions/" + encodeURIComponent(name), doc, "PUT");
	const playerAction = (id, command, args = []) =>
		request(`players/${id}/action`, { command, args });
	const dynamic = (model) => h({ setup: () => () => model.value });

	if (assetViews[target]) {
		const name = assetViews[target];
		let doc;
		try {
			doc = await asset(name);
		} catch (e) {
			if (target !== "chest") throw e;
			return [
				section(
					"Chest pools",
					"No custom chest pools configured. Authored map rewards remain active.",
					[
						actions([
							button("Create chest pool table", () =>
								mutation(`assets/definitions/${name}/initialize`),
							),
						]),
					],
				),
			];
		}
		const save = (value) => saveAsset(name, { version: doc.version, value });
		const buttons = [
			button("Edit table JSON", () =>
				inspect(
					titles[target],
					doc.value,
					save,
					"The complete SQL catalog is validated on save. Idle characters reconnect to refresh their assets.",
				),
			),
		];
		if (target === "catalog")
			buttons.push(
				button("Add item", () =>
					inspect(
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
						(v) => save([...(doc.value ?? []), v]),
					),
				),
			);
		if (target === "chest")
			buttons.push(
				button("Add map pool", () =>
					inspect(
						"New chest pool",
						{
							map_id: 10017,
							category: "",
							respawn_seconds: 60,
							rewards: [{ item_id: 32176, name: "", count: 1, weight: 100 }],
						},
						(v) => save([...(doc.value ?? []), v]),
					),
				),
			);
		const head = section(
			titles[target],
			"Version-checked definitions stored in the assets database.",
			[actions(buttons)],
		);
		if (target !== "catalog")
			return [
				head,
				section("Current definitions", null, [
					h("pre", JSON.stringify(doc.value ?? {}, null, 2)),
				]),
			];
		const rows = doc.value ?? [];
		const move = async (row, delta) => {
			const next = structuredClone(rows),
				index = rows.indexOf(row);
			const group = rows
				.map((r, i) => ({ r, i }))
				.filter(
					({ r }) =>
						r.category_id === row.category_id &&
						(r.is_bonus || 0) === (row.is_bonus || 0),
				);
			const neighbor = group[group.findIndex(({ i }) => i === index) + delta];
			if (!neighbor) return;
			[next[index], next[neighbor.i]] = [next[neighbor.i], next[index]];
			group.forEach(({ i }, rank) => (next[i].order_idx = rank + 1));
			await save(next);
			await refresh();
		};
		return [
			head,
			table(rows, columns.catalog, (row) => [
				button("Edit", () =>
					inspect("Edit mall item", row, (v) =>
						save(rows.map((r) => (r === row ? v : r))),
					),
				),
				button("Up", () => move(row, -1)),
				button("Down", () => move(row, 1)),
				button(
					"Delete",
					confirmAction("Delete this mall item?", async () => {
						await save(rows.filter((r) => r !== row));
						await refresh();
					}),
					true,
				),
			]),
		];
	}
	if (target === "tent") {
		const data = await api("tent-craft");
		const rows = (data.formulas ?? []).map((r) => ({
			...r,
			tent: r.tent_output ? "yes" : "",
		}));
		const max = data.max_duration_seconds ?? 0;
		return [
			section(
				"Tent craft",
				"Recipe durations come from the SQL manufacturing catalog. Zero finishes the craft immediately; existing jobs keep their deadline.",
				[
					actions([
						button("Set every duration to 0", () =>
							mutation("tent-craft", { all: true, duration_seconds: 0 }),
						),
						button("Return all to default values", () =>
							mutation("tent-craft", { reset: true }),
						),
					]),
					table(rows, columns.tent, (row) => [
						button("Edit duration", () =>
							edit({
								title: `Formula ${row.formula_id} · ${row.name || "unnamed"}`,
								note: `Zero finishes the craft immediately. Maximum ${max} seconds. Existing jobs keep their deadline.`,
								fields: [
									[
										"Seconds",
										"duration_seconds",
										row.duration_seconds,
										"number",
										undefined,
										{ min: 0, max },
									],
								],
								save: async (value) => {
									await request("tent-craft", {
										ids: [row.formula_id],
										duration_seconds: Math.max(
											0,
											Math.trunc(Number(value.duration_seconds) || 0),
										),
									});
								},
							}),
						),
					]),
				],
			),
		];
	}
	let data = await api(paths[target] ?? (target === "overview" ? "status" : target));
	data ??= [];
	if (target === "overview")
		return [
			h(
				"div",
				{ class: "grid gap-4 sm:grid-cols-2 xl:grid-cols-4" },
				[
					["Connections", data.connections],
					["Uptime", `${Math.floor((data.uptime_seconds ?? 0) / 60)} min`],
					["Packets received", data.packets_received],
					["Unported packets", data.unsupported_packets],
				].map(([label, value]) =>
					h("section", { class: "panel metric" }, [
						h("p", { class: "muted" }, label),
						h("strong", String(value ?? 0)),
					]),
				),
			),
			h("div", { class: "grid gap-6 xl:grid-cols-2" }, [
				section("Native game data", "Loaded from the assets database.", [
					h(
						"dl",
						{ class: "grid grid-cols-2 gap-x-6 gap-y-3" },
						Object.entries(data.assets ?? {})
							.filter(([key, value]) => typeof value === "number")
							.flatMap(([key, value]) => [
								h("dt", { class: "muted" }, key.replaceAll("_", " ")),
								h("dd", { class: "text-right font-mono" }, value.toLocaleString()),
							]),
					),
					...(data.assets?.warnings ?? []).map((w) =>
						h("p", { class: "warning mt-4" }, w),
					),
				]),
				section(
					"Port coverage",
					null,
					["implemented", "pending"].map((key) =>
						h("div", { class: "mb-5" }, [
							h("h3", key === "implemented" ? "Available" : "Remaining"),
							h(
								"ul",
								{ class: "list-disc pl-5 muted" },
								(data[key] ?? []).map((v) => h("li", v)),
							),
						]),
					),
				),
			]),
		];
	if (target === "sessions")
		return [
			table(data, columns.sessions, (r) => [
				button("Terminate session", () => mutation(`sessions/${r.id}/kick`), true),
			]),
		];
	if (target === "accounts") {
		const online = new Map(
			(await api("sessions")).filter((s) => s.username).map((s) => [s.username, s]),
		);
		return [
			table(
				data.map((r) => ({
					...r,
					session: online.get(r.username),
					connection: online.has(r.username) ? "Online" : "Offline",
				})),
				columns.accounts,
				(r) => [
					button(
						r.banned ? "Unban" : "Ban",
						() => mutation(`accounts/${r.id}/ban`, { banned: !r.banned }),
						!r.banned,
					),
					button(r.gm_level ? "Revoke GM" : "Grant GM", () =>
						mutation(`accounts/${r.id}/gm`, { gm_level: r.gm_level ? 0 : 1 }),
					),
					button("Adjust mall points", () =>
						edit({
							title: `Adjust mall points · ${r.username}`,
							note: "Positive adds points; negative deducts, stopping at zero. Every adjustment is recorded.",
							fields: [
								[
									"Balance",
									"currency",
									"points",
									"text",
									[
										{ value: "points", label: `Mall points (${r.im})` },
										{ value: "bonus", label: `Bonus points (${r.im_bonus})` },
									],
								],
								[
									"Amount",
									"delta",
									1,
									"number",
									null,
									{ required: true, min: -2147483648, max: 2147483647, step: 1 },
								],
							],
							save: async (v) => {
								const delta = Number(v.delta);
								if (!Number.isInteger(delta) || delta === 0)
									throw Error("Enter a nonzero whole number.");
								await request(`accounts/${r.id}/mall`, {
									currency: v.currency,
									delta,
								});
							},
						}),
					),
					button("Reset password", () =>
						edit({
							title: `Reset password · ${r.username}`,
							note: "Connected sessions will be disconnected.",
							fields: [
								[
									"New password",
									"password",
									"",
									"password",
									null,
									{
										required: true,
										minlength: 4,
										maxlength: 14,
										pattern: "[ -~]{4,14}",
										autocomplete: "new-password",
									},
								],
								[
									"Confirm password",
									"confirmation",
									"",
									"password",
									null,
									{ required: true, autocomplete: "new-password" },
								],
							],
							save: async (v) => {
								if (v.password !== v.confirmation)
									throw Error("Passwords do not match.");
								await request(`accounts/${r.id}/password`, {
									password: v.password,
								});
							},
						}),
					),
					...(r.session
						? [
								button(
									"Terminate session",
									() => mutation(`sessions/${r.session.id}/kick`),
									true,
								),
							]
						: []),
					button(
						"Delete",
						confirmAction(
							`Delete account ${r.username} and all its characters? This cannot be undone.`,
							() => mutation(`accounts/${r.id}`, {}, "DELETE"),
						),
						true,
					),
				],
			),
		];
	}
	if (characterViews.includes(target)) {
		const flatten = (rows) =>
			rows.map((r) => ({
				...r,
				...{ id: r.state.id, name: r.state.name, level: r.state.level, map: r.state.map },
			}));
		const open = async (row) => {
			const current = await api(`characters/${row.id}`),
				state = current.state;
			const keys = {
				inventory: ["bag", "equipment", "storage"],
				stats: ["base", "skills", "level", "exp", "stat_points"],
				player_settings: ["settings"],
				character_quests: ["quests"],
			}[target];
			const selected = keys
				? Object.fromEntries(
						keys.map((k) => [
							k,
							state[k] ??
								(k === "settings"
									? {
											pk_allowed: true,
											join_allowed: true,
											trade_allowed: true,
											channels: 31,
										}
									: undefined),
						]),
					)
				: state;
			inspect(
				`Edit ${state.name}`,
				selected,
				(value) =>
					request(
						`characters/${state.id}`,
						{
							version: current.version,
							state: keys ? { ...state, ...value } : value,
							scope:
								target === "characters"
									? ""
									: target === "character_quests"
										? "quests"
										: target,
						},
						"PUT",
					),
				"Identity stays fixed. Online characters must be idle. Refresh after another administrator changes this record.",
			);
		};
		return [
			h(SearchRecords, {
				initial: flatten(data),
				title: titles[target],
				columns: columns.characters,
				notify,
				load: async (q) => flatten(await api("characters?q=" + encodeURIComponent(q))),
				more: async (q, last) =>
					flatten(await api(`characters?after=${last.id}&q=${encodeURIComponent(q)}`)),
				actions: (r) => [
					button("Inspect / edit", () => open(r)),
					...(target === "characters"
						? [
								button(
									"Delete",
									confirmAction(`Delete character ${r.name}?`, () =>
										mutation(
											`characters/${r.id}`,
											{ version: r.version },
											"DELETE",
										),
									),
									true,
								),
							]
						: []),
				],
			}),
		];
	}
	if (target === "npcs")
		return [
			h(SearchRecords, {
				initial: data,
				title: "NPC library",
				columns: columns.npcs,
				notify,
				load: (q) => api("assets/npcs?q=" + encodeURIComponent(q)),
				actions: (r) => [button("Inspect", () => inspect("NPC " + r.id, r))],
			}),
		];
	if (target === "maps") return [table(data, columns.maps)];
	if (target === "configuration")
		return [
			section(
				"Startup configuration",
				`Applies after restarting with ${data.path}. Database engine: ${data.engine}.`,
				[
					actions([
						button("Edit configuration", () =>
							inspect("Startup configuration", data.configuration, (v) =>
								request(
									"configuration",
									{ version: data.version, configuration: v },
									"PUT",
								),
							),
						),
					]),
					h("pre", JSON.stringify(data.configuration, null, 2)),
				],
			),
		];
	if (target === "settings")
		return [
			form(
				"Server settings",
				[
					[
						"Server name",
						"server_name",
						data.server_name,
						"text",
						null,
						{ required: true, maxlength: 200 },
					],
					[
						"Message of the day",
						"motd",
						data.motd,
						"textarea",
						null,
						{ maxlength: 1000 },
					],
				],
				(v) => request("settings", v, "PUT"),
			),
		];
	if (target === "operations")
		return [
			form(
				"Live server settings",
				[
					["Server name", "server_name", data.server_name],
					["Message of the day", "motd", data.motd, "textarea"],
					[
						"Agent / gameplay API access",
						"agent_api_enabled",
						data.agent_api_enabled ? "enabled" : "disabled",
						"text",
						["disabled", "enabled"],
					],
					[
						"EXP multiplier",
						"exp_rate",
						data.exp_rate,
						"number",
						null,
						{ min: 0.01, max: 1000 },
					],
					[
						"Drop multiplier",
						"drop_rate",
						data.drop_rate,
						"number",
						null,
						{ min: 0.1, max: 100 },
					],
					[
						"Launcher traffic status",
						"status_mode",
						data.status_mode,
						"text",
						["auto", "green", "yellow", "red", "offline"],
					],
					[
						"Log level",
						"log_level",
						data.log_level,
						"text",
						["debug", "info", "warn", "error"],
					],
				],
				(v) =>
					request(
						"operations",
						{
							...v,
							agent_api_enabled: v.agent_api_enabled === "enabled",
							exp_rate: Number(v.exp_rate),
							drop_rate: Number(v.drop_rate),
						},
						"PUT",
					),
				"Settings persist across restarts. Agent/API access is disabled by default; disabling it disconnects declared agent sessions immediately.",
			),
			form(
				"Broadcast announcement",
				[
					["Message", "text", "", "textarea"],
					[
						"Channel / color",
						"channel",
						"notice",
						"text",
						["notice", "world", "guild", "whisper", "banner"],
					],
				],
				(v) => request("operations/broadcast", v),
			),
			section("Maintenance", null, [
				actions([
					...["quests", "mall", "drops", "gms", "all"].map((scope) =>
						button("Reload " + scope, () => request("operations/reload", { scope })),
					),
					button(
						"Give starter packs",
						confirmAction(
							"Grant the configured starter pack to every loaded idle character?",
							async () =>
								inspect(
									"Starter pack delivery",
									await request("operations/starter-packs", {}),
								),
						),
					),
					button("Save all characters", () => request("operations/save", {})),
					button(
						"Disconnect all sessions",
						confirmAction("Disconnect every connected session?", () =>
							mutation("operations/kickall"),
						),
						true,
					),
				]),
			]),
			form(
				"Schedule shutdown",
				[
					[
						"Delay in seconds",
						"seconds",
						10,
						"number",
						null,
						{ min: 1, max: 300, required: true },
					],
				],
				(v) =>
					confirmAction(`Shut down the server after ${v.seconds} seconds?`, () =>
						request("operations/shutdown", { seconds: Number(v.seconds) }),
					)(),
				"The server announces the countdown, disconnects sessions and saves pending changes.",
			),
		];
	if (target === "studio") {
		const selected = shallowRef("");
		const run = (command, args = []) => {
			if (!selected.value) throw Error("Select an online character.");
			return playerAction(Number(selected.value), command, args.map(String));
		};
		const choose = h("label", [
			h("span", "Online character"),
			h(
				"select",
				{ value: selected.value, onChange: (e) => (selected.value = e.target.value) },
				[
					h("option", { value: "" }, "Select a character"),
					...data
						.filter((p) => p.character_id)
						.map((p) =>
							h(
								"option",
								{ value: p.character_id },
								`${p.character_name} (#${p.character_id})`,
							),
						),
				],
			),
		]);
		const quick = [
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
		];
		const definitions = [
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
				[["Click ID", "click", 1, "number"]],
				"event",
				(v) => [v.click],
			],
		];
		return [
			section("Online player tools", null, [
				choose,
				actions(
					quick.map(([label, command, args]) => button(label, () => run(command, args))),
				),
			]),
			...definitions.map(([title, fields, command, make]) =>
				form(title, fields, (v) => run(command, make(v))),
			),
			form(
				"Mute or jail",
				[
					[
						"Operation",
						"operation",
						"mute",
						"text",
						["mute", "unmute", "jail", "unjail"],
					],
					["Minutes", "minutes", 10, "number"],
				],
				(v) =>
					run(v.operation, [
						selected.value,
						...(["mute", "jail"].includes(v.operation) ? [v.minutes] : []),
					]),
			),
			section("Inventory and connection", null, [
				actions([
					button(
						"Clear inventory",
						confirmAction("Empty this character's bag?", () => run("clearinv")),
						true,
					),
					button(
						"Kick character",
						() => run("kick", ["Disconnected by administrator."]),
						true,
					),
				]),
			]),
		];
	}
	if (target === "friends")
		return [
			table(data, columns.friends, (r) => [
				button(
					"Remove friendship",
					confirmAction("Remove this friendship?", () =>
						mutation(`friends/${r.character1}/${r.character2}`, {}, "DELETE"),
					),
					true,
				),
			]),
		];
	if (target === "guilds")
		return [
			section(
				"Guild administration",
				"Leader must remain in the member list; existing member roles are preserved.",
				[
					actions([
						button("Create guild", () =>
							inspect(
								"New guild",
								{ id: 0, name: "", notice: "", leader_id: 0, members: [] },
								(v) => request("guilds", v, "PUT"),
							),
						),
					]),
				],
			),
			table(
				data.map((r) => ({ ...r, member_count: r.members.length })),
				columns.guilds,
				(r) => [
					button("Edit / roster", () =>
						inspect(
							r.name,
							{
								id: r.id,
								name: r.name,
								notice: r.notice,
								icon: r.icon,
								leader_id: r.leader_id,
								members: r.members,
							},
							(v) => request("guilds", v, "PUT"),
						),
					),
					button(
						"Disband",
						confirmAction(`Disband ${r.name}?`, () =>
							mutation(`guilds/${r.id}`, {}, "DELETE"),
						),
						true,
					),
				],
			),
		];
	if (target === "marriages")
		return [
			table(data, columns.marriages, (r) => [
				button("Bring spouses together", () => request(`marriages/${r.id}/summon`, {})),
				button(
					"Annul",
					confirmAction("Annul this marriage record?", () =>
						mutation(`marriages/${r.id}`, {}, "DELETE"),
					),
					true,
				),
			]),
		];
	if (target === "bans")
		return [
			form(
				"Ban IP address",
				[
					["IPv4 or IPv6 address", "ip", ""],
					["Reason", "reason", ""],
				],
				(v) => mutation("bans", v),
			),
			table(data, columns.bans, (r) => [
				button("Unban", () => mutation("bans", { ip: r.ip, reason: "" }, "DELETE")),
			]),
		];
	if (target === "battles")
		return [
			table(
				data.map((r) => ({ ...r, party: r.players.join(", ") })),
				columns.battles,
				(r) => [
					button("Inspect", () =>
						inspect("Battle " + r.leader_id, {
							attackers: r.attackers,
							defenders: r.defenders,
						}),
					),
					button("Force victory", () => playerAction(r.leader_id, "winbattle")),
					button(
						"Abort",
						confirmAction("Abort this battle?", () =>
							mutation(`battles/${r.leader_id}/abort`),
						),
						true,
					),
				],
			),
		];
	if (target === "mail") {
		const characters = await api("characters");
		return [
			form(
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
							throw Error(
								"Use explicit recipient batches for more than 500 characters.",
							);
						receivers = characters.map((c) => c.state.id);
					}
					if (!confirm(`Send this mail and gift to ${receivers.length} character(s)?`))
						return false;
					return mutation("mail", {
						receivers,
						message: {
							subject: v.subject,
							body: v.body,
							gold: Number(v.gold),
							item_id: Number(v.item_id),
							count: Number(v.count),
						},
					});
				},
				"Gifts are saved with mail and claimed once. Full inventories retain gifts for a later login.",
			),
			table(data, columns.mail, (r) => [
				button(
					"Delete",
					confirmAction("Delete this mail? Already claimed gifts are preserved.", () =>
						mutation(`mail/${r.id}`, {}, "DELETE"),
					),
					true,
				),
			]),
		];
	}
	if (target === "portals") {
		const detail = shallowRef(null);
		const inspectMap = async (id) => {
			const [map, actors] = await Promise.all([
				api(`assets/maps/${id}`),
				api(`assets/maps/${id}/actors`),
			]);
			detail.value = section("Map " + id, null, [
				actions([
					button("Edit portals and map data", async () => {
						let doc;
						try {
							doc = await asset("Maps");
						} catch {
							await request("assets/definitions/Maps/initialize", {});
							doc = await asset("Maps");
						}
						inspect("Map " + id, map, (value) =>
							saveAsset("Maps", {
								version: doc.version,
								value: { ...(doc.value ?? {}), [String(map.id)]: value },
							}),
						);
					}),
				]),
				form(
					"Edit actor",
					[
						["Character ID", "character", "", "number"],
						["Click ID", "click_id", 1, "number"],
						["Action", "action", "show", "text", ["show", "hide", "open", "close"]],
						["Scope", "scope", "map", "text", ["map", "session"]],
						["Linked quest ID (0 = automatic)", "linked_quest_id", 0, "number"],
					],
					async (v) => {
						const row = await api(`characters/${v.character}`);
						if (Number(row.state.map) !== Number(id))
							throw Error("Character is on another map.");
						return request(`characters/${v.character}/actors`, {
							version: row.version,
							click_id: Number(v.click_id),
							action: v.action,
							scope: v.scope,
							linked_quest_id: Number(v.linked_quest_id),
						});
					},
					"Map scope updates current viewers. Linked quest changes belong only to the selected character.",
				),
				h("h3", "Placements, events and decoded opcodes"),
				h("pre", JSON.stringify(actors, null, 2)),
				h("h3", "Map definition"),
				h("pre", JSON.stringify(map, null, 2)),
			]);
		};
		return [
			form("Inspect map", [["Map ID", "id", 10017, "number"]], (v) => inspectMap(v.id)),
			dynamic(detail),
			table(data, columns.maps, (r) => [button("Inspect", () => inspectMap(r.id))]),
		];
	}
	if (target === "talks") {
		const result = shallowRef(null);
		const render = (rows) =>
			(result.value = table(
				rows,
				[
					["ID / offset", "id"],
					["Lookup", "method"],
					["Dialogue", "text"],
				],
				(r) => [
					button("Send to player", () => {
						const id = prompt("Online character ID");
						if (id) return playerAction(Number(id), "dialogue", [r.text]);
					}),
				],
			));
		render(data);
		return [
			form(
				"Resolve or search dialogue",
				[
					["Talk ID, byte offset, record index or text", "q", ""],
					["Player name for #n", "name", ""],
				],
				async (v) =>
					render(
						await api(
							`assets/talks?q=${encodeURIComponent(v.q)}&name=${encodeURIComponent(v.name)}`,
						),
					),
			),
			dynamic(result),
		];
	}
	if (target === "assets_editor") {
		const result = shallowRef(null);
		return [
			form(
				"NPC placements and event opcodes",
				[["NPC template ID", "id", 10001, "number"]],
				async (v) =>
					(result.value = section("NPC report", null, [
						h("pre", JSON.stringify(await api(`assets/npcs/${v.id}/report`), null, 2)),
					])),
			),
			dynamic(result),
			form(
				"Find a definition",
				[
					[
						"Asset",
						"asset",
						"NPCs",
						"text",
						["NPCs", "Skills", "Talks", "Marks", "NativeItems", "QuestDefinitions"],
					],
					["Game ID", "id", 10001, "number"],
				],
				async (v) => {
					const records = await api(
						`assets/definitions/${encodeURIComponent(v.asset)}/records?id=${encodeURIComponent(v.id)}`,
					);
					if (!records.length) throw Error("No definition found.");
					// Select a record explicitly when IDs occur in more than one collection.
					result.value = section("Matching definitions", null, [
						actions(
							records.map((record) =>
								button(`${record.collection ?? v.asset} · ${record.ordinal}`, () =>
									inspect(`${v.asset} #${v.id}`, record.value, (value) =>
										request(
											`assets/definitions/${encodeURIComponent(v.asset)}/records/${record.ordinal}?collection=${encodeURIComponent(record.collection)}`,
											{ version: record.version, value },
											"PUT",
										),
									),
								),
							),
						),
					]);
				},
			),
			table(
				data,
				[
					["Asset", "asset"],
					["Origin", "origin"],
					["Source", "source"],
				],
				(r) => [
					button("Edit dataset", async () => {
						const doc = await asset(r.asset);
						inspect(r.asset, doc.value, (value) =>
							saveAsset(r.asset, { version: doc.version, value }),
						);
					}),
				],
			),
		];
	}
	if (target === "logs")
		return [
			section(
				"Recent server logs",
				"Newest 500 entries retained in memory. Use the table filter to search messages, levels or fields.",
			),
			table(
				[...data].reverse().map((r) => ({ ...r, fields: JSON.stringify(r.fields) })),
				columns.logs,
			),
		];
	if (target === "audit") return [table(data, columns.audit)];
	throw Error("Unknown administration page.");
}
