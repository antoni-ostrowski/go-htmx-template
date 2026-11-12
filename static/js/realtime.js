// realtime.js subscribes to the todos collection over PocketBase realtime
// (plain SSE, no SDK needed) and tells htmx to refetch the list fragment on
// every change. Auth comes from the readable pb_auth cookie set at login,
// sent as Authorization on the subscription call so PB API rules apply
// (only the owner's events arrive).
//
// Wiring: templates.List renders #todo-list with
// hx-get="/todos-fragment" hx-trigger="pb-todos-changed from:body".
// Included only for signed-in pages (see templates.Page).
(function () {
	function token() {
		var m = document.cookie.split("; ").find(function (c) {
			return c.indexOf("pb_auth=") === 0;
		});
		return m ? decodeURIComponent(m.slice("pb_auth=".length)) : "";
	}

	function subscribe(clientId) {
		var headers = { "Content-Type": "application/json" };
		var t = token();
		if (t) headers["Authorization"] = t;
		fetch("/api/realtime", {
			method: "POST",
			headers: headers,
			body: JSON.stringify({ clientId: clientId, subscriptions: ["todos/*"] }),
		}).catch(function () {});
	}

	function connect() {
		var es = new EventSource("/api/realtime");
		es.addEventListener("PB_CONNECT", function (ev) {
			try {
				subscribe(JSON.parse(ev.data).clientId);
			} catch (e) {}
		});
		es.onmessage = function (ev) {
			try {
				var msg = JSON.parse(ev.data);
				if (msg && msg.action) {
					htmx.trigger(document.body, "pb-todos-changed");
				}
			} catch (e) {}
		};
		// EventSource reconnects on its own (idle timeout ~5min server-side);
		// the new connection re-emits PB_CONNECT and resubscribes.
	}

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", connect);
	} else {
		connect();
	}
})();
