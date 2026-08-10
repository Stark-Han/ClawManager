import mysql from "mysql2/promise";
import { env } from "../fixtures/env.js";
import { sleep, waitForTcpPort } from "./wait-for.js";

async function main() {
  await waitForTcpPort(env.db.host, env.db.port, 120_000);

  let connection: mysql.Connection | undefined;
  let lastError: unknown;

  for (let attempt = 1; attempt <= 30; attempt += 1) {
    try {
      connection = await mysql.createConnection({
        host: env.db.host,
        port: env.db.port,
        user: env.db.user,
        password: env.db.password,
        multipleStatements: false
      });
      break;
    } catch (error) {
      lastError = error;
      await sleep(2000);
    }
  }

  if (!connection) {
    throw new Error(
      `failed to connect to MySQL: ${
        lastError instanceof Error ? lastError.message : String(lastError)
      }`
    );
  }

  try {
    await connection.query(`DROP DATABASE IF EXISTS \`${env.db.database}\``);
    await connection.query(
      `CREATE DATABASE \`${env.db.database}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`
    );
    console.log(`[e2e] reset database ${env.db.database}`);
  } finally {
    await connection.end();
  }
}

main().catch((error: unknown) => {
  console.error("[e2e] failed to reset database");
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
