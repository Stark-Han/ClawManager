import { LoginPage } from "../../pages/LoginPage.js";
import { test, expect } from "../../fixtures/test.js";
import { users } from "../../fixtures/users.js";

test.use({ storageState: undefined });

test("@p0 login page renders", async ({ page }) => {
  const loginPage = new LoginPage(page);

  await loginPage.goto();
  await loginPage.expectVisible();
});

test("@p0 admin can log in", async ({ page }) => {
  const loginPage = new LoginPage(page);

  await loginPage.goto();
  await loginPage.login(users.admin.username, users.admin.password);

  await expect
    .poll(() => page.evaluate(() => window.localStorage.getItem("access_token")))
    .toBeTruthy();
});

test("@p0 failed login shows an error", async ({ page }) => {
  const loginPage = new LoginPage(page);

  await loginPage.goto();
  await loginPage.login(users.admin.username, "wrong-password");

  await expect(page.getByText(/invalid username or password|login failed/i)).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

