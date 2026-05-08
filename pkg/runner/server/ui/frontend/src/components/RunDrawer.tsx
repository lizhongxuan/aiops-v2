import { useEffect, useMemo, useState } from "react";
import type { DryRunResult, ValidationResult } from "../types/workflow";
import { stringifyRedacted } from "../utils/redaction";
import type { RunLogLine, RunState } from "../utils/runEventReducer";

interface RunDrawerProps {
  run: RunState;
  validation: ValidationResult | null;
  dryRun: DryRunResult | null;
  error: string | null;
  eventConnected: boolean;
  replaying: boolean;
  approvalNodes: Array<{ id: string; label: string }>;
  resolvingApprovalNodeId: string | null;
  resolvingApprovalAction: "approve" | "reject" | null;
  onReplayRun: (runId: string) => void;
  onApproveNode: (nodeId: string, comment: string) => void;
  onRejectNode: (nodeId: string, comment: string) => void;
}

export default function RunDrawer(props: RunDrawerProps) {
  const [activeTab, setActiveTab] = useState("events");
  const [replayRunId, setReplayRunId] = useState("");
  const [approvalComments, setApprovalComments] = useState<Record<string, string>>({});
  const stdoutText = useMemo(() => formatLogLines(props.run.stdout), [props.run.stdout]);
  const stderrText = useMemo(() => formatLogLines(props.run.stderr), [props.run.stderr]);
  const exportedVarsText = useMemo(() => stringifyRedacted(props.run.exportedVars), [props.run.exportedVars]);
  const runnerDebugText = useMemo(() => stringifyRedacted(props.run.runnerDebug), [props.run.runnerDebug]);
  const simulatedPaths = props.dryRun?.path_simulation?.paths ?? [];
  const hasLogs = props.run.stdout.length > 0 || props.run.stderr.length > 0 || props.run.timeline.length > 0;

  useEffect(() => {
    if (props.run.runId) setReplayRunId(props.run.runId);
  }, [props.run.runId]);

  function replay() {
    const runId = replayRunId.trim();
    if (runId) props.onReplayRun(runId);
  }

  function exportLogs() {
    if (!hasLogs) return;
    const sections = [
      `run_id=${props.run.runId || ""}`,
      `status=${props.run.status}`,
      "",
      "[timeline]",
      ...props.run.timeline
        .slice()
        .reverse()
        .map((item) => [item.timestamp, item.type, item.status, item.nodeId || item.edgeId, item.message].filter(Boolean).join(" | ")),
      "",
      "[stdout]",
      stdoutText || "",
      "",
      "[stderr]",
      stderrText || "",
    ];
    const blob = new Blob([sections.join("\n")], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${sanitizeFileName(props.run.runId || "runner-run")}.log`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  }

  return (
    <section className={`run-drawer ${props.approvalNodes.length ? "has-approvals" : ""}`}>
      <div className="run-summary">
        <div>
          <span>◌</span>
          <span>Run</span>
          <span className="tag">{props.run.status}</span>
          {props.eventConnected ? <span className="tag tag-success">SSE</span> : null}
        </div>
        <div className="run-history-controls">
          <input value={replayRunId} placeholder="run id" onChange={(event) => setReplayRunId(event.target.value)} onKeyDown={(event) => event.key === "Enter" && replay()} />
          <button type="button" disabled={!replayRunId.trim() || props.replaying} onClick={replay}>
            Replay
          </button>
          <button type="button" disabled={!hasLogs} onClick={exportLogs}>
            Export logs
          </button>
        </div>
      </div>

      {props.approvalNodes.length ? (
        <div className="approval-strip">
          {props.approvalNodes.map((node) => (
            <article key={node.id} className="approval-item">
              <div className="approval-node">
                <span>✓</span>
                <strong>{node.label}</strong>
                <code>{node.id}</code>
              </div>
              <input
                value={approvalComments[node.id] || ""}
                placeholder="approval comment"
                maxLength={160}
                onChange={(event) => setApprovalComments({ ...approvalComments, [node.id]: event.target.value })}
              />
              <button type="button" disabled={isResolving(props, node.id, "approve")} onClick={() => props.onApproveNode(node.id, approvalComments[node.id] || "")}>
                Approve
              </button>
              <button type="button" disabled={isResolving(props, node.id, "reject")} onClick={() => props.onRejectNode(node.id, approvalComments[node.id] || "")}>
                Reject
              </button>
            </article>
          ))}
        </div>
      ) : null}

      <div className="tabs run-tabs">
        <TabButtons active={activeTab} tabs={["events", "hosts", "logs", "vars", "debug"]} onChange={setActiveTab} />
        {activeTab === "events" ? (
          <div className="timeline">
            {props.error ? <TimelineItem type="request_error" message={props.error} error /> : null}
            {props.validation ? <TimelineItem type="validation" message={props.validation.summary || (props.validation.valid ? "Graph is valid" : "Graph has errors")} time={props.validation.valid ? "valid" : `${props.validation.errors.length} errors`} /> : null}
            {props.dryRun ? (
              <article className="timeline-item">
                <span>○</span>
                <div>
                  <strong>dry_run</strong>
                  <span>{props.dryRun.summary || `${props.dryRun.steps_count} steps / ${props.dryRun.target_hosts.length} hosts`}</span>
                  {simulatedPaths.length ? (
                    <ul className="path-simulation-list">
                      {simulatedPaths.slice(0, 6).map((path, index) => (
                        <li key={`${path.terminal_node_id || index}-${path.edge_ids.join("-")}`}>
                          <code>{path.node_ids.join(" -> ")}</code>
                          <span className="tag">{path.status}</span>
                        </li>
                      ))}
                    </ul>
                  ) : null}
                </div>
                <time>{props.dryRun.workflow_name || ""}</time>
              </article>
            ) : null}
            {props.run.timeline.map((item) => (
              <TimelineItem key={item.id} type={item.type} message={item.message || item.nodeId || item.edgeId || "Event received"} time={item.timestamp || ""} />
            ))}
          </div>
        ) : null}
        {activeTab === "hosts" ? (
          <div className="run-panel">
            <table className="host-result-table">
              <thead>
                <tr>
                  <th>Step</th>
                  <th>Host</th>
                  <th>Status</th>
                  <th>Exit</th>
                  <th>Message</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {props.run.hostResults.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="empty-cell">No host result yet.</td>
                  </tr>
                ) : props.run.hostResults.map((row) => (
                  <tr key={row.id}>
                    <td>{row.step || "-"}</td>
                    <td>{row.host || "-"}</td>
                    <td><span className={`tag ${statusClass(row.status)}`}>{row.status || "unknown"}</span></td>
                    <td>{row.exitCode ?? "-"}</td>
                    <td>{row.message || "-"}</td>
                    <td>{row.timestamp || "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
        {activeTab === "logs" ? (
          <div className="run-panel log-grid">
            <LogBlock title="stdout" count={props.run.stdout.length} text={stdoutText || "No stdout yet."} />
            <LogBlock title="stderr" count={props.run.stderr.length} text={stderrText || "No stderr yet."} stderr />
          </div>
        ) : null}
        {activeTab === "vars" ? <div className="run-panel"><pre className="json-panel">{Object.keys(props.run.exportedVars).length ? exportedVarsText : "No exported vars yet."}</pre></div> : null}
        {activeTab === "debug" ? <div className="run-panel"><pre className="json-panel">{Object.keys(props.run.runnerDebug).length ? runnerDebugText : "No runner_debug yet."}</pre></div> : null}
      </div>
    </section>
  );
}

function TabButtons({ active, tabs, onChange }: { active: string; tabs: string[]; onChange: (tab: string) => void }) {
  return <div className="tab-buttons">{tabs.map((tab) => <button key={tab} type="button" className={active === tab ? "is-active" : ""} onClick={() => onChange(tab)}>{tab}</button>)}</div>;
}

function TimelineItem({ type, message, time = "", error = false }: { type: string; message: string; time?: string; error?: boolean }) {
  return <article className={`timeline-item ${error ? "is-error" : ""}`}><span>○</span><div><strong>{type}</strong><span>{message}</span></div><time>{time}</time></article>;
}

function LogBlock({ title, count, text, stderr = false }: { title: string; count: number; text: string; stderr?: boolean }) {
  return <section><div className="drawer-section-heading"><strong>{title}</strong><span>{count} entries</span></div><pre className={`log-block ${stderr ? "stderr" : ""}`}>{text}</pre></section>;
}

function isResolving(props: RunDrawerProps, nodeId: string, action: "approve" | "reject") {
  return props.resolvingApprovalNodeId === nodeId && props.resolvingApprovalAction === action;
}

function formatLogLines(lines: RunLogLine[]): string {
  return [...lines].reverse().map((line) => {
    const scope = [line.timestamp, line.step, line.host].filter(Boolean).join(" ");
    return scope ? `[${scope}] ${line.content}` : line.content;
  }).join("\n");
}

function sanitizeFileName(value: string): string {
  return value.replace(/[^a-zA-Z0-9._-]/g, "_");
}

function statusClass(status?: string) {
  switch ((status || "").toLowerCase()) {
    case "success":
      return "tag-success";
    case "failed":
    case "error":
      return "tag-error";
    case "running":
      return "tag-warning";
    default:
      return "";
  }
}
