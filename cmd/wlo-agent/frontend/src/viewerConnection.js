// One canonical client remains in the host. Every connection starts readonly;
// generations fence held keys and delayed input across control transfers.
export function createViewerConnection(url, ticket, onFrame, onStatus) {
	let socket,
		timer,
		retry,
		stopped = false,
		sequence = 0;
	let state = { owner: "api", generation: 0 },
		viewer = "",
		lastError = "";
	const owns = () =>
		socket?.readyState === 1 && state.owner === "viewer" && state.viewer === viewer;
	function send(operation, data = {}) {
		if (socket?.readyState !== 1) return;
		socket.send(JSON.stringify({ ...data, operation, generation: state.generation }));
	}
	function connect() {
		state = { owner: "api", generation: 0 };
		viewer = "";
		sequence = 0;
		socket = new WebSocket(url);
		socket.onopen = () => {
			socket.send(JSON.stringify({ ticket }));
			timer = setInterval(() => send(owns() ? "heartbeat" : "ping"), 1000);
		};
		socket.onmessage = ({ data }) => {
			const frame = JSON.parse(data);
			if (frame.error) {
				lastError = frame.error;
				stopped = true;
				socket.close();
				return;
			}
			if (state.generation !== frame.control.generation) sequence = 0;
			state = frame.control;
			viewer = frame.viewer;
			onFrame(frame);
			onStatus({ connected: true, owns: owns(), control: state });
		};
		socket.onclose = () => {
			clearInterval(timer);
			onStatus({ connected: false, owns: false, ...(lastError ? { error: lastError } : {}) });
			if (!stopped) retry = setTimeout(connect, 1000);
		};
		socket.onerror = () => socket.close();
	}
	connect();
	return {
		takeover: () => {
			if (state.generation && state.owner === "api") send("takeover");
		},
		release: () => {
			if (owns()) send("release");
		},
		input: (data) => {
			if (owns()) send("input", { ...data, sequence: ++sequence });
		},
		close: () => {
			stopped = true;
			clearTimeout(retry);
			clearInterval(timer);
			socket?.close();
		},
	};
}
