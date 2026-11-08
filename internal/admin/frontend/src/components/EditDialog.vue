<script setup>
	import { ref, onMounted, onBeforeUnmount, nextTick } from "vue";
	const props = defineProps({ model: Object });
	const emit = defineEmits(["close", "saved"]);
	const dialog = ref(),
		form = ref(),
		busy = ref(false),
		error = ref("");
	const json = ref(JSON.stringify(props.model.value, null, 2));
	const fields = ref(
		Object.fromEntries((props.model.fields ?? []).map((f) => [f[1], f[2] ?? ""])),
	);
	let previous;
	onMounted(async () => {
		previous = document.activeElement;
		dialog.value.showModal();
		await nextTick();
		dialog.value.querySelector("input,textarea,button")?.focus();
	});
	onBeforeUnmount(() => {
		fields.value = {};
		previous?.focus();
	});
	function close(e) {
		if (busy.value) {
			e?.preventDefault();
			return;
		}
		emit("close");
	}
	async function save() {
		if (busy.value || !props.model.save || !form.value.reportValidity()) return;
		busy.value = true;
		error.value = "";
		try {
			const value = props.model.fields ? { ...fields.value } : JSON.parse(json.value);
			await props.model.save(value);
			// Close after a successful write, before refreshing; refresh failure must
			// never present an enabled button inviting a duplicate financial change.
			fields.value = {};
			emit("saved");
			emit("close");
		} catch (e) {
			error.value = e.message;
		} finally {
			busy.value = false;
		}
	}
</script>
<template>
	<Teleport to="body"
		><dialog ref="dialog" class="editor-dialog" @cancel="close">
			<form ref="form" @submit.prevent="save">
				<div class="flex justify-between items-start gap-4">
					<h2>{{ model.title }}</h2>
					<button type="button" aria-label="Close dialog" :disabled="busy" @click="close">
						×
					</button>
				</div>
				<p v-if="model.note" class="muted mb-5">{{ model.note }}</p>
				<fieldset :disabled="busy" class="grid gap-4">
					<template v-if="model.fields"
						><label v-for="f in model.fields" :key="f[1]"
							><span>{{ f[0] }}</span
							><select v-if="f[4]" v-model="fields[f[1]]">
								<option v-for="o in f[4]" :key="o.value ?? o" :value="o.value ?? o">
									{{ o.label ?? o }}
								</option></select
							><input
								v-else
								v-model="fields[f[1]]"
								:type="f[3] || 'text'"
								v-bind="f[5]" /></label></template
					><textarea
						v-else
						v-model="json"
						:readonly="!model.save"
						:aria-label="model.title"
						spellcheck="false"
						class="json-editor"
					/>
					<p v-if="error" class="error" role="alert">{{ error }}</p>
					<div class="flex justify-end gap-3">
						<button type="button" @click="close">
							{{ model.save ? "Cancel" : "Close" }}</button
						><button v-if="model.save" class="primary" :disabled="busy">
							{{ busy ? "Saving…" : "Save changes" }}
						</button>
					</div>
				</fieldset>
			</form>
		</dialog></Teleport
	>
</template>
