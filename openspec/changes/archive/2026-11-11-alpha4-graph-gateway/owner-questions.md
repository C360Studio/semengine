# Owner questions: alpha4-graph-gateway

## 1. Where does `trajectory` come from?

**Owner answer (2026-11-10, in Enjoy): drop `trajectory` from SemEngine's gateway; SemTeams mounts its own
component on SemEngine's HTTP server and serves it there.**

**Recommendation: let SemTeams plug `trajectory` into the SemEngine gateway itself, through a small hook. SemEngine then
carries no agent-trajectory code or schema.**

Nothing in SemEngine answers a trajectory request. In SemStreams the gateway sent it to the agent loop, which SemEngine
leaves out and SemTeams is taking over. The trajectory answer is all agent detail: token counts, model and tool names,
step kinds. If SemEngine owned it, the engine would carry agent semantics and depend on the agent subject, and your
ruling said no agentic dependency.

With the hook, SemTeams' startup code tells the gateway "also answer `trajectory`, here is its GraphQL schema text, ask
this NATS subject." The gateway checks the request against that schema and returns only the fields asked for. SemTeams'
UI keeps working, its query unchanged. SemEngine adds one small exported type and one registration function, and each
added operation shows up as a declared output of the gateway, so startup checks see SemTeams' subject like any other
wiring. That is new public surface, which is why I am asking.

The other two choices:

- SemEngine carries the trajectory schema and forwards it to SemTeams' subject. Simpler for SemTeams, but agent meaning
  lives in the engine.
- Drop `trajectory` from SemEngine's gateway. SemTeams answers it on a route of its own. That goes against your ruling.
