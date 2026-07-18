import fs from "node:fs";
import https from "node:https";

const [listenPort, backendPort, certificatePath, keyPath] = process.argv.slice(2);
if (!listenPort || !backendPort || !certificatePath || !keyPath) {
  throw new Error("usage: waf-proxy.mjs LISTEN_PORT BACKEND_PORT CERTIFICATE KEY");
}

const server = https.createServer(
  {
    cert: fs.readFileSync(certificatePath),
    key: fs.readFileSync(keyPath),
  },
  (request, response) => {
    const userAgent = request.headers["user-agent"] ?? "";
	const mutation = ["POST", "PUT", "DELETE"].includes(request.method ?? "") &&
		(request.url?.startsWith("/api/ciphers") || request.url?.startsWith("/api/folders"));
	const attachment = request.url?.startsWith("/attachments/");
	const hasOfficialHeaders = userAgent.startsWith("Bitwarden_CLI/") &&
		request.headers["bitwarden-client-name"] === "cli" &&
		Boolean(request.headers["bitwarden-client-version"]) &&
		Boolean(request.headers["device-type"]);
	const rejected = (attachment && !hasOfficialHeaders) ||
		(mutation && !hasOfficialHeaders);
    if (rejected) {
      response.writeHead(401, { "content-type": "text/plain" });
		response.end("request rejected by e2e WAF: official client headers are required\n");
      return;
    }

    const upstream = https.request(
      {
        hostname: "127.0.0.1",
        port: backendPort,
        path: request.url,
        method: request.method,
        headers: request.headers,
        rejectUnauthorized: false,
      },
      (upstreamResponse) => {
        response.writeHead(upstreamResponse.statusCode ?? 502, upstreamResponse.headers);
        upstreamResponse.pipe(response);
      },
    );
    upstream.on("error", (error) => {
      if (!response.headersSent) {
        response.writeHead(502, { "content-type": "text/plain" });
      }
      response.end(`upstream error: ${error.message}\n`);
    });
    request.pipe(upstream);
  },
);

server.listen(Number(listenPort), "127.0.0.1");
