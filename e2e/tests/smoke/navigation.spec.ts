import { test, expect } from "../../fixtures/test.js";
import { users } from "../../fixtures/users.js";

test.describe("authenticated admin navigation", () => {
  test.use({ storageState: users.admin.storageState });

  test("@p0 authenticated admin reaches workspace dashboard", async ({ page }) => {
    await page.goto("/dashboard");

    await expect(page.getByRole("heading", { name: /workspace/i })).toBeVisible();
  });

  test("@p0 authenticated admin reaches admin console", async ({ page }) => {
    await page.goto("/admin");

    await expect(page.getByRole("heading", { name: /console/i })).toBeVisible();
  });

  test("@p0 authenticated admin can reach instances, settings, and admin entry routes", async ({ page }) => {
    await page.goto("/instances");
    await expect(page.getByRole("heading", { name: /my instances/i })).toBeVisible();

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: /settings/i })).toBeVisible();
    await expect(page.getByRole("heading", { name: /change password/i })).toBeVisible();

    await page.goto("/admin");
    await expect(page.getByRole("link", { name: /^runtime$/i }).first()).toBeVisible();
    await expect(page.getByRole("link", { name: /^settings$/i }).first()).toBeVisible();

    await page.goto("/admin/runtime-pods");
    await expect(page.getByRole("heading", { name: /^runtime$/i })).toBeVisible();

    await page.goto("/admin/settings");
    await expect(page.getByRole("heading", { name: /system settings/i })).toBeVisible();
  });
});

test.describe("unauthenticated navigation", () => {
  test.use({ storageState: undefined });

  test("@p0 unauthenticated users are redirected to login", async ({ page }) => {
    await page.goto("/instances");

    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole("heading", { name: /sign in to clawmanager/i })).toBeVisible();
  });
});
