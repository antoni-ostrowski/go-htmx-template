// realtime.js bridges PocketBase realtime (plain SSE, no SDK) into htmx.
//
// Any element opts into live updates with one attribute; the DOM event it
// fires is derived from its hx-trigger (override with data-realtime-event):
//
//   <ul id="todo-list"
//       hx-get="/todos-fragment" hx-trigger="pb-todos-changed from:body"
//       data-realtime-topic="todos/*">
//
// The script collects every [data-realtime-topic] on the page, subscribes to
// them all in one /api/realtime call, and fires each topic's DOM event on
// record changes. htmx picks it up via hx-trigger and refetches the
// fragment. Add more collections by adding attributes — no JS changes.
//
// Auth comes from the readable pb_auth cookie set at login, sent as
// Authorization on the subscription POST so PB API rules apply (only
// records the user may see produce events).
//
// PB names every SSE event: PB_CONNECT for the handshake, then the
// subscription topic (e.g. "todos/*") for record events. There are no
// unnamed message events.
(function () {
	// topic -> [domEvent, ...]
	function collect() {
		var map = {};
		document.querySelectorAll("[data-realtime-topic]").forEach(function (el) {
			var topic = el.getAttribute("data-realtime-topic");
			var events = eventNamesFor(el);
			if (!map[topic]) map[topic] = [];
			events.forEach(function (ev) {
				if (map[topic].indexOf(ev) === -1) map[topic].push(ev);
			});
		});
		return map;
	}

	// Explicit data-realtime-event wins. Otherwise derive from hx-trigger:
	// the first word of the clause carrying a from: modifier, e.g.
	// hx-trigger="click, pb-todos-changed from:body" -> "pb-todos-changed".
	// (htmx order is free, so last-clause would misfire on
	// hx-trigger="pb-todos-changed from:body, every 2s".) The from: clause
	// is the right one because events fire on document.body — a trigger
	// without from:body wouldn't hear them anyway. Falls back to a generic
	// name when there is no usable trigger.
	function eventNamesFor(el) {
		var explicit = el.getAttribute("data-realtime-event");
		if (explicit) return [explicit];
		var clauses = (el.getAttribute("hx-trigger") || "").split(",");
		var chosen = null;
		for (var i = 0; i < clauses.length; i++) {
			if (/(?:^|\s)from:/.test(clauses[i])) { chosen = clauses[i]; break; }
		}
		if (chosen === null) chosen = clauses[0] || "";
		var name = chosen.trim().split(/\s+/)[0] || "";
		return [name || "pb-realtime-changed"];
	}

	function token() {
		var m = document.cookie.split("; ").find(function (c) {
			return c.indexOf("pb_auth=") === 0;
		});
		return m ? decodeURIComponent(m.slice("pb_auth=".length)) : "";
	}

	function onRecordEvent(topic, events) {
		return function (ev) {
			try {
				var msg = JSON.parse(ev.data);
				if (msg && msg.action) {
					events.forEach(function (name) {
						htmx.trigger(document.body, name);
					});
				}
			} catch (e) {
				console.warn("realtime: bad record event on " + topic, e);
			}
		};
	}

	function subscribe(clientId, topics) {
		var headers = { "Content-Type": "application/json" };
		var t = token();
		if (t) headers["Authorization"] = t;
		fetch("/api/realtime", {
			method: "POST",
			headers: headers,
			body: JSON.stringify({ clientId: clientId, subscriptions: topics }),
		}).then(function (res) {
			if (!res.ok) console.warn("realtime: subscribe failed", res.status);
		}).catch(function (e) {
			console.warn("realtime: subscribe error", e);
		});
	}

	function connect(topicsByEvent) {
		var topics = Object.keys(topicsByEvent);
		if (!topics.length) return;
		var es = new EventSource("/api/realtime");
		es.addEventListener("PB_CONNECT", function (ev) {
			try {
				subscribe(JSON.parse(ev.data).clientId, topics);
			} catch (e) {
				console.warn("realtime: bad PB_CONNECT", e);
			}
		});
		topics.forEach(function (topic) {
			es.addEventListener(topic, onRecordEvent(topic, topicsByEvent[topic]));
		});
		es.onerror = function () {
			// EventSource reconnects on its own (idle timeout ~5min
			// server-side); the new connection re-emits PB_CONNECT and
			// resubscribes. Logging only, no manual retry needed.
			console.warn("realtime: connection error, retrying");
		};
	}

	function start() {
		connect(collect());
	}

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", start);
	} else {
		start();
	}
})();
