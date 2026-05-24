package runtimekernel

import "fmt"

type AIOpsCompactPromptInput struct {
	SessionID string
	TurnID    string
}

// BuildAIOpsCompactPrompt returns the no-tools L4 prompt used to summarize old
// AIOps context while preserving operational continuity.
func BuildAIOpsCompactPrompt(input AIOpsCompactPromptInput) string {
	return fmt.Sprintf(`CRITICAL: Respond with TEXT ONLY. Do NOT call tools.

请为 AIOps 长会话生成可继续工作的上下文摘要。SessionID: %s TurnID: %s

必须保留以下 10 个 AIOps 维度：
1. 用户当前目标和最新约束
2. 当前事故 / 服务 / 主机 / 时间窗
3. 已确认事实和 evidenceRefs
4. 已排除假设
5. 当前最可能 root cause / hypotheses
6. 已执行工具和关键结果摘要
7. pending approvals / denied approvals / action token 状态
8. Runner / OpsManual / MCP / Skills 当前状态
9. 下一步
10. 用户明确反馈和偏好

必须包含 transcript/ref 提示：
- 保留可回读 transcript 边界和 external reference IDs。
- 不要复制大段原始日志、指标序列、trace、文件内容或敏感字段。
- 如信息来自外部引用，请写 evidenceRefs / externalRefs，而不是展开原文。

输出格式：
<summary>
...
</summary>`, input.SessionID, input.TurnID)
}
