<script setup>
	import { ref } from "vue";
	const props = defineProps({ action: Object, row: Object, notify: Function });
	const busy = ref(false),
		error = ref("");
	async function run() {
		if (busy.value) return;
		busy.value = true;
		error.value = "";
		try {
			const result = await props.action.run(props.row);
			if (result?.messages?.length) props.notify?.(result.messages.join("\n"));
		} catch (e) {
			error.value = e.message;
		} finally {
			busy.value = false;
		}
	}
</script>
<template>
	<div class="inline-flex flex-col items-start gap-1">
		<button type="button" :class="{ danger: action.danger }" :disabled="busy" @click="run">
			{{ busy ? "Working…" : action.label }}</button
		><span v-if="error" class="error text-xs max-w-64" role="alert">{{ error }}</span>
	</div>
</template>
