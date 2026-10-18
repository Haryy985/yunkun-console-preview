# Research OS Execution Layer Reference

The cognitive type system (OBSERVATION -> FACT -> HYPOTHESIS -> CONCLUSION) and
scientific experiment framework define HOW to think about security analysis.
The Research OS architecture defines HOW to execute it safely and verifiably.

## Key Components

### Policy Enforcement Point (PEP)
Default DENY. Six-layer intersection: Agent Capability x Task Permission x Scope x Time x Budget x Policy Rule.
Unknown actions default DENY. Path normalization prevents symlink escapes.
Implementation: /root/workspaces/research-os/kernel/policy_engine/pep.py

### Execution Gateway
All tool calls through single gateway. Each execution requires:
run_id, task_id, agent_id, capability_token, scope_id, plan_hash, budget, evidence_sink, idempotency_key.
Any missing = reject. Direct call detection blocks subprocess.Popen, os.system, eval/exec, etc.

### Secret Broker
Agents hold credential_ref (opaque handle), not plaintext credentials.
Host-bound + time-bound + revocable. Secrets filtered from event store, logs, context, skills.

### Trust Isolation
External data = UNTRUSTED. Cannot modify policy, issue tokens, expand scope, modify SOUL.md,
override instructions. Prompt injection detection via 12 bidirectional patterns (EN + CN).

### Evidence Integrity
Hash chain: raw -> analysis -> review -> finding. SHA-256 at each step. Tamper detection via chain verification.

### Event Store
Append-only events.jsonl. Supports replay (--since, --agent, --event_type) and snapshot.
Complete state reconstruction at any point.

## Directory Reference

- Kernel code: /root/workspaces/research-os/kernel/
- Policy files: /root/workspaces/research-os/policy/
- CTF profiles: /root/workspaces/research-os/ctf/profiles/
- Capability packs: /root/workspaces/research-os/ctf/packs/
