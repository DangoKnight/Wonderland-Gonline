export function createAPI(getToken, unauthorized = () => {}) {
	return async (path, options = {}) => {
		const headers = { ...options.headers };
		const token = getToken();
		if (token) headers.Authorization = `Bearer ${token}`;
		if (options.body !== undefined) headers["Content-Type"] = "application/json";
		const response = await fetch(`/api/${path}`, {
			...options,
			headers,
			credentials: "same-origin",
		});
		const text = await response.text();
		let data;
		try {
			data = text ? JSON.parse(text) : null;
		} catch {
			throw new Error(`Invalid server response (${response.status}).`);
		}
		if (!response.ok) {
			if (response.status === 401) unauthorized();
			throw new Error(data?.error || `Request failed (${response.status}).`);
		}
		return data;
	};
}
