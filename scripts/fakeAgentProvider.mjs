import { createServer } from "node:http";

const port = Number.parseInt(process.env.FAKE_AGENT_PORT ?? "3201", 10);

function chunk(content) {
  return `data: ${JSON.stringify({
    choices: [{ delta: { content } }],
  })}\n\n`;
}

function writeChunk(response, content) {
  if (response.writableEnded || response.destroyed) {
    return false;
  }
  response.write(chunk(content));
  return true;
}

function finish(response) {
  if (response.writableEnded || response.destroyed) {
    return;
  }
  response.end("data: [DONE]\n\n");
}

const server = createServer((request, response) => {
  if (request.method === "GET" && request.url === "/healthz") {
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end('{"status":"ok"}');
    return;
  }

  if (request.method !== "POST" || request.url !== "/v1/chat/completions") {
    response.writeHead(404);
    response.end();
    return;
  }

  let body = "";
  request.setEncoding("utf8");
  request.on("data", (value) => {
    body += value;
  });
  request.on("end", () => {
    let payload;
    try {
      payload = JSON.parse(body);
    } catch {
      response.writeHead(400, { "Content-Type": "application/json" });
      response.end('{"error":"invalid json"}');
      return;
    }

    const prompt =
      [...payload.messages]
        .reverse()
        .find((message) => message.role === "user")?.content ?? "";

    response.writeHead(200, {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    });
    response.flushHeaders();

    if (prompt.includes("慢速恢复测试")) {
      writeChunk(response, "第一段");
      setTimeout(() => {
        writeChunk(response, "第二段");
        finish(response);
      }, 1_200);
      return;
    }

    writeChunk(response, `已收到：${prompt}`);
    setTimeout(() => {
      writeChunk(response, "\n\n处理完成。");
      finish(response);
    }, 700);
  });
});

server.listen(port, "127.0.0.1");
