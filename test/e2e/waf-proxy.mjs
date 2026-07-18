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
    if (request.url?.startsWith("/attachments/") && !userAgent.startsWith("Bitwarden_CLI/")) {
      response.writeHead(401, { "content-type": "text/plain" });
      response.end("attachment request rejected by e2e WAF\n");
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
