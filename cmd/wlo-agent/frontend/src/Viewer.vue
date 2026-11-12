<script setup>
	import { onMounted, onUnmounted, ref } from "vue";
	import { createViewerConnection } from "./viewerConnection.js";
	const canvas = ref(null),
		status = ref({ connected: false, owns: false });
	let connection,
		latest = 0;
	function point(e) {
		const rect = canvas.value.getBoundingClientRect();
		return {
			x: Math.floor(((e.clientX - rect.left) * 800) / rect.width),
			y: Math.floor(((e.clientY - rect.top) * 600) / rect.height),
			shift: (e.shiftKey ? 1 : 0) | (e.altKey ? 2 : 0) | (e.ctrlKey ? 4 : 0),
		};
	}
	function pointer(e, kind) {
		e.preventDefault();
		canvas.value.focus();
		if (kind === "down") canvas.value.setPointerCapture(e.pointerId);
		connection?.input({ kind, ...point(e), button: e.button === 2 ? 1 : 0 });
	}
	function key(e, down) {
		if (!(e.ctrlKey && e.key.toLowerCase() === "v")) e.preventDefault();
		connection?.input({
			kind: "key",
			key: e.keyCode,
			down,
			shift: (e.shiftKey ? 1 : 0) | (e.altKey ? 2 : 0) | (e.ctrlKey ? 4 : 0),
		});
		if (down && e.key.length === 1 && !e.ctrlKey && !e.altKey)
			connection?.input({ kind: "text", text: e.key });
	}
	onMounted(() => {
		const ticket = location.hash.slice(1) || sessionStorage.getItem("wlo-viewer-ticket") || "";
		if (ticket) sessionStorage.setItem("wlo-viewer-ticket", ticket);
		// Keep capabilities out of history, referrers, request paths and logs.
		history.replaceState(null, "", location.pathname);
		connection = createViewerConnection(
			`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/viewer/ws`,
			ticket,
			(frame) => {
				if (!frame.jpeg) return;
				const image = new Image(),
					version = ++latest;
				image.onload = () => {
					if (version === latest)
						canvas.value?.getContext("2d").drawImage(image, 0, 0, 800, 600);
				};
				image.src = `data:image/jpeg;base64,${frame.jpeg}`;
			},
			(value) => {
				status.value = value;
			},
		);
	});
	onUnmounted(() => connection?.close());
</script>

<template>
	<main class="flex h-screen flex-col items-center justify-center gap-2 bg-slate-950">
		<div class="flex items-center gap-4">
			<span role="status">{{
				status.error ||
				(!status.connected
					? "Reconnecting…"
					: status.owns
						? "You control this session"
						: "Watching session")
			}}</span>
			<button
				class="rounded bg-blue-700 px-4 py-2 disabled:opacity-40"
				:disabled="!status.connected || status.control?.owner !== 'api'"
				@click="connection.takeover()"
			>
				Take control
			</button>
			<button
				class="rounded bg-slate-700 px-4 py-2 disabled:opacity-40"
				:disabled="!status.owns"
				@click="connection.release()"
			>
				Return to API
			</button>
		</div>
		<canvas
			ref="canvas"
			width="800"
			height="600"
			tabindex="0"
			aria-label="Hosted game session"
			class="max-h-[90vh] max-w-full aspect-[4/3] touch-none outline-none"
			@contextmenu.prevent
			@pointermove="pointer($event, 'move')"
			@pointerdown="pointer($event, 'down')"
			@pointerup="pointer($event, 'up')"
			@pointercancel="connection?.input({ kind: 'reset' })"
			@blur="connection?.input({ kind: 'reset' })"
			@keydown="key($event, true)"
			@keyup="key($event, false)"
			@paste.prevent="
				connection?.input({
					kind: 'text',
					text: $event.clipboardData.getData('text').slice(0, 1024),
				})
			"
			@wheel.prevent="
				connection?.input({ kind: 'wheel', ...point($event), down: $event.deltaY < 0 })
			"
		/>
	</main>
</template>
