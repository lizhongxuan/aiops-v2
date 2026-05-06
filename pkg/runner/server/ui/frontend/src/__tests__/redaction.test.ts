import { describe, expect, it } from "vitest";
import { redactSensitiveValues, stringifyRedacted } from "../utils/redaction";

describe("redaction", () => {
  it("redacts sensitive keys recursively without mutating source data", () => {
    const source = {
      restore_lsn: "0/42",
      API_TOKEN: "plain-token",
      nested: {
        password: "secret-password",
        safe: "visible",
      },
      list: [{ private_key: "pem" }, "keep"],
    };

    const redacted = redactSensitiveValues(source);

    expect(redacted).toEqual({
      restore_lsn: "0/42",
      API_TOKEN: "[redacted]",
      nested: {
        password: "[redacted]",
        safe: "visible",
      },
      list: [{ private_key: "[redacted]" }, "keep"],
    });
    expect(source.API_TOKEN).toBe("plain-token");
    expect(source.nested.password).toBe("secret-password");
  });

  it("stringifies redacted display payloads", () => {
    const text = stringifyRedacted({ access_key: "ak", duration_ms: 42 });

    expect(text).toContain('"access_key": "[redacted]"');
    expect(text).toContain('"duration_ms": 42');
    expect(text).not.toContain("ak");
  });
});
