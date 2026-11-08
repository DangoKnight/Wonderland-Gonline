<script setup>
	import { ref, shallowRef, computed, onMounted, onBeforeUnmount } from "vue";
	import { createAPI } from "./api.js";
	import { navigation, titles, loadView } from "./views.js";
	import EditDialog from "./components/EditDialog.vue";
	import Registration from "./Registration.vue";

	const registration =
		window.location.pathname === "/register" || window.location.pathname === "/register/";
	const token = ref(""),
		inputToken = ref(""),
		authenticated = ref(false),
		connecting = ref(false);
	const current = ref(titles[location.hash.slice(1)] ? location.hash.slice(1) : "overview");
	const content = shallowRef([]),
		editor = shallowRef(null),
		status = ref(null),
		online = ref(false);
	const busy = ref(false),
		message = ref(""),
		failed = ref(false),
		menuOpen = ref(false);
	const viewVersion = ref(0),
		editorVersion = ref(0);
	let generation = 0,
		controller,
		poll;
	function logout() {
		generation++;
		controller?.abort();
		token.value = "";
		inputToken.value = "";
		authenticated.value = false;
		online.value = false;
		content.value = [];
		editor.value = null;
	}
	const api = createAPI(() => token.value, logout);
	const request = (path, body = {}, method = "POST") =>
		api(path, { method, body: JSON.stringify(body) });
	function notify(text) {
		message.value = text;
		failed.value = false;
	}
	async function loadStatus() {
		if (!authenticated.value) return;
		const expected = token.value;
		try {
			const data = await api("status");
			if (expected !== token.value) return;
			status.value = data;
			online.value = true;
		} catch {
			if (expected === token.value) online.value = false;
		}
	}
	async function refresh() {
		if (!authenticated.value) return;
		const id = ++generation;
		controller?.abort();
		controller = new AbortController();
		const signal = controller.signal;
		busy.value = true;
		message.value = "";
		failed.value = false;
		content.value = [];
		try {
			const nodes = await loadView(current.value, {
				api: (path, options = {}) => api(path, { ...options, signal }),
				request,
				notify,
				refresh,
				edit: (model) => {
					editorVersion.value++;
					editor.value = model;
				},
			});
			if (id === generation) {
				content.value = nodes;
				viewVersion.value++;
			}
		} catch (e) {
			if (id === generation && e.name !== "AbortError") {
				message.value = e.message;
				failed.value = true;
			}
		} finally {
			if (id === generation) busy.value = false;
		}
		await loadStatus();
	}
	async function connect() {
		if (connecting.value) return;
		connecting.value = true;
		token.value = inputToken.value;
		message.value = "";
		try {
			status.value = await api("status");
			authenticated.value = true;
			online.value = true;
			inputToken.value = "";
			await refresh();
		} catch (e) {
			logout();
			message.value = e.message;
			failed.value = true;
		} finally {
			connecting.value = false;
		}
	}
	function navigate(key) {
		if (!titles[key]) return;
		current.value = key;
		menuOpen.value = false;
		if (location.hash.slice(1) !== key) location.hash = key;
		refresh();
	}
	function hashChanged() {
		const key = location.hash.slice(1);
		if (titles[key] && key !== current.value) navigate(key);
	}
	onMounted(() => {
		window.addEventListener("hashchange", hashChanged);
		poll = setInterval(loadStatus, 15000);
	});
	onBeforeUnmount(() => {
		clearInterval(poll);
		controller?.abort();
		window.removeEventListener("hashchange", hashChanged);
	});
	const uptime = computed(() => {
		const total = Math.floor(status.value?.uptime_seconds ?? 0);
		return [Math.floor(total / 3600), Math.floor(total / 60) % 60, total % 60]
			.map((v) => String(v).padStart(2, "0"))
			.join(":");
	});
	const services = computed(() => Object.entries(status.value?.services ?? {}));
	const ViewContent = () => content.value;
	async function saved() {
		editor.value = null;
		await refresh();
		if (!failed.value) notify("Changes saved.");
		else message.value = "Changes saved. " + message.value;
	}
</script>
<template>
	<Registration v-if="registration" />
	<div v-else class="min-h-screen lg:grid lg:grid-cols-[248px_minmax(0,1fr)]">
		<aside class="sidebar" :class="{ 'mobile-open': menuOpen }">
			<a href="#overview" class="brand" @click.prevent="navigate('overview')"
				><span class="brand-mark">W</span
				><span><strong>WONDERLAND</strong><small>GONLINE / ADMIN</small></span></a
			>
			<div class="px-5 py-4 border-y border-white/10">
				<span class="text-[10px] tracking-[.18em] text-slate-400">SERVER CONSOLE</span>
				<p class="mt-1 text-sm truncate">
					{{ status?.server_name || "Wonderland Gonline" }}
				</p>
			</div>
			<nav aria-label="Administration" class="grow overflow-y-auto px-3 py-5 space-y-6">
				<div v-for="[group, items] in navigation" :key="group">
					<h2 class="nav-group">{{ group }}</h2>
					<a
						v-for="[key, label] in items"
						:key="key"
						:href="'#' + key"
						class="nav-link"
						:class="{ selected: current === key }"
						:aria-current="current === key ? 'page' : undefined"
						@click.prevent="navigate(key)"
						><span>{{ label }}</span
						><span v-if="current === key" class="text-emerald-300">›</span></a
					>
				</div>
			</nav>
			<div class="p-5 border-t border-white/10 text-xs text-slate-400">
				<p>Wonderland Gonline</p>
				<p class="mt-1 text-slate-500">Server administration</p>
			</div>
		</aside>
		<div class="min-w-0">
			<header class="topbar">
				<div class="flex items-center gap-3">
					<button
						class="lg:!hidden"
						aria-label="Toggle navigation"
						:aria-expanded="menuOpen"
						@click="menuOpen = !menuOpen"
					>
						☰</button
					><span
						class="status-dot"
						:class="{ connected: online && authenticated }"
					/><span class="text-xs font-semibold tracking-wide uppercase">{{
						authenticated
							? online
								? "Connected"
								: "Connection interrupted"
							: "Not connected"
					}}</span>
				</div>
				<div v-if="authenticated" class="hidden md:flex gap-6 text-xs muted">
					<span
						><strong class="text-slate-700">{{ status?.connections ?? 0 }}</strong>
						connections</span
					><span class="font-mono">{{ uptime }}</span
					><span>{{ (status?.packets_received ?? 0).toLocaleString() }} packets</span>
				</div>
				<div class="flex gap-2">
					<button v-if="authenticated" :disabled="busy" @click="refresh">
						{{ busy ? "Refreshing…" : "Refresh" }}</button
					><button v-if="authenticated" @click="logout">Disconnect</button
					><a v-else href="/register" class="text-sm text-emerald-700 font-semibold"
						>Player registration ↗</a
					>
				</div>
			</header>
			<main id="main" class="mx-auto max-w-[1600px] p-5 md:p-8 xl:p-10 space-y-6">
				<div class="flex items-end justify-between gap-4">
					<div>
						<p class="eyebrow">
							ADMINISTRATION /
							{{
								navigation.find(([, items]) =>
									items.some(([key]) => key === current),
								)?.[0]
							}}
						</p>
						<h1>{{ titles[current] }}</h1>
					</div>
					<span class="hidden sm:block text-xs muted">Wonderland Gonline</span>
				</div>
				<div
					v-if="message"
					:class="failed ? 'error-banner' : 'success-banner'"
					role="status"
					aria-live="polite"
				>
					{{ message }}
				</div>
				<section v-if="!authenticated" class="panel max-w-xl !p-8 mt-12">
					<span class="eyebrow">ADMINISTRATOR ACCESS</span>
					<h2 class="!text-2xl !mt-3">Connect to your server</h2>
					<p class="muted mb-6">
						Use the administrator token configured for this server. It stays in memory
						until you disconnect or reload this page.
					</p>
					<form @submit.prevent="connect" class="space-y-5">
						<label
							><span>Administrator token</span
							><input
								v-model="inputToken"
								type="password"
								autocomplete="off"
								minlength="24"
								required
								:disabled="connecting" /></label
						><button class="primary w-full" :disabled="connecting">
							{{ connecting ? "Connecting…" : "Connect to server" }}
						</button>
					</form>
				</section>
				<template v-else
					><div v-if="busy" role="status" class="muted text-sm">
						Loading {{ titles[current].toLowerCase() }}…
					</div>
					<div :aria-busy="busy" class="space-y-6">
						<ViewContent :key="viewVersion" />
					</div>
					<footer class="flex flex-wrap gap-x-5 gap-y-2 text-xs muted pt-4">
						<span v-for="[name, address] in services" :key="name"
							>{{ name }}
							<span class="font-mono">{{ address || "offline" }}</span></span
						>
					</footer></template
				>
			</main>
		</div>
		<EditDialog
			v-if="editor"
			:key="editorVersion"
			:model="editor"
			@close="editor = null"
			@saved="saved"
		/>
	</div>
</template>
