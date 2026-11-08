<script setup>
	import { reactive, ref } from "vue";
	const props = defineProps({
		title: String,
		note: String,
		fields: { type: Array, default: () => [] },
		submit: Function,
		notify: Function,
		label: { default: "Apply" },
	});
	const values = reactive(Object.fromEntries(props.fields.map((f) => [f[1], f[2] ?? ""])));
	const busy = ref(false),
		error = ref("");
	async function save() {
		if (busy.value) return;
		busy.value = true;
		error.value = "";
		try {
			const result = await props.submit({ ...values });
			if (result !== false) props.notify?.(result?.messages?.join("\n") || "Saved.");
		} catch (e) {
			error.value = e.message;
		} finally {
			busy.value = false;
		}
	}
</script>
<template>
	<section class="panel">
		<h2>{{ title }}</h2>
		<p v-if="note" class="muted mb-5">{{ note }}</p>
		<form @submit.prevent="save" class="grid gap-4 sm:grid-cols-2">
			<fieldset :disabled="busy" class="contents">
				<label
					v-for="f in fields"
					:key="f[1]"
					:class="{ 'sm:col-span-2': f[3] === 'textarea' }"
				>
					<span>{{ f[0] }}</span>
					<select v-if="f[4]" v-model="values[f[1]]">
						<option
							v-for="choice in f[4]"
							:key="choice.value ?? choice"
							:value="choice.value ?? choice"
						>
							{{ choice.label ?? choice }}
						</option>
					</select>
					<textarea
						v-else-if="f[3] === 'textarea'"
						v-model="values[f[1]]"
						rows="3"
						v-bind="f[5]"
					/>
					<input
						v-else
						v-model="values[f[1]]"
						:type="f[3] || 'text'"
						:step="f[3] === 'number' ? 'any' : undefined"
						v-bind="f[5]"
					/>
				</label>
				<p v-if="error" class="error sm:col-span-2" role="alert">{{ error }}</p>
				<div class="sm:col-span-2">
					<button class="primary" :disabled="busy">
						{{ busy ? "Working…" : label }}
					</button>
				</div>
			</fieldset>
		</form>
	</section>
</template>
