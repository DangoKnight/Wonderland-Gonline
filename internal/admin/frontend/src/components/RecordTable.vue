<script setup>
	import { computed, ref } from "vue";
	import ActionButton from "./ActionButton.vue";
	const props = defineProps({
		rows: { type: Array, default: () => [] },
		columns: Array,
		actions: Function,
		notify: Function,
		title: String,
	});
	const search = ref(""),
		page = ref(1),
		sort = ref(""),
		reverse = ref(false);
	const pageSize = 25;
	const filtered = computed(() => {
		const needle = search.value.toLowerCase();
		const rows = props.rows.filter(
			(r) => !needle || JSON.stringify(r).toLowerCase().includes(needle),
		);
		if (sort.value)
			rows.sort(
				(a, b) =>
					String(a[sort.value] ?? "").localeCompare(
						String(b[sort.value] ?? ""),
						undefined,
						{
							numeric: true,
						},
					) * (reverse.value ? -1 : 1),
			);
		return rows;
	});
	const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize)));
	const current = computed(() => Math.min(page.value, pages.value));
	const visible = computed(() =>
		filtered.value.slice((current.value - 1) * pageSize, current.value * pageSize),
	);
	function order(key) {
		reverse.value = sort.value === key ? !reverse.value : false;
		sort.value = key;
	}
	function display(v) {
		return v && typeof v === "object" ? JSON.stringify(v) : (v ?? "—");
	}
</script>
<template>
	<section class="panel overflow-hidden !p-0">
		<div
			class="flex flex-wrap items-center justify-between gap-3 p-5 border-b border-slate-200"
		>
			<h2 class="!m-0">
				{{ title || "Records" }}
				<span class="muted text-sm font-normal">{{ filtered.length }}</span>
			</h2>
			<input
				aria-label="Filter records"
				v-model="search"
				@input="page = 1"
				placeholder="Filter records…"
				class="!w-64"
			/>
		</div>
		<div class="overflow-x-auto">
			<table>
				<thead>
					<tr>
						<th
							v-for="[label, key] in columns"
							:key="key"
							:aria-sort="
								sort === key ? (reverse ? 'descending' : 'ascending') : 'none'
							"
						>
							<button class="table-heading" @click="order(key)">
								{{ label
								}}<span v-if="sort === key">{{ reverse ? " ↓" : " ↑" }}</span>
							</button>
						</th>
						<th v-if="actions">Actions</th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="(row, i) in visible" :key="row.id ?? row.ID ?? i">
						<td v-for="[, key] in columns" :key="key">{{ display(row[key]) }}</td>
						<td v-if="actions">
							<div class="flex flex-wrap gap-2 min-w-44">
								<ActionButton
									v-for="(a, j) in actions(row)"
									:key="j"
									:action="a"
									:row="row"
									:notify="notify"
								/>
							</div>
						</td>
					</tr>
					<tr v-if="!visible.length">
						<td
							:colspan="columns.length + (actions ? 1 : 0)"
							class="!py-12 text-center muted"
						>
							No records to show.
						</td>
					</tr>
				</tbody>
			</table>
		</div>
		<div class="flex justify-between items-center p-4 border-t border-slate-200 text-sm muted">
			<span>Page {{ current }} of {{ pages }}</span>
			<div class="flex gap-2">
				<button :disabled="current <= 1" @click="page = current - 1">Previous</button
				><button :disabled="current >= pages" @click="page = current + 1">Next</button>
			</div>
		</div>
	</section>
</template>
