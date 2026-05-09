// @ts-check
import { expect } from "@playwright/test";

export async function expectStablePageScreenshot(page, name, options = {}) {
  await expect(page).toHaveScreenshot(name, {
    animations: "disabled",
    caret: "hide",
    fullPage: false,
    scale: "css",
    ...options,
  });
}

export async function expectStableLocatorScreenshot(locator, name, options = {}) {
  await expect(locator).toHaveScreenshot(name, {
    animations: "disabled",
    caret: "hide",
    scale: "css",
    ...options,
  });
}
