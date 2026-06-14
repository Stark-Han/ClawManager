import { login, registerUser } from "../../fixtures/apiClient.js";
import { env } from "../../fixtures/env.js";
import { test, expect } from "../../fixtures/test.js";
import { users } from "../../fixtures/users.js";

test("@p0 unauthenticated admin route redirects to login", async ({ page }) => {
  await page.goto("/admin/runtime-pods");

  await expect(page).toHaveURL(/\/login$/);
});

test("@p0 @local-only non-admin user cannot open admin routes", async ({ page, request }) => {
  await registerUser(request, users.user);
  const tokens = await login(request, users.user);

  await page.addInitScript(
    ({ accessToken, refreshToken }) => {
      window.localStorage.setItem("access_token", accessToken);
      window.localStorage.setItem("refresh_token", refreshToken);
    },
    { accessToken: tokens.access_token, refreshToken: tokens.refresh_token }
  );

  await page.goto(`${env.frontendUrl}/admin/runtime-pods`);

  await expect(page).toHaveURL(/\/dashboard$/);
});

test.describe("admin access", () => {
  test.use({ storageState: users.admin.storageState });

  test("@p0 admin can open runtime pods route", async ({ page }) => {
    await page.goto("/admin/runtime-pods");

    await expect(page.getByRole("heading", { name: /^runtime$/i })).toBeVisible();
  });
});
