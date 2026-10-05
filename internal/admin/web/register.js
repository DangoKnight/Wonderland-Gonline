"use strict";

const form = document.getElementById("registration");
const username = document.getElementById("username");
const password = document.getElementById("password");
const confirmation = document.getElementById("confirmation");
const email = document.getElementById("email");
const submit = document.getElementById("submit");
const result = document.getElementById("result");
let submitting = false;

function validatePasswords() {
	password.setCustomValidity(
		/^[\x20-\x7e]*$/.test(password.value)
			? ""
			: "Use printable ASCII characters for the game client.",
	);
	confirmation.setCustomValidity(
		confirmation.value === password.value ? "" : "Passwords do not match.",
	);
}
password.addEventListener("input", validatePasswords);
confirmation.addEventListener("input", validatePasswords);

function showResult(message, success) {
	result.textContent = message;
	result.className = success ? "success" : "error";
	result.hidden = false;
	result.focus();
}

form.addEventListener("submit", async (event) => {
	event.preventDefault();
	if (submitting) return;
	validatePasswords();
	if (!form.reportValidity()) return;
	submitting = true;
	submit.disabled = true;
	submit.textContent = "Creating account…";
	result.hidden = true;
	const accountName = username.value;
	try {
		const response = await fetch("/register", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({
				username: accountName,
				password: password.value,
				email: email.value,
			}),
		});
		if (!response.ok) {
			let message = "Could not create your account. Please try again.";
			if (response.status === 409)
				message = "That username is already taken. Choose another.";
			else if (response.status === 429)
				message = "Registration is busy. Please try again shortly.";
			else if (response.status === 400) {
				const details = await response.json();
				if (typeof details.error === "string") message = details.error;
			}
			showResult(message, false);
			return;
		}
		form.reset();
		form.hidden = true;
		showResult(
			`Account ${accountName} created. Open the game and log in with your username and password.`,
			true,
		);
	} catch {
		showResult(
			"Could not confirm account creation. Check your connection and try logging in before submitting again.",
			false,
		);
	} finally {
		submitting = false;
		submit.disabled = false;
		submit.textContent = "Create account";
	}
});
