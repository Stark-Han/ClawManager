import { expect, test as base } from "@playwright/test";
import { env } from "./env.js";

export const test = base.extend({
  baseURL: async ({}, use) => {
    await use(env.frontendUrl);
  }
});

export { expect };

