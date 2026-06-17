export function capabilitySetupHref(kind: unknown) {
  const capabilityKind = String(kind || "").trim();
  return capabilityKind ? `/capabilities?kind=${encodeURIComponent(capabilityKind)}` : "/capabilities";
}
