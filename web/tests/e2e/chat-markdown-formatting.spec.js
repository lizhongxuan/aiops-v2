// @ts-check
import { test, expect } from "@playwright/test";
import {
  createChatFixtureSessions,
  createChatFixtureState,
  openFixturePage,
} from "../helpers/uiFixtureHarness";

const MARKDOWN_WITH_HEADERS_AND_LISTS = `## Nginx 状态检查结果

### 当前状态
- nginx 进程正常运行，PID: 12345
- upstream 连接数: 42/100
- 错误率: 0.3%

### 建议操作
1. 检查 service-a 的 upstream timeout 配置
2. 调整 \`proxy_read_timeout\` 为 60s
3. 重启 nginx 以应用配置

\`\`\`bash
sudo nginx -t && sudo systemctl reload nginx
\`\`\``;

const MARKDOWN_WITH_TABLE = `## 服务健康状态

| 服务 | 状态 | 延迟 | 错误率 |
|------|------|------|--------|
| nginx | 正常 | 12ms | 0.1% |
| service-a | 告警 | 350ms | 2.3% |
| redis | 正常 | 3ms | 0% |

建议优先处理 service-a 的延迟问题。`;

const MARKDOWN_WITH_CODE_BLOCKS = `检查结果如下:

\`\`\`json
{
  "status": "healthy",
  "uptime": "72h",
  "connections": {
    "active": 42,
    "waiting": 8
  }
}
\`\`\`

配置文件中的关键参数:

\`\`\`nginx
upstream backend {
    server 10.0.0.1:8080 weight=5;
    server 10.0.0.2:8080 weight=3;
    keepalive 32;
}
\`\`\``;

const DENSE_CHINESE_PARAGRAPH = "经过检查，nginx 服务当前运行正常。主要指标如下：CPU 使用率 12%，内存占用 256MB，活跃连接数 42。upstream 后端 service-a 出现间歇性超时，建议调整 proxy_read_timeout 参数。同时建议开启 access_log 的 buffer 模式以减少磁盘 IO。";

const MIXED_CHINESE_MARKDOWN = `## 诊断结果

nginx 中间件整体运行正常，以下是详细分析：

### 1. 进程状态
- 主进程 PID: 12345，worker 进程数: 4
- 内存占用: 256MB（正常范围）

### 2. 异常发现
service-a 的 upstream 出现超时：
\`\`\`
2026/04/03 10:15:23 [error] upstream timed out (110: Connection timed out)
\`\`\`

### 3. 建议
1. 将 \`proxy_read_timeout\` 从 30s 调整为 60s
2. 添加健康检查配置
3. 考虑增加 upstream 节点`;

const INLINE_FORMATTING = `检查完成。**nginx** 服务正常，但 \`service-a\` 的 upstream 有 *间歇性超时*。

关键发现:
- 错误日志中有 **23 条** timeout 记录
- 最近一次发生在 \`2026-04-03 10:15:23\`
- 影响范围: ~2.3% 的请求`;

function buildMarkdownTestCards(markdownTexts) {
  const cards = [];
  markdownTexts.forEach((item, index) => {
    cards.push({
      id: `user-md-${index}`,
      type: "UserMessageCard",
      role: "user",
      text: item.question,
      status: "completed",
      createdAt: "2026-04-03T10:00:00Z",
      updatedAt: "2026-04-03T10:00:00Z",
    });
    cards.push({
      id: `assistant-md-${index}`,
      type: "AssistantMessageCard",
      role: "assistant",
      text: item.answer,
      status: "completed",
      createdAt: "2026-04-03T10:00:01Z",
      updatedAt: "2026-04-03T10:00:01Z",
    });
  });
  return cards;
}

function idleRuntime() {
  return {
    turn: { active: false, phase: "idle", hostId: "web-01" },
    codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
  };
}

test.describe("Chat markdown formatting — LLM output fidelity", () => {

  test("headers and lists render as proper HTML elements", async ({ page }) => {
    const cards = buildMarkdownTestCards([
      { question: "帮我看下 nginx 状态", answer: MARKDOWN_WITH_HEADERS_AND_LISTS },
    ]);
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards, runtime: idleRuntime() }),
      sessions: createChatFixtureSessions(),
    });

    const mdBody = page.locator(".aiops-message-markdown").filter({ hasText: "Nginx 状态检查结果" });
    await expect(mdBody).toBeVisible({ timeout: 10000 });

    await expect(mdBody.locator("h2").first()).toBeVisible();
    expect(await mdBody.locator("h3").count()).toBeGreaterThanOrEqual(2);
    expect(await mdBody.locator("ul li").count()).toBeGreaterThanOrEqual(3);
    expect(await mdBody.locator("ol li").count()).toBeGreaterThanOrEqual(3);
    await expect(mdBody.locator("pre code").first()).toBeVisible();
  });

  test("table renders as HTML table, not raw pipe characters", async ({ page }) => {
    const cards = buildMarkdownTestCards([
      { question: "服务健康状态如何", answer: MARKDOWN_WITH_TABLE },
    ]);
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards, runtime: idleRuntime() }),
      sessions: createChatFixtureSessions(),
    });

    const mdBody = page.locator(".aiops-message-markdown").filter({ has: page.locator("table") });
    await expect(mdBody).toBeVisible({ timeout: 10000 });

    await expect(mdBody.locator("table")).toBeVisible();
    expect(await mdBody.locator("th").count()).toBe(4);
    expect(await mdBody.locator("tbody tr").count()).toBe(3);
  });

  test("code blocks preserve formatting", async ({ page }) => {
    const cards = buildMarkdownTestCards([
      { question: "检查 nginx 配置", answer: MARKDOWN_WITH_CODE_BLOCKS },
    ]);
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards, runtime: idleRuntime() }),
      sessions: createChatFixtureSessions(),
    });

    const mdBody = page.locator(".aiops-message-markdown").filter({ hasText: "检查结果如下" });
    await expect(mdBody).toBeVisible({ timeout: 10000 });

    const codeBlocks = mdBody.locator("pre code");
    expect(await codeBlocks.count()).toBe(2);

    const jsonText = await codeBlocks.first().textContent();
    expect(jsonText).toContain('"status"');
    expect(jsonText).toContain('"healthy"');
  });

  test("dense Chinese paragraph renders as readable text", async ({ page }) => {
    const cards = buildMarkdownTestCards([
      { question: "nginx 状态怎么样", answer: DENSE_CHINESE_PARAGRAPH },
    ]);
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards, runtime: idleRuntime() }),
      sessions: createChatFixtureSessions(),
    });

    const mdBody = page.locator(".aiops-message-markdown").filter({ hasText: "经过检查" });
    await expect(mdBody).toBeVisible({ timeout: 10000 });

    const html = await mdBody.innerHTML();
    expect(html).toContain("经过检查");
    expect(html).toContain("nginx");
  });

  test("mixed Chinese markdown preserves structure without double-processing", async ({ page }) => {
    const cards = buildMarkdownTestCards([
      { question: "给我完整诊断", answer: MIXED_CHINESE_MARKDOWN },
    ]);
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards, runtime: idleRuntime() }),
      sessions: createChatFixtureSessions(),
    });

    const mdBody = page.locator(".aiops-message-markdown").filter({ hasText: "诊断结果" });
    await expect(mdBody).toBeVisible({ timeout: 10000 });

    await expect(mdBody.locator("h2").first()).toBeVisible();
    expect(await mdBody.locator("h3").count()).toBeGreaterThanOrEqual(3);
    expect(await mdBody.locator("li").count()).toBeGreaterThanOrEqual(5);
    await expect(mdBody.locator("pre code").first()).toBeVisible();

    const h3Texts = await mdBody.locator("h3").allTextContents();
    expect(h3Texts.some(t => t.includes("进程状态"))).toBe(true);
    expect(h3Texts.some(t => t.includes("异常发现"))).toBe(true);
    expect(h3Texts.some(t => t.includes("建议"))).toBe(true);
  });

  test("inline formatting (bold, code, italic) renders correctly", async ({ page }) => {
    const cards = buildMarkdownTestCards([
      { question: "检查结果", answer: INLINE_FORMATTING },
    ]);
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards, runtime: idleRuntime() }),
      sessions: createChatFixtureSessions(),
    });

    const mdBody = page.locator(".aiops-message-markdown").filter({ hasText: "检查完成" });
    await expect(mdBody).toBeVisible({ timeout: 10000 });

    expect(await mdBody.locator("strong").count()).toBeGreaterThanOrEqual(2);
    expect(await mdBody.locator("code").count()).toBeGreaterThanOrEqual(2);
    expect(await mdBody.locator("em").count()).toBeGreaterThanOrEqual(1);
  });

  test("routing metadata is cleaned before rendering", async ({ page }) => {
    // Test with inline routing JSON (not fenced code block to avoid rendering crash)
    const textWithRouting = '{"route": "direct", "confidence": 0.95}\n\n## 检查结果\nnginx 运行正常，没有发现异常。';
    const cards = buildMarkdownTestCards([
      { question: "nginx 正常吗", answer: textWithRouting },
    ]);
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards, runtime: idleRuntime() }),
      sessions: createChatFixtureSessions(),
    });

    // Wait for the user message to confirm page loaded
    await expect(page.locator(".aiops-message-markdown").filter({ hasText: "nginx 正常吗" })).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1000);

    // The routing JSON should be cleaned by cleanDisplayText
    const bodyText = await page.textContent("body");
    expect(bodyText).not.toContain('"route"');
    // Actual content should be preserved
    expect(bodyText).toContain("nginx 运行正常");
  });

  test("screenshot: markdown rendering fidelity across all formats", async ({ page }) => {
    const cards = buildMarkdownTestCards([
      { question: "帮我看下 nginx 状态", answer: MARKDOWN_WITH_HEADERS_AND_LISTS },
      { question: "服务健康状态", answer: MARKDOWN_WITH_TABLE },
      { question: "检查配置", answer: MARKDOWN_WITH_CODE_BLOCKS },
      { question: "完整诊断", answer: MIXED_CHINESE_MARKDOWN },
      { question: "简要结果", answer: INLINE_FORMATTING },
      { question: "密集段落测试", answer: DENSE_CHINESE_PARAGRAPH },
    ]);
    await page.setViewportSize({ width: 1440, height: 1100 });
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards, runtime: idleRuntime() }),
      sessions: createChatFixtureSessions(),
    });

    await page.waitForTimeout(1000);
    const firstMd = page.locator(".aiops-message-markdown").first();
    await expect(firstMd).toBeVisible({ timeout: 10000 });

    await page.screenshot({
      path: "tests/screenshots/markdown-formatting-fidelity.png",
      fullPage: true,
    });
  });
});
