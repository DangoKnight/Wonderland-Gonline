<script setup>
	import { reactive, ref } from "vue";
	const values = reactive({ username: "", password: "", confirmation: "", email: "" });
	const busy = ref(false),
		complete = ref(false),
		error = ref(""),
		result = ref("");
	async function register() {
		if (busy.value) return;
		error.value = "";
		if (values.password !== values.confirmation) {
			error.value = "Passwords do not match.";
			return;
		}
		if (!/^[\x20-\x7e]+$/.test(values.password)) {
			error.value = "Use printable ASCII characters for the game client.";
			return;
		}
		busy.value = true;
		try {
			const response = await fetch("/register", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					username: values.username,
					password: values.password,
					email: values.email,
				}),
			});
			if (!response.ok) {
				let text = "Could not create your account. Please try again.";
				if (response.status === 409)
					text = "That username is already taken. Choose another.";
				else if (response.status === 429)
					text = "Registration is busy. Please try again shortly.";
				else if (response.status === 400) {
					const data = await response.json();
					text = data.error || text;
				}
				throw Error(text);
			}
			result.value = `Account ${values.username} created. Open the game and log in with your username and password.`;
			complete.value = true;
			values.password = "";
			values.confirmation = "";
		} catch (e) {
			error.value =
				e instanceof TypeError
					? "Could not confirm account creation. Check your connection and try logging in before submitting again."
					: e.message;
		} finally {
			busy.value = false;
		}
	}
</script>
<template>
	<main class="min-h-screen flex items-center justify-center p-6">
		<section class="panel w-full max-w-lg !p-8">
			<a href="/" class="eyebrow">WONDERLAND GONLINE</a>
			<h1 class="!text-3xl mt-4">Create your account</h1>
			<p class="muted my-4">Your next adventure starts here.</p>
			<p v-if="complete" role="status" class="success-banner">{{ result }}</p>
			<form v-else @submit.prevent="register" class="space-y-4">
				<fieldset :disabled="busy" class="space-y-4">
					<label
						><span>Username</span
						><input
							v-model="values.username"
							minlength="8"
							maxlength="10"
							pattern="[A-Za-z0-9_]+"
							autocomplete="username"
							required
						/><small>8–10 letters, numbers or underscores.</small></label
					><label
						><span>Password</span
						><input
							v-model="values.password"
							type="password"
							minlength="8"
							maxlength="10"
							autocomplete="new-password"
							required
						/><small>8–10 printable ASCII characters.</small></label
					><label
						><span>Confirm password</span
						><input
							v-model="values.confirmation"
							type="password"
							minlength="8"
							maxlength="10"
							autocomplete="new-password"
							required /></label
					><label
						><span>Email (optional)</span
						><input
							v-model="values.email"
							type="email"
							maxlength="254"
							autocomplete="email"
					/></label>
					<p v-if="error" role="alert" class="error">{{ error }}</p>
					<button class="primary w-full" :disabled="busy">
						{{ busy ? "Creating account…" : "Create account" }}
					</button>
				</fieldset>
			</form>
			<p class="text-sm muted mt-6">Already registered? Open the game and log in.</p>
		</section>
	</main>
</template>
