# Spec feedback from the Go port

Places where spec 0.15.0 is silent, ambiguous, or written in the shape of
the Swift port. Each item: the section, the problem, what Swift does, what
Go does, and a proposal. Nothing here changes the spec by itself; the
proposals go through the spec repository's change flow.

## 1. §4.5.4: which references count as "used" by an interface

**Problem.** §4.5.4 puts into `clus` the CLUs referenced by "the `cluId` of
any state reference or action of its pages after the transformation". The
oracle `tools/oracles/omp.py` (`used_clus`) collects only component
`state`/`actions`, the `value` of the double widgets' sides, and the
members of `object`. It misses the channels of a `MULTISENSOR` (they are
not under `object`) and the `clickAction` of a `CONTACT_SENSOR_DOUBLE`
side. A CLU referenced only there is in `clus` by the text and missing by
the oracle. No case covers it, so both pass today.

**Go.** Follows the text: every `ValueRef` and `ActionRef` of the converted
pages.

**Proposal.** Keep the text, fix the oracle, and add a project fixture
with a CLU used only by a multisensor channel.

## 2. §4.5.1 item 5: an unpaired surrogate escape in node JSON

**Problem.** The list of errors (invalid UTF-8, `NaN`, out-of-range
numbers, repeated keys, depth) does not mention `"\ud800"`, an escape that
names half a surrogate pair. Python's `json` accepts it; a Go or Swift
string cannot hold it.

**Go.** Decodes it as U+FFFD, as `encoding/json` does.

**Proposal.** Say so: "an unpaired surrogate escape decodes as U+FFFD", or
make it `invalidData`. Either is testable with one node.

## 3. §4.5.1 item 4: which XML encodings a port must read

**Problem.** "The encoding is determined by the XML parser (the `encoding`
declaration, BOM)". Parsers differ in what they support: Go's
`encoding/xml` reads only UTF-8 unless the program supplies a charset
reader, Python's expat reads UTF-8, UTF-16 and Latin-1. The rule is clear
for a document with a DTD (always `invalidData`), but not for a valid
project in another encoding: one port imports it, another rejects it.

**Go.** UTF-8, and UTF-16 with a BOM (transcoded with the standard
library). Any other declared encoding is `invalidData`.

**Proposal.** Name the set: UTF-8 and UTF-16 must be read; any other
encoding is `invalidData`. Object Manager writes UTF-8.

## 4. §4.5.1 item 3: a byte limit does not bound memory

**Problem.** The limits count bytes. A parsed tree costs far more: an
independent review measured 763 MB live heap for 16 MiB of `<a/>`, about
48× the inflated size; `[{},{},…]` in a node is similar. At the allowed
64 MiB that is gigabytes from a 64 KB file.

**Go.** At most 1 000 000 XML elements per document and 2 000 000 JSON
values per node, else `invalidData`.

**Proposal.** Add a recommended element/value limit next to the byte
limits, so that every port bounds memory the same way.

## 5. §4.3 item 5: "its headers" includes the local header, and a CRC of 0

**Problem.** The oracle and Python's `zipfile` check the central directory
only; the `lie` fixtures change both headers. A ZIP library may also skip a
declared CRC of 0 (Go's `archive/zip` does). Neither case has a fixture,
so a port that relies on its library passes while accepting both.

**Go.** Checks size and CRC against the central and the local header
(unless the entry has a data descriptor), and computes the CRC itself.

**Proposal.** Keep the rule, add fixtures `local-header-lies` and
`zero-crc`.

## 6. §6.3.13: the oracle's rule row accepts extra quoted fields

**Problem.** The spec says a row is exactly three fields. The oracle
(`parse_row`) takes the text from the first to the **last** quote, so
`{{1, 1, "0 20 * * *", "x"}}` is a rule with cron `0 20 * * *", "x` and
`{{1, 1, "0 20 "a" * *"}}` a rule with an embedded quote. No case covers
it (`reject-trailing-field` uses an unquoted `5`).

**Go.** Follows the text: the cron text ends at the next quote, and such
rows make the list unknown (`nil`).

**Proposal.** Fix the oracle and add the two inputs as `schedule.rules`
cases. Also say whether "space-separated" means one space or any
whitespace; Go and the oracle split on any whitespace.

## 7. §4.5.5: an empty `serialNumber` on an intermediate object

**Problem.** The CLU ancestor is "the nearest ancestor whose object has
`serialNumber` and `ipAddress`", and §4.5.1 item 7 says an empty element is
a missing field. The oracle tests that the elements exist, so an object with
`<serialNumber/>` and an IP hides the real CLU, and the Push entries below it
are skipped.

**Go.** Follows item 7: an empty field does not make an ancestor.

**Proposal.** Say "non-empty" explicitly in §4.5.5, fix the oracle, add a
fixture.

## 8. §10 item 3: precision is unbounded

**Problem.** `precision` comes from `data.json` as any integer. Python's
`round` returns at once for huge values; an exact implementation in another
language may allocate without bound.

**Go.** Beyond ±400 places, the value itself (or a signed zero).

**Proposal.** Note that `|precision| > 400` behaves as Python does, or
reject a widget whose precision is outside, say, −20…20.

## 9. Suite: no scenario for "a successful ping resets the timeout streak"

**Problem.** §5.4.5: "a successful ping resets it". A mutation of the Go
session that never resets the counter passes all 51 scenarios:
`ping-timeouts-count-per-socket` and `three-ping-timeouts-replace-the-main-socket`
only use unbroken streaks.

**Go.** Tests it itself (`clu/session_test.go`,
`TestAnsweredPingResetsTheTimeoutStreak`).

**Proposal.** A scenario `an-answered-ping-resets-the-streak`: two
timeouts, one reply, two timeouts, `expectQuiet` (no new socket), then a
third timeout closes `@main`.

## 10. conformance.md §4b: the barrier can be exact; say how

**Observation.** The Go runner has no real-time quiet window. The session
reports its runnable goroutines through a test hook (`clu.Activity`), the
runner delivers datagrams and fires timers synchronously, and "count = 0"
is the barrier. 51 scenarios run 20 times under the race detector in about
2 s; four mutations (ping interval, chunk number reuse, timeout-streak
reset, …) were checked against it.

**Proposal.** In §4b, describe this as the recommended barrier (an
activity count over every park point of the session), with the 200 ms
window as the fallback. It also belongs in `docs/implementing.md` under
"The session barrier".
