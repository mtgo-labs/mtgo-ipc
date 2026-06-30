/**
 * Minimal mtgo-ipc client — Bun + TypeScript.
 *
 * Run after starting the server:
 *   bun examples/node-client/client.ts
 *
 * No dependencies — uses Bun's built-in net module.
 */

import { createConnection, type Socket } from "node:net";

const SOCKET_PATH = process.env.MTGO_IPC_SOCKET ?? "/tmp/mtgo-ipc.sock";

interface JsonRpcResponse {
	jsonrpc: string;
	id: number;
	result?: unknown;
	error?: { code: number; message: string; data?: unknown };
}

/** Send a JSON-RPC request and await the newline-delimited response. */
function rpc(sock: Socket, id: number, method: string, params?: Record<string, unknown>): Promise<JsonRpcResponse> {
	const { promise, resolve, reject } = Promise.withResolvers<JsonRpcResponse>();

	const req: Record<string, unknown> = { jsonrpc: "2.0", id, method };
	if (params !== undefined) req.params = params;

	let buf = "";

	const onData = (chunk: Buffer): void => {
		buf += chunk.toString();
		const nl = buf.indexOf("\n");
		if (nl === -1) return;

		const line = buf.slice(0, nl);
		buf = buf.slice(nl + 1);
		sock.off("data", onData);

		try {
			resolve(JSON.parse(line) as JsonRpcResponse);
		} catch (err) {
			reject(err);
		}
	};

	sock.on("data", onData);
	sock.write(JSON.stringify(req) + "\n");
	return promise;
}

async function main(): Promise<void> {
	const sock = createConnection(SOCKET_PATH);
	await new Promise<void>((resolve) => sock.once("connect", resolve));

	try {
		// 1. Health check
		const health = await rpc(sock, 1, "health");
		console.log("health:       ", JSON.stringify(health.result));

	// 2. Connect to Telegram — credentials passed from the client
	const connect = await rpc(sock, 2, "mtgo.connect", {
		api_id: Number(process.env.API_ID),
		api_hash: process.env.API_HASH,
		bot_token: process.env.BOT_TOKEN,
		session: process.env.SESSION,
	});
	console.log("mtgo.connect: ", JSON.stringify(connect.result));

		// 3. Invoke a raw TL method
		const config = await rpc(sock, 3, "telegram.invoke", { method: "help.getConfig", args: {} });
		if (config.result && typeof config.result === "object" && "this_dc" in config.result) {
			console.log("getConfig dc: ", (config.result as { this_dc: number }).this_dc);
		} else if (config.error) {
			console.log("getConfig err:", config.error.message);
		}

		// 4. Check auth
		const auth = await rpc(sock, 4, "auth.status");
		console.log("auth.status:  ", JSON.stringify(auth.result));

		// 5. getMe — users.getFullUser with inputUserSelf
		const me = await rpc(sock, 5, "telegram.invoke", {
			method: "users.getFullUser",
			args: { id: { _: "inputUserSelf" } },
		});
		if (me.result && typeof me.result === "object" && "users" in me.result) {
			const users = (me.result as { users: Array<Record<string, unknown>> }).users;
			if (users.length > 0) {
				const u = users[0];
				console.log("getMe:        ", `id=${u.id} @${u.username} bot=${u.bot} name=${u.first_name}`);
			}
		} else if (me.error) {
			console.log("getMe error:  ", me.error.message);
		}
	} finally {
		sock.end();
	}
}

main().catch((err: unknown) => {
	console.error("error:", err);
	process.exit(1);
});
