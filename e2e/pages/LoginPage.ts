import type { Page } from "@playwright/test";
import { expect } from "../fixtures/test.js";

export class LoginPage {
  constructor(private readonly page: Page) {}

  async goto() {
    await this.page.goto("/login");
  }

  async expectVisible() {
    await expect(this.page.getByRole("heading", { name: /sign in to clawmanager/i })).toBeVisible();
    await expect(this.page.getByLabel(/username/i)).toBeVisible();
    await expect(this.page.getByLabel(/password/i)).toBeVisible();
  }

  async login(username: string, password: string) {
    await this.page.getByLabel(/username/i).fill(username);
    await this.page.getByLabel(/password/i).fill(password);
    await this.page.getByRole("button", { name: /^sign in$/i }).click();
  }
}

