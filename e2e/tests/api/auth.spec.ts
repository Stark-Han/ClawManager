import { getMe, getMeRaw, login } from "../../fixtures/apiClient.js";
import { test, expect } from "../../fixtures/test.js";
import { users } from "../../fixtures/users.js";

test("@p0 auth login returns tokens", async ({ request }) => {
  const tokens = await login(request);

  expect(tokens.access_token).toBeTruthy();
  expect(tokens.refresh_token).toBeTruthy();
  expect(tokens.expires_in).toBeGreaterThan(0);
});

test("@p0 auth me returns current user with valid token", async ({ request }) => {
  const tokens = await login(request);
  const user = await getMe(request, tokens.access_token);

  expect(user.username).toBe(users.admin.username);
  expect(user.role).toBe(users.admin.role);
});

test("@p0 auth me rejects unauthenticated requests", async ({ request }) => {
  const response = await getMeRaw(request);

  expect(response.status()).toBe(401);
});

