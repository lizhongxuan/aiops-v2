# aiops-v2 自优化系统测试报告

日期：2026-05-23
范围：独立 `selfopt/` 子系统、standalone wrapper、报告读取器、静态 dashboard、Playwright 与 browser-in-app 可视化验证。
安全边界：本轮未调用真实 LLM，未写入 LLM API key，未连接远程测试主机，未执行真实 Kubernetes/Coroot 修复动作。

## 本轮覆盖

- 独立 CLI：`go run ./selfopt/cmd/selfopt`
- Wrapper：`./scripts/self-optimization-lab.sh --standalone`
- 真实 aiops-v2 本地测试链路：`./scripts/self-optimization-lab.sh --standalone --real-aiops-tests`
- 报告读取：`selfopt/reportreader`
- Dashboard：`selfopt` 生成的 `dashboard/index.html`
- Playwright dashboard smoke：`web/tests/e2e/self-optimization-dashboard.spec.js`
- browser-in-app 真实页面渲染检查

## TDD 记录

### reportreader 红灯

命令：

```bash
go test ./selfopt/reportreader -count=1
```

结果：预期失败，`ReadLatest` 尚未实现。

### reportreader 绿灯

命令：

```bash
go test ./selfopt/reportreader -count=1
```

结果：通过。覆盖读取 `latest_run.txt`、`prompt-regression-*/eval/report.json`、同级 `diagnosis.json`，并聚合 failed cases。

补充回归测试：

- `movement=worse` 但 `passed=true` 的 case 必须进入退化列表。
- 没有任何 `prompt-regression-*/eval/report.json` 时必须返回错误，不能空报告假通过。
- `--llm-suggestions` 未同时提供 `--allow-real-llm` 时 wrapper 必须失败。
- lab LLM API key 不能通过命令行参数传给下游脚本或诊断进程。
- `--real-aiops-run-dir` 导入的真实 prompt regression 报告必须进入 `aiops-test-summary.json`、`scorecard.json` 和 gate。
- `--real-aiops-tests` 必须真实调用现有 `prompt-regression.sh`，并把产物交给 selfopt 汇总。

## 自动化验证

### Go 单元测试

命令：

```bash
gofmt -w selfopt
go test ./selfopt ./selfopt/cmd/selfopt ./selfopt/reportreader -count=1
```

结果：通过。

### Standalone wrapper

命令：

```bash
./scripts/self-optimization-lab.sh --standalone --max-runs 1 --out <tmpdir> --skip-go-tests --skip-core --skip-synthetic
```

结果：通过。wrapper 调用 `selfopt/cmd/selfopt`，生成 manifest、scorecard、case scores、baseline comparison、impact matrix、regression report、dashboard 和 candidate asset draft。

### 真实 aiops-v2 测试接入

命令：

```bash
./scripts/self-optimization-lab.sh --standalone --real-aiops-tests --skip-go-tests --skip-core \
  --synthetic-cases <tmpcases> --out <tmpdir> --max-runs 1 --no-asset-draft
```

结果：通过。wrapper 调用现有 `prompt-regression.sh`，`prompt-regression.sh` 调用 `cmd/agent-eval` 与 `cmd/prompt-diagnose`，selfopt 再读取生成的 `prompt-regression-synthetic/eval/report.json` 和 `diagnosis.json`，写入 `aiops-test-summary.json` 并纳入 `scorecard.aiopsTests` 与 gate。

### Playwright dashboard smoke

命令：

```bash
cd web
SELFOPT_DASHBOARD_FILE=<tmpdir>/pw/dashboard/index.html PLAYWRIGHT_SKIP_WEB_SERVER=1 \
  SELFOPT_DASHBOARD_REQUIRED=1 \
  npx playwright test tests/e2e/self-optimization-dashboard.spec.js --project=chromium
```

结果：通过。断言 dashboard 包含 `Self Optimization Lab`、`Overview`、run overall score、`Timeline`、`Safety`、gate decision、`Impact`。

### browser-in-app 可视化检查

步骤：

- 用本地 HTTP server 暴露生成的 `dashboard/index.html`
- 用 Codex in-app browser 打开页面
- 读取页面正文并保存首屏截图

结果：

- 页面标题：`Self Optimization Lab`
- 文本断言：`Overview`、`Timeline`、`Safety`、`Impact` 全部存在
- 截图：`/tmp/aiops-selfopt-dashboard.png`

## 发现与修复

- Dashboard 初版只有数据标签，没有显式 section heading，Playwright 对 `overview/timeline/safety/impact` 的用户可见断言失败。已补齐 `Overview`、`Timeline`、`Safety`、`Impact`。
- browser-in-app 初次截图不可读，原因是页面没有显式 light theme，受浏览器默认色彩影响。已在 dashboard 模板中加入浅色背景、正文颜色、section 边框和标签样式。
- reportreader 初版会漏掉 `passed=true` 但 `movement=worse` 的退化 case，并且空 report 目录会静默成功。已补齐回归浮出和空报告错误。
- legacy wrapper 初版的 `--llm-suggestions` 可能沿用生产 `AIOPS_LLM_*`。已改为必须显式 `--allow-real-llm`，且由 `prompt-regression.sh` 优先读取 `AIOPS_LAB_LLM_*`。
- LLM key 初版会作为 `--llm-api-key` 命令行参数继续传递。已改为通过进程环境交给 `prompt-diagnose`，避免出现在 argv 和脚本日志里。
- standalone 初版只跑离线 scorer。已新增 `--real-aiops-tests` 和 `--real-aiops-run-dir`，把真实 aiops-v2 本地 prompt regression 结果接入 scorecard/gate。

## 剩余风险

- 已添加 dashboard smoke，但完整 AI Chat journey runner、K8s 安装闭环和 Coroot RCA 修复闭环仍待接入真实页面流程。
- 本轮没有启用真实 LLM 或远程主机模式；对应能力需要后续在显式开关和审批边界下单独验证。
