import fs from "node:fs";
import path from "node:path";
import { e2eRoot } from "../fixtures/env.js";
import { test, expect } from "../fixtures/test.js";
import { users } from "../fixtures/users.js";
import { LoginPage } from "../pages/LoginPage.js";

test("@p0 auth setup writes admin storage state", async ({ page }) => {
  const loginPage = new LoginPage(page);
  await test.step("open login page", async () => {
    await loginPage.goto();
    await loginPage.expectVisible();
  });

  await test.step("login as seeded admin", async () => {
    await loginPage.login(users.admin.username, users.admin.password);
    await expect
      .poll(() => page.evaluate(() => window.localStorage.getItem("access_token")))
      .toBeTruthy();
  });

  await test.step("write storage state", async () => {
    const statePath = path.join(e2eRoot, users.admin.storageState);
    fs.mkdirSync(path.dirname(statePath), { recursive: true });
    await page.context().storageState({ path: statePath });
  });
});

