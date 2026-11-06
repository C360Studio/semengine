# Owner questions: setup-04a-04-graph-roots-graph-query-search

## 1. Leave the "find similar" search to step 5?

The plan for step 4 listed a semantic strategy, the search that ranks entities by meaning. At the pin only a language
model's answer could send a question there; with the keyword classifier, "find sensors similar to pump-42" went to the
general search instead (probe P1, `notes/probes/step4-pin-probe.log`). Step 5 ports that general search, whose first
move is the same meaning search, and the language-model classifier (#486) is tier 2.

**Proposal:** step 4 does not carry it. "Similar to" questions get the step-4 "not in this build" refusal, and step 5
decides how they are answered. Cost: about 125 pin lines move to step 5; nothing a tier-0 caller could reach is lost.
The other choice is to send "similar" questions to it now; it would still refuse until change 5 brings the embedding
responder.

Answer: accept, or send "similar" questions to the meaning search in step 4.
