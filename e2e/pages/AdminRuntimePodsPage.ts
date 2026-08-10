import type { Page } from "@playwright/test";
import type { RuntimePodRegisterRequest } from "../fixtures/apiClient.js";
import { expect } from "../fixtures/test.js";

export class AdminRuntimePodsPage {
  constructor(private readonly page: Page) {}

  private main() {
    return this.page.getByRole("main");
  }

  private podCard(podName: string) {
    return this.main().locator("article", { hasText: podName }).first();
  }

  private formatRuntimeType(runtimeType: RuntimePodRegisterRequest["runtime_type"]) {
    return runtimeType === "hermes" ? "Hermes" : "OpenClaw";
  }

  async goto() {
    await this.page.goto("/admin/runtime-pods");
  }

  async expectVisible() {
    await expect(this.page.getByRole("heading", { name: /^runtime$/i })).toBeVisible();
  }

  async expectEmptyState() {
    await this.expectVisible();
    await expect(this.main().getByText("No active runtime pods")).toBeVisible();
  }

  async selectRuntimeFilter(label: "All" | "OpenClaw" | "Hermes") {
    await this.main().getByRole("button", { name: label }).click();
  }

  async expectPodVisible(podName: string) {
    await this.expectVisible();
    const card = this.podCard(podName);

    await expect(card).toBeVisible();
    await expect(card.getByRole("heading", { name: podName, exact: true })).toBeVisible();
  }

  async expectPodStatusAndCapacity(pod: RuntimePodRegisterRequest) {
    await this.expectPodVisible(pod.pod_name);
    const card = this.podCard(pod.pod_name);

    await expect(card.getByText(this.formatRuntimeType(pod.runtime_type), { exact: true })).toBeVisible();
    await expect(card.getByText(pod.draining ? "draining" : pod.state, { exact: true })).toBeVisible();
    await expect(card.getByText("Slots", { exact: true })).toBeVisible();
    await expect(card.getByText(`${pod.used_slots} / ${pod.capacity}`, { exact: true })).toBeVisible();
  }

  async expectPodMetricsVisible(podName: string) {
    const card = this.podCard(podName);

    await expect(card.getByText("CPU", { exact: true })).toBeVisible();
    await expect(card.getByText("Memory", { exact: true })).toBeVisible();
    await expect(card.getByText("Disk", { exact: true })).toBeVisible();
    await expect(card.getByText("Network", { exact: true })).toBeVisible();
    await expect(card.getByText(/Last seen/i)).toBeVisible();
  }
}
