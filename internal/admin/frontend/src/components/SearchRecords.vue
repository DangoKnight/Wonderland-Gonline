<script setup>
	import { ref, onBeforeUnmount } from "vue";
	import RecordTable from "./RecordTable.vue";
	const props = defineProps({
		initial: Array,
		load: Function,
		columns: Array,
		actions: Function,
		notify: Function,
		title: String,
		more: Function,
	});
	const rows = ref(props.initial ?? []),
		query = ref(""),
		busy = ref(false),
		error = ref("");
	let sequence = 0;
	onBeforeUnmount(() => sequence++);
	async function search(next = false) {
		const request = ++sequence;
		busy.value = true;
		error.value = "";
		try {
			const result = await (next
				? props.more(query.value, rows.value.at(-1))
				: props.load(query.value));
			if (sequence === request) rows.value = result ?? [];
		} catch (e) {
			if (sequence === request) error.value = e.message;
		} finally {
			if (sequence === request) busy.value = false;
		}
	}
</script>
<template>
	<section class="panel">
		<form @submit.prevent="search(false)" class="flex items-end gap-3">
			<label class="grow"
				><span>Search by name or exact ID</span><input v-model="query" /></label
			><button class="primary" :disabled="busy">
				{{ busy ? "Searching…" : "Search server" }}
			</button>
		</form>
		<p v-if="error" role="alert" class="error mt-3">{{ error }}</p>
	</section>
	<RecordTable
		:rows="rows"
		:columns="columns"
		:actions="actions"
		:notify="notify"
		:title="title"
	/><button v-if="more && rows.length === 500" :disabled="busy" @click="search(true)">
		Next 500 records
	</button>
</template>
