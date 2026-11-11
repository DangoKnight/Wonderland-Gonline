<script setup>
	import { onBeforeUnmount, onMounted, ref } from "vue";
	import { createClientHost } from "./clientHost.js";

	const status = ref("Loading assets...");
	const ready = ref(false);
	const failed = ref(false);
	const host = createClientHost({
		onStatus(message) {
			status.value = message;
		},
		onReady() {
			ready.value = true;
		},
		onError(error) {
			failed.value = true;
			ready.value = false;
			status.value = error.message;
		},
	});

	onMounted(() => host.start());
	onBeforeUnmount(() => host.dispose());
</script>

<template>
	<div
		v-if="!ready"
		id="status"
		:role="failed ? 'alert' : 'status'"
		:class="failed ? 'bg-red-950/90 text-red-100' : 'bg-black/70'"
		class="pointer-events-none fixed top-2.5 left-2.5 z-50 max-w-[calc(100vw-1.25rem)] rounded px-3 py-2 text-sm break-words"
	>
		{{ status }}
	</div>
</template>
