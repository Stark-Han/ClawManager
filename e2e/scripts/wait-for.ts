import net from "node:net";

export function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export async function waitForTcpPort(
  host: string,
  port: number,
  timeoutMs = 60_000
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastError: unknown;

  while (Date.now() < deadline) {
    try {
      await new Promise<void>((resolve, reject) => {
        const socket = net.createConnection({ host, port });
        socket.setTimeout(2000);
        socket.once("connect", () => {
          socket.end();
          resolve();
        });
        socket.once("timeout", () => {
          socket.destroy();
          reject(new Error(`timeout waiting for ${host}:${port}`));
        });
        socket.once("error", reject);
      });
      return;
    } catch (error) {
      lastError = error;
      await sleep(1000);
    }
  }

  throw new Error(
    `timed out waiting for ${host}:${port}: ${
      lastError instanceof Error ? lastError.message : String(lastError)
    }`
  );
}

export async function waitForHttpStatus(
  url: string,
  expectedStatus = 200,
  timeoutMs = 60_000
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastStatus = 0;
  let lastError = "";

  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      lastStatus = response.status;
      if (response.status === expectedStatus) {
        return;
      }
    } catch (error) {
      lastError = error instanceof Error ? error.message : String(error);
    }
    await sleep(1000);
  }

  throw new Error(
    `timed out waiting for ${url}; last status=${lastStatus}; last error=${lastError}`
  );
}

