import assert from "node:assert/strict";
import test from "node:test";
import { ReportRequestError, reportRetryAt, requireReportResponse } from "../src/lib/report-request.ts";

test("Retry-After seconds, dates, zero and missing/invalid delays", () => {
  const now = Date.parse("Tue, 27 Oct 2026 00:00:00 GMT");
  assert.equal(reportRetryAt("12", now), now + 12_000);
  assert.equal(reportRetryAt("0", now), now);
  assert.equal(reportRetryAt("Tue, 27 Oct 2026 00:00:45 GMT", now), now + 45_000);
  assert.equal(reportRetryAt("Mon, 26 Oct 2026 00:00:00 GMT", now), now);
  for (const header of [null, "", "invalid", "-1", "1.5", "Infinity", "99999999999999999999999"]) {
    assert.equal(reportRetryAt(header, now), now + 30_000, String(header));
  }
});

test("429 keeps actual HTTP status and delay without leaking query tokens", async () => {
  const before = Date.now();
  await assert.rejects(
    requireReportResponse(new Response('{"error":"fixture rate limit"}', {
      status: 429, headers: { "Retry-After": "10" },
    }), "/api/report?task=1&token=secret"),
    (error) => {
      assert.ok(error instanceof ReportRequestError);
      assert.equal(error.status, 429);
      assert.ok(error.retryAt >= before + 10_000);
      assert.match(error.message, /HTTP 429/);
      assert.match(error.message, /fixture rate limit/);
      assert.match(error.message, /接口：\/api\/report/);
      assert.doesNotMatch(error.message, /token|secret/);
      return true;
    },
  );
});

test("409 retains quality blockers instead of disguising them as rate limits", async () => {
  await assert.rejects(requireReportResponse(new Response(JSON.stringify({
    error: "quality gate", blockers: ["#7:NEEDS_MORE", "#8:REPORT_EVIDENCE_STALE", null, 12],
  }), { status: 409 }), "/api/exploration/findings/export?ids=7,8"), (error) => {
    assert.equal(error.status, 409);
    assert.equal(error.retryAt, 0);
    assert.match(error.message, /#7:NEEDS_MORE、#8:REPORT_EVIDENCE_STALE/);
    assert.doesNotMatch(error.message, /429|null/);
    return true;
  });
});

test("gateway HTML is not used as report content or exposed as error markup", async () => {
  await assert.rejects(requireReportResponse(new Response("<html>secret gateway page</html>", {
    status: 429, headers: { "Content-Type": "text/html" },
  }), "/api/report"), (error) => {
    assert.equal(error.status, 429);
    assert.ok(error.retryAt > Date.now());
    assert.doesNotMatch(error.message, /html|secret gateway/);
    return true;
  });
});

test("authentication and service errors retain their status without retry delay", async () => {
  for (const status of [401, 403, 500, 503]) {
    await assert.rejects(requireReportResponse(new Response("not JSON", { status }), "/api/report"),
      (error) => error.status === status && error.retryAt === 0 && error.message.includes(`HTTP ${status}`));
  }
});

test("malformed JSON error schema cannot throw an unrelated TypeError", async () => {
  for (const payload of [null, "failure", 123, { error: {}, blockers: "wrong" }]) {
    await assert.rejects(requireReportResponse(new Response(JSON.stringify(payload), { status: 409 }), "/api/report"),
      (error) => error instanceof ReportRequestError && error.status === 409);
  }
});

test("successful response remains unread for normal report/download handling", async () => {
  const response = new Response("# Verified fixture\n");
  assert.equal(await requireReportResponse(response, "/api/report"), response);
  assert.equal(response.bodyUsed, false);
  assert.equal(await response.text(), "# Verified fixture\n");
});
